package session

import (
	"bytes"
	"sync"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/music"
	"tsps-go/internal/protocol"
)

// Session must satisfy the game's varbit-read seam structurally,
// as it satisfies game.SessionPort (see sessionport_test.go):
// implementing GetVarbit activates the examine price line with no
// change in the game package (game/examine_price.go).
var _ game.VarbitReader = (*Session)(nil)

// fixtureVarbits is a hand-built varbit-definition source for the
// packing tests: no real cache is involved. The ids are arbitrary
// fixtures except 13826/13827/13130, which are the real setting
// varbits (ExaminePrice.ts:7,9; Trading.ts:50) mapped onto fixture
// backing varps so the activation tests exercise the real ids.
func fixtureVarbits() VarbitResolver {
	defs := map[int][3]int{
		100:   {10, 0, 0},   // 1-bit field, bit 0 of varp 10
		101:   {10, 1, 3},   // 3-bit field, bits 1-3 of varp 10
		102:   {11, 0, 31},  // full 32-bit field of varp 11
		103:   {12, 4, 7},   // 4-bit field, bits 4-7 of varp 12
		104:   {13, 5, 2},   // invalid: end before start
		13826: {2000, 0, 0}, // examine-price GE setting (fixture backing)
		13827: {2000, 1, 1}, // examine-price alchemy setting (fixture backing)
		13130: {3000, 0, 0}, // trade accept-delay setting (fixture backing)
	}
	return func(id int) (int, int, int, bool) {
		d, ok := defs[id]
		if !ok {
			return 0, 0, 0, false
		}
		return d[0], d[1], d[2], true
	}
}

func TestVarpStoreDefaults(t *testing.T) {
	st := &varpStore{values: make(map[int]int)}
	if got := st.get(5); got != 0 {
		t.Errorf("fresh store get(5) = %d, want 0", got)
	}
	if got := st.getVarbit(100); got != 0 {
		t.Errorf("fresh store getVarbit(100) = %d, want 0 (no resolver)", got)
	}
	// The zero value reads 0 and accepts sets.
	var zero varpStore
	if got := zero.get(5); got != 0 {
		t.Errorf("zero store get(5) = %d, want 0", got)
	}
	zero.set(5, 7)
	if got := zero.get(5); got != 7 {
		t.Errorf("zero store after set get(5) = %d, want 7", got)
	}
}

func TestVarpStoreSetTruncatesToInt32(t *testing.T) {
	st := &varpStore{values: make(map[int]int)}
	// sendConfig stores `state | 0`: a value past int32 range
	// truncates to its low 32 bits, as JS would.
	st.set(5, 1<<32+7)
	if got := st.get(5); got != 7 {
		t.Errorf("get(5) after set(1<<32+7) = %d, want 7", got)
	}
	st.set(6, -1)
	if got := st.get(6); got != -1 {
		t.Errorf("get(6) after set(-1) = %d, want -1", got)
	}
}

func TestVarpStoreReplayOrder(t *testing.T) {
	st := &varpStore{values: make(map[int]int)}
	st.set(30, 1)
	st.set(10, 2)
	st.set(20, 3)
	st.set(10, 4) // Re-set: keeps its original position, as a JS Map does.
	want := [][2]int{{30, 1}, {10, 4}, {20, 3}}
	if got := st.entries(); len(got) != len(want) {
		t.Fatalf("entries() = %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("entries()[%d] = %v, want %v", i, got[i], want[i])
			}
		}
	}
}

func TestVarpStoreGetVarbitPacking(t *testing.T) {
	st := &varpStore{values: make(map[int]int)}
	st.setResolver(fixtureVarbits())
	st.set(10, 0b1101) // bits 0,2,3 set
	if got := st.getVarbit(100); got != 1 {
		t.Errorf("getVarbit(100) = %d, want 1 (bit 0)", got)
	}
	if got := st.getVarbit(101); got != 0b110 {
		t.Errorf("getVarbit(101) = %d, want 6 (bits 1-3)", got)
	}
	// An arithmetic shift: a negative backing varp sign-extends,
	// and the mask keeps only the field.
	st.set(10, -1)
	if got := st.getVarbit(101); got != 0b111 {
		t.Errorf("getVarbit(101) of -1 = %d, want 7", got)
	}
	// The full-width field reads the whole int32.
	st.set(11, -12345)
	if got := st.getVarbit(102); got != -12345 {
		t.Errorf("getVarbit(102) = %d, want -12345", got)
	}
	// An unset backing varp reads 0 through any field.
	if got := st.getVarbit(103); got != 0 {
		t.Errorf("getVarbit(103) = %d, want 0 (unset varp 12)", got)
	}
	// Unknown ids and invalid widths read 0, as TS does (see
	// varpStore.getVarbit): no definition, no field.
	if got := st.getVarbit(999); got != 0 {
		t.Errorf("getVarbit(999) = %d, want 0 (unknown id)", got)
	}
	if got := st.getVarbit(104); got != 0 {
		t.Errorf("getVarbit(104) = %d, want 0 (end before start)", got)
	}
}

