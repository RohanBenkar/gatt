package gatt

import (
	"bytes"
	"encoding/binary"
	"errors"
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

type device struct {
	deviceHandler

	conn xpc.XPC

	reqc chan message
	rspc chan message
	// Only used in client/centralManager implementation
	plist           map[string]*peripheral
	plistmu         *sync.Mutex
	allowDuplicates bool

	// Only used in server/peripheralManager implementation
	advUUIDs []UUID
	advUUID  UUID
	advMajor uint16
	advMinor uint16
	advPower int8
	advName  string
	advRaw   []byte

	lastServiceAttributeId int
	attributes             xpc.Array
}

func NewDevice() (*device, error) {
	d := &device{
		reqc:    make(chan message),
		rspc:    make(chan message),
		plist:   map[string]*peripheral{},
		plistmu: &sync.Mutex{},
	}
	d.conn = xpc.XpcConnect("com.apple.blued", d)
	return d, nil
}

func (d *device) Init(f func(Device, string)) error {
	go d.loop()
	rsp := d.sendReq(1, xpc.Dict{
		"kCBMsgArgName":    fmt.Sprintf("gopher-%v", time.Now().Unix()),
		"kCBMsgArgOptions": xpc.Dict{"kCBInitOptionShowPowerAlert": 0},
		"kCBMsgArgType":    0},
	)
	d.stateChanged = f
	go d.stateChanged(d, STATES[rsp.MustGetInt("kCBMsgArgState")])
	return nil
}

func (d *device) Advertise(opts ...advOption) error {
	for _, opt := range opts {
		opt(d)
	}
	switch {
	case len(d.advName) > 0 || len(d.advUUIDs) > 0:
		return d.advertise(d.advName, d.advUUIDs)
	case len(d.advRaw) > 0:
		return d.advertiseIBeaconData(d.advRaw)
	case len(d.advUUID.Bytes()) > 0:
		return d.advertiseIBeacon(d.advUUID, d.advMajor, d.advMinor, d.advPower)
	default:
		return errors.New("unsupported advertising option")
	}
}

func (d *device) advertise(name string, ss []UUID) error {
	uu := make([][]byte, len(ss))
	for i, s := range ss {
		uu[i] = make([]byte, 16)
		copy(uu[i], s.b)
	}
	rsp := d.sendReq(8, xpc.Dict{
		"kCBAdvDataLocalName":    name,
		"kCBAdvDataServiceUUIDs": uu},
	)
	if res := rsp.MustGetInt("kCBMsgArgResult"); res != 0 {
		return errors.New("FIXME: Advertise error")
	}
	return nil
}

func (d *device) advertiseIBeaconData(data []byte) error {
	rsp := d.sendReq(8, xpc.Dict{"kCBAdvDataAppleBeaconKey": data})
	if res := rsp.MustGetInt("kCBMsgArgResult"); res != 0 {
		return errors.New("FIXME: Advertise error")
	}
	return nil
}

func (d *device) advertiseIBeacon(uuid UUID, major, minor uint16, measuredPower int8) error {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uuid.b)
	binary.Write(buf, binary.BigEndian, major)
	binary.Write(buf, binary.BigEndian, minor)
	binary.Write(buf, binary.BigEndian, measuredPower)
	return d.advertiseIBeaconData(buf.Bytes())
}

func (d *device) StopAdvertising() error {
	rsp := d.sendReq(9, nil)
	if res := rsp.MustGetInt("kCBMsgArgResult"); res != 0 {
		return errors.New("FIXME: Stop Advertise error")
	}
	return nil
}

func (d *device) RemoveServices() error {
	rsp := d.sendReq(12, nil)
	if res := rsp.MustGetInt("kCBMsgArgResult"); res != 0 {
		return errors.New("FIXME: Remove Srvice error")
	}
	return nil
}

func (d *device) AddService(s *Service) error {
	return nil
	attributeId := 1
	arg := xpc.Dict{
		"kCBMsgArgAttributeID":     attributeId,
		"kCBMsgArgAttributeIDs":    []int{},
		"kCBMsgArgCharacteristics": nil,
		"kCBMsgArgType":            1, // 1 => primary, 0 => excluded
		"kCBMsgArgUUID":            s.uuid.String(),
	}

	cs := xpc.Array{}
	for _, c := range s.Characteristics() {
		props := 0
		permissions := 0

		ds := xpc.Array{}
		for _, d := range c.Descriptors() {
			ds = append(ds, xpc.Dict{"kCBMsgArgData": d.value, "kCBMsgArgUUID": d.UUID()})
		}

		characteristicArg := xpc.Dict{
			"kCBMsgArgAttributeID":              attributeId,
			"kCBMsgArgAttributePermissions":     permissions,
			"kCBMsgArgCharacteristicProperties": props,
			"kCBMsgArgData":                     nil, // GGG
			"kCBMsgArgDescriptors":              ds,
			"kCBMsgArgUUID":                     c.UUID(),
		}

		d.attributes = append(d.attributes, c)
		cs = append(cs, characteristicArg)
		attributeId += 1
	}

	arg["kCBMsgArgCharacteristics"] = cs
	rsp := d.sendReq(10, arg)
	if res := rsp.MustGetInt("kCBMsgArgResult"); res != 0 {
		return errors.New("FIXME: Add Srvice error")
	}
	return nil
}

