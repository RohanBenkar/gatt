package gatt

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/paypal/gatt/xpc"
)

type central struct {
	id  UUID
	mtu int
}

func (c *central) Close() error { return nil }
func (c *central) MTU() int     { return c.mtu }

type Device struct {
	peripheralManager
	centralManager

	conn xpc.XPC

	// Only used in server implementation
	plist           map[string]*peripheral
	plistmu         *sync.Mutex
	allowDuplicates bool

	// Only used in client implementation
	lastServiceAttributeId int
	attributes             xpc.Array
}

func NewDevice() (*Device, error) {
	d := &Device{
		plist:   map[string]*peripheral{},
		plistmu: &sync.Mutex{},
	}
	d.conn = xpc.XpcConnect("com.apple.blued", d)
	return d, nil
}

func (d *Device) peripheral(dd xpc.Dict) *peripheral {
	u := UUID{dd.MustGetUUID("kCBMsgArgDeviceUUID")}
	d.plistmu.Lock()
	defer d.plistmu.Unlock()
	return d.plist[u.String()]
}

// process Device events and asynchronous errors
// (implements XpcEventHandler)
func (d *Device) HandleXpcEvent(event xpc.Dict, err error) {
	if err != nil {
		log.Println("error:", err)
	}

	id := event.MustGetInt("kCBMsgId")
	args := event.MustGetDict("kCBMsgArgs")

	switch id {
	case 6: // StateChanged
		if d.StateChanged != nil {
			go d.StateChanged(STATES[args.MustGetInt("kCBMsgArgState")])
		}
	case 16: // advertising started
		// result := args.MustGetInt("kCBMsgArgResult")
	case 17: // AdvertisingStopped
	case 37: // PeripheralDiscovered
		xa := args.MustGetDict("kCBMsgArgAdvertisementData")
		if len(xa) == 0 {
			return
		}
		u := UUID{args.MustGetUUID("kCBMsgArgDeviceUUID")}
		a := &Advertisement{
			LocalName:        xa.GetString("kCBAdvDataLocalName", args.GetString("kCBMsgArgName", "")),
			TxPowerLevel:     xa.GetInt("kCBAdvDataTxPowerLevel", 0),
			ManufacturerData: xa.GetBytes("kCBAdvDataManufacturerData", nil),
		}

		rssi := args.GetInt("kCBMsgArgRssi", 0)

		if xu, ok := xa["kCBAdvDataServiceUUIDs"]; ok {
			for _, xs := range xu.(xpc.Array) {
				s := UUID{reverse(xs.([]byte))}
				a.Services = append(a.Services, s)
			}
		}

		if xsds, ok := xa["kCBAdvDataServiceData"]; ok {
			xsd := xsds.(xpc.Array)
			for i := 0; i < len(xsd); i += 2 {
				sd := ServiceData{
					Uuid: fmt.Sprintf("%x", xsd[i].([]byte)),
					Data: xsd[i+1].([]byte),
				}
				a.ServiceData = append(a.ServiceData, sd)
			}
		}
		if d.PeripheralDiscovered != nil {
			go d.PeripheralDiscovered(&peripheral{id: u, d: d}, a, rssi)
		}

	case 38: // PeripheralConnected
		u := UUID{args.MustGetUUID("kCBMsgArgDeviceUUID")}
		p := &peripheral{
			id:          u,
			d:           d,
			reqc:        make(chan message),
			rspc:        make(chan message),
			quitc:       make(chan struct{}),
			subscribe:   make(map[uint16]func([]byte, error)),
			subscribemu: &sync.Mutex{},
		}
		d.plistmu.Lock()
		d.plist[u.String()] = p
		d.plistmu.Unlock()
		go p.loop()
		if d.PeripheralConnected != nil {
			go d.PeripheralConnected(p)
		}

	case 40: // PeripheralDisconnected
		u := UUID{args.MustGetUUID("kCBMsgArgDeviceUUID")}
		p := d.peripheral(args)
		d.plistmu.Lock()
		delete(d.plist, u.String())
		d.plistmu.Unlock()
		if d.PeripheralDisconnected != nil {
			d.PeripheralDisconnected(p, nil) // TODO: Get Result as error?
		}
		close(p.quitc)

	case 54, // RSSIRead
		55, // ServiceDiscovered
		62, // IncludedServiceDiscovered
		63, // CharacteristicsDiscovered
		70, // CharacteristicRead
		71, // CharacteristicWritten
		73, // NotifyValueSet
		75, // DescriptorsDiscovered
		78, // DescriptorRead
		79: // DescriptorWritten
		p := d.peripheral(args)
		p.rspc <- message{id: id, args: args}
	default:
		log.Printf("Unhandled msg: %d\n%#v \n", id, args)
	}
}

// send a message to Blued
func (d *Device) sendCBMsg(id int, args xpc.Dict) {
	d.conn.Send(xpc.Dict{"kCBMsgId": id, "kCBMsgArgs": args}, false)
}

// initialize Device
func (d *Device) Init() {
	d.sendCBMsg(1, xpc.Dict{"kCBMsgArgName": fmt.Sprintf("goble-%v", time.Now().Unix()),
		"kCBMsgArgOptions": xpc.Dict{"kCBInitOptionShowPowerAlert": 0}, "kCBMsgArgType": 0})
}

