package game

import "sync"

// SessionPort is the outbound interface the session package implements.
// Game code talks ONLY to this interface; it never imports the session
// package (which would create an import cycle: the session package holds a
// *Player and implements SessionPort for it).
//
// The methods mirror the PlayerSession surface World and Player use in TS:
//   - SendPacket queues an encoded outbound packet (PlayerSession
//     sendClientPacket/queueing; see ANALYSIS §4.4).
//   - FlushTick runs the end-of-tick flush for the given process cycle
//     (PlayerSession.flush(processCycle), World.ts flush_network_players
//     phase). An error requests logout in TS; World applies that here.
//   - Close terminates the connection with a close code and reason.
//   - IsOpen reports whether the underlying connection is still usable
//     (World.isPlayerSessionConnected / shouldRunNetworkUpdates gate).
//   - SendRunEnergy queues the run-energy orb packet (PacketSender
//     sendRunEnergy/sendRunStatus, PacketSender.ts:374-410, both of
//     which send encodeRunEnergy). The game layer cannot encode packets
//     itself (it must never import the protocol package), so the typed
//     send lives on this interface and the session package encodes.
//     Added in phase 7 for run-energy drain/restore, auto-stop, and
//     the run toggle.
//   - SendDestination queues the destination-flag packet
//     (PacketSender.sendDestination, PacketSender.ts:1534-1537).
//     Added in phase 7 for syncDestinationFlagToRoute.
//   - SendVarp queues a varp packet (PacketSender.sendConfig,
//     PacketSender.ts:355-359, which sends encodeVarp). Added in
//     phase 10 for plugin content: the first content piece sends
//     the date_minutes varp through it, and the game layer must
//     never encode packets itself, so the typed send lives here
//     and the session package encodes, as with SendRunEnergy.
type SessionPort interface {
	SendPacket(packet []byte)
	SendRunEnergy(units int, running bool, weightKg int, staminaTicks int)
	SendDestination(x, y int)
	SendVarp(id, value int)
	FlushTick(cycle int64) error
	Close(code int, reason string)
	IsOpen() bool
}

// TurnPacket is one queued inbound packet awaiting the owning player's
// next turn. It is deliberately opaque (opcode + raw payload) so this
// package does not import the protocol package; decoding and dispatch
// belong to the session/coordinator layer via TurnHandler.
//
// In TS, turn packets are queued by the network layer and drained by
// PlayerSession.processPackets() inside Player.process()
// (Player.ts:489, client_input phase).
type TurnPacket struct {
	Opcode  byte
	Payload []byte
}

// TurnHandler processes one drained turn packet for a player on the given
// cycle. It is set by the coordinator/session layer (Player.TurnHandler);
// with no handler installed, drained packets are discarded.
type TurnHandler func(p *Player, pkt TurnPacket, cycle int64)

