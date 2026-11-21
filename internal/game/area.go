package game

import "sort"

// Area is game/model/areas/Area.ts: a named set of boundaries
// with member tracking and per-area behavior hooks.
//
// DESIGN (why a concrete struct with hook fields, not an
// interface plus a base struct). TS Area is an abstract class:
// content subclasses it and overrides methods, and several
// behaviors depend on object identity — Area.enter compares
// `previous !== this` (Area.ts:23), AreaManager checks
// `areas.includes(area)` (AreaManager.ts:35), and member maps
// key by index but verify identity (`===`, Area.ts:54-56). In
// Go, an interface-plus-embedded-base design breaks the first
// of those: a method promoted from an embedded base runs with
// the BASE as receiver, so `this` inside Enter would be the
// embedded struct, never the outer area value the manager
// holds, and re-entering the same area would spuriously fire
// leave + enter. A single concrete type keeps identity exact:
// the *Area the manager registers is the *Area whose methods
// run, and pointer comparison is TS `!==`. Overridable TS
// methods become hook fields on the struct (nil = the TS
// default), following this package's hook-field precedent
// (OnAddHook/OnRemoveHook on Mobile, OnEquipmentChanged on
// Player). Content areas construct an Area, fill the hooks
// they need (closures capture any per-area state a TS
// subclass would hold in fields), and register it; no
// subclassing, no identity gap.
//
// TS members with no Go equivalent, deliberately omitted:
//   - playerBots / getPlayerBots (Area.ts:11, 134-136): the
//     map is never populated — enter/leave carry "bots
//     disabled in this runtime" comments (Area.ts:26, 39) and
//     no code writes to it. Nothing to port.
//   - profileName: set by api.registerArea alongside
//     pluginName (PluginManager.ts:3523-3526) and used only
//     as a pluginperf timing label (PluginManager.ts:481-512
//     callArea). No perf timing exists in Go; the hub's
//     AreaMethodCaller labels calls with PluginName/GetName.
//
// Areas are runtime-only: nothing about an Area or its
// membership persists (TS keeps no area state in PlayerSave;
// the actor's area reference is rebuilt by AreaManager
// processing after login). Registration order is priority
// order: AreaManager.Get returns the first registered area
// whose boundary contains the position.
//
// CONCURRENCY. Areas are tick-goroutine-only, like the rest
// of entity processing: AreaManager.Process runs from
// Player/NPC Process on the tick goroutine, and member maps
// are unguarded.
type Area struct {
	name       string
	boundaries []Boundary
	players    map[int]*Player
	npcs       map[int]*NPC
	pluginName string

	// Hook fields. A nil hook means the TS default behavior
	// documented on the corresponding method. Hooks run
	// through the AreaMethodCaller seam when invoked via
	// AreaManager (enter/leave/process) or CallAreaMusicTrack;
	// direct calls to the methods below run the hook directly,
	// as a direct TS method call would.

	// PostEnterHook runs at the end of Enter (Area.ts:36,
	// postEnter: no-op default).
	PostEnterHook func(actor AreaActor)
	// PostLeaveHook runs at the end of Leave (Area.ts:49,
	// postLeave: no-op default). logout reports whether the
	// actor is leaving because it logged out.
	PostLeaveHook func(actor AreaActor, logout bool)
	// ProcessHook runs once per tick for each member, via
	// AreaManager.Process (Area.ts:59-61, process: no-op
	// default).
	ProcessHook func(actor AreaActor)
	// MusicTrackHook answers the area's automatic music track
	// (Area.ts:63-66, getMusicTrack: null default). It returns
	// the track id and true, or false to retain the normal
	// map/instance track.
	MusicTrackHook func(player *Player) (int, bool)
	// CanAttackHook answers whether attacker may attack
	// target (Area.ts:73-75, canAttack: null default). A nil
	// return means "no opinion" and falls through to the
	// plugin hooks.
	CanAttackHook func(attacker, target AreaActor, method any) *bool
	// CanCastOnNpcHook answers spell targeting
	// (Area.ts:77-80, canCastOnNpc: null default). A nil
	// return means "no opinion".
	CanCastOnNpcHook func(player *Player, npc *NPC, spell any) *bool
	// CanTeleportHook answers teleport permission
	// (Area.ts:82-84, canTeleport: null default). A nil
	// return means "no opinion".
	CanTeleportHook func(player *Player, wildernessLevelLimit int, destination any) *bool
	// CanTradeHook answers trade permission (Area.ts:86-89,
	// canTrade: null default). A nil return means "no
	// opinion".
	CanTradeHook func(player, target *Player) *bool
	// HandleForcedWalkHook answers a walk click the engine
	// would refuse mid forced move (Area.ts:91-97,
	// handleForcedWalk: false default): true when the area
	// takes it, false to refuse it.
	HandleForcedWalkHook func(player *Player, destination Location) bool
	// NpcAggressionToleranceHook answers whether npc stays
	// aggressive towards player despite the tolerance timer
	// (Area.ts:99-102, npcAggressionTolerance: null default).
	// A nil return means "no opinion".
	NpcAggressionToleranceHook func(player *Player, npc *NPC) *bool
	// NpcAggressionHook answers whether an aggressive npc
	// starts a fight with player (Area.ts:104-111,
	// npcAggression: null default). A nil return means "no
	// opinion".
	NpcAggressionHook func(player *Player, npc *NPC) *bool
	// IsMultiHook answers whether the area is a multi-combat
	// zone (Area.ts:113-116, isMulti: false default).
	IsMultiHook func() bool
}

