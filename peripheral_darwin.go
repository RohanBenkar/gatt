package gatt

import (
	"errors"
	"log"
	"sync"

	"github.com/paypal/gatt/xpc"
)

type peripheral struct {
	// NameChanged is called whenever the peripheral GAP device name has changed.
	NameChanged func(*peripheral)

	// ServicedModified is called when one or more service of a peripheral have changed.
	// A list of invalid service is provided in the parameter.
	ServicesModified func(*peripheral, []*Service)

	d     *Device
	svcs  []*Service
	state string

	subscribe   map[uint16]func([]byte, error)
	subscribemu *sync.Mutex

	id   UUID
	name string

	reqc  chan message
	rspc  chan message
	quitc chan struct{}
}

type message struct {
	id   int
	args xpc.Dict
	rsp  chan xpc.Dict
}

func NewPeripheral(u UUID) Peripheral { return &peripheral{id: u} }

func (p *peripheral) ID() string           { return p.id.String() }
func (p *peripheral) Name() string         { return p.name }
func (p *peripheral) Services() []*Service { return p.svcs }
func (p *peripheral) State() string        { return p.state }

func (p *peripheral) DiscoverServices(ss []UUID) ([]*Service, error) {
	us := make([]string, len(ss))
	for i, u := range ss {
		us[i] = u.String()
	}
	rsp := p.sendReq(44, xpc.Dict{
		"kCBMsgArgDeviceUUID": p.id.b,
		"kCBMsgArgUUIDs":      us,
	})
	svcs := []*Service{}
	for _, xss := range rsp["kCBMsgArgServices"].(xpc.Array) {
		xs := xss.(xpc.Dict)
		u := MustParseUUID(xs.MustGetHexBytes("kCBMsgArgUUID"))
		n := uint16(xs.MustGetInt("kCBMsgArgServiceStartHandle"))
		s := &Service{
			attr: attr{
				h:     n,
				typ:   AttrCharacteristicUUID,
				props: CharRead,
				value: u.b,
			},
			endh:  uint16(xs.MustGetInt("kCBMsgArgServiceEndHandle")),
			chars: []*Characteristic{},
		}
		svcs = append(svcs, s)
	}
	p.svcs = svcs
	return svcs, nil
}

func (p *peripheral) DiscoverIncludedServices(ss []UUID, s *Service) ([]*Service, error) {
	us := make([]string, len(ss))
	for i, u := range ss {
		us[i] = u.String()
	}
	rsp := p.sendReq(60, xpc.Dict{
		"kCBMsgArgDeviceUUID":         p.id.b,
		"kCBMsgArgServiceStartHandle": s.h,
		"kCBMsgArgServiceEndHandle":   s.endh,
		"kCBMsgArgUUIDs":              us,
	})
	_ = rsp
	return nil, nil
}

func (p *peripheral) DiscoverCharacteristics(cs []UUID, s *Service) ([]*Characteristic, error) {
	us := make([]string, len(cs))
	for i, u := range cs {
		us[i] = u.String()
	}
	rsp := p.sendReq(61, xpc.Dict{
		"kCBMsgArgDeviceUUID":         p.id.b,
		"kCBMsgArgServiceStartHandle": s.h,
		"kCBMsgArgServiceEndHandle":   s.endh,
		"kCBMsgArgUUIDs":              us,
	})
	// sh := uint16(rsp.MustGetInt("kCBMsgArgServiceStartHandle"))
	for _, xcs := range rsp.MustGetArray("kCBMsgArgCharacteristics") {
		xc := xcs.(xpc.Dict)
		u := MustParseUUID(xc.MustGetHexBytes("kCBMsgArgUUID"))
		vn := uint16(xc.MustGetInt("kCBMsgArgCharacteristicValueHandle"))
		cn := uint16(xc.MustGetInt("kCBMsgArgCharacteristicHandle"))
		props := Property(xc.MustGetInt("kCBMsgArgCharacteristicProperties"))

		c := &Characteristic{
			attr: attr{
				h:     cn,
				typ:   AttrCharacteristicUUID,
				props: CharRead,
				value: append([]byte{byte(props), byte(vn), byte(vn >> 8)}, u.b...),
			},
			svc: s,
		}
		d := &Descriptor{
			attr: attr{
				h:     vn,
				typ:   u,
				props: props,
			},
			char: c,
		}
		c.valued = d
		s.chars = append(s.chars, c)
	}
	return s.chars, nil
}

func (p *peripheral) DiscoverDescriptors(ds []UUID, c *Characteristic) ([]*Descriptor, error) {
	us := make([]string, len(ds))
	for i, u := range ds {
		us[i] = u.String()
	}
	rsp := p.sendReq(69, xpc.Dict{
		"kCBMsgArgDeviceUUID":                p.id.b,
		"kCBMsgArgCharacteristicHandle":      c.h,
		"kCBMsgArgCharacteristicValueHandle": c.valued.h,
		"kCBMsgArgUUIDs":                     us,
	})
	// ch := uint16(rsp.MustGetInt("kCBMsgArgCharacteristicHandle"))
	for _, xds := range rsp.MustGetArray("kCBMsgArgDescriptors") {
		xd := xds.(xpc.Dict)
		u := MustParseUUID(xd.MustGetHexBytes("kCBMsgArgUUID"))
		n := uint16(xd.MustGetInt("kCBMsgArgDescriptorHandle"))
		d := &Descriptor{
			attr: attr{
				typ: u,
				h:   n,
			},
			char: c,
		}
		c.descs = append(c.descs, d)
	}
	return c.descs, nil
}

