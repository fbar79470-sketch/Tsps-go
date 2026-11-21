package session

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"tsps-go/internal/protocol"
)

// fakeWorld records WorldPort calls.
type fakeWorld struct {
	full      bool
	addErr    error
	nextIndex int
	added     []*EnteringPlayer
	removed   []int
	logouts   []int
	cycle     int64
	sink      TurnSink
}

func (w *fakeWorld) IsFull() bool { return w.full }
func (w *fakeWorld) AddPlayer(p *EnteringPlayer) (int, TurnSink, error) {
	if w.addErr != nil {
		return -1, nil, w.addErr
	}
	w.added = append(w.added, p)
	index := w.nextIndex
	w.nextIndex++
	return index, w.sink, nil
}
func (w *fakeWorld) RemovePlayer(index int) { w.removed = append(w.removed, index) }
func (w *fakeWorld) QueueLogout(index int)  { w.logouts = append(w.logouts, index) }
func (w *fakeWorld) ProcessCycle() int64    { return w.cycle }

// recordingChannel captures everything sent, like fakeChannel but with a
// configurable address and open state shared with connection tests.
type recordingChannel struct {
	*fakeChannel
}

func newRecordingChannel(addr string) *recordingChannel {
	ch := newFakeChannel()
	ch.addr = addr
	return &recordingChannel{ch}
}

// clientFrame builds one client packet frame using the real client
// length table, mirroring the TS framing (opcode + optional 1/2-byte
// length + payload).
func clientFrame(opcode byte, payload []byte) []byte {
	length, ok := protocol.ClientPacketLength(opcode)
	if !ok {
		panic(fmt.Sprintf("unknown client opcode %d", opcode))
	}
	switch {
	case length == -2:
		out := []byte{opcode, byte(len(payload) >> 8), byte(len(payload))}
		return append(out, payload...)
	case length == -1:
		out := []byte{opcode, byte(len(payload))}
		return append(out, payload...)
	default:
		if len(payload) != length {
			panic(fmt.Sprintf("opcode %d: payload %d != fixed %d", opcode, len(payload), length))
		}
		out := []byte{opcode}
		return append(out, payload...)
	}
}

func latin1(s string) []byte { return append([]byte(s), 0) }

func loginPayload(username, password string, revision int32, flags ...byte) []byte {
	out := append(latin1(username), latin1(password)...)
	var rev [4]byte
	binary.BigEndian.PutUint32(rev[:], uint32(revision))
	out = append(out, rev[:]...)
	return append(out, flags...)
}

func handshakePayload(name string, clientType, capabilities byte, withTrailer bool) []byte {
	out := latin1(name)
	out = append(out, 0) // no appearance block
	if withTrailer {
		out = append(out, clientType, capabilities)
	}
	return out
}

func pingPayload(v int32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v))
	return b[:]
}

type connFixture struct {
	ch    *recordingChannel
	world *fakeWorld
	conn  *Connection
	login *loginFixture
	now   time.Time
}

func newConnFixture(t *testing.T) *connFixture {
	t.Helper()
	lf := newLoginFixture(t)
	ch := newRecordingChannel("9.9.9.9")
	world := &fakeWorld{}
	conn := NewConnection(ch, lf.deps, world, 600)
	return &connFixture{ch: ch, world: world, conn: conn, login: lf, now: lf.now}
}

// loginAndEnter runs a full login + handshake through HandleFrame.
func (f *connFixture) loginAndEnter(t *testing.T, username, password string) {
	t.Helper()
	frame := clientFrame(protocol.ClientLogin, loginPayload(username, password, 242, 0))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("login frame: %v", err)
	}
	frame = clientFrame(protocol.ClientHandshake, handshakePayload(username, 0, 0, true))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("handshake frame: %v", err)
	}
	if !f.conn.Entered() {
		t.Fatalf("connection did not enter: failure=%q", f.conn.LoginFailure())
	}
}

