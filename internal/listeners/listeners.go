package listeners

import (
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// DropReason names why a decoded packet takes no game action. The
// per-listener constants below are the reasons Statuses reports; the
// dispatch-group constants name the shared message dispatches where TS
// splits one message between listeners by content; the outcome
// constants name results that are not a missing system (a refused
// gate, an unwired seam, an examine with no definition source).
type DropReason string

// Per-listener drop reasons, one per file in net/packet/impl/*, in TS
// filename order. Each names the missing system the listener's TS code
// acts through. Listeners implemented in this package have no reason
// constant: their remaining gaps are named in the implemented row of
// Statuses and in the listener file's header comment.
const (
	// ChangeAppearancePacketListener.apply (ChangeAppearancePacketListener.ts:4-31)
	// checks canChangeAppearance, writes the look array through the
	// Appearance system, and closes the interface through the
	// PacketSender. The Appearance system exists (phase 9), but no
	// open-interface tracking exists to gate on or close through,
	// and no interface-removal send exists.
	ReasonChangeAppearance DropReason = "requires open-interface tracking and interface-removal send — NOT STARTED"
	// CommandPacketListener.execute (CommandPacketListener.ts:7-53)
	// dispatches every command to PluginManager.emitCommand; the core
	// CommandManager path is commented out, so no core command exists.
	// The registry arrives through Deps.Chat.Commands (the
	// CommandDispatcher seam, wired by the coordinator to the
	// command registry); "::" text with no dispatcher wired drops
	// with this reason.
	ReasonCommand DropReason = "requires the command registry — unwired (Deps.Chat.Commands is nil)"
	// FollowPlayerPacketListener.follow (FollowPlayerPacketListener.ts:31-45)
	// sets the player's following target and mobile interaction so
	// the pursuit step re-paths every tick. Following/pursuit
	// machinery is NOT STARTED, and Handle has no World lookup to
	// resolve the target index to a player.
	ReasonFollowPlayer DropReason = "requires following/pursuit machinery and World player lookup — NOT STARTED"
	// InterfaceActionClickOpcode.handle (InterfaceActionClickOpcode.ts:7-51)
	// forwards every click to PluginManager.emitInterfaceActionClick
	// after busy/teleport guards. The handle, the emit path, and
	// the hub families are implemented (phase 18,
	// interfaceaction.go); this reason is RETAINED for the
	// reason inventory only — no Handle path returns it
	// anymore. Unhandled interface-action clicks are
	// ReasonNoAction (fully routed, nothing claimed); the
	// widget_target case keeps ReasonWidgetTarget for its
	// missing spell half.
	ReasonInterfaceAction DropReason = "interface-action handling is implemented (phase 18) — retained for the reason inventory; unhandled clicks are ReasonNoAction"
	// ItemActionPacketListener.handleAction/handleFirstAction
	// (ItemActionPacketListener.ts:39-146) reads the item from the
	// Inventory/Equipment container and emits the plugin item-action
	// event (falling back to Equip for wieldables). Containers exist
	// (phase 9) and the equip fallback is implemented; the plugin
	// actions are NOT STARTED.
	ReasonItemAction DropReason = "requires plugin-owned item actions — NOT STARTED"
	// MagicOnItemPacketListener.castOnItem/castGroundItem
	// (MagicOnItemPacketListener.ts:116-164, 481-503) check Magic
	// levels and runes in the Inventory, consume runes, and animate
	// alchemy/Telekinetic Grab through combat, skills, dialogue, and
	// ground-item systems. Implemented (phase 27, magicitem.go):
	// Low/High Alchemy (levels, runes, pacing, queue, coins, XP)
	// and Telekinetic Grab (range, walk-to approach, delayed
	// landing through the task seam) on the NORMAL spellbook.
	// This reason now covers the remaining drops: the other
	// EffectSpells (Superheat, Bones to Bananas/Peaches, Bake
	// Pie, Vengeance — NOT STARTED handlers), the Arceuus
	// reanimations, an unwired ground-item store or task seam
	// (Deps.GroundItems or Deps.SpellHooks.ScheduleTask is nil),
	// and the spell-on-object route (UseItemPacketListener.
	// spellOnObject — NOT STARTED, see useitem.go).
	ReasonMagicOnItem DropReason = "requires an implemented EffectSpells handler, the Arceuus reanimations, or the ground-item store / task seam — unwired (Deps.GroundItems or Deps.SpellHooks.ScheduleTask is nil); spell-on-object NOT STARTED"
	// MagicOnPlayerPacketListener.cast (MagicOnPlayerPacketListener.ts:5-43)
	// resolves a CombatSpell and casts it through the combat system.
	// Implemented (phase 27, player.go): dead gates, index bounds,
	// self-target reset, spell-id resolution, the
	// magic:spell-on-player custom event, the canCastOnTarget
	// check, and the cast through Combat.castSpellOn. This reason
	// now covers only an unwired player lookup or combat seam
	// (Deps.Players or Deps.SpellHooks.CastSpell is nil). The
	// LunarSpells routing (NetworkBuilder.ts:786-806) stays a
	// documented gap: a Lunar-named spell falls through to
	// resolution and refuses silently when no table holds it.
	// The click-run override is still set (INTERACTION_CLICKS).
	ReasonMagicOnPlayer DropReason = "requires the player lookup or combat spell seam — unwired (Deps.Players or Deps.SpellHooks.CastSpell is nil)"
	// NPCOptionPacketListener.executeOption/castSpell
	// (NPCOptionPacketListener.ts:19-198). The Talk-to/dialogue
	// path is implemented (phase 14, npc.go): index lookup,
	// visibility gates, walk-to-NPC, the dialogue-start seam,
	// the interaction/operated emits, and the "Nothing
	// interesting happens." fallback. The combat option is
	// implemented (phase 16, npc.go): the attackable/hitpoints
	// gates, the emitNpcAttackOption gate, and attack initiation
	// through the combat seam. The spell-on-NPC path is
	// implemented (phase 27, npc.go): spell-id resolution, the
	// 24-tile visibility check, the area canCastOnNpc
	// permission, the combat route through Combat.castSpellOn,
	// and the scripted route with the onSpellOnNpc emit. This
	// reason now covers the remaining drops: an unwired NPC
	// lookup, definition source, or combat seam (Deps.NPCs,
	// Deps.NpcDefs, or Deps.SpellHooks.CastSpell is nil). The
	// LunarSpells routing (NetworkBuilder.ts:808-822) stays a
	// documented gap, as do CrossFloor visibility and
	// projectile line of sight on the scripted route.
	ReasonNpcOption DropReason = "requires the NPC lookup, definition source, or combat spell seam — unwired (Deps.NPCs, Deps.NpcDefs, or Deps.SpellHooks.CastSpell is nil)"
	// ObjectActionPacketListener.executeAction
	// (ObjectActionPacketListener.ts:30-172) looks the object up in
	// MapObjects, walks to it with walkToObject, then emits the
	// plugin object interaction (or opens the bank). The
	// main-world path is implemented (phase 13); this reason now
	// means only that the object seams are unwired
	// (Deps.Objects or Deps.ObjectDefs is nil). The boat-deck
	// branches and facing stay gaps (see objectaction.go); the
	// bank branch is claimed by the Bank content (phase 46).
	ReasonObjectAction DropReason = "requires the object lookup and definition source — unwired (Deps.Objects or Deps.ObjectDefs is nil)"
	// PlayerOptionPacketListener.executeClientOption
	// (PlayerOptionPacketListener.ts:17-40) attacks (option 1),
	// trades (option 2), follows (option 3), or emits the plugin
	// player-option event after walking to the target (options
	// 4-8). Option 1 is implemented (phase 16, player.go): the
	// hitpoints/self/target gates, the player:option-route
	// intercept, attack initiation through the combat seam, and
	// the player-attack emit. Option 2 is implemented (phase
	// 17, trade.go). This reason now covers the remaining
	// drops: options 4-8 (the onPlayerOption family —
	// NOT STARTED; walk-to-entity exists since phase 17), and
	// an unwired player lookup or combat seam for option 1
	// (Deps.Players or Deps.CombatAttack is nil). Option 3
	// keeps its own reason (ReasonFollowPlayer).
	ReasonPlayerOption DropReason = "requires plugin-owned player options (options 4-8: onPlayerOption — NOT STARTED) or the player lookup or combat seam for the attack option — unwired (Deps.Players or Deps.CombatAttack is nil)"
	// SecondGroundItemOptionPacketListener.interact
	// (SecondGroundItemOptionPacketListener.ts:6-54) finds the ground
	// item through ItemOnGroundManager, walks to it, and emits the
	// plugin ground-item interaction. The manager, the walk-to
	// machinery, and the operable lookup exist (phases 8-9); the
	// plugin interaction is NOT STARTED.
	ReasonSecondGroundItemOption DropReason = "requires plugin-owned ground-item interactions — NOT STARTED"
	// TradeRequestPacketListener.request/sendRequest
	// (TradeRequestPacketListener.ts:6-62) walks to the target with
	// walkToEntity and requests a trade through the Trading system
	// after a plugin event. Trading and walk-to-entity are
	// implemented (phase 17); this reason now covers only an
	// unwired player lookup (Deps.Players is nil) for option 2.
	ReasonTradeRequest DropReason = "requires the player lookup for the trade request — unwired (Deps.Players is nil)"
	// UseItemPacketListener.itemOn*/spellOnObject
	// (UseItemPacketListener.ts:12-237) verify the used item in the
	// Inventory, walk to the target where applicable, and emit the
	// plugin item-use event (checking MapObjects for object targets).
	// The item-on-object paths are implemented (phase 13) and
	// item-on-item since phase 33 (the inventory kind of
	// INVENTORY_USE_ON routes to the onItemOnItem emit);
	// item-on-ground-item needs an unfired emit family, and
	// spell-on-object needs magic — both NOT STARTED.
	// Item-on-NPC stays a decode-drop (see useitem.go): it needs
	// the unfired onItemOnNpc/onNpcRoute families. (Walk-to-entity
	// exists since phase 17, movement_queue.go; the phase 16
	// decision predates it.) This reason covers those remaining
	// paths, plus unwired object seams for the item-on-object
	// paths (Deps.Objects or Deps.ObjectDefs is nil).
	ReasonUseItem DropReason = "requires unfired item-use emit families or magic — NOT STARTED; item-on-object requires the object seams — unwired"
)

// Dispatch-group drop reasons, for opcodes whose TS message is shared
// between listeners. Handle returns these where the split needs state
// this package cannot see; Statuses still reports the per-listener
// reason for each file.
const (
	// EXAMINE_OBJ (opcode 104) decodes to a ground_item_action with
	// option "Examine", which groundItemAction answers with
	// examineItem (NetworkBuilder.ts lines 1832-1840) through the
	// chat-message encoder. No listener file owns it. The encoder
	// exists since phase 12; this reason now means only that no
	// ExamineSource is wired (Deps.Examine is nil).
	ReasonGroundItemExamine DropReason = "requires the examine definition source — unwired (Deps.Examine is nil)"
	// EXAMINE_NPC / EXAMINE_LOC (opcodes 9/85) are answered in
	// dispatch itself (NetworkBuilder.ts lines 1283-1306), not
	// by a listener file. The encoder exists since phase 12;
	// this reason means only that no ExamineSource is wired
	// (Deps.Examine is nil).
	ReasonExamineUnwired DropReason = "requires the examine definition source — unwired (Deps.Examine is nil)"
	// The widget_action opcodes reach InterfaceActionClickOpcode,
	// ItemAction, or Equip depending on widget group, slot/item
	// contents, and earlier handlers (bank, shop, prayer, combat)
	// in the dispatch case (NetworkBuilder.ts lines 975-1146). The
	// equipment, shop, and inventory branches are implemented, and
	// the fallthrough reaches the opcode handle (phase 18); this
	// reason now covers ONLY the unresolved-inventory `continue`
	// branch (lines 1093-1101: no warnable widget, already warned,
	// or a stateless Deps). The bank/prayer/combat/spell branches
	// stay gaps, but their clicks fall through to the emit rather
	// than dropping here.
	ReasonWidgetAction DropReason = "unresolved inventory widget action — no warnable widget, already warned, or stateless Deps (NetworkBuilder.ts:1093-1101)"
	// IF_BUTTONT (opcode 90) reaches MagicOnItem.castOnItem for a
	// spell on an inventory item, else InterfaceActionClickOpcode
	// (NetworkBuilder.ts lines 1173-1216). The interface-action
	// fallback is implemented (phase 18, routeWidgetTarget) and
	// the alchemy spell-on-item route is implemented (phase 27,
	// magicitem.go: Low/High Alchemy via castOnItem), and the
	// onSpellOnItem hook family fires since phase 43
	// (castSpellOnItem dispatches it before castOnItem); this
	// reason now means the fallback ran unclaimed while the
	// remaining spell-on-item routes (LunarSpells, the other
	// EffectSpells) are NOT STARTED.
	ReasonWidgetTarget DropReason = "spell-on-item handling (MagicOnItem) for a non-alchemy spell — NOT STARTED; the interface-action fallback ran unclaimed"
	// A varp_transmit (opcode 191) whose varpId is not 43, 172
	// or 173. The dispatch case (NetworkBuilder.ts lines
	// 1147-1168) handles varpId 43 through
	// WeaponInterfaceManager.changeCombatStyle +
	// BonusManager.update (lines 1152-1154), varpId 172 through
	// setAutoRetaliate + combat stopAutoRetaliation + sendConfig
	// (lines 1155-1160), and EVERY varpId through
	// applyAudioSettingFromClient (line 1167; Player.ts lines
	// 928-937, backed by the audio-settings store and the
	// audio:setting-changed plugin event). Varps 43 and 172 act
	// since phase 26 (combat styles); the audio-setting
	// application for every varpId, and every other varpId
	// itself, still need systems that do not exist in Go, so
	// those transmissions stay named decode-drops. The packet
	// IS recognized and decoded, so this is a named
	// decode-drop, never "unrecognized". VarpId 173 is the run
	// toggle and acts; its audio-setting application is
	// dropped with the toggle (see the ClientVarpTransmit
	// case).
	ReasonVarpTransmitOther DropReason = "requires audio-settings handling or an unported varp — NOT STARTED"
	// A varp_transmit run toggle (varpId 173) with no player
	// attached, and likewise any implemented listener route with no
	// player. TS guards the dispatch cases on `if (this.player)`
	// (NetworkBuilder.ts lines 975, 1148, 1218): with no player the
	// packet is skipped silently. Only acted branches need this
	// reason — pure drops never dereference the player, so they
	// return their own reason with a nil player too.
	ReasonNoPlayer DropReason = "no player attached — dropped without action"
	// A known listener opcode whose payload fails to decode. TS
	// throws at decode time and the connection handler closes the
	// socket with 1002 (NetworkBuilder.handle, NetworkBuilder.ts
	// lines 608-618), but Go framing (DecodeFrame) already enforces
	// the table lengths before any packet reaches Handle, so a
	// failure here means an internally malformed variable-length
	// payload (e.g. an unterminated string) or a hand-built
	// TurnPacket. Handle has no channel and never disconnects: the
	// packet is dropped and the outcome says so.
	ReasonMalformedPayload DropReason = "malformed payload — dropped without disconnect"
)

// Outcome reasons for implemented listeners: the packet was fully
// routed, but no state change happened, for a reason that is not a
// missing system.
const (
	// An implemented listener's gates refused the packet — a dead
	// player, a slot/id mismatch, an out-of-range slot, an unknown
	// option verb, an inoperable ground item — or a plugin event
	// handled the packet and suppressed its effect. TS returns
	// silently in all of these; the packet is recognized, decoded,
	// and takes no action.
	ReasonNoAction DropReason = "listener gates refused the packet — no action taken"
	// A drop or pickup was otherwise valid but Deps.GroundItems is
	// nil: no ground-item store is wired, so the item could not be
	// registered or resolved. The item stays where it was. The
	// coordinator wires the manager in handleTurnPacket; see Deps.
	ReasonGroundItemsUnwired DropReason = "no ground-item store wired (Deps.GroundItems is nil) — no action taken"
	// A drop of an undroppable item: TS opens the destroy-item
	// interface (DropItemPacketListener.ts:100-118), a widget flow
	// needing interface, attribute, and chatbox-string systems that
	// do not exist. The item is neither dropped nor destroyed.
	ReasonDestroyInterface DropReason = "requires destroy-item widget flow — NOT STARTED"
	// An inventory examine (option "examine", or a widget-action
	// op 10 on a worn item): TS answers with examineItem through
	// the chat-message encoder (NetworkBuilder.ts:1779-1780, 1021).
	// The encoder exists since phase 12; this reason now means
	// only that no ExamineSource is wired (Deps.Examine is nil).
	ReasonItemExamine DropReason = "requires the examine definition source — unwired (Deps.Examine is nil)"
	// A dialogue_continue packet (RESUME_PAUSEBUTTON, opcodes 62
	// and 252) with no dialogue runtime wired: the dispatch case
	// (NetworkBuilder.ts lines 908-951) ends in
	// DialogueManager.handleResume, which arrives through
	// Deps.Dialogue (see npc.go). The coordinator wires it over
	// Player.GetDialogueManager() once the DialogueManager
	// lands; until then the packet is recognized, decoded, and
	// dropped with this reason.
	ReasonDialogueUnwired DropReason = "requires the dialogue runtime — unwired (Deps.Dialogue is nil)"
)

// Deps supplies the ONLY game interactions listeners may perform
// beyond the player itself. Keep it minimal; every field must be
// justified by a TS-grounded implemented behavior.
type Deps struct {
	// GroundItems is the world ground-item store the drop and
	// pickup listeners register with and resolve from. The
	// coordinator passes the world's *game.GroundItemManager in
	// handleTurnPacket (which requires the Items() snapshot
	// accessor on the manager; see the GroundItems interface).
	// When nil, drop and pickup degrade to
	// ReasonGroundItemsUnwired and the item stays put.
	//
	// The former WalkTo field was REMOVED in phase 9. Every TS
	// walk in the listener files is a walkToTile/walkToGroundItem
	// with an arrival callback, re-routing, and reach checks —
	// none of which a bare MovementRequest expresses — and the
	// queue now carries that machinery itself
	// (MovementQueue.WalkToGroundItem, grounded in
	// MovementQueue.ts:1234-1331). The pickup listener calls the
	// queue directly; no listener needs a bare walk seam, and
	// keeping one would invite the wrong semantics phase 6
	// rejected (walking onto target tiles, walking for forged
	// packets, arriving with no interaction). handleTurnPacket
	// must drop the WalkTo field from its Deps literal in the
	// same edit that adds GroundItems.
	GroundItems GroundItems

	// WildernessIn reports whether a location is in the
	// Wilderness proper (PvP ground, neither safe zone nor
	// safe building). The coordinator wires the world's
	// Wilderness behind it (see cmd/server areaswiring.go);
	// when nil, the drop listener keeps the always-register
	// behavior. See dropItemChecked in drop.go.
	WildernessIn func(game.Location) bool

	// Chat supplies the public-chat listener's world seams
	// (command dispatch, broadcast delivery, nearby players,
	// mute data, and the social-packet intercept). See ChatHooks
	// in chat.go: every field is
	// optional and degrades to a no-op when unset, so a zero
	// Deps still runs the listener as far as its own state
	// reaches. The coordinator wires the fields in
	// handleTurnPacket.
	Chat ChatHooks

	// SocialEmit is the social-packet emit for the five social
	// opcodes (181/196/197/198/199) and the CHAT clan and
	// friends-chat kinds (see social.go). The coordinator wires
	// it to Hub.EmitSocialPacket through the cmd/server adapter;
	// nil means no hooks are wired, and the packet is routed
	// with nothing observing it — the no-hooks default, since
	// TS core takes no action beyond the emit. The bool result
	// (whether a hook claimed the packet) never changes the
	// outcome: dispatch returns Implemented for a routed social
	// packet regardless of handled, because the emit IS the
	// handling. This package does not import internal/plugins:
	// the func field keeps the hub behind the coordinator's
	// adapter, as ChatHooks.SocialIntercept does for public
	// chat.
	SocialEmit func(p *game.Player, pkt game.SocialPacket) bool

	// Examine supplies the definition text for examine answers
	// (see examine.go). The coordinator wires the definition
	// Store behind it; when nil, every examine route drops with
	// its examine reason instead of answering with invented
	// text.
	Examine ExamineSource

	// ObjectExamineIntercept runs the world:object-examine
	// plugin event for an examine_object packet and reports
	// whether a plugin marked it handled, in which case the
	// typed message is suppressed (NetworkBuilder.ts:1291-1300).
	// The coordinator wires the hub's custom-event emit here;
	// nil means no plugin intercepts, the no-hooks default.
	ObjectExamineIntercept func(player *game.Player, objectID int) bool

	// Objects resolves locs for the object-action and
	// item-on-object listeners (see objectaction.go). The
	// coordinator wires the world's ObjectRegistry behind it
	// (adapted to the ObjectLookup interface); when nil, object
	// clicks drop with ReasonObjectAction and item-on-object
	// clicks drop with ReasonUseItem.
	Objects ObjectLookup

	// ObjectDefs resolves loc definitions for the object-action
	// and item-on-object listeners (see objectaction.go). The
	// coordinator wires the definition Store behind it; when
	// nil, the same unwired drops apply as for Objects.
	ObjectDefs ObjectDefinitionSource

	// ObjectHooks carries the object-interaction hub emits for
	// the object-action and item-on-object listeners (see
	// objectaction.go). Every field is optional and degrades
	// to the no-hooks default when unset. The coordinator wires
	// the hub's emits here.
	ObjectHooks ObjectHooks

	// ItemActionHooks carries the item-action hub emits for
	// the item-action listener (see itemaction.go). Every
	// field is optional and degrades to the no-hooks default
	// when unset. The coordinator wires the hub's emits here.
	ItemActionHooks ItemActionHooks

	// NPCs resolves NPCs by world index for the NPC-option
	// listener (see npc.go). The coordinator wires the
	// world's NPC list behind it; when nil, NPC clicks drop
	// with ReasonNpcOption.
	NPCs NpcLookup

	// NpcDefs resolves NPC definitions for the NPC-option
	// listener (see npc.go). The coordinator wires the
	// definition Store behind it; when nil, the same unwired
	// drop applies as for NPCs.
	NpcDefs NpcDefinitionSource

	// NpcHooks carries the NPC-interaction hub emits and the
	// dialogue-start seam for the NPC-option listener (see
	// npc.go). Every field is optional and degrades to the
	// no-hooks default when unset. The coordinator wires the
	// hub's operated emit and Worker B's playback entry
	// point here.
	NpcHooks NpcHooks

	// Shop carries the shop widget/container handler
	// (shopwidget.go): the ShopManager branch of the
	// widget_action dispatch case. A nil handler lets shop
	// clicks fall through to the inventory branch, which drops
	// them with ReasonWidgetAction.
	Shop ShopWidgetHandler

	// Bank carries the bank deposit/move flows (bank.go): the
	// 243/244 deposit and 245 bank-move dispatch cases. A nil
	// handler means no bank flows are wired: the packets are
	// decoded and dropped with ReasonNoAction. The coordinator
	// wires it in cmd/server to the bank flows (Worker B).
	Bank BankActions

	// Dialogue is the dialogue-runtime seam for the
	// dialogue_continue packet and the widget_action
	// dialogue-option tail (see npc.go). The coordinator
	// wires it over Player.GetDialogueManager() once the
	// DialogueManager lands; when nil, resume packets drop
	// with ReasonDialogueUnwired and the widget tail is a
	// no-op.
	Dialogue DialogueAccess

	// Players resolves players by world index for the
	// player-option listener (see player.go). The coordinator
	// wires the world's player list behind it; when nil, the
	// attack option drops with ReasonPlayerOption.
	Players PlayerLookup

	// PlayerHooks carries the player-option hub emits for the
	// player-option listener (see player.go). Every field is
	// optional and degrades to the no-hooks default when
	// unset. The coordinator wires the hub's option-route
	// custom event and player-attack emit here.
	PlayerHooks PlayerHooks

	// CombatAttack starts an attack through the combat system:
	// the Go form of player.getCombat().attack(target)
	// (NPCOptionPacketListener.ts:63,
	// PlayerOptionPacketListener.ts:26). The coordinator wires
	// it in cmd/server to player.GetCombat().Attack (the Phase
	// 16 combat contract), with the listeners' autocast-
	// conditional clear first (when an autocast spell is set,
	// the single-cast spell is cleared:
	// NPCOptionPacketListener.ts:62,
	// PlayerOptionPacketListener.ts:25); the listeners package
	// never names the Combat methods itself, so listener
	// dispatch and tests never depend on the combat
	// implementation. The target is the *game.Mobile of the
	// NPC or player being attacked. When nil, attack clicks
	// drop with their listener reason (ReasonNpcOption /
	// ReasonPlayerOption).
	CombatAttack func(attacker *game.Player, target *game.Mobile)

	// SpellHooks carries the spell seams for the magic
	// listeners (see spellhooks.go): the cache spell-name
	// lookup, the spellbook read, the combat spell cast, the
	// scripted-cast hook emit, the rune/disabled gates, and
	// the tick-task scheduling for delayed spell effects.
	// Every field is optional and degrades to the no-hooks
	// default when unset. The coordinator wires them in
	// cmd/server (cache lookup over
	// CacheDefinitions.GetSpellName, cast over
	// Player.GetCombat().CastSpellOn, task scheduling over
	// World.Tasks().Schedule, the rest over the hub).
	SpellHooks SpellHooks

	// InterfaceHooks carries the interface-action hub emits
	// and the busy-refusal set for the opcode handle, the
	// widget-action/widget-drag custom event, the
	// world-input preamble, and the interface_close case
	// (see interfaceaction.go). Every field is optional and
	// degrades to the no-hooks default when unset. The
	// coordinator wires the hub's emits here.
	InterfaceHooks InterfaceHooks

	// InterfaceSends carries the session-side interface
	// sends the dispatch needs: the input-prompt abort, the
	// close-by-group, the sender-form close, and the
	// interruptible-interface check (see prompts.go). Every
	// field is optional and degrades as documented on
	// InterfaceSends. The coordinator wires the session's
	// interface methods here.
	InterfaceSends InterfaceSends

	// WorldEntityInteractable reports whether a world-entity
	// click reaches plugin handlers: the boat exists, the
	// viewer can see it, and the viewer's root tile is within
	// BOAT_VIEW_DISTANCE of the boat's tile
	// (NetworkBuilder.ts:747-762). The coordinator wires
	// BoatManager.CanInteractWithBoat behind it; nil means no
	// boats are installed, and every click drops silently
	// (noAction), as TS drops a click on a boat that does not
	// exist. See worldentity.go.
	WorldEntityInteractable func(viewer *game.Player, entityIndex int) bool

	// WorldEntityEmit emits the world-entity interaction event
	// to plugin hooks (PluginManager.emitWorldEntityInteraction)
	// and reports whether a hook claimed it. The coordinator
	// wires the hub's emit here; nil means no hooks are wired,
	// and the packet is routed with nothing observing it —
	// the no-hooks default, since TS core takes no action
	// beyond the emit. The bool result never changes the
	// outcome: the emit IS the handling, as for social
	// packets. This package does not import internal/plugins:
	// the func field keeps the hub behind the coordinator's
	// adapter. See worldentity.go.
	WorldEntityEmit func(ev *WorldEntityInteractionEvent) bool

	// SetHelmHeading applies an OSRS SetHeading (0-15) from
	// the helm (NetworkBuilder.ts:741-743:
	// BoatManager.setHelmHeading). The coordinator wires the
	// BoatManager behind it; nil means no boats are
	// installed, and the packet drops silently. See
	// worldentity.go.
	SetHelmHeading func(player *game.Player, packedHeading int)

	// UnresolvedInventoryWarned is the per-connection warned flag
	// for unresolved main-inventory widget actions
	// (NetworkBuilder.ts:1093-1101: warn once + send the reload
	// message, only for the main-inventory widget uid). The
	// coordinator allocates one bool per entering player and
	// shares it across that player's packets through the Deps
	// value captured by its TurnHandler closure. A nil pointer
	// (a stateless Deps) keeps the old behavior: the click
	// drops with ReasonWidgetAction and no message is sent.
	UnresolvedInventoryWarned *bool
}

// Outcome is the result of dispatching one turn packet.
type Outcome struct {
	Recognized  bool       // opcode belongs to a known listener
	Implemented bool       // a game action was taken (state changed or a walk-to started)
	Reason      DropReason // "" when Implemented; named reason when decoded-and-dropped
}

// Handle dispatches one turn packet to the listener that owns its
// opcode, decoding the payload exactly as TS does. Implemented
// listeners act: equip/unequip, drop, switch-slot, the item-action
// equip fallback, ground-item pickup (via the walk-to machinery),
// object interaction and item-on-object (via walk-to-object and
// the Deps.ObjectHooks emits), NPC interaction (via walk-to-NPC
// and the Deps.NpcHooks dialogue/interaction seams) and NPC attack
// (via the attack-option gate and the Deps.CombatAttack seam),
// player attack (via the option-route intercept, the combat seam,
// and the player-attack emit), spell-on-NPC/player/item/ground
// (via the spell-id resolution, the game spell registry, the
// combat cast seam, the scripted NPC route, and the alchemy /
// telegrab listeners), dialogue continue/option selection
// (via the Deps.Dialogue seam), public chat (normalization,
// gates, forceChat, and the broadcast through the Deps.Chat seams),
// social-packet routing (decode, the player guard, and the
// Deps.SocialEmit emit for the five social opcodes and the CHAT
// clan/friends-chat kinds, with no further action), bank deposit/move
// (decode, the bank-open gate, and the Deps.Bank seam for the 243/244
// deposit and 245 move cases),
// interface-action dispatch (the opcode handle's gates + the hub
// emit, via the Deps.InterfaceHooks seams, from the widget-action
// fallthrough, the widget-target fallback, local triggers, and
// the dialogue_continue pre-branch), entered-amount/syntax prompt
// answers (the filter + execute + re-arm rule), widget close
// reports and explicit interface closes (via the
// Deps.InterfaceSends seams), world-entity interaction (the
// dispatch-case gates + the hub emit, via the
// Deps.WorldEntityInteractable/WorldEntityEmit seams, with
// the WORLD_INTERACTIONS preamble and the click-run override)
// and the helm's set_heading (via the Deps.SetHelmHeading
// seam), and the WORLD_INTERACTIONS preamble
// (the TRADING skip, the player:world-input emit, and
// closeInterruptibleInterfaces) for world-input packets. Every other recognized packet is
// decoded and dropped with a named reason. Handle never panics on any payload (empty,
// truncated, over-long, or arbitrary bytes): a decode failure on a
// known opcode yields Recognized=true with ReasonMalformedPayload,
// and an unknown opcode yields Recognized=false. Handle is safe with
// a nil player: pure drops never dereference p and return their own
// reason, and implemented routes return ReasonNoPlayer after decode
// (TS guards the dispatch cases on `if (this.player)`). Handle runs
// on the tick goroutine via TurnHandler, as TS runs dispatch() on
// the player's turn, so mutating Player state here keeps
// tick-goroutine discipline. It uses only exported game API.
//
// The coordinator wires Handle into handleTurnPacket AFTER the
// WALK/MOVE_GAMECLICK cases; those opcodes are not recognized here.
// Opcodes TS decodes to messages no listener file owns (the
// no-branch "raw" opcodes 50, 195, 255) return Recognized=false.
// The bank opcodes (243-245) ARE recognized: dispatch routes them
// to the bank seam (see the cases). The world-entity opcodes
// (220-225) ARE recognized: dispatch answers them directly
// with the dispatch-case gates + the hub emit (see the cases).
// The social opcodes
// (181/196/197/198/199) ARE recognized: dispatch answers them
// directly with the social-packet emit (see the cases). The
// trade_action opcode (180) IS recognized:
// NetworkBuilder.tradeAction routes it to the Trading system (see
// the case). The examine_npc/examine_object opcodes (9/85)
// ARE recognized: dispatch answers them directly (see the cases).
func Handle(p *game.Player, pkt game.TurnPacket, cycle int64, deps Deps) Outcome {
	drop := func(reason DropReason) Outcome {
		return Outcome{Recognized: true, Reason: reason}
	}
	malformed := func(err error) Outcome {
		_ = err
		return Outcome{Recognized: true, Reason: ReasonMalformedPayload}
	}
	acted := func() Outcome {
		return Outcome{Recognized: true, Implemented: true}
	}
	noAction := func() Outcome {
		return Outcome{Recognized: true, Reason: ReasonNoAction}
	}

	switch pkt.Opcode {
	// Appearance (ChangeAppearancePacketListener).
	case protocol.NativeAppearanceSet:
		if _, err := protocol.DecodeAppearance(pkt.Payload); err != nil {
			return malformed(err)
		}
		return drop(ReasonChangeAppearance)

	// Chat (ChatPacketListener; CommandPacketListener via "::").
	case protocol.ClientChat:
		msg, err := protocol.DecodeChat(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if msg.Clan || msg.MessageType == protocol.ChatMessageFriendsChat {
			// Clan and friends-chat kinds go to the plugin
			// social packet, not to ChatPacketListener
			// (NetworkBuilder.ts lines 894-911): decode, the
			// `if (this.player)` guard, build the packet, emit
			// — and no further action, whatever the emit
			// returns (see social.go). The nil-player drop
			// comes after decode, as for the other social
			// opcodes below.
			if p == nil {
				return drop(ReasonNoPlayer)
			}
			emitSocialPacket(p, socialPacketFromChat(msg), deps)
			return acted()
		}
		if msg.MessageType == protocol.ChatMessageGame {
			// Game-kind chat (kind 1) is ignored by the TS
			// dispatch switch: neither the public nor the
			// friends-chat branch acts on it (NetworkBuilder.ts
			// lines 894-903). No gate refused it and no system
			// is missing; it simply takes no action.
			return noAction()
		}
		if p == nil {
			// TS guards the whole chat case on `if
			// (this.player)` (NetworkBuilder.ts line 895):
			// with no player the packet is skipped silently.
			return drop(ReasonNoPlayer)
		}
		implemented, reason := handleChat(p, msg, deps.Chat)
		if implemented {
			return acted()
		}
		return drop(reason)

	// Social packets (dispatch-owned cases, NetworkBuilder.ts
	// lines 905-911: no listener file owns them). Every case
	// follows the same shape TS does: decode, the `if
	// (this.player)` guard, build the game.SocialPacket, emit
	// through Deps.SocialEmit — and no further action,
	// regardless of whether a hook claimed the packet, because
	// TS core takes no other action (see social.go). A nil
	// SocialEmit is the no-hooks default, not a drop.
	case protocol.ClientClanChannelAction:
		msg, err := protocol.DecodeClanChannelAction(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		emitSocialPacket(p, game.SocialPacket{
			Kind:   game.SocialPacketClanChannelAction,
			Action: msg.Action,
			Name:   msg.Name,
			Value:  msg.Value,
		}, deps)
		return acted()
	case protocol.ClientFriendsChatAction:
		msg, err := protocol.DecodeFriendsChatAction(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		emitSocialPacket(p, game.SocialPacket{
			Kind:   game.SocialPacketFriendsChatAction,
			Action: msg.Action,
			Name:   msg.Name,
			Rank:   msg.Rank,
		}, deps)
		return acted()
	case protocol.ClientPrivateMessage:
		msg, err := protocol.DecodePrivateMessage(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		emitSocialPacket(p, game.SocialPacket{
			Kind:      game.SocialPacketPrivateMessage,
			Recipient: msg.Recipient,
			Text:      msg.Text,
		}, deps)
		return acted()
	case protocol.ClientChatFilter:
		msg, err := protocol.DecodeChatFilter(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		emitSocialPacket(p, game.SocialPacket{
			Kind:        game.SocialPacketChatFilter,
			PublicMode:  msg.PublicMode,
			PrivateMode: msg.PrivateMode,
			TradeMode:   msg.TradeMode,
		}, deps)
		return acted()
	case protocol.ClientReportAbuse:
		msg, err := protocol.DecodeReportAbuse(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		emitSocialPacket(p, game.SocialPacket{
			Kind:   game.SocialPacketReportAbuse,
			Name:   msg.Name,
			RuleID: msg.RuleID,
			Mute:   msg.Mute,
		}, deps)
		return acted()

	// Examine (dispatch-owned cases, NetworkBuilder.ts lines
	// 1283-1306: no listener file owns them). TS guards both on
	// `if (this.player)`; the nil-player drop comes after decode,
	// as for the chat case.
	case protocol.NativeExamineNpc:
		msg, err := protocol.DecodeExamineNpc(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		if examineNpc(p, msg.ID, deps.Examine) {
			return acted()
		}
		return drop(ReasonExamineUnwired)
	case protocol.NativeExamineLoc:
		msg, err := protocol.DecodeExamineLoc(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		// The world:object-examine plugin event runs first
		// (lines 1292-1300): a handled event suppresses the
		// answer entirely. Nil means the no-hooks default.
		if deps.ObjectExamineIntercept != nil && deps.ObjectExamineIntercept(p, msg.ID) {
			return noAction()
		}
		if examineObject(p, msg.ID, deps.Examine) {
			return acted()
		}
		return drop(ReasonExamineUnwired)

	// NPC options (NPCOptionPacketListener).
	case protocol.NativeOpNpc1Alt, protocol.NativeOpNpc2, protocol.NativeOpNpc3,
		protocol.NativeOpNpc4, protocol.NativeOpNpc1:
		msg, err := protocol.DecodeNpcOption(pkt.Opcode, pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "npc_option", msg, deps) {
			return noAction()
		}
		return routeNpcAction(p, msg, cycle, deps, drop, acted, noAction)
	case protocol.NativeOpNpcT:
		msg, err := protocol.DecodeSpellOnNpc(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "spell_on_npc", msg, deps) {
			return noAction()
		}
		// Spell on NPC (NPCOptionPacketListener.castSpell,
		// NPCOptionPacketListener.ts:161-198): implemented
		// (phase 27, npc.go) — spell-id resolution, the
		// 24-tile visibility check, the area canCastOnNpc
		// permission, the combat route through
		// Combat.castSpellOn, and the scripted route with the
		// onSpellOnNpc emit. The click-run override is set
		// first — spell_on_npc is in INTERACTION_CLICKS
		// (NetworkBuilder.ts:246-260), so TS sets the override
		// in the preamble before dispatch. A nil player or
		// queue keeps the pure drop.
		if p != nil {
			if q := p.MovementQueue(); q != nil {
				q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), msg.ModifierFlags))
			}
		}
		return routeSpellOnNpc(p, msg, deps, drop, acted, noAction)

	// World-entity options (no listener file: the
	// world_entity_option case inline in dispatch(),
	// NetworkBuilder.ts:747-762). Pure hub-emit: no
	// before→main→operated dispatch, no walk-to, no gates
	// beyond the three in the dispatch case. Decode runs
	// first, as TS decodes before dispatch; the case is in
	// WORLD_INTERACTIONS, so the preamble runs, and the route
	// sets the click-run override (INTERACTION_CLICKS, with
	// modifier flags 0: the message has no modifierFlags
	// field). See worldentity.go.
	case protocol.NativeOpWorldEntity1, protocol.NativeOpWorldEntity2,
		protocol.NativeOpWorldEntity3, protocol.NativeOpWorldEntity4,
		protocol.NativeOpWorldEntity5, protocol.NativeOpWorldEntity6:
		msg, err := decodeWorldEntityOption(pkt.Opcode, pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "world_entity_option", msg, deps) {
			return noAction()
		}
		return routeWorldEntityAction(p, msg, deps, drop, acted, noAction)

	// Set heading (the set_heading case inline in dispatch(),
	// NetworkBuilder.ts:741-743): the helm's heading, sent by
	// the client in place of a click while steering. Not a
	// WORLD_INTERACTIONS packet: no preamble, no override.
	case protocol.NativeSetHeading:
		heading, err := decodeSetHeading(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		return routeSetHeading(p, heading, deps, drop, acted, noAction)

	// Dialogue continue (no listener file: the
	// dialogue_continue case inline in dispatch(),
	// NetworkBuilder.ts lines 908-951). Decode runs first, as
	// TS decodes before dispatch: a malformed payload drops
	// malformed even with no player attached.
	case protocol.NativeResumePausebutton, protocol.ClientResumePausebutton:
		msg, err := protocol.DecodeDialogueContinue(pkt.Opcode, pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		return routeDialogueResume(p, msg, deps, drop, acted, noAction)

	// Object options (ObjectActionPacketListener).
	case protocol.NativeOpLoc1, protocol.NativeOpLoc2, protocol.NativeOpLoc3,
		protocol.NativeOpLoc4, protocol.NativeOpLoc5:
		msg, err := protocol.DecodeObjectOption(pkt.Opcode, pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "object_option", msg, deps) {
			return noAction()
		}
		return routeObjectAction(p, msg, cycle, deps, drop, acted, noAction)
	case protocol.ClientLocInteract:
		msg, err := protocol.DecodeLocInteract(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "object_option", msg, deps) {
			return noAction()
		}
		return routeObjectAction(p, msg, cycle, deps, drop, acted, noAction)

	// Player options (PlayerOptionPacketListener; option 1 attacks,
	// option 2 is the trade request, option 3 is follow, options
	// 4-8 are plugin-owned). The route sets the click-run override
	// first (player_option is in INTERACTION_CLICKS); options 1-2
	// are implemented (phases 16-17) and options 3-8 drop with
	// their listener reasons.
	case protocol.NativeOpPlayer1, protocol.NativeOpPlayer2, protocol.NativeOpPlayer3,
		protocol.NativeOpPlayer4, protocol.NativeOpPlayer5, protocol.NativeOpPlayer6,
		protocol.NativeOpPlayer7, protocol.NativeOpPlayer8:
		opt, err := protocol.DecodePlayerOption(pkt.Opcode, pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "player_option", opt, deps) {
			return noAction()
		}
		return routePlayerOption(p, opt, deps, drop, acted, noAction)
	case protocol.ClientInteract:
		opt, err := protocol.DecodeInteract(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "player_option", opt, deps) {
			return noAction()
		}
		return routePlayerOption(p, opt, deps, drop, acted, noAction)
	case protocol.ClientTradeAction:
		action, err := protocol.DecodeTradeAction(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		return routeTradeAction(p, action, drop, acted, noAction)

	// Item use on entities (UseItemPacketListener) and spell on
	// object (UseItemPacketListener.spellOnObject).
	case protocol.NativeOpPlayerU:
		msg, err := protocol.DecodeItemOnPlayer(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "item_on_player", msg, deps) {
			return noAction()
		}
		return drop(ReasonUseItem)
	case protocol.NativeOpNpcU:
		msg, err := protocol.DecodeItemOnNpc(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "item_on_npc", msg, deps) {
			return noAction()
		}
		// Item on NPC (UseItemPacketListener.itemOnNpc, walkToEntity
		// + unfired onItemOnNpc/onNpcRoute — NOT STARTED, see
		// useitem.go): decode-drop, but the click-run override is
		// set first — item_on_npc is in INTERACTION_CLICKS
		// (NetworkBuilder.ts:246-260), so TS sets the override in
		// the preamble before dispatch. A nil player or queue
		// keeps the pure drop.
		if p != nil {
			if q := p.MovementQueue(); q != nil {
				q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), msg.ModifierFlags))
			}
		}
		return drop(ReasonUseItem)
	case protocol.NativeOpLocU:
		msg, err := protocol.DecodeItemOnObject(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "item_on_object", msg, deps) {
			return noAction()
		}
		return routeItemOnObject(p, msg, cycle, deps, drop, acted, noAction)
	case protocol.NativeOpObjU:
		msg, err := protocol.DecodeItemOnGround(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "item_on_ground", msg, deps) {
			return noAction()
		}
		return drop(ReasonUseItem)
	case protocol.NativeOpLocT:
		msg, err := protocol.DecodeSpellOnObject(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "spell_on_object", msg, deps) {
			return noAction()
		}
		// Spell on object (UseItemPacketListener.spellOnObject,
		// UseItemPacketListener.ts:202-241): the route walks to
		// the object, re-checks it still stands on arrival,
		// and emits onSpellOnObject — the emit is the action,
		// an unclaimed cast has no fallback. The click-run
		// override is set inside the route (the
		// INTERACTION_CLICKS preamble + the item-on-object
		// precedent).
		return routeSpellOnObject(p, msg, cycle, deps, drop, acted, noAction)
	case protocol.ClientInventoryUseOn:
		msg, err := protocol.DecodeInventoryUseOn(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "inventory_use_on", msg, deps) {
			return noAction()
		}
		return routeInventoryUseOn(p, msg, cycle, deps, drop, acted, noAction)

	// Spell on player (MagicOnPlayerPacketListener) and spell on
	// ground item (MagicOnItemPacketListener.castGroundItem).
	case protocol.NativeOpPlayerT:
		msg, err := protocol.DecodeSpellOnPlayer(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "spell_on_player", msg, deps) {
			return noAction()
		}
		// Spell on player (MagicOnPlayerPacketListener.cast):
		// implemented (phase 27, player.go) — dead gates,
		// index bounds, self-target reset, spell-id
		// resolution, the magic:spell-on-player custom event,
		// the canCastOnTarget check, and the cast through
		// Combat.castSpellOn. The click-run override IS set:
		// spell_on_player is in INTERACTION_CLICKS
		// (NetworkBuilder.ts:246-260). A nil player or queue
		// keeps the pure drop.
		if p != nil {
			if q := p.MovementQueue(); q != nil {
				q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), msg.ModifierFlags))
			}
		}
		return routeSpellOnPlayer(p, msg, deps, drop, acted, noAction)
	case protocol.NativeOpLocTAlt:
		msg, err := protocol.DecodeSpellOnGround(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "spell_on_ground", msg, deps) {
			return noAction()
		}
		// Spell on ground (MagicOnItemPacketListener.
		// castGroundItem, MagicOnItemPacketListener.ts:481-503,
		// Telekinetic Grab): implemented (phase 27,
		// magicitem.go). The click-run override is set first
		// — spell_on_ground is in INTERACTION_CLICKS
		// (NetworkBuilder.ts:246-260). A nil player or queue
		// keeps the pure drop.
		if p != nil {
			if q := p.MovementQueue(); q != nil {
				q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), msg.ModifierFlags))
			}
		}
		return routeSpellOnGround(p, msg, cycle, deps, drop, acted, noAction)

	// Inventory action (NetworkBuilder.inventoryAction: ItemAction,
	// Equip, and DropItem routing).
	case protocol.ClientInventoryUse:
		msg, err := protocol.DecodeInventoryUse(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "inventory_action", msg, deps) {
			return noAction()
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		return routeInventoryAction(p, int(msg.WidgetID), msg.Slot, msg.ItemID, msg.Option, nil, deps)

	// Inventory move and widget drag (SwitchItemSlotPacketListener).
	case protocol.ClientInventoryMove:
		msg, err := protocol.DecodeInventoryMove(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		if switchItemSlot(p, int(msg.WidgetID), msg.From, msg.To) {
			return acted()
		}
		return noAction()

	// Bank deposit/move (NetworkBuilder.ts:860-878): deposit
	// inventory (243), deposit equipment (244), bank move
	// (245). All three require the bank open; see bank.go.
	case protocol.ClientBankDepositInventory:
		if err := protocol.DecodeBankDepositInventory(pkt.Payload); err != nil {
			return malformed(err)
		}
		return routeBankDeposit(p, false, deps, drop, acted, noAction)
	case protocol.ClientBankDepositEquipment:
		if err := protocol.DecodeBankDepositEquipment(pkt.Payload); err != nil {
			return malformed(err)
		}
		return routeBankDeposit(p, true, deps, drop, acted, noAction)
	case protocol.ClientBankMove:
		msg, err := protocol.DecodeBankMove(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		return routeBankMove(p, msg, deps, drop, acted, noAction)
	case protocol.NativeIfButtond, protocol.ClientIfButtond:
		msg, err := protocol.DecodeWidgetDrag(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		// The player:widget-action plugin event
		// (NetworkBuilder.ts lines 1219-1222): a handled
		// event skips the whole case, including the move.
		// Bank.handleDrag (Bank.ts:607-617) returns false
		// unless bank widgets are open; it is not ported
		// (Worker B), so the drag always falls through to
		// move — as TS falls through when the bank is closed,
		// and as it falls through for a drag whose target is
		// not a bank drag target. move receives the SOURCE
		// widget id, source slot, and target slot
		// (NetworkBuilder.ts lines 1223-1225).
		if emitWidgetAction(p, msg, deps) {
			return noAction()
		}
		if switchItemSlot(p, int(msg.SourceWidgetID), msg.SourceSlot, msg.TargetSlot) {
			return acted()
		}
		return noAction()

	// Ground-item actions (NetworkBuilder.groundItemAction:
	// PickupItem and SecondGroundItemOption routing) and
	// ground-item examine, which decodes to the same message
	// with option "Examine" (opcode 104).
	case protocol.NativeOpObj1, protocol.NativeOpObj2, protocol.NativeOpObj3,
		protocol.NativeOpObj4, protocol.NativeOpObj5, protocol.NativeExamineObj:
		msg, err := protocol.DecodeGroundItemAction(pkt.Opcode, pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "ground_item_action", msg, deps) {
			return noAction()
		}
		return routeGroundItemAction(p, msg, cycle, deps, drop, acted, noAction)
	case protocol.ClientGroundItemAction:
		msg, err := protocol.DecodeGroundItemActionHigh(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if worldInputPreamble(p, "ground_item_action", msg, deps) {
			return noAction()
		}
		return routeGroundItemAction(p, msg, cycle, deps, drop, acted, noAction)

	// Widget actions (NetworkBuilder widget_action dispatch:
	// InterfaceActionClickOpcode, ItemAction, and Equip) and
	// local triggers (InterfaceActionClickOpcode).
	case protocol.NativeIfButton, protocol.NativeIfButtonSub, protocol.NativeIfButtonX,
		protocol.NativeIfButton1, protocol.NativeIfButton2, protocol.NativeIfButton3,
		protocol.NativeIfButton4, protocol.NativeIfButton5, protocol.NativeIfButton6,
		protocol.NativeIfButton7, protocol.NativeIfButton8, protocol.NativeIfButton9,
		protocol.NativeIfButton10:
		msg, err := protocol.DecodeWidgetAction(pkt.Opcode, pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		return routeWidgetAction(p, msg, deps, drop, acted, noAction)
	case protocol.ClientWidgetAction:
		msg, err := protocol.DecodeWidgetActionHigh(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		if p == nil {
			return drop(ReasonNoPlayer)
		}
		return routeWidgetAction(p, msg, deps, drop, acted, noAction)
	case protocol.NativeIfButtont:
		msg, err := protocol.DecodeWidgetTarget(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		return routeWidgetTarget(p, msg, cycle, deps, drop, acted)
	case protocol.NativeIfTriggeroplocal:
		msg, err := protocol.DecodeLocalTrigger(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		return routeLocalTrigger(p, msg, deps, drop, acted, noAction)

	// Entered-amount / entered-syntax prompt answers (no
	// listener file: the dialogue_amount / dialogue_input
	// cases inline in dispatch(), NetworkBuilder.ts lines
	// 954-975). Decode runs first, as TS decodes before
	// dispatch: a malformed payload drops malformed even
	// with no player attached.
	case protocol.ClientResumeCountdialog:
		msg, err := protocol.DecodeDialogueAmount(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		return routeDialogueAmount(p, msg, drop, acted, noAction)
	case protocol.ClientResumeNamedialog, protocol.ClientResumeStringdialog:
		msg, err := protocol.DecodeDialogueInput(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		return routeDialogueInput(p, msg, drop, acted, noAction)

	// Widget open/close reports (no listener file: the
	// widget case inline in dispatch(), NetworkBuilder.ts
	// line 1170). Only the close action acts.
	case protocol.ClientWidget:
		msg, err := protocol.DecodeWidget(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		return routeWidgetClose(p, msg, deps, drop, acted, noAction)

	// Explicit interface close (no listener file: the
	// interface_close case inline in dispatch(),
	// NetworkBuilder.ts lines 1229-1259). The packet carries
	// no fields; a non-empty payload is malformed.
	case protocol.NativeIfClose:
		if err := protocol.DecodeInterfaceClose(pkt.Payload); err != nil {
			return malformed(err)
		}
		return routeInterfaceClose(p, deps, drop, acted, noAction)

	// Run toggle (no listener file: the varp_transmit case inline in
	// dispatch(), NetworkBuilder.ts lines 1147-1168). Decode runs
	// first, as TS decodes before dispatch: a malformed payload
	// drops malformed even with no player attached.
	case protocol.ClientVarpTransmit:
		msg, err := protocol.DecodeVarpTransmit(pkt.Payload)
		if err != nil {
			return malformed(err)
		}
		// Combat style (NetworkBuilder.ts lines 1152-1154):
		// changeCombatStyle + BonusManager.update. Bonuses
		// compute live through game.ComputeBonuses in Go, so
		// only the style change runs; the audio-setting
		// application for this varp (line 1167) still needs the
		// audio system and stays dropped with it.
		if msg.VarpID == 43 {
			if p == nil {
				return drop(ReasonNoPlayer)
			}
			game.ChangeCombatStyle(p, int(msg.Value))
			return acted()
		}
		// Auto-retaliate (NetworkBuilder.ts lines 1155-1160):
		// setAutoRetaliate + stopAutoRetaliation + sendConfig
		// echo. The audio-setting application (line 1167) stays
		// dropped as above.
		if msg.VarpID == 172 {
			if p == nil {
				return drop(ReasonNoPlayer)
			}
			game.ApplyAutoRetaliate(p, int(msg.Value))
			return acted()
		}
		if msg.VarpID != 173 {
			// Every other varpId's audio-setting application
			// needs the audio system; see
			// ReasonVarpTransmitOther.
			return drop(ReasonVarpTransmitOther)
		}
		if p == nil {
			// TS guards the whole case on `if (this.player)`
			// (NetworkBuilder.ts line 1148): with no player the
			// toggle is skipped silently. Player.SendRunEnergy
			// is nil-session-safe but not nil-receiver-safe,
			// so the guard must come before any method call
			// on p.
			return drop(ReasonNoPlayer)
		}
		// The toggle, exactly as TS does it
		// (NetworkBuilder.ts lines 1161-1165): setRunning(value
		// !== 0 && getRunEnergy() > 0), then sendRunStatus() —
		// the run-energy packet with live units, live running
		// state, live weight, and stamina 0 (sendRunStatus runs
		// the shared sendRunEnergyPacket, PacketSender.ts lines
		// 374-410; the status modifiers start from 0 stamina
		// ticks). The packet is sent unconditionally in this
		// branch, even when zero energy blocks the toggle: a
		// zero-energy "run on" click still refreshes the orb.
		// Handle runs on the tick goroutine via TurnHandler, as
		// TS runs dispatch() on the player's turn, so mutating
		// Player state here keeps tick-goroutine discipline.
		// NOT emitted: the `player:run-toggled` plugin event
		// (NetworkBuilder.ts lines 1164-1165) — no plugin system.
		// The `player:varp-transmit` event with its `handled`
		// short-circuit (lines 1149-1151) is likewise omitted:
		// with no plugins nothing handles the transmission, so
		// dispatch always proceeds.
		// NOT applied: applyAudioSettingFromClient for varpId
		// 173 (line 1167) — no audio-settings/Music system; see
		// ReasonVarpTransmitOther.
		p.SetRunning(msg.Value != 0 && p.GetRunEnergy() > 0)
		p.SendRunEnergy(p.GetRunEnergyUnits(), p.IsRunning(), p.GetWeightKg(), 0)
		return acted()

	default:
		return Outcome{}
	}
}

// routeInventoryAction is NetworkBuilder.inventoryAction
// (NetworkBuilder.ts:1754-1784): the shared router for
// INVENTORY_USE packets and for widget_action inventory clicks
// (which arrive with the resolved option and option index).
// widgetID is the interface the click arrived on: 3214 for
// INVENTORY_USE (fixed by the decoder), the full widget uid for
// widget clicks. option is the raw option text ("" when the
// packet carried none); optionIndex is the packet's option index
// when one was supplied (widget path only — INVENTORY_USE has no
// option-index field) and nil otherwise. p is non-nil: Handle
// guards the nil case before routing.
func routeInventoryAction(p *game.Player, widgetID, slot, itemID int, option string, optionIndex *int, deps Deps) Outcome {
	// Slot/id check (line 1756): the slot must hold the named
	// item. A zero-value player has no inventory; playerReady
	// keeps the check from panicking the tick goroutine.
	if !playerReady(p) || p.Inventory().ItemAt(slot).ID() != itemID {
		return Outcome{Recognized: true, Reason: ReasonNoAction}
	}
	optionLower := strings.ToLower(option)
	// Option-index computation (lines 1758-1761): a non-empty,
	// non-examine option resolves against the item's inventory
	// actions; otherwise the packet's index stands. With no
	// action source installed the ObjType defaults resolve
	// (Drop at index 5); custom verbs then read index 0 and
	// return below, which is the same no-action outcome the
	// plugin gap would produce.
	if optionLower != "" && optionLower != "examine" {
		index := findActionIndex(inventoryActions(actionSource(p), itemID), optionLower) + 1
		optionIndex = &index
	}
	// Wield branch (lines 1763-1772): an exact wield verb on a
	// wearable item equips, after the teleport/dead gates. The
	// teleport gate degenerates to pass (no teleporting state
	// exists). clearPendingAction(true) cancels the pending
	// walk-to; its combat/following/plugin parts are gaps (see
	// itemaction.go). Equip is called with interface 3214, as
	// in TS, whatever interface the click arrived on.
	if wieldOption(optionLower) && equipmentSlotOf(p, itemID) >= 0 {
		if hitpoints(p) <= 0 {
			return Outcome{Recognized: true, Reason: ReasonNoAction}
		}
		p.MovementQueue().CancelWalkTo()
		if equip(p, itemID, slot, game.InventoryInterfaceID) {
			return Outcome{Recognized: true, Implemented: true}
		}
		return Outcome{Recognized: true, Reason: ReasonNoAction}
	}
	// Unknown option verb (line 1773): findIndex missed, index
	// 0, silent return.
	if optionIndex != nil && *optionIndex == 0 {
		return Outcome{Recognized: true, Reason: ReasonNoAction}
	}
	if isDropOption(optionLower, optionIndex) {
		// Drop branch (lines 1774-1777): handleAction first;
		// when it does not handle the click, drop. TS calls
		// drop whenever handleAction returns false — including
		// gate refusals — so every non-handled result falls
		// through to drop here. clickType defaults to 5.
		clickType := 5
		if optionIndex != nil {
			clickType = *optionIndex
		}
		if handleAction(p, widgetID, itemID, slot, clickType, option, deps.ItemActionHooks) == itemActionHandled {
			return Outcome{Recognized: true, Implemented: true}
		}
		switch dropItemChecked(p, itemID, game.InventoryInterfaceID, slot, deps.GroundItems, deps.WildernessIn) {
		case dropDone:
			return Outcome{Recognized: true, Implemented: true}
		case dropDestroy:
			return Outcome{Recognized: true, Reason: ReasonDestroyInterface}
		case dropNoGroundItems:
			return Outcome{Recognized: true, Reason: ReasonGroundItemsUnwired}
		default:
			return Outcome{Recognized: true, Reason: ReasonNoAction}
		}
	}
	if optionLower == "examine" {
		// Examine (lines 1778-1779): examineItem through the
		// chat-message encoder, with the examined stack's
		// amount (used only by the price line, a gap — see
		// examineItem in examine.go).
		if examineItem(p, itemID, p.Inventory().ItemAt(slot).Amount(), deps.Examine) {
			return Outcome{Recognized: true, Implemented: true}
		}
		return Outcome{Recognized: true, Reason: ReasonItemExamine}
	}
	// Everything else (lines 1780-1782): handleAction with
	// clickType defaulting to 1 (first action). A handled
	// result means the equip fallback ran; a plugin result
	// means no plugin handled the click; a refusal means a
	// gate (dead player, changed item) refused it.
	clickType := 1
	if optionIndex != nil {
		clickType = *optionIndex
	}
	switch handleAction(p, widgetID, itemID, slot, clickType, option, deps.ItemActionHooks) {
	case itemActionHandled:
		return Outcome{Recognized: true, Implemented: true}
	case itemActionRefused:
		return Outcome{Recognized: true, Reason: ReasonNoAction}
	default:
		return Outcome{Recognized: true, Reason: ReasonItemAction}
	}
}

// routeGroundItemAction is NetworkBuilder.groundItemAction
// (NetworkBuilder.ts:1832-1866) plus the click-run-override
// preamble (lines 713-719): every ground-item click except an
// examine replaces the player's click run choice before routing.
// The drop/acted/noAction closures are Handle's outcome builders.
func routeGroundItemAction(p *game.Player, msg protocol.GroundItemAction, cycle int64, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	option := ""
	if msg.HasOption {
		option = strings.ToLower(msg.Option)
	}
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if option == "examine" {
		// Examine (lines 1836-1839): examineItem through the
		// chat-message encoder. Examine clicks do not set the
		// click-run override. The amount is 1: the examined
		// stack-amount lookup (examinedGroundStackAmount)
		// feeds only the price line, a gap (see examineItem),
		// so no ground-item resolution runs here.
		if examineItem(p, msg.ItemID, 1, deps.Examine) {
			return acted()
		}
		return drop(ReasonGroundItemExamine)
	}
	// Click-run override preamble: set for every non-examine
	// ground-item click, before routing, even when routing
	// then refuses the packet. game.ClickRunOverride carries
	// the documented energy degeneration (see movement.go).
	// The nil-queue guard only matters for a zero-value Player
	// (NewPlayer always installs a queue); downstream guards
	// refuse the packet for such a player.
	if q := p.MovementQueue(); q != nil {
		q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), msg.ModifierFlags))
	}
	if option == "take" {
		// Take (lines 1840-1843): straight to pickup, whatever
		// the option index says.
		return routePickup(p, msg, cycle, deps, drop, acted, noAction)
	}
	actions := groundActions(actionSource(p), msg.ItemID)
	if option != "" {
		// Named option (lines 1845-1853): the name must match
		// the cache ground actions, and a supplied option
		// number must agree with it. Otherwise the packet
		// returns silently. A match routes to the second
		// ground-item option listener (plugin GAP).
		namedIndex := findActionIndex(actions, option)
		if namedIndex < 0 {
			return noAction()
		}
		index := namedIndex + 1
		if msg.HasOptionIndex {
			index = msg.OptionIndex
		}
		if index != namedIndex+1 {
			return noAction()
		}
		return drop(ReasonSecondGroundItemOption)
	}
	// Unnamed option (lines 1854-1865): the option number
	// defaults to 3. A "take" action — or no action at all on
	// option 3 — picks up; no action on any other option
	// returns silently; a named action routes to the second
	// ground-item option listener (plugin GAP).
	index := 3
	if msg.HasOptionIndex {
		index = msg.OptionIndex
	}
	action := ""
	if index >= 1 && index <= len(actions) {
		action = actions[index-1]
	}
	if strings.EqualFold(action, "take") || (index == 3 && action == "") {
		return routePickup(p, msg, cycle, deps, drop, acted, noAction)
	}
	if action == "" {
		return noAction()
	}
	return drop(ReasonSecondGroundItemOption)
}

// routePickup runs the pickup listener for a routed take. A nil
// ground-item store degrades to ReasonGroundItemsUnwired before
// any lookup; a refused pickup (no operable stack, dead player)
// is ReasonNoAction.
func routePickup(p *game.Player, msg protocol.GroundItemAction, cycle int64, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if deps.GroundItems == nil {
		return drop(ReasonGroundItemsUnwired)
	}
	stackID := 0
	if msg.HasStackID {
		stackID = int(msg.StackID)
	}
	if pickupItem(p, msg.ItemID, msg.X, msg.Y, stackID, msg.HasStackID, cycle, deps.GroundItems) {
		return acted()
	}
	return noAction()
}

// mainInventoryGroupID is MAIN_INVENTORY_GROUP_ID
// (ClientProtocol.ts:2536): the widget group of the main
// inventory, whose clicks the widget_action case routes to the
// inventory-action resolver.
const mainInventoryGroupID = 149

// routeWidgetAction is the ported subset of the widget_action
// dispatch case (NetworkBuilder.ts:975-1130): the
// player:widget-action emit, the equipment-slot branch, the
// shop branch, and the main-inventory branch, with the
// InterfaceActionClickOpcode fallthrough. p is non-nil: Handle
// guards the nil case before routing (TS guards the whole case
// on `if (this.player)`). The drop/acted/noAction closures are
// Handle's outcome builders.
//
// Order is TS order. The player:widget-action plugin event
// (line 977) runs first: a handled event skips the whole case,
// including the dialogue-option tail. The world-map checks
// (lines 988-992) are GAPs with no world-map system; they
// are safe to skip because no world-map widget (group 595, or an
// orbs-interface component) resolves an equipment slot. The
// bank/prayer branches (lines 1025-1089) are GAPs: every
// handler reads a system that does not exist, so each would
// return false and fall through to the inventory branch. The
// spell branch in that range IS implemented (phase 45): the
// onSpellCast emit runs after the autocast branch, then the
// self-cast route (Charge only — other self-cast spells route
// to NOT STARTED handlers). The
// combat branches in that range ARE implemented (phase 26):
// the auto-retaliate button and the style buttons route to
// game.HandleAutoRetaliateButton / game.HandleStyleButton;
// the special-attack buttons stay Worker C and fall through.
// The shop branch in that range IS implemented (phase 17): it
// delegates to deps.Shop, which degrades to a fall-through when
// unwired. The fallthrough calls the opcode handle
// (InterfaceActionClickOpcode.handle): a handled click skips
// the dialogue tail (TS `continue`, line 1132); an unhandled
// click runs the tail, after the simple-click button-click
// fallback (lines 1133-1137, phase 46: onButtonClick /
// onButton hooks for simple IF_BUTTON clicks; a claimed
// button click also skips the tail).
func routeWidgetAction(p *game.Player, msg protocol.WidgetAction, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	// The player:widget-action custom event (lines 977-979):
	// handled skips the whole case, tail included.
	if emitWidgetAction(p, msg, deps) {
		return noAction()
	}
	buttonNum := 0
	if msg.HasButtonNum {
		buttonNum = msg.ButtonNum
	}
	// The dialogue this click can answer: one opened by the
	// click itself must not take the same click as its option
	// (NetworkBuilder.ts:983). The tail runs on every return
	// below except the unresolved-inventory branch, which
	// `continue`s in TS (lines 1093-1101); see
	// dialogueWidgetOption in npc.go.
	dialogueOpen := dialogueCurrentToken(deps, p)
	finish := func(o Outcome) Outcome {
		dialogueWidgetOption(deps, p, dialogueOpen, buttonNum)
		return o
	}
	// Equipment-slot resolution (lines 984-987): group 12 is
	// Bank.MAIN_INTERFACE_ID (Bank.ts:70), and the worn-equipment
	// panel resolves only while the bank is open with button 1.
	// Group 12 is deliberately left unresolved here even though
	// BankIsOpen exists now (phase 46): bank widget clicks
	// route through the interface-action hub to the bank
	// content's (group, child) routing (PHASE46_PLAN.md §4),
	// and the child-to-equipment-slot map for the worn panel
	// is not grounded in the plan. The TS op-1-unequip leg
	// while the bank is open stays a gap for the bank widget
	// work (Worker B).
	equipmentSlot := -1
	if msg.GroupID != 12 {
		equipmentSlot = resolveEquipmentSlot(msg.GroupID, msg.ChildID)
	}
	if equipmentSlot >= 0 {
		// Equipment branch (lines 993-1024). The emitItemAction
		// plugin event is a GAP: with no hooks it returns
		// handled=false. Only op 1 (buttonNum 1) removes the
		// item; op 10 examines; other ops on a worn item send
		// "Nothing interesting happens". The
		// teleport/TRADING gates on unequip degenerate to pass:
		// no teleporting state and no player-status system
		// exist.
		if buttonNum == 1 {
			if unequip(p, equipmentSlot) {
				return finish(acted())
			}
			return finish(noAction())
		}
		if !playerReady(p) {
			return finish(noAction())
		}
		worn := p.Equipment().ItemAt(equipmentSlot)
		if buttonNum == 10 && worn.ID() > 0 {
			// Op 10 examines (lines 1020-1021): examineItem
			// with the worn stack's amount.
			if examineItem(p, worn.ID(), worn.Amount(), deps.Examine) {
				return finish(acted())
			}
			return finish(drop(ReasonItemExamine))
		}
		// Other ops on a worn item with an item present send
		// "Nothing interesting happens." (lines 1022-1023);
		// an empty slot takes no action.
		if worn.ID() > 0 {
			p.SendMessage("Nothing interesting happens.")
			return finish(acted())
		}
		return finish(noAction())
	}

	// Shop branch (lines 1025-1089, ShopManager.handleWidgetAction):
	// group 300/301 clicks route to the shop handler, which
	// answers only while a shop is open for the player; a
	// handled click claims the packet, an unhandled one falls
	// through to the inventory branch. The bank branch before
	// it is handled through the onButtonClick emit (phase 46:
	// the Bank content claims its buttons), and the
	// prayer/spell branches after the combat branches stay
	// GAPs.
	if HandleShopWidgetAction(p, msg, deps.Shop) {
		return finish(acted())
	}

	// Combat branches (NetworkBuilder.ts:1035-1055), in TS
	// order: the auto-retaliate button (593:32), then the
	// special-attack buttons (Worker C — they fall through
	// here), then the style buttons (593:6/10/14/18). A
	// handled click claims the packet; anything else falls
	// through to the inventory branch. The shop branch above
	// and the bank/prayer branches stay as documented; the
	// spell branch is wired just below (the onSpellCast emit
	// after the autocast branch, then the self-cast route).
	if game.HandleAutoRetaliateButton(p, msg.GroupID, msg.ChildID) {
		return finish(acted())
	}
	if game.HandleStyleButton(p, msg.GroupID, msg.ChildID) {
		return finish(acted())
	}
	// Autocast branch (Autocasting.handleWidgetAction): combat
	// interface 593 children 23/26/28 and autocast interface
	// 201 slot picks. A handled click claims the packet; the
	// handler returns false for anything else, so the click
	// falls through to the inventory branch.
	if game.HandleAutocastWidgetAction(p, msg.GroupID, msg.ChildID, msg.Slot) {
		return finish(acted())
	}

	// Spell-cast emit (NetworkBuilder.ts:1060, 1804-1812; phase
	// 45): after the autocast branch, before the self-cast
	// handlers. A claimed click is consumed; an unclaimed one
	// falls through to the self-cast route below. opId is
	// buttonNum ?? opId ?? 1.
	opID := 1
	if msg.HasButtonNum {
		opID = msg.ButtonNum
	} else if msg.HasOpID {
		opID = msg.OpID
	}
	if emitSpellCast(deps, p, int(msg.WidgetID), msg.GroupID, msg.ChildID, msg.ItemID, opID) {
		return finish(acted())
	}

	// Self-cast route (NetworkBuilder.ts:1073-1075): the
	// spell's own handleSelf. SpellTeleports.handleSelf and
	// LunarSpells.handleSelf have no Go form (no teleport
	// system, Lunar not ported); CombatSpells.handleSelf is
	// game.HandleSelfSpell, which today answers only Charge —
	// other self-cast spells route to NOT STARTED handlers.
	if name, ok := resolveSpellName(deps, int(msg.WidgetID), msg.GroupID, msg.ChildID, msg.ItemID); ok && game.HandleSelfSpell(p, name) {
		return finish(acted())
	}

	// Main-inventory branch (lines 1090-1119): group 149 with a
	// slot and item id matching the inventory contents. The
	// decoder uses -1 for an absent slot/item, and IF_BUTTON_SUB
	// passes the slot through raw, so the slot must also be in
	// range: TS reads getItems()[slot]?.getId(), which is
	// undefined out of range and never equals a real item id.
	// A forged packet naming item id -1 drops here rather than
	// resolving an action for a phantom item.
	if msg.GroupID == mainInventoryGroupID && playerReady(p) &&
		msg.Slot >= 0 && msg.Slot < p.Inventory().Capacity() &&
		msg.ItemID >= 0 && p.Inventory().ItemAt(msg.Slot).ID() == msg.ItemID {
		optionIndex, option, ok := resolveInventoryWidgetAction(p, msg.ItemID, buttonNum)
		if !ok {
			// Unresolved inventory action (lines 1093-1101):
			// TS warns once and sends a reload message, but
			// only for the main-inventory widget uid. The
			// console warning has no log contract in this
			// package and is not reproduced; the message is
			// sent once per connection through the Deps
			// warned flag. Any other widget, an
			// already-warned connection, or a stateless Deps
			// drops with ReasonWidgetAction.
			if msg.WidgetID == protocol.MainInventoryWidgetUID && deps.UnresolvedInventoryWarned != nil && !*deps.UnresolvedInventoryWarned {
				*deps.UnresolvedInventoryWarned = true
				p.SendMessage("That backpack action is unavailable. If your client was open during an update, reload and try again.")
				return acted()
			}
			return drop(ReasonWidgetAction)
		}
		// The subOpId plugin emit (lines 1103-1114) is a GAP:
		// with no hooks it returns handled=false and the
		// click proceeds to inventoryAction.
		return finish(routeInventoryAction(p, int(msg.WidgetID), msg.Slot, msg.ItemID, option, &optionIndex, deps))
	}

	// Fallthrough (lines 1121-1136): the opcode handle with
	// the widget uid as the button id and the button number
	// as the action. A handled click SKIPS the dialogue tail
	// (TS `continue`, line 1132: a prompt callback may start
	// a new dialogue, which must not take this same click as
	// its option); an unhandled click runs the tail, after the
	// simple-click button-click fallback (lines 1133-1137:
	// a claimed button click also skips the tail). Absent
	// fields stay absent: the decoder's -1 slot/item mean "no
	// value", a zero subOpId means "no submenu entry", and op
	// id / option carry their own presence flags.
	// IF_BUTTON_SUB passes its slot through raw (65535 stays
	// 65535), as TS does.
	ev := &InterfaceActionClickEvent{
		Player:     p,
		ButtonID:   int(msg.WidgetID),
		Action:     buttonNum,
		GroupID:    msg.GroupID,
		HasGroupID: true,
		ChildID:    msg.ChildID,
		HasChildID: true,
	}
	if msg.HasOpID {
		ev.OpID, ev.HasOpID = msg.OpID, true
	}
	if msg.ItemID >= 0 {
		ev.ItemID, ev.HasItemID = msg.ItemID, true
	}
	if msg.Slot != -1 {
		ev.Slot, ev.HasSlot = msg.Slot, true
	}
	if msg.HasOption {
		ev.Option, ev.HasOption = msg.Option, true
	}
	if msg.SubOpID != 0 {
		ev.SubOpID, ev.HasSubOpID = msg.SubOpID, true
	}
	if handleInterfaceActionClick(ev, deps) {
		return acted()
	}
	// Simple-click button-click fallback (lines 1133-1137):
	// a simple IF_BUTTON click unclaimed by the
	// interface-action hub fires the onButtonClick /
	// onButton hooks; a claimed click skips the dialogue
	// tail, like a handled interface-action click.
	// Non-simple clicks (IF_BUTTON_SUB/X/1-10, widget_action)
	// never reach it.
	if msg.Simple && emitButtonClick(p, int(msg.WidgetID), deps) {
		return acted()
	}
	return finish(noAction())
}

// playerOptionReason maps a decoded player_option number to the
// listener that owns it in PlayerOptionPacketListener.executeClientOption
// (PlayerOptionPacketListener.ts lines 17-40): option 1 attacks,
// option 2 requests a trade, option 3 follows, and options 4-8 go to
// the plugin route (or the generic walk-to-entity + plugin event).
// The option number comes from the opcode (or the INTERACT byte), so
// this split needs no game state.
func playerOptionReason(option int) DropReason {
	switch option {
	case 2:
		return ReasonTradeRequest
	case 3:
		return ReasonFollowPlayer
	default:
		return ReasonPlayerOption
	}
}

// Status is one row of the per-listener status table.
type Status struct {
	Listener string
	Opcodes  string
	Action   string // Action: "implemented" or "decode-drop: <reason>" or "gap: <reason>"
}

// Statuses returns one row per file in net/packet/impl/*, in TS
// filename order, plus one row per dispatch-inline case with no
// listener file: the dialogue prompt answers
// (dialogue_amount/dialogue_input), the interface-close/widget
// dispatch, and the varp_transmit run toggle. Opcodes lists the client
// opcodes whose decoded messages reach the file, including shared
// dispatches and the sub-dispatches with no opcode of their own.
// Implemented rows name the ported behavior and the gaps that remain
// in the same row; decode-drop rows name the missing system. The
// opcode mappings are grounded in the decode branches
// (ClientProtocol.ts lines 508-1032) and the dispatch switch plus
// helpers (NetworkBuilder.ts lines 661-1386, 1754-1876); the
// inventory lists the evidence per row in the phase report.
func Statuses() []Status {
	decodeDrop := func(reason DropReason) string {
		return "decode-drop: " + string(reason)
	}
	return []Status{
		{
			Listener: "ChangeAppearancePacketListener",
			Opcodes:  "37 (APPEARANCE_SET)",
			Action:   decodeDrop(ReasonChangeAppearance),
		},
		{
			Listener: "ChatPacketListener",
			Opcodes:  "190 (CHAT: public/game kinds; clan and friends_chat kinds route to the social-packet emit, ::-prefix routes to Command)",
			Action:   "implemented: text normalization, mute/filter gates, forceChat overhead text, local-list + nearby-supplement broadcast with the live relations gate and autochat types; gaps: chat icons empty until content sets them, mute/social/command/delivery/nearby seams unwired by default (Deps.Chat)",
		},
		{
			Listener: "CommandPacketListener",
			Opcodes:  "none of its own (sub-dispatch of 190 CHAT when the text starts with \"::\")",
			Action:   "implemented: dispatch through the Deps.Chat.Commands seam — the command registry parses, gates, and dispatches; gaps: no core commands exist (all commands are plugin-registered content), rights read from the carried save, no dispatcher wired by default",
		},
		{
			Listener: "DropItemPacketListener",
			Opcodes:  "240 (INVENTORY_USE, drop options) + widget_action inventory path (shared with ItemAction/Equip)",
			Action:   "implemented: hitpoints/interface/slot/id/amount gates, stopSkillable, registers() ground-item registration, slot clear + refresh; gaps: teleport/busy/interface-removal degenerations, DEVELOPER message, plugin drop policy/warning/discarded events, Wilderness (always registers), destroy-item widget flow, sounds",
		},
		{
			Listener: "EquipPacketListener",
			Opcodes:  "240 (INVENTORY_USE, wield options) + widget_action equipment/inventory paths (shared with ItemAction)",
			Action:   "implemented: equip/unequip, 2H/shield rule, stackable merge, requirements via GetUnmetRequirement, appearance via refresh hooks, weapon-interface assign + fight-type/varp 43 update on weapon equip/unequip (WeaponInterfaceManager.assign), bonuses live through game.ComputeBonuses; gaps: resetWeapon special/autocast halves, spec-orb updateBar, following clear, interface-removal branch, refusal messages, plugin can-equip/unequip gates, sounds",
		},
		{
			Listener: "FollowPlayerPacketListener",
			Opcodes:  "46 (OPPLAYER3) + 233 (INTERACT, byte 0), via player_option option 3",
			Action:   decodeDrop(ReasonFollowPlayer),
		},
		{
			Listener: "InterfaceActionClickOpcode",
			Opcodes:  "251 (WIDGET_ACTION) + native 13, 89, 91, 23, 25, 31, 63, 69, 11, 14, 19, 20, 84 (IF_BUTTON family) + 90 (IF_BUTTONT, fallback) + 30 (IF_TRIGGEROPLOCAL) + the dialogue_continue pre-branch (62/252); helper, not a registered listener — called from the widget_action/widget_target/local_trigger/dialogue_continue dispatch cases",
			Action:   "implemented: dead/busy/teleport gates (teleport degenerates to pass: no teleport system), BUSY_MESSAGE refusal via the hub refuseWhileBusy set, prompt-first hub emit (onInterfaceActionClick/onInterfaceActionButton, one list in registration order, handled short-circuit) from the widget-action fallthrough (a handled click skips the dialogue tail), the simple-click button-click fallback (onButtonClick/onButton hooks, phase 46; a claimed button click also skips the tail), the widget-target fallback (inventory-group close first), local triggers (op 1-10 + script triggers with argsData), and the ungated dialogue_continue pre-branch; the player:widget-action emit (widget_action + widget_drag) and the WORLD_INTERACTIONS preamble (TRADING skip, player:world-input emit, closeInterruptibleInterfaces with the preservesInterfaceOnEquip exception) run in dispatch order; gaps: bank/prayer/combat/spell widget branches, Lunar/other-EffectSpells spell-on-item routes, animation-stall hold, creation-menu pre-branch",
		},
		{
			Listener: "ItemActionPacketListener",
			Opcodes:  "240 (INVENTORY_USE) + widget_action inventory path (shared with Equip/DropItem)",
			Action:   "implemented: handleAction/handleFirstAction gates, clearPendingAction walk-to cancellation, equip fallback for wieldables, widget-op resolution, drop-option detection; gaps: plugin item actions and can-use gate, 9520/2545 messages, interface removal",
		},
		{
			Listener: "MagicOnItemPacketListener",
			Opcodes:  "94 (OPLOC_T_ALT, spell_on_ground) + 90 (IF_BUTTONT, spell on inventory item, shared with InterfaceActionClickOpcode)",
			Action:   "implemented: Low/High Alchemy (levels 21/55, nature + fire runes with bypass/infinite-staff filtering, spell-disabled gate, coin payment from alchemy values, 3/5-tick pacing with queue, Magic XP 31/65, click-run override) and Telekinetic Grab (level 33, law + air runes, range 10 with walk-to approach, inventory capacity, delayed landing through the task seam, Magic XP 43, click-run override); gaps: other EffectSpells (Superheat, Bones to Bananas/Peaches, Bake Pie, Vengeance) and the Arceuus reanimations route to NOT STARTED handlers, spell-on-object (opcode 2) routes to onSpellOnObject (phase 45; the emit is the action, no fallback), alchemy confirm dialogue for untradeable items (warning sent, cast refused), 500ms click delay, animations/graphics/projectile/sounds, endSpellbookSwap, sendTab(6)",
		},
		{
			Listener: "MagicOnPlayerPacketListener",
			Opcodes:  "32 (OPPLAYER_T, spell_on_player)",
			Action:   "implemented: dead gates, index bounds, self-target reset, spell-id resolution (NetworkBuilder.resolveClientCombatSpellId), magic:spell-on-player custom event, canCastOnTarget check, cast through Combat.castSpellOn, click-run override; gaps: LunarSpells routing (documented drop), no distance/plane/Wilderness checks here (CombatFactory owns them)",
		},
		{
			Listener: "NPCOptionPacketListener",
			Opcodes:  "76, 12, 34, 70, 57 (npc_option clickType 1-5; 57 is OPNPC1 in the native enum but decodes as option 5, 76 as option 1; 50/OPNPC5 has no decode branch) + 75 (OPNPC_T, spell_on_npc)",
			Action:   "implemented: hitpoints gate (busy() degenerates to it), index lookup, 24-tile same-plane visibility, combat option (attackable/hitpoints gates, attack-option gate, autocast-conditional clear + attack via the combat seam), walk-to-NPC + route hooks, arrival re-check, dialogue-start seam + interaction/operated emits, \"Nothing interesting happens.\" fallback, click-run override; spell-on-NPC (phase 27): spell-id resolution, area canCastOnNpc permission, combat route through Combat.castSpellOn iff attackable, scripted route (walk-to approach + onSpellOnNpc emit with consumeRunes) with \"You can't attack this npc.\" fallback; gaps: LunarSpells routing (documented drop), CrossFloor visibility, projectile line of sight on the scripted route, facing, boats/private areas/owners, onBeforeNpcInteraction table-only",
		},
		{
			Listener: "ObjectActionPacketListener",
			Opcodes:  "96, 28, 42, 38, 51 (object_option clickType 1-5) + 231 (LOC_INTERACT)",
			Action:   "implemented: hitpoints gate (busy() degenerates to it), player-plane location, registry lookup, forPlayer definition, clickType/action-text normalization, walk-to-object + route hooks, still-stands re-check, interaction emit + operated emit, silent no-handler fallback, click-run override; gaps: boat-deck branches, facing, DEVELOPER notice, mounted-decoration reach",
		},
		{
			Listener: "PickupItemPacketListener",
			Opcodes:  "102, 43, 103, 56, 82 (ground_item_action) + 232 (GROUND_ITEM_ACTION), Take routing needs cache groundActions (shared with SecondGroundItemOption)",
			Action:   "implemented: getOperableGroundItem preference order, canOperate, walkToGroundItem arrival, transferToInventory (capacity/credit/partial/deregister), 300ms throttle as a cycle stamp, click-run override; gaps: busy() degeneration, private-area checks, plugin pickup/take/reward/taken events, reach animation, sounds",
		},
		{
			Listener: "PlayerOptionPacketListener",
			Opcodes:  "44, 45, 46, 73, 6, 48, 10, 21 (player_option 1-8) + 233 (INTERACT, options 2-3)",
			Action:   "implemented: option 1 attack (hitpoints/self/target gates, player:option-route intercept, autocast-conditional clear + attack via the combat seam, player-attack emit, click-run override; no distance/plane/Wilderness/multi checks — CombatFactory owns them); option 2 trade request (TradeRequestPacketListener: index/distance/HP/registration gates, plugin request emit, walk-to-entity delivery, busy lines); decode-drop: option 3 follow (ReasonFollowPlayer), options 4-8 plugin-owned (onPlayerOption NOT STARTED, ReasonPlayerOption); gaps: following clear degenerates, route intercept not fired for options 4-8",
		},
		{
			Listener: "SecondGroundItemOptionPacketListener",
			Opcodes:  "102, 43, 103, 56, 82 (ground_item_action) + 232 (GROUND_ITEM_ACTION), non-Take routing needs cache groundActions (shared with PickupItem)",
			Action:   decodeDrop(ReasonSecondGroundItemOption),
		},
		{
			Listener: "SwitchItemSlotPacketListener",
			Opcodes:  "242 (INVENTORY_MOVE) + 1, 253 (IF_BUTTOND widget_drag, shared with Bank.handleDrag)",
			Action:   "implemented: bank rearrange (tab from interface id, search tab 10 when searching, live insert-mode attribute) and inventory swap + refresh for interfaces 3214/5064/1688; gaps: tab reconfiguration",
		},
		{
			Listener: "TradeRequestPacketListener",
			Opcodes:  "45 (OPPLAYER2) + 233 (INTERACT, byte != 0), via player_option option 2",
			Action:   "implemented: index bounds, 20-tile Chebyshev same-plane distance, hitpoints/registration gates, canTrade gate, plugin request emit (handled replacement), walkToEntity delivery with distance re-check, busy lines; gaps: setMobileInteraction/facing, unreachable-NPC branch, pre-step pursuit timing (see trade.go)",
		},
		{
			Listener: "UseItemPacketListener",
			Opcodes:  "65, 36, 86, 79 (item_on_player/npc/object/ground) + 2 (OPLOC_T, spell_on_object) + 241 (INVENTORY_USE_ON)",
			Action:   "implemented: item-on-object (slot/id verification, walk-to-object clickType 0, item-on-object emit, \"Nothing interesting happens.\" fallback) via OPLOCU and the loc kind of INVENTORY_USE_ON, and item-on-item (slot verification, immediate onItemOnItem emit, silent fallthrough) via the inventory kind of INVENTORY_USE_ON since phase 33; decode-drop: item-on-NPC/player (unfired onItemOnNpc/onNpcRoute families, click-run override still set), item-on-ground-item (unfired emit), spell-on-object (magic NOT STARTED, click-run override still set); gap: canUseItem gate degenerates to allow",
		},
		{
			// No listener file: the dialogue_amount /
			// dialogue_input cases inline in dispatch()
			// (NetworkBuilder.ts lines 954-975). Listed
			// with the dispatch-inline rows, after the
			// filename-ordered rows.
			Listener: "DialoguePromptAnswers",
			Opcodes:  "192 (RESUME_COUNTDIALOG) + 193/194 (RESUME_NAMEDIALOG/RESUME_STRINGDIALOG)",
			Action:   "implemented: amount filter (positive, or 0 with acceptsZero, or negative with acceptsNegative), execute, and the re-arm rule (clear only if the action did not replace itself); syntax answers run unconditionally with the same re-arm rule; the prompt-open frames (runClientScript 108/110) and the shop buy/sell-X arming live in the session/shop sender wiring",
		},
		{
			// No listener file: the widget and
			// interface_close cases inline in dispatch()
			// (NetworkBuilder.ts lines 1170, 1229-1259).
			// Listed with the dispatch-inline rows, after
			// the filename-ordered rows.
			Listener: "InterfaceCloseDispatch",
			Opcodes:  "55 (IF_CLOSE) + 250 (WIDGET, close action only; open reports are routed and ignored)",
			Action:   "implemented: interface:close custom event (a handled event still aborts the armed prompt), close-on-interface-close overlay branch, had-something-open check (status/interface id/dialogue/interruptible interface/armed prompt) with the sender-form close, widget close-by-group through the close seam, prompt abort (trigger frame + holder clears + amount Cancel); gaps: nothing-open logout-tab branch (TS TODO, unconfirmed tab-switch mechanism), Bank.abortingPrompt (no bank interface flows)",
		},
		{
			// No listener file: the varp_transmit case inline in
			// dispatch() (NetworkBuilder.ts lines 1147-1168).
			// Listed with the dispatch-inline rows, after the
			// filename-ordered rows.
			Listener: "VarpTransmitRunToggle",
			Opcodes:  "191 (VARP_TRANSMIT): varpId 173 toggles running; other varpIds decode-drop (43 combat style, 172 auto-retaliate, audio settings for every id)",
			Action:   "implemented: setRunning(value != 0 && getRunEnergy() > 0), then send run status; player:run-toggled plugin event and audio-setting application NOT STARTED",
		},
		{
			// No listener file: the social-packet cases
			// inline in dispatch() (NetworkBuilder.ts lines
			// 894-911). Listed last, after the
			// filename-ordered rows.
			Listener: "SocialPacketDispatch",
			Opcodes:  "181 (CLAN_CHANNEL_ACTION) + 196 (FRIENDS_CHAT_ACTION) + 197 (PRIVATE_MESSAGE) + 198 (CHAT_FILTER) + 199 (REPORT_ABUSE) + 190 (CHAT: clan kind 4 and friends_chat kind 2)",
			Action:   "implemented: decode, the player guard, build game.SocialPacket, emit through Deps.SocialEmit (nil = no hooks), no further action regardless of handled — TS core takes no other action; gaps: no core default handling by design (content ports own add-friend/PM/filter behavior), unknown actions decode-fail as malformed (the TS raw return)",
		},
	}
}
