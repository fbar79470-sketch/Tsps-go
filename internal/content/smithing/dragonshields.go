// DragonShields.Smithing.js: shields and armour forged from
// dragon parts — the dragon sq shield repair, the visage
// shields (dragonfire shield / dragonfire ward), the dragon
// forge kiteshield and platebody (Dragon Slayer II), and the
// ancient wyvern shield on Fossil Island's shield machine.
package smithing

import (
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- ids (obj gamevals resolved against the real rev-242
// cache, probe 2026-11-21; ItemIdentifiers constants
// transcribed from ItemIdentifiers.ts, upstream bfd81e3, with
// display names verified against the same cache) ----

const (
	// DragonForgeLocID is loc ds2_ac_forge_anvil (32215).
	DragonForgeLocID = 32215
	// ShieldMachineLocID is loc fossil_lab_shield_maker (30944).
	ShieldMachineLocID = 30944
)

const (
	dragonShieldLeftHalfID  = 2366 // Shield left half — obj dragonshield_a
	dragonShieldRightHalfID = 2368 // Shield right half — obj dragonshield_b
	dragonSqShieldID        = 1187 // Dragon sq shield — obj dragon_sq_shield

	draconicVisageID       = 11286 // Draconic visage — obj dragonfire_visage
	dragonfireShieldUnchID = 11284 // Dragonfire shield — obj dragonfire_shield_uncharged
	skeletalVisageID       = 22006 // Skeletal visage — obj skeletal_visage
	dragonfireWardUnchID   = 22003 // Dragonfire ward — obj dragonfire_ward_uncharged
	antiDragonShieldID     = 1540  // Anti-dragon shield — ItemIdentifiers.ts:1474
	dragonMetalSliceID     = 22100 // Dragon metal slice — ItemIdentifiers.ts:20382
	dragonMetalShardID     = 22097 // Dragon metal shard — ItemIdentifiers.ts:20379
	dragonMetalLumpID      = 22103 // Dragon metal lump — ItemIdentifiers.ts:20385
	dragonChainbodyID      = 3140  // Dragon chainbody — obj dragon_chainbody
	dragonKiteshieldID     = 21895 // Dragon kiteshield — obj dragon_kiteshield
	dragonPlatebodyID      = 21892 // Dragon platebody — obj dragon_platebody
	wyvernVisageID         = 21637 // Wyvern visage — obj wyvern_visage
	elementalShieldID      = 2890  // Elemental shield — ItemIdentifiers.ts:2763
	wyvernShieldUnchID     = 21634 // Ancient wyvern shield — obj wyvern_shield_uncharged
)

const (
	// dragonSqShieldLevel/XP: 60 Smithing, 75 XP.
	dragonSqShieldLevel = 60
	dragonSqShieldXP    = 75
	// wyvernShieldLevel: 66 Smithing and Magic, 2,000 XP each.
	wyvernShieldLevel = 66
	wyvernShieldXP    = 2000
)

const (
	dragonSqShieldStartMessage     = "You set to work trying to fix the ancient shield. It's seen some heavy action and needs some serious work doing to it."
	dragonSqShieldDoneMessage      = "Even for an experienced armourer it is not an easy task, but eventually it is ready. You have restored the dragon square shield to its former glory."
	dragonSqShieldNoHammerMessage  = "You'll need a hammer if you want to attempt to repair this shield."
	dragonSqShieldOtherHalfMessage = "You need to find the other half of this shield in order to repair it."
	dragonSqShieldByHandMessage    = "You'll need to repair the shield at an anvil, with a hammer."
	presetItemMessage              = "Preset items cannot be used for this."
	dragonForgeQuestMessage        = "You need to complete Dragon Slayer II to use the dragon forge."
	dragonForgeNoHammerMessage     = "You need a hammer to work the dragon metal."
	wyvernShieldNeedPartsMessage   = "You need a wyvern visage and an elemental shield to use this machine."
	wyvernShieldNoHammerMessage    = "You need a hammer to use this machine."
	wyvernShieldLevelMessage       = "You need level 66 Smithing and Magic to make an ancient wyvern shield."
	wyvernShieldDoneMessage        = "At a great cost to your personal magical energies, you have crafted the wyvern visage and elemental shield into an ancient wyvern shield."
)

// visageShield is one visage shield
// (DragonShields.Smithing.js visageShields).
type visageShield struct {
	visage     int
	result     int
	visageName string
	resultName string
	level      int
	xp         int
	start      string
	done       string
	noHammer   string
	lowLevel   string
}

// visageShields is the dragonfire shield and the dragonfire
// ward. The ward's messages follow the dragonfire shield's
// (not captured).
func visageShields() []visageShield {
	shield := func(visage, result int, visageName, resultName string) visageShield {
		return visageShield{
			visage:     visage,
			result:     result,
			visageName: visageName,
			resultName: resultName,
			level:      90,
			xp:         2000,
			start:      "You set to work, trying to attach the ancient " + visageName + " to your anti-dragonbreath shield. It's not easy to work with the ancient artefact and it takes all of your skill as a master smith.",
			done:       "Even for an expert armourer it is not an easy task, but eventually it is ready. You have crafted the " + visageName + " and anti-dragonbreath shield into a " + resultName + ".",
			noHammer:   "You'll need a hammer if you wish to construct a " + resultName + ".",
			lowLevel:   "You need a Smithing level of at least 90 to construct a " + resultName + ".",
		}
	}
	return []visageShield{
		shield(draconicVisageID, dragonfireShieldUnchID, "draconic visage", "dragonfire shield"),
		shield(skeletalVisageID, dragonfireWardUnchID, "skeletal visage", "dragonfire ward"),
	}
}

// dragonForgeRecipe is one dragon-forge recipe
// (DragonShields.Smithing.js forgeRecipes).
type dragonForgeRecipe struct {
	result  int
	parts   []int
	level   int
	xp      int
	ticks   int
	missing string
}

// dragonForgeRecipes is the kiteshield and the platebody.
func dragonForgeRecipes() []dragonForgeRecipe {
	return []dragonForgeRecipe{
		{result: dragonKiteshieldID, parts: []int{dragonSqShieldID, dragonMetalSliceID, dragonMetalShardID}, level: 75, xp: 1000, ticks: 2,
			missing: "You need a dragon metal shard, a dragon metal slice and a dragon sq shield."},
		{result: dragonPlatebodyID, parts: []int{dragonChainbodyID, dragonMetalLumpID, dragonMetalShardID}, level: 90, xp: 2000, ticks: 2,
			missing: "You need a dragon metal shard, a dragon metal lump and a dragon chainbody."},
	}
}

// plainSlot is the slot of a plain copy of id: a preset
// (untradeable, unbankable) part never goes into a fresh,
// tradeable product (DragonShields.Smithing.js plainSlot).
func plainSlot(player *game.Player, id int) int {
	if player == nil || player.Inventory() == nil {
		return -1
	}
	for i, item := range player.Inventory().Items() {
		if item.IsValid() && item.ID() == id && !restricted(item) {
			return i
		}
	}
	return -1
}

// carries reports whether the player holds a plain copy of
// every id (DragonShields.Smithing.js carries).
func carries(player *game.Player, ids []int) bool {
	for _, id := range ids {
		if plainSlot(player, id) < 0 {
			return false
		}
	}
	return true
}

// presetOnly reports whether every part is held, but some
// only as preset copies (DragonShields.Smithing.js
// presetOnly).
func presetOnly(player *game.Player, ids []int) bool {
	if carries(player, ids) {
		return false
	}
	if player == nil || player.Inventory() == nil {
		return false
	}
	for _, id := range ids {
		if !player.Inventory().Contains(id) {
			return false
		}
	}
	return true
}

// anvilSource is the anvil provenance a forge records on its
// smithing:success emit (DragonShields.Smithing.js
// anvilSource): the object id and the location.
type anvilSource struct {
	objectID int
	location game.Location
}

// giveDragonShield swaps the used parts for the result and
// grants the XP (DragonShields.Smithing.js give). False when
// a plain copy left the inventory mid-forge.
func (s *smithing) giveDragonShield(player *game.Player, used []int, result, xp int, source *anvilSource) bool {
	if player == nil || player.Inventory() == nil || !carries(player, used) {
		return false
	}
	inv := player.Inventory()
	for _, id := range used {
		inv.DeleteAtSlot(plainSlot(player, id), 1, true)
	}
	inv.Adds(result, 1)
	addXP(player, game.SkillSmithing, float64(xp))
	ev := &smithingSuccessEvent{Player: player, Skill: game.SkillSmithing, ItemID: result, Amount: 1}
	if source != nil {
		ev.StationObjectID = source.objectID
		ev.Location = source.location
		ev.Source = "anvil"
	}
	s.emitEvent("smithing:success", ev)
	return true
}

// ---- dragon sq shield ----

// repairSqShield repairs the two halves at an anvil
// (DragonShields.Smithing.js repairSqShield): the start box,
// then the timeline (anim at 0/3, the done box at 6, and the
// shield when that box is continued).
func (s *smithing) repairSqShield(player *game.Player, source *anvilSource) {
	halves := []int{dragonShieldLeftHalfID, dragonShieldRightHalfID}
	if !carries(player, halves) {
		messageBox(player, dragonSqShieldOtherHalfMessage)
		return
	}
	if !hasHammer(player) {
		messageBox(player, dragonSqShieldNoHammerMessage)
		return
	}
	if s.smithingLevel(player) < dragonSqShieldLevel {
		messageBox(player, "You need a Smithing level of 60 to repair this shield.")
		return
	}
	animate := func(p *game.Player) bool {
		p.PerformAnimation(humanSmithingSeq, 0)
		return true
	}
	messageBoxThen(player, dragonSqShieldStartMessage, func() {
		s.timeline(player, "dragon-sq-shield", []timelineStep{
			{at: 0, run: animate},
			{at: 3, run: animate},
			{at: 6, run: func(p *game.Player) bool {
				messageBoxThen(p, dragonSqShieldDoneMessage, func() {
					s.giveDragonShield(p, halves, dragonSqShieldID, dragonSqShieldXP, source)
				})
				return true
			}},
		})
	})
}

// ---- visage shields ----

// visageShieldFor is the visage shield the player can make:
// a visage and an anti-dragon shield carried
// (DragonShields.Smithing.js visageShieldFor).
func visageShieldFor(player *game.Player) *visageShield {
	for _, shield := range visageShields() {
		if carries(player, []int{shield.visage, antiDragonShieldID}) {
			shield := shield
			return &shield
		}
	}
	return nil
}

// attachVisage attaches a visage to an anti-dragon shield
// (DragonShields.Smithing.js attachVisage): the hammer and
// level gates, then the item box and the timeline (anim at
// 0/4, the shield and done box at 8).
func (s *smithing) attachVisage(player *game.Player, shield *visageShield) {
	if shield == nil {
		return
	}
	if !hasHammer(player) {
		messageBox(player, shield.noHammer)
		return
	}
	if s.smithingLevel(player) < shield.level {
		messageBox(player, shield.lowLevel)
		return
	}
	used := []int{shield.visage, antiDragonShieldID}
	animate := func(p *game.Player) bool {
		p.PerformAnimation(humanSmithingSeq, 0)
		return true
	}
	itemBoxThen(player, shield.visage, shield.start, func() {
		s.timeline(player, "visage-shield", []timelineStep{
			{at: 0, run: animate},
			{at: 4, run: animate},
			{at: 8, run: func(p *game.Player) bool {
				if !s.giveDragonShield(p, used, shield.result, shield.xp, nil) {
					return false
				}
				itemBox(p, shield.result, shield.done)
				return true
			}},
		})
	})
}

// ---- dragon forge ----

// forgeAtDragonForge makes a kiteshield or platebody at the
// dragon forge (DragonShields.Smithing.js forgeAtDragonForge).
// The Dragon Slayer II gate fails closed: "quest:is-complete"
// is unanswered in Go (gap 26c), so the forge is locked until
// a quest system lands.
func (s *smithing) forgeAtDragonForge(event *plugins.ItemOnObjectEvent) bool {
	player := event.Player
	var candidates []dragonForgeRecipe
	for _, recipe := range dragonForgeRecipes() {
		for _, part := range recipe.parts {
			if part == event.ItemID {
				candidates = append(candidates, recipe)
				break
			}
		}
	}
	heldParts := func(recipe dragonForgeRecipe) int {
		n := 0
		for _, part := range recipe.parts {
			if player.Inventory().Contains(part) {
				n++
			}
		}
		return n
	}
	var recipe *dragonForgeRecipe
	for i := range candidates {
		if carries(player, candidates[i].parts) {
			recipe = &candidates[i]
			break
		}
	}
	if recipe == nil && len(candidates) > 0 {
		best := 0
		for i := range candidates {
			if heldParts(candidates[i]) > heldParts(candidates[best]) {
				best = i
			}
		}
		recipe = &candidates[best]
	}
	if recipe == nil {
		return false
	}
	event.Handled = true
	if !s.questComplete(player, "dragon_slayer_ii") {
		player.SendMessage("You need to complete " + questName("dragon_slayer_ii") + " to use the dragon forge.")
		return true
	}
	var problem string
	switch {
	case presetOnly(player, recipe.parts):
		problem = presetItemMessage
	case !carries(player, recipe.parts):
		problem = recipe.missing
	case !hasHammer(player):
		problem = dragonForgeNoHammerMessage
	case s.smithingLevel(player) < recipe.level:
		problem = "You need a Smithing level of " + strconv.Itoa(recipe.level) + " to make that."
	}
	if problem != "" {
		messageBox(player, problem)
		return true
	}
	player.PerformAnimation(humanSmithingSeq, 0)
	parts, result, xp, ticks := recipe.parts, recipe.result, recipe.xp, recipe.ticks
	s.timeline(player, "dragon-forge", []timelineStep{
		{at: ticks, run: func(p *game.Player) bool {
			return s.giveDragonShield(p, parts, result, xp, nil)
		}},
	})
	return true
}

// ---- ancient wyvern shield ----

// useOnShieldMachine makes an ancient wyvern shield on the
// Fossil Island shield machine
// (DragonShields.Smithing.js useOnShieldMachine): 66 Smithing
// and Magic, 2,000 XP in each, draining Magic to 0.
func (s *smithing) useOnShieldMachine(event *plugins.ItemOnObjectEvent) bool {
	player := event.Player
	if event.ItemID != wyvernVisageID && event.ItemID != elementalShieldID {
		return false
	}
	event.Handled = true
	parts := []int{wyvernVisageID, elementalShieldID}
	if !carries(player, parts) {
		messageBox(player, wyvernShieldNeedPartsMessage)
		return true
	}
	if !hasHammer(player) {
		messageBox(player, wyvernShieldNoHammerMessage)
		return true
	}
	if s.smithingLevel(player) < wyvernShieldLevel || player.Skills().CurrentLevel(game.SkillMagic) < wyvernShieldLevel {
		messageBox(player, wyvernShieldLevelMessage)
		return true
	}
	player.PerformAnimation(humanSmithingSeq, 0)
	s.timeline(player, "wyvern-shield", []timelineStep{
		{at: 3, run: func(p *game.Player) bool {
			if !s.giveDragonShield(p, parts, wyvernShieldUnchID, wyvernShieldXP, nil) {
				return false
			}
			addXP(p, game.SkillMagic, wyvernShieldXP)
			p.Skills().SetCurrentLevel(game.SkillMagic, 0, true)
			p.SendMessage(wyvernShieldDoneMessage)
			return true
		}},
	})
	return true
}

// ---- entry points (the router / anvil call these) ----

// TryDragonShield is "Smith" on an anvil when the player
// holds a visage and an anti-dragon shield
// (DragonShields.Smithing.js smithAtAnvil): it starts the
// attach and claims the click. False when no visage shield
// is held, and the anvil interface opens instead.
func TryDragonShield(player *game.Player) bool {
	return shared.tryDragonShieldAtAnvil(player)
}

// tryDragonShieldAtAnvil is TryDragonShield on the shared
// instance.
func (s *smithing) tryDragonShieldAtAnvil(player *game.Player) bool {
	shield := visageShieldFor(player)
	if shield == nil {
		return false
	}
	s.attachVisage(player, shield)
	return true
}

// TryDragonShieldOnAnvil is an item used on an anvil
// (DragonShields.Smithing.js useOnAnvil): the dragon forge,
// a shield half, or a visage / anti-dragon shield. It claims
// the use when it handles one; false when the item is none of
// those, and the router tries Anvil.
func TryDragonShieldOnAnvil(event *plugins.ItemOnObjectEvent) bool {
	return shared.tryDragonShieldOnAnvil(event)
}

// tryDragonShieldOnAnvil is TryDragonShieldOnAnvil on the
// shared instance.
func (s *smithing) tryDragonShieldOnAnvil(event *plugins.ItemOnObjectEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	source := &anvilSource{objectID: event.ObjectID, location: event.Location}
	if event.ObjectID == DragonForgeLocID && s.forgeAtDragonForge(event) {
		return true
	}
	if event.ItemID == dragonShieldLeftHalfID || event.ItemID == dragonShieldRightHalfID {
		event.Handled = true
		s.repairSqShield(player, source)
		return true
	}
	var shield *visageShield
	for _, candidate := range visageShields() {
		if candidate.visage == event.ItemID {
			candidate := candidate
			shield = &candidate
			break
		}
	}
	if shield == nil && event.ItemID == antiDragonShieldID {
		shield = visageShieldFor(player)
	}
	if shield == nil {
		return false
	}
	event.Handled = true
	parts := []int{shield.visage, antiDragonShieldID}
	if presetOnly(player, parts) {
		messageBox(player, presetItemMessage)
		return true
	}
	if !carries(player, parts) {
		messageBox(player, "You need an anti-dragon shield and a "+itemNameLower(player, shield.visage)+" to do that.")
		return true
	}
	s.attachVisage(player, shield)
	return true
}

// TryWyvernShield is an item used on the shield machine
// (DragonShields.Smithing.js useOnShieldMachine, exported for
// the entry router).
func TryWyvernShield(event *plugins.ItemOnObjectEvent) bool {
	return shared.useOnShieldMachine(event)
}

// itemNameLower is an item's display name, lower-cased, for
// the "You need an anti-dragon shield and a …" message.
func itemNameLower(player *game.Player, id int) string {
	if player == nil {
		return ""
	}
	name := player.ItemName(id)
	if name == "" {
		return ""
	}
	return strings.ToLower(name)
}

// ---- item-on-item: halves by hand ----

// halvesByHand is the two shield halves used on each other
// (DragonShields.Smithing.js halvesByHand): they can only be
// repaired at an anvil, with a hammer.
func (s *smithing) halvesByHand(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	halves := []int{dragonShieldLeftHalfID, dragonShieldRightHalfID}
	used, target := event.UsedItemID, event.UsedWithItemID
	isHalf := func(id int) bool { return id == halves[0] || id == halves[1] }
	if isHalf(used) && isHalf(target) {
		event.Handled = true
		event.Player.SendMessage(dragonSqShieldByHandMessage)
	}
}

// ---- registration ----

// registerDragonShields wires the dragon-shield hooks
// (DragonShields.Smithing.js attach): halves-by-hand on
// item-on-item. The rest is reached via the entry router
// (TryDragonShield / TryDragonShieldOnAnvil / TryWyvernShield).
func (s *smithing) registerDragonShields(api *plugins.Api) {
	api.OnItemOnItem(s.halvesByHand, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
}