func TestSendWelcome(t *testing.T) {
	f := newConnFixture(t)
	if err := f.conn.SendWelcome(123456789); err != nil {
		t.Fatalf("SendWelcome: %v", err)
	}
	sent := f.ch.sentBytes()
	if len(sent) != 1 {
		t.Fatalf("sends = %d, want 1", len(sent))
	}
	want := protocol.EncodeWelcome(600, 123456789)
	if !bytes.Equal(sent[0], want) {
		t.Errorf("welcome = %v, want %v", sent[0], want)
	}
	if sent[0][0] != protocol.ServerWelcome {
		t.Errorf("welcome opcode = %d, want %d", sent[0][0], protocol.ServerWelcome)
	}
}

func TestHandleFrameLoginHandshakeEnter(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	f.loginAndEnter(t, "Alice", "secret")

	if len(f.world.added) != 1 {
		t.Fatalf("AddPlayer calls = %d", len(f.world.added))
	}
	ep := f.world.added[0]
	if ep.Username != "Alice" || ep.DisplayName != "Alice" || ep.Profile != "main" {
		t.Errorf("EnteringPlayer identity: %+v", ep)
	}
	if ep.Session == nil || ep.Session != f.conn.Session() {
		t.Error("EnteringPlayer session mismatch")
	}
	if ep.Save == nil || ep.RemoteAddr != "9.9.9.9" {
		t.Errorf("EnteringPlayer save/addr: %+v", ep)
	}
	if f.conn.PlayerIndex() != 0 {
		t.Errorf("PlayerIndex = %d", f.conn.PlayerIndex())
	}
	// Pending names released on entering.
	if len(f.login.resv.held) != 0 {
		t.Errorf("reservations held after enter: %v", f.login.resv.held)
	}
	// The login success response went out: opcode 3, success byte 1.
	found := false
	for _, frame := range f.ch.sentBytes() {
		if len(frame) > 2 && frame[0] == protocol.ServerLoginResponse && frame[2] == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("no success login response in sends: %v", f.ch.sentBytes())
	}
	// The bootstrap was sent at the end of entering the world, so a
	// later Bootstrap call is a no-op returning nil.
	if err := f.conn.Bootstrap(); err != nil {
		t.Errorf("Bootstrap after enter = %v, want nil (idempotent)", err)
	}
}

func TestHandleFrameMalformedCloses1002(t *testing.T) {
	f := newConnFixture(t)
	// Unknown client opcode 0.
	err := f.conn.HandleFrame([]byte{0})
	var ce *CloseError
	if !errors.As(err, &ce) || ce.Code != 1002 {
		t.Fatalf("unknown opcode: err = %v, want CloseError 1002", err)
	}
	if !f.conn.IsClosed() {
		t.Error("connection not cleaned up after malformed frame")
	}
	// Truncated fixed-length packet (PING needs 4 payload bytes).
	f2 := newConnFixture(t)
	err = f2.conn.HandleFrame([]byte{protocol.ClientPing, 1, 2})
	if !errors.As(err, &ce) || ce.Code != 1002 {
		t.Fatalf("truncated: err = %v, want CloseError 1002", err)
	}
	// Framing-valid but payload-malformed login (unterminated strings).
	f3 := newConnFixture(t)
	err = f3.conn.HandleFrame(clientFrame(protocol.ClientLogin, []byte("abc")))
	if err != nil {
		t.Fatalf("login with bad payload should close inside Dispatch, HandleFrame = %v", err)
	}
	if !f3.conn.IsClosed() || len(f3.ch.closes) != 1 || f3.ch.closes[0][0] != 1002 {
		t.Errorf("bad login payload: closed=%v closes=%v", f3.conn.IsClosed(), f3.ch.closes)
	}
}

func TestHandleFrameOversizeCloses1008(t *testing.T) {
	f := newConnFixture(t)
	frame := bytes.Repeat([]byte{protocol.ClientPing}, 65537)
	err := f.conn.HandleFrame(frame)
	var ce *CloseError
	if !errors.As(err, &ce) || ce.Code != 1008 {
		t.Fatalf("oversize: err = %v, want CloseError 1008", err)
	}
}

