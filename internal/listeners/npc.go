// NPCOptionPacketListener: npc_option (opcodes 76, 12, 34, 70, 57)
// + the dialogue_continue resume path (opcodes 62, 252), porting
// NPCOptionPacketListener.executeOption (lines 19-96), the
// operate/dispatch pair (lines 106-172), and the dialogue_continue
// dispatch case (NetworkBuilder.ts:908-951) plus the widget_action
// dialogue-option tail (NetworkBuilder.ts:1140-1143).
//
// Opcode map (verified against the TS listener + the Go decode
// table): 76 decodes as option 1, 12 as option 2, 34 as option 3,
// 70 as option 4, and 57 — OPNPC1 in the native enum — as option 5
// (ClientProtocol.ts lines 760-779; DecodeNpcOption). Opcode 50
// (OPNPC5) has no decode branch and stays unrecognized. Opcode 75
// (OPNPC_T, spell_on_npc) is NPCOptionPacketListener.castSpell and
// stays a decode-drop: the spell front-end (spell-id resolution,
// CombatSpells, rune/level validation) is NOT STARTED, even though
// TS core routes combat spells through Combat.castSpellOn — see
// the spell-on-player boundary note in listeners.go. Opcode 36
// (OPNPC_U, item_on_npc) belongs to UseItemPacketListener and is
// untouched here. Examine (opcode 9, EXAMINE_NPC) was ported in
// phase 12 (examine.go) and is not duplicated.
//
// Dispatch order, TS order. executeOption gates, resolves the NPC
// and its definition, refuses combat options before walking, fires
// the route event, and walks to the NPC with walkToEntity; the
// arrival closure re-checks the definition/option and calls
// operate, which calls dispatch and emits npc:operated only when
// dispatch claims the click. dispatch runs, in order:
// NpcInteractionManager.handle (the static-interaction bindings —
// the core.shops.open branch, the core.player.teleport branch, the
// core.dialogues.start branch, and plugin-method bindings),
// the plugin NPC interaction emit (before hooks, then main hooks,
// then any-NPC hooks — the Talk-to transcript lives in the
// any-hooks), the bank branch, and finally "Nothing interesting
// happens." with no operated emit. The shop branch is ported here
// (see npcOperate): a core.shops.open action opens through the
// OpenShop seam, a failed open falls through to the interaction
// emit, and a gated shop (the seam returns true with no interface
// opened, as ShopManager.open does when emitCanShop answers
// false) claims the click. The teleport branch is NOT STARTED and
// falls through as unhandled; plugin-method bindings are NOT
// STARTED (registerNpcInteraction stays table-only, and the
// pluginMethods registry has no Go form).
//
// Go shape, following the phase 13 object pattern: the NPC lookup
// (NpcLookup) and the definition source (NpcDefinitionSource) are
// Deps fields; hub emits arrive as func fields on NpcHooks, so
// listeners never import the plugins package; the dialogue start
// is the NpcHooks.StartDialogue seam (see below). When the lookup
// or the definition source is unwired, the packet drops with
// ReasonNpcOption.
//
// The StartDialogue seam covers TWO TS positions, because both
// are owned by the dialogue runtime (phase 14, Worker B) and the
// coordinator wires them together: the NpcInteractionManager
// position (a static core.dialogues.start binding for this
// npcId/clickType) and the transcript position (the Talk-to
// transcript the NpcDialogues any-handler plays for the
// "Talk-to" option). The seam returns whether a dialogue started;
// false falls through to the interaction emit, exactly as a
// missing binding falls through in TS. ORDERING CAVEAT: in TS the
// transcript runs inside emitNpcInteraction, AFTER specific
// plugin handlers, while the static binding runs BEFORE them.
// No NPC interaction hook family is ported yet
// (onNpcInteraction/onBeforeNpcInteraction/onAnyNpcInteraction
// stay table-only), so no specific handler exists and the order
// is unobservable today. When those families land, the transcript
// call must move into the emit (any-hooks last) and StartDialogue
// must narrow to static bindings only — the seam's adapter, not
// this file, owns that split.
//
// Walk-to: walkToEntity (MovementQueue.ts:1478-1589) is NOT
// STARTED in Go — no NPC interaction targets exist in the queue
// (movement_queue.go) — and this package cannot add it. The
// walk reuses WalkToTile with the ReachedObj predicate (the
// phase 9 ground-item shape): the player routes near the NPC's
// tile and arrives when on it or orthogonally adjacent with no
// wall between. DEVIATIONS from walkToEntity, all at this site:
// the NPC footprint size is ignored (a 1x1 reach at the NPC's
// origin tile, so a large NPC's far corner does not reach); there
// is no pursuit re-pathing (NPCs are static, milestone 6, so the
// target never moves); the range>1 projectile-clip term of the
// reached test is not evaluated (no clip access from listeners);
// the frozen-player counter/nearest-tile branches do not exist
// (no freeze system); and the exact-tile-match shortcut in
// WalkToTile runs the arrival before any mobility check (the
// queue's own shape, shared with pickup). The arrival closure
// re-validates the target the way the walkToEntity task does
// (same NPC id still standing on the clicked tile) and re-reads
// the definition/option, returning silently on a combat option,
// exactly as the TS arrival closure does.
//
// Gates (executeOption lines 28-61), ported only as TS core
// enforces them: the player must exist, have hitpoints above 0,
// and not be busy() — busy() degenerates to the hitpoints term,
// as for objects (see objectaction.go); the NPC slot must hold
// an NPC; the NPC must be within 24 tiles on the same plane
// (CrossFloor.visible with no cross-floor policy is the
// same-plane Chebyshev test, Location.IsWithinDistance); and the
// planes must match (kept at its TS position after the combat
// check, redundant with the visibility test in Go). The owner
// gate, the private-area gate, and the whole boat/other-ship
// branch (operateAcross, OTHER_SHIP_REACH_TILES = 16) degenerate
// to pass: no pets/owners, private areas, or boats exist in Go.
// Facing (setPositionToFace) and the mobile interaction
// (setMobileInteraction, including the post-dialogue NPC turn)
// are GAPS: no position-to-face/flag machinery exists, as for
// objects. The combat branch attacks before walking: a combat
// option ("attack", the only combat option —
// NpcDefinition.combatOptions starts as {"attack"} and
// addCombatOption is not ported) on a non-attackable NPC, or on
// an NPC at hitpoints 0 or below, returns silently, as TS does;
// otherwise the emitNpcAttackOption gate runs (a claim returns
// silently, without attacking), and the attack starts through
// the combat seam (Deps.CombatAttack), which performs the
// autocast-conditional clear first (when an autocast spell is
// set, the single-cast spell is cleared, line 62).
// The NPC hitpoints term IS enforced: every production NPC gets
// hitpoints above 0 from the spawn path (SpawnNPCsWithHitpoints,
// default 10), so hitpoints at or below 0 in the world means
// dead/dying, matching TS; a hand-built NPC without hitpoints
// refuses, which is the TS reading of hitpoints <= 0. No
// distance/plane/multi-combat check beyond the 24-tile visibility
// test belongs in the combat branch: the plane check sits after
// it in TS, and distance/multi-combat/permission live in
// CombatFactory (Worker 2), which the combat process consults
// every tick. The approach is the combat system's, not a walk:
// Combat.attack resets the movement queue and the combat process
// routes the attacker in, so the combat branch never walks.
//
// The WORLD_INTERACTIONS preamble (NetworkBuilder.ts:680-693)
// runs in Handle before routing (worldInputPreamble): the
// animation-stall hold is a gap (no stalled-input queue), the
// TRADING skip and the player:world-input emit run, and
// closeInterruptibleInterfaces runs unless the packet is
// skipped. The INTERACTION_CLICKS preamble
// (NetworkBuilder.ts:713-719) IS ported: npc_option and
// spell_on_npc are both in INTERACTION_CLICKS, so the click-run
// override is set before routing, even when routing then
// refuses the packet.
//
// The bank branch is a GAP, as for objects: when no dialogue and
// no plugin handles the interaction and the option is "bank", TS
// opens the bank and the click counts as operated. No interface
// runtime, player status, or bank-open state exists in Go
// (milestone 8), so the bank option stays silent: no open, no
// operated emit, no message.
//
// The no-handler fallback is the TS message, not silence (unlike
// objects): dispatch sends "Nothing interesting happens." and
// returns false, so no operated emit follows. That message is
// implemented here; it is the only message this listener sends.
//
// Resume path: dialogue_continue (RESUME_PAUSEBUTTON, opcodes 62
// native / 252 high-level) reaches DialogueManager.handleResume
// through the dispatch case (NetworkBuilder.ts:908-951), after
// two pre-branches: the creation-menu branch degenerates away
// (CreationMenu NOT STARTED), and the interface-action-click
// emit runs first, ungated (routeDialogueResume calls
// emitInterfaceActionClick with the resume widget/child before
// DialogueAccess.Resume; the prompt check inside the hub emit
// precedes the hooks, PluginManager.ts:2089). The coordinator's
// DialogueAccess.Resume adapter re-checks the pending
// MultiChatboxPrompt before handleResume; the re-check is
// redundant after the emit and harmless (same registry, same
// inputs, idempotent prune). handleResume's own gating — the
// OptionDialogue widget/child check (OPTIONS_WIDGET_ID =
// (219 << 16) | 1, child 1..options) and the canContinue
// chatbox-interface check — lives in the DialogueManager and is
// passed through untouched: this file only routes the packet to
// the DialogueAccess seam.
// dialogue_amount (192) and dialogue_input (193/194) do NOT
// resume: they answer amount/syntax prompts (prompts.go). Option selection ALSO arrives through
// widget_action: the dispatch tail (NetworkBuilder.ts:1140-1143)
// calls handleOption(buttonNum - 1) when a dialogue is active,
// the current entry is the one open before the click's branches
// ran, and the button number is 1..5. That tail is ported in
// routeWidgetAction (listeners.go) through the same seam; the
// "opened by the click itself" guard is the Current() identity
// comparison. A nil DialogueAccess degrades the resume packet to
// ReasonDialogueUnwired and the widget tail to a no-op.
//
// Seams: the coordinator wires NpcLookup over the World's NPC
// list, NpcDefinitionSource over the definition Store
// (*definition.NpcDefinition satisfies NpcDefinition
// structurally), NpcHooks over the hub (operated + attack-option
// gate — the route/interaction emits have no hub families yet),
// the StartDialogue adapter over Worker B's playback entry point,
// the Interactions adapter over the definition Store's
// Interaction lookup (see NpcInteractionSource), the OpenShop
// adapter over the ShopManager open (see NpcHooks.OpenShop),
// DialogueAccess over Player.GetDialogueManager(), and
// Deps.CombatAttack over the combat contract (see listeners.go).
// The playback-entry seam name is NpcHooks.StartDialogue; the
// dialogue-runtime seam name is Deps.Dialogue (DialogueAccess).