// Player is the foundation core of game/entity/impl/player/Player.ts
// (1,731 lines in TS; only the fields listed here are ported).
//
// Ported core fields:
//   - Mobile core (index, location, flags, facing, timers).
//   - Username / DisplayName (Player.ts:145-146). DisplayName falls back
//     to Username when empty (Player.ts:957).
//   - Profile: the character profile key (Player.ts:153, MAIN_PROFILE by
//     default in TS; here it is a plain string — the empty string means
//     "unset", and the coordinator assigns the world profile at login).
//   - Session: the outbound SessionPort (nil until the session layer
//     attaches one; a nil session means "no network updates").
//   - RemoteAddr: the peer address for logging (Player.getHostAddress,
//     Player.ts:997).
//   - LastInputCycle: the last process cycle on which client input was
//     received. In TS the network layer sets it on socket receipt
//     (NetworkBuilder.ts:680); the session layer may set this field
//     directly, and Player.Process also refreshes it when it drains at
//     least one turn packet (foundation bookkeeping — see Process).
//   - IsBot: whether this player is a player bot (Player.ts:161,
//     playerBot). Bots have no network client in TS and are excluded from
//     network/visibility work by World.shouldRunNetworkUpdates; the
//     foundation World gates flushing on Session.IsOpen instead, and
//     IsBot is recorded for the milestones that need bot strides.
//   - LoggedIn: whether the player has completed entering the world.
//     The coordinator sets it; nothing in this package reads it yet
//     beyond recording state honestly.
//   - DisconnectSource / LogoutHoldCycle: logout bookkeeping for
//     ReadyToLeaveWorld (Player.ts:135, 137, 700-709).
//   - View distance: the adaptive player view radius, the optional
//     fixed (spectator) override, and the NPC view radius
//     (Player.ts:1051-1107). See ViewDistance/SetFixedViewDistance/
//     NpcViewDistance/SetNpcViewDistance/ResizeViewDistance.
//   - LocalPlayers / LocalNPCs: the per-tick sync view lists
//     (Player.ts:112-113, 1129-1135). They are tick-goroutine-only:
//     World rebuilds them in the update_players_npcs phase and the
//     flush phase reads them in the same tick. See LocalPlayers.
//   - MovementQueue: the walk queue (MovementQueue.ts core subset;
//     see movement_queue.go), constructed by NewPlayer and wired to
//     the world's collision by the coordinator. It steps the player
//     from Player.Process; see MovementQueue().
//   - Running: the run setting (Player.ts:160, isRunning = true).
//     See IsRunning/SetRunning. Live run-energy units live on the
//     player too (see runenergy.go); the Agility level and carried
//     weight the drain reads come live from the SkillManager and
//     the containers.
//   - Inventory/Equipment/Banks/Skills/Appearance: the phase 9
//     live systems (Player.ts:115, 227; getInventory/getEquipment/
//     getBank/getSkillManager/getAppearance). Constructed by
//     NewPlayer; see container.go, skills.go, appearance.go.
//     The coordinator injects item definitions through
//     SetItemDefinitions and installs OnEquipmentChanged; with
//     neither installed the systems hold state but send nothing.
//   - Relations: friend/ignore lists, chat modes, and presence
//     gates (PlayerRelations.ts; see relations.go). Constructed
//     by NewPlayer; see Relations(). The long username it keys
//     off is the LongUsername field below, set by the login path.
//   - LongUsername: the base-37 name hash (Player.ts:151,
//     976-978). Set by the login path at enterWorld; 0 until
//     then (the TS field default). See GetLongUsername.
//
// NOT ported (absent on purpose; do not read gameplay meaning into zero
// values): combat, prayer,
// trading, duelling, PacketSender, skull
// timers, rights, and hitpoints. The movement queue IS
// ported (see above); the freeze/stun timer gates ARE ported
// (see movement_queue.go), while the other systems that gate
// or consume movement (combat, following, wilderness) are not —
// see movement_queue.go for the per-site degenerations. Run-energy
// drain/restore ARE ported (runenergy.go, the queue's
// drainRunEnergy, the restore branch in Process), with identity
// equipment modifiers and no stamina system.
type Player struct {
	Mobile

	Username    string
	DisplayName string
	Profile     string

	// LongUsername is the player's base-37 name hash
	// (Player.ts:151, public longUsername = 0n). The login path
	// sets it at enterWorld (NetworkBuilder.ts:1662,
	// setLongUsername with Misc.stringToLongBigInt); until then
	// it reads 0. Relations key friend/ignore membership off it
	// (see relations.go). Read through GetLongUsername.
	LongUsername int64

	// Session is the outbound network port. Nil means no client is
	// attached; World skips flushing such players.
	Session SessionPort

	// RemoteAddr is the peer address (host) for logging.
	RemoteAddr string

	// chatIcons is the lazily allocated chat-icon (crown) store:
	// the ids rendered before the player's name in chat
	// (Player.ts:237: private chatIcons = []). No core code sets
	// it — only content does, through SetChatIcons (see
	// internal/game/chat.go). The pointer form mirrors the
	// attribute store (attributes.go): the mutex inside is never
	// copied, and login-goroutine content writes cannot race
	// tick-goroutine listener reads. Read through ChatIcons,
	// which returns a copy.
	chatIcons *chatIconStore

	// relations is the player's friend/ignore lists, chat modes,
	// and presence gates (Player.ts:117, relations = new
	// PlayerRelations(this)). Constructed by NewPlayer; a
	// zero-value Player gets one lazily through Relations(),
	// which serializes the allocation (see relations.go). Read
	// through Relations(), never directly.
	relations *PlayerRelations

	// sailing is the player's boat state (Player.ts:185,
	// getSailing; see sailing_state.go). A value, not a
	// pointer: the zero value is a usable empty state, and
	// GetSailing hands out the live field so Sailing
	// transitions mutate it in place, as TS mutates the
	// object getSailing returns. Read and written through
	// GetSailing/SetSailing. Owned save state: entry loads
	// it through NormalizeSailingState and snapshots write
	// it normalized (see persist OwnedState.Sailing).
	// Tick-goroutine-only after entry, like the rest of
	// Player; SetSailing runs once at entry, before the
	// player enters the world.
	sailing SailingState

	// skullTimer / skullType / skullIconOverride (+Always)
	// are the live skull state (Player.ts:216-223: skullType,
	// skullIconOverride, skullIconOverrideAlways, skullTimer).
	// The timer is in ticks; the type is WHITE (0) or RED (1);
	// the override is nil when unset and shows even without a
	// skull timer only when Always is true. Zero values are
	// the TS constructor state (setSkullTimer(0),
	// WHITE_SKULL, null override — Player.ts:278-279), so
	// NewPlayer needs no initialization. Owned save state:
	// entry loads timer/type from the save and snapshots
	// write the live values (see skull.go). Read and written
	// through SkullTimer/SetSkullTimer, SkullType/SetSkullType,
	// SetSkullIconOverride, HeadSkullIcon (see skull.go).
	// Tick-goroutine-only after entry, like the rest of Player.
	skullTimer              int
	skullType               SkullType
	skullIconOverride       *int
	skullIconOverrideAlways bool

	// spellbook is the player's current magic spellbook
	// (Player.ts:201, MagicSpellbook.NORMAL field default).
	// Session-only state: the save holds the book's interface
	// id (persist SpellBook), entry restores it through
	// applyEntrySpellbook (cmd/server), and snapshots keep the
	// interface id. The zero value is SpellbookNormal, so a
	// fresh Player reads the TS field default. Read through
	// Spellbook(), written through SetSpellbook/ChangeSpellbook
	// (see spellbook.go).
	spellbook Spellbook

	// LastInputCycle is the last process cycle with client input.
	LastInputCycle int64

	// IsBot marks a player bot (no real client unless a session with a
	// network client is attached by an administrator tool).
	IsBot bool

	// LoggedIn records completed entry into the world.
	LoggedIn bool

	// TurnHandler dispatches drained turn packets. Set by the
	// coordinator/session layer; nil discards drained packets.
	TurnHandler TurnHandler

	// OnMovementRequest, when set, receives decoded walk/click
	// requests (MovementRequest) from the coordinator's TurnHandler,
	// on the tick the request is drained — the Go form of walk()
	// (NetworkBuilder.ts:1739-1752). The wired server installs a hook
	// that routes the request through the player's MovementQueue
	// (destination z = the player's current level, run choice from
	// the WALK force-run bit or ClickRunOverride); with no hook
	// installed (a bare NewPlayer in tests), requests drop and no
	// position changes.
	OnMovementRequest func(*Player, MovementRequest, int64)

	// InCombatHold, when set, reports whether the player was hit in
	// combat too recently to leave the world (TS inLogoutCombat,
	// Player.ts:678-680, backed by combat state that is NOT STARTED).
	// Nil means "never in combat hold", which makes ReadyToLeaveWorld
	// return true immediately — the correct foundation behavior, since
	// without combat no player can be in combat.
	InCombatHold func(cycle int64) bool

	// OnRequestLogout, when set, is called by RequestLogout. World sets
	// it when the player is added (if nil) so a logout request enqueues
	// the player for removal; the coordinator may set its own hook to
	// also notify the session layer.
	OnRequestLogout func(*Player)

	disconnectSource string
	logoutHoldCycle  int64

	// forceMovement is the player's active forced glide, or nil
	// (Player.ts:187, forceMovement). Installed by
	// ForceMovementTask; read through ForceMovement(), written
	// through SetForceMovement (which flags Flag.FORCED_MOVEMENT
	// when installing). Tick-goroutine-only. See forcemovement.go.
	forceMovement *ForceMovement

	// clickDelayCycle is the world process cycle of the most recent
	// click-delay reset: the Go form of the getClickDelay
	// TickStopwatch (Player.ts:123, clickDelay = new
	// TickStopwatch()). -1 means "never" (NewPlayer initialises
	// it; a zero-value Player reads 0, which only matters on cycle
	// 0). The elapsed check is ClickDelayElapsed; the stamp is
	// ResetClickDelay. Tick-goroutine-only. See forcemovement.go.
	clickDelayCycle int64

	// lastItemPickupCycle is the world process cycle of the most
	// recent successful ground-item pickup: the Go form of the
	// getLastItemPickup TickStopwatch
	// (PickupItemPacketListener.ts:18, ItemOnGroundManager.ts:
	// 245). TS blocks a pickup when the stopwatch reads under
	// 300ms; ticks are 600ms and transfers happen only on ticks,
	// so "under 300ms" is exactly "the same cycle", and the
	// cycle stamp is exactly equivalent — no wall clock is
	// needed. -1 means "never" (NewPlayer initialises it; a
	// zero-value Player reads 0, which only matters on cycle 0
	// and only for players the listeners refuse as unready
	// anyway). Tick-goroutine-only. See ItemPickupThrottled and
	// RecordItemPickup.
	lastItemPickupCycle int64

	// viewDistance is the adaptive player view radius, in tiles. It
	// shrinks in crowds and regrows when quiet; see
	// ResizeViewDistance. Player.ts:1051 (viewDistance).
	viewDistance int
	// viewDistanceQuietCycles counts consecutive unsaturated cycles
	// since the last shrink. Player.ts:1052.
	viewDistanceQuietCycles int
	// fixedViewDistance, when non-nil, pins the effective view
	// distance regardless of crowding (a spectator watching a room);
	// nil resumes adaptive resizing. A *int carries the TS
	// `number | null` (Player.ts:1070, fixedViewDistance): nil is
	// null. The pointed-to value is always clamped to
	// 1..MaxFixedViewDistance by SetFixedViewDistance.
	fixedViewDistance *int
	// npcViewDistance is how far away NPCs are seen, in tiles.
	// Player.ts:1078 (npcViewDistance).
	npcViewDistance int

	// localPlayers and localNPCs are the per-tick sync view lists
	// (Player.ts:112-113). They are tick-goroutine-only: World
	// mutates them in the update_players_npcs phase and the flush
	// phase reads them in the same tick; no other goroutine may read
	// or write them. See LocalPlayers/LocalNPCs for the aliasing
	// rules callers must follow.
	localPlayers []*Player
	localNPCs    []*NPC

	// movementQueue is the player's walk queue. Tick-goroutine-only
	// (see movement_queue.go). NewPlayer always installs one; a
	// zero-value Player has none, and Process guards for that.
	movementQueue *MovementQueue

	// positionToFace is the tile this player turns toward
	// (Mobile.ts:88, positionToFace), or nil for none. See
	// player_facing.go: the player form of NPC.SetPositionToFace.
	// Tick-goroutine-only, like Mobile.Facing.
	positionToFace *Location

	// arrivalAnimation is an arrival-step animation held for
	// one cycle (Player.ts:440, 446-463): an animation with
	// standing priority 1 that lands on the tick the player
	// stepped is held a tick, because the client drops a
	// priority-1 seq that lands in the same update as a step.
	// Process releases it after combat post-movement unless
	// the player stepped again. Tick-goroutine-only.
	arrivalAnimation *Animation

	// areaManager is the world's AreaManager, installed by World
	// when the player is added (see drainAddPlayers). Nil for a
	// standalone Player: Process skips area processing, and the
	// area reference stays nil. Tick-goroutine-only.
	areaManager *AreaManager

	// equipmentEffects is the world's run-energy modifier
	// registry, installed by World when the player is added
	// (see drainAddPlayers), alongside the AreaManager. Nil for
	// a standalone Player: the drain/restore formulas apply
	// identity. Tick-goroutine-only after installation.
	equipmentEffects *EquipmentEffects

	// multiIcon is the multi-combat icon value last sent to the
	// client (0 single, 1 multi). Player.ts:159 (multiIcon,
	// declared without an initializer). NewPlayer sets -1, the
	// unset sentinel: TS compares `undefined != 0` on the first
	// tick and always sends, so the Go sentinel must sit
	// outside {0, 1} for the first AreaManager.Process to send
	// as well. See MultiIcon/SetMultiIcon. Tick-goroutine-only.
	multiIcon int

	// botAreaProcessTick is the bot area-downsampling counter
	// (Player.ts:547-554, botAreaProcessTick): bots process
	// areas every third tick while idle. See Process.
	botAreaProcessTick int

	// running is the run setting: whether the player runs when no
	// run override applies. Player.ts:160 (isRunning = true).
	running bool

	// runEnergyUnits is the live run-energy state in OSRS units
	// (0..10,000). Player.ts:185 (runEnergy = RUN_ENERGY_MAX_UNITS).
	// See GetRunEnergy/SetRunEnergyUnits in runenergy.go.
	// Tick-goroutine-only, like the rest of Player. The Agility
	// level the drain reads comes live from the SkillManager
	// (GetCurrentAgilityLevel) and the carried weight live from
	// the containers (GetWeightKg); neither is stored here.
	runEnergyUnits int
	// audioSettings is the entry-time audio-settings snapshot
	// (music mode, loop, area type, volumes), set once at entry
	// from the save (PlayerSave.ts:650) and static for the session
	// (live varp_transmit application is NOT STARTED). See
	// GetAudioSettings/SetAudioSettings in audio.go.
	audioSettings audioSettings

	// inventory, equipment, banks, skills, and appearance are the
	// phase 9 live systems. Banks is 11 slots (Bank.TOTAL_BANK_TABS;
	// the last is the search tab) with lazy per-tab creation through
	// Bank(), mirroring Player.getBank (Player.ts:1480-1488). All
	// are tick-goroutine-only, like the rest of Player.
	inventory  *Inventory
	equipment  *Equipment
	banks      []*Bank
	skills     *SkillManager
	appearance *Appearance

	// skill is the player's current skill action (Player.ts:204,
	// `private skill: any`), or nil when none is running. Read
	// and written through GetSkill/SetSkill (see skills.go);
	// SkillManager.StartSkillable/StopSkillable own its
	// lifecycle. Session-only: no skill state persists.
	// Tick-goroutine-only, like the rest of Player.
	skill Skillable

	// creationMenu is the player's open creation menu
	// (Player.ts:205, `private creationMenu: CreationMenu`), or
	// nil when none is open. Read and written through
	// GetCreationMenu/SetCreationMenu (see skills.go);
	// PacketSender.sendCreationMenu installs it, and
	// closeCreationMenu / stopSkillable / resetInterfaceState
	// clear it. Session-only: no menu state persists.
	// Tick-goroutine-only, like the rest of Player.
	creationMenu *CreationMenu

	// dialogueManager is the player's dialogue chain state
	// (DialogueManager, Player.ts:143, 1642-1644). Constructed by
	// NewPlayer; see GetDialogueManager. Session-only: no dialogue
	// state persists (see dialogue_manager.go).
	dialogueManager *DialogueManager

	// interfaceId is the open main interface id, -1 when none is
	// open (Player.ts:156, interfaceId = -1). Read and written
	// through GetInterfaceId/SetInterfaceId (see
	// interface_state.go); SetInterfaceId emits the
	// interface:closed / interface:changed custom events.
	// interfaceMu guards it together with the entered-action
	// holders: SetInterfaceId runs from session sender calls,
	// which the login goroutine can reach through login hooks,
	// while the tick goroutine reads it in close gates.
	interfaceId int

	// enteredAmountAction / enteredSyntaxAction are the actions
	// waiting on an entered-amount / entered-syntax prompt
	// answer (Player.ts:207-208). Nil means no prompt is armed.
	// See enteredaction.go. Guarded by interfaceMu, as above:
	// content arms them from hooks and the turn-packet dispatch
	// (tick goroutine) consumes them.
	enteredAmountAction *EnteredAmountAction
	enteredSyntaxAction *EnteredSyntaxAction

	// interfaceMu guards interfaceId and the entered-action
	// holders; see above. It is never held across a custom-event
	// emit or an action callback: both can call back into this
	// player.
	interfaceMu sync.Mutex

	// EmitCustomEvent is the custom-event emit sink, installed
	// once at entry by the coordinator wiring (to
	// Hub.EmitCustomEvent) and never reassigned afterwards. Nil
	// means no subscribers: interface-id changes emit nothing.
	// It follows the TranscriptDeps.EmitCustomEvent seam
	// (transcript.go): game code never imports the plugins
	// package. Set once before the player is queued, so no
	// lock guards the field itself.
	EmitCustomEvent func(name string, payload any)

	// itemDefs supplies item-definition lookups (stackable, noted,
	// weight, equipment slot, requirements, tradeable/dropable) to
	// the container systems. It is nil until the coordinator
	// injects it; with no source the systems use the documented
	// no-definition defaults (see itemdef.go). Set once at entry;
	// tick-goroutine-only thereafter.
	itemDefs ItemDefinitionSource

	// OnEquipmentChanged, when set, is invoked by
	// onEquipmentChanged after an equipment refresh flags
	// APPEARANCE (Player.onEquipmentChanged, Player.ts:662). The
	// coordinator installs it to rebuild the sync appearance block
	// from live state; with no hook installed the flag is still
	// set and nothing else happens.
	OnEquipmentChanged func(*Player)

	turnMu      sync.Mutex
	turnPackets []TurnPacket
}

