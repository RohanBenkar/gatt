package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"../../gatt"
)

func explorer(p gatt.Peripheral) {
	// Discovery services
	log.Printf("Discovery")
	ss, err := p.DiscoverServices(nil)
	if err != nil {
		fmt.Printf("Failed to discover services, err: %s", err)
		return
	}
	log.Printf("Discovery done")

	for _, s := range ss {
		msg := "Service: " + s.UUID().String()
		if len(s.Name()) > 0 {
			msg += " (" + s.Name() + ")"
		}
		fmt.Println(msg)

		// Discovery characteristics
		cs, err := p.DiscoverCharacteristics(nil, s)
		if err != nil {
			fmt.Printf("Failed to discover characteristics, err: %s", err)
			continue
		}

		for _, c := range cs {
			msg := "  Characteristic  " + c.UUID().String()
			if len(c.Name()) > 0 {
				msg += " (" + c.Name() + ")"
			}
			msg += "\n    properties    " + c.Properties().String()
			fmt.Println(msg)

			// Read the characteristic, if possible.
			if (c.Properties() & gatt.CharRead) != 0 {
				b, err := p.ReadCharacteristic(c)
				if err != nil {
					fmt.Printf("Failed to read characteristic, err: %s", err)
					continue
				}
				fmt.Printf("    value         %x | %q\n", b, b)
			}

			// Discovery descriptors
			ds, err := p.DiscoverDescriptors(nil, c)
			if err != nil {
				fmt.Printf("Failed to discover descriptors, err: %s", err)
				continue
			}

			// Subscribe the characteristic, if possible.
			// This can only be done after the character's descriptors are discovered.
			if c.Properties()&gatt.CharNotify != 0 {
				f := func(c *gatt.Characteristic, b []byte, err error) {
					if err != nil {
						fmt.Printf("    notified error: %s\n", err)
						return
					}
					fmt.Printf("    notified      %x | %q\n", b, b)
				}
				p.SetNotifyValue(c, f)
				// Unsubscribe after 3 seconds
				time.Sleep(time.Second * 3)
				p.SetNotifyValue(c, nil)
			}

			for _, d := range ds {
				msg := "  Descriptor      " + d.UUID().String()
				if len(d.Name()) > 0 {
					msg += " (" + d.Name() + ")"
				}
				fmt.Println(msg)

				// Read descriptor (could fail)
				b, err := p.ReadDescriptor(d)
				if err != nil {
					fmt.Printf("Failed to read descriptor, err: %s", err)
					continue
				}
				fmt.Printf("    value         %x | %q\n", b, b)
			}
		}
	}
}

func main() {

	// verbose := flag.Bool("verbose", false, "dump all events")
	// dups := flag.Bool("allow-duplicates", false, "allow duplicates when scanning")
	flag.Parse()

	if len(flag.Args()) != 1 {
		fmt.Println("usage:", os.Args[0], "[options] peripheral-id")
		os.Exit(1)
	}

	id := flag.Args()[0]

	done := make(chan struct{})
	tmo := time.Second * 10
	t := time.AfterFunc(tmo, func() {
		fmt.Println("Timeout")
		close(done)
	})

	d, err := gatt.NewDevice()
	if err != nil {
		fmt.Printf("Failed to open device, err :%s", err)
		return
	}

	d.StateChanged = func(state string) {
		fmt.Println("State:", state)
		if state == "poweredOn" {
			fmt.Println("scaning...")
			d.Scan(nil)
			return
		}
		d.StopScan()
	}

	d.PeripheralDiscovered = func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {

		// Search for Service ID
		// found := false
		// svc := gatt.UUID16(0xFE11)
		// svc := gatt.MustParseUUID("09fc95c0c11111e399040002a5d5c51b")
		// for _, u := range a.Services {
		// 	if svc.Equal(u) {
		// 		found = true
		// 		break
		// 	}
		// }
		// if !found {
		// 	return
		// }

		// Search for peripheral id (UUID on OSX, MAC for Linux)
		if p.ID() != id {
			return
		}

		t.Stop()
		d.StopScan()
		fmt.Printf("\nPeripheral found ID:%s, NAME:(%s)\n", p.ID(), p.Name())
		fmt.Println("  Local Name        =", a.LocalName)
		fmt.Println("  TX Power Level    =", a.TxPowerLevel)
		fmt.Println("  Manufacturer Data =", a.ManufacturerData)
		fmt.Println("  Service Data      =", a.ServiceData)
		d.Connect(p)
	}

	d.PeripheralDisconnected = func(p gatt.Peripheral, err error) {
		fmt.Println("Disconnected")
		close(done)
	}

	d.PeripheralConnected = func(p gatt.Peripheral) {
		fmt.Println("Connected")
		explorer(p)
		d.CancelConnection(p)
	}

	go d.Init()
	fmt.Println("Waiting...")
	<-done
	fmt.Println("Done")
}
