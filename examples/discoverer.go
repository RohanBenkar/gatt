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

	d.HandleStateChanged(func(state string) {
		log.Printf("StateChanged: %s", state)
		if state == "poweredOn" {
			svc := gatt.MustParseUUID("09fc95c0-c111-11e3-9904-0002a5d5c51b")
			// svc := gatt.UUID16(0xFE11)
			d.Scan([]gatt.UUID{svc}, false)
			// d.Scan([]gatt.UUID{gatt.UUID16(0xFE11)}, false)
			return
		}
		d.StopScanning()
	})

	d.HandlePeripheralDiscovered(func(p gatt.Peripheral, a *gatt.Advertisement, rssi int) {
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
				fmt.Println("\t\t", d.UUID, ":", d.Data)
			}
		case len(a.ManufacturerData) > 0:
			fmt.Println("\there is my manufacturer data:")
			fmt.Println("\t\t", a.ManufacturerData)
		case a.TxPowerLevel != 0:
			fmt.Println("\tmy TX power level is:")
			fmt.Println("\t\t", a.TxPowerLevel)
		}
	})

	go d.Init()
	select {}
}
