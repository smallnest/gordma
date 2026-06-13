package rdmanet

// streamReader adapts a message-receiving function into an io.Reader-style byte
// stream. It buffers the leftover of a message whose bytes did not fit the
// caller's slice, returning them on subsequent reads, so byte reads transparently
// span message boundaries. It is build-agnostic and unit-tested without RDMA
// hardware; the linux Conn.Read uses the same buffering logic against the real
// transport.
type streamReader struct {
	buf  []byte
	recv func() ([]byte, error)
}

// read fills p from the buffered leftover, fetching the next message via recv
// when the buffer is empty. It returns at least one byte unless p is empty or
// recv errors. This mirrors io.Reader semantics.
func (s *streamReader) read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(s.buf) == 0 {
		msg, err := s.recv()
		if err != nil {
			return 0, err
		}
		s.buf = msg
	}
	n := copy(p, s.buf)
	s.buf = s.buf[n:]
	if len(s.buf) == 0 {
		s.buf = nil
	}
	return n, nil
}
