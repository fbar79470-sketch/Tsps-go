package game

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"tsps-go/internal/definition"
)

// Ground items: ItemOnGround + ItemOnGroundManager, porting
// game/entity/impl/grounditem/ItemOnGround.ts,
// game/entity/impl/grounditem/ItemOnGroundManager.ts, and the respawn
// half of game/task/impl/GroundItemRespawnTask.ts.
//
// TS keeps the manager as statics over the global World item list; Go
// keeps it as a struct owning its list, so tests and the coordinator
// can own an instance. The coordinator installs the per-tick pass
// behind WorldHooks.GroundItems (world.go), supplies the player list,
// the send adapter, the task scheduler, and the definition lookups,
// and applies loaded spawns through ApplySpawn. See the
// GroundItemManagerConfig doc for each seam and its nil degradation.
//
// What is NOT ported here, by the Phase 8 scope: pickup, telegrab,
// transfer-to-inventory, takeOperation, getOperableGroundItem,
// canOperate, playReachAnimation, and rewardItem
// (ItemOnGroundManager.ts:196-364, 448-470) — items are visible but
// uninteractable. No listener change lives here either.
//
// Degenerations versus TS, all deliberate:
//   - Private areas do not exist in Go, so every item and every player
//     is in the same (nil) area: the area-equality checks in merge,
//     canSee, and register degenerate to true, and the
//     area ground-item despawn delay in register never applies.
//   - CrossFloor has no plane policy in Go, so CrossFloor.visible
//     degenerates to same-plane plus Chebyshev distance (see
//     crossFloorVisible).
//   - TS skips only unwatched bots in performPlayer; Go skips ALL bots
//     (no watched/unwatched distinction exists — the same documented
//     deviation as the Phase 7 destination flag).
//   - Item ids are manager-scoped and assigned at Register time (from
//     1), where TS assigns them at construction from a process-wide
//     static counter. Registered items still get unique ids starting
//     at 1, and ids only leave the manager through views and remove
//     lists, so nothing observable changes.
//   - The game layer never imports protocol: views cross to the
//     session layer as game.GroundItemView values through the
//     GroundItemSender interface, and the cmd/server adapter converts
//     them to protocol.GroundItemView.
//
// Tick-goroutine-only, like the rest of the world: Register,
// Deregister, Merge, Process, and the view builders must run on the
// tick goroutine (in TS everything runs on one event loop). The
// Players func may read the internally locked MobileList from any
// goroutine, but the manager itself is not synchronized.

// PublicRevealDelay is the ticks an owner-only stack stays private
// before going public: PUBLIC_REVEAL_DELAY
// (ItemOnGroundManager.ts:26). OSRS/Lost City floor-item behavior is
// owner-only for 100 ticks, then public.
const PublicRevealDelay = 100

// DespawnDelay is the default ticks a stack lasts on the floor:
// DESPAWN_DELAY (ItemOnGroundManager.ts:27). An item with its own
// despawn delay keeps it (ItemOnGround.getDespawnDelay,
// ItemOnGround.ts:218-220).
const DespawnDelay = 300

// MaxStackAmount is the largest amount a single stack may hold:
// MAX_STACK_AMOUNT (game/model/container/StackType.ts:6). The client
// renders quantities as a signed 32-bit integer.
const MaxStackAmount = 0x7fffffff

// GroundItemViewRange is the Chebyshev range within which a player can
// see a ground item: the 64 in canSee (ItemOnGroundManager.ts:452).
const GroundItemViewRange = 64

// GroundItemState is the visibility state of a ground item: State
// (ItemOnGround.ts:236-238). The numeric values matter: PacketSender
// .groundItemView tests state === 0 for isPrivate
// (PacketSender.ts:1461-1476), so SEEN_BY_PLAYER must stay 0.
type GroundItemState int

const (
	// GroundItemSeenByPlayer is State.SEEN_BY_PLAYER (0): only the
	// owner sees the stack.
	GroundItemSeenByPlayer GroundItemState = iota
	// GroundItemSeenByEveryone is State.SEEN_BY_EVERYONE (1): every
	// player who can see the tile sees the stack.
	GroundItemSeenByEveryone
)

// GroundItemOp is one ground-item fan-out operation: OperationType
// (ItemOnGroundManager.ts:472-476). TS uses distinct instances with no
// numeric meaning; the Go values are arbitrary.
type GroundItemOp int

const (
	// GroundItemCreate sends the stack to viewers (CREATE).
	GroundItemCreate GroundItemOp = iota
	// GroundItemDelete removes the stack from viewers (DELETE).
	GroundItemDelete
	// GroundItemAlter re-sends the stack with new state (ALTER).
	GroundItemAlter
)

// Ownership values for GroundItemView.Ownership, from
// PacketSender.groundItemView (PacketSender.ts:1461-1476): 0 when the
// stack has no owner, 1 when the viewer owns it, 2 when another player
// owns it. The TS GroundItemView type also allows 3, but
// groundItemView never produces it.
const (
	GroundItemOwnershipNone  = 0
	GroundItemOwnershipMine  = 1
	GroundItemOwnershipOther = 2
)

// GroundPickupReplacement is the item a take credits instead of the
// stack itself: GroundPickupReplacement (ItemOnGround.ts:7). Only the
// identity fields participate in merge policy; the take itself is NOT
// STARTED (see the file header).
type GroundPickupReplacement struct {
	ID int
	// TelegrabDenial is the denial message, or "" when the entry
	// carries none. TS requires a present denial to be a non-empty
	// string (GroundItemSpawnDefinitionLoader.ts:112-114), so ""
	// unambiguously means absent — the same convention as
	// definition.PickupReplacement.
	TelegrabDenial string
}

