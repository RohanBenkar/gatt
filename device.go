package gatt

import "errors"

var notImplemented = errors.New("not implemented")

type option func(Device)
type advOption func(Device)

var STATES = []string{"unknown", "resetting", "unsupported", "unauthorized", "poweredOff", "poweredOn"}

type Device interface {
	Init(func(Device, string)) error
	Advertise(opts ...advOption) error
	// Advertise(name string, ss []UUID) error
	// AdvertiseIBeaconData(data []byte) error
	// AdvertiseIBeacon(uuid UUID, major, minor uint16, measuredPower int8) error
	StopAdvertising() error
	RemoveServices() error
	AddService(s *Service) error
	SetServices(ss []*Service) error

	Scan(ss []UUID, dup bool)
	StopScanning()
	Connect(p Peripheral)
	CancelConnection(p Peripheral)
	// UpdateRSSI(p Peripheral) error

	Option(opts ...option)
}

// deviceHandler is the call back handlers of the Device.
// Device is an interface, and can't define fields(property).
// Instead, we define the deviceHandler, to different platform implementations
// can embedded it, and have keep compatible in API level.
// Package user can use the Option to set these handlers.
type deviceHandler struct {
	// name is the device name, exposed via the Generic Access Service (0x1800).
	name string

	stateChanged func(d Device, s string)

	// connect is called when a central device connects to the device.
	centralConnected func(c Central)

	// disconnect is called when a central device disconnects to the device.
	centralDisconnected func(c Central)

	// peripheralDiscovered is called when a peripheral device is found during scan procedure.
	peripheralDiscovered func(p Peripheral, a *Advertisement, rssi int)

	// peripheralConnected is called when a peripheral is conneted.
	peripheralConnected func(p Peripheral, err error)

	// peripheralConnected is called when a peripheral is disconneted.
	peripheralDisconnected func(p Peripheral, err error)
}

// Option sets the options specified.
func (d *device) Option(opts ...option) {
	for _, opt := range opts {
		opt(d)
	}
}

func AdvName(s string) advOption {
	return func(d Device) { d.(*device).name = s }
}

func AdvUUIDs(u []UUID) advOption {
	return func(d Device) { d.(*device).advUUIDs = u }
}

func AdvIBeacon(u UUID, major, minor uint16, pwr int8) advOption {
	return func(d Device) {
		d.(*device).advUUID = u
		d.(*device).advMajor = major
		d.(*device).advMinor = minor
		d.(*device).advPower = pwr
	}
}

// Name sets the device name, which is exposed via the Generic Access Service (0x1800).
func Name(s string) option {
	return func(d Device) { d.(*device).name = s }
}

// CentralConnected sets a function to be called when a device connects to the server.
func CentralConnected(f func(Central)) option {
	return func(d Device) { d.(*device).centralConnected = f }
}

// CentralDisconnected sets a function to be called when a device disconnects from the server.
func CentralDisconnected(f func(Central)) option {
	return func(d Device) { d.(*device).centralDisconnected = f }
}

// PeripheralDiscovered sets a function to be called when a peripheral device is found during scan procedure.
func PeripheralDiscovered(f func(Peripheral, *Advertisement, int)) option {
	return func(d Device) { d.(*device).peripheralDiscovered = f }
}

// PeripheralConnected sets a function to be called when a peripheral device connects.
func PeripheralConnected(f func(Peripheral, error)) option {
	return func(d Device) { d.(*device).peripheralConnected = f }
}

// PeripheralDisconnected sets a function to be called when a peripheral device disconnects.
func PeripheralDisconnected(f func(Peripheral, error)) option {
	return func(d Device) { d.(*device).peripheralDisconnected = f }
}
