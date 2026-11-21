package session

import "tsps-go/internal/protocol"

// SendMultiIcon queues the multi-combat icon packet: the
// session side of game.MultiIconSender.SendMultiIcon,
// porting PacketSender.sendMultiIcon
// (PacketSender.ts:774-776), which sends varbit 4605 through
// sendVarbit. The game layer calls this from
// Player.SetMultiIcon (change-detected there, per
// Player.ts:1186-1191) when AreaManager.Process resolves a
// new multi-combat state; the packet joins the pending queue
// and flushes with the tick like any other queued packet.
// The value is clamped to 0/1 by the encoder, as TS clamps
// it in sendMultiIcon. It routes through SendVarbit, as TS
// does: the frame is byte-identical to EncodeMultiIcon's, and
// the packed value is recorded in the varp store.
func (s *Session) SendMultiIcon(value int) {
	if value != 0 {
		value = 1
	}
	s.SendVarbit(protocol.MulticombatAreaVarbit, value)
}
