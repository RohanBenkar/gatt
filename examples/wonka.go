package main

import (
	"log"

	"github.com/paypal/gatt"
	"github.com/paypal/gatt/examples/option"
	"github.com/paypal/gatt/examples/service"
	"github.com/paypal/gatt/examples/wonka"
)

type listener struct {
	connc chan wonka.Conn
	conn  wonka.Conn
}

// NewListener creates a new wonka listener.
func NewWonka() *listener {
	return &listener{connc: make(chan wonka.Conn)}
}

// Accept return a listener connection.
func (w *listener) Accept() wonka.Conn {
	return <-w.connc
}

// ServeWrite receives incoming frames from a remote central device.
func (w *listener) ServeWrite(r gatt.Request, data []byte) byte {
	// OS X specific trick, since corebluetooth doesn't report when central connected or disconnected
	if w.conn == nil {
		w.conn = wonka.NewWonkaConn(r.Central)
		w.connc <- w.conn
	}

	w.conn.InFrame(data)
	return gatt.StatusSuccess
}

// ServeNotify sends outgoing frames to a remote central device.
func (w *listener) ServeNotify(r gatt.Request, n gatt.Notifier) {
	go func() {
		for !n.Done() {
			// OS X specific trick, since corebluetooth doesn't report when central connected or disconnected
			if w.conn == nil {
				w.conn = wonka.NewWonkaConn(r.Central)
				w.connc <- w.conn
			}

			f := w.conn.OutFrame()
			if f == nil {
				return
			}
			n.Write(f)
		}
	}()
}

var (
	CheckinServiceUUID = gatt.UUID16(0xFEFA)
	WonkaRequestUUID   = gatt.UUID16(0xFEF9)
	WonkaResponseUUID  = gatt.UUID16(0xFEFA)
)

func main() {
	d, err := gatt.NewDevice(option.DefaultServerOptions...)
	if err != nil {
		log.Printf("BLE device initialize failed: %s", err)
	}

	w := NewWonka()

	// onConnect wraps a coneccted gatt connection with listener connnection,
	// and passes it to upper stack via connc and Accept.
	onConnect := func(gc gatt.Central) {
		log.Printf("connected")
		wc := wonka.NewWonkaConn(gc)
		w.conn = wc
		w.connc <- wc
	}

	// onDisconnect notifies upper layer when a gatt connection disconnects.
	onDisconnect := func(gc gatt.Central) {
		log.Printf("disconnected")
		w.conn.Close()
		w.conn = nil
	}

	// Register CentralConnected and CentralDisconnected events to the device.
	d.Handle(
		gatt.CentralConnected(onConnect),
		gatt.CentralDisconnected(onDisconnect),
	)

	// Setup the gatt server, and advertise service and iBeacon alternately.
	onStateChanged := func(d gatt.Device, s gatt.State) {
		log.Printf("ble device state: %s", s)
		switch s {
		case gatt.StatePoweredOn:
			// Setup GAP and GATT services for Linux implementation.
			// OS X doesn't export the access of these services.
			d.AddService(service.NewGapService("Wonka")) // no effect on OS X
			d.AddService(service.NewGattService())       // no effect on OS X

			// Add checkin service.
			svc := gatt.NewService(CheckinServiceUUID)
			svc.AddCharacteristic(WonkaResponseUUID).HandleNotify(w)
			svc.AddCharacteristic(WonkaRequestUUID).HandleWrite(w)
			d.AddService(svc)

			// Advertise device name and service's UUIDs.
			d.AdvertiseNameAndServices("Wonka", []gatt.UUID{svc.UUID()})
		}
	}

	d.Init(onStateChanged)

	for {
		wc := w.Accept()
		for {
			raw, num := wc.ReadStream()
			if len(raw) == 0 {
				return
			}
			log.Printf("[%d] % X", num, raw)
			// echo back
			wc.WriteStream(raw, num)
		}
	}
}
