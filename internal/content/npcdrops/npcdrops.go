// Package npcdrops is the Go port of the NPC drop system
// (server/plugins/npcs/NpcDrops.plugin.js, 1,320 lines, commit
// bfd81e3). A player kill rolls the dead NPC's drop table and puts
// the loot on the ground at the death tile, owner-only until the
// reveal delay, exactly as TS placeDrops does through the
// ItemOnGroundManager.
//
// Line numbers in comments cite NpcDrops.plugin.js unless another
// file is named.
//
// This package is the INTEGRATION (Worker C, Phase 50): the
// onNpcDeath handler, dropFor / placeDrops / reportLoot, the
// npc-drops:* custom-event emits, and the ::reloaddrops owner
// command. The loader (Worker A, internal/definition) and the
// roller (Worker B, roller.go + types.go in this package) are the
// other two pieces, plugged in behind two small seams:
//
//   - TableSource (TablesForNPC + NPCName): the coordinator
//     installs the loaded drop data through SetTableSource. It
//     returns B's *Table values (types.go), in dump order (first
//     table by default; overrides append second tables after the
//     dump one). NPCName is the drop file's own npcs[].name — the
//     Looting-bag gate needs the NPC name and game.NPCDefinition
//     has no Name(), so the name comes from the drop data rather
//     than the *definition.Store.
//   - TableRoller (RollTable): B's *Roller satisfies it; the
//     coordinator installs the production roller through
//     SetRoller (NewRoller over the loaded shared tables).
//
// The TableSource side is grounded in Worker A's loader
// (internal/definition/npcdrops.go): Store.NpcDropNPC(npcID)
// (*NpcDropNPC, bool) carries Tables (dump order first,
// override-appended after) and Name, and Store.NpcDropTable(npcID)
// returns the table objects. The coordinator's adapter lives in
// cmd/server/npcdropswiring.go: a definition.NpcDropTable ->
// npcdrops.Table converter (field-for-field; A parses into
// NpcDropEntry / NpcDropSharedEntry shapes, B rolls over Entry /
// SharedEntry / SharedTable shapes) plus the shared-table map (A:
// Store.NpcDropSharedTable(name) / Store.NpcDropSeedTable(name)).
// Until the coordinator installs both seams, they are nil and the
// plugin is inert: kills roll nothing (no table source) and the
// reloaddrops command reports that reload is unavailable. That is
// the deliberate parallel-work state, not a bug.
//
// The seams are resolved lazily at kill time (see the npcDrops
// comment below): hub.Load() runs Register before the definition
// Store exists, and installNpcDropsWiring runs after the Store
// loads.
//
// Deviations, named:
//   - TS lazy-loads the ~10 MiB tables on the first NPC death
//     (ensureDrops, :1240-1244); Go loads eagerly at boot (the
//     loader pattern), so there is no ensure step here.
//   - TS dropFor returns early when itemOnGroundManager is null;
//     Go has no such null (the DropGroundItem seam is installed
//     by InstallDeathWiring, cmd/server/combatwiring.go:295-323),
//     so the early return is the nil-seam guard in placeDrops.
//   - TS ids are JS numbers, so the handler falls back to
//     npc.getId() for a non-integer npcId (:1280-1282). Go ids are
//     ints, so there is no non-integer case; the handler guards
//     nil killer / nil NPC only.
//   - TS lootKey starts at 1 (:42); Go matches (atomic counter,
//     first kill reports key 1).
//   - The npc-drops:location event's public:true is accepted in
//     the payload but cannot be honoured: the DropGroundItem seam
//     always places owner-only (goes global at 100 ticks) with no
//     public-from-birth option. Chambers-of-Xeric-style public
//     drops stay owner-only until the seam grows the option.
//   - Ring of Wealth coin auto-collect is NOT implemented (no RoW
//     plugin in Go); currency drops go to the ground like any
//     other drop.
//   - The npc-drops:claim / :table / :add / :roll / :generated /
//     :location emits exist with no consumers yet (this phase
//     builds no diary/defender/shared-loot listeners); the
//     default path is the normal path, TS-faithful.
package npcdrops

