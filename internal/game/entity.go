package game

import "sync"

// Entity is the foundation core of game/entity/Entity.ts: an index in its
// world list, a world location, and the current area reference
// (Entity.ts: area). The string flag set (Entity.ts: flags) is NOT
// STARTED: the flags are content markers, distinct from the update
// Flags on Mobile.
type Entity struct {
	// Index is the entity's slot in its MobileList. Slot 0 is never
	// assigned; valid indices run 1..capacity. See MobileList.
	Index int
	// Loc is the entity's current world position (Entity.ts: location).
	Loc Location
	// area is the entity's current area (Entity.ts: area), resolved
	// once per tick by AreaManager.Process. Nil means the entity is
	// in no area. Read/written through Mobile.Area/SetArea; see
	// area.go. Tick-goroutine-only, like Loc.
	area *Area
}

// Mobile is the foundation core of game/entity/impl/Mobile.ts (1,007 lines
// in TS; only the fields listed here are ported).
//
// Ported core fields:
//   - Index / Loc via Entity (Mobile.ts:71, Entity.ts: location).
//   - Facing: the last compass direction faced or stepped in, which
//     survives ResetUpdating (Mobile.ts:81-85, facingDirection).
//   - LastKnownRegion: baseline region for movement/updates, initialized
//     to the spawn location (Mobile.ts:72, 143-144).
//   - Flags: the per-tick update flag set (Mobile.ts:87, updateFlag).
//   - WalkingDirection / RunningDirection: this tick's step directions,
//     cleared by ResetUpdating (Mobile.ts:78-79, 264-278).
//   - NeedsPlacement / ResetMovementQueue: sync placement markers cleared
//     by ResetUpdating (Mobile.ts:265-278).
//   - Timers: the lazily allocated timer repository
//     (Mobile.ts:73, TimerRepository), accessed through
//     GetTimers and ticked once per turn by ProcessTimers;
//     see timers.go.
//   - Attributes: the lazily allocated attribute map (Mobile.ts:102-110),
//     accessed through GetAttribute/SetAttribute/HasAttribute in
//     attributes.go.
//   - ForcedChat: this tick's overhead chat text (Mobile.ts:76,
//     forcedChat), set by ForceChat and cleared by ResetUpdating;
//     see chat.go.
//
// NOT ported (absent on purpose; zero values here mean "not present", never
// a gameplay default): combat, hitpoints, animation/graphic,
// following/interaction targets, prayers, special attack,
// poison/venom, teleport serials, and tick hits. Mobile itself holds no
// MovementQueue field: the queue lives on Player (see Player.MovementQueue),
// which is the only owner wired this phase; NPCs get no queue (see npc.go).
// In particular there is no Size field: TS getSize() is abstract and
// definition-driven, and inventing a default of 1 would silently change
// reach/visibility math. Player.Size returns 1 explicitly (Player.ts:93);
// NPC size stays unported until definitions land.
type Mobile struct {
	Entity

	// Facing is the last direction faced or stepped in; DirNone until a
	// face target or movement supplies one. TS establishes no spawn
	// direction for players (Mobile.ts:82-85).
	Facing Direction

	// LastKnownRegion is the last region the sync layer placed this
	// entity in; it starts as the spawn location.
	LastKnownRegion Location

	// Flags is the per-tick update flag set.
	Flags Flags

	// WalkingDirection and RunningDirection are this tick's step
	// directions (DirNone when the entity did not step).
	WalkingDirection Direction
	RunningDirection Direction

	// NeedsPlacement marks the entity for (re)placement in the sync
	// frame; ResetMovementQueue marks its movement queue for reset.
	// Both are cleared by ResetUpdating.
	NeedsPlacement     bool
	ResetMovementQueue bool

	// timers is the lazily allocated timer repository
	// (Mobile.ts:73, `new TimerRepository()`). It stays nil
	// until the first GetTimers call; see timerRepository.
	// The field is a pointer so Mobile stays copyable by
	// value without copying the repository's mutex, as with
	// attributes below. TS constructs the repository eagerly
	// in the field initializer and lazily allocates only the
	// map inside it (TimerRepository.ts:7); the Go laziness
	// is one level up and unobservable — GetTimers never
	// returns nil for a non-nil Mobile, and always returns
	// the same repository.
	timers *TimerRepository

	// combat is the lazily allocated combat state (Mobile.ts:74,
	// 146, `new Combat(this)`). It stays nil until the first
	// GetCombat call; see combat.go. The field is a pointer so
	// Mobile stays copyable by value without copying the
	// combat state's mutex, as with timers above. TS constructs
	// it eagerly in the constructor; the Go laziness is
	// unobservable — GetCombat never returns nil for a non-nil
	// Mobile, and always returns the same Combat.
	combat *Combat

	// OnAddHook / OnRemoveHook run from OnAdd / OnRemove. TS Mobile
	// declares onAdd/onRemove abstract (Mobile.ts:149-151) and
	// Player/NPC implement them; in Go the concrete types set these
	// hooks or leave them nil.
	OnAddHook    func()
	OnRemoveHook func()

	// attributes is the lazily allocated attribute store
	// (Mobile.ts:102, `Map | null`). It stays nil until the first
	// SetAttribute stores a value; see attributes.go for the
	// accessors and the thread-safety rules. The field is a
	// pointer so Mobile stays copyable by value without copying
	// a mutex.
	attributes *attributeStore

	// forcedChat is this tick's overhead chat text, or nil when
	// the mobile has none (Mobile.ts:76, `forcedChat: string`,
	// null until set). Set by ForceChat with FlagForcedChat;
	// cleared by ResetUpdating. Tick-goroutine-only, like Flags
	// (see chat.go); the pointer mirrors the TS nullability so
	// the sync gate can check flag AND text, as TS does.
	forcedChat *string

	// animation is this tick's animation (Mobile.ts:369-392,
	// `animation: Animation`, null until set). Set by
	// PerformAnimation with the priority rules of
	// mobile_animation.go; cleared by ResetUpdating, after
	// the sync snapshot has been taken. The pointer mirrors
	// the TS nullability so the sync gate can check flag AND
	// animation, as TS does (createActorUpdates).
	animation *Animation
	// animationGeneration bumps on every accepted animation
	// (Mobile.ts:91, 390, getAnimationGeneration).
	animationGeneration int
	// graphic is this tick's spotanim in slot 0
	// (Mobile.ts:394-402, `graphic: Graphic`, null until
	// set). Set by PerformGraphic; cleared by ResetUpdating.
	graphic *Graphic
	// slotGraphics is this tick's spotanim slots other than 0
	// (Mobile.ts:407, `slotGraphics: Map<number, Graphic |
	// null> | null`). A nil value is a cleared slot (syncs
	// as id -1); slot 0 never appears here (it is graphic).
	// Set by PerformGraphicInSlot / ClearGraphicInSlot;
	// cleared by ResetUpdating.
	slotGraphics map[int]*Graphic

	registered bool

	// poisonDamage is the live poison severity, or the venom
	// damage when venomed (Mobile.ts:118-126, poisonDamage =
	// 0). Venom uses a different damage curve (see
	// CombatPoisonEffect): it starts low and escalates over
	// time instead of decaying, and doesn't expire on its
	// own. Owned save state: the save carries it
	// (persist poisonDamage); see PoisonSaveState /
	// RestorePoisonFromSave (poison.go).
	poisonDamage int
	// venomed reports whether the current affliction is
	// venom rather than ordinary poison (Mobile.ts:120-126).
	venomed bool
	// lastVenomDamage is the last venom hit's damage taken,
	// or nil when none / not venomed (Mobile.ts:
	// lastVenomDamage).
	lastVenomDamage *int
	// venomGeneration bumps whenever the affliction ends or
	// changes kind (Mobile.ts: venomGeneration): a delayed
	// OnApplied observer only records its hit when the
	// generation still matches, so a cure between the queue
	// and the landing cannot record damage for a dead
	// affliction.
	venomGeneration int
}