package listeners

import (
	"math"
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// NpcLookup resolves an NPC by world index: the Go form of
// World.getNpcs().get(index) (NPCOptionPacketListener.ts:35). A
// miss (empty slot, out-of-range index) returns nil. The
// coordinator implements it over World.NPCs().
type NpcLookup interface {
	Find(index int) *game.NPC
}

// NpcDefinition is the listener-facing view of an NPC
// definition: the resolved (current) definition's identity,
// name, menu actions, footprint size, and attackability. The
// definition package's NpcDefinition satisfies this interface
// structurally; listeners never import that package.
type NpcDefinition interface {
	ID() int
	Name() string
	Actions() []string
	Size() int
	IsAttackable() bool
}

// NpcDefinitionSource resolves the definition a player sees for
// an NPC id: NPC.getCurrentDefinition(player)
// (NPCOptionPacketListener.ts:55, 78), including the
// transform/variant rule. It reports absent when no definition
// resolves; the listener then treats the option as absent and
// proceeds, as TS does with a null definition. The coordinator
// implements it over the definition Store. TRANSFORM GAP: Go
// has no NPC transform/var resolution, so the source answers
// the base definition; a transform NPC reads its base actions.
type NpcDefinitionSource interface {
	NpcForPlayer(id int, player *game.Player) (NpcDefinition, bool)
}

// NpcInteractionAction is the listener-facing form of one
// static-interaction action: the method to invoke and its decoded
// arguments (NpcInteractionActionDefinition,
// NpcInteractionDefinition.ts:1-4). Args holds decoded JSON values
// (numbers are float64) except for the synthesized shorthand
// actions, whose ids are ints — exactly as the definition
// package's NpcInteractionAction carries them. The coordinator's
// adapter passes Method and Args through verbatim; this file
// applies the TS Number()/integer reading of the shop id at the
// use site, as NpcInteractionManager.handle does.
type NpcInteractionAction struct {
	Method string
	Args   map[string]any
}

// NpcInteractionSource resolves the static-interaction action for
// one NPC id and click type: NpcInteractionDefinition.forNpcId
// (npcId)?.getAction(clickType)
// (NpcInteractionManager.ts, handle). The npcID is the CONTENT id
// (npc.getContentId(player)); for Go NPCs without transforms it
// equals the NPC's definition id, as for NpcAttackOptionEvent. It
// reports absent when no definition or no action exists for the
// click; the listener then proceeds as if no static binding
// existed, as TS does when handle finds no action. The coordinator
// implements it over the definition Store. A nil source degrades
// the same way: the shop step is skipped and dispatch proceeds
// to the dialogue seam, preserving the pre-shop behavior.
type NpcInteractionSource interface {
	InteractionAction(npcID, clickType int) (NpcInteractionAction, bool)
}

// NpcRouteRequest is the listener-facing form of the
// PluginNpcRouteEvent (PluginTypes.ts:169-177): the walk-to
// path's view of the NPC being approached. Range starts at 1
// and IgnoreProjectileClip at false (NPCOptionPacketListener
// .ts:67-71); route hooks may raise the range (clamped to
// 1..24 by emitNpcRoute, PluginManager.ts:1235-1243) or set
// IgnoreProjectileClip. Location is the NPC's tile at click
// time. ItemID (the item-on-NPC route field) has no click-path
// form and is omitted.
type NpcRouteRequest struct {
	Player               *game.Player
	NPC                  *game.NPC
	Definition           NpcDefinition
	NpcID                int
	NpcIndex             int
	ClickType            int
	Location             game.Location
	Range                int
	IgnoreProjectileClip bool
}

// NpcInteractionEvent is the listener-facing form of the
// PluginNpcInteractionEvent (PluginTypes.ts:160-168). Location
// is the NPC's tile. Handled is the claim flag: a hook (or the
// emit adapter) sets it to claim the interaction. The
// before/main/any split lives inside the hub emit
// (emitNpcInteraction runs the three lists in order,
// PluginManager.ts:1246-1263); the listener calls one seam.
type NpcInteractionEvent struct {
	Player     *game.Player
	NPC        *game.NPC
	Definition NpcDefinition
	NpcID      int
	NpcIndex   int
	ClickType  int
	Location   game.Location
	Handled    bool
}

// NpcDialogueRequest is the dialogue-start seam's request: the
// click that may start a dialogue, at the
// NpcInteractionManager.handle position in dispatch (see the
// file header for the two TS positions this seam covers).
// Option is the lower-cased menu text for the click type, when
// the slot holds text (HasOption); Definition is the freshly
// resolved definition at arrival time, or nil when none
// resolved. Location is the NPC's tile.
type NpcDialogueRequest struct {
	Player     *game.Player
	NPC        *game.NPC
	Definition NpcDefinition
	NpcID      int
	NpcIndex   int
	ClickType  int
	Option     string
	HasOption  bool
	Location   game.Location
}

// NpcOperatedEvent is the listener-facing form of the
// npc:operated payload (NPCOptionPacketListener.ts:106-112,
// OPERATED_EVENT): a click something answered — a dialogue
// start or a claimed plugin interaction. Option is the
// lower-cased option text; it may be absent (HasOption false)
// when the clicked slot carries no action text.
type NpcOperatedEvent struct {
	Player    *game.Player
	NPC       *game.NPC
	NpcID     int
	Option    string
	HasOption bool
}

// NpcAttackOptionEvent is the listener-facing form of the
// PluginNpcAttackOptionEvent (PluginTypes.ts:190-196): the player,
// the NPC, the content NPC id, the click type, and the handled
// flag. A hook (or the emit adapter) sets Handled to take the
// combat click over; the listener treats either the seam's return
// value or the flag as a claim, matching the interaction
// dual-claim pattern. NpcID is the content id
// (NPC.getContentId); for Go NPCs without transforms it equals
// the NPC's definition id.
type NpcAttackOptionEvent struct {
	Player    *game.Player
	NPC       *game.NPC
	NpcID     int
	ClickType int
	Handled   bool
}

// NpcHooks carries the NPC-interaction hub emits and the
// dialogue-start seam as func fields, following the Phase 13
// precedent (ObjectHooks). Every field is optional: nil means
// no hooks, which degenerates to the no-hooks defaults (range
// 1, no interaction claimed, no dialogue started, no operated
// delivery, no attack-option claim, no static shop opened).
// The coordinator wires these in cmd/server; the route and
// interaction emits have no hub families yet, so in practice
// StartDialogue, EmitOperated, EmitNpcAttackOption,
// Interactions, and OpenShop are wired.
type NpcHooks struct {
	// EmitNpcRoute fires the onNpcRoute hooks (table-only;
	// no hub emit exists yet). Range and
	// IgnoreProjectileClip are read back after the call.
	EmitNpcRoute func(*NpcRouteRequest)
	// EmitNpcAttackOption fires the onNpcAttackOption hooks
	// and reports whether one took the combat click over
	// (NPCOptionPacketListener.ts:61). The event's Handled
	// flag carries the same claim; either one claiming is
	// enough. A nil seam means no claim: the attack starts.
	EmitNpcAttackOption func(*NpcAttackOptionEvent) bool
	// StartDialogue starts whatever dialogue this click
	// should start (static binding and/or Talk-to
	// transcript — see the file header) and reports
	// whether one started. The coordinator wires it to
	// Worker B's playback entry point.
	StartDialogue func(*NpcDialogueRequest) bool
	// EmitNpcInteraction fires the onNpcInteraction hooks
	// (table-only; no hub emit exists yet) and reports
	// whether the interaction was handled. The event's
	// Handled flag carries the same claim; either one
	// claiming is enough.
	EmitNpcInteraction func(*NpcInteractionEvent) bool
	// EmitOperated delivers the npc:operated event for a
	// claimed click.
	EmitOperated func(*NpcOperatedEvent)
	// Interactions resolves the static-interaction action
	// for an NPC id and click type (see
	// NpcInteractionSource). A nil source means no static
	// bindings exist: the shop step is skipped and dispatch
	// proceeds to StartDialogue, as before shops landed.
	Interactions NpcInteractionSource
	// OpenShop opens a shop for the player: the Go form of
	// ShopManager.open(player, shopId, true)
	// (NpcInteractionManager.ts, the core.shops.open
	// branch — resetScroll is always true on this path).
	// It reports whether the shop opened, following the TS
	// return contract exactly: true when the interface
	// opened OR when the open was gated (emitCanShop
	// answered false — the click is claimed, no interface
	// opens), false when no shop exists under the id. A nil
	// seam behaves like a failed open: dispatch falls
	// through to the interaction emit. The coordinator
	// wires it over the ShopManager; this package never
	// names the ShopManager type.
	OpenShop func(*game.Player, int) bool
}

// DialogueAccess is the dialogue-runtime seam: the
// DialogueManager surface the resume paths need, behind an
// interface so this package never names Worker B's concrete
// manager type. The coordinator implements it over
// Player.GetDialogueManager() once that method lands:
//
//	Resume(p, widgetID, childIndex) — DialogueManager.handleResume
//	IsActive(p) — DialogueManager.isActive
//	Current(p) — DialogueManager.getCurrent, as a comparable
//	  identity token (the entry pointer, or nil when inactive)
//	HandleOption(p, option) — DialogueManager.handleOption,
//	  with option as the zero-based DialogueOption ordinal
//	  (buttonNum - 1 on the widget path)
//
// Current's return MUST be comparable with == (a pointer or
// nil): the widget tail compares the pre-click token with the
// post-branch current entry by identity, mirroring TS object
// identity (NetworkBuilder.ts:1140-1143).
type DialogueAccess interface {
	Resume(p *game.Player, widgetID int32, childIndex int) bool
	IsActive(p *game.Player) bool
	Current(p *game.Player) any
	HandleOption(p *game.Player, option int)
}

// npcVisibilityRange is the CrossFloor.visible range for NPC
// clicks (NPCOptionPacketListener.ts:52).
const npcVisibilityRange = 24

// isCombatOption mirrors NpcDefinition.isCombatOption
// (NpcDefinition.ts:175-177) over the combat-options set as it
// starts: {"attack"} (NpcDefinition.ts:172). Plugin-added
// combat options (addCombatOption) are not ported, so only
// "attack" counts. The option arrives lower-cased; the
// comparison is case-insensitive for defense.
func isCombatOption(option string, hasOption bool) bool {
	return hasOption && strings.EqualFold(option, "attack")
}

// npcOptionAt returns the lower-cased menu text at the clickType
// slot (1-based) of def, or absent when def is nil, the click
// type is out of range, or the slot carries no text
// (NPCOptionPacketListener.ts:56, 79).
func npcOptionAt(def NpcDefinition, clickType int) (string, bool) {
	if def == nil || clickType < 1 {
		return "", false
	}
	actions := def.Actions()
	if clickType > len(actions) || actions[clickType-1] == "" {
		return "", false
	}
	return strings.ToLower(actions[clickType-1]), true
}

// npcRouteResult is npcAction's outcome: the walk started, the
// attack started, the packet was refused silently, or the combat
// path was taken but the combat seam is unwired (a named drop).
type npcRouteResult int

const (
	npcStarted npcRouteResult = iota
	npcRefused
	npcCombat
	npcAttacked
)

// routeNpcAction routes one decoded npc_option message. The
// click-run override is set before routing, even when routing
// then refuses the packet (the INTERACTION_CLICKS preamble +
// the object precedent). A nil player drops with
// ReasonNpcOption (pinned behavior: the TS dispatch case guards
// on `if (this.player)`, and the nil-player outcome was pinned
// to this reason before this listener was implemented). An
// unwired lookup or definition source drops with
// ReasonNpcOption; a refused packet (gates, lookup miss,
// distance, non-attackable or dead combat target, claimed
// attack-option gate) returns noAction, the silent return TS
// gives; a combat option on an attackable live NPC with no
// combat seam wired drops with ReasonNpcOption; a started walk
// or a started attack counts as acted.
func routeNpcAction(p *game.Player, msg protocol.NpcOption, cycle int64, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNpcOption)
	}
	if q := p.MovementQueue(); q != nil {
		q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), msg.ModifierFlags))
	}
	if deps.NPCs == nil || deps.NpcDefs == nil {
		return drop(ReasonNpcOption)
	}
	switch npcAction(p, msg, cycle, deps) {
	case npcStarted, npcAttacked:
		return acted()
	case npcCombat:
		return drop(ReasonNpcOption)
	default:
		return noAction()
	}
}

