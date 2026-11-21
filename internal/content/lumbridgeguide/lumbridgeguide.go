// Package lumbridgeguide is the Go port of the LumbridgeGuide
// plugin (server/plugins/npcs/LumbridgeGuide.plugin.js, the
// entry shim, plus
// server/plugins/npcs/lumbridgeguide/Dialogue.LumbridgeGuide.js,
// the unit file the shim requires): the smallest dialogue
// content in the tree — one onNpcDialogueVariant registration
// that picks the Lumbridge Guide's transcript variant depending
// on whether the player has lost their Amulet of the Eye.
//
// The port is faithful: same plugin name and description, same
// attach-only register, same page/variant constants verbatim,
// same fall-through when the guide's page is absent, same
// mutable-request custom-event query, same page-qualified
// answer object. Line numbers in comments cite
// Dialogue.LumbridgeGuide.js unless they name the entry shim.
//
// Event inventory:
//
//   - Emits "lumbridge-guide:amulet-status"
//     (game.LumbridgeGuideAmuletStatusEventName) as a query: NO
//     answerer in Go — the Guardians of the Rift Amulet unit
//     that answers it (status,
//     Amulet.GuardiansOfTheRift.js:50-52) is not ported, so
//     Reclaim stays false and the guide always plays the
//     standard dialogue in production. That is the honest
//     no-hooks default, and the designed behaviour: the JS
//     header comment requires the normal F2P conversation to
//     remain available when the members' recovery content is
//     absent. No GOTR content is invented here. The LOST branch
//     is unit-covered through a fake emit func and live-covered
//     by the phase smoke test's temporary probe answerer.
//   - Listens to onNpcDialogueVariant (an already-firable hub
//     family): produced by the transcript playback through the
//     cmd/server adapter on every Talk-to resolution.
//
// One deliberate deviation, forced by the language change:
//
//   - The JS keeps the captured api in a module-level `let
//     pluginApi` (Dialogue.LumbridgeGuide.js:6, set in attach).
//     Go keeps the emit func in the piece struct, captured at
//     Register — the gracefuloutfit precedent. No global
//     mutable state, same attach-time capture.
package lumbridgeguide

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Page is the transcript page this plugin answers for
// (PAGE, Dialogue.LumbridgeGuide.js:3). The playback's page
// names carry no "Transcript:" prefix (the loader strips it),
// so this matches the event's page entry exactly as in TS.
const Page = "Lumbridge Guide"

// NormalVariant is the standard dialogue for a player who still
// has the Amulet of the Eye or has not unlocked it (NORMAL,
// Dialogue.LumbridgeGuide.js:4).
const NormalVariant = "standard-dialogue-if-the-player-still-has-the-amulet-of-the-eye-or-has-not-unlocked-it"

// LostVariant is the standard dialogue for a player who lost
// the Amulet of the Eye (LOST,
// Dialogue.LumbridgeGuide.js:5).
const LostVariant = "standard-dialogue-if-the-player-lost-the-amulet-of-the-eye"

// guide holds the plugin's emit func: the Go form of the JS
// module-level pluginApi, captured at Register. One instance
// serves the registered Plugin value; tests build their own
// with a fake emit func.
type guide struct {
	emit func(name string, payload any)
}

func newGuide(emit func(name string, payload any)) *guide {
	return &guide{emit: emit}
}

// chooseVariant picks the Lumbridge Guide's transcript variant
// (chooseVariant, Dialogue.LumbridgeGuide.js:8-13): no answer
// when the guide's page is absent from the event, otherwise the
// amulet-status query decides between the lost-amulet dialogue
// and the standard one. A nil context answers nothing (Go
// nil-safety; TS never calls with a null event — the `?.` in
// line 9 covers only a missing pages array).
func (s *guide) chooseVariant(ctx *plugins.NpcDialogueContext) *plugins.NpcDialogueVariantChoice {
	if ctx == nil || !hasPage(ctx.Pages, Page) {
		return nil
	}
	request := &game.LumbridgeGuideAmuletStatusRequest{Player: ctx.Player, Reclaim: false}
	if s.emit != nil {
		s.emit(game.LumbridgeGuideAmuletStatusEventName, request)
	}
	if request.Reclaim {
		return plugins.DialogueVariantOnPage(Page, LostVariant)
	}
	return plugins.DialogueVariantOnPage(Page, NormalVariant)
}

// hasPage reports whether any entry of pages names page
// (event.pages?.some(({ page }) => page === PAGE),
// Dialogue.LumbridgeGuide.js:9).
func hasPage(pages []plugins.NpcDialoguePage, page string) bool {
	for _, entry := range pages {
		if entry.Page == page {
			return true
		}
	}
	return false
}

func (s *guide) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "LumbridgeGuide",
		Description: "Picks the Lumbridge Guide's dialogue depending on whether the player has lost their Amulet of the Eye.",
		Register: func(api *plugins.Api) {
			// The emit func is captured here, as TS captures
			// pluginApi in attach
			// (Dialogue.LumbridgeGuide.js:16).
			s.emit = api.EmitCustomEvent
			api.OnNpcDialogueVariant(s.chooseVariant)
		},
	}
}

// Plugin is the registered LumbridgeGuide plugin value.
var Plugin = newGuide(nil).plugin()

func init() {
	plugins.Register(Plugin)
}
