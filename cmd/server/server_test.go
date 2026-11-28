package main

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"tsps-go/internal/config"
	"tsps-go/internal/game"
	"tsps-go/internal/persist"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

const testTarget = "osrs-242_2026-11-03"

const testWorldJSON = `{
    "description": "",
    "experienceMultiplier": 1,
    "name": "Old School RuneScape",
    "spawn": {"x": 3089, "y": 3524, "z": 0},
    "zones": []
}`

// writeTestRoot creates a minimal data root (target.txt + world.json)
// and chdirs the test into it, restoring the working directory when
// the test ends.
func writeTestRoot(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte(testTarget+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defs := filepath.Join(root, "data", "definitions")
	if err := os.MkdirAll(defs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defs, "world.json"), []byte(testWorldJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	// Phase 6: buildServer loads the JSON definition files, and TS
	// fails boot when the core files below are missing (direct
	// readFileSync in the loadStartupData path), so the test root
	// ships minimal valid contents for each required file. See
	// internal/definition doc.go for the per-file behavior table.
	// npc-spawns.json is deliberately absent: a missing spawn
	// source is tolerated (zero spawns), as in TS.
	for name, content := range map[string]string{
		"interface-layouts.json": "{}",
		"object-spawns.json":     "[]",
		"ground-items.json":      "[]",
		"npc-combat-defs.json":   "{}",
		"npc-animations.json":    "{}",
		"npc-drops.json":         `{"tables": {}, "npcs": {}}`,
		"shops.json":             "[]",
		"custom-shops.json":      "[]",
		// Phase 14: npc-dialogues.json is required by the
		// dialogue loader (TS load() throws when its read
		// throws); event-dialogues.json and the index
		// tolerate absence, so they stay absent here.
		"npc-dialogues.json": "{}",
	} {
		if err := os.WriteFile(filepath.Join(defs, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
}

func testServer(t *testing.T, tickMs int) *Server {
	t.Helper()
	writeTestRoot(t)
	srv, err := buildServer(Options{
		NoCache:     true,
		TickRateMs:  tickMs,
		SaveBackend: "memory",
		Log:         log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("buildServer: %v", err)
	}
	t.Cleanup(srv.Stop)
	return srv
}

// recordingChannel is a session.Channel that captures outbound frames
// for assertions. It never delivers inbound data; tests drive the
// Connection with HandleFrame directly.
type recordingChannel struct {
	mu     sync.Mutex
	frames [][]byte
	closed bool
	code   int
	reason string
}

func (c *recordingChannel) Kind() string       { return "headless" }
func (c *recordingChannel) RemoteAddr() string { return "127.0.0.1" }
func (c *recordingChannel) Send(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("channel closed")
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	c.frames = append(c.frames, cp)
	return nil
}
func (c *recordingChannel) Close(code int, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.code = code
	c.reason = reason
}
func (c *recordingChannel) IsOpen() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}
func (c *recordingChannel) sent() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.frames))
	copy(out, c.frames)
	return out
}

// clientFrame builds one client->server packet with the framing the
// opcode's length-table entry requires.
func clientFrame(t *testing.T, opcode byte, payload []byte) []byte {
	t.Helper()
	n, ok := protocol.ClientPacketLength(opcode)
	if !ok {
		t.Fatalf("unknown client opcode %d", opcode)
	}
	frame := []byte{opcode}
	switch {
	case n == -1:
		if len(payload) > 255 {
			t.Fatalf("payload too long for -1 length: %d", len(payload))
		}
		frame = append(frame, byte(len(payload)))
	case n == -2:
		frame = append(frame, byte(len(payload)>>8), byte(len(payload)))
	default:
		if len(payload) != n {
			t.Fatalf("opcode %d: payload %d bytes, want fixed %d", opcode, len(payload), n)
		}
	}
	return append(frame, payload...)
}

func latin1(s string) []byte { return append([]byte(s), 0) }

func loginPayload(username, password string, revision int32, flags byte) []byte {
	var b []byte
	b = append(b, latin1(username)...)
	b = append(b, latin1(password)...)
	var rev [4]byte
	binary.BigEndian.PutUint32(rev[:], uint32(revision))
	b = append(b, rev[:]...)
	return append(b, flags)
}

// handshakePayload builds a minimal handshake: name, no appearance
// block, clientType 0, capabilities 0.
func handshakePayload(name string) []byte {
	var b []byte
	b = append(b, latin1(name)...)
	b = append(b, 0, 0, 0)
	return b
}

// decodeServerFrame decodes one server->client frame (one or more
// concatenated packets) using the server length table. protocol
// .DecodeFrame decodes client->server frames only, so tests that
// assert on outbound bytes need this mirror.
func decodeServerFrame(t *testing.T, frame []byte) []protocol.Packet {
	t.Helper()
	var out []protocol.Packet
	rest := frame
	for len(rest) > 0 {
		opcode := rest[0]
		n, ok := protocol.ServerPacketLength(opcode)
		if !ok {
			t.Fatalf("unknown server opcode %d", opcode)
		}
		rest = rest[1:]
		var length int
		switch {
		case n == -1:
			if len(rest) < 1 {
				t.Fatal("truncated -1 length prefix")
			}
			length = int(rest[0])
			rest = rest[1:]
		case n == -2:
			if len(rest) < 2 {
				t.Fatal("truncated -2 length prefix")
			}
			length = int(binary.BigEndian.Uint16(rest[:2]))
			rest = rest[2:]
		default:
			length = n
		}
		if len(rest) < length {
			t.Fatalf("truncated server packet opcode %d: %d bytes, want %d", opcode, len(rest), length)
		}
		payload := append([]byte(nil), rest[:length]...)
		out = append(out, protocol.Packet{Opcode: opcode, Payload: payload})
		rest = rest[length:]
	}
	return out
}

// findPacket decodes all recorded frames and returns the payload of
// the first packet with the given opcode.
func findPacket(t *testing.T, frames [][]byte, opcode byte) []byte {
	t.Helper()
	for _, frame := range frames {
		for _, p := range decodeServerFrame(t, frame) {
			if p.Opcode == opcode {
				return p.Payload
			}
		}
	}
	t.Fatalf("no packet with opcode %d in %d recorded frames", opcode, len(frames))
	return nil
}

// parseLoginResponse decodes a LOGIN_RESPONSE payload: success byte,
// error code int32, then message/displayName/token strings.
func parseLoginResponse(t *testing.T, payload []byte) (success byte, code int32, msg, display, token string) {
	t.Helper()
	r := protocol.NewReader(payload)
	var err error
	if success, err = r.Byte(); err != nil {
		t.Fatalf("login response success byte: %v", err)
	}
	if code, err = r.Int(); err != nil {
		t.Fatalf("login response code: %v", err)
	}
	if msg, err = r.String(); err != nil {
		t.Fatalf("login response message: %v", err)
	}
	if display, err = r.String(); err != nil {
		t.Fatalf("login response displayName: %v", err)
	}
	if token, err = r.String(); err != nil {
		t.Fatalf("login response token: %v", err)
	}
	return success, code, msg, display, token
}

func TestBuildServerNoCacheAndTick(t *testing.T) {
	srv := testServer(t, 10)
	if srv.Revision() != 242 {
		t.Fatalf("revision = %d, want 242", srv.Revision())
	}
	if srv.WorldName() != "Old School RuneScape" {
		t.Fatalf("world name = %q", srv.WorldName())
	}
	srv.Start()
	deadline := time.Now().Add(2 * time.Second)
	for srv.world.ProcessCycle() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("tick engine did not advance the world cycle")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLoginAndEnterWorld(t *testing.T) {
	srv := testServer(t, 10)
	srv.Start()

	ch := &recordingChannel{}
	conn := session.NewConnection(ch, srv.loginDeps, srv.adapter, 10)
	if err := conn.SendWelcome(0); err != nil {
		t.Fatalf("SendWelcome: %v", err)
	}
	welcome := findPacket(t, ch.sent(), protocol.ServerWelcome)
	if len(welcome) != 8 {
		t.Fatalf("welcome payload = %d bytes, want 8 (tickMs + serverTime)", len(welcome))
	}

	// Wrong revision is refused with code 6.
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientLogin, loginPayload("alice", "secret", 241, 0))); err != nil {
		t.Fatalf("HandleFrame(login, wrong revision): %v", err)
	}
	_, code, _, _, _ := parseLoginResponse(t, findPacket(t, ch.sent(), protocol.ServerLoginResponse))
	if code != 6 {
		t.Fatalf("wrong-revision login code = %d, want 6", code)
	}

	// Correct login succeeds (new account, password login).
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientLogin, loginPayload("alice", "secret", 242, 0))); err != nil {
		t.Fatalf("HandleFrame(login): %v", err)
	}
	// The latest login response is the success; find the last one.
	var last []byte
	for _, frame := range ch.sent() {
		for _, p := range decodeServerFrame(t, frame) {
			if p.Opcode == protocol.ServerLoginResponse {
				last = p.Payload
			}
		}
	}
	success, code, _, display, _ := parseLoginResponse(t, last)
	if success != 1 || code != -1 {
		t.Fatalf("login response = success %d code %d, want 1/-1", success, code)
	}
	// New account: the response name is the typed name (FormatDisplayName
	// only swaps underscores for spaces; it does not capitalize).
	if display != "alice" {
		t.Fatalf("login display name = %q, want %q", display, "alice")
	}

	// Handshake enters the world: the player is queued and assigned
	// an index by the next tick.
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientHandshake, handshakePayload("alice"))); err != nil {
		t.Fatalf("HandleFrame(handshake): %v", err)
	}
	if !conn.Entered() {
		t.Fatal("connection did not enter the world after handshake")
	}
	if idx := conn.PlayerIndex(); idx < 1 {
		t.Fatalf("player index = %d, want >= 1", idx)
	}
	if got := srv.world.Players().Len(); got != 1 {
		t.Fatalf("world players = %d, want 1", got)
	}

	// A second login for the same name is refused as already online.
	ch2 := &recordingChannel{}
	conn2 := session.NewConnection(ch2, srv.loginDeps, srv.adapter, 10)
	if err := conn2.HandleFrame(clientFrame(t, protocol.ClientLogin, loginPayload("alice", "secret", 242, 0))); err != nil {
		t.Fatalf("HandleFrame(second login): %v", err)
	}
	_, code, _, _, _ = parseLoginResponse(t, findPacket(t, ch2.sent(), protocol.ServerLoginResponse))
	if code != 5 {
		t.Fatalf("second login code = %d, want 5 (already online)", code)
	}

	conn.Cleanup()
	conn2.Cleanup()
}