// NewMobile returns a Mobile at loc with LastKnownRegion initialized to
// loc and facing DirNone, mirroring the Mobile/Entity constructors
// (Mobile.ts:141-147).
func NewMobile(loc Location) Mobile {
	return Mobile{
		Entity:           Entity{Loc: loc},
		Facing:           DirNone,
		LastKnownRegion:  loc,
		WalkingDirection: DirNone,
		RunningDirection: DirNone,
	}
}

// Location returns the entity's current world position.
// Entity.ts: getLocation. It is a value copy: Location is a value type
// in Go, so callers cannot mutate the entity's position through it.
func (m *Mobile) Location() Location { return m.Loc }

// SetLocation moves the entity to loc. It ports only the assignment
// half of Mobile.setLocation (Mobile.ts:156-166): the TS override also
// fires World.onPlayerMoved / World.onNpcMoved for registered entities
// whose tile changed, and those hooks are NOT STARTED in Go (no
// tile-occupancy or active-region systems exist). Visibility rebuild
// reads Loc directly every tick (see visibility.go), so position
// changes still reach the sync layer without the hooks.
func (m *Mobile) SetLocation(loc Location) { m.Loc = loc }

// GetLastKnownRegion returns the last region the sync layer placed this
// entity in. Mobile.ts:954-956 (getLastKnownRegion). The Get- prefix is
// forced by the exported LastKnownRegion field: a method may not share
// a field's name.
func (m *Mobile) GetLastKnownRegion() Location { return m.LastKnownRegion }