// ItemOnGround is one stack on the floor: ItemOnGround
// (ItemOnGround.ts:9-234). The TS Item object is flattened to
// ItemID/Amount/Meta here: no Item type exists in the Go game layer,
// and the ground-item logic only ever reads the id, the amount, the
// meta (for merge identity and the untradeable flag), and the
// definition lookups, which arrive as injected funcs on the manager.
//
// Owner is "" when the stack has no owner (TS undefined). Usernames
// are never empty, so "" is an unambiguous sentinel, and every check
// below treats "" exactly as TS treats undefined/null: isOwner
// requires a non-empty owner, ownership is 0, and no owner DELETE is
// sent on reveal.
//
// Use NewItemOnGround: it initializes the derived fields (respawn and
// old amounts, enabled ops, aged cycle) exactly as the TS constructor
// does. A zero-value ItemOnGround has enabled ops 0 (TS default
// 0x1f) and aged cycle 0 (TS default -1).
type ItemOnGround struct {
	id int

	// Position is the stack's tile (TS position: Location).
	Position Location
	// State is the visibility state (TS state).
	State GroundItemState
	// Owner is the owning username, or "" for no owner (TS owner).
	Owner string
	// ItemID is the stack's item id (TS item id).
	ItemID int
	// Amount is the stack's quantity (TS item amount).
	Amount int
	// Meta is the stack's item metadata (TS item meta,
	// Record<string, unknown> | null). Nil means no meta. Values
	// must be JSON-encodable: merge identity compares the canonical
	// JSON form (see metaKey). The map is deep-copied on
	// construction and on Clone, as TS Item.cloneMeta does
	// (game/model/Item.ts:27-50).
	Meta map[string]any
	// Tick counts the ticks the stack has aged (TS tick).
	Tick int
	// GoesGlobal reports whether the stack goes public after the
	// reveal delay (TS goesGlobal).
	GoesGlobal bool
	// PendingRemoval marks a stack awaiting removal from the list
	// (TS pendingRemoval). Deregister sets it; Process removes the
	// stack at the end of its pass.
	PendingRemoval bool
	// RespawnTimer is the ticks until the stack respawns after
	// removal, or <= 0 when it never respawns (TS respawnTimer,
	// default -1). Respawns reports timer > 0.
	RespawnTimer int
	// RespawnAmount is the original recurring-static supply,
	// independent of partial pickups (TS respawnAmount,
	// ItemOnGround.ts:26). The constructor sets it to the initial
	// amount; the respawn task resets a static respawning stack to
	// it.
	RespawnAmount int
	// AgedCycle is the world cycle in which the stack last aged in
	// Process (TS agedCycle, default -1).
	AgedCycle int64
	// OldAmount is the amount before the last merge or partial
	// removal (TS oldAmount). The constructor sets it to the
	// initial amount.
	OldAmount int
	// StaticSpawn marks a static world spawn: it never despawns on
	// its own and is visible to everyone (TS staticSpawn,
	// ItemOnGround.ts:33). Respawning after a pickup still comes
	// from the respawn timer.
	StaticSpawn bool
	// DespawnDelay is the stack's own lifetime in ticks, or nil to
	// keep DespawnDelay (TS despawnDelay: number | null,
	// ItemOnGround.ts:35). See EffectiveDespawnDelay.
	DespawnDelay *int
	// AgingPaused freezes the stack: while set, it neither ages nor
	// goes public (TS agingPaused, ItemOnGround.ts:37).
	AgingPaused bool
	// PickupReplacement is the take-credit override, or nil when
	// the stack carries none (TS pickupReplacement).
	PickupReplacement *GroundPickupReplacement

	// enabledOps is the native ground-action bitmask, bits 0..4
	// (TS enabledOps, default 0x1f, ItemOnGround.ts:14).
	enabledOps int
	// viewerOps holds per-viewer operation masks (TS
	// viewerEnabledOps, a WeakMap keyed by Player). Go has no
	// weak map; entries live until ResetEnabledOps or the item is
	// dropped. Viewers are *Player pointers, as in TS.
	viewerOps map[*Player]int
	// viewerOverrideCount counts viewerOps entries (TS
	// viewerOverrideCount). It is maintained alongside the map so
	// HasSameOperationPolicy need not scan it.
	viewerOverrideCount int

	// manager is the manager the stack is registered with, or nil.
	// It backs refreshEnabledOps: TS reaches the static manager
	// from setEnabledOps, and Go reaches the owning manager
	// through this pointer. Set by Register, cleared on removal.
	manager *GroundItemManager
}

// NewItemOnGround builds a stack, mirroring the TS constructor
// (ItemOnGround.ts:39-53): tick 0, not pending removal, respawn and
// old amounts set to amount, enabled ops 0x1f, aged cycle -1. The
// private-area parameter has no Go form (no areas exist); meta is
// deep-copied, as the TS Item constructor clones it. The id is
// assigned by Register, not here (see the file header).
func NewItemOnGround(state GroundItemState, owner string, pos Location, itemID, amount int, meta map[string]any, goesGlobal bool, respawnTimer int, staticSpawn bool, replacement *GroundPickupReplacement) *ItemOnGround {
	return &ItemOnGround{
		Position:          pos,
		State:             state,
		Owner:             owner,
		ItemID:            itemID,
		Amount:            amount,
		Meta:              cloneMeta(meta),
		GoesGlobal:        goesGlobal,
		RespawnTimer:      respawnTimer,
		RespawnAmount:     amount,
		AgedCycle:         -1,
		OldAmount:         amount,
		StaticSpawn:       staticSpawn,
		PickupReplacement: replacement,
		enabledOps:        0x1f,
	}
}

// ID returns the stack's manager-assigned id (TS getId). It is 0
// until Register assigns it.
func (it *ItemOnGround) ID() int { return it.id }

// Respawns reports whether the stack respawns after removal: timer >
// 0 (ItemOnGround.ts:214-216).
func (it *ItemOnGround) Respawns() bool { return it.RespawnTimer > 0 }