// NewArea returns an area named name covering boundaries.
// name is the Go analogue of TS getName(), which returns the
// constructor name (Area.ts:122-124): content areas pass
// their type name. boundaries is stored as given, as the TS
// constructor stores its array (Area.ts:15-17); GetBoundaries
// returns the same slice, so callers must not mutate it after
// registration (TS offers no protection either, and the
// AreaManager search index assumes stable boundaries — see
// areamanager.go). A nil boundaries slice means the area
// covers nothing: AreaManager never resolves it, exactly as
// a TS area with an undefined/empty boundaries array never
// matches (AreaManager.ts:152-154).
func NewArea(name string, boundaries []Boundary) *Area {
	return &Area{
		name:       name,
		boundaries: boundaries,
		players:    make(map[int]*Player),
		npcs:       make(map[int]*NPC),
	}
}

// AreaActor is the minimal surface Area and AreaManager need
// from an actor (TS: Mobile). The coordinator makes *Player
// and *NPC satisfy it; see the report for the exact accessors
// the coordinator must add. Fixture actors in tests implement
// it directly.
//
// The method set is exactly what TS reads on the actor in
// Area.ts and AreaManager.ts:
//   - Location: the actor's position (getLocation).
//   - Area/SetArea: the actor's current area (Entity.ts
//     getArea/setArea). AreaManager records it before
//     processing; Area.Enter reads it for the previous-area
//     leave.
//   - GetIndex: the MobileList slot (getIndex), the member
//     map key. It is a method, not the Index field, because
//     an interface cannot require a field; it must read the
//     same slot MobileList assigns.
//   - IsPlayer/IsNPC: the member-map selection (Area.ts:28,
//     41, 53-56) and the multi-icon gate
//     (AreaManager.ts:87). Both predicates exist because TS
//     tests both: an actor that is neither is tracked in
//     neither map but still receives postEnter/postLeave.
//   - AsPlayer/AsNPC: the typed identity stored in the
//     member maps (getAsPlayer/getAsNpc). Each returns
//     non-nil exactly when the matching predicate is true;
//     HasMember compares the stored pointer against it, as
//     TS compares with ===.
//
// Multi-icon get/set is deliberately NOT part of this
// interface: it applies to players only, and AreaManager
// reaches it through the optional MultiIconActor assertion
// (see areamanager.go), so NPCs need no stub methods.
type AreaActor interface {
	Location() Location
	Area() *Area
	SetArea(area *Area)
	GetIndex() int
	IsPlayer() bool
	IsNPC() bool
	AsPlayer() *Player
	AsNPC() *NPC
}