func TestDispatchImmediateVsTurn(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	f.loginAndEnter(t, "Alice", "secret")

	// A WALK packet queues as a turn packet.
	walk := clientFrame(protocol.ClientWalk, []byte{0x0c, 0x35, 0x0d, 0xc4, 0x00})
	if err := f.conn.HandleFrame(walk); err != nil {
		t.Fatalf("walk frame: %v", err)
	}
	// A PING is immediate: sets keepalive, does not queue.
	if err := f.conn.HandleFrame(clientFrame(protocol.ClientPing, pingPayload(7))); err != nil {
		t.Fatalf("ping frame: %v", err)
	}
	// A HELLO is immediate and ignored.
	if err := f.conn.HandleFrame(clientFrame(protocol.ClientHello, clientHelloPayload())); err != nil {
		t.Fatalf("hello frame: %v", err)
	}
	turn := f.conn.TurnPackets()
	if len(turn) != 1 || turn[0].Opcode != protocol.ClientWalk {
		t.Errorf("turn packets = %+v, want 1 WALK", turn)
	}
	if len(f.conn.TurnPackets()) != 0 {
		t.Error("TurnPackets did not drain")
	}
	if !f.conn.CheckIdle(f.now.Add(31 * time.Second)) {
		t.Error("idle check false after ping + 31s (keepalive should be armed)")
	}
	// LOGOUT is immediate: queued with the world, not as a turn packet.
	if err := f.conn.HandleFrame(clientFrame(protocol.ClientLogout, []byte{})); err != nil {
		t.Fatalf("logout frame: %v", err)
	}
	if len(f.world.logouts) != 1 || f.world.logouts[0] != 0 {
		t.Errorf("logouts = %v", f.world.logouts)
	}
	if len(f.conn.TurnPackets()) != 0 {
		t.Error("logout queued as turn packet")
	}
	if f.conn.IsClosed() {
		t.Error("logout packet closed the connection (TS answers on the next tick)")
	}
}

// clientHelloPayload builds a HELLO payload per the TS decoder: two
// strings. The length table entry decides the framing; read it back.
func clientHelloPayload() []byte {
	return append(latin1("client"), latin1("1")...)
}

func TestDispatchPreLoginDropsGameplay(t *testing.T) {
	f := newConnFixture(t)
	// A WALK packet before login is dropped, not queued.
	walk := clientFrame(protocol.ClientWalk, []byte{0x0c, 0x35, 0x0d, 0xc4, 0x00})
	if err := f.conn.HandleFrame(walk); err != nil {
		t.Fatalf("walk frame: %v", err)
	}
	if len(f.conn.TurnPackets()) != 0 {
		t.Error("pre-login walk queued")
	}
	// LOGOUT before login is a no-op.
	if err := f.conn.HandleFrame(clientFrame(protocol.ClientLogout, []byte{})); err != nil {
		t.Fatalf("logout frame: %v", err)
	}
	if len(f.world.logouts) != 0 {
		t.Error("pre-login logout reached the world")
	}
	// Handshake without a pending login is a no-op.
	if err := f.conn.HandleFrame(clientFrame(protocol.ClientHandshake, handshakePayload("Nobody", 0, 0, true))); err != nil {
		t.Fatalf("handshake frame: %v", err)
	}
	if len(f.world.added) != 0 {
		t.Error("handshake without login entered the world")
	}
}

func TestDispatchTurnCapCloses1008(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	f.loginAndEnter(t, "Alice", "secret")
	walk := clientFrame(protocol.ClientWalk, []byte{0x0c, 0x35, 0x0d, 0xc4, 0x00})
	for i := 0; i < 240; i++ {
		if err := f.conn.HandleFrame(walk); err != nil {
			t.Fatalf("walk %d: %v", i, err)
		}
	}
	if f.conn.IsClosed() {
		t.Fatal("closed at exactly 240 queued packets (cap is >= 240 on the NEXT)")
	}
	if err := f.conn.HandleFrame(walk); err != nil {
		t.Fatalf("241st walk: %v", err)
	}
	if !f.conn.IsClosed() || len(f.ch.closes) != 1 || f.ch.closes[0][0] != 1008 {
		t.Errorf("cap exceeded: closed=%v closes=%v", f.conn.IsClosed(), f.ch.closes)
	}
	if len(f.world.removed) != 1 {
		t.Errorf("player not queued for removal on cap close: %v", f.world.removed)
	}
}

