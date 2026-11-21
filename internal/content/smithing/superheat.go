// Superheat.Smithing.js: Superheat Item — 43 Magic, 4 fire
// and 1 nature rune, 53 Magic XP, cast every 3 ticks. Iron
// ore with two coal (and 30 Smithing) always makes steel,
// otherwise iron, never failing. A carried smithing catalyst
// works as at a furnace (half the coal, double the Smithing
// XP, not the Magic XP). Jogre bones become burnt jogre bones
// (Cooking 30, 25 Cooking XP).
//
// Captured (rsprox, rev 227, recordings 100, 1749, 1834,
// 1835): the cast plays sequence 725 (human_castsuperheatitem)
// with spotanim 148 (superheatitem_casting), swaps the ores
// for the bar, switches back to the spellbook tab (script 915,
// 6), plays sound 190 and gives Magic and Smithing XP on the
// same tick. Cast on coal (rev 237): "You need to cast
// superheat item on ore.", spotanim 85 (failedspell_impact),
// the same tab switch and sound 191; no XP.
//
// The spell itself is the game spell system's Superheat Item
// utility spell (id 1173, EffectSpells.ts): its CanCast
// checks the level, the spellbook and the runes (with the
// bypass gate and infinite staves) and consumes them. The
// 3-tick pace is enforced here, on the "superheat:next-cast-at"
// attribute (TS Spell.getCastCooldown), because the utility
// spell carries no cooldown attribute of its own.
package smithing