// SetLastKnownRegion records the last region the sync layer placed this
// entity in. Mobile.ts:958-961 (setLastKnownRegion).
func (m *Mobile) SetLastKnownRegion(region Location) { m.LastKnownRegion = region }

// SetWalkingDirection records this tick's walking step direction, and
// faces the entity that way when the direction id is >= 0.
// Mobile.ts:806-811 (setWalkingDirection): the facing update is skipped
// for DirNone (id -1), so a no-step tick leaves Facing untouched.
func (m *Mobile) SetWalkingDirection(d Direction) {
	m.WalkingDirection = d
	if int(d) >= 0 {
		m.Facing = d
	}
}

// SetRunningDirection records this tick's running step direction, and
// faces the entity that way when the direction id is >= 0.
// Mobile.ts:817-822 (setRunningDirection), same facing rule as
// SetWalkingDirection.
func (m *Mobile) SetRunningDirection(d Direction) {
	m.RunningDirection = d
	if int(d) >= 0 {
		m.Facing = d
	}
}

// IsRegistered reports whether the entity currently holds a MobileList
// slot. Mobile.ts:831-833 (isRegistered).
func (m *Mobile) IsRegistered() bool { return m.registered }

// SetRegistered sets the registration state. Called only by MobileList.
// Mobile.ts:835-837 (setRegistered).
func (m *Mobile) SetRegistered(registered bool) { m.registered = registered }

// PoisonDamage returns the live poison severity, or the
// venom damage when venomed (Mobile.ts:550,
// getPoisonDamage).
func (m *Mobile) PoisonDamage() int { return m.poisonDamage }

// SetPoisonDamage sets the poison severity / venom damage
// (Mobile.ts:554-558, setPoisonDamage): dropping to 0 or
// below bumps the venom generation (the affliction ended)
// and clears the last venom damage.
func (m *Mobile) SetPoisonDamage(poisonDamage int) {
	if poisonDamage <= 0 && m.poisonDamage > 0 {
		m.venomGeneration++
	}
	m.poisonDamage = poisonDamage
	if poisonDamage <= 0 {
		m.lastVenomDamage = nil
	}
}

// IsPoisoned reports whether the mobile is poisoned or
// venomed (Mobile.ts:559-561, isPoisoned): damage > 0.
func (m *Mobile) IsPoisoned() bool { return m.poisonDamage > 0 }

// IsVenomed reports whether the current affliction is venom
// (Mobile.ts:563-565, isVenomed): venomed AND damage > 0.
func (m *Mobile) IsVenomed() bool { return m.venomed && m.poisonDamage > 0 }

// SetVenomed records whether the affliction is venom
// (Mobile.ts:567-571, setVenomed): a change bumps the venom
// generation, and clearing venom clears the last venom
// damage.
func (m *Mobile) SetVenomed(venomed bool) {
	if m.venomed != venomed {
		m.venomGeneration++
	}
	m.venomed = venomed
	if !venomed {
		m.lastVenomDamage = nil
	}
}

// LastVenomDamage returns the last venom hit's damage taken,
// or nil when none (Mobile.ts:573-575,
// getLastVenomDamage).
func (m *Mobile) LastVenomDamage() *int { return m.lastVenomDamage }

// VenomGeneration returns the venom generation counter
// (Mobile.ts:577-579, getVenomGeneration).
func (m *Mobile) VenomGeneration() int { return m.venomGeneration }

// SetLastVenomDamage records a venom hit's damage taken
// (Mobile.ts:581-584, setLastVenomDamage): only positive
// damage counts, capped at 20; anything else clears it.
// (TS also floors the value; Go damage is already whole.)
func (m *Mobile) SetLastVenomDamage(damage int) {
	if damage > 0 {
		value := damage
		if value > 20 {
			value = 20
		}
		m.lastVenomDamage = &value
		return
	}
	m.lastVenomDamage = nil
}

// SetIndex assigns the MobileList slot. Called only by MobileList.
// Mobile.ts:949-952 (setIndex).
func (m *Mobile) SetIndex(index int) { m.Index = index }

// GetIndex returns the MobileList slot. It is the AreaActor form of
// the Index field (Entity.ts: getIndex; an interface cannot require
// a field, so AreaManager reads the slot through this method).
func (m *Mobile) GetIndex() int { return m.Index }

// Area returns the entity's current area, or nil when it is in no
// area. Entity.ts:64 (getArea). AreaManager.Process resolves it once
// per tick; the logout path clears membership through Leave, not by
// clearing this reference (see World.drainRemovePlayers).
func (m *Mobile) Area() *Area { return m.area }

