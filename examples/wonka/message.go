package wonka

import (
	"fmt"
	"log"
)

// Packetize takes data and breaks it up into a slice of packets ready to be sent.
func Packetize(data []byte, stream uint8, currentMaxChunkSize int) ([]Packet, error) {
	length := len(data)
	if length == 0 {
		return []Packet{}, fmt.Errorf("empty data")
	}
	if length > MaxFrame*currentMaxChunkSize {
		return []Packet{}, fmt.Errorf("data too long: %d bytes, max is %d", length, MaxFrame*currentMaxChunkSize)
	}

	packets := make([]Packet, 0, length/currentMaxChunkSize+1) // no need to be precise about whether the +1 is needed, it is just a capacity
	for idx, offset := 0, 0; offset < length || idx == 0; offset += currentMaxChunkSize {
		p := Packet{
			Stream: stream,
			Frame:  uint8(idx),
		}
		p.Fin = offset+currentMaxChunkSize >= length
		packetLength := length - offset
		if packetLength > currentMaxChunkSize {
			packetLength = currentMaxChunkSize
		}
		packetData := data[offset : offset+packetLength]
		p.Data = make([]byte, len(packetData)) // make a copy of the data, to be safe
		copy(p.Data, packetData)
		packets = append(packets, p)
		idx++
	}
	return packets, nil
}

// TODO: Think through message building algorithm for
// resource- and time-efficiency; worry about DOS attacks.
type Message struct {
	Packets map[uint8]Packet
}

func (m *Message) AddPacket(p Packet) {
	if m.Packets == nil {
		m.Packets = make(map[uint8]Packet)
	}
	// sanity-check: if the same identical frame gets received twice, fine,
	// but if we get a *different* frame, assume something has gone wrong
	// and reset (and keep this new frame).
	q, ok := m.Packets[p.Frame]
	if ok && !equalPackets(p, q) {
		log.Printf("panic-resetting message %v due to frame collision", m)
		m.Packets = make(map[uint8]Packet)
	}
	m.Packets[p.Frame] = p
}

func (m Message) Complete() bool {
	if m.Packets == nil {
		return false
	}

	var frame uint16 // if we use uint8 here, we could get stuck in an infinite loop
	for frame = 0; frame <= MaxFrame; frame++ {
		packet, ok := m.Packets[uint8(frame)]
		if !ok {
			return false // missing frame
		}
		if packet.Fin {
			return true
		}
	}

	// This is bad. We have run through all
	// frame positions, there is a packet at each one
	// of them, but none of them have the fin bit set.
	// Strictly speaking, this is an incomplete message,
	// but is likely a bad situation.
	log.Printf("received incomplete message -- all frames used, but no fin bit!")
	return false
}

func (m Message) Bytes() (data []byte, ok bool) {
	if !m.Complete() {
		return nil, false
	}
	// avoid multiple allocs by taking a first pass through
	// to calculate the total data length.
	var frame uint16 // if we use uint8 here, we could get stuck in an infinite loop
	var totalLen int
	for frame = 0; frame <= MaxFrame; frame++ {
		packet := m.Packets[uint8(frame)]
		totalLen += len(packet.Data)
		if packet.Fin {
			break
		}
	}

	// TODO: Sanity check packet.Frame, packet.Stream as we go?
	data = make([]byte, totalLen)
	var index int
	for frame = 0; frame <= MaxFrame; frame++ {
		packet := m.Packets[uint8(frame)]
		packetLen := len(packet.Data)
		copied := copy(data[index:index+packetLen], packet.Data)
		// sanity check; better to mysteriously return
		// no data than to return corrupt data
		if copied != packetLen {
			return nil, false
		}
		index += packetLen
		if packet.Fin {
			break
		}
	}

	return data, true
}

func (m Message) String() string {
	data, _ := m.Bytes()
	return fmt.Sprintf("<Message (complete? %v): [% X]>", m.Complete(), data)
}
