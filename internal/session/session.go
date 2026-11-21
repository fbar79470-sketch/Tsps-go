package session

import (
	"errors"
	"sync"

	"tsps-go/internal/config"
	"tsps-go/internal/protocol"
)

// ErrSessionClosed is returned by Flush when the channel is no longer
// open. The pending queue is dropped first, matching TS flushPackets,
// which clears the queue before checking the channel.
var ErrSessionClosed = errors.New("session: channel closed")

// Session is the per-player outbound packet queue, porting the
// queue/flush core of PlayerSession (PlayerSession.ts) only:
// sendClientPacket (lines 287-311), flushPackets (lines 331-355), and
// the flush(tick) entry point (lines 232-234). FlushTick (flush.go)
// ports flushClient (lines 355-660) on top of this queue, reduced to
// the ported snapshot state; its doc lists exactly what it omits.
//
// Session satisfies game.SessionPort structurally (SendPacket/FlushTick/
// Close/IsOpen); this package does not import game (game must never
// import session), and sessionport_test.go asserts the match.
type Session struct {
	mu           sync.Mutex
	ch           Channel
	pending      [][]byte
	pendingBytes int
	closed       bool

	// Flush wiring, guarded by mu (see flush.go): the sync source,
	// the XTEA provider, the region-replacement sender, and this
	// session's own stored appearance block. Set by the
	// coordinator glue; FlushTick snapshots them under mu and
	// calls the source without holding it.
	syncSource              FlushSource
	xteaProvider            XteaProvider
	regionReplacementSender func(tileX, tileY int)
	syncAppearance          []byte

	// Per-tick sync state, porting the PlayerSession fields
	// (PlayerSession.ts:116-131) plus the phase 8 music, ground-item,
	// and effect-queue fields. Tick-goroutine-only: only FlushTick
	// touches these, so mu does not guard them (see flush.go).
	// The replayed-area fields (replayedPrivateArea,
	// replayedSceneVersion, replayedTemplated) port the scene
	// replay bookkeeping (PlayerSession.ts:653-660): the private
	// area identity, scene version, and templated-ness the last
	// replayed scene carried. replayedPrivateArea holds the
	// comparable area identity from the owner's SyncArea
	// snapshot (see flush.go); nil means the replayed scene was
	// the main world (or a main-world-counting area).
	sceneBaseX, sceneBaseY                 int
	replayedSceneBaseX, replayedSceneBaseY int
	replayedSceneLevel                     int
	replayedPrivateArea                    any
	replayedSceneVersion                   int
	replayedTemplated                      bool
	hasReplayedScene                       bool
	playerSyncState                        *protocol.PlayerSyncState
	npcSyncState                           *protocol.NpcSyncState
	appearanceCache                        map[int]struct {
		identity any
		payload  []byte
	}
	pendingInitialLocalAppearance bool
	// music is the per-session music state machine (music.go);
	// groundItems is the per-session ground-item send state
	// (grounditems.go); objects is the per-session object send
	// state (objects.go); npcSpecificEffects is the transient
	// viewer-specific effect queue (effects.go). lastGroundItemRegion
	// and lastGroundItemLevel are the region/level of the last
	// ground-item region-change send (PlayerSession.ts:201-202),
	// -1 before the first flush.
	music                *musicState
	groundItems          *GroundItemSync
	objects              *ObjectSync
	npcSpecificEffects   NpcSpecificEffects
	lastGroundItemRegion int
	lastGroundItemLevel  int

	// iface is the per-session interface runtime state
	// (interface_state.go): the sub-interface tracking map and
	// the wiring seams. Guarded by mu, like the queue: sender
	// methods run from both the login goroutine (hooks) and the
	// tick goroutine. Initialized by NewSession; methods
	// lazily re-create it for a zero-value Session, as the
	// music state does.
	iface *interfaceState

	// varps is the per-session live varp store (varps.go),
	// porting the PacketSender.varps map (PacketSender.ts:221).
	// Worker 3 owns the store type and the SendVarp/SendVarbit
	// methods (runstatus.go); this field + the NewSession init
	// are Worker 1's wiring. Guarded by the store's own
	// conventions (see varps.go), not by mu.
	varps *varpStore

	// Chatbox state, porting the PacketSender fields
	// (PacketSender.ts:227-231): chatboxGroupId is the mounted
	// chatbox group, -1 when none is mounted; chatboxOwner is the
	// owner recorded by the last mount from the presenting owner
	// in force at the time; presentingChatboxOwner is the owner a
	// WithChatboxOwner presentation scopes to mounts inside it;
	// chatboxUnclamped is whether the mounted chatbox was mounted
	// unclamped. Guarded by mu, like the queue: dialogue sends
	// arrive from the tick goroutine (listeners, hooks) and the
	// login goroutine (login hooks), and SendPacket takes mu too,
	// so WithChatboxOwner sets/restores under mu but runs the
	// presentation outside it (TS is single-threaded; concurrent
	// presentations from two goroutines could interleave owners,
	// but presentations are synchronous and short-lived).
	chatboxGroupId         int
	chatboxOwner           any
	presentingChatboxOwner any
	chatboxUnclamped       bool
}

