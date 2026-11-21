package session

import "tsps-go/internal/protocol"

// Hitsplat snapshots: the mapping from landed combat hits to the
// sync views (PlayerSession.ts createActorUpdates / hitView /
// maxHitsplatType / hitsplatType). The session package must not
// import game, so the flush source (coordinator-owned, in
// cmd/server) maps game.TickHit values into HitsplatSnapshot
// and populates the snapshot Hits/Health fields; this file
// owns only the viewer-relative type selection and the wire
// mapping.
//
// A nil Hits slice means no hitsplat block (the HIT mask bit
// stays unset). The source must leave Hits nil when no hits
// landed: an empty non-nil slice would still set the bit.
// Health rides with hits (TS sends health whenever hits exist
// and the bar is not hidden); the source leaves Health nil
// for a hidden bar.

// HitMask values (HitMask.ts): BLUE 0, RED 1, GREEN 2,
// YELLOW 3. Restated here so the snapshot need not import
// game; the adapter copies game.HitDamage.Mask() verbatim.
const (
	hitsplatMaskBlue   = 0
	hitsplatMaskRed    = 1
	hitsplatMaskGreen  = 2
	hitsplatMaskYellow = 3
)

// Hitsplat type ids (RuneLite HitsplatID, via
// PlayerSession.ts hitsplatType / maxHitsplatType).
const (
	hitsplatDamageMeBlue      = 12
	hitsplatDamageOtherBlue   = 13
	hitsplatDamageMe          = 16
	hitsplatDamageOther       = 17
	hitsplatDamageMeYellow    = 22
	hitsplatDamageOtherYellow = 23
	hitsplatDamageMaxMe       = 43
	hitsplatDamageMaxMeYellow = 46
	hitsplatDamageMeGreen     = 65
	hitsplatDamageOtherGreen  = 66
)

// HitsplatSnapshot is one landed hit as the viewer sees it.
// The flush source fills it from a game.TickHit plus the
// viewer relation: OnMe when the actor is the viewer,
// SourceIsViewer when the viewer dealt the hit, IsMaxHit
// from the hit, MaxHitThreshold from the viewer's varbit
// 14195 (0 when unread: any max hit qualifies, matching the
// TS getVarbit default), and the splat overrides (-1 when
// none).
type HitsplatSnapshot struct {
	Mask            int
	Damage          int
	OnMe            bool
	SourceIsViewer  bool
	IsMaxHit        bool
	MaxHitThreshold int
	SplatMine       int
	SplatOthers     int
}

// HealthSnapshot is the hitpoints bar sent with hits.
type HealthSnapshot struct {
	Current int
	Max     int
}

// hitsplatViews maps snapshots to wire hitsplat views
// (PlayerSession.ts hitView): the splat override wins, then
// the max-hit variant, then the mask type. Delay is 0: TS
// hitView carries no delay.
func hitsplatViews(hits []HitsplatSnapshot) []protocol.HitsplatView {
	if len(hits) == 0 {
		return nil
	}
	views := make([]protocol.HitsplatView, 0, len(hits))
	for _, hit := range hits {
		views = append(views, protocol.HitsplatView{
			Type:   hitsplatTypeFor(hit),
			Damage: hit.Damage,
		})
	}
	return views
}

// hitsplatTypeFor is PlayerSession.hitView + maxHitsplatType
// + hitsplatType: own = onMe || source == viewer; the splat
// override (for own) wins; the max-hit variant applies only
// to a max hit the viewer dealt (not on them) meeting the
// threshold, RED/YELLOW only; otherwise the mask type.
func hitsplatTypeFor(hit HitsplatSnapshot) int {
	own := hit.OnMe || hit.SourceIsViewer
	// A splat override of 0 means unset (no valid hitsplat
	// type is 0; TS getSplatType returns null when unset).
	if own && hit.SplatMine > 0 {
		return hit.SplatMine
	}
	if !own && hit.SplatOthers > 0 {
		return hit.SplatOthers
	}
	if maxType, ok := maxHitsplatTypeFor(hit); ok {
		return maxType
	}
	switch hit.Mask {
	case hitsplatMaskBlue:
		if own {
			return hitsplatDamageMeBlue
		}
		return hitsplatDamageOtherBlue
	case hitsplatMaskGreen:
		if own {
			return hitsplatDamageMeGreen
		}
		return hitsplatDamageOtherGreen
	case hitsplatMaskYellow:
		if own {
			return hitsplatDamageMeYellow
		}
		return hitsplatDamageOtherYellow
	default:
		if own {
			return hitsplatDamageMe
		}
		return hitsplatDamageOther
	}
}

// maxHitsplatTypeFor is PlayerSession.maxHitsplatType: the
// max-hit variant of a hit this viewer dealt, when it met
// the "Max hit hitsplats threshold" setting. Only damage
// from a weapon or spell roll counts (IsMaxHit carries the
// rolled-maximum check); the client shows it or the
// ordinary splat by the "Max hit hitsplats" setting
// (varbit 14196, client-side).
func maxHitsplatTypeFor(hit HitsplatSnapshot) (int, bool) {
	if hit.OnMe || !hit.SourceIsViewer || !hit.IsMaxHit {
		return 0, false
	}
	if hit.Damage < hit.MaxHitThreshold {
		return 0, false
	}
	switch hit.Mask {
	case hitsplatMaskRed:
		return hitsplatDamageMaxMe, true
	case hitsplatMaskYellow:
		return hitsplatDamageMaxMeYellow, true
	default:
		return 0, false
	}
}

// healthView maps a health snapshot to the wire health view
// (default bar: nil Bar means id 0, width 30).
func healthView(health *HealthSnapshot) *protocol.HealthView {
	if health == nil {
		return nil
	}
	return &protocol.HealthView{Current: health.Current, Max: health.Max}
}
