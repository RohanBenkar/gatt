package gatt

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/paypal/gatt/linux"
)

type device struct {
	deviceHandler

	hci   *linux.HCI
	state string

	// All the following fields are only used peripheralManager (server) implementation.
	svcs  []*Service
	attrs *attrRange

	maxConn int

	advUUIDs []UUID
	advUUID  UUID
	advMajor uint16
	advMinor uint16
	advPower int8
	advName  string
	advRaw   []byte

	advPkt      []byte
	scanRespPkt []byte
	mfData      []byte

	advIntMin  uint16
	advIntMax  uint16
	advChnlMap uint8
}

func NewDevice() (Device, error) {
	d := &device{
		advIntMin:  0x00F4, // Spec default: 0x0800
		advIntMax:  0x00F4, // Spec default: 0x0800
		advChnlMap: 7,
		maxConn:    1,
	}
	h, err := linux.NewHCI(d.maxConn)
	if err != nil {
		return nil, err
	}
	d.hci = h
	return d, nil
}

func (d *device) Init(f func(Device, string)) error {
	d.hci.AcceptMasterHandler = func(pd *linux.PlatData) {
		c := newCentral(d, d.attrs, pd.Conn, true)
		if d.centralConnected != nil {
			d.centralConnected(c)
		}
		c.loop()
		if d.centralDisconnected != nil {
			d.centralDisconnected(c)
		}
	}
	d.hci.AcceptSlaveHandler = func(pd *linux.PlatData) {
		p := &peripheral{
			d:     d,
			pd:    pd,
			l2c:   pd.Conn,
			reqc:  make(chan message),
			quitc: make(chan struct{}),
			sub:   newSubscriber(),
		}
		if d.peripheralConnected != nil {
			go d.peripheralConnected(p, nil)
		}
		p.loop()
		if d.peripheralDisconnected != nil {
			d.peripheralDisconnected(p, nil)
		}
	}
	d.hci.AdvertisementHandler = func(pd *linux.PlatData) {
		a := &Advertisement{}
		a.Unmarshall(pd.Data)
		a.Connectable = pd.Connectable
		p := &peripheral{pd: pd, d: d}
		if d.peripheralDiscovered != nil {
			pd.Name = a.LocalName
			d.peripheralDiscovered(p, a, int(pd.RSSI))
		}
	}
	go d.heartbeat()
	d.state = "poweredOn"
	d.stateChanged = f
	d.stateChanged(d, d.state)
	return nil
}

func (d *device) Stop() error {
	d.state = "poweredOff"
	defer d.stateChanged(d, d.state)
	return d.hci.Close()
}

func (d *device) AddService(s *Service) error {
	d.svcs = append(d.svcs, s)
	d.attrs = generateAttributes(d.name, d.svcs, uint16(1)) // ble attrs start at 1
	return nil
}

func (d *device) RemoveServices() error {
	d.svcs = nil
	d.attrs = nil
	return nil
}

func (d *device) SetServices(s []*Service) error {
	d.RemoveServices()
	d.svcs = append(d.svcs, s...)
	d.attrs = generateAttributes(d.name, d.svcs, uint16(1)) // ble attrs start at 1
	return nil
}

func (d *device) Advertise(opts ...advOption) error {
	for _, opt := range opts {
		opt(d)
	}
	defer func() {
		d.advUUIDs = nil
		d.advName = ""
		d.advRaw = nil
	}()
	switch {
	case len(d.advName) > 0 || len(d.advUUIDs) > 0:
		return d.advertise(d.advName, d.advUUIDs)
	case len(d.advRaw) > 0:
		return d.advertiseIBeaconData(d.advRaw)
	default:
		return d.advertiseIBeacon(d.advUUID, d.advMajor, d.advMinor, d.advPower)
	}
	return errors.New("unsupported advertising option")
}

func (d *device) advertise(name string, u []UUID) error {
	ad, _ := serviceAdvertisingPacket(u)
	d.advPkt = ad
	d.setDefaultAdvertisement(d.name)
	return d.hci.SetAdvertiseEnable(true)
}

func (d *device) advertiseIBeaconData(b []byte) error {
	d.advPkt = b
	d.advertiseService()
	return d.hci.SetAdvertiseEnable(true)
}

func (d *device) advertiseIBeacon(u UUID, major, minor uint16, power int8) error {
	b := [30]byte{}
	b[0] = 0x02 // field length
	b[1] = 0x01 // type flag
	b[2] = 0x06 // LEOnly | GeneralDiscoverable

	b[3] = 0x1A // field length
	b[4] = 0xFF // type manufacturer data
	binary.LittleEndian.PutUint16(b[5:], uint16(0x004C))
	b[7] = 0x02 // data type
	b[8] = 0x15 // data length
	copy(b[9:], u.b)
	binary.BigEndian.PutUint16(b[25:], major)
	binary.BigEndian.PutUint16(b[27:], minor)
	b[29] = uint8(power)

	return d.advertiseIBeaconData(b[:])
}