// NewPlayer returns a Player at loc with logout bookkeeping initialized
// (logoutHoldCycle = -1, "unset", per Player.ts:137), the view
// distances at their TS field defaults (Player.ts:1051, 1078-1079),
// the run setting on (Player.ts:160), run energy at max
// (Player.ts:185), live
// inventory/equipment/skills/appearance systems, an 11-slot bank
// array (tabs created lazily), and a
// MovementQueue owned by the player (nil collision/clipping — open
// ground — until the coordinator wires the world's RegionManager).
// Players must be built with NewPlayer: a zero-value Player has view
// distances of 0, no movement queue, and 0 run energy, which TS field
// initializers never produce.
func NewPlayer(username string, loc Location) *Player {
	p := &Player{
		Mobile:              NewMobile(loc),
		Username:            username,
		logoutHoldCycle:     -1,
		lastItemPickupCycle: -1,
		clickDelayCycle:     -1,
		viewDistance:        PreferredViewDistance,
		npcViewDistance:     NPCViewDistance,
		running:             true,
		runEnergyUnits:      RunEnergyMaxUnits,
		multiIcon:           -1,
		interfaceId:         -1,
		audioSettings:       newAudioSettings(),
		banks:               make([]*Bank, 11),
		sailing:             EmptySailingState(),
	}
	p.movementQueue = NewMovementQueue(p, nil, nil)
	p.inventory = NewInventory(p)
	p.equipment = NewEquipment(p)
	p.skills = NewSkillManager(p)
	p.appearance = NewAppearance(p)
	p.dialogueManager = NewDialogueManager(p)
	p.relations = NewPlayerRelations(p)
	return p
}