func TestVarpStoreApplyVarbitPacking(t *testing.T) {
	st := &varpStore{values: make(map[int]int)}
	st.setResolver(fixtureVarbits())
	st.applyVarbit(101, 5)
	if got := st.get(10); got != 5<<1 {
		t.Errorf("varp 10 after applyVarbit(101, 5) = %d, want %d", got, 5<<1)
	}
	// Other bits in the shared backing varp are preserved.
	st.set(10, 0xFF)
	st.applyVarbit(101, 0)
	if got := st.get(10); got != 0xF1 {
		t.Errorf("varp 10 after clearing bits 1-3 of 0xFF = %#x, want 0xF1", got)
	}
	// Out-of-range values clamp to 0 (mask 7 for a 3-bit field),
	// clearing the field rather than overflowing into neighbors.
	st.set(10, 0xFF)
	st.applyVarbit(101, 8)
	if got := st.get(10); got != 0xF1 {
		t.Errorf("varp 10 after applyVarbit(101, 8) = %#x, want 0xF1", got)
	}
	// Negative values clamp to 0 too.
	st.set(10, 0xFF)
	st.applyVarbit(101, -1)
	if got := st.get(10); got != 0xF1 {
		t.Errorf("varp 10 after applyVarbit(101, -1) = %#x, want 0xF1", got)
	}
	// A full-width field takes any non-negative value whole.
	st.applyVarbit(102, 0x12345678)
	if got := st.get(11); got != 0x12345678 {
		t.Errorf("varp 11 after applyVarbit(102, 0x12345678) = %#x", got)
	}
	// Unknown ids and invalid widths record nothing.
	st.applyVarbit(999, 1)
	if got := st.get(10); got != 0xF1 {
		t.Errorf("varp 10 after applyVarbit(999, 1) = %#x, want unchanged 0xF1", got)
	}
	st.set(13, 0xAA)
	st.applyVarbit(104, 1)
	if got := st.get(13); got != 0xAA {
		t.Errorf("varp 13 after applyVarbit(104, 1) = %#x, want unchanged 0xAA", got)
	}
}

func TestSendVarpRecordsAndSends(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendVarp(3078, 42)
	if got := s.GetVarp(3078); got != 42 {
		t.Errorf("GetVarp(3078) = %d, want 42", got)
	}
	packets := queuedPackets(t, s)
	want := protocol.EncodeVarp(3078, 42)
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Errorf("queued = %x, want [%x]", packets, want)
	}
	// An unset varp reads 0.
	if got := s.GetVarp(9999); got != 0 {
		t.Errorf("GetVarp(9999) = %d, want 0", got)
	}
}

func TestSendToggleAndVarpLong(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// sendToggle is sendConfig under another name: it records.
	s.SendToggle(500, 3)
	if got := s.GetVarp(500); got != 3 {
		t.Errorf("GetVarp(500) after SendToggle = %d, want 3", got)
	}
	// sendVarpLong records nothing: the store holds 32-bit varps.
	s.SendVarpLong(501, 1<<40)
	if got := s.GetVarp(501); got != 0 {
		t.Errorf("GetVarp(501) after SendVarpLong = %d, want 0", got)
	}
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued = %d packets, want 2", len(packets))
	}
	if want := protocol.EncodeVarp(500, 3); !bytes.Equal(packets[0], want) {
		t.Errorf("packet 0 = %x, want %x", packets[0], want)
	}
	if want := protocol.EncodeVarpLong(501, 1<<40); !bytes.Equal(packets[1], want) {
		t.Errorf("packet 1 = %x, want %x", packets[1], want)
	}
}

