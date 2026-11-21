package session

import "tsps-go/internal/protocol"

// SendSkillDelta sends one skill's delta to the client: the session
// side of SkillManager.updateSkill (SkillManager.ts:284-294), which
// calls PacketSender.sendSkill (PacketSender.ts:1480). sendSkill
// sends encodeSkillsDelta with a SINGLE skillView plus the totals
// (getTotalLevel/getCombatLevel) — never a snapshot; the snapshot
// (sendSkillsSnapshot, PacketSender.ts:1499) is bootstrap-only.
//
// The view shape is PacketSender.skillView (PacketSender.ts:1509):
// id is the CLIENT id (skillClientIDs, bootstrap.go — only
// Construction/Hunter differ from the array index), xp the floored
// sanitized XP, baseLevel the computed base level (XP floor + cap),
// virtualLevel equal to baseLevel, boost the current-minus-base
// difference, currentLevel the sanitized current level. The caller
// (the cmd/server adapter, from the SkillManager update hook)
// supplies the array index and the computed values; this method
// only translates the index and encodes.
//
// An out-of-range skill index sends nothing, mirroring sendSkill's
// guard against a missing skill object. The packet joins the
// pending queue through SendPacket and flushes with the tick, like
// any other queued packet.
func (s *Session) SendSkillDelta(skill, currentLevel, maxLevel, experience, totalLevel, combatLevel int) {
	if skill < 0 || skill >= len(skillClientIDs) {
		return
	}
	view := protocol.SkillView{
		ID:           skillClientIDs[skill],
		XP:           experience,
		BaseLevel:    maxLevel,
		VirtualLevel: maxLevel,
		Boost:        currentLevel - maxLevel,
		CurrentLevel: currentLevel,
	}
	s.SendPacket(protocol.EncodeSkillsDelta([]protocol.SkillView{view}, totalLevel, combatLevel))
}

// SendExpDrop is the session side of SkillManager.addExperience's
// sendExpDrop call (SkillManager.ts:153), porting
// PacketSender.sendExpDrop (PacketSender.ts:1759-1767).
//
// DEVIATION FROM THE QUEUE PATTERN, GROUNDED. Unlike
// SendSkillDelta, this method queues NOTHING, deliberately: TS
// builds the PacketBuilder and hands it to session.write, which
// is a no-op at bfd81e3 (PlayerSession.write,
// PlayerSession.ts:228-230), so no exp-drop frame ever reaches a
// client in TS — and the legacy opcode 116 collides with
// WIDGET_SET_ROTATE_SPEED (fixed length 8) in the native table,
// so queuing the 6 legacy bytes would desync the client's frame
// decoder (see protocol/expdrop.go). This mirrors the Phase 19
// legacy relations sends (social_sender.go): the method exists
// so the exp-drop hook has a session target, and the wire effect
// is exactly TS's — none. The live XP display travels on the
// skills delta/snapshot packets SendSkillDelta and the bootstrap
// send.
//
// What TS still does, this still does: the skill guard (TS
// returns when the skill object is missing; Go returns for an
// out-of-range array index, like SendSkillDelta), the client-id
// translation through skillClientIDs (the session package owns
// wire ids, mirroring SendSkillDelta — only Construction/Hunter
// differ), and building the legacy layout, which is then
// discarded exactly as write() discards the PacketBuilder. The
// translation is therefore exercised but unobservable on the
// wire, as in TS; the byte layout itself is pinned by the
// protocol encoder tests. If a native exp-drop encoder ever
// lands in TS, only this method changes: queue the native frame
// here instead of discarding the legacy one.
func (s *Session) SendExpDrop(skill, exp int) {
	if skill < 0 || skill >= len(skillClientIDs) {
		return
	}
	// Deliberate discard: PlayerSession.write is a no-op in TS.
	_ = protocol.EncodeExpDrop(skillClientIDs[skill], exp)
}
