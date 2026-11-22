// Small helpers the Crafting units share: statement dialogues,
// the persisted thread counter, varbit access through the
// session, and id lookups over the resolved tables. Each is the
// Go form of a one-liner the TS units repeat (the smithing
// messageBox / sendVarbit and fletching spoolCharges
// precedents).
package crafting

import (
	"strings"

	"tsps-go/internal/game"
)

// statement shows a one-line message box (StatementDialogue,
// the leather/pottery too-low lines).
func statement(player *game.Player, text string) {
	if player == nil {
		return
	}
	if dm := player.GetDialogueManager(); dm != nil {
		dm.StartDialogues(game.NewDialogueChainBuilder().Add(game.NewStatementDialogue(0, text)))
	}
}

// itemStatement shows a one-line message box with an item icon
// (ItemStatementDialogue, the gem/snelm too-low lines and the
// pouch-upgrade line).
func itemStatement(player *game.Player, itemID int, text string) {
	if player == nil {
		return
	}
	if dm := player.GetDialogueManager(); dm != nil {
		dm.StartDialogues(game.NewDialogueChainBuilder().Add(game.NewItemStatementDialogue(0, itemID, text)))
	}
}

// threadUses reads the persisted "crafting:thread-uses"
// counter (Leather.Crafting.js useThread): an int attribute, 0
// when absent or of another type (the fletching spoolCharges
// shape).
func threadUses(player *game.Player) int {
	if player == nil {
		return 0
	}
	switch v := player.GetAttribute(ThreadUsesAttribute).(type) {
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

// varbitReader / varbitSender are the session-side varbit
// surfaces, reached through a type assertion on the player's
// Session (the fletching getVarbit/sendVarbit precedent).
// Player has no GetVarbit/SendVarbit method; a session-less
// player reads 0 and sends nowhere.
type varbitReader interface {
	GetVarbit(id int) int
}

type varbitSender interface {
	SendVarbit(id, value int)
}

func getVarbit(player *game.Player, id int) int {
	if player == nil || player.Session == nil {
		return 0
	}
	if reader, ok := player.Session.(varbitReader); ok {
		return reader.GetVarbit(id)
	}
	return 0
}

func sendVarbit(player *game.Player, id, value int) {
	if player == nil || player.Session == nil {
		return
	}
	if sender, ok := player.Session.(varbitSender); ok {
		sender.SendVarbit(id, value)
	}
}

// itemID resolves an ItemIdentifiers name through the tables,
// or 0 when the tables hold no such name (a fixture that never
// names it). Production tables resolve every name the units
// look up; a 0 here degrades the handler to a no-op, never to
// a wrong item.
func (c *crafting) itemID(name string) int {
	if c == nil || c.tables == nil {
		return 0
	}
	id, _ := c.tables.ItemID(name)
	return id
}

// seqID resolves an animation gameval name through the tables,
// or 0 when the tables hold no such name (no animation).
func (c *crafting) seqID(name string) int {
	if c == nil || c.tables == nil {
		return 0
	}
	return c.tables.SeqIDs[name]
}

// chiselOf is the chisel the player carries, Jeweller's first
// (Gems.Crafting.js chiselOf, shared with Snelms and
// Limestone): 0 when neither is carried.
func (c *crafting) chiselOf(player *game.Player) int {
	if player == nil || player.Inventory() == nil {
		return 0
	}
	inv := player.Inventory()
	if id := c.itemID("JEWELLERS_CHISEL"); id != 0 && inv.Contains(id) {
		return id
	}
	if id := c.itemID("CHISEL"); id != 0 && inv.Contains(id) {
		return id
	}
	return 0
}

// capitalizeFirst capitalizes a recipe name's first letter
// (Pottery.Crafting.js openPotteryMenu's label: name[0].
// toUpperCase() + name.slice(1)).
func capitalizeFirst(name string) string {
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