func TestSendVarbitRecordsAndSendsOriginalValue(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SetVarbitResolver(fixtureVarbits())
	s.SendVarp(10, 0)
	// The frame carries the original value even when the record
	// clamps it to 0 (PacketSender.ts:1530 sends `value`).
	s.SendVarbit(101, 8)
	if got := s.GetVarp(10); got != 0 {
		t.Errorf("GetVarp(10) after SendVarbit(101, 8) = %d, want 0 (clamped record)", got)
	}
	if got := s.GetVarbit(101); got != 0 {
		t.Errorf("GetVarbit(101) = %d, want 0", got)
	}
	s.SendVarbit(101, 5)
	if got := s.GetVarbit(101); got != 5 {
		t.Errorf("GetVarbit(101) = %d, want 5", got)
	}
	packets := queuedPackets(t, s)
	if len(packets) != 3 {
		t.Fatalf("queued = %d packets, want 3", len(packets))
	}
	if want := protocol.EncodeVarbit(101, 8); !bytes.Equal(packets[1], want) {
		t.Errorf("packet 1 = %x, want original-value frame %x", packets[1], want)
	}
	if want := protocol.EncodeVarbit(101, 5); !bytes.Equal(packets[2], want) {
		t.Errorf("packet 2 = %x, want %x", packets[2], want)
	}
}

func TestVarbitWithoutResolverSendsButRecordsNothing(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// No resolver installed: reads are 0, sends still go out,
	// and no backing varp is recorded.
	if got := s.GetVarbit(101); got != 0 {
		t.Errorf("GetVarbit(101) without resolver = %d, want 0", got)
	}
	s.SendVarbit(101, 5)
	if got := s.GetVarp(10); got != 0 {
		t.Errorf("GetVarp(10) after resolver-less SendVarbit = %d, want 0", got)
	}
	packets := queuedPackets(t, s)
	if len(packets) != 1 || !bytes.Equal(packets[0], protocol.EncodeVarbit(101, 5)) {
		t.Errorf("queued = %x, want one EncodeVarbit(101, 5) frame", packets)
	}
}

func TestReplayVarpsResendsEveryEntryInOrder(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendVarp(30, 1)
	s.SendVarp(10, 2)
	s.SendVarp(10, 4)
	s.ReplayVarps()
	packets := queuedPackets(t, s)
	// 3 original sends + 2 replayed entries (varp 10 once, with
	// its latest value), in first-sent order.
	want := [][]byte{
		protocol.EncodeVarp(30, 1),
		protocol.EncodeVarp(10, 2),
		protocol.EncodeVarp(10, 4),
		protocol.EncodeVarp(30, 1),
		protocol.EncodeVarp(10, 4),
	}
	if len(packets) != len(want) {
		t.Fatalf("queued = %d packets, want %d", len(packets), len(want))
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Errorf("packet %d = %x, want %x", i, packets[i], want[i])
		}
	}
	// An empty store replays nothing.
	empty := NewSession(NewHeadlessChannel(""))
	empty.ReplayVarps()
	if got := queuedPackets(t, empty); len(got) != 0 {
		t.Errorf("empty replay queued %d packets, want 0", len(got))
	}
}

func TestApplyAudioSettingsMergesSettingsVarp(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// Server-owned bits of varp 19 already recorded: bit 1 is
	// outside SETTINGS_MASK, so the merge must preserve it.
	const serverBits = 0x2
	s.SendVarp(music.SettingsVarp, serverBits)
	s.ApplyAudioSettings(map[int]int{
		music.ModeVarp:     2,
		music.SettingsVarp: music.DefaultSettings, // loop bit on, client-owned
		168:                100,
	})
	want19 := (serverBits &^ music.SettingsMask) | music.DefaultSettings
	if got := s.GetVarp(music.SettingsVarp); got != want19 {
		t.Errorf("GetVarp(19) = %#x, want %#x", got, want19)
	}
	if got := s.GetVarp(music.ModeVarp); got != 2 {
		t.Errorf("GetVarp(18) = %d, want 2", got)
	}
	// Sends go in ascending varp-id order (TS Object.entries
	// order): the initial send plus 18, 19, 168.
	packets := queuedPackets(t, s)
	want := [][]byte{
		protocol.EncodeVarp(music.SettingsVarp, serverBits),
		protocol.EncodeVarp(music.ModeVarp, 2),
		protocol.EncodeVarp(music.SettingsVarp, want19),
		protocol.EncodeVarp(168, 100),
	}
	if len(packets) != len(want) {
		t.Fatalf("queued = %d packets, want %d", len(packets), len(want))
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Errorf("packet %d = %x, want %x", i, packets[i], want[i])
		}
	}
	// A nil map sends nothing.
	s.ApplyAudioSettings(nil)
	if got := len(queuedPackets(t, s)); got != len(want) {
		t.Errorf("queued after nil ApplyAudioSettings = %d, want %d", got, len(want))
	}
}

