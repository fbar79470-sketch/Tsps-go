package game

// LumbridgeGuideAmuletStatusEventName is the custom-event name
// for the Lumbridge Guide's amulet-status query
// ("lumbridge-guide:amulet-status",
// Dialogue.LumbridgeGuide.js:11, emitted by chooseVariant). The
// guide's variant hook emits the query with a mutable request
// and reads Reclaim after the emit returns; the owning reward
// plugin answers whether recovery applies.
//
// There are NO handlers in Go: the sole upstream listener is the
// Guardians of the Rift Amulet unit (status,
// Amulet.GuardiansOfTheRift.js:50-52, registered at :114), which
// is not ported, so Reclaim stays false — the honest no-hooks
// default, and the designed behaviour: the JS header comment
// (Dialogue.LumbridgeGuide.js:1-2) requires the normal F2P
// conversation to remain available when the members' recovery
// content is absent. Do not invent GOTR content to answer it.
//
// What the unported answerer grounds (for the future port):
// canReclaim (Amulet.GuardiansOfTheRift.js:47-49) is
// Records.setting(player, "gotr_amulet_of_the_eye_obtained") ===
// 1 AND the Temple of the Eye quest complete (via the
// "quest:is-complete" query) AND the player owning none of the
// four amulet variants (inventory, equipment, banks).
// Records.setting (Records.GuardiansOfTheRift.js:133-134) reads
// the "guardians-of-the-rift:settings" player attribute; the
// last-seen amulet colour is kept in the
// "guardians-of-the-rift:amulet-colour" attribute
// (Amulet.GuardiansOfTheRift.js:27). The attribute reads belong
// to the answerer, not to the guide's variant hook, which only
// ever emits this query.
const LumbridgeGuideAmuletStatusEventName = "lumbridge-guide:amulet-status"

// LumbridgeGuideAmuletStatusRequest is the payload of the
// "lumbridge-guide:amulet-status" custom event: the player
// talking to the guide, and the answer, which starts false and
// a handler sets true when the player lost the Amulet of the
// Eye and may reclaim it (Dialogue.LumbridgeGuide.js:10: the
// request object { player, reclaim: false }). The querier reads
// Reclaim after the emit returns; with no handlers it stays
// false and the guide plays the standard dialogue.
type LumbridgeGuideAmuletStatusRequest struct {
	Player  *Player
	Reclaim bool
}
