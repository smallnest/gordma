package rdmanet

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// TestStreamReaderSpansMessages verifies the byte-stream adapter returns bytes
// across message boundaries and buffers the leftover of an oversized message —
// the core guarantee of Conn.Read. It is hardware-free: recv is a canned
// sequence of messages.
func TestStreamReaderSpansMessages(t *testing.T) {
	msgs := [][]byte{[]byte("hello"), []byte("world"), []byte("!")}
	i := 0
	s := &streamReader{recv: func() ([]byte, error) {
		if i >= len(msgs) {
			return nil, io.EOF
		}
		m := msgs[i]
		i++
		return m, nil
	}}

	// Read 3 bytes at a time; the stream must reconstruct "helloworld!" across
	// message boundaries regardless of read size.
	var got bytes.Buffer
	p := make([]byte, 3)
	for {
		n, err := s.read(p)
		got.Write(p[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read: %v", err)
		}
	}
	if got.String() != "helloworld!" {
		t.Errorf("stream: want %q, got %q", "helloworld!", got.String())
	}
}

func TestStreamReaderLeftoverBuffered(t *testing.T) {
	// One 5-byte message read 2 bytes at a time must yield 2,2,1 then fetch again.
	calls := 0
	s := &streamReader{recv: func() ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("abcde"), nil
		}
		return nil, io.EOF
	}}
	p := make([]byte, 2)
	want := []string{"ab", "cd", "e"}
	for _, w := range want {
		n, err := s.read(p)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(p[:n]) != w {
			t.Errorf("read: want %q, got %q", w, p[:n])
		}
	}
	if calls != 1 {
		t.Errorf("recv should have been called once for a single buffered message, got %d", calls)
	}
	// Next read drains the source.
	if _, err := s.read(p); !errors.Is(err, io.EOF) {
		t.Errorf("read after drain: want EOF, got %v", err)
	}
}

func TestStreamReaderEmptyDst(t *testing.T) {
	s := &streamReader{recv: func() ([]byte, error) { t.Fatal("recv must not be called for empty dst"); return nil, nil }}
	if n, err := s.read(nil); n != 0 || err != nil {
		t.Errorf("read(nil): want 0,nil got %d,%v", n, err)
	}
}

func TestStreamReaderPropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	s := &streamReader{recv: func() ([]byte, error) { return nil, wantErr }}
	if _, err := s.read(make([]byte, 4)); !errors.Is(err, wantErr) {
		t.Errorf("read: want %v, got %v", wantErr, err)
	}
}
