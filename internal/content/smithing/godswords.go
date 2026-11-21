// Godswords.Smithing.js: godsword blades and hilts —
// forging shard pieces into pairs and a blade at an anvil
// (80 Smithing, 100 XP per forge), joining a blade and a hilt
// by hand (no skill), and dismantling a godsword back into its
// blade and hilt.
//
// Captured (rsprox recs 1515, 1517; docs/smithing.md): Smith
// (or a piece used on the anvil) opens "You set to work,
// trying to fix the ancient sword..." with human_smithing,
// again 3 ticks later; 6 ticks in the pieces become the
// result, shown in an item box "Even for an experienced smith
// it is not an easy task, but eventually it is done.", with
// 100 XP.
package smithing

import (
	"sort"
	"strconv"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- ids (ItemIdentifiers.ts, upstream bfd81e3; display names
// verified against the real rev-242 cache, probe 2026-11-21)
// ----

const (
	godswordShard1ID   = 11818 // Godsword shard 1 — ItemIdentifiers.ts:10286
	godswordShard2ID   = 11820 // Godsword shard 2 — ItemIdentifiers.ts:10288
	godswordShard3ID   = 11822 // Godsword shard 3 — ItemIdentifiers.ts:10290
	godswordShards12ID = 11794 // Godsword shards 1 & 2 — ItemIdentifiers.ts:10262
	godswordShards13ID = 11796 // Godsword shards 1 & 3 — ItemIdentifiers.ts:10264
	godswordShards23ID = 11800 // Godsword shards 2 & 3 — ItemIdentifiers.ts:10268
	godswordBladeID    = 11798 // Godsword blade — ItemIdentifiers.ts:10266

	armadylHiltID   = 11810 // Armadyl hilt — ItemIdentifiers.ts:10278
	bandosHiltID    = 11812 // Bandos hilt — ItemIdentifiers.ts:10280
	saradominHiltID = 11814 // Saradomin hilt — ItemIdentifiers.ts:10282
	zamorakHiltID   = 11816 // Zamorak hilt — ItemIdentifiers.ts:10284
	ancientHiltID   = 26370 // Ancient hilt — ItemIdentifiers.ts:24327

	armadylGodswordID   = 11802 // Armadyl godsword — ItemIdentifiers.ts:10270
	bandosGodswordID    = 11804 // Bandos godsword — ItemIdentifiers.ts:10272
	saradominGodswordID = 11806 // Saradomin godsword — ItemIdentifiers.ts:10274
	zamorakGodswordID   = 11808 // Zamorak godsword — ItemIdentifiers.ts:10276
	ancientGodswordID   = 26233 // Ancient godsword — ItemIdentifiers.ts:24190
)

const (
	godswordForgeLevel = 80
	godswordForgeXP    = 100
)

const (
	godswordStartMessage      = "You set to work, trying to fix the ancient sword..."
	godswordDoneMessage       = "Even for an experienced smith it is not an easy task, but eventually it is done."
	godswordNeedTwoMessage    = "You need at least two different Godsword blade pieces in your inventory to reforge them."
	godswordNoHammerMessage   = "You'll need a hammer if you wish to repair the Godsword."
	godswordJoinByHandMessage = "Those pieces of the Godsword can't be joined together like that - try forging them on an anvil."
	godswordRestrictedMessage = "Preset items cannot be used for this."
	godswordDismantlePrompt   = "Dismantle your godsword?"
	godswordDismantleYes      = "Yes, dismantle it."
	godswordDismantleNo       = "No."
	godswordDismantledMessage = "You dismantle the godsword."
	godswordNoSpaceMessage    = "You don't have enough inventory space to do that."
)

// godswordPiece is one shard piece and the shards it holds
// (Godswords.Smithing.js pieces).
type godswordPiece struct {
	id     int
	shards []int
}

// godswordPieces is every piece and the blade, in TS order.
func godswordPieces() []godswordPiece {
	return []godswordPiece{
		{godswordShard1ID, []int{1}},
		{godswordShard2ID, []int{2}},
		{godswordShard3ID, []int{3}},
		{godswordShards12ID, []int{1, 2}},
		{godswordShards13ID, []int{1, 3}},
		{godswordShards23ID, []int{2, 3}},
		{godswordBladeID, []int{1, 2, 3}},
	}
}

// godswordHilts maps a hilt to its godsword
// (Godswords.Smithing.js hilts).
func godswordHilts() map[int]int {
	return map[int]int{
		armadylHiltID:   armadylGodswordID,
		bandosHiltID:    bandosGodswordID,
		saradominHiltID: saradominGodswordID,
		zamorakHiltID:   zamorakGodswordID,
		ancientHiltID:   ancientGodswordID,
	}
}

// godswordHiltOf maps a godsword back to its hilt (the
// Dismantle table).
func godswordHiltOf() map[int]int {
	out := make(map[int]int, len(godswordHilts()))
	for hilt, sword := range godswordHilts() {
		out[sword] = hilt
	}
	return out
}

// isGodswordPiece reports whether id is a forgeable piece
// (a shard or a pair — not the blade).
func isGodswordPiece(id int) bool {
	for _, piece := range godswordPieces() {
		if piece.id == id && len(piece.shards) < 3 {
			return true
		}
	}
	return false
}

// godswordCombination is the first two carried pieces that
// hold different shards, and what they forge into
// (Godswords.Smithing.js combination). Nil when no two
// carried pieces fit.
type godswordCombination struct {
	from [2]int
	into int
}

func godswordCombinationFor(player *game.Player) *godswordCombination {
	if player == nil || player.Inventory() == nil {
		return nil
	}
	inv := player.Inventory()
	var carried []godswordPiece
	for _, piece := range godswordPieces() {
		if len(piece.shards) < 3 && inv.Contains(piece.id) {
			carried = append(carried, piece)
		}
	}
	for i := 0; i < len(carried); i++ {
		for j := i + 1; j < len(carried); j++ {
			shards := append(append([]int(nil), carried[i].shards...), carried[j].shards...)
			sort.Ints(shards)
			seen := map[int]bool{}
			dup := false
			for _, shard := range shards {
				if seen[shard] {
					dup = true
					break
				}
				seen[shard] = true
			}
			if dup {
				continue
			}
			for _, piece := range godswordPieces() {
				if len(piece.shards) != len(shards) {
					continue
				}
				match := true
				for k := range shards {
					if piece.shards[k] != shards[k] {
						match = false
						break
					}
				}
				if match {
					return &godswordCombination{from: [2]int{carried[i].id, carried[j].id}, into: piece.id}
				}
			}
		}
	}
	return nil
}

// holdsGodswordPiece reports whether the player holds any
// forgeable piece (Godswords.Smithing.js holdsPiece).
func holdsGodswordPiece(player *game.Player) bool {
	if player == nil || player.Inventory() == nil {
		return false
	}
	for _, piece := range godswordPieces() {
		if len(piece.shards) < 3 && player.Inventory().Contains(piece.id) {
			return true
		}
	}
	return false
}

// forgeGodsword starts the forge timeline for the player's
// first fitting combination (Godswords.Smithing.js forge):
// the level and hammer gates, then the timeline (anim at
// 0/3, result at 6 ticks).
func (s *smithing) forgeGodsword(player *game.Player) {
	pair := godswordCombinationFor(player)
	if pair == nil {
		messageBox(player, godswordNeedTwoMessage)
		return
	}
	if !hasHammer(player) {
		messageBox(player, godswordNoHammerMessage)
		return
	}
	if s.smithingLevel(player) < godswordForgeLevel {
		messageBox(player, "You need a Smithing level of "+strconv.Itoa(godswordForgeLevel)+" to repair the Godsword.")
		return
	}
	animate := func(p *game.Player) bool {
		p.PerformAnimation(humanSmithingSeq, 0)
		return true
	}
	s.timeline(player, "godsword", []timelineStep{
		{at: 0, run: func(p *game.Player) bool {
			messageBox(p, godswordStartMessage)
			return animate(p)
		}},
		{at: 3, run: animate},
		{at: 6, run: func(p *game.Player) bool { return s.finishGodsword(p, pair) }},
	})
}

// finishGodsword swaps the pair for the result, grants the XP
// and shows the done box (Godswords.Smithing.js finish).
// False when a piece left the inventory mid-forge.
func (s *smithing) finishGodsword(player *game.Player, pair *godswordCombination) bool {
	if player == nil || player.Inventory() == nil || pair == nil {
		return false
	}
	inv := player.Inventory()
	for _, id := range pair.from {
		if !inv.Contains(id) {
			return false
		}
	}
	for _, id := range pair.from {
		inv.DeleteNumber(id, 1)
	}
	inv.Adds(pair.into, 1)
	addXP(player, game.SkillSmithing, godswordForgeXP)
	itemBox(player, pair.into, godswordDoneMessage)
	s.emitEvent("smithing:success", &smithingSuccessEvent{Player: player, Skill: game.SkillSmithing, ItemID: pair.into, Amount: 1})
	return true
}

// ---- entry points (the router / anvil call these) ----

// TryGodsword is "Smith" on an anvil when the player holds
// two pieces that fit (Godswords.Smithing.js smithAtAnvil):
// it starts the forge and claims the click. False when no
// combination is held, and the anvil interface opens instead.
func TryGodsword(player *game.Player) bool {
	return shared.tryGodswordAtAnvil(player)
}

// tryGodswordAtAnvil is TryGodsword on the shared instance.
func (s *smithing) tryGodswordAtAnvil(player *game.Player) bool {
	if godswordCombinationFor(player) == nil {
		return false
	}
	s.forgeGodsword(player)
	return true
}

// TryGodswordOnAnvil is a piece used on an anvil
// (Godswords.Smithing.js useOnAnvil): it starts the forge
// and claims the use. False when the item is not a piece,
// and the router tries DragonShields, then Anvil.
func TryGodswordOnAnvil(event *plugins.ItemOnObjectEvent) bool {
	return shared.tryGodswordOnAnvil(event)
}

// tryGodswordOnAnvil is TryGodswordOnAnvil on the shared
// instance.
func (s *smithing) tryGodswordOnAnvil(event *plugins.ItemOnObjectEvent) bool {
	if event == nil || event.Player == nil || !isGodswordPiece(event.ItemID) {
		return false
	}
	event.Handled = true
	s.forgeGodsword(event.Player)
	return true
}

// ---- item-on-item: joining by hand ----

// joinGodswordByHand is a hilt or a shard used on another
// item (Godswords.Smithing.js joinByHand): a hilt + blade
// makes the godsword at once (no skill); two pieces cannot
// join by hand. It claims the use in both cases.
func (s *smithing) joinGodswordByHand(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	used, target := event.UsedItemID, event.UsedWithItemID
	hilts := godswordHilts()
	hilt := 0
	if _, ok := hilts[used]; ok {
		hilt = used
	} else if _, ok := hilts[target]; ok {
		hilt = target
	}
	other := used
	if hilt == used {
		other = target
	}
	if hilt != 0 {
		if godsword, ok := hilts[hilt]; ok && other == godswordBladeID {
			inv := event.Player.Inventory()
			if inv == nil {
				return
			}
			inv.DeleteNumber(godswordBladeID, 1)
			inv.DeleteNumber(hilt, 1)
			inv.Adds(godsword, 1)
			event.Handled = true
			return
		}
	}
	if isGodswordPiece(used) && isGodswordPiece(target) {
		event.Player.SendMessage(godswordJoinByHandMessage)
		event.Handled = true
	}
}

// ---- Dismantle ----

// dismantleGodsword is the "Dismantle" action on a godsword
// (Godswords.Smithing.js dismantle): a restricted (preset)
// copy refuses at once; otherwise the player is asked, and
// "Yes" splits the sword back into its blade and hilt.
func (s *smithing) dismantleGodsword(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	hilt, ok := godswordHiltOf()[event.ItemID]
	if !ok {
		return false
	}
	player := event.Player
	if item := s.godswordItem(player, event.ItemID); item.IsValid() && restricted(item) {
		player.SendMessage(godswordRestrictedMessage)
		return true
	}
	dm := player.GetDialogueManager()
	if dm == nil {
		return true
	}
	swordID := event.ItemID
	dm.StartDialogues(game.NewDialogueChainBuilder().Add(
		game.NewOptionDialogue(0, dialogueOptionActionFunc(func(option game.DialogueOption) {
			// The option values are the resume path's
			// one-based child indices (the fletching spool
			// precedent): 1 is the first option.
			if dm := player.GetDialogueManager(); dm != nil {
				dm.Advance()
			}
			if option == 1 {
				s.finishDismantle(player, swordID, hilt)
			}
		}), godswordDismantleYes, godswordDismantleNo).SetTitle(godswordDismantlePrompt),
	))
	return true
}

// godswordItem finds the godsword stack the action ran on:
// the inventory first, else the wielded weapon. Invalid when
// neither holds it.
func (s *smithing) godswordItem(player *game.Player, swordID int) game.Item {
	if player == nil {
		return game.Item{}
	}
	if inv := player.Inventory(); inv != nil {
		for _, item := range inv.Items() {
			if item.IsValid() && item.ID() == swordID {
				return item
			}
		}
	}
	if eq := player.Equipment(); eq != nil {
		if weapon := eq.ItemAt(game.WeaponSlot); weapon.IsValid() && weapon.ID() == swordID {
			return weapon
		}
	}
	return game.Item{}
}

// finishDismantle splits a godsword back into its blade and
// hilt (Godswords.Smithing.js finishDismantle): the inventory
// copy becomes the blade in place and the hilt joins the
// inventory; a wielded copy leaves the weapon slot and both
// parts join the inventory. A restricted copy refuses, and
// too few free slots refuse before anything moves.
func (s *smithing) finishDismantle(player *game.Player, swordID, hiltID int) {
	if player == nil || player.Inventory() == nil {
		return
	}
	inv := player.Inventory()
	invSlot := -1
	for i, item := range inv.Items() {
		if item.IsValid() && item.ID() == swordID {
			invSlot = i
			break
		}
	}
	eqSlot := -1
	if eq := player.Equipment(); eq != nil {
		if weapon := eq.ItemAt(game.WeaponSlot); weapon.IsValid() && weapon.ID() == swordID {
			eqSlot = game.WeaponSlot
		}
	}
	if invSlot < 0 && eqSlot < 0 {
		return
	}
	item := game.Item{}
	if invSlot >= 0 {
		item = inv.Items()[invSlot]
	} else {
		item = player.Equipment().ItemAt(eqSlot)
	}
	if !item.IsValid() || item.ID() != swordID {
		return
	}
	if restricted(item) {
		player.SendMessage(godswordRestrictedMessage)
		return
	}
	neededSlots := 1
	if invSlot < 0 {
		neededSlots = 2
	}
	if inv.GetFreeSlots() < neededSlots {
		player.SendMessage(godswordNoSpaceMessage)
		return
	}
	if invSlot >= 0 {
		inv.SetItem(invSlot, game.NewItem(godswordBladeID, 1))
		inv.Adds(hiltID, 1)
	} else {
		player.Equipment().DeleteAtSlot(eqSlot, 1, true)
		inv.Adds(godswordBladeID, 1)
		inv.Adds(hiltID, 1)
		player.Equipment().RefreshItems()
	}
	inv.RefreshItems()
	player.SendMessage(godswordDismantledMessage)
}

// ---- registration ----

// registerGodswords wires the godsword hooks
// (Godswords.Smithing.js attach): join-by-hand on item-on-item
// and Dismantle on all five godswords.
func (s *smithing) registerGodswords(api *plugins.Api) {
	api.OnItemOnItem(s.joinGodswordByHand, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
	for _, name := range []string{"Armadyl godsword", "Bandos godsword", "Saradomin godsword", "Zamorak godsword", "Ancient godsword"} {
		api.OnItemActionNamed(name, map[string]plugins.ItemActionHandler{"Dismantle": s.dismantleGodsword}, nil)
	}
}