func TestLoginFailureKeepsSocketOpen(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	// Wrong revision: failure response, socket open, retry succeeds.
	bad := clientFrame(protocol.ClientLogin, loginPayload("Alice", "secret", 241, 0))
	if err := f.conn.HandleFrame(bad); err != nil {
		t.Fatalf("bad login frame: %v", err)
	}
	if f.conn.IsClosed() || !f.ch.IsOpen() {
		t.Fatal("failed login closed the socket")
	}
	if f.conn.LoginFailure() != "Please close the client and reload to update." {
		t.Errorf("LoginFailure = %q", f.conn.LoginFailure())
	}
	if len(f.login.resv.held) != 0 {
		t.Errorf("reservations held after failed login: %v", f.login.resv.held)
	}
	f.loginAndEnter(t, "Alice", "secret")
}

func TestEnterWorldBanRecheck(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	frame := clientFrame(protocol.ClientLogin, loginPayload("Alice", "secret", 242, 0))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("login frame: %v", err)
	}
	// Ban lands between login and handshake.
	if err := f.login.deps.Punishment.Ban("Alice"); err != nil {
		t.Fatal(err)
	}
	frame = clientFrame(protocol.ClientHandshake, handshakePayload("Alice", 1, 1, true))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("handshake frame: %v", err)
	}
	if f.conn.Entered() || len(f.world.added) != 0 {
		t.Error("banned player entered the world")
	}
	if f.conn.LoginFailure() != "Your account has been disabled." {
		t.Errorf("LoginFailure = %q", f.conn.LoginFailure())
	}
	if len(f.login.resv.held) != 0 {
		t.Errorf("reservations held after ban rejection: %v", f.login.resv.held)
	}
	// The socket stays open (failLogin semantics): no close sent.
	if len(f.ch.closes) != 0 {
		t.Errorf("closes = %v, want none", f.ch.closes)
	}
}

func TestEnterWorldFullCloses1013(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	f.world.addErr = errors.New("full")
	frame := clientFrame(protocol.ClientLogin, loginPayload("Alice", "secret", 242, 0))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("login frame: %v", err)
	}
	frame = clientFrame(protocol.ClientHandshake, handshakePayload("Alice", 0, 0, true))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("handshake frame: %v", err)
	}
	if f.conn.Entered() {
		t.Error("entered despite AddPlayer error")
	}
	if len(f.ch.closes) != 1 || f.ch.closes[0][0] != 1013 {
		t.Errorf("closes = %v, want one 1013", f.ch.closes)
	}
	if f.conn.LoginFailure() != "This world is full." {
		t.Errorf("LoginFailure = %q", f.conn.LoginFailure())
	}
}

func TestEnterWorldCarriesCredentialName(t *testing.T) {
	// Primary login: credential name equals the username.
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	f.loginAndEnter(t, "Alice", "secret")
	if len(f.world.added) != 1 {
		t.Fatalf("added = %d", len(f.world.added))
	}
	if got := f.world.added[0].CredentialName; got != "Alice" {
		t.Errorf("CredentialName = %q, want Alice", got)
	}

	// Alt login: the credential name is the primary's name, so the
	// adapter can apply the primary-only token-fold rule.
	f2 := newConnFixture(t)
	f2.login.seedAccount(t, "Alice", "secret", "Alicia")
	f2.loginAndEnter(t, "Alicia", "secret")
	if len(f2.world.added) != 1 {
		t.Fatalf("added = %d", len(f2.world.added))
	}
	ep := f2.world.added[0]
	if ep.Username != "Alicia" || ep.CredentialName != "Alice" {
		t.Errorf("EnteringPlayer names = username %q credential %q, want Alicia/Alice", ep.Username, ep.CredentialName)
	}
}

func TestEnterWorldCarriesClientTypeAndCapabilities(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	frame := clientFrame(protocol.ClientLogin, loginPayload("Alice", "secret", 242, 0))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("login: %v", err)
	}
	frame = clientFrame(protocol.ClientHandshake, handshakePayload("Alice", 1, protocol.HandshakeCapabilityNativeClient, true))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if len(f.world.added) != 1 {
		t.Fatalf("added = %d", len(f.world.added))
	}
	ep := f.world.added[0]
	if ep.ClientType != 1 || ep.Capabilities != protocol.HandshakeCapabilityNativeClient {
		t.Errorf("clientType=%d capabilities=%d", ep.ClientType, ep.Capabilities)
	}
}