// npcAction ports executeOption
// (NPCOptionPacketListener.ts:19-96): gates, lookup, distance,
// the combat branch (gate + attack through the combat seam), the
// route event, and the walk-to-NPC with the operate arrival
// closure. It reports whether the walk-to started, the attack
// started, the packet was refused, or the combat path was taken
// with no combat seam wired. WalkToTile's own silent refusals
// (mobility, destination) still count as started — the route
// event fired and the override was set — mirroring the object
// precedent.
func npcAction(p *game.Player, msg protocol.NpcOption, cycle int64, deps Deps) npcRouteResult {
	if !playerReady(p) {
		return npcRefused
	}
	// Gates (lines 28-29): hitpoints <= 0 or busy(), and the
	// click type in 1..5. busy() degenerates to the hitpoints
	// term in Go (see the file header). The decoder only
	// produces click types 1..5; the range check guards
	// hand-built messages.
	if hitpoints(p) <= 0 || msg.ClickType < 1 || msg.ClickType > 5 {
		return npcRefused
	}

	// Lookup (lines 30-35): the index range and slot checks
	// collapse into the lookup miss — a nil lookup result is
	// an empty or out-of-range slot. The owner gate (line 37)
	// degenerates to pass (no owners), as do the boat and
	// private-area gates (lines 39-49).
	npc := deps.NPCs.Find(msg.Index)
	if npc == nil {
		return npcRefused
	}
	npcLoc := npc.Location()

	// Visibility (line 52): CrossFloor.visible(root, root,
	// 24). With no boats, root locations are the plain
	// locations, and with no cross-floor policy visible() is
	// the same-plane Chebyshev test.
	if !p.Location().IsWithinDistance(npcLoc, npcVisibilityRange) {
		return npcRefused
	}

	// Definition + option (lines 55-56). A miss leaves the
	// option absent and the walk proceeds, as TS does with a
	// null definition.
	def, _ := deps.NpcDefs.NpcForPlayer(npc.ContentID(), p)
	option, hasOption := npcOptionAt(def, msg.ClickType)

	// Combat branch (lines 55-64): a combat option attacks
	// before walking — no plane check, no walk. Non-attackable
	// (or definition-less) NPCs and NPCs at hitpoints 0 or
	// below return silently; otherwise the attack-option gate
	// runs (a claim returns silently, without attacking),
	// and the attack starts through the combat seam, which
	// performs the autocast-conditional clear first (line
	// 62 — see the file header). An unwired seam takes the
	// named-drop path.
	if isCombatOption(option, hasOption) {
		if def == nil || !def.IsAttackable() || npc.Hitpoints <= 0 {
			return npcRefused
		}
		if deps.NpcHooks.EmitNpcAttackOption != nil {
			ev := &NpcAttackOptionEvent{
				Player:    p,
				NPC:       npc,
				NpcID:     npc.ContentID(),
				ClickType: msg.ClickType,
			}
			if deps.NpcHooks.EmitNpcAttackOption(ev) || ev.Handled {
				return npcRefused
			}
		}
		if deps.CombatAttack == nil {
			return npcCombat
		}
		deps.CombatAttack(p, &npc.Mobile)
		return npcAttacked
	}

	// Plane check (line 70): kept at its TS position, after
	// the combat branch. Redundant with the visibility test
	// in Go (same-plane is part of it).
	if p.Location().Z != npcLoc.Z {
		return npcRefused
	}

	// Facing (line 71): GAP — no position-to-face machinery.

	// Route event (lines 72-77): range 1,
	// ignoreProjectileClip false. Hooks may change them; the
	// range is clamped to 1..24, the emitNpcRoute clamp
	// (PluginManager.ts:1235-1243), applied here because no
	// hub emit exists yet to apply it.
	route := &NpcRouteRequest{
		Player:     p,
		NPC:        npc,
		Definition: def,
		NpcID:      npc.ContentID(),
		NpcIndex:   msg.Index,
		ClickType:  msg.ClickType,
		Location:   npcLoc,
		Range:      1,
	}
	if deps.NpcHooks.EmitNpcRoute != nil {
		deps.NpcHooks.EmitNpcRoute(route)
	}
	if route.Range < 1 {
		route.Range = 1
	}
	if route.Range > npcVisibilityRange {
		route.Range = npcVisibilityRange
	}

	// Arrival closure: the walkToEntity runnable (lines
	// 78-93) through operate/dispatch (lines 106-172).
	arrival := func() {
		// Target re-validation (the walkToEntity task's
		// targetValid): the NPC standing on the clicked
		// tile must still be the clicked NPC — same id,
		// same tile. A despawned, moved, or replaced NPC
		// drops the interaction silently.
		current := deps.NPCs.Find(msg.Index)
		if current == nil || current.ID != npc.ID || current.Location() != npcLoc {
			return
		}
		// Definition + option re-read (lines 78-79), and
		// the combat re-check (line 80): a combat option
		// returns silently, even if the pre-walk check
		// passed (the definition may have changed).
		arrivalDef, _ := deps.NpcDefs.NpcForPlayer(npc.ContentID(), p)
		arrivalOption, arrivalHasOption := npcOptionAt(arrivalDef, msg.ClickType)
		if isCombatOption(arrivalOption, arrivalHasOption) {
			return
		}
		// Facing (line 79) and the post-dialogue NPC turn
		// (lines 86-90): GAPs — no facing machinery, and
		// no mobile-interaction state.
		npcOperate(p, npc, msg.Index, msg.ClickType, arrivalDef, arrivalOption, arrivalHasOption, npcLoc, deps)
	}

	// Reached predicate: range <= 1 is the entity reach
	// (ReachedObj, the 1x1 form — see the file header for
	// the footprint deviation); range > 1 is the
	// within-distance term only (the projectile-clip term
	// is a gap: no clip access from listeners). NPCs are
	// static, so the click-time tile stands for the live
	// tile.
	reached := func(dest game.Location) bool {
		if route.Range <= 1 {
			return p.MovementQueue().ReachedObj(dest)
		}
		return p.Location().IsWithinDistance(dest, route.Range)
	}
	p.MovementQueue().WalkToTile(npcLoc, arrival, reached, true, cycle)
	return npcStarted
}

