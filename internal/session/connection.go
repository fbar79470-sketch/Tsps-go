package session

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"tsps-go/internal/config"
	"tsps-go/internal/persist"
	"tsps-go/internal/protocol"
)

// Close codes used on this connection, all from NetworkBuilder.ts:
// 1001 idle/pending-login timeout, 1002 malformed packet, 1008 input
// limit, 1011 login-extension/send failure, 1013 world full /
// backpressure.

// CloseError is the typed error HandleFrame returns when the caller must
// close the connection with a WebSocket close code: 1002 for a malformed
// frame, 1008 for an oversize frame. The handler maps it with errors.As.
type CloseError struct {
	Code   int
	Reason string
	Err    error
}

func (e *CloseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("session: close %d (%s): %v", e.Code, e.Reason, e.Err)
	}
	return fmt.Sprintf("session: close %d (%s)", e.Code, e.Reason)
}

// Unwrap returns the decode error, if any.
func (e *CloseError) Unwrap() error { return e.Err }

// LoginRejection is returned by EnterWorld when entering is refused
// after the handshake (ban re-check, world full). Code/Message are the
// failLogin values already sent to the client.
type LoginRejection struct {
	Code    int32
	Message string
}

func (e *LoginRejection) Error() string {
	return fmt.Sprintf("session: login rejected (%d): %s", e.Code, e.Message)
}

// EnteringPlayer is everything the game world needs to admit a player,
// carried from the pending login across the handshake. It ports the
// Player construction in enterWorld (NetworkBuilder.ts:1653-1678). The
// cmd/server adapter builds a game.Player from it; the adapter owns:
//   - resolving the shown display name via DisplayNames.NameOf(Username)
//     (TS: player.setDisplayName(DisplayNames.nameOf(...))),
//   - the long username (NameToLong(Username)),
//   - mobile/native transient attributes from ClientType/Capabilities
//     (clientType 1 = mobile; capabilities bit 0x1 = native client,
//     protocol.HandshakeCapabilityNativeClient),
//   - applying Save (applyToPlayer) or attribute defaults for a new
//     character, the password hash, and LoginTokens on the primary,
//   - DEVELOPER rights for configured developer usernames,
//   - the spawn location and appearance flag.
type EnteringPlayer struct {
	Username     string
	DisplayName  string
	Profile      string
	Session      *Session
	Save         *persist.PlayerSave
	PasswordHash string
	RemoteAddr   string
	ClientType   byte
	Capabilities byte

	// CredentialName is the account primary's name, carried from the
	// pending login: the adapter folds LoginTokens into the carried
	// save only when the entering character is the primary, i.e.
	// Accounts.key(CredentialName) == Accounts.key(Username)
	// (NetworkBuilder.ts:1672-1674).
	CredentialName string

	// LoginTokens carries remembered-login entries for a primary
	// entering without a save (new account / unmade save); see
	// PendingLogin.LoginTokens.
	LoginTokens []persist.TokenEntry
}

// WorldPort is the interface the game world implements; the session
// calls it from EnterWorld and from logout/cleanup. The game agent
// implements this shape against its own types via an adapter in the
// cmd/server wiring (the coordinator owns that adapter).
//
// NOTE on AddPlayer: TS World.getPlayers().add() assigns the player
// index synchronously, and enterWorld uses the index immediately. The
// Go game.World queues additions for the next tick instead, so the
// adapter must decide how the index is produced (e.g. by reserving it
// from the player list). Any error from AddPlayer is treated as
// world-full (code 2 + close 1013), matching TS where add() fails only
// when full. AddPlayer also returns the connection -> player
// turn-packet bridge for the admitted player (see TurnSink in
// bridge.go): a nil sink means the connection queues turn packets
// locally for draining via TurnPackets.
//
// ProcessCycle reports the world's current process cycle, which the
// login bootstrap sends as the gameframe's map clock
// (NetworkBuilder.ts:1720). It must be safe to call from the
// connection's read loop.
type WorldPort interface {
	IsFull() bool
	AddPlayer(p *EnteringPlayer) (index int, sink TurnSink, err error)
	RemovePlayer(index int)
	QueueLogout(index int)
	ProcessCycle() int64
}

