// This corresponds to the sample code found in doc.go.
// TODO: Clean this up and turn it into proper examples.

package main

import (
	"fmt"
	"log"
	"time"

	"../../gatt"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	n := 0

	d, err := gatt.NewDevice()
	if err != nil {
		fmt.Printf("Failed to open device, err :%s", err)
		return
	}

	// NOTE & TODO: OSX provides GAP and GATT services, and can't be customized.
	// For Linux/Embedded, however, this is definitly something we want to have fully control.
	gapSvc := gatt.NewService(gatt.AttrGAPUUID)
	gapSvc.AddCharacteristic(gatt.AttrDeviceNameUUID).SetValue([]byte("gopher"))
	gapSvc.AddCharacteristic(gatt.AttrAppearanceUUID).SetValue([]byte{0x00, 0x80})
	gapSvc.AddCharacteristic(gatt.AttrPeripheralPrivacyUUID).SetValue([]byte{0x00})
	gapSvc.AddCharacteristic(gatt.AttrReconnectionAddrUUID).SetValue([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	gapSvc.AddCharacteristic(gatt.AttrPeferredParamsUUID).SetValue([]byte{0x06, 0x00, 0x06, 0x00, 0x00, 0x00, 0xd0, 0x07})

	gattSvc := gatt.NewService(gatt.AttrGATTUUID)
	gattSvc.AddCharacteristic(gatt.AttrServiceChangedUUID).HandleNotifyFunc(
		func(r gatt.Request, n gatt.Notifier) {
			go func() {
				log.Printf("TODO: indicate client when the services are changed")
			}()
		})

	svc := gatt.NewService(gatt.MustParseUUID("09fc95c0-c111-11e3-9904-0002a5d5c51b"))
	// svc := gatt.NewService(gatt.MustParseUUID("FE11"))

	svc.AddCharacteristic(gatt.MustParseUUID("11fac9e0-c111-11e3-9246-0002a5d5c51b")).HandleReadFunc(
		// svc.AddCharacteristic(gatt.MustParseUUID("FE12")).HandleReadFunc(
		func(rsp gatt.ResponseWriter, req *gatt.ReadRequest) {
			fmt.Fprintf(rsp, "count: %d", n)
			n++
		})

	svc.AddCharacteristic(gatt.MustParseUUID("16fe0d80-c111-11e3-b8c8-0002a5d5c51b")).HandleWriteFunc(
		// svc.AddCharacteristic(gatt.MustParseUUID("FE13")).HandleWriteFunc(
		func(r gatt.Request, data []byte) (status byte) {
			log.Println("Wrote:", string(data))
			return gatt.StatusSuccess
		})

	svc.AddCharacteristic(gatt.MustParseUUID("1c927b50-c116-11e3-8a33-0800200c9a66")).HandleNotifyFunc(
		// svc.AddCharacteristic(gatt.MustParseUUID("FE14")).HandleNotifyFunc(
		func(r gatt.Request, n gatt.Notifier) {
			go func() {
				count := 0
				for !n.Done() {
					fmt.Fprintf(n, "Count: %d", count)
					count++
					time.Sleep(time.Second)
				}
			}()
		})

	// TODO: Works well for one chip, but on the other. Need to figure out why.
	altAdvertise := func() {
		advUUIDs := gatt.AdvUUIDs([]gatt.UUID{svc.UUID()})

		// RedBear Labs iBeacon
		advIBeacon := gatt.AdvIBeacon(
			gatt.MustParseUUID("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFF5A"),
			0x0001,
			0x0002,
			0x30)

		// Alternately broadcast SVC UUID and iBeacon
		for {
			d.Advertise(advUUIDs)
			time.Sleep(time.Second * 5)
			d.Advertise(advIBeacon)
			time.Sleep(time.Millisecond * 100)
		}
	}

	startServer := func(d gatt.Device, state string) {
		fmt.Printf("State: %s", state)
		switch state {
		case "poweredOn":
			d.AddService(gapSvc)
			d.AddService(gattSvc)
			d.AddService(svc)

			d.Advertise(gatt.AdvName("Gopher"), gatt.AdvUUIDs([]gatt.UUID{svc.UUID()}))
			// d.AdvertiseIBeacon(gatt.MustParseUUID("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFF5A"), 0x0001, 0x0002, 0x30)
			// go altAdvertise()
			_ = altAdvertise
		}
	}

	d.Option(
		gatt.CentralConnected(func(c gatt.Central) { log.Println("Connect: ", c) }),
		gatt.CentralDisconnected(func(c gatt.Central) { log.Println("Disconnect: ", c) }),
	)

	d.Init(startServer)

	select {}
}