// Enter adds actor to this area's members and runs PostEnter
// (Area.ts:19-34). If the actor is currently a member of a
// DIFFERENT area, that area hears it leave first (logout =
// false), through the AreaMethodCaller seam — code that moves
// an actor straight into another area bypasses AreaManager,
// so without this the old area would never hear it left
// (Area.ts:20-25). Re-entering the same area skips the leave:
// the previous area must differ by identity AND still hold
// the actor as a member. PostEnter runs even for an actor
// that is neither player nor NPC (tracked in neither map).
func (a *Area) Enter(actor AreaActor) {
	// Area.ts:22-25. The leave goes through the seam (TS:
	// PluginManager.callArea), so a throwing postLeave in
	// the old area cannot break entry into the new one once
	// the hub installs an isolating caller.
	previous := actor.Area()
	if previous != nil && previous != a && previous.HasMember(actor) {
		callAreaLeave(previous, actor, false)
	}
	if actor.IsPlayer() {
		a.players[actor.GetIndex()] = actor.AsPlayer()
	} else if actor.IsNPC() {
		a.npcs[actor.GetIndex()] = actor.AsNPC()
	}
	a.PostEnter(actor)
}

// PostEnter runs the PostEnterHook, if any (Area.ts:36,
// postEnter: no-op default).
func (a *Area) PostEnter(actor AreaActor) {
	if a.PostEnterHook != nil {
		a.PostEnterHook(actor)
	}
}

// Leave removes actor from this area's members and runs
// PostLeave (Area.ts:38-47). PostLeave runs even when the
// actor was never a member: TS deletes unconditionally, then
// calls postLeave.
func (a *Area) Leave(actor AreaActor, logout bool) {
	if actor.IsPlayer() {
		delete(a.players, actor.GetIndex())
	} else if actor.IsNPC() {
		delete(a.npcs, actor.GetIndex())
	}
	a.PostLeave(actor, logout)
}

// PostLeave runs the PostLeaveHook, if any (Area.ts:49,
// postLeave: no-op default).
func (a *Area) PostLeave(actor AreaActor, logout bool) {
	if a.PostLeaveHook != nil {
		a.PostLeaveHook(actor, logout)
	}
}

// HasMember reports whether actor entered this area and has
// not left it (Area.ts:51-57). Membership is identity, not
// index alone: the stored member at the actor's index must be
// the SAME actor (TS `===`). A different actor reusing the
// index — after a logout without a leave, say — is not a
// member. The nil guards cover actors whose typed accessor
// violates the AreaActor contract (a player returning a nil
// *Player); a conforming actor never triggers them.
func (a *Area) HasMember(actor AreaActor) bool {
	if actor.IsPlayer() {
		player := actor.AsPlayer()
		return player != nil && a.players[actor.GetIndex()] == player
	}
	if !actor.IsNPC() {
		return false
	}
	npc := actor.AsNPC()
	return npc != nil && a.npcs[actor.GetIndex()] == npc
}

// Process runs the ProcessHook, if any (Area.ts:59-61,
// process: no-op default). AreaManager calls it once per tick
// for each actor whose recorded area is this area, AFTER
// recording the area — process() may move the actor into
// another area, which is only detectable against what was
// recorded (AreaManager.ts:69-70).
func (a *Area) Process(actor AreaActor) {
	if a.ProcessHook != nil {
		a.ProcessHook(actor)
	}
}

// GetMusicTrack returns the area's automatic music track
// (Area.ts:63-66, getMusicTrack: null default). The second
// result is false when the area has no opinion, meaning the
// normal map/instance track is retained; it is the Go form of
// the TS null. The music block reaches this through
// CallAreaMusicTrack, which routes through the AreaMethodCaller
// seam as TS routes through PluginManager.callArea
// (PlayerSession.ts:394-395).
func (a *Area) GetMusicTrack(player *Player) (int, bool) {
	if a.MusicTrackHook != nil {
		return a.MusicTrackHook(player)
	}
	return 0, false
}

// CanAttack asks the area whether attacker may attack target
// (Area.ts:72-75). A nil return means "no opinion" and falls
// through to the plugin hooks. No caller exists yet: combat
// is NOT STARTED, so this is interface surface plus the
// default, with no call site invented.
func (a *Area) CanAttack(attacker, target AreaActor, method any) *bool {
	if a.CanAttackHook != nil {
		return a.CanAttackHook(attacker, target, method)
	}
	return nil
}