// Timeout constants, from ClientConnection (NetworkBuilder.ts:449-456).
const (
	// IdleTimeout is how long a logged-in connection that pings may go
	// without any frame before it is closed (30 s).
	IdleTimeout = 30 * time.Second
	// IdleCheckInterval is how often the idle check runs (5 s).
	IdleCheckInterval = 5 * time.Second
	// PendingLoginTimeout is how long a login may wait for its
	// handshake before the connection is closed (5 minutes).
	PendingLoginTimeout = 5 * time.Minute
)

// immediateOpcodes are the client opcodes acted on as they arrive,
// porting IMMEDIATE_PACKETS (NetworkBuilder.ts:290-303) by opcode. The
// TS set is keyed by decoded message type; each of these types maps to
// exactly one opcode, except "raw" (any known-length opcode without a
// typed decoder), which needs no entry here: untyped packets from a
// logged-in player queue as turn packets and are ignored when the game
// layer decodes them, and from a pre-login connection they are dropped,
// matching the TS switch (case "raw": continue, and non-immediate
// packets without a player fall through guarded cases).
var immediateOpcodes = map[byte]bool{
	protocol.ClientHello:           true,
	protocol.ClientPing:            true,
	protocol.ClientLogin:           true,
	protocol.ClientLoginForget:     true,
	protocol.ClientCharacterSelect: true,
	protocol.ClientCharacterCreate: true,
	protocol.ClientCharacterDelete: true,
	protocol.ClientHandshake:       true,
	protocol.ClientPluginMessage:   true,
	protocol.ClientMovementInput:   true,
	protocol.ClientLogout:          true,
}

// Connection ports ClientConnection (NetworkBuilder.ts:440-2018),
// transport-agnostic over a Channel. It is driven synchronously by one
// read loop (the HTTP handler): frames arrive via HandleFrame, which
// decodes and dispatches inline, matching the TS serialized promise
// chain (this.input). Because dispatch is inline, the TS queued-input
// frame count (queuedInputFrames, capped at PACKET_QUEUE_LIMIT) has no
// Go equivalent and cannot trigger; the turn-packet cap is enforced in
// Dispatch instead.
//
// NOT ported: bot control (controlPlayer/releasePlayerControl/
// endPlayerControl), stalled-input holding (animation stalls are a game
// concern), the plugin login extension hooks, and plugin-message
// handling (no plugin system exists in Go yet). See Dispatch.
type Connection struct {
	mu     sync.Mutex
	ch     Channel
	deps   *LoginDeps
	world  WorldPort
	tickMs int

	closed         bool
	entered        bool
	playerIndex    int
	session        *Session
	pending        *PendingLogin
	pendingAt      time.Time
	turnPackets    []protocol.Packet
	turnSink       TurnSink
	lastFrameAt    time.Time
	sendsKeepalive bool
	loginFailure   string
	idleStarted    bool

	// Bootstrap state, retained across enterWorld for
	// sendBootstrapLocked (bootstrap.go): the save snapshot, the
	// username the handshake and gameframe carry, the display name
	// the sync appearance block carries (pending.DisplayName, which
	// may differ from the username for alt characters), and the
	// handshake's clientType/capabilities. Set in enterWorldLocked;
	// the save is read, never mutated.
	bootstrapped          bool
	bootstrapSave         *persist.PlayerSave
	bootstrapUsername     string
	bootstrapDisplayName  string
	bootstrapClientType   byte
	bootstrapCapabilities byte
}

// NewConnection returns a Connection over ch. deps may be nil (logins
// then fail closed with code 8); world may be nil (entering the world
// then fails with code 8). tickMs is the tick rate sent in the welcome
// packet (GameConstants.GAME_ENGINE_PROCESSING_CYCLE_RATE, 600).
func NewConnection(ch Channel, deps *LoginDeps, world WorldPort, tickMs int) *Connection {
	if deps == nil {
		deps = &LoginDeps{}
	}
	return &Connection{
		ch:          ch,
		deps:        deps,
		world:       world,
		tickMs:      tickMs,
		playerIndex: -1,
		lastFrameAt: deps.now(),
	}
}

func (c *Connection) now() time.Time { return c.deps.now() }

// SendWelcome sends the WELCOME packet immediately on open: opcode 0,
// tickMs + serverTime (encodeWelcome, ClientProtocol.ts:2739). TS sends
// it from the ClientConnection constructor; in Go the handler calls this
// right after NewConnection. serverTime is milliseconds since the epoch
// truncated to int32, as TS Date.now() | 0.
func (c *Connection) SendWelcome(serverTime int32) error {
	return c.ch.Send(protocol.EncodeWelcome(int32(c.tickMs), serverTime))
}