func TestTimeoutPredicates(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")

	// Before login: neither timeout applies, even after 31s + ping.
	if err := f.conn.HandleFrame(clientFrame(protocol.ClientPing, pingPayload(1))); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if f.conn.CheckIdle(f.now.Add(31 * time.Second)) {
		t.Error("idle timeout before entering the world")
	}
	if f.conn.CheckPendingLogin(f.now.Add(6 * time.Minute)) {
		t.Error("pending timeout with no pending login")
	}

	// After login, before handshake: pending timeout applies at 5 min.
	frame := clientFrame(protocol.ClientLogin, loginPayload("Alice", "secret", 242, 0))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("login: %v", err)
	}
	if f.conn.CheckPendingLogin(f.now.Add(5*time.Minute - time.Second)) {
		t.Error("pending timeout early")
	}
	if !f.conn.CheckPendingLogin(f.now.Add(5 * time.Minute)) {
		t.Error("pending timeout not reached at 5 min")
	}
	// Idle still gated on entering.
	if f.conn.CheckIdle(f.now.Add(31 * time.Second)) {
		t.Error("idle timeout before entering despite ping")
	}

	// After entering: pending timeout cleared, idle timeout live.
	frame = clientFrame(protocol.ClientHandshake, handshakePayload("Alice", 0, 0, true))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if f.conn.CheckPendingLogin(f.now.Add(6 * time.Minute)) {
		t.Error("pending timeout after entering")
	}
	// Last frame was the handshake at f.now; ping arms keepalive (already
	// armed pre-login).
	if f.conn.CheckIdle(f.now.Add(29 * time.Second)) {
		t.Error("idle timeout early")
	}
	if !f.conn.CheckIdle(f.now.Add(30 * time.Second)) {
		t.Error("idle timeout not reached at 30s")
	}
}

func TestIdleRequiresPing(t *testing.T) {
	// A client that never pings is never timed out (older clients
	// without keepalive).
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	f.loginAndEnter(t, "Alice", "secret")
	if f.conn.CheckIdle(f.now.Add(time.Hour)) {
		t.Error("idle timeout without any ping")
	}
}

func TestCleanupReleasesAndRemoves(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	f.loginAndEnter(t, "Alice", "secret")
	f.conn.Cleanup()
	if !f.conn.IsClosed() {
		t.Error("not closed after Cleanup")
	}
	if len(f.world.removed) != 1 || f.world.removed[0] != 0 {
		t.Errorf("removed = %v", f.world.removed)
	}
	// Idempotent: no double removal.
	f.conn.Cleanup()
	if len(f.world.removed) != 1 {
		t.Errorf("double cleanup removed twice: %v", f.world.removed)
	}
	// Frames after cleanup are dropped.
	if err := f.conn.HandleFrame(clientFrame(protocol.ClientPing, pingPayload(1))); err != nil {
		t.Errorf("frame after cleanup: %v", err)
	}
}

func TestCleanupPendingReleasesNames(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	frame := clientFrame(protocol.ClientLogin, loginPayload("Alice", "secret", 242, 0))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("login: %v", err)
	}
	if len(f.login.resv.held) == 0 {
		t.Fatal("no reservation held after login")
	}
	f.conn.Cleanup()
	if len(f.login.resv.held) != 0 {
		t.Errorf("reservations held after cleanup: %v", f.login.resv.held)
	}
	if len(f.world.removed) != 0 {
		t.Error("removal queued without entering")
	}
}

func TestStartIdleCheckStopAndDoubleStart(t *testing.T) {
	f := newConnFixture(t)
	stop := make(chan struct{})
	f.conn.StartIdleCheck(stop)
	f.conn.StartIdleCheck(stop) // no-op, no panic, no second goroutine leak
	close(stop)
	// Give the goroutine a chance to exit; the test ending would catch a
	// leak only via -race/deadlock, so just exercise the path.
	time.Sleep(10 * time.Millisecond)
}