func TestWebSocketLoginRoundTrip(t *testing.T) {
	srv := testServer(t, 10)
	srv.Start()
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http")
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()
	if err := ws.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}

	// The server sends WELCOME immediately on connect.
	msgType, welcome, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("reading welcome: %v", err)
	}
	if msgType != websocket.BinaryMessage {
		t.Fatalf("welcome message type = %d, want binary", msgType)
	}
	packets := decodeServerFrame(t, welcome)
	if len(packets) != 1 || packets[0].Opcode != protocol.ServerWelcome {
		t.Fatalf("welcome packets = %+v, want one WELCOME", packets)
	}
	if len(packets[0].Payload) != 8 {
		t.Fatalf("welcome payload = %d bytes, want 8", len(packets[0].Payload))
	}

	// LOGIN over the socket is answered with a success response.
	login := clientFrame(t, protocol.ClientLogin, loginPayload("bob", "hunter2", 242, 0))
	if err := ws.WriteMessage(websocket.BinaryMessage, login); err != nil {
		t.Fatalf("writing login: %v", err)
	}
	_, resp, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("reading login response: %v", err)
	}
	packets = decodeServerFrame(t, resp)
	if len(packets) != 1 || packets[0].Opcode != protocol.ServerLoginResponse {
		t.Fatalf("login response packets = %+v, want one LOGIN_RESPONSE", packets)
	}
	success, code, _, display, _ := parseLoginResponse(t, packets[0].Payload)
	if success != 1 || code != -1 {
		t.Fatalf("login response = success %d code %d, want 1/-1", success, code)
	}
	if display != "bob" {
		t.Fatalf("login display name = %q, want %q", display, "bob")
	}
}

