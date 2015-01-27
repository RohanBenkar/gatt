package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"../../gatt"
)

var done = make(chan struct{})

func stateChanged(d gatt.Device, state string) {
	fmt.Println("State:", state)
	if state == "poweredOn" {
		fmt.Println("scaning...")
		// svc := gatt.MustParseUUID("09fc95c0-c111-11e3-9904-0002a5d5c51b")
		d.Scan([]gatt.UUID{}, false)
		return
	}
	d.StopScanning()
}

func periphDiscovered(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
	id := flag.Args()[0]
	// Search for peripheral id (UUID on OSX, MAC for Linux)
	if p.ID() != id {
		return
	}

	log.Printf("P: %v", p.ID())
	p.Device().StopScanning()
	fmt.Printf("\nPeripheral found ID:%s, NAME:(%s)\n", p.ID(), p.Name())
	fmt.Println("  Local Name        =", a.LocalName)
	fmt.Println("  TX Power Level    =", a.TxPowerLevel)
	fmt.Println("  Manufacturer Data =", a.ManufacturerData)
	fmt.Println("  Service Data      =", a.ServiceData)

	p.Device().Connect(p)
}

func periphConnected(p gatt.Peripheral, err error) {
	fmt.Println("Connected")
	defer p.Device().CancelConnection(p)

	// Discovery services
	ss, err := p.DiscoverServices(nil)
	if err != nil {
		fmt.Printf("Failed to discover services, err: %s", err)
		return
	}

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
			if c.Properties()&gatt.CharNotify != 0 && !c.UUID().Equal(gatt.UUID16(0x2a05)) {
				f := func(c *gatt.Characteristic, b []byte, err error) {
					if err != nil {
						fmt.Printf("    notified error: %s\n", err)
						return
					}
					fmt.Printf("    notified      %x | %q\n", b, b)
				}
				p.SetNotifyValue(c, f)
				// Unsubscribe after 2 seconds
				time.Sleep(time.Second * 2)
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

func periphDisconnected(p gatt.Peripheral, err error) {
	fmt.Println("Disconnected")
	close(done)
}

func main() {

	// verbose := flag.Bool("verbose", false, "dump all events")
	// dups := flag.Bool("allow-duplicates", false, "allow duplicates when scanning")
	flag.Parse()

	if len(flag.Args()) != 1 {
		fmt.Println("usage:", os.Args[0], "[options] peripheral-id")
		os.Exit(1)
	}

	// tmo := time.Second * 10
	// t := time.AfterFunc(tmo, func() {
	// 	fmt.Println("Timeout")
	// 	close(done)
	// })

	d, err := gatt.NewDevice()
	if err != nil {
		fmt.Printf("Failed to open device, err :%s", err)
		return
	}

	d.Option(
		gatt.PeripheralDiscovered(periphDiscovered),
		gatt.PeripheralConnected(periphConnected),
		gatt.PeripheralDisconnected(periphDisconnected),
	)

	d.Init(stateChanged)
	fmt.Println("Waiting...")
	<-done
	fmt.Println("Done")
}