// HandleFrame decodes one WebSocket message and dispatches it, porting
// the onData handler + handle() (NetworkBuilder.ts:557-620). A malformed
// frame returns a *CloseError with code 1002; an oversize frame
// (>64 KiB) returns a *CloseError with code 1008. In both cases the
// connection is cleaned up first (pending names released, player queued
// for removal); the caller must still close the channel with the
// returned code. Frames arriving after cleanup are dropped.
func (c *Connection) HandleFrame(frame []byte) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.lastFrameAt = c.now()
	c.mu.Unlock()

	if len(frame) > config.MaxFrameBytes {
		c.cleanup()
		return &CloseError{Code: 1008, Reason: "input limit",
			Err: fmt.Errorf("frame of %d bytes exceeds %d", len(frame), config.MaxFrameBytes)}
	}
	packets, err := protocol.DecodeFrame(frame)
	if err != nil {
		c.cleanup()
		return &CloseError{Code: 1002, Reason: "malformed packet", Err: err}
	}
	c.Dispatch(packets)
	return nil
}

// Dispatch acts on decoded packets, porting dispatch()
// (NetworkBuilder.ts:659-1382) for the packets this layer owns:
//
//   - Immediate packets (see immediateOpcodes) act at once: hello/ping
//     upkeep, login, handshake, logout, and the login-extension packets
//     (accepted, validated, and dropped — see below).
//   - Any other packet from a logged-in player is delivered to the
//     game layer through the turn-packet bridge (see TurnSink in
//     bridge.go): when AddPlayer returned a sink, the packet goes to
//     the sink at once — the sink enqueues it on the player's turn
//     queue, and Player.Process drains it on the next tick, matching
//     live OSRS "input acts next tick". A sink refusal (player queue
//     at its 240-packet cap) cleans up and closes with 1008, as the
//     TS cap branch does. When there is no sink, the packet queues
//     into turnPackets (cap 240, config.PacketQueueLimit) for the
//     game loop to drain via TurnPackets, with the same overflow
//     behavior. A packet is delivered to exactly one of the two,
//     never both.
//   - Any other packet from a connection that has not entered the world
//     is dropped: TS guards every gameplay case on this.player.
//
// Login-extension packets (character_select/create/delete) and
// login_forget are decoded for shape (a malformed payload closes with
// 1002, as a TS decode throw would) and then dropped: the accounts
// login-extension plugin owns those flows in TS and does not exist in
// Go. Plugin login extension hooks are NOT implemented. plugin_message
// packets are dropped (no plugin event system to emit to, and no reply
// encoder in the protocol contract for the TS "unavailable" reply).
// movement_input packets are dropped (no player:movement-input event
// system yet).
//
// A malformed login/handshake payload closes the connection with 1002,
// matching TS, where the typed decode runs inside decodeClientPackets
// and a throw rejects the whole frame.
func (c *Connection) Dispatch(packets []protocol.Packet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	for _, packet := range packets {
		if c.closed {
			return
		}
		if c.entered && !immediateOpcodes[packet.Opcode] {
			if c.turnSink != nil {
				if !c.turnSink(packet) {
					c.cleanupLocked()
					c.ch.Close(1008, "input limit")
					return
				}
				continue
			}
			if len(c.turnPackets) >= config.PacketQueueLimit {
				c.cleanupLocked()
				c.ch.Close(1008, "input limit")
				return
			}
			c.turnPackets = append(c.turnPackets, packet)
			continue
		}
		c.dispatchImmediateLocked(packet)
	}
}