func TestSaveAdapterSemantics(t *testing.T) {
	store := persist.NewMemoryStore()
	adapter := saveAdapter{store: store}

	// A missing save is (nil, nil), not an error.
	save, err := adapter.Load("nobody", persist.ProfileMain)
	if err != nil || save != nil {
		t.Fatalf("Load missing = (%v, %v), want (nil, nil)", save, err)
	}

	want := &persist.PlayerSave{DisplayName: "Carol"}
	if err := adapter.Save("carol", persist.ProfileLeague, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := adapter.Load("carol", persist.ProfileLeague)
	if err != nil || got == nil {
		t.Fatalf("Load league = (%v, %v), want save", got, err)
	}
	if got.DisplayName != "Carol" || got.Profile != persist.ProfileLeague {
		t.Fatalf("loaded save = %+v, want Carol/league", got)
	}
	// The main-profile key is separate.
	save, err = adapter.Load("carol", persist.ProfileMain)
	if err != nil || save != nil {
		t.Fatalf("Load main = (%v, %v), want (nil, nil)", save, err)
	}
}

func TestStarterWorldJSONParses(t *testing.T) {
	// The shipped starter data/definitions/world.json must load under
	// the same strict validation production uses. The test working
	// directory is the package directory (cmd/server).
	world, err := config.LoadWorld("../../data")
	if err != nil {
		t.Fatalf("loading shipped starter world.json: %v", err)
	}
	if world.Name != "Old School RuneScape" {
		t.Fatalf("starter name = %q", world.Name)
	}
	if world.Spawn.X != 3089 || world.Spawn.Y != 3524 || world.Spawn.Z != 0 {
		t.Fatalf("starter spawn = %+v, want (3089, 3524, 0)", world.Spawn)
	}
	if !world.MembersWorld {
		t.Fatal("starter world should be a members world (no FreeToPlay switch)")
	}
}

// waitFor polls condition until it reports true or the deadline passes.
func waitFor(t *testing.T, what string, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func enterTestPlayer(t *testing.T, srv *Server, username, password string) (*recordingChannel, *session.Connection) {
	t.Helper()
	ch := &recordingChannel{}
	conn := session.NewConnection(ch, srv.loginDeps, srv.adapter, 10)
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientLogin, loginPayload(username, password, 242, 0))); err != nil {
		t.Fatalf("HandleFrame(login): %v", err)
	}
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientHandshake, handshakePayload(username))); err != nil {
		t.Fatalf("HandleFrame(handshake): %v", err)
	}
	if !conn.Entered() {
		t.Fatalf("connection did not enter: %q", conn.LoginFailure())
	}
	return ch, conn
}