// npcOperate ports operate + dispatch
// (NPCOptionPacketListener.ts:106-172): the static shop step,
// the dialogue seam, the plugin interaction emit, the bank gap,
// and the "Nothing interesting happens." fallback. A claimed
// click emits npc:operated; the bank gap and the fallback do
// not.
func npcOperate(p *game.Player, npc *game.NPC, index, clickType int, def NpcDefinition, option string, hasOption bool, npcLoc game.Location, deps Deps) {
	// Static shop step (NpcInteractionManager.handle, the
	// core.shops.open branch — the FIRST position in dispatch,
	// before the interaction emit). A shop action consumes the
	// static position whether the open succeeds or fails: on
	// success the click is claimed (operated); on failure
	// (unknown shop, invalid id, or an unwired OpenShop seam)
	// dispatch falls through to the interaction emit WITHOUT
	// consulting StartDialogue — in TS the transcript lives
	// inside emitNpcInteraction, after handle, so a shop-bound
	// click never reaches a dialogue start through the static
	// position. DEGENERATION: the Go StartDialogue seam also
	// carries the transcript (see the file header), so a
	// Talk-to click bound to a shop action whose open fails
	// reaches the fallback message instead of the transcript
	// a TS any-hook would play. Shop actions bind to Trade,
	// not Talk-to, so the case is hypothetical; when the NPC
	// interaction families land and the transcript moves into
	// the emit, it disappears. Non-shop actions (including a
	// missing action or an unwired source) proceed to
	// StartDialogue exactly as before.
	skipDialogue := false
	if shopID, isShop, valid := staticShopID(deps, npc.ContentID(), clickType); isShop {
		skipDialogue = true
		if valid && deps.NpcHooks.OpenShop != nil && deps.NpcHooks.OpenShop(p, shopID) {
			npcEmitOperated(p, npc, option, hasOption, deps)
			return
		}
	}
	// Static bindings + transcript (NpcInteractionManager
	// .handle position; see the file header). A started
	// dialogue claims the click.
	if !skipDialogue && deps.NpcHooks.StartDialogue != nil {
		started := deps.NpcHooks.StartDialogue(&NpcDialogueRequest{
			Player:     p,
			NPC:        npc,
			Definition: def,
			NpcID:      npc.ContentID(),
			NpcIndex:   index,
			ClickType:  clickType,
			Option:     option,
			HasOption:  hasOption,
			Location:   npcLoc,
		})
		if started {
			npcEmitOperated(p, npc, option, hasOption, deps)
			return
		}
	}
	// Plugin interaction emit (before → main → any, inside
	// the hub emit). No families are ported yet, so a nil
	// seam means unhandled.
	if deps.NpcHooks.EmitNpcInteraction != nil {
		ev := &NpcInteractionEvent{
			Player:     p,
			NPC:        npc,
			Definition: def,
			NpcID:      npc.ContentID(),
			NpcIndex:   index,
			ClickType:  clickType,
			Location:   npcLoc,
		}
		if deps.NpcHooks.EmitNpcInteraction(ev) || ev.Handled {
			npcEmitOperated(p, npc, option, hasOption, deps)
			return
		}
	}
	if option == "bank" {
		// Bank branch (lines 160-164): GAP — opening the
		// bank needs the interface runtime (milestone 8).
		// No open, no operated emit, no message (the
		// object bank gap behaves the same way).
		return
	}
	// No-handler fallback (lines 166-167): the message, and
	// no operated emit.
	p.SendMessage("Nothing interesting happens.")
}

