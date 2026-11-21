package game

import "math"

// Examine prices: the port of game/content/ExaminePrice.ts
// (formatExaminePrice, examinePrices, sendExaminePrice,
// examineItem).
//
// VARBIT GATE. sendExaminePrice gates the line on the player's
// examine-price settings, read as varbits 13826 (GE) and 13827
// (alchemy) through the PacketSender's varp store
// (PacketSender.getVarbit, PacketSender.ts:344-348, which reads
// the sender's own varp state through the cache VarBit
// definition). The session's varp store landed in phase 18 and
// the session implements GetVarbit, so SendExaminePrice consults
// the optional VarbitReader extension of the player's session
// (the ChatMessageSender precedent in message.go): with no
// reader attached, no price line is sent.

// ExaminePriceGEVarbit is EXAMINE_PRICE_GE_VARBIT
// (ExaminePrice.ts:7): Settings > Chat "Examine price info -
// Grand Exchange" (row 234), on by default.
const ExaminePriceGEVarbit = 13826

// ExaminePriceAlchVarbit is EXAMINE_PRICE_ALCH_VARBIT
// (ExaminePrice.ts:9): Settings > Chat "Examine price info -
// Alchemy" (row 235), on by default.
const ExaminePriceAlchVarbit = 13827

// VarbitReader is the optional SessionPort extension for
// reading a player's varbit state (PacketSender.getVarbit,
// PacketSender.ts:344-348). No implementation exists in Go yet
// (see the file header); SendExaminePrice drops the price line
// when the session does not implement it.
type VarbitReader interface {
	// GetVarbit returns the current value of a varbit (0 when
	// the varbit was never set — the TS getVarbit default of
	// reading an unset varp).
	GetVarbit(id int) int
}

// ExaminePrices is the item's Grand Exchange guide price and
// alchemy values, per item (ExaminePrices, ExaminePrice.ts:12).
type ExaminePrices struct {
	GE int
	HA int
	LA int
}

// FormatExaminePrice ports formatExaminePrice (ExaminePrice.ts:
// 22-39): the line shown after an item's examine text — "Price
// of Steel scimitar: GE 67  HA 240  LA 160", or for a stack
// "Price of 100 x Feather: GE 300 (3ea)  HA 100 (1ea)". Each
// value is left out while its setting is off or it is 0, and
// the whole line is absent (ok false, the TS null) when nothing
// is left. Values render red with comma grouping; a stack shows
// the total with the per-item value in parentheses. name is the
// (un-noted) item name and amount the stack size; a non-positive
// amount counts as 1, as TS max(1, floor(amount) || 1) does.
func FormatExaminePrice(name string, amount int, prices ExaminePrices, showGE, showAlch bool) (string, bool) {
	count := amount
	if count < 1 {
		count = 1
	}
	var parts []string
	add := func(label string, each int) {
		if !(each > 0) {
			return
		}
		if count > 1 {
			parts = append(parts, label+" "+redNumber(each*count)+" ("+redNumber(each)+"ea)")
		} else {
			parts = append(parts, label+" "+redNumber(each))
		}
	}
	if showGE {
		add("GE", prices.GE)
	}
	if showAlch {
		add("HA", prices.HA)
		add("LA", prices.LA)
	}
	if len(parts) == 0 {
		return "", false
	}
	line := "Price of "
	if count > 1 {
		line += insertCommas(count) + " x "
	}
	line += name + ": "
	for i, part := range parts {
		if i > 0 {
			line += "  "
		}
		line += part
	}
	return line, true
}

// redNumber ports ExaminePrice.ts's red/number helpers: the
// value comma-grouped (toLocaleString "en-US") in red markup.
func redNumber(value int) string {
	return "<col=ff0000>" + insertCommas(value) + "</col>"
}

