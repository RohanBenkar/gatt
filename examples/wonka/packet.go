package wonka

import (
	"bytes"
	"fmt"
)

const (
	supportedProtocol = 0
)

const (
	reservedBits = 1
	protocolBits = 2
	finBits      = 1
	streamBits   = 4
	frameBits    = 8

	frameShift    = 0
	streamShift   = frameBits
	finShift      = streamShift + streamBits
	protocolShift = finShift + finBits
	reservedShift = protocolShift + protocolBits

	MaxStream = 1<<streamBits - 1
	MaxFrame  = 1<<frameBits - 1

	headerSize = 2
)

type Packet struct {
	Data   []byte
	Fin    bool
	Stream uint8
	Frame  uint8
}

// bitRange extracts bits bits from u after shifting shift bits to the right.
func bitRange(u uint16, shift uint8, bits uint8) uint16 {
	return (u >> shift) & (1<<bits - 1)
}

func equalPackets(p, q Packet) bool {
	return p.Fin == q.Fin && p.Stream == q.Stream && p.Frame == q.Frame && bytes.Equal(p.Data, q.Data)
}

func (p Packet) Encode() ([]byte, error) {
	if p.Stream > MaxStream {
		return nil, fmt.Errorf("max stream is %d, got %d", MaxStream, p.Stream)
	}
	if uint(p.Frame) > MaxFrame {
		return nil, fmt.Errorf("max frame is %d, got %d", MaxFrame, p.Frame)
	}
	packed := make([]byte, len(p.Data)+headerSize)

	var fin uint16 = 0
	if p.Fin {
		fin = 1
	}

	var header uint16

	// the compiler should optimize away pointless bits here, so I am leaving them all in for clarity
	header = (0 << reservedShift) | (0 << protocolShift) | (fin << finShift) | (uint16(p.Stream) << streamShift) | (uint16(p.Frame) << frameShift)

	packed[0] = byte((header >> 8) & 0xFF)
	packed[1] = byte((header >> 0) & 0xFF)

	copy(packed[headerSize:], p.Data)

	return packed, nil
}

func Decode(data []byte, currentMaxChunkSize int) (Packet, error) {
	if len(data) < headerSize+1 {
		return Packet{}, fmt.Errorf("packet too short: %d, min is %d", len(data), headerSize+1)
	}
	if len(data) > headerSize+currentMaxChunkSize {
		return Packet{}, fmt.Errorf("packet too long: %d, max is %d", len(data), headerSize+currentMaxChunkSize)
	}

	var p Packet
	var header uint16
	header = uint16(data[0])<<8 + uint16(data[1]<<0)

	if reserved := bitRange(header, reservedShift, reservedBits); reserved != 0 {
		return Packet{}, fmt.Errorf("reserved bit set")
	}
	if protocol := bitRange(header, protocolShift, protocolBits); protocol != supportedProtocol {
		return Packet{}, fmt.Errorf("unsupported protocol %d", protocol)
	}

	p.Fin = bitRange(header, finShift, finBits) > 0
	p.Stream = uint8(bitRange(header, streamShift, streamBits))
	p.Frame = uint8(bitRange(header, frameShift, frameBits))

	p.Data = make([]byte, len(data)-headerSize)
	copy(p.Data, data[headerSize:])

	return p, nil
}

// mustEncode is a helper that wraps a call to a function returning []byte, error
// and panics if the error is non-nil. It is intended for use in variable
// initializations and tests.
func mustEncode(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

// mustDecode is a helper that wraps a call to a function returning Packet, error
// and panics if the error is non-nil. It is intended for use in variable
// initializations and tests.
func mustDecode(p Packet, err error) Packet {
	if err != nil {
		panic(err)
	}
	return p
}
