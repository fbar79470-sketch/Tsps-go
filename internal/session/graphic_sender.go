package session

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Graphic senders: the session side of PacketSender's
// graphic sends — sendGraphic / sendGlobalGraphic
// (PacketSender.ts:1217-1234) via sendClientPacket, and
// sendNpcSpotAnimationSpecific (PacketSender.ts:215-218)
// via the NPC-specific-effect queue.
//
// NAME CORRECTION. The phase 21 plan (§A.6) and the Worker
// A brief call the broadcast sender "sendGraphicToArea";
// no such method exists in TS at bfd81e3 (verified by
// grep). The broadcast is sendGlobalGraphic
// (PacketSender.ts:1226-1234), and the Go broadcast below
// takes that name. There is no SendGraphicToArea to port.
//
// NO GRAPHIC CLASS. TS sendGraphic takes a Graphic
// (game/model/Graphic.ts: id, delay, height, priority)
// and a Location; Go has no Graphic type, so these
// senders take the id/height/delay primitives the
// encoders need. The height values are GraphicHeight
// numbers (LOW 0, MIDDLE 50, HIGH 100, plus the captured
// SURGE_CAST 92 / SURGE_IMPACT 124 / ARCHER_DRAW 96);
// callers pass them as ints. The Core `Graphic` /
// `GraphicHeight` / `Priority` members stay table-only.

// SendGraphic plays a graphic on a world tile for this
// player only, porting PacketSender.sendGraphic
// (PacketSender.ts:1217-1224): encodeTileSpotAnim through
// sendClientPacket. The packet joins the pending queue
// and flushes with the tick.
func (s *Session) SendGraphic(graphicID, height, delay, x, y, level int) {
	s.SendPacket(protocol.EncodeTileSpotAnim(graphicID, x, y, level, height, delay))
}

// graphicViewer is the per-viewer delivery surface
// SendGlobalGraphic asserts: a session that can play a
// tile graphic. *Session satisfies it through SendGraphic;
// the assertion keeps the broadcast total for sessions
// that do not.
type graphicViewer interface {
	SendGraphic(graphicID, height, delay, x, y, level int)
}

// SendGlobalGraphic plays a graphic on a world tile for
// everyone in the source's area who can see it, porting
// PacketSender.sendGlobalGraphic (PacketSender.ts:1226-1234):
// for each network player, skip when the player's area
// differs from the source area or the tile is not
// viewable from the player's location, else sendGraphic.
//
// The Go form differs from TS in two structural ways,
// both forced by the layering:
//
//   - TS reads the source area from this.player (the
//     sender's PacketSender) and iterates
//     World.forEachNetworkPlayer. Go has no global world
//     and a Session holds no player pointer, so the
//     caller supplies the source area (nil = main world)
//     and the viewer list (the world's players in slot
//     order). The area comparison is the contract-2
//     Area()-pointer equivalence (see game.PlayAreaSound),
//     and the view check is Location.IsViewableFrom
//     (15 tiles, same plane).
//   - TS forEachNetworkPlayer applies
//     shouldRunNetworkUpdates (World.ts:921-931); Go
//     applies the same degeneration as
//     game.Projectile.SendProjectile: bots skipped (no
//     watched/unwatched distinction), nil or closed
//     sessions skipped (SessionPort.IsOpen).
//
// It is a package-level function, not a Session method:
// it sends to many sessions, and none of them is "this"
// session. No production caller exists yet; content calls
// it with the world player list when a global graphic is
// needed.
func SendGlobalGraphic(graphicID, height, delay int, loc game.Location, sourceArea *game.Area, viewers []*game.Player) {
	for _, viewer := range viewers {
		if viewer == nil || viewer.IsBot {
			continue
		}
		if viewer.Session == nil || !viewer.Session.IsOpen() {
			continue
		}
		if viewer.Area() != sourceArea {
			continue
		}
		if !loc.IsViewableFrom(viewer.Location()) {
			continue
		}
		if sender, ok := viewer.Session.(graphicViewer); ok {
			sender.SendGraphic(graphicID, height, delay, loc.X, loc.Y, loc.Z)
		}
	}
}

// SendNpcSpotAnimationSpecific plays a spot animation in
// one viewer slot of one NPC for this viewer only,
// porting PacketSender.sendNpcSpotAnimationSpecific
// (PacketSender.ts:215-218): the encoded frame goes to
// the NPC-specific-effect queue (QueueNpcEffect), NOT to
// SendPacket directly — it sends after the NPC sync only
// if the NPC is still visible at the same index (see
// effects.go). A nil NPC, or an out-of-range value the
// encoder rejects (id -1 clears the slot; any other
// negative id is an error), queues nothing: TS would
// throw into the caller, and a session sender must not
// break the tick. This is the first production producer
// of the Phase 8 effect queue; the effects.go header's
// "no producer" note is superseded for this one sender
// (the head-icon and animation-specific senders remain
// unported — their encoders are NOT STARTED, and
// sendNpcHeadIconSpecific / sendNpcAnimationSpecific are
// out of this phase's scope).
func (s *Session) SendNpcSpotAnimationSpecific(npc *game.NPC, graphicID, slot, height, delay int) {
	if npc == nil {
		return
	}
	packet, err := protocol.EncodeNpcSpotAnimationSpecific(npc.GetIndex(), graphicID, slot, height, delay)
	if err != nil {
		return
	}
	s.QueueNpcEffect(npc, npc.GetIndex(), packet)
}