// GetSailing returns the player's live boat state: the Go
// form of Player.getSailing (Player.ts:185). The pointer
// addresses the live field, so Sailing transitions mutate the
// player's state in place, as TS mutates the object
// getSailing returns. A nil player yields nil; a zero-value
// Player yields an empty state with non-nil boats/tools
// slices, as normalizeSailingState produces. Tick-goroutine-
// only after entry.
func (p *Player) GetSailing() *SailingState {
	if p == nil {
		return nil
	}
	if p.sailing.Boats == nil {
		p.sailing.Boats = []OwnedBoat{}
	}
	if p.sailing.Tools == nil {
		p.sailing.Tools = []int{}
	}
	return &p.sailing
}

// SetSailing replaces the player's boat state wholesale: the
// entry path installing the carried save's normalized state.
// A nil player ignores the call. Runs once at entry, before
// the player enters the world.
func (p *Player) SetSailing(state SailingState) {
	if p == nil {
		return
	}
	p.sailing = state
}

// ItemPickupThrottled reports whether a ground-item pickup on
// cycle is blocked by the 300ms throttle: the Go form of
// getLastItemPickup().elapsedTime(300)
// (PickupItemPacketListener.ts:18). The stopwatch is a cycle
// stamp (see lastItemPickupCycle): a pickup is throttled exactly
// when a successful pickup already stamped this cycle. A nil
// player is never throttled.
func (p *Player) ItemPickupThrottled(cycle int64) bool {
	if p == nil {
		return false
	}
	return p.lastItemPickupCycle == cycle
}

// RecordItemPickup stamps cycle as the cycle of a successful
// ground-item pickup: the Go form of the getLastItemPickup reset
// (ItemOnGroundManager.ts:245). The caller passes the cycle the
// pickup was requested on, which equals the transfer cycle on
// the immediate path — the only path the throttle binds, since
// the throttle gates the listener entry, not the arrival
// callback. A nil player records nothing.
func (p *Player) RecordItemPickup(cycle int64) {
	if p == nil {
		return
	}
	p.lastItemPickupCycle = cycle
}

