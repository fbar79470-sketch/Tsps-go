// Karambwan is the Go port of plugins/skills/fishing/
// Karambwan.Fishing.js (commit bfd81e3): the karambwan
// vessel — a baited vessel holds one raw karambwanji, and
// every attempt uses it up whether a karambwan bites or
// not; the vessel then reloads itself from the inventory.
// An empty vessel loads itself when the player starts
// fishing.
//
// Pure inventory ops, in exact TS order. The vessel and
// bait ids come from the resolved FishingIDs: the empty
// vessel (KARAMBWAN_VESSEL, 3157), the baited vessel
// (KARAMBWAN_VESSEL_3, 3159 — the cache gameval
// "tbwt_karambwan_vessel_loaded_with_karambwanji"), and
// the bait (RAW_KARAMBWANJI, 3150). The baited-vessel slot
// is found the TS way: inventory.getSlotForItemId, the Go
// form of which is Container.GetSlotForItemID
// (internal/game/container.go:694). No hooks are
// registered here: the core loop calls karambwanReady in
// hasToolRequirements, karambwanLoad from the vessel's
// onStart, and karambwanUseBait per attempt.
package fishing

import (
	"tsps-go/internal/game"
)

// karambwanReady reports whether the player can fish with a
// karambwan vessel (ready, Karambwan.Fishing.js): a baited
// vessel, or an empty one and a karambwanji to load. Nil ids
// read as not ready — fail-closed.
func (f *fishing) karambwanReady(player *game.Player) bool {
	if player == nil || f == nil || f.ids == nil {
		return false
	}
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	return inv.Contains(f.ids.KarambwanVesselBaited) ||
		(inv.Contains(f.ids.KarambwanVessel) && inv.Contains(f.ids.RawKarambwanji))
}

// karambwanLoad loads an empty vessel in its own slot with
// one raw karambwanji (load, Karambwan.Fishing.js — the
// vessel's onStart); false when it can't. A baited vessel
// already carried loads nothing and reports true.
func (f *fishing) karambwanLoad(player *game.Player) bool {
	if player == nil || f == nil || f.ids == nil {
		return false
	}
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	if inv.Contains(f.ids.KarambwanVesselBaited) {
		return true
	}
	if !inv.Contains(f.ids.KarambwanVessel) || !inv.Contains(f.ids.RawKarambwanji) {
		return false
	}
	inv.DeleteNumber(f.ids.RawKarambwanji, 1)
	inv.SetItem(inv.GetSlotForItemID(f.ids.KarambwanVessel), game.NewItem(f.ids.KarambwanVesselBaited, 1))
	inv.RefreshItems()
	return true
}

// karambwanUseBait spends one attempt's bait (useBait,
// Karambwan.Fishing.js): the vessel empties, then reloads
// if karambwanji are left. No baited vessel carried is a
// no-op — the loop only calls it while fishing with one.
func (f *fishing) karambwanUseBait(player *game.Player) {
	if player == nil || f == nil || f.ids == nil {
		return
	}
	inv := player.Inventory()
	if inv == nil {
		return
	}
	slot := inv.GetSlotForItemID(f.ids.KarambwanVesselBaited)
	if slot < 0 {
		return
	}
	refill := inv.Contains(f.ids.RawKarambwanji)
	if refill {
		inv.DeleteNumber(f.ids.RawKarambwanji, 1)
	}
	next := f.ids.KarambwanVessel
	if refill {
		next = f.ids.KarambwanVesselBaited
	}
	inv.SetItem(slot, game.NewItem(next, 1))
	inv.RefreshItems()
}
