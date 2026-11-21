// Spool unit: the bow string spool
// (server/plugins/skills/fletching/Spool.Fletching.js, 186
// lines, commit bfd81e3). Bow strings are stored on the
// account, not the item, so a lost spool keeps its strings;
// a carried spool strings bows by itself (a virtual bow-string
// source and a trigger alias — registered in shared.go's
// indexRecipes, which runs after Shared's init in the plugin's
// UNITS order).
//
// Capacity: 250 strings, more for each spare spool used on it
// (500, 2,000, 5,000, 10,000 and 100,000 at six spools). The
// client shows varbit bowstring_spool_charges on the buff bar
// while a spool is carried (cache script 5923);
// bowstring_spool_size holds the spare spools added. No capture
// covers the spool (it is newer than the recordings), so its
// messages are TS's own.
package fletching

import (
	"fmt"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- persisted attributes (Spool.Fletching.js:10-11) ----

const (
	// SpoolChargesAttribute is "fletching:spool-charges": the
	// bow strings stored on the account. Owned save state,
	// Api.PersistAttribute, like woodcutting:logs-chopped.
	SpoolChargesAttribute = "fletching:spool-charges"
	// SpoolSparesAttribute is "fletching:spool-spares": the
	// spare spools added to the carried spool.
	SpoolSparesAttribute = "fletching:spool-spares"
)

// CapacityBySpares is the spool capacity by spare spools
// added (Spool.Fletching.js:12, CAPACITY_BY_SPARES).
var CapacityBySpares = [6]int{250, 500, 2000, 5000, 10000, 100000}

// registerSpool attaches the Spool unit's hooks, in TS attach
// order (Spool.Fletching.js attach): the persisted attributes,
// onPlayerLogin, the Bow string spool Fill/Check/Empty actions,
// bow string (or its note) on the spool, spool on spool, and
// the crafting:spun listener (no producer — harmless).
func (f *fletching) registerSpool(api *plugins.Api) {
	api.PersistAttribute(SpoolChargesAttribute, 0)
	api.PersistAttribute(SpoolSparesAttribute, 0)
	api.OnPlayerLogin(f.onSpoolLogin)
	api.OnItemActionNamed("Bow string spool", map[string]plugins.ItemActionHandler{
		"Fill":  f.fillSpoolAction,
		"Check": f.checkSpoolAction,
		"Empty": f.emptySpoolAction,
	}, nil)
	api.OnItemOnItemNamed("Bow string", "Bow string spool", f.stringsOnSpool, nil)
	api.OnItemOnItemNamed("Bow string spool", "Bow string spool", f.combineSpools, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
	api.OnCustomEvent("crafting:spun", f.absorbSpun)
}

// ---- stored counts ----

// spoolCharges reads the stored bow-string count: an int
// attribute, 0 when absent or of another type (the
// woodcutting logsChoppedCount shape).
func spoolCharges(player *game.Player) int {
	if player == nil {
		return 0
	}
	switch v := player.GetAttribute(SpoolChargesAttribute).(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// spoolSpares reads the stored spare-spool count, same shape.
func spoolSpares(player *game.Player) int {
	if player == nil {
		return 0
	}
	switch v := player.GetAttribute(SpoolSparesAttribute).(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// spoolCapacity is the spool capacity by spares (Spool
// .Fletching.js capacity: the tier index clamps at the last).
func spoolCapacity(player *game.Player) int {
	spares := spoolSpares(player)
	if spares < 0 {
		spares = 0
	}
	if spares >= len(CapacityBySpares) {
		spares = len(CapacityBySpares) - 1
	}
	return CapacityBySpares[spares]
}

// sendSpoolVarbits sends the spool varbits: charges and size
// (Spool.Fletching.js sendVarbits). The ids come from the
// tables (Worker A's loader resolves the varbit names).
func (f *fletching) sendSpoolVarbits(player *game.Player) {
	if player == nil || f.tables == nil {
		return
	}
	sendVarbit(player, f.tables.SpoolChargesVarbitID, spoolCharges(player))
	sendVarbit(player, f.tables.SpoolSizeVarbitID, spoolSpares(player))
}

// setSpoolCharges stores a new charge count and re-sends the
// varbits (Spool.Fletching.js setCharges).
func (f *fletching) setSpoolCharges(player *game.Player, value int) {
	if player == nil {
		return
	}
	player.SetAttribute(SpoolChargesAttribute, value)
	f.sendSpoolVarbits(player)
}

// carriesSpool reports whether the player carries a spool
// (Spool.Fletching.js carriesSpool: inventory only).
func (f *fletching) carriesSpool(player *game.Player) bool {
	if player == nil || f.tables == nil {
		return false
	}
	inventory := player.Inventory()
	return inventory != nil && inventory.Contains(f.tables.SpoolID)
}

// spoolSourceCount is the spool's virtual-material count
// (Spool.Fletching.js SOURCE.count): the stored charges, but
// only while a spool is carried.
func (f *fletching) spoolSourceCount(player *game.Player) int {
	if !f.carriesSpool(player) {
		return 0
	}
	return spoolCharges(player)
}

// spoolSourceTake takes up to amount bow strings off the spool
// (Spool.Fletching.js SOURCE.take): 0 without a carried spool,
// else capped at the stored charges, re-sending the varbits.
func (f *fletching) spoolSourceTake(player *game.Player, amount int) int {
	if !f.carriesSpool(player) {
		return 0
	}
	taken := min(amount, spoolCharges(player))
	if taken > 0 {
		f.setSpoolCharges(player, spoolCharges(player)-taken)
	}
	return taken
}

// ---- noted bow string ----

// notedBowStringID resolves a bow string id to its note id
// (Spool.Fletching.js notedBowString: core.ItemDefinition
// .forId(bowStringId).getNoteId()). Go has no NoteID on
// ItemDefinitionSource; the optional ItemNoteIDSource
// capability carries it (the EntTrunk notedLogID precedent:
// the test override wins, else the player's source, else the
// unnoted id when the source lacks the capability or the item
// has no note form). VERIFIED at port time: the production
// adapter implements ItemNoteIDSource over the definition
// Store (definition/item.go NoteID), and the unnoted fallback
// is safe in fill — the plain-string pass already moved every
// plain string, so a second pass over the same id moves 0.
func (f *fletching) notedBowStringID(player *game.Player) int {
	if f == nil || f.tables == nil {
		return 0
	}
	bowStringID := f.tables.BowStringID
	if f.noteID != nil {
		if noted := f.noteID(bowStringID); noted > 0 {
			return noted
		}
		return bowStringID
	}
	if player != nil {
		if src, ok := player.ItemDefinitions().(game.ItemNoteIDSource); ok && src != nil {
			if noted := src.NoteID(bowStringID); noted > 0 {
				return noted
			}
		}
	}
	return bowStringID
}

// ---- Fill / Check / Empty ----

// checkSpoolAction is the spool's Check action (Spool
// .Fletching.js checkSpool).
func (f *fletching) checkSpoolAction(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	sendMessage(event.Player, fmt.Sprintf("Your spool holds %d / %d bow strings.", spoolCharges(event.Player), spoolCapacity(event.Player)))
	return true
}

// fillSpoolAction is the spool's Fill action (Spool
// .Fletching.js fillSpool → fill).
func (f *fletching) fillSpoolAction(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	f.fill(event.Player)
	return true
}

// fill moves bow strings and their notes from the inventory
// onto the spool, up to its capacity (Spool.Fletching.js
// fill).
func (f *fletching) fill(player *game.Player) {
	if player == nil || f.tables == nil {
		return
	}
	inventory := player.Inventory()
	if inventory == nil {
		return
	}
	room := spoolCapacity(player) - spoolCharges(player)
	if room <= 0 {
		sendMessage(player, "Your spool can't hold any more bow strings.")
		return
	}
	added := 0
	for _, id := range []int{f.tables.BowStringID, f.notedBowStringID(player)} {
		moved := min(room, inventory.GetAmount(id))
		if moved <= 0 {
			continue
		}
		inventory.DeleteNumber(id, moved)
		room -= moved
		added += moved
	}
	if added <= 0 {
		sendMessage(player, "You don't have any bow strings to add to the spool.")
		return
	}
	f.setSpoolCharges(player, spoolCharges(player)+added)
	plural := "s"
	if added == 1 {
		plural = ""
	}
	sendMessage(player, fmt.Sprintf("You add %d bow string%s to the spool. It now holds %d.", added, plural, spoolCharges(player)))
}

// emptySpoolAction is the spool's Empty action (Spool
// .Fletching.js emptySpool): an option dialogue asking whether
// to take the strings back as bow strings or as notes.
func (f *fletching) emptySpoolAction(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	if spoolCharges(player) <= 0 {
		sendMessage(player, "Your spool is empty.")
		return true
	}
	if dm := player.GetDialogueManager(); dm != nil {
		dm.StartDialogues(game.NewDialogueChainBuilder().Add(
			game.NewOptionDialogue(0, dialogueOptionActionFunc(func(option game.DialogueOption) {
				// The TS executeOption sends the interface
				// removal first. Content cannot reach the
				// player's DialogueSender (unexported), so the
				// close goes through the manager: Advance from
				// the live option entry starts the next entry,
				// and with no entry following it removes the
				// interface (DialogueManager.Advance).
				// The option values are the resume path's
				// one-based child indices (DialogueOption's
				// resume quirk): 1 is the first option, 2 the
				// second — exactly the values TS compares.
				if dm := player.GetDialogueManager(); dm != nil {
					dm.Advance()
				}
				if option == 1 {
					f.withdraw(player, false)
				}
				if option == 2 {
					f.withdraw(player, true)
				}
			}), "Take them as bow strings.", "Take them as notes.", "Cancel.").SetTitle("Empty the spool?"),
		))
	}
	return true
}

// dialogueOptionActionFunc adapts a plain function to
// game.DialogueOptionAction (no func adapter exists in the
// game package — only DialogueActionFunc for ActionDialogue).
type dialogueOptionActionFunc func(game.DialogueOption)

// ExecuteOption runs the function.
func (fn dialogueOptionActionFunc) ExecuteOption(option game.DialogueOption) {
	if fn != nil {
		fn(option)
	}
}

// withdraw takes stored bow strings back off the spool
// (Spool.Fletching.js withdraw): plain strings honour free
// slots, notes honour the addable amount, and 0 withdrawable
// refuses with "You don't have enough inventory space."
func (f *fletching) withdraw(player *game.Player, noted bool) {
	if player == nil || f.tables == nil {
		return
	}
	inventory := player.Inventory()
	if inventory == nil {
		return
	}
	stored := spoolCharges(player)
	if stored <= 0 {
		sendMessage(player, "Your spool is empty.")
		return
	}
	var amount int
	var id int
	if noted {
		id = f.notedBowStringID(player)
		amount = inventory.GetAddableAmount(game.NewItem(id, stored))
	} else {
		id = f.tables.BowStringID
		amount = min(stored, inventory.GetFreeSlots())
	}
	if amount <= 0 {
		sendMessage(player, "You don't have enough inventory space.")
		return
	}
	inventory.Adds(id, amount)
	f.setSpoolCharges(player, stored-amount)
	plural := "s"
	if amount == 1 {
		plural = ""
	}
	sendMessage(player, fmt.Sprintf("You take %d bow string%s off the spool.", amount, plural))
}

// stringsOnSpool fills the spool from bow strings (or their
// notes) used on it (Spool.Fletching.js stringsOnSpool). It
// claims the use even when fill refuses — the fill messages
// are the answer.
func (f *fletching) stringsOnSpool(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	f.fill(event.Player)
	return true
}

// combineSpools combines a spare spool into the carried one
// (Spool.Fletching.js combineSpools): needs 2 spools in the
// inventory; spares cap at 5 (the sixth tier); the cap
// message fires when already at the cap. It claims the use
// even with fewer than 2 spools, as TS sets handled before
// the count check.
func (f *fletching) combineSpools(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	inventory := player.Inventory()
	if inventory == nil || f.tables == nil {
		return true
	}
	if inventory.GetAmount(f.tables.SpoolID) < 2 {
		return true
	}
	if spoolSpares(player) >= len(CapacityBySpares)-1 {
		sendMessage(player, "Your spool can't be made any bigger.")
		return true
	}
	inventory.DeleteNumber(f.tables.SpoolID, 1)
	player.SetAttribute(SpoolSparesAttribute, spoolSpares(player)+1)
	f.sendSpoolVarbits(player)
	sendMessage(player, fmt.Sprintf("You combine the spools. Your spool can now hold %d bow strings.", spoolCapacity(player)))
	return true
}

// absorbSpun moves spun bow strings onto a carried spool
// (Spool.Fletching.js absorbSpun): only bow strings, only
// while a spool is carried, capped by the amount spun, the
// room left, and the plain strings held. No Go producer exists
// for "crafting:spun" yet (Crafting is unported), so this
// never fires in production.
func (f *fletching) absorbSpun(payload any) {
	event, ok := payload.(*CraftingSpunEvent)
	if !ok || event == nil || event.Player == nil || f.tables == nil {
		return
	}
	player := event.Player
	if event.ItemID != f.tables.BowStringID || !f.carriesSpool(player) {
		return
	}
	inventory := player.Inventory()
	if inventory == nil {
		return
	}
	stored := min(event.Amount, spoolCapacity(player)-spoolCharges(player), inventory.GetAmount(f.tables.BowStringID))
	if stored <= 0 {
		return
	}
	inventory.DeleteNumber(f.tables.BowStringID, stored)
	f.setSpoolCharges(player, spoolCharges(player)+stored)
}

// onSpoolLogin re-sends the spool varbits on login (Spool
// .Fletching.js login).
func (f *fletching) onSpoolLogin(event *plugins.PlayerLoginEvent) {
	if event == nil {
		return
	}
	f.sendSpoolVarbits(event.Player)
}