// ExaminePricesFor ports examinePrices (ExaminePrice.ts:42-55):
// an item's examine prices, or ok false for an item that can't
// be traded (no line at all). A noted item is priced and named
// as the item it notes. The GE value counts only when a quote
// exists; the alchemy values are the item's own when set, else
// 60%/40% of the store value (the values the alchemy spells
// pay: 0 for an item they refuse, such as a bond — a set 0
// stays 0, which is why the source distinguishes set from
// unset). A nil source answers not-ok: with no definitions,
// nothing is tradeable, matching the TS DEFAULT definition.
func ExaminePricesFor(itemID int, src ShopItemDefinitionSource) (name string, prices ExaminePrices, ok bool) {
	if src == nil {
		return "", ExaminePrices{}, false
	}
	unnoted := src.UnNote(itemID)
	if !src.IsTradeable(unnoted) {
		return "", ExaminePrices{}, false
	}
	value := src.ItemValue(unnoted)
	ge := 0
	if src.ItemHasGrandExchangeValue(unnoted) {
		ge = int(math.Floor(float64(src.ItemGrandExchangeValue(unnoted))))
	}
	ha := int(math.Floor(float64(value) * 0.6))
	if src.ItemHasHighAlchValue(unnoted) {
		ha = src.ItemHighAlchValue(unnoted)
	}
	la := int(math.Floor(float64(value) * 0.4))
	if src.ItemHasLowAlchValue(unnoted) {
		la = src.ItemLowAlchValue(unnoted)
	}
	return src.ItemName(unnoted), ExaminePrices{GE: ge, HA: ha, LA: la}, true
}

// SendExaminePrice ports sendExaminePrice (ExaminePrice.ts:
// 55-69): send the price line after an item's examine text, as
// the player's settings ask. Both settings off sends nothing,
// an untradeable item sends nothing, and a line with no values
// sends nothing. With no VarbitReader on the session (see the
// file header) or no definition source, nothing is sent.
func SendExaminePrice(player *Player, itemID, amount int, src ShopItemDefinitionSource) {
	if player == nil || src == nil {
		return
	}
	name, prices, ok := ExaminePricesFor(itemID, src)
	if !ok {
		return
	}
	SendExaminePriceLine(player, name, prices, amount)
}

// SendExaminePriceLine sends a precomputed price line: the
// settings gate and formatting half of sendExaminePrice
// (ExaminePrice.ts:55-69), for callers that resolved the values
// themselves (the listeners' examine path, whose definition
// source lives outside this package). With no VarbitReader on
// the session, or both settings off, or no values to show,
// nothing is sent.
func SendExaminePriceLine(player *Player, name string, prices ExaminePrices, amount int) {
	if player == nil {
		return
	}
	reader, ok := player.Session.(VarbitReader)
	if !ok {
		return
	}
	showGE := reader.GetVarbit(ExaminePriceGEVarbit) == 1
	showAlch := reader.GetVarbit(ExaminePriceAlchVarbit) == 1
	if !showGE && !showAlch {
		return
	}
	if line, ok := FormatExaminePrice(name, amount, prices, showGE, showAlch); ok {
		player.SendMessage(line)
	}
}

// ExamineItem ports examineItem (ExaminePrice.ts:71-75):
// examine an item the player holds, wears, banks or sees — its
// examine text, then the price line. The text is the item's own
// examine (of the item as clicked, notes included), or its name
// when the examine is empty; the price line un-notes (see
// ExaminePricesFor). A nil source sends the empty text and no
// price line, matching the TS DEFAULT definition's empty
// examine and name; the listeners' examineItem keeps its own
// no-source drop (see internal/listeners/examine.go), so this
// helper is for the in-package call sites (the shop widget)
// that always pass the manager's source.
func ExamineItem(player *Player, itemID, amount int, src ShopItemDefinitionSource) {
	if player == nil {
		return
	}
	text := ""
	if src != nil {
		text = src.ItemExamine(itemID)
		if text == "" {
			text = src.ItemName(itemID)
		}
	}
	player.SendMessage(text)
	SendExaminePrice(player, itemID, amount, src)
}
