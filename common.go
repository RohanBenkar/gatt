package gatt

// Supported statuses for GATT characteristic read/write operations.
const (
	StatusSuccess         = 0
	StatusInvalidOffset   = 1
	StatusUnexpectedError = 2
)

var STATES = []string{"unknown", "resetting", "unsupported", "unauthorized", "poweredOff", "poweredOn"}

type Central interface {
	Close() error // Close disconnects the connection.
	MTU() int     // MTU returns the current connection mtu.
}

// A Request is the context for a request from a connected central device.
// TODO: Replace this with more general context, such as:
// http://godoc.org/golang.org/x/net/context
type Request struct {
	Central        Central
	Characteristic *Characteristic
}

// A ReadRequest is a characteristic read request from a connected device.
type ReadRequest struct {
	Request
	Cap    int // maximum allowed reply length
	Offset int // request value offset
}

type Property int

// Characteristic property flags (spec 3.3.3.1)
// Do not re-order the bit flags below;
// they are organized to match the BLE spec.
const (
	CharBroadcast   Property = 1 << iota // the characteristic may be brocasted
	CharRead                             // the characteristic may be read
	CharWriteNR                          // the characteristic may be written to, with no reply
	CharWrite                            // the characteristic may be written to, with a reply
	CharNotify                           // the characteristic supports notifications
	CharIndicate                         // the characteristic supports Indications
	CharSignedWrite                      // the characteristic supports signed write
	CharExtended                         // the characteristic supports extended properties
)

func (p Property) String() (result string) {
	if (p & CharBroadcast) != 0 {
		result += "broadcast "
	}
	if (p & CharRead) != 0 {
		result += "read "
	}
	if (p & CharWriteNR) != 0 {
		result += "writeWithoutResponse "
	}
	if (p & CharWrite) != 0 {
		result += "write "
	}
	if (p & CharNotify) != 0 {
		result += "notify "
	}
	if (p & CharIndicate) != 0 {
		result += "indicate "
	}
	if (p & CharSignedWrite) != 0 {
		result += "authenticateSignedWrites "
	}
	if (p & CharExtended) != 0 {
		result += "extendedProperties "
	}
	return
}

// A Service is a BLE service.
type Service struct {
	attr
	endh  uint16
	chars []*Characteristic
}

func NewService(u UUID) *Service {
	return &Service{
		attr: attr{
			typ:   AttrPrimaryServiceUUID,
			value: u.b,
			props: CharRead,
		},
	}
}

// AddCharacteristic adds a characteristic to a service.
// AddCharacteristic panics if the service already contains another characteristic with the same UUID.
func (s *Service) AddCharacteristic(u UUID) *Characteristic {
	// TODO: write test for this panic
	for _, c := range s.chars {
		if c.typ.Equal(u) {
			panic("service already contains a characteristic with uuid " + u.String())
		}
	}

	c := &Characteristic{
		attr: attr{
			typ:   AttrCharacteristicUUID,
			props: CharRead,
			value: append([]byte{0, 0, 0}, u.b...),
		},
		svc: s,
	}
	d := &Descriptor{
		attr: attr{
			typ:   u,
			value: nil,
		},
		char: c,
	}
	c.valued = d
	s.chars = append(s.chars, c)
	return c
}

func (s *Service) UUID() UUID                         { return UUID{s.value} }
func (s *Service) Name() string                       { return knownServices[s.UUID().String()].Name }
func (s *Service) Characteristics() []*Characteristic { return s.chars }

// A Characteristic is a BLE characteristic.
type Characteristic struct {
	attr
	endh uint16

	// Here we just borrow the *Descriptor for the implementation.
	valued *Descriptor

	descs []*Descriptor
	cccd  *Descriptor
	svc   *Service
}

func (c *Characteristic) UUID() UUID                 { return UUID{c.value[3:]} }
func (c *Characteristic) Name() string               { return knownCharacteristics[c.UUID().String()].Name }
func (c *Characteristic) Service() *Service          { return c.svc }
func (c *Characteristic) Properties() Property       { return c.valued.props }
func (c *Characteristic) Descriptors() []*Descriptor { return c.descs }
func (c *Characteristic) AddDescriptor(u UUID) *Descriptor {
	d := &Descriptor{
		attr: attr{
			typ:   u,
			value: nil,
		},
		char: c,
	}
	c.descs = append(c.descs, d)
	return d
}

func (c *Characteristic) SetValue(b []byte) {
	c.value[0] |= byte(CharRead)
	c.valued.SetValue(b)
}