// NewSession returns a Session writing to ch. The sync state starts
// as TS constructs it: scene bases and the replayed level at -1, a
// fresh NPC sync state, an empty appearance cache, and the initial
// local appearance pending.
func NewSession(ch Channel) *Session {
	s := &Session{
		ch:                 ch,
		sceneBaseX:         -1,
		sceneBaseY:         -1,
		replayedSceneBaseX: -1,
		replayedSceneBaseY: -1,
		replayedSceneLevel: -1,
		npcSyncState:       protocol.CreateNpcSyncState(),
		appearanceCache: map[int]struct {
			identity any
			payload  []byte
		}{},
		pendingInitialLocalAppearance: true,
		music:                         newMusicState(),
		lastGroundItemRegion:          -1,
		lastGroundItemLevel:           -1,
		chatboxGroupId:                -1,
		iface:                         newInterfaceState(),
		varps:                         &varpStore{values: make(map[int]int)},
	}
	s.groundItems = NewGroundItemSync(s)
	s.objects = NewObjectSync(s)
	return s
}

// SendPacket appends an encoded packet to the pending queue, porting
// PlayerSession.sendClientPacket (PlayerSession.ts:287-311): if the
// pending bytes plus this packet would exceed 64 KiB, the queue is
// flushed first; a packet arriving on a closed channel is dropped.
//
// Two TS behaviors are deliberately not reproduced:
//   - The microtask auto-flush (queueMicrotask -> flushPackets): Go has
//     no microtask queue. Queued packets go out on the next explicit
//     Flush/FlushTick; the game loop flushes every tick, and pre-login
//     traffic bypasses the session (Connection.send writes direct).
//   - The outbound backpressure guards (OUTBOUND_WS_BUFFER_*): the Go
//     Channel has no bufferedAmount, and gorilla writes are synchronous,
//     so there is no unbounded socket buffer to guard. The pending queue
//     itself stays bounded by the 64 KiB flush-first rule.
func (s *Session) SendPacket(packet []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !s.ch.IsOpen() {
		return
	}
	if s.pendingBytes+len(packet) > config.MaxFrameBytes {
		if err := s.flushLocked(); err != nil {
			// TS: a failed flush drops the new packet too.
			return
		}
	}
	cp := make([]byte, len(packet))
	copy(cp, packet)
	s.pending = append(s.pending, cp)
	s.pendingBytes += len(cp)
}

// Flush concatenates the pending buffers into ONE frame and sends it,
// porting flushPackets (PlayerSession.ts:331-355). An empty queue sends
// nothing and returns nil. A send failure closes the channel with code
// 1011 (WS_CLOSE_SEND_FAILURE, PlayerSession.ts:79) and returns the
// error.
func (s *Session) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushLocked()
}

func (s *Session) flushLocked() error {
	if len(s.pending) == 0 {
		return nil
	}
	frame := make([]byte, 0, s.pendingBytes)
	for _, p := range s.pending {
		frame = append(frame, p...)
	}
	// Cleared before the send attempt, as in TS: a failed flush does not
	// retry the same packets.
	s.pending = nil
	s.pendingBytes = 0
	if s.closed || !s.ch.IsOpen() {
		return ErrSessionClosed
	}
	if err := s.ch.Send(frame); err != nil {
		s.ch.Close(1011, "send failed")
		return err
	}
	return nil
}

// FlushTick lives in flush.go: it ports flushClient on top of the
// queue above. With no FlushSource attached it degrades to Flush.

// Close flushes any pending packets, then terminates the channel with
// code and reason. Flushing first matches the TS logout() sequence
// (PlayerSession.ts:323-329: queue the final packets, flushPackets,
// then close). It is idempotent: a second call neither flushes nor
// re-closes. Later SendPacket calls drop.
func (s *Session) Close(code int, reason string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	_ = s.flushLocked()
	s.closed = true
	s.mu.Unlock()
	s.ch.Close(code, reason)
}

// IsOpen reports whether the session can still send: Close has not been
// called and the channel is open.
func (s *Session) IsOpen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.ch.IsOpen()
}

// Channel returns the underlying transport.
func (s *Session) Channel() Channel {
	return s.ch
}
