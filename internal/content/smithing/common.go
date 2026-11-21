// Smithing: smelting at furnaces and smithing at anvils
// (Smithing.plugin.js + smithing/*.Smithing.js).
//
// The entry router lives in smithing.go; the make-X loops in
// sessions.go; Smelting, Anvil, Cannonballs, Spears and Gear in
// their own files. This file holds the shared helpers
// (Common.Smithing.js): message boxes, spam, hammer checks, the
// Smiths' uniform, level boosts, and the cache-verified id
// constants the frozen Tables API does not expose.
//
// Ids below are NOT invented: each was read from the real
// rev-242 cache with a temporary probe test (since deleted) on
// 2026-11-21, except the ItemIdentifiers constants, which were
// transcribed from ItemIdentifiers.ts (upstream bfd81e3) and
// whose display names were verified against the same cache:
//   - varbit smithing_bar_type 3216, smithing_bar_lasttype
//     13894, smithing_item_lasttype 13895; varp makexcrafting
//     2224; interface "smithing" 312; component
//     toplevel_osrs_stretch:mainmodal (161<<16)|16; seq
//     human_furnace 899, human_smithing 898,
//     human_pickupfloor 827; loc varrock_diary_furnace 16469.
//   - HAMMER 2347, IMCANDO_HAMMER 25644,
//     IMCANDO_HAMMER_OFF_HAND_ 29775, AMMO_MOULD 4,
//     DOUBLE_AMMO_MOULD 27012, RING_OF_FORGING 2568,
//     RING_OF_FORGING_3 16359, GOLDSMITH_GAUNTLETS 776,
//     COAL 453, SMITHING_CATALYST 27017,
//     SMITHS_TUNIC 27023, SMITHS_TROUSERS 27025,
//     SMITHS_BOOTS 27027, SMITHS_GLOVES 27029,
//     SMITHS_GLOVES_I_ 27031, VARROCK_ARMOUR_1..4
//     13104..13107.
//
// Residual gaps (Phase 43 plan §26, recorded at the port
// sites, none silent):
//
//	(a) XP truncates to int at the grant site (gap 26a).
//	(b) No SkillLevelUp emitter; "level-up:popup-opened" is
//	    never emitted in Go, so the session pause never fires.
//	(c) Quest gates fail-closed ("quest:is-complete"
//	    unanswered): blurite smelting, cannonballs, dragon
//	    forge locked; Spears fail-open per TS.
//	(d) "skillcape:has-perk" unanswered (goldsmith XP boost
//	    never applies); coal bag "storage:amount" /
//	    "storage:take" unanswered; "skills:level-boost" /
//	    "skills:actions-per-cycle" / "skills:produced"
//	    unanswered.
//	(e) "smithing:anvil-access" unanswered (Dorics Quest
//	    unported).
//	(f) The frozen Tables API exposes no varbit/varp/
//	    interface ids, no furnace/anvil loc id sets, and no
//	    object-name resolver: the anvil interface ids above
//	    are probe-verified constants, and the item-on-object
//	    router needs SetObjectNameResolver installed by wiring
//	    (the cooking SetObjectInfo precedent) — it is, by
//	    installSmithingTables (cmd/server/smithingwiring.go),
//	    so the router no longer degrades to a no-op in
//	    production. With nil defs (NoCache mode) the resolver
//	    stays uninstalled and the router still degrades.
//	    Blurite anvil products carry no Quest field, so the
//	    Knight's Sword gate never applies on the anvil side
//	    (the smelting side gates the blurite bar itself).
//	(g) Station snapshots hold the location only: Go has no
//	    getTeleportSerial / getArea, so a teleport that lands
//	    back on the same tile does not stop a session.
//
// Godswords, DragonShields and Superheat are Phase 43 item 3:
// the router and smith() call their Try* entry points, and
// register() wires all three registers in (item 3 landed
// during this item and left its registers uncalled until then).
package smithing

import (
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
)

// ---- cache-verified ids (see the file header) ----

