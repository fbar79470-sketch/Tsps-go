// Shared world state for the Fishing sub-units: the
// Wilderness handle.
//
// Why package-level state, and why it is safe: the
// *fishing struct (Worker B's fishing.go) carries no
// Wilderness handle, and the sub-unit methods may not reach
// into f.api — so there is no per-instance handle for it.
// The Woodcutting precedent is the same shape
// (resolveWilderness, lazily from the Api; here the handle
// is installed directly): SetWilderness is called once, in
// buildServer right after newAreaManagerAndWilderness
// (installFishingWilderness, cmd/server/fishingwiring.go) —
// the Wilderness does not exist until then, and the handle
// is only read at catch time on the tick goroutine, so
// installing it before the engine starts is in time — and
// only read afterwards, on the tick goroutine.
//
// The members-world flag does NOT live here: Worker B's
// struct carries it as the membersWorld field, installed
// through B's SetMembersWorld before hub Load, and the
// sub-units read f.membersWorld directly. (An earlier draft
// of this file defined its own SetMembersWorld; it
// collided with B's and was removed — one source of truth.)
//
// A nil Wilderness reads as not in the Wilderness, so the
// imbued-ring clue-bottle doubling never fires without
// one — the same degradation Woodcutting's
// resolveWilderness documents.
package fishing

import (
	"tsps-go/internal/game"
)

// fishingWilderness is the world's Wilderness for the
// imbued-ring clue-bottle check, or nil. The coordinator's
// wiring installs it in buildServer, after the Wilderness
// is constructed and before the engine starts; nil reads as
// not in the Wilderness.
var fishingWilderness *game.Wilderness

// SetWilderness installs the Wilderness handle for the
// Fishing sub-units. The coordinator's wiring calls it in
// buildServer, after the Wilderness is constructed; nil
// clears it.
func SetWilderness(w *game.Wilderness) {
	fishingWilderness = w
}

// wilderness reads the installed Wilderness handle.
func wilderness() *game.Wilderness {
	return fishingWilderness
}
