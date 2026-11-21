// Dialogue helpers shared by the item-3 units (Godswords,
// DragonShields): the messageBox/itemBox forms with a
// continuation (Common.Smithing.js messageBox/itemBox with
// `then`), the restricted-item check, and the option-dialogue
// action adapter. The plain messageBox lives in common.go;
// these belong there when the workers merge — they sit here
// so item 3 does not edit the core-loops worker's files.
package smithing

import (
	"tsps-go/internal/game"
)

// dialogueOptionActionFunc adapts a plain function to
// game.DialogueOptionAction (the fletching spool precedent:
// no func adapter exists in the game package).
type dialogueOptionActionFunc func(game.DialogueOption)

// ExecuteOption runs the function.
func (fn dialogueOptionActionFunc) ExecuteOption(option game.DialogueOption) {
	if fn != nil {
		fn(option)
	}
}

// restricted reports whether an item is a preset (or
// meta-untradeable, unbankable) copy: it must never come out
// as part of a fresh, tradeable product
// (Common.Smithing.js restricted).
func restricted(item game.Item) bool {
	return item.IsValid() && (item.IsPresetItem() || item.IsUntradeable() || item.IsUnbankable())
}

// itemBox shows an item statement dialogue
// (Common.Smithing.js itemBox).
func itemBox(player *game.Player, itemID int, text string) {
	itemBoxThen(player, itemID, text, nil)
}

// itemBoxThen shows an item statement dialogue, running then
// on continue (Common.Smithing.js itemBox with a
// continuation): the chain resets the dialogue manager
// before running then, as TS's ActionDialogue does.
func itemBoxThen(player *game.Player, itemID int, text string, then func()) {
	if player == nil {
		return
	}
	dm := player.GetDialogueManager()
	if dm == nil {
		return
	}
	chain := game.NewDialogueChainBuilder().Add(game.NewItemStatementDialogue(0, itemID, text))
	if then != nil {
		chain.Add(game.NewActionDialogue(1, game.DialogueActionFunc(func() {
			if dm := player.GetDialogueManager(); dm != nil {
				dm.Reset()
			}
			then()
		})))
	}
	dm.StartDialogues(chain)
}

// messageBoxThen shows a statement dialogue, running then on
// continue (Common.Smithing.js messageBox with a
// continuation).
func messageBoxThen(player *game.Player, text string, then func()) {
	if player == nil {
		return
	}
	dm := player.GetDialogueManager()
	if dm == nil {
		return
	}
	chain := game.NewDialogueChainBuilder().Add(game.NewStatementDialogue(0, text))
	if then != nil {
		chain.Add(game.NewActionDialogue(1, game.DialogueActionFunc(func() {
			if dm := player.GetDialogueManager(); dm != nil {
				dm.Reset()
			}
			then()
		})))
	}
	dm.StartDialogues(chain)
}