// Inventory returns the player's live inventory container. It is never
// nil for a player built with NewPlayer. Tick-goroutine-only.
func (p *Player) Inventory() *Inventory { return p.inventory }

// Equipment returns the player's live equipment container. It is never
// nil for a player built with NewPlayer. Tick-goroutine-only.
func (p *Player) Equipment() *Equipment { return p.equipment }

// Bank returns the player's bank container for tab, creating it on
// first use (Player.getBank, Player.ts:1480-1488). Tabs are 0..10;
// the last tab is the search tab (Bank.TOTAL_BANK_TABS = 11). An
// out-of-range tab returns nil. Tick-goroutine-only.
func (p *Player) Bank(tab int) *Bank {
	if tab < 0 || tab >= len(p.banks) {
		return nil
	}
	if p.banks[tab] == nil {
		p.banks[tab] = NewBank(p)
	}
	return p.banks[tab]
}

// BankCount returns the number of bank tabs (11). It exists so save
// and test code need not restate the tab count.
func (p *Player) BankCount() int { return len(p.banks) }

// SetBank installs a bank container for tab, replacing any existing
// one (Player.setBank, Player.ts:1490-1495). Out-of-range tabs are
// ignored. Entry loading uses it; gameplay uses Bank().
func (p *Player) SetBank(tab int, bank *Bank) {
	if tab < 0 || tab >= len(p.banks) {
		return
	}
	p.banks[tab] = bank
}

// Skills returns the player's live skill manager. It is never nil for
// a player built with NewPlayer. Tick-goroutine-only.
func (p *Player) Skills() *SkillManager { return p.skills }

// Appearance returns the player's live appearance. It is never nil
// for a player built with NewPlayer. Tick-goroutine-only.
func (p *Player) Appearance() *Appearance { return p.appearance }

// GetDialogueManager returns the player's dialogue manager. It is
// never nil for a player built with NewPlayer (Player.ts:143, 1642-1644:
// getDialogueManager). Tick-goroutine-only, like the rest of Player.
// A zero-value Player has none, as with the other per-player managers.
func (p *Player) GetDialogueManager() *DialogueManager { return p.dialogueManager }

// ItemDefinitions returns the injected item-definition source, or nil
// when the coordinator has not installed one.
func (p *Player) ItemDefinitions() ItemDefinitionSource { return p.itemDefs }

// SetItemDefinitions installs the item-definition source the container
// systems read through. The coordinator calls it once at entry.
func (p *Player) SetItemDefinitions(src ItemDefinitionSource) { p.itemDefs = src }

// ItemName returns the display name of id through the installed
// item-definition source's optional ItemNameSource capability
// (ItemDefinition.getName, ItemDefinition.ts:177-179). It is the
// Go form of the name lookup content reads in TS through
// coreApi.ItemDefinition.forId(id)?.getName?.() (GracefulOutfit
// .plugin.js:27-33, itemName): "" when id <= 0, when no source
// is installed, when the source lacks the capability, or when
// the source has no name for the id (the TS `?? null` reads as
// "" here, and no prefix matches ""). A nil player reads "".
// The deviation from TS is the route only: TS content reads names
// via the Core ItemDefinition surface; Go content reads them via
// the player's installed definition source (the same Store); a
// Core ItemDefinition surface stays table-only.
func (p *Player) ItemName(id int) string {
	if p == nil || id <= 0 {
		return ""
	}
	return itemName(p.itemDefs, id)
}

// onEquipmentChanged runs the equipment-changed bookkeeping:
// Flag.APPEARANCE is set (Player.onEquipmentChanged,
// Player.ts:662) and the coordinator-installed OnEquipmentChanged
// hook, if any, is invoked so the sync appearance block can be
// rebuilt from live state. Equipment refresh calls it.
func (p *Player) onEquipmentChanged() {
	p.Flags = p.Flags.Set(FlagAppearance)
	if p.OnEquipmentChanged != nil {
		p.OnEquipmentChanged(p)
	}
}

// MovementQueue returns the player's walk queue. It is never nil for
// a player built with NewPlayer. Tick-goroutine-only: see
// movement_queue.go.
func (p *Player) MovementQueue() *MovementQueue { return p.movementQueue }

// PerformAnimation shadows Mobile.PerformAnimation with the
// Player override (Player.ts:446-463): OSRS p_arrivedelay,
// applied globally — the client drops a priority-1 seq
// (skilling loops) that lands in the same update as a step,
// so one started on the tick the player stepped is held a
// tick (see arrivalAnimation, released by Process). A
// further step drops it, as the client would. Otherwise
// the animation plays through Mobile.PerformAnimation,
// which measures an accepted animation as a swing stall
// (see measureAnimation): a seq turned away by priority
// was never started, so it must not be measured.
func (p *Player) PerformAnimation(id, delay int) {
	anim := &Animation{ID: id, Delay: delay, Priority: AnimationPriorityLow}
	if p.movementQueue != nil && p.movementQueue.SteppedThisWorldCycle(combatCycle()) &&
		sequencePriority(id) == 1 {
		p.arrivalAnimation = anim
		return
	}
	p.arrivalAnimation = nil
	p.Mobile.PerformAnimation(id, delay)
}

// releaseArrivalAnimation releases last cycle's held
// arrival animation, unless this cycle stepped again
// (Player.ts:525-528, in process after combat
// post-movement). It plays through the Mobile state
// machine directly — TypeScript calls
// super.performAnimation here, which never measures a
// stall — not through the Player override, which would
// re-hold it.
func (p *Player) releaseArrivalAnimation() {
	held := p.arrivalAnimation
	p.arrivalAnimation = nil
	if held == nil {
		return
	}
	if p.movementQueue != nil && p.movementQueue.DidMoveThisCycle() {
		return
	}
	p.Mobile.performAnimationValue(held)
}

// Size returns the player's footprint: 1 tile. Player.ts:93-95
// (getSize). It satisfies the MovementOwner interface.
func (p *Player) Size() int { return 1 }

// IsPlayer reports that this owner is a player. It satisfies the
// MovementOwner interface (Mobile.isPlayer, Mobile.ts:967-970).
func (p *Player) IsPlayer() bool { return true }

// IsNPC reports false: a Player is never an NPC. It completes the
// AreaActor predicate pair (Area.ts:28, 41, 53-56 tests both).
func (p *Player) IsNPC() bool { return false }

// AsPlayer returns p: the typed identity Area stores in its player
// member map (getAsPlayer).
func (p *Player) AsPlayer() *Player { return p }