// CanCastOnNpc asks the area whether a spell may target npc
// (Area.ts:77-80). A nil return means "no opinion". No caller
// exists yet: combat/magic is NOT STARTED.
func (a *Area) CanCastOnNpc(player *Player, npc *NPC, spell any) *bool {
	if a.CanCastOnNpcHook != nil {
		return a.CanCastOnNpcHook(player, npc, spell)
	}
	return nil
}

// CanTeleport asks the area whether player may teleport
// (Area.ts:82-84). A nil return means "no opinion". No caller
// exists yet: teleportation is NOT STARTED.
func (a *Area) CanTeleport(player *Player, wildernessLevelLimit int, destination any) *bool {
	if a.CanTeleportHook != nil {
		return a.CanTeleportHook(player, wildernessLevelLimit, destination)
	}
	return nil
}

// CanTrade asks the area whether player may trade with target
// (Area.ts:86-89). A nil return means "no opinion". No caller
// exists yet: trading is NOT STARTED.
func (a *Area) CanTrade(player, target *Player) *bool {
	if a.CanTradeHook != nil {
		return a.CanTradeHook(player, target)
	}
	return nil
}

// HandleForcedWalk asks the area to take a walk click the
// engine would refuse because the player is mid forced move
// (Area.ts:91-97). It returns false by default (refuse). No
// caller exists yet: force movement is NOT STARTED.
func (a *Area) HandleForcedWalk(player *Player, destination Location) bool {
	if a.HandleForcedWalkHook != nil {
		return a.HandleForcedWalkHook(player, destination)
	}
	return false
}

// NpcAggressionTolerance asks whether npc stays aggressive
// towards player despite the tolerance timer (Area.ts:99-102).
// A nil return means "no opinion". No caller exists yet: NPC
// aggression is NOT STARTED.
func (a *Area) NpcAggressionTolerance(player *Player, npc *NPC) *bool {
	if a.NpcAggressionToleranceHook != nil {
		return a.NpcAggressionToleranceHook(player, npc)
	}
	return nil
}

// NpcAggression asks whether an aggressive npc starts a fight
// with player (Area.ts:104-111). A nil return means "no
// opinion". No caller exists yet: NPC aggression is NOT
// STARTED.
func (a *Area) NpcAggression(player *Player, npc *NPC) *bool {
	if a.NpcAggressionHook != nil {
		return a.NpcAggressionHook(player, npc)
	}
	return nil
}

// IsMulti reports whether this area is a multi-combat zone
// (Area.ts:113-116, isMulti: false default). Areas outside
// the Wilderness opt in here; Wilderness multi-combat comes
// from the zone boundaries via AreaManager.InMulti.
func (a *Area) IsMulti() bool {
	if a.IsMultiHook != nil {
		return a.IsMultiHook()
	}
	return false
}

// GetBoundaries returns the area's boundaries (Area.ts:118-120),
// the same slice NewArea received. Callers must not mutate it.
func (a *Area) GetBoundaries() []Boundary { return a.boundaries }

// GetName returns the area's name (Area.ts:122-124): the name
// NewArea received, standing in for the TS constructor name.
func (a *Area) GetName() string { return a.name }

// GetNPCs returns the NPC members, in ascending index order
// (Area.ts:126-128, Object.values: JS integer keys iterate in
// ascending numeric order). The slice is fresh; mutating it
// does not affect membership.
func (a *Area) GetNPCs() []*NPC {
	return sortedMembers(a.npcs)
}

// GetPlayers returns the player members, in ascending index
// order (Area.ts:130-132; see GetNPCs). The slice is fresh.
func (a *Area) GetPlayers() []*Player {
	return sortedMembers(a.players)
}

// PluginName returns the name of the plugin that registered
// this area, or "" when unset (Area.ts:12-13, pluginName:
// null until api.registerArea sets it).
func (a *Area) PluginName() string { return a.pluginName }

// SetPluginName records the registering plugin's name. The
// hub's registerArea calls it before pushing the area onto
// the registry (PluginManager.ts:3523-3526).
func (a *Area) SetPluginName(name string) { a.pluginName = name }