// EffectiveDespawnDelay returns the stack's lifetime in ticks: its
// own delay when set, DespawnDelay otherwise (getDespawnDelay,
// ItemOnGround.ts:218-220).
func (it *ItemOnGround) EffectiveDespawnDelay() int {
	if it.DespawnDelay != nil {
		return *it.DespawnDelay
	}
	return DespawnDelay
}

// SetDespawnDelay sets the stack's own lifetime in ticks
// (setDespawnDelay, ItemOnGround.ts:223-226).
func (it *ItemOnGround) SetDespawnDelay(ticks int) {
	it.DespawnDelay = &ticks
}

// MergeRespawnAmount adds another stack's respawn amount to this
// one, for recurring-static merges (ItemOnGround.ts:211-213).
func (it *ItemOnGround) MergeRespawnAmount(other *ItemOnGround) {
	it.RespawnAmount += other.RespawnAmount
}

// EnabledOps returns the ground-action mask for viewer: the
// per-viewer override when one exists, the stack mask otherwise
// (getEnabledOps, ItemOnGround.ts:112-115). A nil viewer reads the
// stack mask. A stored override of 0 reads back as 0, as in TS (the
// ?? default applies only when no override exists).
func (it *ItemOnGround) EnabledOps(viewer *Player) int {
	if viewer != nil && it.viewerOps != nil {
		if mask, ok := it.viewerOps[viewer]; ok {
			return mask
		}
	}
	return it.enabledOps
}

// ViewerOverrideCount returns the number of per-viewer operation
// overrides (TS viewerOverrideCount, ItemOnGround.ts:16).
func (it *ItemOnGround) ViewerOverrideCount() int { return it.viewerOverrideCount }

// SetEnabledOps sets the ground-action mask: the stack mask when
// viewer is nil, the per-viewer override otherwise (setEnabledOps,
// ItemOnGround.ts:117-126). Masks outside 0..31 are an error and
// change nothing — TS throws a RangeError. A successful set
// refreshes viewers with an ALTER, as TS refreshEnabledOps does
// (see refreshEnabledOps); for an unregistered stack the refresh is
// a no-op.
func (it *ItemOnGround) SetEnabledOps(mask int, viewer *Player) error {
	if mask < 0 || mask > 0x1f {
		return fmt.Errorf("grounditems: ground operation mask must be 0..31, got %d", mask)
	}
	if viewer != nil {
		if it.viewerOps == nil {
			it.viewerOps = make(map[*Player]int)
		}
		if _, ok := it.viewerOps[viewer]; !ok {
			it.viewerOverrideCount++
		}
		it.viewerOps[viewer] = mask
	} else {
		it.enabledOps = mask
	}
	it.refreshEnabledOps(viewer)
	return nil
}

// ResetEnabledOps drops viewer's per-viewer override, restoring the
// stack mask for that viewer (resetEnabledOps,
// ItemOnGround.ts:128-134). Dropping a nonexistent override is a
// no-op with no refresh, as in TS.
func (it *ItemOnGround) ResetEnabledOps(viewer *Player) {
	if viewer == nil || it.viewerOps == nil {
		return
	}
	if _, ok := it.viewerOps[viewer]; ok {
		delete(it.viewerOps, viewer)
		it.viewerOverrideCount--
		it.refreshEnabledOps(viewer)
	}
}

// IsOperationEnabled reports whether ground operation 1..5 is enabled
// for viewer (isOperationEnabled, ItemOnGround.ts:136-139). Only
// operations 1..5 can be enabled; anything else is false.
func (it *ItemOnGround) IsOperationEnabled(viewer *Player, operation int) bool {
	return operation >= 1 && operation <= 5 && (it.EnabledOps(viewer)&(1<<(operation-1))) != 0
}

// HasSameOperationPolicy reports whether two stacks share mergeable
// operation policy (ItemOnGround.ts:141-147): equal stack masks, no
// per-viewer overrides on either stack (viewer policies belong to
// one exact stack/session and must not merge away), and equal pickup
// replacements (id and telegrab denial, absent equal to absent).
func (it *ItemOnGround) HasSameOperationPolicy(other *ItemOnGround) bool {
	return it.enabledOps == other.enabledOps &&
		it.viewerOverrideCount == 0 && other.viewerOverrideCount == 0 &&
		pickupReplacementEqual(it.PickupReplacement, other.PickupReplacement)
}

// refreshEnabledOps re-sends the stack after an operation-mask change
// (ItemOnGround.ts:148-152): nothing unless the stack is registered
// and not pending removal; a viewer-scoped refresh goes to that
// viewer only, a stack-wide refresh fans out through Perform.
func (it *ItemOnGround) refreshEnabledOps(viewer *Player) {
	m := it.manager
	if m == nil || it.PendingRemoval || !m.contains(it) {
		return
	}
	if viewer != nil {
		m.PerformPlayer(viewer, it, GroundItemAlter)
	} else {
		m.Perform(it, GroundItemAlter)
	}
}

// Clone copies the stack for the respawn task (ItemOnGround.ts clone,
// lines 260-267): same state, owner, position, item (deep-copied
// meta), goes-global, respawn timer, static flag, and pickup
// replacement; tick 0, not pending removal, aged cycle -1, respawn
// amount, despawn delay, and stack enabled ops copied. Per-viewer
// overrides are NOT copied (a fresh override map, as in TS), and the
// clone has no id and no manager until Register assigns them. TS
// shares the position object with the original; Go copies the
// Location value, which is equivalent because Location is a value
// type.
func (it *ItemOnGround) Clone() *ItemOnGround {
	c := NewItemOnGround(it.State, it.Owner, it.Position, it.ItemID, it.Amount, it.Meta, it.GoesGlobal, it.RespawnTimer, it.StaticSpawn, cloneReplacement(it.PickupReplacement))
	c.RespawnAmount = it.RespawnAmount
	if it.DespawnDelay != nil {
		delay := *it.DespawnDelay
		c.DespawnDelay = &delay
	}
	c.enabledOps = it.enabledOps
	return c
}