const (
	// smithingInterfaceID is interface 312 ("smithing").
	smithingInterfaceID = 312
	// smithingBarTypeVarbit is varbit smithing_bar_type.
	smithingBarTypeVarbit = 3216
	// smithingBarLastTypeVarbit is varbit smithing_bar_lasttype.
	smithingBarLastTypeVarbit = 13894
	// smithingItemLastTypeVarbit is varbit
	// smithing_item_lasttype.
	smithingItemLastTypeVarbit = 13895
	// makexcraftingVarp is varp makexcrafting.
	makexcraftingVarp = 2224
	// mainModalTargetUID is the main modal container
	// (toplevel_osrs_stretch:mainmodal).
	mainModalTargetUID = (161 << 16) | 16

	// humanFurnaceSeq is seq human_furnace (899).
	humanFurnaceSeq = 899
	// humanSmithingSeq is seq human_smithing (898).
	humanSmithingSeq = 898
	// humanPickupFloorSeq is seq human_pickupfloor (827).
	humanPickupFloorSeq = 827
	// smeltingSoundID is Sound.SMELTING (2725).
	smeltingSoundID = 2725
	// edgevilleFurnaceLocID is loc varrock_diary_furnace.
	edgevilleFurnaceLocID = 16469
)

// ---- ItemIdentifiers constants (see the file header) ----

const (
	hammerItemID           = 2347 // HAMMER
	imcandoHammerItemID    = 25644
	imcandoHammerOffHandID = 29775
	ammoMouldItemID        = 4
	doubleAmmoMouldItemID  = 27012
	ringOfForgingItemID    = 2568
	ringOfForging3ItemID   = 16359
	goldsmithGauntletsID   = 776
	coalItemID             = 453
	smithingCatalystItemID = 27017
	goldBarItemID          = 2357
	perfectGoldBarItemID   = 2365
	smithsTunicItemID      = 27023
	smithsTrousersItemID   = 27025
	smithsBootsItemID      = 27027
	smithsGlovesItemID     = 27029
	smithsGlovesImbuedID   = 27031
	varrockArmour1ItemID   = 13104
	varrockArmour2ItemID   = 13105
	varrockArmour3ItemID   = 13106
	varrockArmour4ItemID   = 13107
)

// ---- custom-event payloads (the mining/fletching shapes) ----

// questStateRequest is the "quest:is-complete" query
// (Core.Smithing.js questComplete): Complete defaults false,
// so an unanswered query FAILS CLOSED (quests are unported).
type questStateRequest struct {
	Player   *game.Player
	Key      string
	Complete bool
}

// levelBoostRequest is the "skills:level-boost" query
// (Common.Smithing.js smithingLevel): Boost defaults 0.
type levelBoostRequest struct {
	Player *game.Player
	Skill  int
	Boost  int
}

// actionsPerCycleRequest is the "skills:actions-per-cycle"
// query (Common.Smithing.js actionsPerCycle): Actions
// defaults 1.
type actionsPerCycleRequest struct {
	Player  *game.Player
	Skill   int
	Kind    string
	Actions int
}

// storageAmountRequest is the "storage:amount" query
// (Smelting.Smithing.js storedAmount): Amount starts 0 and a
// storage answerer adds what its container holds. Unanswered
// it stays 0 (gap 26d: the coal bag never contributes).
type storageAmountRequest struct {
	Player *game.Player
	ItemID int
	Amount int
}

// storageTakeRequest is the "storage:take" request
// (Smelting.Smithing.js consumeIngredients): Taken answers
// how many of Amount storage handed over. Unanswered it stays
// 0 (gap 26d).
type storageTakeRequest struct {
	Player *game.Player
	ItemID int
	Amount int
	Taken  int
}

// producedRequest is the "skills:produced" event
// (Common.Smithing.js produced).
type producedRequest struct {
	Player *game.Player
	Skill  int
	ItemID int
	Amount int
	XP     float64
	Kind   string
}

// smithingSuccessEvent is the "smithing:success" event the
// make-X loops emit (Anvil/Cannonballs/Spears): the player,
// skill, made item and amount, plus the anvil provenance
// (station object id and location, when there is one).
type smithingSuccessEvent struct {
	Player          *game.Player
	Skill           int
	ItemID          int
	Amount          int
	BarID           int
	Bars            int
	StationObjectID int
	Location        game.Location
	Source          string
}