// sortedMembers returns the values of an index-keyed member
// map in ascending index order. It is generic over the member
// type because players and NPCs share the ordering rule; the
// type parameter is inferred from the argument.
func sortedMembers[T any](members map[int]T) []T {
	keys := make([]int, 0, len(members))
	for k := range members {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := make([]T, 0, len(keys))
	for _, k := range keys {
		out = append(out, members[k])
	}
	return out
}

// AreaMethodCaller is the PluginManager.callArea analogue
// (PluginManager.ts:481-512): the seam through which Area
// methods run when invoked by the engine. TS callArea runs
// one area method the way hooks run — under the registering
// plugin's context, timed under pluginperf when enabled, and
// a throw is logged instead of escaping into the actor's tick
// (where it would log a player out), returning null.
//
// This package must never import the plugins package, so the
// caller is an installed seam: the default is direct calls
// (a panic propagates, as a direct method call would), and
// the coordinator/hub installs an isolating implementation
// via SetAreaMethodCaller at boot. The isolating caller must
// recover per call, log the failure with the area's plugin
// name, and return the zero value (MusicTrack: no opinion),
// mirroring callArea's catch-and-return-null. Perf timing is
// the hub's concern; no timing hook exists here.
//
// The seam covers exactly the callArea sites in TS: enter,
// leave, and process from AreaManager.process
// (AreaManager.ts:41, 63, 73), the previous-area leave from
// Area.enter (Area.ts:24), and getMusicTrack from the music
// block (PlayerSession.ts:395). The rule hooks (CanAttack et
// al.) have no callArea sites in TS core — their callers ask
// the area directly — so they are not routed through the seam.
type AreaMethodCaller interface {
	// Enter runs area.Enter.
	Enter(area *Area, actor AreaActor)
	// Leave runs area.Leave.
	Leave(area *Area, actor AreaActor, logout bool)
	// Process runs area.Process.
	Process(area *Area, actor AreaActor)
	// MusicTrack runs area.GetMusicTrack.
	MusicTrack(area *Area, player *Player) (int, bool)
}

// directAreaCaller is the default AreaMethodCaller: direct
// method calls, no isolation. It is what runs when no hub
// caller is installed.
type directAreaCaller struct{}

func (directAreaCaller) Enter(area *Area, actor AreaActor) { area.Enter(actor) }
func (directAreaCaller) Leave(area *Area, actor AreaActor, logout bool) {
	area.Leave(actor, logout)
}
func (directAreaCaller) Process(area *Area, actor AreaActor) { area.Process(actor) }
func (directAreaCaller) MusicTrack(area *Area, player *Player) (int, bool) {
	return area.GetMusicTrack(player)
}

// areaCaller is the installed AreaMethodCaller. It is set
// once at boot via SetAreaMethodCaller, before the world
// starts ticking, and never changed afterwards; like the
// other coordinator-installed wiring in this package it is
// deliberately unsynchronized.
var areaCaller AreaMethodCaller = directAreaCaller{}

// SetAreaMethodCaller installs the AreaMethodCaller the
// engine routes area method calls through. A nil caller
// restores direct calls. Call once at boot, before ticking
// starts; see AreaMethodCaller.
func SetAreaMethodCaller(caller AreaMethodCaller) {
	if caller == nil {
		areaCaller = directAreaCaller{}
		return
	}
	areaCaller = caller
}

// callAreaEnter runs area.Enter through the installed caller.
func callAreaEnter(area *Area, actor AreaActor) { areaCaller.Enter(area, actor) }

// callAreaLeave runs area.Leave through the installed caller.
func callAreaLeave(area *Area, actor AreaActor, logout bool) {
	areaCaller.Leave(area, actor, logout)
}

// callAreaProcess runs area.Process through the installed caller.
func callAreaProcess(area *Area, actor AreaActor) { areaCaller.Process(area, actor) }

// CallAreaMusicTrack runs area.GetMusicTrack through the
// installed caller (PlayerSession.ts:394-395: the music block
// asks the player's area for a track override via callArea).
// A nil area yields no opinion, mirroring the TS optional
// chain (`privateArea?.getMusicTrack`), which never reaches
// callArea for a null area. The coordinator's music-block
// closure calls this; see internal/session/music.go.
func CallAreaMusicTrack(area *Area, player *Player) (int, bool) {
	if area == nil {
		return 0, false
	}
	return areaCaller.MusicTrack(area, player)
}
