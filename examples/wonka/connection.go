package wonka

import (
	"log"
	"sync"
	"time"

	"github.com/paypal/gatt"
)

type Conn interface {
	// ReadStream reads a single available stream (data + stream number)
	// from the connection.
	ReadStream() ([]byte, uint8)
	// WritesStream writes a stream response b to the stream s.
	WriteStream(b []byte, s uint8)
	// Timer retrieves the syncutil.Timer associated with this connection,
	// so that clients may alter it. When the Timer expires, the connection
	// will be forcibly closed.

	OutFrame() frame
	InFrame(f frame)

	Close()

	// Timer() *syncutil.Timer
}

type wonkaConn struct {
	readc    chan fullstream // read complete streams
	writec   chan fullstream // write complete streams
	closec   chan struct{}   // closed when the wonkaConn is closed
	outFrame chan frame
	inFrame  chan frame
	demux    Demuxer
	// t         *syncutil.Timer
	gc        gatt.Central
	closeOnce sync.Once
}

type fullstream struct {
	data   []byte
	number uint8
}

const (
	checkinTimeout = time.Second * 3
	setupTimeout   = time.Second * 10
)

type frame []byte

func NewWonkaConn(gc gatt.Central) Conn {
	wc := &wonkaConn{
		demux:    Demuxer{}, // TODO: Make demuxer zero value useful
		readc:    make(chan fullstream),
		writec:   make(chan fullstream),
		inFrame:  make(chan frame),
		outFrame: make(chan frame),
		closec:   make(chan struct{}),
		gc:       gc,
		// t:         new(syncutil.Timer),
		closeOnce: sync.Once{},
	}

	// wc.t.Set(checkinTimeout) // FIXME
	go wc.loop()
	return wc
}

func (wc *wonkaConn) ReadStream() ([]byte, uint8) {
	select {
	case <-wc.closec:
		return nil, 0
	case s := <-wc.readc:
		return s.data, s.number
	}
}

func (wc *wonkaConn) WriteStream(b []byte, s uint8) {
	select {
	case <-wc.closec:
	case wc.writec <- fullstream{data: b, number: s}:
	}
}

func (wc *wonkaConn) OutFrame() frame {
	select {
	case <-wc.closec:
		return nil
	case f := <-wc.outFrame:
		return f
	}
}

func (wc *wonkaConn) InFrame(f frame) {
	select {
	case <-wc.closec:
	case wc.inFrame <- f:
	}
}

func (wc *wonkaConn) Close() {
	wc.closeOnce.Do(
		func() {
			close(wc.closec)
		})
}

func (wc *wonkaConn) currentChunkSize() int {
	return wc.gc.MTU() - headerSize // FIXME
}

// func (wc *wonkaConn) Timer() *syncutil.Timer {
// 	return wc.t
// }

func (wc *wonkaConn) loop() {
	go func() {
		for {
			select {
			// case <-wc.t.WaitC():
			// 	wc.Close()
			// 	wc.gc.Close()
			// 	return
			case <-wc.closec:
				// wc.t.Cancel()
				return
			}
		}
	}()
	for {
		select {
		case <-wc.closec:
			return
		case f := <-wc.inFrame:
			message, stream, err := wc.demux.Receive(f, wc.currentChunkSize())
			if err != nil {
				log.Printf("Encountered err in received frame %v: %v", f, err)
				continue
			}

			if !message.Complete() {
				log.Printf("Message on stream %d incomplete, waiting.", stream)
				continue
			}
			log.Printf("Message on stream %d complete, processing", stream)

			// Message complete -- queue it up for dispatch.
			data, _ := message.Bytes()
			// Check again in case of race condition.
			select {
			case <-wc.closec:
				return
			case wc.readc <- fullstream{data: data, number: stream}:
				wc.demux.Clear(stream)
			}
		case s := <-wc.writec:
			// enqueue everything for sending
			wonkapackets, err := Packetize(s.data, s.number, wc.currentChunkSize())
			if err != nil {
				log.Printf("Failed to packetize %#v: %v", s, err)
				continue
			}

			// embed our wonka packets into ble frames for transmission...
			for _, wonkapacket := range wonkapackets {
				encoded, err := wonkapacket.Encode()
				if err != nil {
					log.Printf("Failed to encode wonka packet %v: %v", wonkapacket, err)
					continue
				}

				// Check again in case of race condition.
				select {
				case <-wc.closec:
					return
				case wc.outFrame <- encoded:
				}
			}
		}
	}
}