// and routes read requests to h. HandleRead must be called
// before any server using c has been started.
func (c *Characteristic) HandleRead(h ReadHandler) {
	c.value[0] |= byte(CharRead)
	c.valued.HandleRead(h)
}

// HandleReadFunc calls HandleRead(ReadHandlerFunc(f)).
func (c *Characteristic) HandleReadFunc(f func(resp ResponseWriter, req *ReadRequest)) {
	c.HandleRead(ReadHandlerFunc(f))
}

// HandleWrite makes the characteristic support write and
// write-no-response requests, and routes write requests to h.
// The WriteHandler does not differentiate between write and
// write-no-response requests; it is handled automatically.
// HandleWrite must be called before any server using c has been started.
func (c *Characteristic) HandleWrite(h WriteHandler) {
	c.value[0] |= byte(CharWrite)
	c.valued.HandleWrite(h)
}

// HandleWriteFunc calls HandleWrite(WriteHandlerFunc(f)).
func (c *Characteristic) HandleWriteFunc(f func(r Request, data []byte) (status byte)) {
	c.HandleWrite(WriteHandlerFunc(f))
}

// HandleNotify makes the characteristic support notify requests,
// and routes notification requests to h. HandleNotify must be called
// before any server using c has been started.
func (c *Characteristic) HandleNotify(h NotifyHandler) {
	if c.cccd != nil {
		return
	}
	p := CharNotify | CharIndicate

	vd := c.valued
	vd.props |= p
	vd.handleNotify(h)

	// Sync up characteristic's attribute value.
	c.value[0] |= byte(p)

	// add ccc (client characteristic configuration) descriptor
	secure := Property(0)
	// If the characteristic requested secure notifications,
	// then set ccc security to r/w.
	if c.secure&p != 0 {
		secure = CharRead | CharWrite | CharWriteNR
	}
	cd := &Descriptor{
		attr: attr{
			typ:    AttrClientCharacteristicConfigUUID,
			props:  CharRead | CharWrite | CharWriteNR,
			secure: secure,
			value:  []byte{0x00, 0x00},
		},
		char: c,
	}
	c.cccd = cd
	c.descs = append(c.descs, cd)
}

// HandleNotifyFunc calls HandleNotify(NotifyHandlerFunc(f)).
func (c *Characteristic) HandleNotifyFunc(f func(r Request, n Notifier)) {
	c.HandleNotify(NotifyHandlerFunc(f))
}

type Descriptor struct {
	attr
	char *Characteristic
}

func (d *Descriptor) UUID() UUID                      { return d.typ }
func (d *Descriptor) Name() string                    { return knownDescriptors[d.UUID().String()].Name }
func (d *Descriptor) Characteristic() *Characteristic { return d.char }

func (d *Descriptor) SetValue(b []byte) {
	d.props |= CharRead
	d.value = make([]byte, len(b))
	copy(d.value, b)
}

// and routes read requests to h. HandleRead must be called
// before any server using d has been started.
func (d *Descriptor) HandleRead(h ReadHandler) {
	d.props |= CharRead
	d.secure |= CharRead
	d.rhandler = h
}

// HandleReadFunc calls HandleRead(ReadHandlerFunc(f)).
func (d *Descriptor) HandleReadFunc(f func(resp ResponseWriter, req *ReadRequest)) {
	d.HandleRead(ReadHandlerFunc(f))
}

// HandleWrite makes the characteristic support write and
// write-no-response requests, and routes write requests to h.
// The WriteHandler does not differentiate between write and
// write-no-response requests; it is handled automatically.
// HandleWrite must be called before any server using d has been started.
func (d *Descriptor) HandleWrite(h WriteHandler) {
	d.props |= CharWrite | CharWriteNR
	d.secure |= CharWrite | CharWriteNR
	d.whandler = h
}

// HandleWriteFunc calls HandleWrite(WriteHandlerFunc(f)).
func (d *Descriptor) HandleWriteFunc(f func(r Request, data []byte) (status byte)) {
	d.HandleWrite(WriteHandlerFunc(f))
}

// HandleNotify makes the characteristic support notify requests,
// and routes notification requests to h. HandleNotify must be called
// before any server using d has been started.
func (d *Descriptor) handleNotify(h NotifyHandler) {
	d.props |= (CharNotify | CharIndicate)
	d.secure |= (CharNotify | CharIndicate)
	d.nhandler = h
}