// AsNPC returns nil: a Player has no NPC identity (getAsNpc on a
// player is never a member of the NPC map).
func (p *Player) AsNPC() *NPC { return nil }

// MultiIcon returns the multi-combat icon value last sent (0
// single, 1 multi), or -1 before the first send. Player.ts:1182
// (getMultiIcon). See MultiIconActor in areamanager.go.
func (p *Player) MultiIcon() int { return p.multiIcon }

// SetMultiIcon records icon and sends it to the client when it
// changed. Player.ts:1186-1191 (setMultiIcon): the equality check
// lives in the method itself, so callers (AreaManager.Process)
// compare too and this method still change-detects. The send goes
// through the optional MultiIconSender extension asserted on the
// attached Session (the ChatMessageSender precedent): a
// session-less player, or a SessionPort without the extension,
// records the value and sends nothing.
func (p *Player) SetMultiIcon(icon int) {
	if p.multiIcon == icon {
		return
	}
	p.multiIcon = icon
	if sender, ok := p.Session.(MultiIconSender); ok {
		sender.SendMultiIcon(icon)
	}
}

// IsPlayerBot reports whether this player is a bot (the IsBot field).
// It satisfies the MovementOwner interface; TS keys the queue's bot
// branches off isPlayerBot/isUnwatchedBot.
func (p *Player) IsPlayerBot() bool { return p.IsBot }

// IsRunning reports the run setting: whether the player runs when no
// run override applies. Player.ts:1156-1158 (isRunningReturn). The
// run-toggle packet listener is NOT STARTED, so the setting can
// only change via SetRunning for now.
func (p *Player) IsRunning() bool { return p.running }

// SetRunning sets the run setting. Player.ts:1160-1163 (setRunning).
// Unlike TS, it sends no run-status packet: no PacketSender exists.
func (p *Player) SetRunning(running bool) { p.running = running }

// SendRunEnergy forwards the run-energy orb packet to the player's
// session, if one is attached. It satisfies the MovementOwner
// interface (the queue's drain/restore/auto-stop sends). A nil
// session drops the packet: session-less/test players have no
// client to inform.
func (p *Player) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
	if p.Session != nil {
		p.Session.SendRunEnergy(units, running, weightKg, staminaTicks)
	}
}

// SendDestination forwards the destination-flag packet to the
// player's session, if one is attached. It satisfies the
// MovementOwner interface (syncDestinationFlagToRoute). A nil
// session drops the packet.
func (p *Player) SendDestination(x, y int) {
	if p.Session != nil {
		p.Session.SendDestination(x, y)
	}
}

// SendVarp forwards a varp packet to the player's session, if one
// is attached (PacketSender.sendConfig, PacketSender.ts:355-359).
// Plugin content sends varps through this; the session records
// the value in its varp store (phase 18) as sendConfig does, so
// a later getVarp read sees it. A nil session drops the packet.
func (p *Player) SendVarp(id, value int) {
	if p.Session != nil {
		p.Session.SendVarp(id, value)
	}
}

// GetDisplayName returns the name other players see: the display name when
// set, otherwise the username. Player.ts:955-958.
func (p *Player) GetDisplayName() string {
	if p.DisplayName != "" {
		return p.DisplayName
	}
	return p.Username
}

// ViewDistance returns the effective player view radius, in tiles: the
// fixed distance when one is set, otherwise the adaptive distance.
// Player.ts:1054-1056 (getViewDistance: fixedViewDistance ?? viewDistance).
func (p *Player) ViewDistance() int {
	if p.fixedViewDistance != nil {
		return *p.fixedViewDistance
	}
	return p.viewDistance
}

// SetFixedViewDistance fixes how far other players are seen (up to 64),
// as for a spectator; a nil distance clears the override and adaptive
// resizing resumes. Non-nil distances are clamped to
// 1..MaxFixedViewDistance. Player.ts:1073-1075 (setFixedViewDistance).
// TS truncates fractional distances with Math.trunc; Go *int values are
// already integral, so there is no truncation step.
func (p *Player) SetFixedViewDistance(distance *int) {
	if distance == nil {
		p.fixedViewDistance = nil
		return
	}
	clamped := min(max(*distance, 1), MaxFixedViewDistance)
	p.fixedViewDistance = &clamped
}

// NpcViewDistance returns how far away NPCs are seen, in tiles.
// Player.ts:1083-1085 (getNpcViewDistance).
func (p *Player) NpcViewDistance() int {
	return p.npcViewDistance
}

// SetNpcViewDistance widens how far NPCs are seen, as OSRS does in some
// large arenas; a nil distance puts it back to NPCViewDistance (15).
// Non-nil distances are clamped to 1..MaxNPCViewDistance.
// Player.ts:1091-1095 (setNpcViewDistance). As in
// SetFixedViewDistance, no Math.trunc step exists for Go ints.
func (p *Player) SetNpcViewDistance(distance *int) {
	if distance == nil {
		p.npcViewDistance = NPCViewDistance
		return
	}
	p.npcViewDistance = min(max(*distance, 1), MaxNPCViewDistance)
}

// ResizeViewDistance adapts the player view radius to crowding, porting
// Player.ts:1097-1107 exactly: at or above PreferredLocalPlayers locals
// the distance shrinks toward 1 and the quiet-cycle count resets;
// otherwise, after ViewDistanceRegrowCycles consecutive quiet cycles it
// regrows toward PreferredViewDistance. It adjusts only the adaptive
// distance — a fixed distance still wins in ViewDistance. World calls
// it at the end of UpdateLocalPlayers (World.ts:789).
func (p *Player) ResizeViewDistance(localPlayerCount int) {
	if localPlayerCount >= PreferredLocalPlayers {
		if p.viewDistance > 1 {
			p.viewDistance--
		}
		p.viewDistanceQuietCycles = 0
		return
	}
	p.viewDistanceQuietCycles++
	if p.viewDistanceQuietCycles < ViewDistanceRegrowCycles {
		return
	}
	p.viewDistanceQuietCycles = 0
	if p.viewDistance < PreferredViewDistance {
		p.viewDistance++
	}
}

