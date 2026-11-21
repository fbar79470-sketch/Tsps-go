package game

// Dialogue shop steps: the TranscriptDeps FindShops / OpenShop seams
// for open_shop steps, filled through a minimal ShopOpener interface
// the coordinator binds to the ShopManager at integration. This file
// must not reference any shop-manager implementation: the manager is
// parallel work (phase 17, Worker A), and the game package's dialogue
// playback reaches it only through these seams.
//
// TS grounding (NpcDialogues.plugin.js, commit bfd81e3):
//
//   - shopName (lines 569-576): a shop page split into versions lists
//     them in step.shops; the "npc-dialogue:shop" custom event lets a
//     plugin pick one by game state, else the version whose name ends
//     in " (Default)" opens. Without a shops list the step target
//     opens directly. When no Default version exists, shopName
//     returns undefined — the lookup below then matches nothing.
//     The Go runner ports shopName itself (transcript_steps.go);
//     this file owns only the lookup and the open.
//   - The open_shop branch (lines 1155-1164): the shops named by
//     the resolved target are
//     `ShopDefinition.all().filter(shop => shop.getName() === target)` —
//     an EXACT, case-sensitive name match over every loaded shop
//     definition, including plugin-registered ones. When several
//     shops share the name, only the "core" source ones are kept
//     (a skill master's shop also has its skillcape copies under
//     its name; the shops.json shop is opened, and the skillcape
//     plugin turns it into the copy for a player with 99). When
//     exactly one shop remains, the dialogue closes and
//     `ShopManager.open(player, shops[0].getId(), true)` runs —
//     resetScroll true; a successful open ends the conversation,
//     a failed open falls through to the next step. The >1
//     core-filter, the close, and the fall-through live in the
//     runner (executeShopStep); FindShops returns ALL name
//     matches, unfiltered, and OpenShop reports the open's
//     result.
//
// The lookup reads DEFINITIONS, not runtime shops: ShopDefinition
// .all() is the definition registry, so a shop whose runtime
// stock was removed still resolves by name (and its open then
// fails, falling through). The coordinator's adapter implements
// FindShops as an exact-name filter over the definition Store's
// Shops() and OpenShop as the ShopManager open with resetScroll
// true; see ShopOpener.

// ShopOpener is the minimal shop surface dialogue playback needs:
// find definitions by exact name, and open one by id. The
// coordinator binds it to the ShopManager (plus the definition
// Store for the lookup) at integration. Both methods must be safe
// to call from the tick goroutine (playback runs there) and must
// never panic on unknown names or ids: unknown names yield no
// shops, unknown ids yield a failed open.
type ShopOpener interface {
	// FindShops returns every loaded shop definition whose name
	// equals name exactly (case-sensitive), in load order —
	// the Go form of ShopDefinition.all().filter(getName() ===
	// target). The runner applies the >1 core-source filter
	// itself, so this method must NOT filter: it returns all
	// matches, including non-core copies. An empty name
	// matches nothing (no shop is named ""), which is the Go
	// form of TS shopName returning undefined. A nil result
	// and an empty result are the same "no shops" reading.
	FindShops(name string) []TranscriptShop
	// OpenShop opens shopID for player with resetScroll true
	// (the dialogue call site always passes true,
	// NpcDialogues.plugin.js:1163) and reports whether the
	// shop opened, following the ShopManager.open return
	// contract: true when the interface opened OR when the
	// open was gated (emitCanShop answered false — the click
	// is claimed, no interface opens), false when no shop
	// exists under the id. A nil player yields false.
	OpenShop(player *Player, shopID int) bool
}

// ShopDialogueSeams returns the TranscriptDeps FindShops and
// OpenShop functions bound to opener, for the coordinator to
// install on the DialoguePlayback deps:
//
//	deps.FindShops, deps.OpenShop = game.ShopDialogueSeams(opener)
//
// A nil opener yields no-op functions (no shops found, no shop
// opened): the same outcome as nil TranscriptDeps seams — every
// open_shop step falls through to the next step — without
// requiring the caller to nil-check the opener first. The
// returned functions call opener directly on every invocation;
// they hold no state of their own.
func ShopDialogueSeams(opener ShopOpener) (findShops func(name string) []TranscriptShop, openShop func(player *Player, shopID int) bool) {
	findShops = func(name string) []TranscriptShop {
		if opener == nil {
			return nil
		}
		return opener.FindShops(name)
	}
	openShop = func(player *Player, shopID int) bool {
		if opener == nil {
			return false
		}
		return opener.OpenShop(player, shopID)
	}
	return findShops, openShop
}