func (c *Connection) dispatchImmediateLocked(packet protocol.Packet) {
	switch packet.Opcode {
	case protocol.ClientHello:
		// No-op (TS case "hello": continue).
	case protocol.ClientPing:
		// The first ping arms the idle timeout: older clients without
		// keepalive are never timed out (NetworkBuilder.ts:450, 1363).
		c.sendsKeepalive = true
	case protocol.ClientLogin:
		username, password, revision, flags, _, err := protocol.DecodeLogin(packet.Payload)
		if err != nil {
			c.cleanupLocked()
			c.ch.Close(1002, "malformed packet")
			return
		}
		c.handleLoginLocked(username, password, revision, flags)
	case protocol.ClientLoginForget:
		// Shape-validated, then dropped: forgetting a remembered login
		// needs an offline token revoke plus live-player attribute
		// access (forgetLoginToken, NetworkBuilder.ts:149-180), neither
		// of which the session layer can reach. NOT implemented.
		if _, err := decodeStrings(packet.Payload, 2); err != nil {
			c.cleanupLocked()
			c.ch.Close(1002, "malformed packet")
			return
		}
	case protocol.ClientCharacterSelect, protocol.ClientCharacterCreate:
		if _, err := decodeStrings(packet.Payload, 1); err != nil {
			c.cleanupLocked()
			c.ch.Close(1002, "malformed packet")
			return
		}
		// Dropped: owned by the accounts login extension in TS.
	case protocol.ClientCharacterDelete:
		if _, err := decodeStrings(packet.Payload, 2); err != nil {
			c.cleanupLocked()
			c.ch.Close(1002, "malformed packet")
			return
		}
		// Dropped: owned by the accounts login extension in TS.
	case protocol.ClientHandshake:
		_, clientType, capabilities, err := protocol.DecodeHandshake(packet.Payload)
		if err != nil {
			c.cleanupLocked()
			c.ch.Close(1002, "malformed packet")
			return
		}
		// TS ignores the handshake's name; only clientType and
		// capabilities reach enterWorld.
		_ = c.enterWorldLocked(clientType, capabilities)
	case protocol.ClientPluginMessage:
		// Dropped: no plugin system. TS rate-limits (64/s) and emits
		// network:plugin-message, replying "unavailable" when
		// unhandled; neither exists here.
	case protocol.ClientMovementInput:
		// Dropped: no player:movement-input event system yet. TS also
		// ignores it while trading.
	case protocol.ClientLogout:
		// Answered on the next tick (World.answerLogoutRequest), which
		// closes the socket — as in live OSRS. Nothing is closed here.
		if c.entered && c.world != nil && c.playerIndex >= 0 {
			c.world.QueueLogout(c.playerIndex)
		}
	default:
		// Pre-login non-immediate packet: dropped (no player yet).
	}
}

// decodeStrings reads n latin1 NUL-terminated strings from a packet
// payload, validating shape the way the TS typed decoders do.
func decodeStrings(payload []byte, n int) ([]string, error) {
	r := protocol.NewReader(payload)
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		s, err := r.String()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// handleLoginLocked runs one LOGIN packet: release any previous pending
// login's reservations (TS login() starts with releasePendingName),
// authenticate, and answer. A failure sends the login response and
// keeps the socket open for retry (failLogin semantics). A second LOGIN
// packet on an entered connection is processed like the first, matching
// TS, where dispatch calls login() unconditionally.
func (c *Connection) handleLoginLocked(username, password string, revision int32, flags byte) {
	c.releasePendingLocked()
	pending, code, msg := Authenticate(c.deps, username, password, revision, flags, c.ch.RemoteAddr())
	if pending == nil {
		c.failLoginLocked(code, msg)
		return
	}
	c.pending = pending
	c.pendingAt = c.now()
	c.sendLocked(protocol.EncodeLoginResponse(true, -1, "", pending.DisplayName, pending.LoginToken))
}

// failLoginLocked records the failure (for headless/test introspection,
// cf. getLoginFailure) and sends the failure response. The socket stays
// open. Port of failLogin (NetworkBuilder.ts:1933-1936).
func (c *Connection) failLoginLocked(code int32, message string) {
	c.loginFailure = message
	c.sendLocked(protocol.EncodeLoginResponse(false, code, message, "", ""))
}

// sendLocked ports ClientConnection.send (NetworkBuilder.ts:1943-1947):
// once a player exists, packets go through the session queue; before
// that, they go direct to the channel when open.
func (c *Connection) sendLocked(packet []byte) {
	if c.entered && c.session != nil {
		c.session.SendPacket(packet)
		return
	}
	if c.ch.IsOpen() {
		_ = c.ch.Send(packet)
	}
}

// TurnPackets drains the queued turn packets for the game loop,
// returning them in arrival order. The queue is cleared. Draining is
// allowed after disconnect: TS processTurnPackets runs input that
// arrived while the player is held in the world
// (NetworkBuilder.ts:634-637). The coordinator converts these to
// game.TurnPacket values (same opcode+payload shape).
//
// When AddPlayer returned a turn sink, packets bypass this queue
// entirely (Dispatch hands them to the sink), so TurnPackets returns
// nil; the two delivery paths never double-deliver.
func (c *Connection) TurnPackets() []protocol.Packet {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.turnPackets) == 0 {
		return nil
	}
	out := c.turnPackets
	c.turnPackets = nil
	return out
}