func (d *device) StopAdvertising() error {
	return d.hci.SetAdvertiseEnable(false)
}

func (d *device) Scan(ss []UUID, dup bool) {
	// TODO: filter
	d.hci.SetScanEnable(true, dup)
}

func (d *device) StopScanning() {
	d.hci.SetScanEnable(false, true)
}

func (d *device) Connect(p Peripheral) {
	d.hci.Connect(p.(*peripheral).pd)
}

func (d *device) CancelConnection(p Peripheral) {
	d.hci.CancelConnection(p.(*peripheral).pd)
}

// func (d *device) ConnectedPeripherals([]UUID) []Peripheral {
// 	return nil
// }

// func (d *device) Peripherals([]UUID) []Peripheral {
// 	return nil
// }

// heartbeat monitors the status of the BLE controller
func (d *device) heartbeat() {
	// Send a HCI command to controller periodically, if we don't get response
	// for a while, close the server to notify upper layer.
	t := time.AfterFunc(time.Second*5, func() {
		d.hci.Close()
	})
	for _ = range time.Tick(time.Second * 3) {
		d.hci.Ping()
		t.Reset(time.Second * 5)
	}
}

func (d *device) setDefaultAdvertisement(name string) error {
	if len(d.scanRespPkt) == 0 {
		ScanResponsePacket(nameScanResponsePacket(name))(d)
	}
	if len(d.advPkt) == 0 {
		u := []UUID{}
		for _, s := range d.svcs {
			u = append(u, s.uuid)
		}
		ad, _ := serviceAdvertisingPacket(u)
		AdvertisingPacket(ad)(d)
	}
	return d.advertiseService()
}

func (d *device) advertiseService() error {
	d.hci.SetAdvertiseEnable(false)
	defer d.hci.SetAdvertiseEnable(true)

	if err := d.hci.SetAdvertisingParameters(
		d.advIntMin,
		d.advIntMax,
		d.advChnlMap); err != nil {
		return err
	}

	if len(d.scanRespPkt) > 0 {
		// Scan response command takes exactly 31 bytes data
		// The length indicating the significant part of the data.
		data := [31]byte{}
		n := copy(data[:31], d.scanRespPkt)
		if err := d.hci.SetScanResponsePacket(uint8(n), data); err != nil {
			return err
		}
	}

	if len(d.advPkt) > 0 {
		// Advertising data command takes exactly 31 bytes data, including manufacture data.
		// The length indicating the significant part of the data.
		data := [31]byte{}
		n := copy(data[:31], append(d.advPkt, d.mfData...))
		if err := d.hci.SetAdvertisingData(uint8(n), data); err != nil {
			return err
		}
	}

	return nil
}

// AdvertisingPacket is an optional custom advertising packet.
// If nil, the advertising data will constructed to advertise
// as many services as possible. The AdvertisingPacket must be no
// longer than MaxAdvertisingPacketLength.
// If ManufacturerData is also set, their total length must be no
// longer than MaxAdvertisingPacketLength.
func AdvertisingPacket(b []byte) option {
	return func(d Device) { d.(*device).advPkt = b }
}

// ScanResponsePacket is an optional custom scan response packet.
// If nil, the scan response packet will set to return the server
// name, truncated if necessary. The ScanResponsePacket must be no
// longer than MaxAdvertisingPacketLength.
func ScanResponsePacket(b []byte) option {
	return func(d Device) { d.(*device).scanRespPkt = b }
}

// ManufacturerData is an optional custom data.
// If set, it will be appended in the advertising data.
// The length of AdvertisingPacket ManufactureData must be no longer
// than MaxAdvertisingPacketLength .
func ManufacturerData(b []byte) option {
	return func(d Device) { d.(*device).mfData = b }
}

// AdvertisingIntervalMin is an optional parameter.
// If set, it overrides the default minimum advertising interval for
// undirected and low duty cycle directed advertising.
func AdvertisingIntervalMin(n uint16) option {
	return func(d Device) { d.(*device).advIntMin = n }
}

// AdvertisingIntervalMax is an optional parameter.
// If set, it overrides the default maximum advertising interval for
// undirected and low duty cycle directed advertising.
func AdvertisingIntervalMax(n uint16) option {
	return func(d Device) { d.(*device).advIntMax = n }
}

// AdvertisingChannelMap is an optional parameter.
// If set, it overrides the default advertising channel map.
func AdvertisingChannelMap(n uint8) option {
	return func(d Device) { d.(*device).advChnlMap = n }
}

// AdvertisingServices is an optional parameter.
// If set, it overrides the default advertising services.
func AdvertisingServices(u []UUID) option {
	return func(d Device) {
		d.(*device).advUUIDs = u
		d.(*device).advPkt, _ = serviceAdvertisingPacket(u)
	}
}