// isUntradeable reports whether the stack's item carries the
// untradeable meta flag: Item.isUntradeable
// (game/model/Item.ts:211-213) reads meta["untradeable"] === true —
// a strict boolean true, so any other value (1, "true") is not
// untradeable.
func (it *ItemOnGround) isUntradeable() bool {
	if it.Meta == nil {
		return false
	}
	v, ok := it.Meta["untradeable"]
	return ok && v == true
}

// cloneReplacement copies a pickup replacement, preserving nil.
func cloneReplacement(r *GroundPickupReplacement) *GroundPickupReplacement {
	if r == nil {
		return nil
	}
	c := *r
	return &c
}

// pickupReplacementEqual compares replacements with TS optional-chain
// semantics (ItemOnGround.ts:145-146): absent equals absent, and a
// present replacement equals only an identical one.
func pickupReplacementEqual(a, b *GroundPickupReplacement) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.TelegrabDenial == b.TelegrabDenial
}

// cloneMeta deep-copies item metadata, preserving nil (Item.cloneMeta,
// game/model/Item.ts:27-50: a non-object meta becomes null, and nested
// objects/arrays are copied recursively).
func cloneMeta(meta map[string]any) map[string]any {
	if meta == nil {
		return nil
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = cloneMetaValue(v)
	}
	return out
}

func cloneMetaValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = cloneMetaValue(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = cloneMetaValue(e)
		}
		return out
	default:
		return v
	}
}

// metaKey is the canonical form merge identity compares: the JSON
// encoding of meta, or "null" for nil meta (TS compares
// JSON.stringify(meta ?? null), ItemOnGroundManager.ts:162). Go's
// encoder sorts map keys, so two metas with the same entries in a
// different insertion order compare equal here where TS stringify
// (insertion order) would differ — a deliberate canonicalization, and
// the only divergence: equal JSON objects merge, unequal ones do not.
// A value that cannot be JSON-encoded falls back to Go formatting;
// meta values must be JSON-encodable for merge identity to be
// meaningful.
func metaKey(meta map[string]any) string {
	b, err := json.Marshal(meta)
	if err != nil {
		return fmt.Sprintf("%#v", meta)
	}
	return string(b)
}

// GroundItemTiming carries the world cycles at which a stack was
// created, goes public, and despawns, per getTiming
// (ItemOnGroundManager.ts:58-70). A 0 private/expires tick means
// "never" (the protocol carries int32s and 0 means unknown).
type GroundItemTiming struct {
	CreatedTick      int64
	PrivateUntilTick int64
	ExpiresTick      int64
}

// GroundItemView is one stack as one viewer sees it: GroundItemView
// (ClientProtocol.ts:260-275) in game-layer form, without the
// protocol import. The adapter converts it to protocol.GroundItemView
// field for field. All timing/owner fields are always resolved here,
// as PacketSender.groundItemView resolves them
// (PacketSender.ts:1461-1476): CreatedTick is the computed cycle,
// PrivateUntilTick/ExpiresTick are 0 when the event never happens,
// OwnerID is -1 unless the viewer owns the stack, and EnabledOps is
// the viewer's resolved mask.
type GroundItemView struct {
	EnabledOps       int
	ID               int
	ItemID           int
	Quantity         int
	X, Y, Level      int
	CreatedTick      int64
	PrivateUntilTick int64
	ExpiresTick      int64
	OwnerID          int
	IsPrivate        bool
	Ownership        int
}

// GroundItemSender carries ground-item deltas from the game layer to
// one player's session. The cmd/server adapter implements it by
// converting views to protocol.GroundItemView and calling the
// session's delta send (the Phase 7 SessionPort pattern: the game
// layer never imports protocol, the session layer never imports
// game). Upserts carry full views (CREATE/ALTER); removes carry
// stack ids (DELETE). Exactly one of the two is non-empty per call,
// matching PacketSender alter/delete/createGroundItem
// (PacketSender.ts:1424-1449), each of which sends one delta packet.
type GroundItemSender interface {
	SendGroundItemDelta(player *Player, upserts []GroundItemView, removes []int)
}

// TaskScheduler schedules one-shot respawn tasks. *TaskManager
// satisfies it; the interface keeps the manager testable without a
// world.
type TaskScheduler interface {
	Schedule(task *Task)
}

var _ TaskScheduler = (*TaskManager)(nil)

// GroundItemManagerConfig wires a GroundItemManager to the world and
// to definitions. Every field is optional; each nil degrades honestly
// as documented, and the coordinator must wire the production seams
// (see NewGroundItemManager).
type GroundItemManagerConfig struct {
	// Players lists the registered players for fan-out, in slot
	// order (TS World.getItems()/forEachNetworkPlayer iteration).
	// Nil means no players: performs send nothing and owner
	// lookups find no one. The coordinator supplies the world's
	// player list.
	Players func() []*Player

	// Sender delivers per-player deltas. Nil means performs are
	// computed and gated but never sent.
	Sender GroundItemSender

	// Scheduler runs respawn tasks. Nil means respawning stacks
	// are removed and never return: the GroundItemRespawnTask has
	// nowhere to run. The coordinator supplies World.Tasks().
	Scheduler TaskScheduler

	// Cycle returns the current world process cycle, for view
	// timing (TS getTiming reads World.getProcessCycle). Nil
	// means the last cycle Process ran (0 before the first tick).
	// The coordinator supplies World.ProcessCycle.
	Cycle func() int64

	// Stackable reports whether an item id stacks, backing the
	// merge attempts in Register and in the reveal branch (TS
	// Item.getDefinition().isStackable(), backed by
	// ItemDefinition.isStackable <- cache ObjType stackability,
	// ItemDefinition.ts:145,220-222). Nil means nothing stacks:
	// no merges ever happen. With no definition source the
	// manager cannot know stackability, and inventing it would
	// merge stacks TS keeps apart.
	Stackable func(itemID int) bool

	// TradeableDropable reports an item id's definition-level
	// tradeable/dropable flags, backing the public-visibility term
	// in CanSee (TS Item.isTradeable/isDropable, backed by
	// ItemDefinition.isTradeable/isDropable <- cache ObjType
	// isTradable and the "Drop" inventory action,
	// ItemDefinition.ts:147-148,224-234, combined with the
	// untradeable meta flag in Item.ts:198-209). Nil means no
	// item is tradeable or dropable, so the term is false and
	// public stacks are visible only to their owner, to everyone
	// when static, or when unowned. The coordinator supplies a
	// definition-backed source.
	TradeableDropable func(itemID int) (tradeable, dropable bool)
}