// LocalPlayers returns the live local-player list: the players this
// player currently sees, maintained by World.UpdateLocalPlayers.
// Player.ts:1129-1131 (getLocalPlayers returns the live array).
//
// Aliasing/thread-safety rules (the session worker reads this list in
// the flush phase — follow them exactly):
//   - Tick-goroutine-only: the list is mutated in the
//     update_players_npcs phase and read in the flush phase of the
//     same tick. No other goroutine may read or write it, and readers
//     must never mutate it (no append, sort, or element writes).
//   - Re-fetch every tick: UpdateLocalPlayers compacts and appends in
//     place and may reslice or reallocate, so a slice fetched in an
//     earlier tick has a stale length. The returned slice shares its
//     backing array with the player's list until an append reallocates;
//     do not retain it past the tick that produced it.
//   - The list never contains the player itself, and entries are the
//     registered players visible this tick (see UpdateLocalPlayers).
func (p *Player) LocalPlayers() []*Player {
	return p.localPlayers
}

// LocalNPCs returns the live local-NPC list: the NPCs this player
// currently sees, maintained by World.UpdateLocalNPCs.
// Player.ts:1133-1135 (getLocalNpcs returns the live array). The same
// aliasing/thread-safety rules as LocalPlayers apply: tick-goroutine
// only, read-only for callers, re-fetch every tick, do not retain.
func (p *Player) LocalNPCs() []*NPC {
	return p.localNPCs
}

// QueueTurnPacket appends pkt to the player's turn queue, returning false
// when the queue is at PacketQueueLimit (240) and the packet is dropped.
// NetworkConstants.ts: PACKET_QUEUE_LIMIT = 240.
//
// QueueTurnPacket is safe for concurrent use: the network goroutine
// enqueues while the tick goroutine drains. (TS needs no lock because it
// runs on one event loop; Go does.)
func (p *Player) QueueTurnPacket(pkt TurnPacket) bool {
	p.turnMu.Lock()
	defer p.turnMu.Unlock()
	if len(p.turnPackets) >= PacketQueueLimit {
		return false
	}
	p.turnPackets = append(p.turnPackets, pkt)
	return true
}

// DrainTurnPackets removes and returns all queued turn packets in arrival
// order. The returned slice is owned by the caller.
func (p *Player) DrainTurnPackets() []TurnPacket {
	p.turnMu.Lock()
	defer p.turnMu.Unlock()
	if len(p.turnPackets) == 0 {
		return nil
	}
	out := p.turnPackets
	p.turnPackets = nil
	return out
}

// TurnPacketCount returns the number of queued turn packets.
func (p *Player) TurnPacketCount() int {
	p.turnMu.Lock()
	defer p.turnMu.Unlock()
	return len(p.turnPackets)
}

// Process runs this player's foundation turn for the given cycle.
//
// The full TS turn order (Player.process(), Player.ts:483-586) is:
//
//	client input → map-clock varp → own hit queue → timers → combat
//	pre-movement → pursuit/movement → combat post-movement → arrival
//	animation → walk-to interaction reach checks → aggression tolerance +
//	NPC aggression → area processing → skull timer → run-energy restore →
//	prayer drain → stat restoration.
//
// This foundation implements the client-input, timer, movement, area,
// and run-energy-restore steps: drain turn packets through TurnHandler,
// run the timer phase (Mobile.ProcessTimers, the
// TimerRepository process step), then run the movement queue, then
// area processing (AreaManager.Process, with the bot downsampling
// rule), then the run-energy restore. The movement step
// ports the TS shape (Player.ts:506-518): beginCycle
// unconditionally, process only when the queue has pending work,
// flag APPEARANCE when the queue is moving, and end the
// click-run-override lifecycle at the end of the turn
// (dropPendingClickRunOverride / endClickRunOverrideWhenIdle,
// Player.ts:535-536 — the walk-to reach checks they follow in TS do
// not exist in Go yet, so they run directly after movement).
// processEntityPursuit (Player.ts:512) is omitted: no pursuit exists
// (see movement_queue.go). The run-energy restore branch
// (Player.ts:559-566) runs after movement, as in TS; the intervening
// TS steps do not exist in Go (see the restore comment in Process).
// Every other step is absent — not stubbed with fake behavior — and
// must be added by its owning milestone in the order above. Walk-to
// reach checks (TaskManager.ProcessWalkTo) are the coordinator's/World's
// job via the walk-to sweep; they are not run here because the walk-to
// owner key in TS is the player index and the TaskManager lives on World.
//
// LastInputCycle is refreshed to cycle when at least one packet was
// drained. (In TS the network layer sets lastInputCycle at socket receipt,
// NetworkBuilder.ts:680; the session layer in Go may set the field
// directly on receipt for exact parity. The refresh here keeps the field
// honest when packets only ever arrive through the turn queue.)
//
// A panic in TurnHandler propagates to World, which logs it and requests
// logout for the player (World.ts process_players phase); Process does
// not recover.
func (p *Player) Process(cycle int64) {
	packets := p.DrainTurnPackets()
	if len(packets) > 0 {
		p.LastInputCycle = cycle
		if p.TurnHandler != nil {
			for _, pkt := range packets {
				p.TurnHandler(p, pkt, cycle)
			}
		}
	}
	// Queued impacts land before this player acts (Player.ts:498,
	// combat_hits), ahead of timers: a hit that kills here stops
	// the rest of the turn via the isDying/hitpoints guards. The
	// typed GetCombat binds the player combat identity on first
	// use, so combat checks later this tick do not fail closed
	// on an unbound fighter (see combat.go).
	combat := p.GetCombat()
	combat.ProcessHitQueue(cycle)
	p.ProcessTimers(cycle)
	// Movement (see the doc comment). A zero-value Player has no
	// queue; NewPlayer always installs one.
	ranThisCycle := false
	if q := p.movementQueue; q != nil {
		q.BeginCycle()
		// Combat pre-movement (Player.ts:507-510) runs
		// unconditionally, not only when HasPendingWork: the
		// cycle guards inside Pre/PostMovementProcess make
		// the no-work call cheap, and pending retaliation
		// must fire even with no target (see combat.go).
		combat.PreMovementProcess()
		if q.HasPendingWork() {
			q.Process(cycle)
		}
		if q.IsMoving() {
			p.Flags = p.Flags.Set(FlagAppearance)
		}
		// Combat post-movement (Player.ts:521-523): landing
		// hits in range, after this cycle's steps.
		combat.PostMovementProcess()
		// Release last cycle's held arrival animation, unless
		// this cycle stepped again (Player.ts:525-528).
		p.releaseArrivalAnimation()
		q.DropPendingClickRunOverride()
		q.EndClickRunOverrideWhenIdle()
		ranThisCycle = q.DidRunThisCycle()
	}
	// NPC aggression (Player.ts:538-542): after movement and
	// the walk-to reach checks (which resolve in the World's
	// walk_to_sweep phase, after process_players), before area
	// processing. ProcessPlayerAggression skips bots and runs
	// the tolerance update plus the local-NPC pass.
	ProcessPlayerAggression(p, p.aggressionEnv())
	// Area processing (Player.ts:547-554): after movement,
	// walk-to reach checks, and aggression, before the skull
	// timer and run-energy restore. The walk-to and aggression
	// steps do not run inside Process (walk-to resolves in the
	// World's walk_to_sweep phase, after process_players; NPC
	// aggression is NOT STARTED), so the area phase runs
	// directly after the movement block — an arrival that
	// resolves in the sweep sees its area update on the next
	// tick. Bots downsample: full-frequency area processing
	// while moving or under force movement, otherwise every
	// third tick (Player.ts:549-553). Force movement does not
	// exist in Go, so that term is always false; the moving
	// term reads the queue, and the counter advances only on
	// ticks that reach this branch. A nil areaManager (a
	// standalone Player with no World) skips the phase.
	if p.areaManager != nil {
		// The counter advances only when the earlier terms
		// are false (TS short-circuits the assignment into
		// the || chain).
		shouldProcessArea := !p.IsPlayerBot() ||
			(p.movementQueue != nil && p.movementQueue.IsMoving())
		if !shouldProcessArea {
			p.botAreaProcessTick = (p.botAreaProcessTick + 1) % 3
			shouldProcessArea = p.botAreaProcessTick == 0
		}
		if shouldProcessArea {
			p.areaManager.Process(p)
		}
	}
	// Skull timer (Player.ts:556): after area processing,
	// before run-energy restore. ProcessSkullTimer skips
	// unskulled and dying players itself, holds the last tick
	// during PvP combat, and flags APPEARANCE (rebuilding the
	// cached appearance block) on the tick the skull runs out
	// (see skull.go).
	p.ProcessSkullTimer()
	// Run-energy restore (Player.ts:559-566): every tick not spent
	// running, not a bot, energy below max — add the restore units
	// (if > 0) and send run energy. TS runs it after movement,
	// with the combat post-movement, arrival-animation, walk-to
	// reach, aggression, area, and skull-timer steps in between;
	// of those, the area and skull-timer steps exist in Go
	// (above), so the restore runs directly after them, preserving the
	// after-movement order that matters (didRunThisCycle is only
	// meaningful once movement has run). Stamina ticks are always
	// 0: no stamina system exists (see runenergy.go).
	if !p.IsBot && p.GetRunEnergyUnits() < RunEnergyMaxUnits && !ranThisCycle {
		if restored := runEnergyRestoreUnits(p, p.GetCurrentAgilityLevel()); restored > 0 {
			p.AddRunEnergyUnits(restored)
			p.SendRunEnergy(p.GetRunEnergyUnits(), p.IsRunning(), p.GetWeightKg(), 0)
		}
	}
	// Prayer drain (Player.ts:568-570): only while draining;
	// ProcessDrain no-ops when no drain loop is running, which
	// is the Go form of the isDrainingPrayer gate.
	ProcessDrain(p)
	// Special-attack restore: TS starts the restore task on
	// login (Player.ts:794) and on drain (CombatSpecial.ts:178);
	// the ensure call here covers login (no tick-goroutine-safe
	// submit exists on the login path) and any other path that
	// lowered the energy. It no-ops when energy is full or a
	// restore is already running (see special_restore.go).
	EnsureSpecialRestore(&p.Mobile)
}

