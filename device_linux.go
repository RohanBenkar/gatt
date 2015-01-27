package gatt

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/paypal/gatt/linux"
)

type Device struct {
	peripheralManager
	centralManager

	hci *linux.HCI

	// All the following fields are only used peripheralManager (server) implementation.
	svcs  []*Service
	attrs *attrRange

	maxConn int

	advSvcs     []UUID
	advPkt      []byte
	scanRespPkt []byte
	mfData      []byte

	advIntMin  uint16
	advIntMax  uint16
	advChnlMap uint8
}

func NewDevice() (*Device, error) {
	d := &Device{
		advIntMin:  0x00F4, // Spec default: 0x0800
		advIntMax:  0x00F4, // Spec default: 0x0800
		advChnlMap: 4,      // FIXME
		maxConn:    1,
	}
	h, err := linux.NewHCI(d.maxConn)
	if err != nil {
		return nil, err
	}
	d.hci = h
	return d, nil
}

func (d *Device) Init() error {
	d.hci.AcceptMasterHandler = func(l2c io.ReadWriteCloser, addr net.HardwareAddr) {
		c := newCentral(d, d.attrs, l2c, true)
		if d.Connected != nil {
			d.Connected(c)
		}
		c.loop()
		if d.Disconnected != nil {
			d.Disconnected(c)
		}
	}
	d.hci.AcceptSlaveHandler = func(l2c io.ReadWriteCloser, pd *linux.PlatData) {
		p := &peripheral{
			d:           d,
			pd:          pd,
			l2c:         l2c,
			reqc:        make(chan message),
			quitc:       make(chan struct{}),
			subscribe:   make(map[uint16]func([]byte, error)),
			subscribemu: &sync.Mutex{},
		}
		if d.PeripheralConnected != nil {
			go d.PeripheralConnected(p)
		}
		p.loop()
		if d.PeripheralDisconnected != nil {
			d.PeripheralDisconnected(p, nil)
		}
	}
	d.hci.AdvertisementHandler = func(pd *linux.PlatData) {
		a := &Advertisement{}
		a.Unmarshall(pd.Data)
		a.Connectable = pd.Connectable
		p := &peripheral{pd: pd}
		if d.PeripheralDiscovered != nil {
			pd.Name = a.LocalName
			d.PeripheralDiscovered(p, a, int(pd.RSSI))
		}
	}
	go d.heartbeat()
	if d.StateChanged != nil {
		d.StateChanged("poweredOn")
	}
	return nil
}

func (d *Device) Stop() error {
	if d.StateChanged != nil {
		defer d.StateChanged("poweredOff")
	}
	return d.hci.Close()
}

func (d *Device) AddService(svc *Service) *Service {
	d.svcs = append(d.svcs, svc)
	d.attrs = generateAttributes(d.Name, d.svcs, uint16(1)) // ble attrs start at 1
	return svc
}

func (d *Device) RemoveService(svc Service) error { return notImplemented }
func (d *Device) RemoveAllServices() error        { return notImplemented }
func (d *Device) Advertise() error {
	if len(d.advPkt) == 0 {
		d.setDefaultAdvertisement(d.Name)
	}
	return d.hci.Advertise()
}
func (d *Device) StopAdvertising() error { return d.hci.StopAdvertising() }

func (d *Device) Scan([]UUID)                              { d.hci.Scan() }
func (d *Device) StopScan()                                { d.hci.StopScan() }
func (d *Device) Connect(p Peripheral)                     { d.hci.Connect(p.(*peripheral).pd) } // FIXME
func (d *Device) CancelConnection(p Peripheral)            { d.hci.CancelConnection(p.(*peripheral).pd) }
func (d *Device) ConnectedPeripherals([]UUID) []Peripheral { return nil }
func (d *Device) Peripherals([]UUID) []Peripheral          { return nil }

// heartbeat monitors the status of the BLE controller
func (d *Device) heartbeat() {
	// Send d HCI command to controller periodically, if we don't get response
	// for d while, close the server to notify upper layer.
	t := time.AfterFunc(time.Second*30, func() {
		d.hci.Close()
		if d.Closed != nil {
			d.Closed(errors.New("Device does not respond"))
		}
	})
	for _ = range time.Tick(time.Second * 10) {
		d.hci.Ping()
		t.Reset(time.Second * 30)
	}
}

func (d *Device) setDefaultAdvertisement(name string) error {
	if len(d.scanRespPkt) == 0 {
		ScanResponsePacket(nameScanResponsePacket(name))(d)
	}
	if len(d.advPkt) == 0 {
		u := []UUID{}
		for _, svc := range d.svcs {
			u = append(u, UUID{svc.value})
		}
		ad, _ := serviceAdvertisingPacket(u)
		AdvertisingPacket(ad)(d)
	}
	return d.advertiseService()
}

func (d *Device) advertiseService() error {
	d.StopAdvertising()
	defer d.Advertise()

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

// Option sets the options specified.
func (d *Device) Option(opts ...option) {
	for _, opt := range opts {
		opt(d)
	}
	d.advertiseService()
}

// AdvertisingPacket is an optional custom advertising packet.
// If nil, the advertising data will constructed to advertise
// as many services as possible. The AdvertisingPacket must be no
// longer than MaxAdvertisingPacketLength.
// If ManufacturerData is also set, their total length must be no
// longer than MaxAdvertisingPacketLength.
func AdvertisingPacket(b []byte) option { return func(d *Device) { d.advPkt = b } }

// ScanResponsePacket is an optional custom scan response packet.
// If nil, the scan response packet will set to return the server
// name, truncated if necessary. The ScanResponsePacket must be no
// longer than MaxAdvertisingPacketLength.
func ScanResponsePacket(b []byte) option { return func(d *Device) { d.scanRespPkt = b } }

// ManufacturerData is an optional custom data.
// If set, it will be appended in the advertising data.
// The length of AdvertisingPacket ManufactureData must be no longer
// than MaxAdvertisingPacketLength .
func ManufacturerData(b []byte) option { return func(d *Device) { d.mfData = b } }

// AdvertisingIntervalMin is an optional parameter.
// If set, it overrides the default minimum advertising interval for
// undirected and low duty cycle directed advertising.
func AdvertisingIntervalMin(n uint16) option { return func(d *Device) { d.advIntMin = n } }

// AdvertisingIntervalMax is an optional parameter.
// If set, it overrides the default maximum advertising interval for
// undirected and low duty cycle directed advertising.
func AdvertisingIntervalMax(n uint16) option { return func(d *Device) { d.advIntMax = n } }

// AdvertisingChannelMap is an optional parameter.
// If set, it overrides the default advertising channel map.
func AdvertisingChannelMap(n uint8) option { return func(d *Device) { d.advChnlMap = n } }

// AdvertisingServices is an optional parameter.
// If set, it overrides the default advertising services.
func AdvertisingServices(u []UUID) option {
	return func(d *Device) {
		d.advSvcs = u
		d.advPkt, _ = serviceAdvertisingPacket(u)
	}
}
