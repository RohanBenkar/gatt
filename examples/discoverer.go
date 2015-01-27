package main

import (
	"fmt"
	"log"

	"../../gatt"
)

func main() {
	d, err := gatt.NewDevice()
	if err != nil {
		fmt.Printf("failed to open device, err: %s", err)
		return
	}

	d.StateChanged = func(state string) {
		log.Printf("StateChanged: %s", state)
		if state == "poweredOn" {
			d.Scan(nil)
			return
		}
		d.StopScan()
	}

	d.PeripheralDiscovered = func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
		fmt.Println()
		fmt.Println("peripheral discovered (", p.ID(), "):")
		fmt.Println("\thello my local name is:")
		fmt.Println("\t\t", a.LocalName)
		fmt.Println("\tcan I interest you in any of the following advertised services:")
		fmt.Println("\t\t", a.Services)

		sd := a.ServiceData
		switch {
		case len(sd) > 0:
			fmt.Println("\there is my service data:")
			for _, d := range sd {
				fmt.Println("\t\t", d.Uuid, ":", d.Data)
			}
		case len(a.ManufacturerData) > 0:
			fmt.Println("\there is my manufacturer data:")
			fmt.Println("\t\t", a.ManufacturerData)
		case a.TxPowerLevel != 0:
			fmt.Println("\tmy TX power level is:")
			fmt.Println("\t\t", a.TxPowerLevel)
		}
	}

	go d.Init()
	select {}
}