func TestTurnPacketBridgeEndToEnd(t *testing.T) {
	srv := testServer(t, 10)
	srv.Start()

	_, conn := enterTestPlayer(t, srv, "alice", "secret")
	defer conn.Cleanup()

	var player *game.Player
	waitFor(t, "player registration", 2*time.Second, func() bool {
		players := srv.world.Players().All()
		if len(players) == 1 {
			player = players[0]
			return true
		}
		return false
	})
	if player.TurnHandler == nil {
		t.Fatal("TurnHandler not installed by AddPlayer")
	}
	if player.OnMovementRequest == nil {
		t.Fatal("OnMovementRequest not installed by AddPlayer (movement hook missing)")
	}
	if player.MovementQueue() == nil {
		t.Fatal("MovementQueue missing on an entered player")
	}

	// The installed hook routes a WALK packet through the queue: the
	// player must leave the spawn toward (3100, 3530) over ticks.
	// NoCache mode wires the default open-ground RegionManager, so
	// nothing blocks the walk. Loc is tick-goroutine-only once the
	// player moves, so this phase waits on race-free signals only
	// (the mutex-guarded turn count, then the atomic cycle counter);
	// the position itself is snapshotted on the tick goroutine by
	// the recording hook below.
	walkPayload := []byte{0x0c, 0x1c, 0x0d, 0xca, 0x03} // (3100, 3530), flags 3
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientWalk, walkPayload)); err != nil {
		t.Fatalf("HandleFrame(walk): %v", err)
	}
	waitFor(t, "turn queue drain", 2*time.Second, func() bool {
		return player.TurnPacketCount() == 0
	})
	drainedCycle := srv.world.ProcessCycle()
	waitFor(t, "ticks after drain", 2*time.Second, func() bool {
		return srv.world.ProcessCycle() >= drainedCycle+8
	})

	// Install a recording hook: the next packets must reach it through
	// the bridge, exactly once each, on a tick cycle. The hook also
	// snapshots the player's position on the tick goroutine, which
	// is the race-free way to observe movement (see above).
	var mu sync.Mutex
	var got []game.MovementRequest
	var cycles []int64
	var snap game.Location
	player.OnMovementRequest = func(p *game.Player, req game.MovementRequest, cycle int64) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, req)
		cycles = append(cycles, cycle)
		snap = p.Loc
	}
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientWalk, walkPayload)); err != nil {
		t.Fatalf("HandleFrame(walk): %v", err)
	}
	// MOVE_GAMECLICK for the same tile: worldY shortAddLE, modifier
	// flags 2 byteNeg, worldX shortAddLE, target loc id 0 shortAdd.
	gameclick := []byte{0x0d, 0x4a, 0xfe, 0x0c, 0x9c, 0x80, 0x00}
	if err := conn.HandleFrame(clientFrame(t, protocol.NativeMoveGameclick, gameclick)); err != nil {
		t.Fatalf("HandleFrame(gameclick): %v", err)
	}
	waitFor(t, "movement hook calls", 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 2
	})
	mu.Lock()
	defer mu.Unlock()
	want := []game.MovementRequest{
		{X: 3100, Y: 3530, Run: true, ModifierFlags: 1},
		{X: 3100, Y: 3530, ModifierFlags: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("hook calls = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("hook call %d = %+v, want %+v", i, got[i], w)
		}
		if cycles[i] <= 0 {
			t.Errorf("hook call %d cycle = %d, want > 0", i, cycles[i])
		}
	}
	// Movement execution: the first WALK moved the player off the
	// spawn (the recording hook replaced the routing hook, so the
	// later packets only record). The snapshot is the position the
	// last hook call saw on the tick goroutine, after 8+ stepping
	// ticks; the queue may still be stepping, so only the departure
	// is asserted.
	if snap.X == 3089 && snap.Y == 3524 {
		t.Errorf("player never left the spawn (movement hook did not route)")
	}
}

