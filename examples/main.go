package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"../../gatt"
)

func main() {
	verbose := flag.Bool("verbose", false, "dump all events")
	advertise := flag.Int("advertise", 0, "Duration of advertising - 0: does not advertise")
	dups := flag.Bool("allow-duplicates", false, "allow duplicates when scanning")
	ibeacon := flag.Int("ibeacon", 0, "Duration of IBeacon advertising - 0: does not advertise")
	scan := flag.Int("scan", 10, "Duration of scanning - 0: does not scan")
	uuid := flag.String("uuid", "", "device uuid (for ibeacon uuid,major,minor,power)")
	connect := flag.Bool("connect", false, "connect to device")
	disconnect := flag.Bool("disconnect", false, "disconnect from device")
	rssi := flag.Bool("rssi", false, "update rssi for device")
	remove := flag.Bool("remove", false, "Remove all services")
	discover := flag.Bool("discover", false, "Discover services")

	flag.Parse()

	ble := gatt.New()

	ble.SetVerbose(*verbose)

	log.Println("Init...")
	ble.Init()

	if *advertise > 0 {
		uuids := []gatt.UUID{}

		if len(*uuid) > 0 {
			uuids = append(uuids, gatt.MustParseUUID(*uuid))
		}

		time.Sleep(1 * time.Second)
		log.Println("Start Advertising...")
		ble.StartAdvertising("gobble", uuids)

		time.Sleep(time.Duration(*advertise) * time.Second)
		log.Println("Stop Advertising...")
		ble.StopAdvertising()
	}

	if *ibeacon > 0 {
		parts := strings.Split(*uuid, ",")
		id := parts[0]

		var major, minor uint16
		var measuredPower int8

		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &major)
		}
		if len(parts) > 2 {
			fmt.Sscanf(parts[2], "%d", &minor)
		}
		if len(parts) > 2 {
			fmt.Sscanf(parts[3], "%d", &measuredPower)
		}

		time.Sleep(1 * time.Second)
		log.Println("Start Advertising IBeacon...")
		ble.StartAdvertisingIBeacon(gatt.MustParseUUID(id), major, minor, measuredPower)

		time.Sleep(time.Duration(*ibeacon) * time.Second)
		log.Println("Stop Advertising...")
		ble.StopAdvertising()
	}

	if *scan > 0 {
		time.Sleep(1 * time.Second)
		log.Println("Start Scanning...")
		ble.StartScanning(nil, *dups)

		time.Sleep(time.Duration(*scan) * time.Second)
		log.Println("Stop Scanning...")
		ble.StopScanning()
	}

	if *connect {
		time.Sleep(1 * time.Second)
		u := gatt.MustParseUUID(*uuid)
		log.Println("Connect", u)
		ble.Connect(gatt.NewPeripheral(u))
		time.Sleep(5 * time.Second)
	}

	if *rssi {
		time.Sleep(1 * time.Second)
		u := gatt.MustParseUUID(*uuid)
		log.Println("UpdateRssi", u)
		ble.UpdateRssi(gatt.NewPeripheral(u))
		time.Sleep(5 * time.Second)
	}

	if *discover {
		time.Sleep(1 * time.Second)
		u := gatt.MustParseUUID(*uuid)
		log.Println("DiscoverServices", u)
		ble.DiscoverServices(gatt.NewPeripheral(u), nil)
		time.Sleep(5 * time.Second)
	}

	if *disconnect {
		time.Sleep(1 * time.Second)
		u := gatt.MustParseUUID(*uuid)
		log.Println("Disconnect", u)
		ble.Disconnect(gatt.NewPeripheral(u))
		time.Sleep(5 * time.Second)
	}

	if *remove {
		time.Sleep(1 * time.Second)
		log.Println("Remove all services")
		ble.RemoveServices()
		time.Sleep(5 * time.Second)
	}

	time.Sleep(5 * time.Second)
	log.Println("Goodbye!")
}