func (d *device) SetServices(ss []*Service) error {
	d.RemoveServices()
	for _, s := range ss {
		d.AddService(s)
	}
	return nil
}

func (d *device) Scan(ss []UUID, dup bool) {
	args := xpc.Dict{
		"kCBMsgArgUUIDs": uuidSlice(ss),
		"kCBMsgArgOptions": xpc.Dict{
			"kCBScanOptionAllowDuplicates": map[bool]int{true: 1, false: 0}[dup],
		},
	}
	d.sendCmd(29, args)
}

func (d *device) StopScanning() {
	d.sendCmd(30, nil)
}

func (d *device) Connect(p Peripheral) {
	pp := p.(*peripheral)
	d.plist[pp.id.String()] = pp
	d.sendCmd(31,
		xpc.Dict{
			"kCBMsgArgDeviceUUID": pp.id,
			"kCBMsgArgOptions": xpc.Dict{
				"kCBConnectOptionNotifyOnDisconnection": 1,
			},
		})
}

func (d *device) CancelConnection(p Peripheral) {
	d.sendCmd(32, xpc.Dict{"kCBMsgArgDeviceUUID": p.(*peripheral).id})
}

// process device events and asynchronous errors
// (implements XpcEventHandler)
func (d *device) HandleXpcEvent(event xpc.Dict, err error) {
	if err != nil {
		log.Println("error:", err)
	}

	id := event.MustGetInt("kCBMsgId")
	args := event.MustGetDict("kCBMsgArgs")

	switch id {
	case 6: // StateChanged
		d.rspc <- message{id: id, args: args}
	case // device event
		16, // AdvertisingStarted
		17: // AdvertisingStopped
		d.rspc <- message{id: id, args: args}
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
					// UUID: fmt.Sprintf("%x", xsd[i].([]byte)),
					Data: xsd[i+1].([]byte),
				}
				a.ServiceData = append(a.ServiceData, sd)
			}
		}
		if d.peripheralDiscovered != nil {
			go d.peripheralDiscovered(&peripheral{id: xpc.UUID(u.b), d: d}, a, rssi)
		}

	case 38: // PeripheralConnected
		u := UUID{args.MustGetUUID("kCBMsgArgDeviceUUID")}
		p := &peripheral{
			id:    xpc.UUID(u.b),
			d:     d,
			reqc:  make(chan message),
			rspc:  make(chan message),
			quitc: make(chan struct{}),
			sub:   newSubscriber(),
		}
		d.plistmu.Lock()
		d.plist[u.String()] = p
		d.plistmu.Unlock()
		go p.loop()

		if d.peripheralConnected != nil {
			go d.peripheralConnected(p, nil)
		}

	case 40: // PeripheralDisconnected
		u := UUID{args.MustGetUUID("kCBMsgArgDeviceUUID")}
		d.plistmu.Lock()
		p := d.plist[u.String()]
		delete(d.plist, u.String())
		d.plistmu.Unlock()
		if d.peripheralDisconnected != nil {
			d.peripheralDisconnected(p, nil) // TODO: Get Result as error?
		}
		close(p.quitc)

	case // Peripheral events
		54, // RSSIRead
		55, // ServiceDiscovered
		62, // IncludedServiceDiscovered
		63, // CharacteristicsDiscovered
		70, // CharacteristicRead
		71, // CharacteristicWritten
		73, // NotifyValueSet
		75, // DescriptorsDiscovered
		78, // DescriptorRead
		79: // DescriptorWritten

		u := UUID{args.MustGetUUID("kCBMsgArgDeviceUUID")}
		d.plistmu.Lock()
		p := d.plist[u.String()]
		d.plistmu.Unlock()
		p.rspc <- message{id: id, args: args}

	default:
		log.Printf("Unhandled event: %#v", event)
	}
}

func (d *device) sendReq(id int, args xpc.Dict) xpc.Dict {
	m := message{id: id, args: args, rspc: make(chan xpc.Dict)}
	d.reqc <- m
	return <-m.rspc
}

func (d *device) sendCmd(id int, args xpc.Dict) {
	d.reqc <- message{id: id, args: args}
}

func (d *device) loop() {
	for req := range d.reqc {
		d.sendCBMsg(req.id, req.args)
		if req.rspc == nil {
			continue
		}
		m := <-d.rspc
		req.rspc <- m.args
	}
}

func (d *device) sendCBMsg(id int, args xpc.Dict) {
	d.conn.Send(xpc.Dict{"kCBMsgId": id, "kCBMsgArgs": args}, false)
}
