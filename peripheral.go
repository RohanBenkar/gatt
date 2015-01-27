package gatt

import (
	"errors"
	"sync"
)

// Peripheral is the interface that represent a remoe peripheral device.
type Peripheral interface {
	Device() Device

	// ID returns platform specific unique ID, e.g. MAC for Linux, Peripheral UUID for MacOS.
	ID() string

	// Name returns the name of the peripheral.
	Name() string

	// Services returnns the services of the peripheral which has been discovered.
	Services() []*Service

	// DiscoverServices discover the specified services of the peripheral.
	// If the specified services is set to nil, all the available services of the peripheral are returned.
	DiscoverServices(s []UUID) ([]*Service, error)

	// DiscoverIncludedServices discovers the specified included services of a service.
	// If the specified services is set to nil, all the available services of the peripheral are returned.
	DiscoverIncludedServices(ss []UUID, s *Service) ([]*Service, error)

	// DiscoverCharacteristics discovers the specified characteristics of a service.
	DiscoverCharacteristics(c []UUID, s *Service) ([]*Characteristic, error)

	// DiscoverDescriptors discovers the descriptors of a characteristic.
	DiscoverDescriptors(d []UUID, c *Characteristic) ([]*Descriptor, error)

	// ReadCharacteristic retrieves the value of a specified characteristic.
	ReadCharacteristic(c *Characteristic) ([]byte, error)

	// ReadDescriptor retrieves the value of a specified characteristic descriptor.
	ReadDescriptor(d *Descriptor) ([]byte, error)

	// WriteCharacteristic writes the value of a characteristic.
	WriteCharacteristic(c *Characteristic, b []byte, noRsp bool) error

	// WriteDescriptor writes the value of a characteristic descriptor.
	WriteDescriptor(d *Descriptor, b []byte) error

	// SetNotifyValue sets notifications or indications for the value of a specified characteristic.
	SetNotifyValue(c *Characteristic, f func(*Characteristic, []byte, error)) error

	// ReadRSSI retrieves the current RSSI value for the peripheral while it is connected to the central manager.
	ReadRSSI() int
}

type subscriber struct {
	sub map[uint16]subscribefn
	mu  *sync.Mutex
}

type subscribefn func([]byte, error)

func newSubscriber() *subscriber {
	return &subscriber{
		sub: make(map[uint16]subscribefn),
		mu:  &sync.Mutex{},
	}
}

func (s *subscriber) subscribe(h uint16, f subscribefn) {
	s.mu.Lock()
	s.sub[h] = f
	s.mu.Unlock()
}

func (s *subscriber) unsubscribe(h uint16) {
	s.mu.Lock()
	delete(s.sub, h)
	s.mu.Unlock()
}

func (s *subscriber) fn(h uint16) subscribefn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sub[h]
}

var (
	ErrInvalidLength = errors.New("invalid length")
)