// anvilAccessRequest is the "smithing:anvil-access" query
// (Anvil.Smithing.js anvilAccess): Denied defaults false.
// Dorics Quest listens; unported in Go, so it is never
// denied (gap 26e).
type anvilAccessRequest struct {
	Player *game.Player
	Object plugins.GameObject
	Denied bool
}

// barForOreRequest is the "smithing:bar-for-ore" query
// (Smelting.Smithing.js answerBarForOre): the bar an ore
// smelts into and its XP. OutputID 0 means any bar.
type barForOreRequest struct {
	OreID    int
	OutputID int
	BarID    int
	XP       float64
}

// smeltingSuccessEvent is the "smelting:success" event
// (Smelting.Smithing.js finishSmelt).
type smeltingSuccessEvent struct {
	Player    *game.Player
	Skill     int
	ItemID    int
	FurnaceID int
}

// ---- messaging ----

// spam sends a spam-channel line (Common.Smithing.js spam).
func spam(player *game.Player, text string) {
	if player == nil {
		return
	}
	player.SendTypedMessage(protocol.ChatTypeSpam, text, "")
}

// messageBox shows a failure as a statement dialogue
// (Common.Smithing.js messageBox; the mining precedent).
func messageBox(player *game.Player, text string) {
	if player == nil {
		return
	}
	if dm := player.GetDialogueManager(); dm != nil {
		dm.StartDialogues(game.NewDialogueChainBuilder().Add(game.NewStatementDialogue(0, text)))
	}
}

// ---- levels / XP ----

// smithingLevel is the Smithing level the skill checks: the
// current level, raised by content ("skills:level-boost")
// (Common.Smithing.js smithingLevel). Unanswered the boost
// stays 0 (gap 26d).
func (s *smithing) smithingLevel(player *game.Player) int {
	if player == nil {
		return 0
	}
	req := &levelBoostRequest{Player: player, Skill: game.SkillSmithing}
	s.emitEvent("skills:level-boost", req)
	level := player.Skills().CurrentLevel(game.SkillSmithing)
	if req.Boost < 0 {
		req.Boost = 0
	}
	return level + req.Boost
}

// skillIDForRecipe is the skill a smelt recipe grants XP to
// and checks its level against (Smelting.Smithing.js
// levelOf): the recipe's skill, or Smithing when unset.
func skillIDForRecipe(recipe *SmeltRecipe) int {
	if recipe == nil {
		return game.SkillSmithing
	}
	switch recipe.Skill {
	case "CRAFTING":
		return game.SkillCrafting
	default:
		return game.SkillSmithing
	}
}

// levelForRecipe is the level a smelt recipe checks
// (Smelting.Smithing.js levelOf): the boosted Smithing level
// for Smithing recipes, the plain current level otherwise.
func (s *smithing) levelForRecipe(player *game.Player, recipe *SmeltRecipe) int {
	if recipe == nil || recipe.Skill == "" || recipe.Skill == "SMITHING" {
		return s.smithingLevel(player)
	}
	if player == nil {
		return 0
	}
	return player.Skills().CurrentLevel(skillIDForRecipe(recipe))
}

// addXP grants XP, truncating floats at the call site (the
// Mining precedent, mining.go:692; gap 26a).
func addXP(player *game.Player, skill int, xp float64) {
	if player == nil || xp <= 0 {
		return
	}
	player.Skills().AddExperiences(skill, int(xp))
}

// questComplete answers whether the player completed a quest
// (Core.Smithing.js questComplete): an unanswered
// "quest:is-complete" fails closed (gap 26c).
func (s *smithing) questComplete(player *game.Player, key string) bool {
	if key == "" {
		return true
	}
	req := &questStateRequest{Player: player, Key: key}
	s.emitEvent("quest:is-complete", req)
	return req.Complete
}

// questName is the display name of a quest key
// (Data.Smithing.js questName).
func questName(key string) string {
	if tables != nil {
		if name, ok := tables.QuestNames[key]; ok && name != "" {
			return name
		}
	}
	return key
}