func TestHandleTurnPacketDropsAndDecodes(t *testing.T) {
	p := game.NewPlayer("alice", game.Location{X: 3089, Y: 3524, Z: 0})
	var got []game.MovementRequest
	p.OnMovementRequest = func(_ *game.Player, req game.MovementRequest, _ int64) {
		got = append(got, req)
	}
	// Unknown opcodes are dropped.
	handleTurnPacket(p, game.TurnPacket{Opcode: protocol.ClientEmote, Payload: []byte{0, 0, 0}}, 5)
	// Malformed payloads are dropped, not fatal.
	handleTurnPacket(p, game.TurnPacket{Opcode: protocol.ClientWalk, Payload: []byte{1, 2}}, 5)
	handleTurnPacket(p, game.TurnPacket{Opcode: protocol.NativeMoveGameclick, Payload: []byte{1, 2, 3}}, 5)
	if len(got) != 0 {
		t.Fatalf("hook calls = %v, want none", got)
	}
	// A gameclick with a nonzero target loc id carries it through.
	// worldY=100 shortAddLE, flags 0, worldX=200 shortAddLE, target 1234.
	payload := []byte{0x00, 0xe4, 0x00, 0x00, 0x48, 0x52, 0x04}
	handleTurnPacket(p, game.TurnPacket{Opcode: protocol.NativeMoveGameclick, Payload: payload}, 7)
	if len(got) != 1 || got[0] != (game.MovementRequest{X: 200, Y: 100, TargetLocID: 1234}) {
		t.Fatalf("hook calls = %v, want one (200, 100, target 1234)", got)
	}
	// A nil hook drops even well-formed movement packets.
	p.OnMovementRequest = nil
	handleTurnPacket(p, game.TurnPacket{Opcode: protocol.ClientWalk, Payload: []byte{0, 1, 0, 2, 1}}, 8)
	if len(got) != 1 {
		t.Fatalf("hook calls after nil hook = %d, want 1", len(got))
	}
}