// aggressionEnv builds the AggressionEnv for this player's
// aggression pass from the installed AreaManager: the
// Wilderness for the isAggressiveTo level term, and the
// manager's InMulti (with its Wilderness zone fallback) for
// multi-combat. A nil areaManager (a standalone Player with
// no World) yields the zero env, whose documented
// degenerations apply (no wilderness, area-only multi).
// The EmitNpcAggressionTolerance hook stays nil:
// onNpcAggressionTolerance has no hub surface yet.
func (p *Player) aggressionEnv() AggressionEnv {
	var env AggressionEnv
	if p.areaManager == nil {
		return env
	}
	// Assign only when non-nil: a typed-nil *Wilderness in the
	// interface would defeat IsAggressiveTo's nil check and
	// panic on the method call.
	if wilderness := p.areaManager.Wilderness(); wilderness != nil {
		env.Wilderness = wilderness
	}
	manager := p.areaManager
	env.InMulti = func(actor AreaActor) bool { return manager.InMulti(actor) }
	return env
}

// ReadyToLeaveWorld reports whether a player queued for removal leaves the
// world this cycle, porting Player.ts:700-709 (readyToLeaveWorld) with the
// combat predicate supplied by InCombatHold:
//
//   - Out of combat (or no combat system): leave at once.
//   - In combat: stay until combat ends or LogoutHoldTicks (100) cycles
//     pass from the first cycle the hold was observed.
//
// The forced-logout timer branch of the TS method is absent: the timer
// type is NOT STARTED. The coordinator can force departure by clearing
// InCombatHold or via World's Updating flag.
func (p *Player) ReadyToLeaveWorld(cycle int64) bool {
	inCombat := false
	if p.InCombatHold != nil {
		inCombat = p.InCombatHold(cycle)
	}
	if !inCombat {
		return true
	}
	if p.logoutHoldCycle < 0 {
		p.logoutHoldCycle = cycle
	}
	return cycle-p.logoutHoldCycle >= LogoutHoldTicks
}

// SetDisconnectSource records why the connection ended; the first reason
// wins. Player.ts:688-690.
func (p *Player) SetDisconnectSource(source string) {
	if p.disconnectSource == "" {
		p.disconnectSource = source
	}
}

// DisconnectSource returns the recorded disconnect reason, or "".
func (p *Player) DisconnectSource() string { return p.disconnectSource }

// RequestLogout asks for removal from the world via the OnRequestLogout
// hook (TS Player.requestLogout, Player.ts:716-724, pushes the player onto
// World's remove queue and tells a connected session). With no hook
// installed this is a no-op; World installs the hook on add.
func (p *Player) RequestLogout() {
	if p.OnRequestLogout != nil {
		p.OnRequestLogout(p)
	}
}

// ResetUpdating clears per-tick update state via Mobile.ResetUpdating.
// TS Player has no resetUpdating override beyond Mobile's; NPC does
// (NPC.resetUpdating), Player does not.
func (p *Player) ResetUpdating() { p.Mobile.ResetUpdating() }