import (
	"fmt"
	"sync/atomic"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// LootTrackerScript is loottracker_add_loot
// (NpcDrops.plugin.js:39-40): cache script 7192, sent once per
// dropped item with (npc id, per-kill key, item id, amount).
const LootTrackerScript = 7192

// TableSource is the loader seam (Worker A, via the coordinator's
// adapter): the drop tables for an NPC, in dump order, and the
// NPC's name from the drop file. TablesForNPC returns nil when
// the NPC has no table (Vorkath 8058/8060, Amoxliatl 13686 —
// 0 tables) or is unknown.
type TableSource interface {
	// TablesForNPC returns the tables for npcID in dump order
	// (overrides append second tables after the dump one), or
	// nil when the NPC has no table.
	TablesForNPC(npcID int) []*Table
	// NPCName returns the drop file's npcs[].name for npcID,
	// or "" when unknown.
	NPCName(npcID int) string
}

// TableRoller is the roller seam (Worker B): B's *Roller
// satisfies it (RollTable, roller.go:113). Tests fake it.
type TableRoller interface {
	// RollTable rolls one kill of table and returns the drops.
	RollTable(table *Table, ctx KillContext) []Drop
}

// Reloader reloads the three drop files and reports the loaded
// counts (Worker A's Store reload, installed by the coordinator).
// It returns (npcs, tables, err).
type Reloader func() (npcs, tables int, err error)

var (
	installedSource     TableSource
	installedRoller     TableRoller
	installedReloader   Reloader
	installedWilderness func(game.Location) bool
)

// SetTableSource installs the loader's table source. Install
// before ticking starts; the coordinator calls it once Workers A
// lands (nil clears it).
func SetTableSource(source TableSource) { installedSource = source }

// SetRoller installs the production roller (Worker B's *Roller,
// via NewRoller over the loaded shared tables). Install before
// ticking starts; the coordinator calls it once Worker B lands
// (nil clears it).
func SetRoller(roller TableRoller) { installedRoller = roller }

// SetReloader installs the drop-file reloader for ::reloaddrops.
// Install before ticking starts; the coordinator wires it to the
// loader's reload (nil clears it).
func SetReloader(reload Reloader) { installedReloader = reload }

// SetWildernessChecker installs the death-tile Wilderness test
// (cmd/server wiring, over game.Wilderness.IsInLocation). Install
// before ticking starts; nil means "nowhere is Wilderness", so
// Looting-bag rows never drop — the safe default.
func SetWildernessChecker(check func(game.Location) bool) { installedWilderness = check }

// lootKey is the per-kill loot-tracker key (NpcDrops.plugin.js
// :40-42): starts at 1, increments once per kill that reaches
// reportLoot. Atomic: deaths dispatch on the tick goroutine and
// tests may call reportLoot concurrently.
var lootKey atomic.Int64

func init() { lootKey.Store(1) }

// ClaimEvent is the payload of "npc-drops:claim"
// (NpcDrops.plugin.js:1285-1289): a shared-loot boss takes the
// kill over by setting Handled; the normal path is skipped.
type ClaimEvent struct {
	Killer   *game.Player
	NPC      *game.NPC
	NpcID    int
	Damagers []*game.Player
	Handled  bool
}

// TableEvent is the payload of "npc-drops:table"
// (tableForNpc, :1116-1127): emitted only when the NPC has more
// than one table. A listener picks another table by setting
// Table; nil means the first table. Tables is the dump-order
// list; listeners must not mutate it.
type TableEvent struct {
	Killer *game.Player
	NPC    *game.NPC
	NpcID  int
	Tables []*Table
	Table  *Table
}

// AddEvent is the payload of "npc-drops:add" (dropFor, :1144):
// contextual drops (Catacombs totem pieces/shards) are added
// before anything below reads the drops. Listeners add by
// reassigning Drops (append returns a new slice header); the
// plugin reads Drops back after the emit.
type AddEvent struct {
	Killer *game.Player
	NPC    *game.NPC
	NpcID  int
	Drops  []Drop
}

// RollEvent is the payload of "npc-drops:roll" (dropFor, :1147):
// drops that depend on the killer's progress (Warriors' Guild
// defenders; diary noted-drop marking) are edited in place.
// Listeners edit by reassigning Drops; Damagers is every player
// who damaged the NPC (an Ironman gets nothing when others
// helped). The plugin reads Drops back after the emit.
type RollEvent struct {
	Killer   *game.Player
	NPC      *game.NPC
	NpcID    int
	Drops    []Drop
	Damagers []*game.Player
}

// GeneratedEvent is the payload of "npc-drops:generated"
// (dropFor, :1149-1150): a listener that sets Handled takes the
// generated drops over and NOTHING is placed. The plugin reads
// Drops back after the emit (a handler may also edit them).
type GeneratedEvent struct {
	Killer  *game.Player
	NPC     *game.NPC
	NpcID   int
	Drops   []Drop
	Handled bool
}

// LocationEvent is the payload of "npc-drops:location"
// (dropFor, :1152-1154): a listener may move the drop location
// (Zulrah's body is out of reach — under the killer) or set
// Public (Chambers of Xeric). The plugin reads Location back
// after the emit; Public is accepted but cannot be honoured by
// the DropGroundItem seam (see the package comment).
type LocationEvent struct {
	Killer   *game.Player
	NPC      *game.NPC
	NpcID    int
	Location game.Location
	Public   bool
}

// npcDrops is the plugin's per-registration state: the emit func
// captured at Register (Api.EmitCustomEvent) plus the installed
// seams. The seams are NOT snapshotted here: hub.Load() runs
// Register (cmd/server/server.go:185) before the definition Store
// exists, and installNpcDropsWiring runs after the Store loads — so
// the fields are nil at Register and the accessors below resolve
// them lazily at kill time (field first, then the package-level
// installed seam). This is the Phase 32 hub contract: surfaces
// installed after plugin Load must be resolved lazily, never
// captured in Register. Tests build their own with newNpcDrops and
// override the fields directly.
type npcDrops struct {
	emit       func(name string, payload any)
	source     TableSource
	roller     TableRoller
	reloader   Reloader
	wilderness func(game.Location) bool
}

func newNpcDrops() *npcDrops {
	return &npcDrops{
		emit:       func(string, any) {},
		source:     installedSource,
		roller:     installedRoller,
		reloader:   installedReloader,
		wilderness: installedWilderness,
	}
}

// The seam accessors resolve lazily: a test-injected field wins,
// otherwise the currently installed seam is read at call time. The
// fields above are a Register-time snapshot only — the production
// install happens after Register (see the npcDrops comment) — so a
// nil field must fall through to the installed seam instead of
// staying inert.
func (n *npcDrops) tableSource() TableSource {
	if n.source != nil {
		return n.source
	}
	return installedSource
}

func (n *npcDrops) tableRoller() TableRoller {
	if n.roller != nil {
		return n.roller
	}
	return installedRoller
}

func (n *npcDrops) dropReloader() Reloader {
	if n.reloader != nil {
		return n.reloader
	}
	return installedReloader
}

func (n *npcDrops) wildernessCheck() func(game.Location) bool {
	if n.wilderness != nil {
		return n.wilderness
	}
	return installedWilderness
}

// npcDied handles an NPC death (NpcDrops.plugin.js:1277-1300), in
// TS order: early return with no killer or no NPC (the hub's
// OnNpcDeath wrapper already guards both, so these are the
// direct-call guards); emit npc-drops:claim first — a handled
// claim skips the normal path entirely; otherwise dropFor. TS
// wraps dropFor in a try/catch that logs and swallows errors; Go
// has no such catch — a panicking roller is contained by the
// hub's emitHooks recovery around the hook itself.
func (n *npcDrops) npcDied(event *plugins.NpcDeathEvent) {
	if event == nil || event.Killer == nil || event.NPC == nil {
		return
	}
	claim := &ClaimEvent{
		Killer:   event.Killer,
		NPC:      event.NPC,
		NpcID:    event.NpcID,
		Damagers: event.Damagers,
	}
	n.emit("npc-drops:claim", claim)
	if claim.Handled {
		return
	}
	n.dropFor(event.Killer, event.NPC, event.NpcID, event.Location, event.Damagers)
}

// tableForNpc answers the table a kill rolls (tableForNpc,
// :1116-1127): no table → nil (a kill with no base table can
// still receive contextual drops from npc-drops:add listeners);
// one table → it; multiple tables → emit npc-drops:table so a
// plugin can pick another (no picker exists yet → the first
// table). A listener's non-nil Table wins, as in TS.
func (n *npcDrops) tableForNpc(npcID int, killer *game.Player, npc *game.NPC) *Table {
	source := n.tableSource()
	if source == nil {
		return nil
	}
	tables := source.TablesForNPC(npcID)
	if len(tables) == 0 {
		return nil
	}
	if len(tables) > 1 {
		req := &TableEvent{Killer: killer, NPC: npc, NpcID: npcID, Tables: tables}
		n.emit("npc-drops:table", req)
		if req.Table != nil {
			return req.Table
		}
	}
	return tables[0]
}

// dropFor rolls a kill's drops and puts them down (dropFor,
// :1131-1155), in TS order: roll the base table (or no drops
// when there is none), npc-drops:add, npc-drops:roll,
// npc-drops:generated (handled short-circuits placement —
// nothing is placed and nothing is reported), npc-drops:location,
// placeDrops, reportLoot. It returns the rolled drop count, as
// TS returns drops.length.
func (n *npcDrops) dropFor(killer *game.Player, npc *game.NPC, npcID int, location game.Location, damagers []*game.Player) int {
	var drops []Drop
	if table := n.tableForNpc(npcID, killer, npc); table != nil {
		if roller := n.tableRoller(); roller != nil {
			drops = roller.RollTable(table, n.killContext(killer, npc, npcID, location, damagers))
		}
	}
	add := &AddEvent{Killer: killer, NPC: npc, NpcID: npcID, Drops: drops}
	n.emit("npc-drops:add", add)
	drops = add.Drops
	roll := &RollEvent{Killer: killer, NPC: npc, NpcID: npcID, Drops: drops, Damagers: damagers}
	n.emit("npc-drops:roll", roll)
	drops = roll.Drops
	generated := &GeneratedEvent{Killer: killer, NPC: npc, NpcID: npcID, Drops: drops}
	n.emit("npc-drops:generated", generated)
	if generated.Handled {
		return len(generated.Drops)
	}
	drops = generated.Drops
	where := &LocationEvent{Killer: killer, NPC: npc, NpcID: npcID, Location: location}
	n.emit("npc-drops:location", where)
	n.placeDrops(killer, drops, where.Location)
	n.reportLoot(killer, npcID, drops)
	return len(drops)
}

// killContext builds the roller's context for a kill (B's
// KillContext, types.go): the NPC name from the drop file (the
// Looting-bag gate reads it), the combat level from the NPC
// definition (0 when the definition carries none — the roller
// falls back to the lowest seed category, as TS does for an
// unknown level), the Wilderness test on the death tile, and the
// death tile's map y (the Legends' Quest talisman substitution
// reads MapY >= 6400 as underground).
func (n *npcDrops) killContext(killer *game.Player, npc *game.NPC, npcID int, location game.Location, damagers []*game.Player) KillContext {
	ctx := KillContext{MapY: location.Y}
	if source := n.tableSource(); source != nil {
		ctx.NPCName = source.NPCName(npcID)
	}
	if npc != nil {
		if def := npc.Definition(); def != nil {
			ctx.NPCCombatLevel = def.CombatLevel()
		}
	}
	if check := n.wildernessCheck(); check != nil {
		ctx.InWilderness = check(location)
	}
	return ctx
}

// validDrop reports whether a rolled drop may be spawned
// (validDrop, :1161-1168): an integer id >= 0 and a positive
// amount. Go ids are ints, so the integer check is the >= 0
// bound; a non-positive amount is skipped.
func validDrop(d Drop) bool {
	return d.ItemID >= 0 && d.Amount > 0
}

// droppedItemID resolves the item a drop lands as
// (droppedItemId, :1181-1189): its note when the table drops it
// noted and a note exists. The note-id lookup is grounded, not
// invented: the player's item-definition source carries the
// optional game.ItemNoteIDSource capability (cache Note field,
// via Store.Item(id).NoteID() — internal/definition/item.go),
// the same lookup the Ent trunk's noted-log delivery uses; and
// the note must itself be noted (the TS forId(noteId).isNoted()
// check, read through ItemDefinitionSource.IsNoted). No note,
// or a note id that is not a note, lands unnoted.
func (n *npcDrops) droppedItemID(killer *game.Player, d Drop) int {
	if !d.Noted || killer == nil {
		return d.ItemID
	}
	src := killer.ItemDefinitions()
	if src == nil {
		return d.ItemID
	}
	noteSrc, ok := src.(game.ItemNoteIDSource)
	if !ok || noteSrc == nil {
		return d.ItemID
	}
	noteID := noteSrc.NoteID(d.ItemID)
	if noteID <= 0 || !src.IsNoted(noteID) {
		return d.ItemID
	}
	return noteID
}

// isStackable reads stackability through the killer's
// item-definition source (ItemDefinitionSource.IsStackable), or
// false with no source (the DEFAULT shape is non-stackable).
func (n *npcDrops) isStackable(killer *game.Player, itemID int) bool {
	if killer == nil {
		return false
	}
	src := killer.ItemDefinitions()
	if src == nil {
		return false
	}
	return src.IsStackable(itemID)
}

// placeDrops puts rolled drops on the ground for the killer
// (placeDrops, :1209-1227, minus the Ring of Wealth coin
// auto-collect — not implemented, see the package comment): per
// drop, skip invalid drops; noted drops swap to the note id;
// stackables register as one stack, non-stackables one call per
// single unit — through the existing engine seam
// game.Hooks.DropGroundItem (installed by InstallDeathWiring,
// cmd/server/combatwiring.go:295-323), the same seam woodcutting,
// fishing, cooking and mining already use.
//
// Seam limits, stated (not worked around): the seam drops
// silently when the owner is nil; it always places owner-only
// with goesGlobal=true and respawnTimer=-1, so there is no
// public-from-birth placement and no per-drop despawn override;
// the npc-drops:location Public flag is therefore accepted and
// ignored.
func (n *npcDrops) placeDrops(killer *game.Player, drops []Drop, location game.Location) {
	if killer == nil || game.Hooks.DropGroundItem == nil {
		return
	}
	dropGround := game.Hooks.DropGroundItem
	for _, d := range drops {
		if !validDrop(d) {
			continue
		}
		id := n.droppedItemID(killer, d)
		if n.isStackable(killer, id) {
			dropGround(game.NewItem(id, d.Amount), location, killer)
		} else {
			for i := 0; i < d.Amount; i++ {
				dropGround(game.NewItem(id, 1), location, killer)
			}
		}
	}
}

// reportLoot tells the killer's client what the kill dropped
// (reportLoot, :1170-1179): client script 7192
// (loottracker_add_loot) once per valid drop, with the NPC id, a
// per-kill key (increments once per kill that reaches here),
// the landed (possibly noted) item id and the amount.
//
// Send path, grounded: Go has no Player.SendClientScript —
// client scripts go through the session's DialogueSender
// (internal/game/dialogue_entries.go:122, SendClientScript),
// reached by asserting the player's Session, exactly the
// bankDialogueSender pattern (internal/game/bankstate.go:469).
// The real session implements it
// (internal/session/dialogue_sender.go:74, encodeRunClientScript).
// A session-less player (tests, bots) sends nothing. Whether the
// TS client renders script 7192's loot-tracker entry is NOT
// verified here — the frame is sent; rendering is the client's.
func (n *npcDrops) reportLoot(killer *game.Player, npcID int, drops []Drop) {
	if killer == nil {
		return
	}
	sender, _ := killer.Session.(game.DialogueSender)
	if sender == nil {
		return
	}
	key := int(lootKey.Add(1) - 1)
	for _, d := range drops {
		if !validDrop(d) {
			continue
		}
		sender.SendClientScript(LootTrackerScript,
			game.ClientScriptInt(npcID),
			game.ClientScriptInt(key),
			game.ClientScriptInt(n.droppedItemID(killer, d)),
			game.ClientScriptInt(d.Amount))
	}
}

// reloadDrops is the ::reloaddrops OWNER command
// (NpcDrops.plugin.js:1301-1315): reload the three drop files
// and report the counts, or the error line when the reload
// fails. With no reloader installed (coordinator has not wired
// the loader yet) it says so instead of failing. Registration
// follows the ::item pattern (internal/content/admincommands):
// api.RegisterCommand with the rights gate.
func (n *npcDrops) reloadDrops(e *game.CommandEvent) bool {
	if e == nil || e.Player == nil {
		return false
	}
	reloader := n.dropReloader()
	if reloader == nil {
		e.Player.SendMessage("Drops reload is not available.")
		return true
	}
	npcs, tables, err := reloader()
	if err != nil {
		e.Player.SendMessage("Error reloading npc drops.")
		return true
	}
	e.Player.SendMessage(fmt.Sprintf("Reloaded drops: %d npcs, %d tables.", npcs, tables))
	return true
}

// Plugin is the registered NpcDrops plugin value: name and
// description from the plugin file (NpcDrops.plugin.js:1-8).
var Plugin = plugins.Plugin{
	Name:        "NpcDrops",
	Description: "NPC drops: rolls drop tables on player kills, places the loot on the ground, and reports it to the loot tracker.",
	Register: func(api *plugins.Api) {
		inst := newNpcDrops()
		inst.emit = api.EmitCustomEvent
		api.OnNpcDeath(inst.npcDied)
		api.RegisterCommand("reloaddrops", inst.reloadDrops, game.RightsOwner, "Reload NPC drops")
	},
}

func init() {
	plugins.Register(Plugin)
}