// GroundItemManager is the Go form of the static ItemOnGroundManager
// (ItemOnGroundManager.ts): the world ground-item list plus the
// register/merge/process/canSee/timing operations over it.
//
// The coordinator must wire, at minimum: Players (world player
// list), Sender (cmd/server adapter), Scheduler (world tasks), and
// Cycle (world process cycle). Stackable and TradeableDropable need
// definition-backed sources for faithful merging and public
// visibility; with nil sources the manager degrades as documented
// on GroundItemManagerConfig, which is unobservable while only
// static spawns exist (static spawns are visible regardless of the
// tradeable term, and duplicate static stacks on one tile are the
// only stacks that could merge).
type GroundItemManager struct {
	items         []*ItemOnGround
	nextID        int
	finishedCycle int64
	lastCycle     int64

	players           func() []*Player
	sender            GroundItemSender
	scheduler         TaskScheduler
	cycle             func() int64
	stackable         func(itemID int) bool
	tradeableDropable func(itemID int) (bool, bool)
}

// NewGroundItemManager returns a manager with ids starting at 1 and
// the finished cycle at -1 (TS: nextId = 1, ItemOnGround.ts:11;
// finishedCycle = -1, ItemOnGroundManager.ts:32).
func NewGroundItemManager(cfg GroundItemManagerConfig) *GroundItemManager {
	return &GroundItemManager{
		nextID:            1,
		finishedCycle:     -1,
		players:           cfg.Players,
		sender:            cfg.Sender,
		scheduler:         cfg.Scheduler,
		cycle:             cfg.Cycle,
		stackable:         cfg.Stackable,
		tradeableDropable: cfg.TradeableDropable,
	}
}

// nowCycle returns the cycle view timing reads: the injected cycle
// source when set, the last Process cycle otherwise.
func (m *GroundItemManager) nowCycle() int64 {
	if m.cycle != nil {
		return m.cycle()
	}
	return m.lastCycle
}

// isStackable reports whether an item id stacks, degrading to false
// with no definition source (see GroundItemManagerConfig.Stackable).
func (m *GroundItemManager) isStackable(itemID int) bool {
	return m.stackable != nil && m.stackable(itemID)
}

// contains reports whether item is currently in the list (TS
// World.getItems().includes).
func (m *GroundItemManager) contains(item *ItemOnGround) bool {
	for _, it := range m.items {
		if it == item {
			return true
		}
	}
	return false
}

// removeItem splices item out of the list and clears its manager
// pointer. TS removes by indexOf + splice (ItemOnGroundManager.ts:49
// -51); a missing item would make TS splice(-1, 1) and drop the last
// element instead — unreachable in TS because every processed item
// came from the list, and guarded here (a missing item is a no-op)
// because a duplicate-registered pointer would otherwise corrupt the
// list on its second pass.
func (m *GroundItemManager) removeItem(item *ItemOnGround) {
	for i, it := range m.items {
		if it == item {
			m.items = append(m.items[:i], m.items[i+1:]...)
			item.manager = nil
			return
		}
	}
}

// Register adds a stack to the world (ItemOnGroundManager.ts:121-135).
// The private-area despawn-delay step has no Go form: no areas exist,
// so no area delay ever applies and an explicitly set delay is never
// overwritten. A stackable stack first attempts a merge; a merged
// stack is never appended and never assigned an id. Otherwise the
// stack is appended, assigned the next id (when it has none), and a
// CREATE is performed. A nil item is ignored, matching the nil guards
// on the world's queue methods.
func (m *GroundItemManager) Register(item *ItemOnGround) {
	if item == nil {
		return
	}
	if m.isStackable(item.ItemID) && m.Merge(item) {
		return
	}
	if item.id == 0 {
		item.id = m.nextID
		m.nextID++
	}
	item.manager = m
	m.items = append(m.items, item)
	m.Perform(item, GroundItemCreate)
}

// Deregister marks a stack for removal and performs a DELETE
// (ItemOnGroundManager.ts:193-196). Process removes the stack from
// the list on its pass; the DELETE reaches viewers that can see the
// stack including pending removal (see PerformPlayer).
func (m *GroundItemManager) Deregister(item *ItemOnGround) {
	if item == nil {
		return
	}
	item.PendingRemoval = true
	m.Perform(item, GroundItemDelete)
}

// Items returns the live stacks in registration order (the Go form
// of World.getItems(), which the pickup listener's operable-stack
// lookup scans in ItemOnGroundManager.ts:150-194). The returned
// slice is a snapshot copy: mutating it never affects the
// manager's list. The stacks themselves are shared: callers must
// not mutate them except through the pickup transfer path
// (OldAmount/Amount on a partial pickup, exactly as TS mutates
// the stack item). Tick-goroutine-only, like the rest of the
// manager. A nil manager yields no stacks.
func (m *GroundItemManager) Items() []*ItemOnGround {
	if m == nil {
		return nil
	}
	out := make([]*ItemOnGround, len(m.items))
	copy(out, m.items)
	return out
}

