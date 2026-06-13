package rdmanet

import (
	"bytes"
	"errors"
	"testing"
)

func TestFrameHeaderRoundTrip(t *testing.T) {
	cases := []frameHeader{
		{flags: 0, value: 0},
		{flags: flagMore, value: 1500},
		{flags: flagCredit, value: 7},
		{flags: flagMore | flagCredit, value: 0xdeadbeef},
	}
	buf := make([]byte, frameHeaderSize)
	for _, h := range cases {
		h.encode(buf)
		got, err := decodeHeader(buf)
		if err != nil {
			t.Fatalf("decodeHeader(%v): %v", h, err)
		}
		if got.flags != h.flags || got.value != h.value {
			t.Errorf("round-trip: want %+v, got %+v", h, got)
		}
		if got.hasMore() != (h.flags&flagMore != 0) {
			t.Errorf("hasMore mismatch for %+v", h)
		}
		if got.isCredit() != (h.flags&flagCredit != 0) {
			t.Errorf("isCredit mismatch for %+v", h)
		}
	}
}

func TestDecodeHeaderShort(t *testing.T) {
	if _, err := decodeHeader(make([]byte, frameHeaderSize-1)); err == nil {
		t.Error("decodeHeader: want error for short buffer")
	}
}

func TestFragmentCount(t *testing.T) {
	cases := []struct{ msg, chunk, want int }{
		{0, 100, 1}, // empty message still takes one frame for the boundary
		{1, 100, 1},
		{100, 100, 1},
		{101, 100, 2},
		{200, 100, 2},
		{201, 100, 3},
		{10, 0, 0}, // invalid chunk
	}
	for _, c := range cases {
		if got := fragmentCount(c.msg, c.chunk); got != c.want {
			t.Errorf("fragmentCount(%d,%d): want %d, got %d", c.msg, c.chunk, c.want, got)
		}
	}
}

// TestReassemblerRoundTrip fragments messages of various sizes and reassembles
// them, asserting the bytes and boundaries are preserved — the core guarantee
// behind 16MB-message support.
func TestReassemblerRoundTrip(t *testing.T) {
	chunk := 64
	r := newReassembler(0)
	for _, size := range []int{0, 1, 63, 64, 65, 200, 4096, 16 << 20} {
		msg := make([]byte, size)
		for i := range msg {
			msg[i] = byte(i * 31)
		}
		frames := fragmentCount(size, chunk)
		var got []byte
		var complete bool
		var err error
		for f := 0; f < frames; f++ {
			start := f * chunk
			end := start + chunk
			if end > size {
				end = size
			}
			more := f < frames-1
			got, complete, err = r.add(msg[start:end], more)
			if err != nil {
				t.Fatalf("size %d frame %d: %v", size, f, err)
			}
			if more && complete {
				t.Fatalf("size %d: completed early at frame %d", size, f)
			}
		}
		if !complete {
			t.Fatalf("size %d: never completed", size)
		}
		if !bytes.Equal(got, msg) {
			t.Errorf("size %d: reassembled bytes differ", size)
		}
	}
}

func TestReassemblerMaxGuard(t *testing.T) {
	r := newReassembler(100)
	if _, _, err := r.add(make([]byte, 101), false); !errors.Is(err, ErrMessageTooLargeReassembly) {
		t.Errorf("want ErrMessageTooLargeReassembly, got %v", err)
	}
	// After the guard trips, the reassembler must be usable again.
	got, complete, err := r.add(make([]byte, 10), false)
	if err != nil || !complete || len(got) != 10 {
		t.Errorf("post-guard add: complete=%v len=%d err=%v", complete, len(got), err)
	}
}

// TestReassemblerTwoMessages ensures boundaries between consecutive messages
// are preserved (no bleed-over).
func TestReassemblerTwoMessages(t *testing.T) {
	r := newReassembler(0)
	a, complete, _ := r.add([]byte("hello"), false)
	if !complete || string(a) != "hello" {
		t.Fatalf("msg1: %q complete=%v", a, complete)
	}
	r.add([]byte("wor"), true)
	b, complete, _ := r.add([]byte("ld"), false)
	if !complete || string(b) != "world" {
		t.Fatalf("msg2: %q complete=%v", b, complete)
	}
}
