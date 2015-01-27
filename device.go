package gatt

import "errors"

var notImplemented = errors.New("not implemented")

type option func(*Device)

type peripheralManager struct {
	// Name is the device name, exposed via the Generic Access Service (0x1800).
	Name string

	StateChanged func(s string)

	// Connect is called when a device connects to the server.
	Connected func(c Central)

	// Disconnect is called when a device disconnects from the server.
	Disconnected func(c Central)

	// ReceiveRSSI is called when an RSSI measurement is received for a connection.
	ReceiveRSSI func(c Central, rssi int)

	// Closed is called when a server is closed.
	// err will be any associated error.
	// If the server was closed by calling Close, err may be nil.
	Closed func(error)

	// Notify the centerals
	UpdateValue func(char *Characteristic, value []byte, centrals []Central) error

	// TODO: unused in linux, see if we can remove it in MacOS port.
	CharacteristicSubscribed  func(c Central, char *Characteristic)
	CharacteristicUnubscribed func(c Central, char *Characteristic)
}

type centralManager struct {
	PeripheralConnected          func(p Peripheral)
	PeripheralDisconnected       func(p Peripheral, err error)
	PeripheralFailToConnect      func(p Peripheral, err error)
	PeripheralDiscovered         func(p Peripheral, a *Advertisement, rssi int)
	RetrieveConnectedPeripherals func(p []Peripheral)
	RetrievePeripherals          func(p []Peripheral)
}