// Merge folds item into an existing stack on the same tile
// (ItemOnGroundManager.ts:137-191). It reports whether the merge
// happened; on success the existing stack grows, keeps its lifetime
// (tick reset to 0), and viewers get an ALTER, while item itself is
// left for the caller to drop (Register returns without appending;
// the reveal branch marks it pending removal).
//
// The candidate must match on: same tile (x, y, z —
// Location.isSameTile, Location.ts:52-56), same area (degenerate:
// no areas), same effective despawn delay (a stack with its own
// lifetime keeps it rather than joining a shorter-lived one),
// same operation policy, same owner (stacks from different owners
// never merge, private or public), same item id, and equal meta.
// Recurring static supply (a static spawn that respawns) must not
// absorb drops or a different respawn lifecycle: when either side
// is recurring, both must be, with equal respawn timers, states,
// goes-global flags, and aging-paused flags, and the combined
// respawn amount must fit the stack cap. The merged amount must
// fit MaxStackAmount (exactly the cap is allowed: TS continues
// only when the sum is greater).
func (m *GroundItemManager) Merge(item *ItemOnGround) bool {
	if item == nil {
		return false
	}
	for _, existing := range m.items {
		if existing == nil || existing.PendingRemoval || existing == item {
			continue
		}
		if !existing.Position.Equals(item.Position) {
			continue
		}
		// Private areas: no areas exist, so every stack shares
		// the nil area and the TS area-identity check always
		// passes.
		if existing.EffectiveDespawnDelay() != item.EffectiveDespawnDelay() {
			continue
		}
		if !existing.HasSameOperationPolicy(item) {
			continue
		}
		if existing.Owner != item.Owner {
			continue
		}
		if existing.ItemID != item.ItemID {
			continue
		}
		if metaKey(existing.Meta) != metaKey(item.Meta) {
			continue
		}
		recurring := existing.StaticSpawn && existing.Respawns()
		incomingRecurring := item.StaticSpawn && item.Respawns()
		if recurring || incomingRecurring {
			if !recurring || !incomingRecurring ||
				existing.RespawnTimer != item.RespawnTimer ||
				existing.State != item.State ||
				existing.GoesGlobal != item.GoesGlobal ||
				existing.AgingPaused != item.AgingPaused ||
				existing.RespawnAmount+item.RespawnAmount > MaxStackAmount {
				continue
			}
		}
		oldAmount := existing.Amount
		if oldAmount+item.Amount > MaxStackAmount {
			continue
		}
		existing.Amount += item.Amount
		if recurring {
			existing.MergeRespawnAmount(item)
		}
		existing.OldAmount = oldAmount
		existing.Tick = 0
		m.Perform(existing, GroundItemAlter)
		return true
	}
	return false
}

// Process runs one ground-item pass for cycle (ItemOnGroundManager
// .process, ItemOnGroundManager.ts:40-53; ItemOnGround.process,
// ItemOnGround.ts:55-103). It iterates a COPY of the list —
// removing from the live list mid-loop would skip the next item's
// tick — ages each stack, removes stacks pending removal
// (scheduling a respawn task first when the stack respawns), and
// records the finished cycle AFTER the loop, so getTiming's
// ages-this-cycle rule sees the pass as unfinished until every
// stack has aged.
func (m *GroundItemManager) Process(cycle int64) {
	m.lastCycle = cycle
	snapshot := append([]*ItemOnGround(nil), m.items...)
	for _, item := range snapshot {
		m.processItem(item, cycle)
		if item.PendingRemoval {
			if item.Respawns() {
				m.scheduleRespawn(item)
			}
			m.removeItem(item)
		}
	}
	m.finishedCycle = cycle
}

// processItem ages one stack (ItemOnGround.process,
// ItemOnGround.ts:55-103): the aged cycle is stamped even for a
// paused stack, then a paused stack returns without aging. A
// private stack that goes global reveals at PublicRevealDelay: the
// owner gets a DELETE first, a stackable stack attempts a merge
// (on success it is marked pending removal and returns without
// going public), and otherwise it becomes public with a CREATE and
// returns before the despawn check. A stack whose tick reaches its
// despawn delay is deregistered — unless it respawns or is a
// static spawn, which persist until picked up.
func (m *GroundItemManager) processItem(item *ItemOnGround, cycle int64) {
	item.AgedCycle = cycle
	if item.AgingPaused {
		return
	}
	item.Tick++
	switch item.State {
	case GroundItemSeenByEveryone, GroundItemSeenByPlayer:
		if item.State == GroundItemSeenByPlayer && item.GoesGlobal && item.Tick >= PublicRevealDelay {
			if item.Owner != "" {
				if owner := m.playerByName(item.Owner); owner != nil {
					m.PerformPlayer(owner, item, GroundItemDelete)
				}
			}
			if m.isStackable(item.ItemID) {
				if m.Merge(item) {
					item.PendingRemoval = true
					return
				}
			}
			item.State = GroundItemSeenByEveryone
			m.Perform(item, GroundItemCreate)
			return
		}
		if item.Tick >= item.EffectiveDespawnDelay() && !item.Respawns() && !item.StaticSpawn {
			m.Deregister(item)
		}
	}
}

// scheduleRespawn submits the respawn task for a removed stack
// (ItemOnGroundManager.ts:45-47; GroundItemRespawnTask.ts:12-26).
// The task is a one-shot NewTask with the stack's respawn timer as
// its delay: the Go TaskManager's delay semantics (Task.ts tick:
// countdown reaches 0, then reset; a task whose Action returns done
// stops) express the TS task exactly — execute once after the
// delay, then stop. The task clones at execution time (not at
// schedule time, as in TS), resets the amount to the respawn
// amount for a static respawning stack, and registers the clone.
// With no scheduler the stack never returns (see
// GroundItemManagerConfig.Scheduler).
func (m *GroundItemManager) scheduleRespawn(item *ItemOnGround) {
	if m.scheduler == nil {
		return
	}
	task := NewTask(item.RespawnTimer, func(cycle int64) (done bool) {
		restored := item.Clone()
		if restored.StaticSpawn && restored.Respawns() {
			restored.Amount = restored.RespawnAmount
		}
		m.Register(restored)
		return true
	})
	task.Name = "GroundItemRespawnTask"
	m.scheduler.Schedule(task)
}