// staticShopID resolves the core.shops.open step for one click:
// the static action for (npcID, clickType) through the
// Interactions source, and its shop id read with TS Number()
// semantics (NpcInteractionManager.ts: handle —
// `Number(action.args.shopId)`, then integer-and->=0). It
// reports (shopID, true, true) for a shop action with a usable
// id, (0, true, false) for a shop action whose id does not
// convert (absent, non-numeric, fractional, or negative — TS
// handle returns false for it, and dispatch falls through),
// and (0, false, false) when no shop action exists (no source,
// no action, or another method — including core.dialogues.start
// and core.player.teleport, which this step never claims). The
// npcID is the content id (see NpcInteractionSource).
func staticShopID(deps Deps, npcID, clickType int) (shopID int, isShop, valid bool) {
	if deps.NpcHooks.Interactions == nil {
		return 0, false, false
	}
	action, ok := deps.NpcHooks.Interactions.InteractionAction(npcID, clickType)
	if !ok || action.Method != "core.shops.open" {
		return 0, false, false
	}
	raw, present := action.Args["shopId"]
	if !present {
		// Number(undefined) is NaN: unusable.
		return 0, true, false
	}
	num, ok := interactionNumber(raw)
	if !ok || math.IsNaN(num) || math.IsInf(num, 0) || math.Trunc(num) != num || num < 0 {
		return 0, true, false
	}
	return int(num), true, true
}

