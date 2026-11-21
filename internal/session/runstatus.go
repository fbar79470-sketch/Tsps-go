package session

import (
	"sort"

	"tsps-go/internal/music"
	"tsps-go/internal/protocol"
)

// SendRunEnergy queues the run-energy orb packet: the session side of
// game.SessionPort.SendRunEnergy, porting PacketSender.sendRunEnergy /
// sendRunStatus (PacketSender.ts:374-410), both of which send
// encodeRunEnergy(units, running, weightKg, staminaTicks). The game
// layer calls this when energy drains, restores, auto-stops running,
// or the run setting toggles; the packet joins the pending queue and
// flushes with the tick like any other queued packet.
func (s *Session) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
	s.SendPacket(protocol.EncodeRunEnergy(units, running, weightKg, staminaTicks))
}

// SendDestination queues the destination-flag packet: the session side
// of game.SessionPort.SendDestination, porting
// PacketSender.sendDestination (PacketSender.ts:1534-1537). The
// movement queue calls this through syncDestinationFlagToRoute when
// the queued destination changes.
func (s *Session) SendDestination(x, y int) {
	s.SendPacket(protocol.EncodeDestination(x, y))
}

// SendVarp queues a varp packet: the session side of
// game.SessionPort.SendVarp, porting PacketSender.sendConfig
// (PacketSender.ts:355-359), which records the value in the varps
// map (`state | 0`) and sends encodeVarp. The record lands in the
// per-session varpStore (varps.go), so GetVarp/GetVarbit reflect
// what was sent; a session whose store was never wired (a nil
// store, which NewSession never produces once Worker 1's wiring
// lands) degrades to send-only rather than dropping the packet.
// Plugin content sends varps through this.
func (s *Session) SendVarp(id, value int) {
	if s.varps != nil {
		s.varps.set(id, value)
	}
	s.SendPacket(protocol.EncodeVarp(id, value))
}

// SendToggle queues a varp packet, porting PacketSender.sendToggle
// (PacketSender.ts:361-363), which is exactly sendConfig under
// another name. It records and sends exactly as SendVarp does.
func (s *Session) SendToggle(id, state int) {
	s.SendVarp(id, state)
}

// SendVarpLong queues a 64-bit varp packet, porting
// PacketSender.sendVarpLong (PacketSender.ts:350-353), which sends
// encodeVarpLong and records NOTHING: the varps map holds 32-bit
// values only, and a long varp never lands in it. GetVarp therefore
// never reflects a long varp, as in TS.
func (s *Session) SendVarpLong(id int, value int64) {
	s.SendPacket(protocol.EncodeVarpLong(id, value))
}

// GetVarp returns the last value recorded for a varp, or 0 when it
// was never sent (getVarp, PacketSender.ts:340-342). A session with
// no store reads 0 for every varp, the fresh-store default.
func (s *Session) GetVarp(id int) int {
	if s.varps == nil {
		return 0
	}
	return s.varps.get(id)
}

// GetVarbit returns the current value of a varbit: the field of its
// backing varp (getVarbit, PacketSender.ts:344-348; see
// varpStore.getVarbit for the packing and unknown-definition
// behavior). Implementing this on *Session activates the game's
// VarbitReader extension (game/examine_price.go): the examine
// price line and any other varbit-gated read now see live values.
// A session with no store, or with no varbit resolver installed,
// reads 0 for every varbit.
func (s *Session) GetVarbit(id int) int {
	if s.varps == nil {
		return 0
	}
	return s.varps.getVarbit(id)
}

// SendVarbit queues a varbit packet, porting PacketSender.sendVarbit
// (PacketSender.ts:1523-1531): the value is packed into the backing
// varp's record (read-modify-write, range-clamped; see
// varpStore.applyVarbit) and the encodeVarbit frame goes out with
// the ORIGINAL value. The frame is always sent, even when no store
// or no resolver is installed — TS sends it unconditionally, and
// the record update is what degrades, not the send.
func (s *Session) SendVarbit(id, value int) {
	if s.varps != nil {
		s.varps.applyVarbit(id, value)
	}
	s.SendPacket(protocol.EncodeVarbit(id, value))
}

// SetVarbitResolver installs the varbit-definition source for this
// session's store (see VarbitResolver in varps.go). The coordinator
// calls it once at AddPlayer time, before the player is queued,
// with a closure over the boot-loaded CacheDefinitions:
//
//	defs.GetVarbit(id) + Defined(): an undefined definition resolves
//	to ok=false (see varpStore.getVarbit for why that mirrors TS).
//
// It follows the SetMusicData precedent: setup-time only, before
// the session is shared. A nil store is created here, so a session
// built without Worker 1's NewSession wiring still gains a store
// when a resolver is installed.
func (s *Session) SetVarbitResolver(r VarbitResolver) {
	if s.varps == nil {
		s.varps = &varpStore{values: make(map[int]int)}
	}
	s.varps.setResolver(r)
}

// ReplayVarps re-sends every recorded varp, porting
// PacketSender.replayVarps (PacketSender.ts:336-338), which loops
// the whole varps map and re-sends each entry with encodeVarp. TS
// calls it once, in bindControlledPlayer (NetworkBuilder.ts:538),
// right after sendPlayerBootstrap, when a (possibly returning)
// player is bound to a connection: the recorded setup is replayed
// onto the fresh channel. Go creates a fresh Session — and an
// empty store — on every EnterWorld, so a login-time call would
// send nothing and no call site is wired; the method exists for
// any future path that reuses a session across a rebind, where
// the coordinator should call it after the bootstrap sequence,
// in the TS position. Entries go out in first-sent order.
func (s *Session) ReplayVarps() {
	if s.varps == nil {
		return
	}
	for _, entry := range s.varps.entries() {
		s.SendPacket(protocol.EncodeVarp(entry[0], entry[1]))
	}
}

// ApplyAudioSettings replays the player's audio settings through
// the varp store, porting the audio loop at the end of
// Player.onLogin (Player.ts:801-810): every entry goes out with
// sendConfig, except the music settings varp (19), whose
// client-owned bits are merged onto the server-owned bits already
// recorded for it: sent = (getVarp(19) & ~SETTINGS_MASK) | value.
// The settings values arrive pre-masked from the audio store
// (setAudioSetting keeps only SETTINGS_MASK bits for varp 19,
// Player.ts:908-911), so no further masking is applied here, as
// in TS. Entries send in ascending varp-id order, matching TS
// Object.entries iteration over integer-like keys.
//
// This is the STORE side of the varp_transmit story, and it runs
// at LOGIN, not on transmit: an inbound varp_transmit never
// writes the varps map in TS (NetworkBuilder.ts:1147-1168 writes
// only the Player audio-settings record, via
// applyAudioSettingFromClient, plus the varpId-172 branch's
// sendConfig). The coordinator calls this once at AddPlayer time
// with the player's GetAudioSettings snapshot, after the session
// wiring block; a nil map sends nothing. It is NOT called from
// the listeners' varp_transmit case.
func (s *Session) ApplyAudioSettings(settings map[int]int) {
	if len(settings) == 0 {
		return
	}
	ids := make([]int, 0, len(settings))
	for id := range settings {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		sent := settings[id]
		if id == music.SettingsVarp {
			sent = (s.GetVarp(id) &^ music.SettingsMask) | sent
		}
		s.SendVarp(id, sent)
	}
}