// SetArea records the entity's current area. Entity.ts (setArea).
// Called by AreaManager.Process (which records the area before
// processing it) and by area-transition code that moves an actor
// directly; direct calls bypass AreaManager and therefore skip
// enter/leave delivery, exactly as a direct TS setArea does.
func (m *Mobile) SetArea(area *Area) { m.area = area }

// PrivateArea returns the private area the entity is in, or nil
// when it is in no private area. Entity.ts:68-70 (getPrivateArea):
// the area, but only when it "carries objects" — the duck-type
// check for a PrivateArea (PrivateArea is the Area subclass with
// getObjects). Go cannot duck-type the concrete *Area, so the
// answer comes from the private-area index (see private_area.go):
// the PrivateArea whose embedded *Area is this entity's area.
// A nil Mobile has no area. The area a destroyed instance leaves
// behind resolves to nil (see the PrivateArea comment); members
// of a live private area always resolve.
func (m *Mobile) PrivateArea() *PrivateArea {
	if m == nil {
		return nil
	}
	return privateAreaForArea(m.area)
}

// OnAdd runs the add hook, if any. Called by MobileList.Add after the
// slot is assigned (MobileList.ts: add() calls e.onAdd()).
func (m *Mobile) OnAdd() {
	if m.OnAddHook != nil {
		m.OnAddHook()
	}
}

// OnRemove runs the remove hook, if any. Called by MobileList.Remove
// after the slot is freed (MobileList.ts: removes()/remove()).
func (m *Mobile) OnRemove() {
	if m.OnRemoveHook != nil {
		m.OnRemoveHook()
	}
}

// timerAllocMu serializes lazy allocation of TimerRepository
// instances and every load of a Mobile's timers pointer. See
// timerRepository; the shape mirrors attributeAllocMu in
// attributes.go.
var timerAllocMu sync.Mutex

// timerRepository returns m's timer repository, allocating it
// when create is true and none exists yet. It returns nil when
// no repository exists and create is false. A nil receiver has
// no repository. This mirrors the store() helper in
// attributes.go: lazy allocation itself is serialized by
// timerAllocMu, so a first GetTimers racing a ProcessTimers
// cannot tear the pointer.
func (m *Mobile) timerRepository(create bool) *TimerRepository {
	if m == nil {
		return nil
	}
	timerAllocMu.Lock()
	defer timerAllocMu.Unlock()
	if m.timers == nil && create {
		m.timers = NewTimerRepository()
	}
	return m.timers
}

// GetTimers returns this mobile's timer repository, allocating
// it on first use. Mobile.ts:963-965 (getTimers). A nil Mobile
// has no repository and returns nil; TimerRepository methods
// on a nil receiver are safe no-ops/zero reads (see
// timers.go).
func (m *Mobile) GetTimers() *TimerRepository {
	return m.timerRepository(true)
}

// ProcessTimers ticks every registered timer once for this
// cycle: TimerRepository.process() (TimerRepository.ts:65-71),
// called from Player.process (Player.ts:501, before movement)
// and NPC.process (NPC.ts:309). A mobile with no repository
// yet has no timers to tick, so this does not allocate one.
//
// The cycle parameter is ACCEPTED BUT UNUSED: TS process()
// takes no cycle, and nothing in the tick depends on it. The
// signature is kept because Player.Process and NPC.Process
// call ProcessTimers(cycle) with the turn's cycle; changing
// it would churn both call sites for no behavior.
func (m *Mobile) ProcessTimers(cycle int64) {
	_ = cycle
	if repo := m.timerRepository(false); repo != nil {
		repo.Process()
	}
}

// ResetUpdating clears the per-tick update state, mirroring the foundation
// subset of Mobile.resetUpdating() (Mobile.ts:264-278): flags, step
// directions, placement markers, forced chat, animation, graphic,
// and slot graphics. (The TS reset also clears tick hits and tint
// state — those are cleared by their own phases in Go, not here.)
// Facing is deliberately NOT cleared: it survives resetUpdating in TS
// (Mobile.ts:81-85).
func (m *Mobile) ResetUpdating() {
	m.Flags = m.Flags.Reset()
	m.WalkingDirection = DirNone
	m.RunningDirection = DirNone
	m.NeedsPlacement = false
	m.ResetMovementQueue = false
	m.forcedChat = nil
	m.animation = nil
	m.graphic = nil
	m.slotGraphics = nil
}

// IndexedMobile is the contract MobileList requires of its elements: the
// index/registration bookkeeping TS MobileList performs on Mobile
// (MobileList.ts: add/removes). *Player and *NPC satisfy it through the
// embedded Mobile.
type IndexedMobile interface {
	IsRegistered() bool
	SetRegistered(bool)
	SetIndex(int)
	OnAdd()
	OnRemove()
}