// interactionNumber is the Number() conversion
// NpcInteractionManager.handle applies to action.args.shopId,
// over the value kinds decoded JSON produces (plus the ints
// the shorthand synthesizer stores). Numbers convert
// directly; strings parse with JS Number() rules (whitespace
// trimmed, empty-to-zero, hex/binary/octal prefixes,
// Infinity); booleans are 1/0; nil (an explicit JSON null) is
// 0, as Number(null) is — distinct from an absent key, which
// the caller treats as undefined/NaN. Objects, arrays, and
// unparseable strings do not convert. The conversion only ever
// feeds the integer-and->=0 check, so spellings that parse to
// infinities or NaN in either language (case variants of
// "inf"/"nan") end invalid in both; the one spelling handled
// explicitly is the underscore (Go-literal only — see
// jsNumberString).
func interactionNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case string:
		return jsNumberString(v)
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	case nil:
		return 0, true
	default:
		return 0, false
	}
}

// jsNumberString parses s with JS Number() string rules: trim
// surrounding whitespace, then the empty string is 0, the
// Infinity spellings are infinities, 0x/0b/0o-prefixed
// integers parse in their radix, and anything else must parse
// as a decimal float in full (a trailing-garbage string is
// NaN, unlike ParseFloat's prefix behavior — ParseFloat is
// only consulted after the whole-string shape is confirmed by
// its own error, since Go's ParseFloat rejects trailing
// garbage too).
func jsNumberString(s string) (float64, bool) {
	text := strings.TrimSpace(s)
	if text == "" {
		return 0, true
	}
	switch text {
	case "Infinity", "+Infinity":
		return math.Inf(1), true
	case "-Infinity":
		return math.Inf(-1), true
	}
	if len(text) > 2 && text[0] == '0' && (text[1] == 'x' || text[1] == 'X' || text[1] == 'b' || text[1] == 'B' || text[1] == 'o' || text[1] == 'O') {
		if parsed, err := strconv.ParseInt(text, 0, 64); err == nil {
			return float64(parsed), true
		}
		return 0, false
	}
	// Underscores are Go-literal syntax, not JS Number syntax:
	// Number("1_0") is NaN, while ParseFloat would accept it.
	if strings.Contains(text, "_") {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// npcEmitOperated delivers the npc:operated event for a claimed
// click (operate, NPCOptionPacketListener.ts:106-112). A nil
// seam delivers nothing.
func npcEmitOperated(p *game.Player, npc *game.NPC, option string, hasOption bool, deps Deps) {
	if deps.NpcHooks.EmitOperated == nil {
		return
	}
	deps.NpcHooks.EmitOperated(&NpcOperatedEvent{
		Player:    p,
		NPC:       npc,
		NpcID:     npc.ContentID(),
		Option:    option,
		HasOption: hasOption,
	})
}

// routeDialogueResume routes one decoded dialogue_continue
// message: the dispatch case (NetworkBuilder.ts:908-951). The
// creation-menu pre-branch runs FIRST: a click against group 270,
// child >= 15, while a creation menu is open, is a skillmulti
// item click, not a dialogue resume — the item is
// menu.items[child-15], the amount is childIndex clamped to the
// menu's maxAmount (or 28), default 1; the player attribute is
// set to child-15, the menu closes, and menu.execute runs, all
// before the interface-action-click emit and handleResume. Only
// when no menu is open, the group/child do not match, or the
// child is past the menu's items does the resume fall through to
// the emit-then-handleResume order below. The
// interface-action-click emit runs first, ungated (the emit,
// not the opcode handle — NetworkBuilder.ts:939-945 calls
// PluginManager.emitInterfaceActionClick directly): a pending
// prompt or a claiming hook consumes the resume and
// handleResume never runs. Only an unclaimed resume reaches
// DialogueAccess.Resume, whose adapter re-checks the prompt
// before handleResume (harmless: the emit already ran the same
// check with the same inputs). A nil player drops
// with ReasonNoPlayer (TS guards the case on `this.player`);
// an unwired dialogue seam drops with ReasonDialogueUnwired;
// a resume the manager refused (no active dialogue, wrong
// widget, out-of-range child) is ReasonNoAction; an accepted
// resume — the emit claimed it, the dialogue advanced, or an
// option executed — counts as acted.
func routeDialogueResume(p *game.Player, msg protocol.DialogueContinue, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if menu := p.GetCreationMenu(); menu != nil {
		groupID := int(uint32(msg.WidgetID) >> 16)
		childID := int(uint32(msg.WidgetID) & 0xffff)
		if groupID == game.CreationMenuGroupID && childID >= game.CreationMenuFirstItemComponent {
			if itemID, ok := menu.ItemAt(childID - game.CreationMenuFirstItemComponent); ok {
				amount := 1
				if msg.ChildIndex > 0 {
					amount = min(msg.ChildIndex, menu.AmountLimit())
				}
				// Remembered for the space key next time
				// (CREATION_MENU_LAST_ITEM_VARP).
				p.SetAttribute(game.CreationMenuLastItemAttribute, childID-game.CreationMenuFirstItemComponent)
				p.SetCreationMenu(nil)
				if sender, ok := p.Session.(game.CreationMenuSender); ok {
					sender.CloseCreationMenu()
				}
				menu.Execute(itemID, amount)
				return acted()
			}
		}
	}
	ev := &InterfaceActionClickEvent{
		Player:     p,
		ButtonID:   int(msg.WidgetID),
		Action:     msg.ChildIndex,
		GroupID:    int(uint32(msg.WidgetID) >> 16),
		HasGroupID: true,
		ChildID:    int(uint32(msg.WidgetID) & 0xffff),
		HasChildID: true,
	}
	if emitInterfaceActionClick(ev, deps) {
		return acted()
	}
	if deps.Dialogue == nil {
		return drop(ReasonDialogueUnwired)
	}
	if deps.Dialogue.Resume(p, msg.WidgetID, msg.ChildIndex) {
		return acted()
	}
	return noAction()
}

// dialogueCurrentToken captures the current dialogue entry's
// identity before a widget_action click's branches run
// (NetworkBuilder.ts:983: openDialogue). A nil seam yields a
// nil token, which never matches.
func dialogueCurrentToken(deps Deps, p *game.Player) any {
	if deps.Dialogue == nil {
		return nil
	}
	return deps.Dialogue.Current(p)
}

// dialogueWidgetOption is the widget_action dialogue-option
// tail (NetworkBuilder.ts:1140-1143): when a dialogue is
// active, the current entry is the one open before the click,
// and the button number is 1..5, the click selects option
// buttonNum - 1 (the zero-based DialogueOption ordinal). A
// nil token (no seam, or no dialogue open at click time)
// never fires: a dialogue opened by the click itself must not
// take the same click as its option.
func dialogueWidgetOption(deps Deps, p *game.Player, open any, buttonNum int) {
	if deps.Dialogue == nil || open == nil {
		return
	}
	if !deps.Dialogue.IsActive(p) {
		return
	}
	if deps.Dialogue.Current(p) != open {
		return
	}
	if buttonNum < 1 || buttonNum > 5 {
		return
	}
	deps.Dialogue.HandleOption(p, buttonNum-1)
}

// Spell on NPC (NPCOptionPacketListener.castSpell,
// NPCOptionPacketListener.ts:161-198, and
// castSpellOnScriptedNpc, lines 200-262). The dispatch case sets
// the click-run override before routing (spell_on_npc is in
// INTERACTION_CLICKS, NetworkBuilder.ts:246-260), exactly as the
// option clicks do.
//
// Gaps, documented at their sites:
//   - Lunar routing (NetworkBuilder.ts:808-822,
//     LunarSpells.handleNpcTarget first): Lunar is out of scope,
//     so a Lunar-named spell falls through to resolution and
//     refuses silently when no table holds it — the same
//     observable outcome as TS with no Lunar handler installed.
//   - CrossFloor visibility: Go has no CrossFloor policy, so the
//     24-tile check is the same-plane Chebyshev test the option
//     clicks use (combat_factory.go documents same-Z only as
//     degenerate); a cross-floor cast reads as out of range.
//   - Projectile line of sight on the scripted route
//     (RegionManager.canProjectileAttackTarget): no Go
//     equivalent, so the scripted approach is distance-only.
//   - Boats and private-area boarding on the scripted route:
//     the private-area and owner gates are ported; the boat
//     gate degenerates to pass (no boat seam in the listeners).
//   - Facing and cast sounds on the scripted route's
//     consumeRunes: no facing or sound systems exist (Phase 29).

// spellNpcResult is the spell-on-NPC route's outcome.
type spellNpcResult int

const (
	// spellNpcCast: the cast started (combat route, or a
	// scripted cast a hook claimed and paid for).
	spellNpcCast spellNpcResult = iota
	// spellNpcWalk: the scripted walk-to started; the cast
	// resolves on arrival.
	spellNpcWalk
	// spellNpcRefused: gates refused the packet silently, as
	// TS's false return does.
	spellNpcRefused
	// spellNpcUnwired: a lookup, definition source, or combat
	// seam is nil.
	spellNpcUnwired
)

// spellScriptedApproachRange is the scripted route's approach
// distance (NPCOptionPacketListener.ts:209-214: within 10 tiles).
const spellScriptedApproachRange = 10

// routeSpellOnNpc routes one decoded spell_on_npc message. An
// unwired lookup, definition source, or combat seam drops with
// ReasonNpcOption; a refused packet returns noAction, the silent
// return TS gives; a started cast or walk counts as acted.
func routeSpellOnNpc(p *game.Player, msg protocol.SpellOnNpc, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	switch castSpellOnNpc(p, msg, deps) {
	case spellNpcCast, spellNpcWalk:
		return acted()
	case spellNpcUnwired:
		return drop(ReasonNpcOption)
	default:
		return noAction()
	}
}

// castSpellOnNpc ports NPCOptionPacketListener.castSpell
// (NPCOptionPacketListener.ts:161-198): dead/busy gates, NPC
// lookup, spell resolution (combat spell, else effect spell,
// else the magic:resolve-spell custom event for
// plugin-supplied utility spells), the 24-tile visibility
// check, the area canCastOnNpc permission, the combat route
// through Combat.castSpellOn iff attackable, else the scripted
// route, the canCastOnTarget check, and the movement reset on
// failure. busy() degenerates to the hitpoints term, as for the
// option clicks.
func castSpellOnNpc(p *game.Player, msg protocol.SpellOnNpc, deps Deps) spellNpcResult {
	if !playerReady(p) || hitpoints(p) <= 0 {
		return spellNpcRefused
	}
	if deps.NPCs == nil || deps.NpcDefs == nil {
		return spellNpcUnwired
	}
	// Lookup (line 163): the index range and slot checks
	// collapse into the lookup miss, as for the option clicks.
	npc := deps.NPCs.Find(msg.TargetIndex)
	if npc == nil {
		if q := p.MovementQueue(); q != nil {
			q.Reset(true)
		}
		return spellNpcRefused
	}
	npcLoc := npc.Location()

	// Spell resolution (lines 165-172): the combat spell, else
	// the effect spell, else the magic:resolve-spell custom
	// event when the cache named the click.
	spellID := resolveSpellID(deps, int(msg.SpellWidget), msg.SpellChild, msg.SpellItemID)
	combat, caster, ok := game.ResolveSpell(spellID)
	if !ok {
		if name, hasName := spellNameOf(deps, int(msg.SpellWidget), msg.SpellItemID); hasName {
			ev := &game.SpellResolveEvent{
				Player:      p,
				SpellID:     spellID,
				SpellName:   name,
				SpellWidget: int(msg.SpellWidget),
			}
			emitCustom(p, "magic:resolve-spell", ev)
			switch s := ev.Spell.(type) {
			case game.CombatSpell:
				combat, caster, ok = s, s, true
			case game.SpellCaster:
				caster, ok = s, true
			}
		}
	}
	// Gates (lines 173-177): no spell, out of 24-tile
	// visibility, or a dead NPC resets the movement queue and
	// refuses.
	if !ok || !p.Location().IsWithinDistance(npcLoc, npcVisibilityRange) || npc.Hitpoints <= 0 {
		if q := p.MovementQueue(); q != nil {
			q.Reset(true)
		}
		return spellNpcRefused
	}

	// Area permission (line 178): the NPC's private area, else
	// its area. A nil answer means "no opinion".
	var area *game.Area
	if npc.Area() != nil {
		area = npc.Area()
	}
	if pa := npc.PrivateArea(); pa != nil {
		area = pa.Area
	}
	var permission *bool
	if area != nil {
		var spellAny any = caster
		if combat != nil {
			spellAny = combat
		}
		permission = area.CanCastOnNpc(p, npc, spellAny)
	}

	// Definition + attackable (line 181): a missing definition
	// reads as not attackable, as TS's optional chain does.
	def, _ := deps.NpcDefs.NpcForPlayer(npc.ContentID(), p)
	attackable := def != nil && def.IsAttackable()

	// Combat route (lines 181-197): only a CombatSpell may
	// enter Combat.castSpellOn, and only when the area permits
	// (an explicit permission wins) or the NPC is attackable.
	allowed := false
	if combat != nil {
		if permission != nil {
			allowed = *permission
		} else {
			allowed = attackable
		}
	}
	if !allowed {
		// Scripted route (lines 182-188): a plane mismatch
		// refuses WITHOUT the queue reset (TS returns before
		// it); otherwise the queue resets, an explicit area
		// denial refuses, and anything else goes scripted.
		if p.Location().Z != npcLoc.Z {
			return spellNpcRefused
		}
		if q := p.MovementQueue(); q != nil {
			q.Reset(true)
		}
		if permission != nil && !*permission {
			return spellNpcRefused
		}
		return castSpellOnScriptedNpc(p, msg.TargetIndex, npc, caster, spellID, attackable, deps, false)
	}
	if !combat.CanCastOnTarget(p, &npc.Mobile) {
		if q := p.MovementQueue(); q != nil {
			q.Reset(true)
		}
		return spellNpcRefused
	}
	if deps.SpellHooks.CastSpell == nil {
		return spellNpcUnwired
	}
	deps.SpellHooks.CastSpell(p, &npc.Mobile, combat)
	return spellNpcCast
}

// castSpellOnScriptedNpc ports
// NPCOptionPacketListener.castSpellOnScriptedNpc
// (NPCOptionPacketListener.ts:200-262): the walk-to approach
// and the onSpellOnNpc hook emit. A combat spell on the
// scripted route needs projectile line of sight in TS; Go has
// no such check, so the approach is distance-only (see the
// file header). The walk-to re-invokes with approached=true;
// a second failure refuses instead of walking again.
func castSpellOnScriptedNpc(p *game.Player, index int, npc *game.NPC, spell game.SpellCaster, spellID int, attackable bool, deps Deps, approached bool) spellNpcResult {
	npcLoc := npc.Location()
	// Gates (lines 205-208): plane, private area, owner, and
	// the spell's own target/cast checks. The boat gate
	// degenerates to pass (no boat seam in the listeners).
	if p.Location().Z != npcLoc.Z {
		return spellNpcRefused
	}
	if pa := npc.PrivateArea(); pa != nil && pa != p.PrivateArea() {
		return spellNpcRefused
	}
	if owner := npc.Owner(); owner != nil && owner != p {
		return spellNpcRefused
	}
	if !spell.CanCastOnTarget(p, &npc.Mobile) || !spell.CanCast(p, false) {
		return spellNpcRefused
	}
	// Approach (lines 209-221): within 10 tiles, else walk in
	// and resolve on arrival. The walk-to counts as acted by
	// the caller even though TS returns false here — the walk
	// started, as for the option clicks.
	if !p.Location().IsWithinDistance(npcLoc, spellScriptedApproachRange) {
		if approached {
			return spellNpcRefused
		}
		p.MovementQueue().WalkToEntity(game.NPCTarget(npc), func() {
			castSpellOnScriptedNpc(p, index, npc, spell, spellID, attackable, deps, true)
		})
		return spellNpcWalk
	}

	// Emit (lines 222-261): the hook pays through
	// consumeRunes. Facing (setPositionToFace) and the cast
	// sound have no Go systems and are skipped; the rune
	// payment, startCast, and XP grant are ported.
	consumed := false
	consumptionAttempted := false
	ev := &SpellOnNpcEvent{
		Player:   p,
		NPC:      npc,
		NpcID:    npc.ContentID(),
		NpcIndex: index,
		Spell:    spell,
		SpellID:  spellID,
		Location: npcLoc,
		ConsumeRunes: func() bool {
			consumptionAttempted = true
			if consumed || !spell.CanCast(p, true) {
				return false
			}
			consumed = true
			spell.StartCast(&p.Mobile, &npc.Mobile)
			if xp := spell.BaseExperience(); xp > 0 {
				p.Skills().AddExperiences(game.SkillMagic, xp)
			}
			return true
		},
	}
	handled := false
	if deps.SpellHooks.EmitSpellOnNpc != nil {
		handled = deps.SpellHooks.EmitSpellOnNpc(ev) || ev.Handled
	}
	// "You can't attack this npc." (lines 256-258): an
	// unhandled, unpaid cast on a non-attackable NPC.
	if !handled && !consumptionAttempted && !attackable {
		p.SendMessage("You can't attack this npc.")
	}
	// TS returns handled && consumed; the Go outcome tracks
	// state change instead — a hook that claimed the cast, or
	// a paid cast, counts as acted even when the other half
	// is false, because the return value is unobserved in TS
	// (NetworkBuilder calls castSpell as a statement).
	if handled || consumed {
		return spellNpcCast
	}
	return spellNpcRefused
}