func (p *peripheral) ReadCharacteristic(c *Characteristic) ([]byte, error) {
	rsp := p.sendReq(64, xpc.Dict{
		"kCBMsgArgDeviceUUID":                p.id.b,
		"kCBMsgArgCharacteristicHandle":      c.h,
		"kCBMsgArgCharacteristicValueHandle": c.valued.h,
	})
	// ch := uint16(rsp.MustGetInt("kCBMsgArgCharacteristicHandle"))
	// ret := rsp.MustGetInt("kCBMsgArgResult")
	// isNotification := rsp.GetInt("kCBMsgArgIsNotification", 0) != 0
	b := rsp.MustGetBytes("kCBMsgArgData")
	return b, nil
}

func (p *peripheral) WriteCharacteristic(c *Characteristic, b []byte, noRsp bool) error {
	rsp := p.sendReq(65, xpc.Dict{
		"kCBMsgArgDeviceUUID":                p.id.b,
		"kCBMsgArgCharacteristicHandle":      c.h,
		"kCBMsgArgCharacteristicValueHandle": c.valued.h,
		"kCBMsgArgData":                      b,
		"kCBMsgArgType":                      map[bool]int{false: 0, true: 1}[noRsp],
	})
	if noRsp {
		return nil
	}
	ret := rsp.MustGetInt("kCBMsgArgResult")
	// ch := uint16(rsp.MustGetInt("kCBMsgArgCharacteristicHandle"))
	return map[int]error{0: nil, 1: errors.New("failed to write")}[ret]
}

func (p *peripheral) ReadDescriptor(d *Descriptor) ([]byte, error) {
	rsp := p.sendReq(76, xpc.Dict{
		"kCBMsgArgDeviceUUID":       p.id.b,
		"kCBMsgArgDescriptorHandle": d.h,
	})
	ret := rsp.MustGetInt("kCBMsgArgResult")
	// dh := uint16(rsp.MustGetInt("kCBMsgArgDescriptorHandle"))
	err := map[int]error{0: nil, 1: errors.New("failed to read descriptor")}
	b := rsp.MustGetBytes("kCBMsgArgData")
	return b, err[ret]
}

func (p *peripheral) WriteDescriptor(d *Descriptor, b []byte) error {
	rsp := p.sendReq(77, xpc.Dict{
		"kCBMsgArgDeviceUUID":       p.id.b,
		"kCBMsgArgDescriptorHandle": d.h,
		"kCBMsgArgData":             b,
	})
	ret := rsp.MustGetInt("kCBMsgArgResult")
	// dh := uint16(rsp.MustGetInt("kCBMsgArgDescriptorHandle"))
	return map[int]error{0: nil, 1: errors.New("failed to write descriptor")}[ret]
}

func (p *peripheral) SetNotifyValue(c *Characteristic, f func(*Characteristic, []byte, error)) error {
	set := 1
	if f == nil {
		set = 0
	}
	rsp := p.sendReq(67, xpc.Dict{
		"kCBMsgArgDeviceUUID":                p.id.b,
		"kCBMsgArgCharacteristicHandle":      c.h,
		"kCBMsgArgCharacteristicValueHandle": c.valued.h,
		"kCBMsgArgState":                     set,
	})
	// To avoid race condition, registeration is handled before requesting the server.
	if f != nil {
		p.subscribemu.Lock()
		p.subscribe[c.h] = func(b []byte, err error) { f(c, b, err) }
		p.subscribemu.Unlock()
	}
	ret := rsp.MustGetInt("kCBMsgArgResult")
	// To avoid race condition, unregisteration is handled after requesting the server.
	if f == nil {
		p.subscribemu.Lock()
		delete(p.subscribe, c.valued.h)
		p.subscribemu.Unlock()
	}
	// ch := uint16(rsp.MustGetInt("kCBMsgArgCharacteristicHandle"))
	return map[int]error{0: nil, 1: errors.New("failed to subscribe")}[ret]
}

func (p *peripheral) ReadRSSI() int {
	rsp := p.sendReq(43, xpc.Dict{"kCBMsgArgDeviceUUID": p.id.b})
	return rsp.MustGetInt("kCBMsgArgData")
}

func (p *peripheral) sendReq(id int, args xpc.Dict) xpc.Dict {
	m := message{id: id, args: args, rsp: make(chan xpc.Dict)}
	p.reqc <- m
	return <-m.rsp
}

func (p *peripheral) loop() {
	rspc := make(chan message)

	go func() {
		for {
			select {
			case req := <-p.reqc:
				p.d.sendCBMsg(req.id, req.args)
				m := <-rspc
				req.rsp <- m.args
			case <-p.quitc:
				return
			}
		}
	}()

	for {
		select {
		case rsp := <-p.rspc:
			// Notification
			if rsp.id == 70 && rsp.args.GetInt("kCBMsgArgIsNotification", 0) != 0 {
				ch := uint16(rsp.args.MustGetInt("kCBMsgArgCharacteristicHandle"))
				b := rsp.args.MustGetBytes("kCBMsgArgData")
				p.subscribemu.Lock()
				f, ok := p.subscribe[ch]
				p.subscribemu.Unlock()
				if !ok {
					log.Printf("notified by unsubscribed handle")
					// FIXME: should terminate the connection
				} else {
					go f(b, nil)
				}
				break
			}
			rspc <- rsp
		case <-p.quitc:
			return
		}
	}
}