import (
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// ---- ids / constants (Superheat.Smithing.js) ----

const (
	superheatSpellName     = "Superheat Item"
	superheatSpellID       = 1173 // EffectSpells SUPERHEAT_ITEM
	superheatMagicXP       = 53
	superheatCastTicks     = 3
	superheatCastAnimID    = 725 // human_castsuperheatitem
	superheatCastGraphicID = 148 // superheatitem_casting
	superheatFailGraphicID = 85  // failedspell_impact
	superheatCastSoundID   = 190
	superheatFailSoundID   = 191
	// superheatTabScriptID is toplevel_sidebutton_switch
	// (script 915); superheatSpellbookTab is the spellbook
	// tab (6) it switches back to.
	superheatTabScriptID  = 915
	superheatSpellbookTab = 6
	superheatNextCastAttr = "superheat:next-cast-at"
	jogreCookingLevel     = 30
	jogreCookingXP        = 25
)

const (
	ironOreID         = 440  // Iron ore — ItemIdentifiers.ts:439
	steelBarID        = 2353 // Steel bar — ItemIdentifiers.ts:2266
	ironBarID         = 2351 // Iron bar — ItemIdentifiers.ts:2264
	elementalOreID    = 2892 // Elemental ore — ItemIdentifiers.ts:2765
	jogreBonesID      = 3125 // Jogre bones — ItemIdentifiers.ts:2968
	burntJogreBonesID = 3127 // Burnt jogre bones — ItemIdentifiers.ts:2970
)

const (
	superheatNotOreMessage       = "You need to cast superheat item on ore."
	superheatNotHotEnoughMessage = "Even this spell is not hot enough to heat this item."
)

// superheatSpell resolves the Superheat Item utility spell
// from the game spell system. A nil spell (unregistered)
// fails the cast closed.
func superheatSpell() game.SpellCaster {
	return game.GetUtilitySpell(superheatSpellID)
}

// superheatCastTicksFor is the ticks to the next cast
// (Superheat.Smithing.js castTicks): the base 3, lowered by
// content through "skills:action-interval" (a league relic's
// 2-tick pace), never under 1.
func (s *smithing) superheatCastTicksFor(player *game.Player) int {
	return skillactions.ActionInterval(s.emitEvent, player, game.SkillMagic, "superheat", superheatCastTicks, 0)
}

// superheatReady reports whether the 3-tick pace allows a
// cast on this cycle (TS getCastCooldown's attribute check).
// A 0 cycle (no clock) never blocks.
func superheatReady(player *game.Player, cycle int64) bool {
	if player == nil || cycle <= 0 {
		return true
	}
	if next, ok := player.GetAttribute(superheatNextCastAttr).(int64); ok && cycle < next {
		return false
	}
	return true
}

// superheatStamp records the next castable cycle after a
// consumed cast. A 0 cycle stamps nothing.
func (s *smithing) superheatStamp(player *game.Player, cycle int64) {
	if player == nil || cycle <= 0 {
		return
	}
	player.SetAttribute(superheatNextCastAttr, cycle+int64(s.superheatCastTicksFor(player)))
}

// superheatRecipeFor is the furnace recipe a cast on oreID
// smelts, or an empty choice when the item isn't an ore it
// heats (Superheat.Smithing.js recipeFor). Iron ore with two
// coal (and 30 Smithing) makes steel, otherwise iron, never
// failing; coal is in several recipes but is never the ore;
// molten glass is Crafting, not a bar.
func (s *smithing) superheatRecipeFor(player *game.Player, oreID int) smeltChoice {
	if tables == nil {
		return smeltChoice{}
	}
	if oreID == ironOreID {
		steel := tables.SmeltByBar(steelBarID)
		if steel != nil {
			choice := s.resolveCatalyst(player, steel)
			if s.smithingLevel(player) >= steel.Level && s.hasIngredients(player, choice) {
				return choice
			}
		}
		iron := tables.SmeltByBar(ironBarID)
		if iron == nil {
			return smeltChoice{}
		}
		return smeltChoice{recipe: iron, ingredients: iron.Ingredients}
	}
	if oreID == coalItemID {
		return smeltChoice{}
	}
	for i := range tables.Smelting {
		recipe := &tables.Smelting[i]
		if recipe.Skill != "" {
			continue
		}
		for _, ing := range recipe.Ingredients {
			if ing.ItemID == oreID {
				return s.resolveCatalyst(player, recipe)
			}
		}
	}
	return smeltChoice{}
}

// superheatFail plays the failed cast: the message, the fail
// graphic, the tab switch and the fail sound
// (Superheat.Smithing.js fail). No runes are spent and no XP
// is paid — the fail runs before the consuming canCast.
func (s *smithing) superheatFail(player *game.Player, message string) {
	if player == nil {
		return
	}
	player.SendMessage(message)
	player.PerformGraphic(superheatFailGraphicID, superheatGraphicHeight, 0)
	s.superheatTabSwitch(player)
	game.SendSound(player, game.NewSound(superheatFailSoundID, 1, 0, 0))
}

// superheatGraphicHeight is GraphicHeight.HIGH (100): the
// height Superheat.Smithing.js passes for both graphics. (The
// rev-227 capture notes height 92 for the cast graphic; the
// code passes HIGH, and the code wins.)
const superheatGraphicHeight = 100

// superheatCastEffects plays the successful cast: the anim,
// the graphic, the tab switch and the cast sound
// (Superheat.Smithing.js playCast, castOnItem).
func (s *smithing) superheatCastEffects(player *game.Player) {
	if player == nil {
		return
	}
	player.PerformAnimation(superheatCastAnimID, 0)
	player.PerformGraphic(superheatCastGraphicID, superheatGraphicHeight, 0)
	s.superheatTabSwitch(player)
	game.SendSound(player, game.NewSound(superheatCastSoundID, 1, 0, 0))
}

// clientScriptSender is the session-side client-script send,
// reached through a type assertion on the player's Session
// (the cooking varbitSender precedent).
type clientScriptSender interface {
	SendClientScript(scriptID int, args ...game.ClientScriptArg)
}

// superheatTabSwitch switches back to the spellbook tab
// (script 915, 6). A session-less player switches nowhere.
func (s *smithing) superheatTabSwitch(player *game.Player) {
	if player == nil || player.Session == nil {
		return
	}
	if sender, ok := player.Session.(clientScriptSender); ok {
		sender.SendClientScript(superheatTabScriptID, game.ClientScriptInt(superheatSpellbookTab))
	}
}

// superheatJogreBones burns jogre bones into burnt jogre
// bones (Superheat.Smithing.js superheatJogreBones): Cooking
// 30 (the max level, not boostable), 53 Magic and 25 Cooking
// XP, the runes spent by the consuming canCast. The bones'
// own slot is written in place: the runes were just taken,
// so the slot still holds the bones.
func (s *smithing) superheatJogreBones(player *game.Player, slot int, spell game.SpellCaster, cycle int64) {
	skills := player.Skills()
	if skills == nil {
		return
	}
	if skills.MaxLevel(game.SkillCooking) < jogreCookingLevel {
		player.SendMessage("You need a Cooking level of at least " + strconv.Itoa(jogreCookingLevel) + " to burn Jogre bones.")
		return
	}
	if !superheatReady(player, cycle) {
		return
	}
	if spell == nil || !spell.CanCast(player, true) {
		return
	}
	s.superheatStamp(player, cycle)
	player.Inventory().SetItem(slot, game.NewItem(burntJogreBonesID, 1))
	player.Inventory().RefreshItems()
	s.superheatCastEffects(player)
	addXP(player, game.SkillMagic, superheatMagicXP)
	addXP(player, game.SkillCooking, jogreCookingXP)
}

// superheatSmelt casts on an ore (Superheat.Smithing.js
// castOnItem, after the jogre branch): the recipe, the
// Smithing level and ingredient gates, then the consuming
// canCast, the ingredient spend, the bar and the XP.
func (s *smithing) superheatSmelt(player *game.Player, oreID int, spell game.SpellCaster, cycle int64) {
	choice := s.superheatRecipeFor(player, oreID)
	if choice.recipe == nil {
		s.superheatFail(player, superheatNotOreMessage)
		return
	}
	if s.smithingLevel(player) < choice.recipe.Level {
		player.SendMessage("You need a Smithing level of at least " + strconv.Itoa(choice.recipe.Level) + " to smelt this bar.")
		return
	}
	if !s.hasIngredients(player, choice) {
		message := choice.recipe.MissingMessage
		if message == "" {
			message = "You don't have the required ores to smelt this bar."
		}
		player.SendMessage(message)
		return
	}
	if !superheatReady(player, cycle) {
		return
	}
	if spell == nil || !spell.CanCast(player, true) {
		return
	}
	s.superheatStamp(player, cycle)
	s.consumeIngredients(player, choice)
	player.Inventory().Adds(choice.recipe.BarID, 1)
	s.superheatCastEffects(player)
	addXP(player, game.SkillMagic, superheatMagicXP)
	addXP(player, game.SkillSmithing, s.smeltingXP(player, choice))
}

// castSuperheatOnItem is the onSpellOnItem handler
// (Superheat.Smithing.js castOnItem): only Superheat Item
// claims the cast. The slot/id gate and the first, free
// canCast probe run before any fail branch, exactly as TS
// does — a cast the spell cannot pay (level, runes) ends
// silently, its message already sent by canCast.
func (s *smithing) castSuperheatOnItem(event *plugins.SpellOnItemEvent) {
	if event == nil || event.Player == nil {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(event.SpellName), superheatSpellName) {
		return
	}
	event.Handled = true
	player := event.Player
	spell := superheatSpell()
	if spell == nil {
		return
	}
	inv := player.Inventory()
	if inv == nil || event.Slot < 0 || event.Slot >= inv.Capacity() {
		return
	}
	if held := inv.ItemAt(event.Slot); !held.IsValid() || held.ID() != event.ItemID {
		return
	}
	if !spell.CanCast(player, false) {
		return
	}
	if event.ItemID == elementalOreID {
		s.superheatFail(player, superheatNotHotEnoughMessage)
		return
	}
	if event.ItemID == jogreBonesID {
		s.superheatJogreBones(player, event.Slot, spell, event.Cycle)
		return
	}
	s.superheatSmelt(player, event.ItemID, spell, event.Cycle)
}

// ---- registration ----

// registerSuperheat wires the Superheat Item hook
// (Superheat.Smithing.js attach): the onSpellOnItem family.
func (s *smithing) registerSuperheat(api *plugins.Api) {
	api.OnSpellOnItem(s.castSuperheatOnItem)
}