// EnterWorld admits the pending login to the world on a handshake
// packet, porting enterWorld (NetworkBuilder.ts:1623-1701) up to
// WorldPort.AddPlayer plus pending-name release, session creation, and
// the bootstrap packet sequence (sendPlayerBootstrap, called at the
// end of enterWorld in TS). It is a no-op (nil error) when there is no
// pending login, the player has already entered, or the connection is
// closed. On rejection it sends the failLogin response and returns a
// *LoginRejection; on world full it additionally closes the channel
// with 1013, as TS does.
//
// The remaining TS enterWorld steps (login-extension entered hook,
// Sailing.onLogin, refreshActiveRegions, player:login event) belong to
// the game/plugin layers and are not performed here.
func (c *Connection) EnterWorld(clientType, capabilities byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enterWorldLocked(clientType, capabilities)
}

func (c *Connection) enterWorldLocked(clientType, capabilities byte) error {
	if c.pending == nil || c.entered {
		return nil
	}
	pending := c.pending
	if c.closed {
		c.releasePendingLocked()
		return nil
	}
	// Re-check bans: a ban landing after login still rejects.
	if c.deps.Punishment != nil {
		if c.deps.Punishment.Banned(pending.Username) || c.deps.Punishment.Banned(pending.CredentialName) {
			c.releasePendingLocked()
			c.failLoginLocked(LoginCodeBanned, msgBanned)
			return &LoginRejection{Code: LoginCodeBanned, Message: msgBanned}
		}
	}
	if c.world == nil {
		c.releasePendingLocked()
		c.failLoginLocked(LoginCodeBusy, msgBusy)
		return &LoginRejection{Code: LoginCodeBusy, Message: msgBusy}
	}
	session := NewSession(c.ch)
	index, sink, err := c.world.AddPlayer(&EnteringPlayer{
		Username:       pending.Username,
		DisplayName:    pending.DisplayName,
		Profile:        pending.Profile,
		Session:        session,
		Save:           pending.Save,
		PasswordHash:   pending.PasswordHash,
		RemoteAddr:     c.ch.RemoteAddr(),
		ClientType:     clientType,
		Capabilities:   capabilities,
		CredentialName: pending.CredentialName,
		LoginTokens:    pending.LoginTokens,
	})
	if err != nil {
		// TS: add() fails only when full.
		c.releasePendingLocked()
		c.failLoginLocked(LoginCodeWorldFull, msgWorldFull)
		c.cleanupLocked()
		c.ch.Close(1013, "world full")
		return &LoginRejection{Code: LoginCodeWorldFull, Message: msgWorldFull}
	}
	c.session = session
	c.playerIndex = index
	c.turnSink = sink
	c.entered = true
	c.bootstrapSave = pending.Save
	c.bootstrapUsername = pending.Username
	c.bootstrapDisplayName = pending.DisplayName
	c.bootstrapClientType = clientType
	c.bootstrapCapabilities = capabilities
	c.releasePendingLocked()
	// TS calls sendPlayerBootstrap at the end of enterWorld; the
	// bootstrap reads the retained state above.
	c.sendBootstrapLocked()
	return nil
}

// Bootstrap sends the post-login packet sequence, porting
// sendPlayerBootstrap (NetworkBuilder.ts:1703-1732) — see
// sendBootstrapLocked in bootstrap.go for the sequence and its
// grounding. It is normally already sent: enterWorldLocked sends it
// at the end of entering the world, exactly where TS calls it, so a
// call after entering is a no-op returning nil. Calling it before
// entering is an error: there is no player index, session, or save
// snapshot to bootstrap from.
func (c *Connection) Bootstrap() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.entered {
		return errors.New("session: Bootstrap called before EnterWorld")
	}
	c.sendBootstrapLocked()
	return nil
}

// CheckIdle reports whether the connection has timed out for idleness
// at now, porting the startIdleCheck predicate
// (NetworkBuilder.ts:1979-1989): only a logged-in connection that has
// sent at least one ping is ever timed out, and only after 30 s without
// any frame. The entered gate reproduces TS starting the interval only
// in enterWorld.
func (c *Connection) CheckIdle(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || !c.entered || !c.sendsKeepalive {
		return false
	}
	return now.Sub(c.lastFrameAt) >= IdleTimeout
}

