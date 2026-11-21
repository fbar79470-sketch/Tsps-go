package session

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Sound senders: the session side of PacketSender's sound
// sends (PacketSender.ts:252-306), porting sendSoundEffect,
// sendSound, sendJingle, and sendAreaSound. All four go
// through sendClientPacket in TS — the LIVE queued path —
// and queue real frames here through SendPacket.
//
// THE DROPPED VOLUME. PacketSender.sendSoundEffect accepts
// a volume argument and never sends it: it calls
// encodeSound with only {loops, delay} (PacketSender.ts:
// 258-260), and the encoder has no volume field. The
// volume argument is therefore accepted and ignored by
// SendSoundEffect/SendSound below, exactly as TS accepts
// and drops it. Callers pass Sound.ClientVolume() (the
// 0..10 clamp) out of fidelity to Sounds.sendSound, which
// reads it before calling; the value never reaches the
// wire. Do not "fix" this by stuffing the volume into the
// radius/attenuation fields: TS leaves those at their
// defaults (0) for unpositioned sounds.

// Session must satisfy the game's sound seam structurally,
// as it satisfies game.SessionPort and game.RelationsSender
// (see sessionport_test.go and social_sender.go): if either
// side drifts, the build fails here rather than in the
// coordinator.
var _ game.SoundSender = (*Session)(nil)

// SendSoundEffect queues an unpositioned sound effect: the
// session side of SoundSender.SendSoundEffect, porting
// PacketSender.sendSoundEffect (PacketSender.ts:252-261),
// which sends encodeSound(soundId, {loops:
// Math.max(1, loopType), delay}) through sendClientPacket.
// The volume is accepted and dropped (see the file header).
// The packet joins the pending queue and flushes with the
// tick, like any other queued packet.
func (s *Session) SendSoundEffect(soundID, loopType, delay, volume int) {
	// Volume deliberately unused: TS drops it (see header).
	_ = volume
	s.SendPacket(protocol.EncodeSound(soundID, protocol.SoundOptions{
		Loops: loopType,
		Delay: delay,
	}))
}

// SendSound queues a sound effect with loops forced to 1:
// the session side of PacketSender.sendSound
// (PacketSender.ts:264-267), which delegates to
// sendSoundEffect(soundId, 1, delay, volume). The stale
// comment on the TS method (claiming opcodes 174/175) is
// wrong; the encoder's opcode 131 governs.
func (s *Session) SendSound(soundID, volume, delay int) {
	s.SendSoundEffect(soundID, 1, delay, volume)
}

// SendJingle queues a jingle: the session side of
// PacketSender.sendJingle (PacketSender.ts:285-291). Live
// sends the current track with zero fades on the same tick
// first (every one of the 1,584 jingles in the rsprox
// captures), and the client resumes that track when the
// jingle ends; the current track comes from
// CurrentMusicTrack (getCurrentMusicTrack,
// PlayerSession.ts:147-149), and no track-first frame is
// sent when no track is playing (track -1). The delay is
// the old 24-bit length field, which the client no longer
// reads; the encoder still writes it in shuffled IME order.
func (s *Session) SendJingle(id, delay int) {
	if track := s.CurrentMusicTrack(); track >= 0 {
		s.SendPacket(protocol.EncodePlaySong(track, protocol.SongNoFades))
	}
	s.SendPacket(protocol.EncodePlayJingle(id, delay))
}

// SendAreaSound queues a positioned sound: the session side
// of SoundSender.SendAreaSound, porting
// PacketSender.sendAreaSound (PacketSender.ts:293-306),
// which sends encodeSound(soundId, {x, y, level, loops,
// delay, radius, attenuation}) through sendClientPacket.
// TS defaults (loops 1, delay 0, radius 0, attenuation 0)
// are the encoder's clamps, so callers pass explicit
// values; a loops value below 1 encodes as 1, and
// radius/attenuation clamp to 0..31.
func (s *Session) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
	s.SendPacket(protocol.EncodeSound(soundID, protocol.SoundOptions{
		Positioned:  true,
		X:           x,
		Y:           y,
		Level:       level,
		Loops:       loops,
		Delay:       delay,
		Radius:      radius,
		Attenuation: attenuation,
	}))
}