func TestSessionWithoutStoreDegradesToSendOnly(t *testing.T) {
	// A Session built without a store (only possible before
	// Worker 1's NewSession wiring, or by direct construction):
	// sends still go out, reads are 0, replay is a no-op, and
	// installing a resolver creates the store.
	s := &Session{ch: NewHeadlessChannel("")}
	if got := s.GetVarp(1); got != 0 {
		t.Errorf("GetVarp = %d, want 0", got)
	}
	if got := s.GetVarbit(101); got != 0 {
		t.Errorf("GetVarbit = %d, want 0", got)
	}
	s.SendVarp(1, 9)
	s.SendVarbit(101, 5)
	s.ReplayVarps()
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued = %d packets, want 2 (replay sends nothing)", len(packets))
	}
	s.SetVarbitResolver(fixtureVarbits())
	s.SendVarbit(101, 5)
	if got := s.GetVarbit(101); got != 5 {
		t.Errorf("GetVarbit(101) after resolver install = %d, want 5", got)
	}
}

func TestVarpStoreConcurrentAccess(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SetVarbitResolver(fixtureVarbits())
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s.SendVarp(g, i)
				_ = s.GetVarp(g)
				s.SendVarbit(101, i&7)
				_ = s.GetVarbit(101)
				s.ReplayVarps()
			}
		}(g)
	}
	wg.Wait()
}

// TestExaminePriceLineActivatesViaStore proves the Phase 12 gap
// closes: with the examine-price varbits set through the store,
// SendExaminePriceLine sends the line; with them unset, it stays
// silent — the game code unchanged, the session now satisfying
// game.VarbitReader.
func TestExaminePriceLineActivatesViaStore(t *testing.T) {
	newPlayer := func(s *Session) *game.Player {
		p := game.NewPlayer("alice", game.Location{})
		p.Session = s
		return p
	}
	prices := game.ExaminePrices{GE: 67, HA: 240, LA: 160}

	// Unset: both settings off, nothing is sent.
	quiet := NewSession(NewHeadlessChannel(""))
	quiet.SetVarbitResolver(fixtureVarbits())
	game.SendExaminePriceLine(newPlayer(quiet), "Steel scimitar", prices, 1)
	if got := queuedPackets(t, quiet); len(got) != 0 {
		t.Errorf("unset settings queued %d packets, want 0", len(got))
	}

	// GE on only: the line shows GE alone.
	geOnly := NewSession(NewHeadlessChannel(""))
	geOnly.SetVarbitResolver(fixtureVarbits())
	geOnly.SendVarp(2000, 0b01)
	game.SendExaminePriceLine(newPlayer(geOnly), "Steel scimitar", prices, 1)
	packets := queuedPackets(t, geOnly)
	if len(packets) != 2 { // the varp frame + the chat line
		t.Fatalf("GE-on queued %d packets, want 2", len(packets))
	}
	wantLine, ok := game.FormatExaminePrice("Steel scimitar", 1, prices, true, false)
	if !ok {
		t.Fatal("FormatExaminePrice returned no line for GE-on")
	}
	want := protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindGame, Text: wantLine, PlayerID: -1,
	})
	if !bytes.Equal(packets[1], want) {
		t.Errorf("price line packet = %x, want %x", packets[1], want)
	}

	// Both on: GE + alchemy values.
	both := NewSession(NewHeadlessChannel(""))
	both.SetVarbitResolver(fixtureVarbits())
	both.SendVarp(2000, 0b11)
	game.SendExaminePriceLine(newPlayer(both), "Steel scimitar", prices, 1)
	packets = queuedPackets(t, both)
	if len(packets) != 2 {
		t.Fatalf("both-on queued %d packets, want 2", len(packets))
	}
	wantLine, _ = game.FormatExaminePrice("Steel scimitar", 1, prices, true, true)
	want = protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindGame, Text: wantLine, PlayerID: -1,
	})
	if !bytes.Equal(packets[1], want) {
		t.Errorf("price line packet = %x, want %x", packets[1], want)
	}
}