// actionsPerCycle is how many items one cycle of a make-X loop
// makes ("skills:actions-per-cycle"); 1 unless content says
// more (Common.Smithing.js actionsPerCycle, gap 26d).
func (s *smithing) actionsPerCycle(player *game.Player, skill int, kind string) int {
	req := &actionsPerCycleRequest{Player: player, Skill: skill, Kind: kind, Actions: 1}
	s.emitEvent("skills:actions-per-cycle", req)
	if req.Actions < 1 {
		return 1
	}
	return req.Actions
}

// produced emits a "skills:produced" event for a made product
// (Common.Smithing.js produced, gap 26d).
func (s *smithing) produced(player *game.Player, skill, itemID, amount int, xp float64, kind string) {
	s.emitEvent("skills:produced", &producedRequest{
		Player: player, Skill: skill, ItemID: itemID, Amount: amount, XP: xp, Kind: kind,
	})
}

// ---- hammers / uniform ----

// hasHammer reports whether the player has a hammer, or an
// Imcando hammer carried or wielded in either hand
// (Common.Smithing.js hasHammer).
func hasHammer(player *game.Player) bool {
	if player == nil {
		return false
	}
	inv := player.Inventory()
	if inv != nil {
		for _, id := range []int{hammerItemID, imcandoHammerItemID, imcandoHammerOffHandID} {
			if inv.Contains(id) {
				return true
			}
		}
	}
	eq := player.Equipment()
	if eq == nil {
		return false
	}
	for _, slot := range []int{game.WeaponSlot, game.ShieldSlot} {
		item := eq.ItemAt(slot)
		if !item.IsValid() {
			continue
		}
		switch item.ID() {
		case hammerItemID, imcandoHammerItemID, imcandoHammerOffHandID:
			return true
		}
	}
	return false
}

// smithsUniformChance is the Smiths' uniform chance that an
// anvil item takes 4 ticks instead of 5: 20% per piece, 100%
// with the full set (tunic, trousers, boots and either
// gloves) (Common.Smithing.js smithsUniformChance).
func smithsUniformChance(player *game.Player) float64 {
	if player == nil || player.Equipment() == nil {
		return 0
	}
	eq := player.Equipment()
	pieces := 0
	for _, id := range []int{smithsTunicItemID, smithsTrousersItemID, smithsBootsItemID} {
		if eq.Contains(id) {
			pieces++
		}
	}
	if eq.Contains(smithsGlovesItemID) || eq.Contains(smithsGlovesImbuedID) {
		pieces++
	}
	if pieces >= 4 {
		return 1
	}
	return float64(pieces) * 0.2
}

// ---- names ----

// titleCase uppercases the first letter of each word
// (Anvil.Smithing.js titleCase, for the level refusal).
func titleCase(name string) string {
	words := strings.Split(name, " ")
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

// metalName is the metal in "You hammer the <metal> …" / "You
// heat the <metal> bar …": the bar's display name without
// " bar", lower-cased. "" when the bar has no name (the
// callers fall back to "metal").
func metalName(player *game.Player, barID int) string {
	if player == nil {
		return ""
	}
	name := player.ItemName(barID)
	if name == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(name, " bar"))
}

// ---- varbits ----

// varbitSender is the session-side varbit send, reached
// through a type assertion on the player's Session (the
// fletching getVarbit/sendVarbit precedent).
type varbitSender interface {
	SendVarbit(id, value int)
}

// sendVarbit sends a varbit for a player with a session; a
// session-less player sends nowhere.
func sendVarbit(player *game.Player, id, value int) {
	if player == nil || player.Session == nil {
		return
	}
	if sender, ok := player.Session.(varbitSender); ok {
		sender.SendVarbit(id, value)
	}
}

// barIDByName resolves a smelt recipe's bar id by its display
// name ("Bronze bar"), or 0 when the tables hold no such
// recipe. The tables carry the ids; nothing here hardcodes a
// bar id.
func barIDByName(name string) int {
	if tables == nil {
		return 0
	}
	for i := range tables.Smelting {
		if tables.Smelting[i].Name == name {
			return tables.Smelting[i].BarID
		}
	}
	return 0
}

// itoa formats an int without importing strconv (the mining
// precedent).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