// GetTiming returns the cycles at which item was created, goes
// public (0 if never), and despawns (0 if never)
// (ItemOnGroundManager.ts:58-70). cycle is the current world
// process cycle. A stack that has not aged this cycle — and whose
// pass has not finished this cycle, e.g. a drop made by a task
// after the ground-item phase — still ages once more, so its
// creation reads one cycle earlier. Private/expires ticks past
// the int32 range wrap to 0 (the protocol carries int32s and 0
// means unknown); the created tick is returned raw, as in TS.
func (m *GroundItemManager) GetTiming(item *ItemOnGround, cycle int64) GroundItemTiming {
	if item == nil {
		return GroundItemTiming{}
	}
	agesThisCycle := item.AgedCycle != cycle && m.finishedCycle != cycle
	createdTick := cycle - int64(item.Tick)
	if agesThisCycle {
		createdTick--
	}
	wrap := func(value int64) int64 {
		if value <= 0x7fffffff {
			return value
		}
		return 0
	}
	timing := GroundItemTiming{CreatedTick: createdTick}
	if item.State == GroundItemSeenByPlayer && item.GoesGlobal {
		timing.PrivateUntilTick = wrap(createdTick + PublicRevealDelay)
	}
	if !item.Respawns() && !item.StaticSpawn {
		timing.ExpiresTick = wrap(createdTick + int64(item.EffectiveDespawnDelay()))
	}
	return timing
}

// CanSee reports whether player can see item (ItemOnGroundManager
// .canSee, ItemOnGroundManager.ts:448-457): the stack must be in
// the player's area (degenerate: no areas, always true) and within
// CrossFloor range 64; a private stack is visible only to its
// owner; a public stack is visible to its owner, to everyone when
// it is a static spawn or unowned, or when the item is tradeable
// AND dropable. Pending-removal stacks are invisible unless
// includePendingRemoval is set (DELETE fan-out passes true, so a
// removed stack still reaches viewers that saw it).
func (m *GroundItemManager) CanSee(player *Player, item *ItemOnGround, includePendingRemoval bool) bool {
	if player == nil || item == nil || (!includePendingRemoval && item.PendingRemoval) {
		return false
	}
	if !crossFloorVisible(player.Location(), item.Position, GroundItemViewRange) {
		return false
	}
	if item.State == GroundItemSeenByPlayer {
		return isOwner(player.Username, item)
	}
	return isOwner(player.Username, item) ||
		item.StaticSpawn ||
		item.Owner == "" ||
		m.tradeableAndDropable(item)
}

// tradeableAndDropable evaluates the public-visibility term: the
// item is tradeable and dropable (Item.isTradeable/isDropable,
// game/model/Item.ts:198-209), each requiring the definition flag
// AND the absence of the untradeable meta flag. With no definition
// source the term is false (see GroundItemManagerConfig
// .TradeableDropable).
func (m *GroundItemManager) tradeableAndDropable(item *ItemOnGround) bool {
	if m.tradeableDropable == nil || item.isUntradeable() {
		return false
	}
	tradeable, dropable := m.tradeableDropable(item.ItemID)
	return tradeable && dropable
}

// VisibleViews builds the viewer-specific views of every stack
// viewer can see, in list order (ItemOnGroundManager.onRegionChange
// filters World.getItems() by canSee, ItemOnGroundManager.ts:34-38;
// the per-viewer shaping is PacketSender.groundItemView,
// PacketSender.ts:1461-1476). The coordinator's FlushTick
// region-change call reads the visible stacks through this method
// (via the per-session source closure); cycle is the current world
// process cycle, for view timing. A nil viewer sees nothing.
func (m *GroundItemManager) VisibleViews(viewer *Player, cycle int64) []GroundItemView {
	if viewer == nil {
		return nil
	}
	var views []GroundItemView
	for _, item := range m.items {
		if m.CanSee(viewer, item, false) {
			views = append(views, m.viewFor(viewer, item, cycle))
		}
	}
	return views
}

// viewFor shapes one stack for one viewer (PacketSender
// .groundItemView, PacketSender.ts:1461-1476). Ownership uses
// STRICT username equality (owner === player username), not the
// protocol-form comparison isOwner uses: the owner id is the
// viewer's index only for an exact match. isPrivate is the state
// test (state === 0, SEEN_BY_PLAYER); ownership is 0 for an
// unowned stack, 1 for the viewer's own, 2 for another's.
func (m *GroundItemManager) viewFor(viewer *Player, item *ItemOnGround, cycle int64) GroundItemView {
	timing := m.GetTiming(item, cycle)
	mine := item.Owner != "" && viewer != nil && item.Owner == viewer.Username
	ownership := GroundItemOwnershipNone
	if item.Owner != "" {
		if mine {
			ownership = GroundItemOwnershipMine
		} else {
			ownership = GroundItemOwnershipOther
		}
	}
	ownerID := -1
	if mine {
		ownerID = viewer.Index
	}
	return GroundItemView{
		EnabledOps:       item.EnabledOps(viewer),
		ID:               item.id,
		ItemID:           item.ItemID,
		Quantity:         item.Amount,
		X:                item.Position.X,
		Y:                item.Position.Y,
		Level:            item.Position.Z,
		CreatedTick:      timing.CreatedTick,
		PrivateUntilTick: timing.PrivateUntilTick,
		ExpiresTick:      timing.ExpiresTick,
		OwnerID:          ownerID,
		IsPrivate:        item.State == GroundItemSeenByPlayer,
		Ownership:        ownership,
	}
}