func TestHandleTurnPacketDispatchesToListeners(t *testing.T) {
	// Phase 6: non-movement opcodes reach listeners.Handle even
	// when the movement hook is nil (the hook guard covers only
	// the movement cases). Listener packets decode and drop with
	// a named reason — observably: no panic, no movement request,
	// no position change.
	p := game.NewPlayer("alice", game.Location{X: 3089, Y: 3524, Z: 0})
	var got []game.MovementRequest
	p.OnMovementRequest = func(_ *game.Player, req game.MovementRequest, _ int64) {
		got = append(got, req)
	}
	// Opcode 76 (NPC option 1): mods byte + ShortAddLE index.
	handleTurnPacket(p, game.TurnPacket{Opcode: 76, Payload: []byte{0x00, 0x01, 0x00}}, 5)
	// Opcode 96 (object option 1, fixed length 7): x + y + mods + id.
	handleTurnPacket(p, game.TurnPacket{Opcode: 96, Payload: []byte{0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0x00}}, 5)
	if len(got) != 0 {
		t.Fatalf("hook calls = %v, want none (no walk is routed for listener packets)", got)
	}
	if loc := p.Location(); loc.X != 3089 || loc.Y != 3524 {
		t.Fatalf("location moved to %v, want spawn", loc)
	}
	// Nil hook: listener dispatch still runs (must not panic).
	p.OnMovementRequest = nil
	handleTurnPacket(p, game.TurnPacket{Opcode: 76, Payload: []byte{0x00, 0x01, 0x00}}, 6)
}

func TestLogoutResponseSentBeforeClose(t *testing.T) {
	srv := testServer(t, 10)
	srv.Start()

	ch, conn := enterTestPlayer(t, srv, "alice", "secret")
	defer conn.Cleanup()

	// The logout button press is answered on the next tick.
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientLogout, []byte{})); err != nil {
		t.Fatalf("HandleFrame(logout): %v", err)
	}
	waitFor(t, "socket close after logout", 2*time.Second, func() bool {
		ch.mu.Lock()
		defer ch.mu.Unlock()
		return ch.closed
	})
	ch.mu.Lock()
	code, reason := ch.code, ch.reason
	ch.mu.Unlock()
	if code != 1000 || reason != "logout" {
		t.Fatalf("close = (%d, %q), want (1000, \"logout\")", code, reason)
	}
	// A LOGOUT_RESPONSE (success, empty reason) was sent before the
	// close: it is in the recorded frames.
	found := false
	for _, frame := range ch.sent() {
		for _, p := range decodeServerFrame(t, frame) {
			if p.Opcode == protocol.ServerLogoutResponse {
				found = true
				if len(p.Payload) != 2 || p.Payload[0] != 1 || p.Payload[1] != 0 {
					t.Errorf("logout response payload = %v, want [1 0]", p.Payload)
				}
			}
		}
	}
	if !found {
		t.Error("no LOGOUT_RESPONSE packet in recorded frames")
	}
}

// TestServerStartHooksBeforeFirstTick is a race-regression test: the
// server-startup hooks schedule work (TaskManager.Schedule from the
// smithing and enchant-jewellery ensureTask hooks) while TaskManager
// is tick-goroutine-owned and unsynchronised, so the hooks must run
// before the engine's first tick. Before the fix (hooks emitted after
// engine.Start) this test failed under -race with TaskManager.Process
// vs Schedule (TestShutdownEngineDrivenSavesWalkedPosition,
// 2026-11-22). It only bites under -race; without it, it just starts
// and stops a fast-tick server.
func TestServerStartHooksBeforeFirstTick(t *testing.T) {
	srv := testServer(t, 5)
	srv.Start()
	deadline := time.Now().Add(2 * time.Second)
	for srv.engine.TickCount() < 5 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if srv.engine.TickCount() < 5 {
		t.Fatalf("TickCount = %d, want >= 5 (engine never ticked)", srv.engine.TickCount())
	}
}
