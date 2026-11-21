package session

import (
	"bytes"
	"errors"
	"sync"
	"testing"
)

// fakeChannel records sends for Session tests.
type fakeChannel struct {
	mu      sync.Mutex
	open    bool
	sent    [][]byte
	closes  [][2]any
	sendErr error
	addr    string
}

func newFakeChannel() *fakeChannel { return &fakeChannel{open: true, addr: "1.2.3.4"} }

func (f *fakeChannel) Kind() string       { return "websocket" }
func (f *fakeChannel) RemoteAddr() string { return f.addr }
func (f *fakeChannel) Send(payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return f.sendErr
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	f.sent = append(f.sent, cp)
	return nil
}
func (f *fakeChannel) Close(code int, reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.open = false
	f.closes = append(f.closes, [2]any{code, reason})
}
func (f *fakeChannel) IsOpen() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.open
}
func (f *fakeChannel) sentBytes() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]byte(nil), f.sent...)
}

func TestSessionQueueFlushConcat(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	s.SendPacket([]byte{1, 2})
	s.SendPacket([]byte{3})
	s.SendPacket([]byte{4, 5, 6})
	if got := ch.sentBytes(); len(got) != 0 {
		t.Fatalf("queued packets sent before flush: %d sends", len(got))
	}
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	sent := ch.sentBytes()
	if len(sent) != 1 {
		t.Fatalf("Flush sent %d frames, want 1", len(sent))
	}
	if want := []byte{1, 2, 3, 4, 5, 6}; !bytes.Equal(sent[0], want) {
		t.Errorf("flushed frame = %v, want %v", sent[0], want)
	}
	// Second flush with an empty queue sends nothing.
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush (empty): %v", err)
	}
	if got := ch.sentBytes(); len(got) != 1 {
		t.Errorf("empty flush sent, total sends = %d", len(got))
	}
}

func TestSessionFlushTickFlushesQueue(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	s.SendPacket([]byte{9})
	if err := s.FlushTick(42); err != nil {
		t.Fatalf("FlushTick: %v", err)
	}
	if got := ch.sentBytes(); len(got) != 1 || !bytes.Equal(got[0], []byte{9}) {
		t.Errorf("FlushTick sent %v", got)
	}
}

func TestSession64KiBSplit(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	// Fill the queue to exactly 64 KiB: no flush yet (the rule flushes
	// only when the new total would EXCEED 64 KiB).
	big := bytes.Repeat([]byte{'a'}, 65536)
	s.SendPacket(big)
	if got := ch.sentBytes(); len(got) != 0 {
		t.Fatalf("64 KiB packet flushed eagerly: %d sends", len(got))
	}
	// One more byte pushes the total over: the queue flushes first,
	// then the new packet queues.
	s.SendPacket([]byte{'b'})
	sent := ch.sentBytes()
	if len(sent) != 1 || len(sent[0]) != 65536 {
		t.Fatalf("after split, sends = %d frames (lens %v), want 1 x 65536", len(sent), lens(sent))
	}
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	sent = ch.sentBytes()
	if len(sent) != 2 || !bytes.Equal(sent[1], []byte{'b'}) {
		t.Errorf("second frame = %v, want [b]", sent)
	}
}

func lens(frames [][]byte) []int {
	out := make([]int, len(frames))
	for i, f := range frames {
		out[i] = len(f)
	}
	return out
}

func TestSessionSendPacketCopies(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	buf := []byte{1, 2, 3}
	s.SendPacket(buf)
	buf[0] = 99
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got := ch.sentBytes(); !bytes.Equal(got[0], []byte{1, 2, 3}) {
		t.Errorf("queued packet aliased caller buffer: %v", got[0])
	}
}

func TestSessionClosedChannel(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	s.SendPacket([]byte{1})
	ch.Close(1000, "bye")
	// Queued packets are dropped by Flush on a closed channel.
	if err := s.Flush(); !errors.Is(err, ErrSessionClosed) {
		t.Errorf("Flush on closed channel = %v, want ErrSessionClosed", err)
	}
	if got := ch.sentBytes(); len(got) != 0 {
		t.Errorf("flush sent on closed channel: %v", got)
	}
	// Further packets drop silently, and an empty flush is a no-op.
	s.SendPacket([]byte{2})
	if err := s.Flush(); err != nil {
		t.Errorf("Flush with empty queue on closed channel = %v, want nil", err)
	}
	if s.IsOpen() {
		t.Error("IsOpen after channel close = true")
	}
}

func TestSessionSendFailureCloses1011(t *testing.T) {
	ch := newFakeChannel()
	ch.sendErr = errors.New("boom")
	s := NewSession(ch)
	s.SendPacket([]byte{1})
	if err := s.Flush(); err == nil {
		t.Fatal("Flush with failing channel returned nil")
	}
	if len(ch.closes) != 1 || ch.closes[0][0] != 1011 {
		t.Errorf("closes = %v, want one close with 1011", ch.closes)
	}
}

func TestSessionCloseFlushesThenCloses(t *testing.T) {
	ch := newFakeChannel()
	s := NewSession(ch)
	s.SendPacket([]byte{7})
	s.Close(1000, "logout")
	sent := ch.sentBytes()
	if len(sent) != 1 || !bytes.Equal(sent[0], []byte{7}) {
		t.Errorf("Close did not flush pending: %v", sent)
	}
	if len(ch.closes) != 1 || ch.closes[0][0] != 1000 || ch.closes[0][1] != "logout" {
		t.Errorf("closes = %v", ch.closes)
	}
	if s.IsOpen() {
		t.Error("IsOpen after Close = true")
	}
	// Idempotent: no second close, packets drop.
	s.Close(1000, "again")
	s.SendPacket([]byte{8})
	if len(ch.closes) != 1 {
		t.Errorf("second Close re-closed: %v", ch.closes)
	}
	if err := s.Flush(); err != nil {
		t.Fatalf("Flush after Close: %v", err)
	}
	if got := ch.sentBytes(); len(got) != 1 {
		t.Errorf("packet sent after Close: %v", got)
	}
}

func TestSessionChannel(t *testing.T) {
	ch := newFakeChannel()
	if NewSession(ch).Channel() != Channel(ch) {
		t.Error("Channel() did not return the construction channel")
	}
}

func TestHeadlessChannel(t *testing.T) {
	h := NewHeadlessChannel("")
	if h.Kind() != "headless" || h.RemoteAddr() != "headless" || !h.IsOpen() {
		t.Errorf("default headless: kind=%q addr=%q open=%v", h.Kind(), h.RemoteAddr(), h.IsOpen())
	}
	if err := h.Send([]byte{1, 2, 3}); err != nil {
		t.Errorf("headless Send: %v", err)
	}
	h.Close(1000, "done")
	if h.IsOpen() {
		t.Error("headless open after Close")
	}
	if err := h.Send([]byte{1}); err != nil {
		t.Errorf("headless Send after close: %v", err)
	}
	h2 := NewHeadlessChannel("bot-1")
	if h2.RemoteAddr() != "bot-1" {
		t.Errorf("headless addr = %q", h2.RemoteAddr())
	}
}