// start advertising
func (d *Device) StartAdvertising(name string, serviceUuids []UUID) {
	uuids := make([]string, len(serviceUuids))
	for i, uuid := range serviceUuids {
		uuids[i] = uuid.String()
	}
	d.sendCBMsg(8, xpc.Dict{"kCBAdvDataLocalName": name, "kCBAdvDataServiceUUIDs": uuids})
}

// start advertising as IBeacon (raw data)
func (d *Device) StartAdvertisingIBeaconData(data []byte) {
	d.sendCBMsg(8, xpc.Dict{"kCBAdvDataAppleBeaconKey": data})
}

// start advertising as IBeacon
func (d *Device) StartAdvertisingIBeacon(uuid UUID, major, minor uint16, measuredPower int8) {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uuid.b)
	binary.Write(buf, binary.BigEndian, major)
	binary.Write(buf, binary.BigEndian, minor)
	binary.Write(buf, binary.BigEndian, measuredPower)

	d.sendCBMsg(8, xpc.Dict{"kCBAdvDataAppleBeaconKey": buf.Bytes()})
}

// stop advertising
func (d *Device) StopAdvertising() { d.sendCBMsg(9, nil) }

// remove all services
func (d *Device) RemoveServices() { d.sendCBMsg(12, nil) }

// set services
func (d *Device) SetServices(services []Service) {
	d.sendCBMsg(12, nil) // remove all services
	d.attributes = xpc.Array{}

	attributeId := 1
	for _, service := range services {
		arg := xpc.Dict{
			"kCBMsgArgAttributeID":     attributeId,
			"kCBMsgArgAttributeIDs":    []int{},
			"kCBMsgArgCharacteristics": nil,
			"kCBMsgArgType":            1, // 1 => primary, 0 => excluded
			"kCBMsgArgUUID":            service.typ.String(),
		}

		d.attributes = append(d.attributes, service)
		d.lastServiceAttributeId = attributeId
		attributeId += 1

		characteristics := xpc.Array{}
		for _, characteristic := range service.Characteristics() {
			props := 0
			permissions := 0

			if CharRead&characteristic.props != 0 {
				props |= 0x02

				if CharRead&characteristic.secure != 0 {
					permissions |= 0x04
				} else {
					permissions |= 0x01
				}
			}

			if CharWriteNR&characteristic.props != 0 {
				props |= 0x04

				if CharWriteNR&characteristic.secure != 0 {
					permissions |= 0x08
				} else {
					permissions |= 0x02
				}
			}

			if CharWrite&characteristic.props != 0 {
				props |= 0x08

				if CharWriteNR&characteristic.secure != 0 {
					permissions |= 0x08
				} else {
					permissions |= 0x02
				}
			}

			if CharNotify&characteristic.props != 0 {
				if CharNotify&characteristic.secure != 0 {
					props |= 0x100
				} else {
					props |= 0x10
				}
			}

			if CharIndicate&characteristic.props != 0 {
				if CharIndicate&characteristic.secure != 0 {
					props |= 0x200
				} else {
					props |= 0x20
				}
			}

			descriptors := xpc.Array{}
			for _, descriptor := range characteristic.Descriptors() {
				descriptors = append(descriptors, xpc.Dict{"kCBMsgArgData": descriptor.value, "kCBMsgArgUUID": descriptor.typ.String()})
			}

			characteristicArg := xpc.Dict{
				"kCBMsgArgAttributeID":              attributeId,
				"kCBMsgArgAttributePermissions":     permissions,
				"kCBMsgArgCharacteristicProperties": props,
				"kCBMsgArgData":                     characteristic.value,
				"kCBMsgArgDescriptors":              descriptors,
				"kCBMsgArgUUID":                     characteristic.typ.String(),
			}

			d.attributes = append(d.attributes, characteristic)
			characteristics = append(characteristics, characteristicArg)
			attributeId += 1
		}

		arg["kCBMsgArgCharacteristics"] = characteristics
		d.sendCBMsg(10, arg) // remove all services
	}
}

// start scanning
// func (d *Device) Scan(serviceUuids []UUID, allowDuplicates bool) {
func (d *Device) Scan(serviceUuids []UUID) {
	uuids := []string{}
	for _, uuid := range serviceUuids {
		uuids = append(uuids, uuid.String())
	}
	args := xpc.Dict{"kCBMsgArgUUIDs": uuids}
	// if allowDuplicates {
	// 	args["kCBMsgArgOptions"] = xpc.Dict{"kCBScanOptionAllowDuplicates": 1}
	// } else {
	args["kCBMsgArgOptions"] = xpc.Dict{}
	// }
	// d.allowDuplicates = allowDuplicates
	d.allowDuplicates = true
	d.sendCBMsg(29, args)
}

// stop scanning
func (d *Device) StopScan() { d.sendCBMsg(30, nil) }

// connect
func (d *Device) Connect(p Peripheral) {
	pp := p.(*peripheral)
	d.plist[pp.id.String()] = pp
	d.sendCBMsg(31,
		xpc.Dict{
			"kCBMsgArgDeviceUUID": pp.id.b,
			"kCBMsgArgOptions":    xpc.Dict{"kCBConnectOptionNotifyOnDisconnection": 1},
		})
}

// disconnect
func (d *Device) CancelConnection(p Peripheral) {
	d.sendCBMsg(32, xpc.Dict{"kCBMsgArgDeviceUUID": p.(*peripheral).id.b})
}

// update rssi
func (d *Device) UpdateRssi(p Peripheral) {
	d.sendCBMsg(43, xpc.Dict{"kCBMsgArgDeviceUUID": p.(*peripheral).id.b})
}
