package wonka

// Demuxer manages inbound packets from the
// network, separating them into streams.
type Demuxer struct {
	Streams [MaxStream + 1]*Message
}

// Receive informs the demuxer that a complete chunk of data has been received
// from the network. If the data contains an invalid packet, Receive will return
// an error. Otherwise, Receive will return a message, which may or may not
// be complete.
func (d *Demuxer) Receive(data []byte, currentMaxChunkSize int) (message *Message, stream uint8, err error) {
	p, e := Decode(data, currentMaxChunkSize)
	if e != nil {
		return nil, 0, e
	}

	// no need to check p.Stream; it cannot be > MaxStream
	message = d.Streams[p.Stream]
	if message == nil {
		message = &Message{}
		d.Streams[p.Stream] = message
	}
	message.AddPacket(p)
	return message, p.Stream, nil
}

// Clear informs the demuxer that a particular stream
// is no longer of interest, usually because the message has been
// received and processed.
func (d *Demuxer) Clear(stream uint8) {
	d.Streams[stream] = nil
}