// CheckPendingLogin reports whether the pending login has waited for
// its handshake for 5 minutes at now, porting the pending-login timeout
// (NetworkBuilder.ts:1958-1971): the timeout starts at login acceptance
// and is cleared on entering the world or on cleanup.
func (c *Connection) CheckPendingLogin(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.entered || c.pending == nil || c.pendingAt.IsZero() {
		return false
	}
	return now.Sub(c.pendingAt) >= PendingLoginTimeout
}

// StartIdleCheck starts the liveness goroutine: every 5 s it enforces
// the idle timeout (post-login, post-ping only) and the pending-login
// timeout, closing with 1001 on either, matching the TS close codes
// ("idle timeout" / "login timeout"). TS runs these as two separate
// timers (a 5 s interval started in enterWorld, and a 5-minute timeout
// started at login acceptance); the Go ticker enforces both with at
// most 5 s of extra granularity on the pending deadline. The goroutine
// exits when stop closes, when either timeout fires, or when the
// connection is cleaned up. Calling it twice is a no-op. The wiring
// (HTTP handler) owns the stop channel.
func (c *Connection) StartIdleCheck(stop <-chan struct{}) {
	c.mu.Lock()
	if c.idleStarted {
		c.mu.Unlock()
		return
	}
	c.idleStarted = true
	c.mu.Unlock()

	go func() {
		ticker := time.NewTicker(IdleCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				if c.CheckIdle(now) {
					c.Close(1001, "idle timeout")
					return
				}
				if c.CheckPendingLogin(now) {
					c.Close(1001, "login timeout")
					return
				}
				if c.IsClosed() {
					return
				}
			}
		}
	}()
}

// Close runs cleanup (once) and terminates the channel with code and
// reason. It is idempotent.
func (c *Connection) Close(code int, reason string) {
	c.cleanup()
	c.ch.Close(code, reason)
}

// Cleanup releases connection state without closing the channel: the
// socket-close/socket-error path, where the transport is already dead.
// It ports cleanup() (NetworkBuilder.ts:1991-2018) for the state this
// layer owns: timers are implicit (StartIdleCheck exits on IsClosed),
// pending names are released, and an entered player is queued for
// removal via WorldPort.RemovePlayer. Trade closing and the disconnect
// save are game/persistence concerns the world handles on removal. It
// is idempotent.
func (c *Connection) Cleanup() {
	c.cleanup()
}

func (c *Connection) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanupLocked()
}

func (c *Connection) cleanupLocked() {
	if c.closed {
		return
	}
	c.closed = true
	c.releasePendingLocked()
	if c.entered && c.world != nil && c.playerIndex >= 0 {
		c.world.RemovePlayer(c.playerIndex)
	}
}

// releasePendingLocked releases the name reservations held for the
// pending login and clears it, porting releasePendingName
// (NetworkBuilder.ts:1949-1953). The reserved keys are recomputed from
// the pending login: pendingKey(CredentialName) is the account key
// reserved in Authenticate step 5 (the account key is the
// DisplayNames.key of the primary, and pendingKey re-applies that key
// function, so both forms coincide), and pendingKey(Username) is the
// character key reserved when the pending login was built.
func (c *Connection) releasePendingLocked() {
	if c.pending == nil {
		return
	}
	if c.deps.ReleaseName != nil {
		accountKey := pendingKey(c.pending.CredentialName)
		characterKey := pendingKey(c.pending.Username)
		c.deps.ReleaseName(accountKey)
		if characterKey != accountKey {
			c.deps.ReleaseName(characterKey)
		}
	}
	c.pending = nil
	c.pendingAt = time.Time{}
}

// IsClosed reports whether cleanup has run.
func (c *Connection) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// Entered reports whether the connection has entered the world.
func (c *Connection) Entered() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.entered
}

// PlayerIndex returns the world index assigned by AddPlayer, or -1.
func (c *Connection) PlayerIndex() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.playerIndex
}

// Session returns the player session created by EnterWorld, or nil.
func (c *Connection) Session() *Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session
}

// LoginFailure returns the last login failure message, if any (cf.
// getLoginFailure; used by headless logins to report why login did not
// complete).
func (c *Connection) LoginFailure() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loginFailure
}

// RemoteAddr returns the channel's remote address.
func (c *Connection) RemoteAddr() string {
	return c.ch.RemoteAddr()
}
