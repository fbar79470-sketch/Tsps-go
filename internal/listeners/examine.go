// Examine answers: the dispatch-owned examine cases
// (NetworkBuilder.ts) plus examineItem (ExaminePrice.ts). TS has
// no listener file for examines: the examine_npc / examine_object
// cases live in dispatch() itself, and item examines are answered
// from inventoryAction, groundItemAction, and the worn-item
// widget branch. The Go port mirrors that shape: Handle owns the
// opcode cases and the routes call the helpers here.
//
// All three answers need definition text, which arrives through
// ExamineSource (Deps.Examine): the coordinator wires the
// definition Store behind it. With no source installed the routes
// keep their decode-drop reasons — a bare Deps{} answers nothing,
// rather than answering with invented text.

package listeners

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// ExamineSource supplies the definition text the examine answers
// send. The coordinator implements it over the definition Store
// (see cmd/server/chatwiring.go); the methods encode the TS
// fallback rules, so the listeners only send what they return.
//
//   - NpcExamine: NpcDefinition.getExamine
//     (NpcDefinition.ts:152-160) — the definition's own examine,
//     else the Wiki examine, else a generic line. Never "".
//   - ObjectExamine: the examine_object dispatch rule
//     (NetworkBuilder.ts:1291-1306), including the multi-loc
//     variant rule: when the id has no recorded examine text, the
//     text is read from the variant the player sees, and the
//     variant's text is used only when it has recorded text of
//     its own. Never "".
//   - ItemExamine: examineItem's text (ExaminePrice.ts:71-75) —
//     the item's examine, or its name when the examine is empty.
type ExamineSource interface {
	NpcExamine(id int) string
	ObjectExamine(id int) string
	ItemExamine(id int) string
}

// ExaminePriceSource is the optional ExamineSource extension for
// the examine price line (sendExaminePrice, ExaminePrice.ts:
// 55-69). A source that implements it enables the line on every
// examineItem route; a source that does not keeps the text-only
// answer. The coordinator implements it over the definition
// Store (Store.Item) with the examinePrices rule
// (ExaminePrice.ts:42-55), mirrored by game.ExaminePricesFor:
// un-note the id, report not-ok for an untradeable item, name
// the un-noted item, take the GE value only when a quote exists
// (HasGrandExchangeValue), and take the alchemy values when set
// (HasHighAlchValue/HasLowAlchValue), else 60%/40% of the store
// value floored. The settings gate (varbits 13826/13827) is NOT
// the source's job: game.SendExaminePriceLine reads it from the
// player's session (see game.VarbitReader), and no varbit store
// exists in Go yet, so the line stays silent until a session
// implements it.
type ExaminePriceSource interface {
	// ExaminePriceValues returns the un-noted item's name and
	// per-item prices, or ok false for an item with no line
	// (untradeable).
	ExaminePriceValues(itemID int) (name string, prices game.ExaminePrices, ok bool)
}

// examineItem sends an item's examine text plus, when the source
// supplies prices, the price line: examineItem (ExaminePrice.ts:
// 71-75). The text is the definition's examine, or the item name
// when empty (see ExamineSource.ItemExamine). It reports false
// when no source is installed, so the caller can drop with the
// examine reason. The amount argument feeds only the price line
// (formatExaminePrice counts stacks); with no price source it
// is unused, and the ground-item stack-amount lookup
// (examinedGroundStackAmount, NetworkBuilder.ts:1821-1830) stays
// unneeded until a price source is wired — the ground route
// passes 1 today, as the pre-price-line code did.
//
// The price line fires at exactly the TS examineItem call sites
// this package serves: the inventory, ground-item, and worn-item
// routes (NetworkBuilder.ts:1780, 1837, 1018). The bank call
// sites (Bank.ts:910, 948, 1040) and the shop-widget call sites
// (ShopManager.ts:385, 402) live in systems outside this
// package: the bank's stay unwired (bank interface flows are not
// ported), and the shop's go through game.ExamineItem.
func examineItem(p *game.Player, itemID, amount int, src ExamineSource) bool {
	if src == nil {
		return false
	}
	p.SendMessage(src.ItemExamine(itemID))
	sendExaminePrice(p, itemID, amount, src)
	return true
}

// sendExaminePrice sends the price line after an examine's text,
// when the source supplies prices (see ExaminePriceSource). The
// settings gate and formatting live in game.SendExaminePriceLine.
func sendExaminePrice(p *game.Player, itemID, amount int, src ExamineSource) {
	priced, ok := src.(ExaminePriceSource)
	if !ok {
		return
	}
	name, prices, ok := priced.ExaminePriceValues(itemID)
	if !ok {
		return
	}
	game.SendExaminePriceLine(p, name, prices, amount)
}

// examineNpc sends an NPC's examine text as a typed message
// (NetworkBuilder.ts:1284-1289: sendTypedMessage with
// ChatType.NPC_EXAMINE). It reports false when no source is
// installed.
func examineNpc(p *game.Player, id int, src ExamineSource) bool {
	if src == nil {
		return false
	}
	p.SendTypedMessage(protocol.ChatTypeNPCExamine, src.NpcExamine(id), "")
	return true
}

// examineObject sends an object's examine text as a typed message
// (NetworkBuilder.ts:1291-1306: sendTypedMessage with
// ChatType.LOC_EXAMINE). The world:object-examine plugin event
// runs before this in Handle's case; a handled event skips the
// call entirely. It reports false when no source is installed.
func examineObject(p *game.Player, id int, src ExamineSource) bool {
	if src == nil {
		return false
	}
	p.SendTypedMessage(protocol.ChatTypeLocExamine, src.ObjectExamine(id), "")
	return true
}