func TestCharacterPacketsDroppedButValidated(t *testing.T) {
	f := newConnFixture(t)
	// Well-formed character_select: accepted, dropped, connection open.
	frame := clientFrame(protocol.ClientCharacterSelect, latin1("Alice"))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("character_select: %v", err)
	}
	if f.conn.IsClosed() {
		t.Error("character_select closed the connection")
	}
	// Malformed (unterminated string): 1002.
	f2 := newConnFixture(t)
	if err := f2.conn.HandleFrame(clientFrame(protocol.ClientCharacterSelect, []byte("abc"))); err != nil {
		t.Fatalf("malformed character_select HandleFrame: %v", err)
	}
	if !f2.conn.IsClosed() || len(f2.ch.closes) != 1 || f2.ch.closes[0][0] != 1002 {
		t.Errorf("malformed character_select: closed=%v closes=%v", f2.conn.IsClosed(), f2.ch.closes)
	}
	// login_forget well-formed: dropped.
	f3 := newConnFixture(t)
	forget := append(latin1("Alice"), latin1("token")...)
	if err := f3.conn.HandleFrame(clientFrame(protocol.ClientLoginForget, forget)); err != nil {
		t.Fatalf("login_forget: %v", err)
	}
	if f3.conn.IsClosed() {
		t.Error("login_forget closed the connection")
	}
}

func TestBootstrapBeforeEnter(t *testing.T) {
	f := newConnFixture(t)
	err := f.conn.Bootstrap()
	if err == nil {
		t.Error("Bootstrap before enter = nil, want an error")
	}
}

func TestMultiplePacketsPerFrame(t *testing.T) {
	f := newConnFixture(t)
	f.login.seedAccount(t, "Alice", "secret")
	// Login + handshake concatenated in ONE frame, as headless clients do.
	frame := append(
		clientFrame(protocol.ClientLogin, loginPayload("Alice", "secret", 242, 0)),
		clientFrame(protocol.ClientHandshake, handshakePayload("Alice", 0, 0, true))...,
	)
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("combined frame: %v", err)
	}
	if !f.conn.Entered() {
		t.Fatalf("did not enter; failure=%q", f.conn.LoginFailure())
	}
}

func TestLoginResponseBytes(t *testing.T) {
	f := newConnFixture(t)
	// Failure response bytes: opcode 3, length, success=0, code, message
	// + two empty strings.
	frame := clientFrame(protocol.ClientLogin, loginPayload("Alice", "secret", 241, 0))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("login: %v", err)
	}
	sent := f.ch.sentBytes()
	if len(sent) != 1 {
		t.Fatalf("sends = %d", len(sent))
	}
	want := protocol.EncodeLoginResponse(false, 6, "Please close the client and reload to update.", "", "")
	if !bytes.Equal(sent[0], want) {
		t.Errorf("failure response = %v, want %v", sent[0], want)
	}
}

func TestHeadlessEndToEnd(t *testing.T) {
	// The headless path: no socket, login + handshake via HandleFrame.
	lf := newLoginFixture(t)
	lf.seedAccount(t, "Bot", "secret")
	ch := NewHeadlessChannel("")
	world := &fakeWorld{}
	conn := NewConnection(ch, lf.deps, world, 600)
	if err := conn.SendWelcome(1); err != nil {
		t.Fatalf("welcome: %v", err)
	}
	frame := append(
		clientFrame(protocol.ClientLogin, loginPayload("Bot", "secret", 242, 0)),
		clientFrame(protocol.ClientHandshake, handshakePayload("Bot", 0, 0, true))...,
	)
	if err := conn.HandleFrame(frame); err != nil {
		t.Fatalf("frames: %v", err)
	}
	if !conn.Entered() || len(world.added) != 1 {
		t.Fatalf("headless enter failed: entered=%v added=%d failure=%q", conn.Entered(), len(world.added), conn.LoginFailure())
	}
	if got := conn.RemoteAddr(); got != "headless" {
		t.Errorf("headless remote addr = %q", got)
	}
}