// Perform fans an operation out by stack state (ItemOnGroundManager
// .perform, ItemOnGroundManager.ts:72-87): a private stack goes to
// its owner only (when the owner is online), a public stack goes to
// every network player. Any other state sends nothing.
func (m *GroundItemManager) Perform(item *ItemOnGround, op GroundItemOp) {
	if item == nil {
		return
	}
	switch item.State {
	case GroundItemSeenByPlayer:
		if owner := m.playerByName(item.Owner); owner != nil {
			m.PerformPlayer(owner, item, op)
		}
	case GroundItemSeenByEveryone:
		for _, player := range m.allPlayers() {
			m.PerformPlayer(player, item, op)
		}
	}
}

// PerformPlayer sends one operation to one player
// (ItemOnGroundManager.performPlayer, ItemOnGroundManager.ts:89-119).
// A pending-removal stack always sends DELETE, whatever operation
// was requested. Bots are skipped — TS skips only unwatched bots
// (isUnwatchedBot ?? isPlayerBot, Player.ts:1538-1540), but Go has
// no watched/unwatched distinction, so ALL bots are skipped, the
// same documented deviation as the Phase 7 destination flag.
// Disconnected sessions are skipped (TS isPlayerSessionConnected,
// World.ts:140-156; the Go gate is SessionPort.IsOpen). The
// visibility gate passes includePendingRemoval for DELETE, so a
// removed stack reaches viewers that saw it. CREATE/ALTER send one
// upsert delta, DELETE sends one remove delta; an unknown operation
// is logged and dropped, never crashing the tick (TS console.warn).
func (m *GroundItemManager) PerformPlayer(player *Player, item *ItemOnGround, op GroundItemOp) {
	if player == nil || item == nil {
		return
	}
	if item.PendingRemoval {
		op = GroundItemDelete
	}
	if player.IsBot {
		return
	}
	if player.Session == nil || !player.Session.IsOpen() {
		return
	}
	if !m.CanSee(player, item, op == GroundItemDelete) {
		return
	}
	if m.sender == nil {
		return
	}
	switch op {
	case GroundItemAlter, GroundItemCreate:
		m.sender.SendGroundItemDelta(player, []GroundItemView{m.viewFor(player, item, m.nowCycle())}, nil)
	case GroundItemDelete:
		m.sender.SendGroundItemDelta(player, nil, []int{item.id})
	default:
		log.Printf("[grounditems] unsupported operation (%d) on ground item id=%d item=%d amount=%d", op, item.id, item.ItemID, item.Amount)
	}
}

// allPlayers returns the registered players, or none when no player
// source is wired.
func (m *GroundItemManager) allPlayers() []*Player {
	if m.players == nil {
		return nil
	}
	return m.players()
}

// playerByName finds a registered player by username, normalizing
// both sides as World.getPlayerByName does (World.ts:135-138): the
// needle is formatNameForProtocol(formatDisplayName(username)) and
// each candidate is formatNameForProtocol(username). World
// .PlayerByName compares exactly and is deliberately NOT used here:
// TS ground-item owner lookup normalizes, and an owner stored with
// spaces must still match its player. The first match in slot order
// wins, as in TS search.
func (m *GroundItemManager) playerByName(username string) *Player {
	needle := formatNameForProtocol(formatDisplayName(username))
	for _, p := range m.allPlayers() {
		if p != nil && formatNameForProtocol(p.Username) == needle {
			return p
		}
	}
	return nil
}

// isOwner reports whether username owns item: a non-empty owner whose
// protocol form matches the username's protocol form
// (ItemOnGroundManager.isOwner, ItemOnGroundManager.ts:443-446).
func isOwner(username string, item *ItemOnGround) bool {
	return item.Owner != "" && formatNameForProtocol(item.Owner) == formatNameForProtocol(username)
}

// crossFloorVisible lives in visibility.go: with no CrossFloor policy
// registered it is same-plane plus Chebyshev distance
// (CrossFloor.ts:22-45), which is exactly what canSee needs. It is
// reused here rather than re-ported.

// formatNameForProtocol ports Misc.formatNameForProtocol
// (util/Misc.ts:801-803): lowercase, spaces as underscores. The game
// package cannot import session (import cycle), so the two-line
// function is ported locally; session.FormatNameForProtocol is the
// canonical copy and the two must stay identical.
func formatNameForProtocol(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

// formatDisplayName ports Misc.formatDisplayName
// (util/Misc.ts:806-808): trimmed, underscores as spaces. Local port
// for the same reason as formatNameForProtocol; see
// session.FormatDisplayName.
func formatDisplayName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "_", " ")
}

// ApplySpawn registers one loaded ground-item spawn, per
// toItemOnGround (GroundItemSpawnDefinitionLoader.ts:96-129): seen
// by everyone, no owner, never goes global, respawn timer
// respawn > 0 ? respawn : -1, static spawn, with the entry's pickup
// replacement. The members-skip and cache-support checks stay in
// the already-ported loader (loadGroundItems,
// internal/definition/grounditems.go): Store.GroundItems returns
// only validated, supported, world-eligible records, so ApplySpawn
// does not re-validate. Registration may merge the spawn into an
// identical stack (see Register); the returned item is the
// constructed stack, whether or not it was appended.
func (m *GroundItemManager) ApplySpawn(rec definition.GroundItem) *ItemOnGround {
	timer := -1
	if rec.Spawn.Respawn > 0 {
		timer = rec.Spawn.Respawn
	}
	var replacement *GroundPickupReplacement
	if rec.PickupReplacement != nil {
		replacement = &GroundPickupReplacement{
			ID:             rec.PickupReplacement.ID,
			TelegrabDenial: rec.PickupReplacement.TelegrabDenial,
		}
	}
	item := NewItemOnGround(
		GroundItemSeenByEveryone,
		"",
		Location{X: rec.Spawn.X, Y: rec.Spawn.Y, Z: rec.Spawn.Z},
		rec.Spawn.ID,
		rec.Spawn.Amount,
		nil,
		false,
		timer,
		true,
		replacement,
	)
	m.Register(item)
	return item
}
