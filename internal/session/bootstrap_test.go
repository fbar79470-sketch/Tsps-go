package session

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"

	"tsps-go/internal/persist"
	"tsps-go/internal/protocol"
)

// splitServerPackets splits one server->client frame using the server
// length table. protocol.DecodeFrame decodes client->server frames
// only, so bootstrap assertions need this mirror.
func splitServerPackets(t *testing.T, frame []byte) []protocol.Packet {
	t.Helper()
	var out []protocol.Packet
	rest := frame
	for len(rest) > 0 {
		opcode := rest[0]
		n, ok := protocol.ServerPacketLength(opcode)
		if !ok {
			t.Fatalf("unknown server opcode %d", opcode)
		}
		rest = rest[1:]
		var length int
		switch {
		case n == -1:
			if len(rest) < 1 {
				t.Fatal("truncated -1 length prefix")
			}
			length = int(rest[0])
			rest = rest[1:]
		case n == -2:
			if len(rest) < 2 {
				t.Fatal("truncated -2 length prefix")
			}
			length = int(binary.BigEndian.Uint16(rest[:2]))
			rest = rest[2:]
		default:
			length = n
		}
		if len(rest) < length {
			t.Fatalf("truncated server packet opcode %d: %d bytes, want %d", opcode, len(rest), length)
		}
		payload := append([]byte(nil), rest[:length]...)
		out = append(out, protocol.Packet{Opcode: opcode, Payload: payload})
		rest = rest[length:]
	}
	return out
}

// flushedBootstrap flushes the entered connection's session queue and
// returns the bootstrap packets: the login response went direct to the
// channel, so the flush frame holds exactly the bootstrap sequence.
func flushedBootstrap(t *testing.T, f *connFixture) []protocol.Packet {
	t.Helper()
	if err := f.conn.Session().Flush(); err != nil {
		t.Fatalf("session Flush: %v", err)
	}
	sent := f.ch.sentBytes()
	if len(sent) < 2 {
		t.Fatalf("sent frames = %d, want >= 2 (login response + bootstrap)", len(sent))
	}
	return splitServerPackets(t, sent[len(sent)-1])
}

type handshakePacket struct {
	index   int32
	name    string
	present byte
	gender  byte
	colors  []int
	kits    []int
	equip   []int
	icons   []int
	admin   byte
	members byte
}

func parseHandshake(t *testing.T, payload []byte) handshakePacket {
	t.Helper()
	r := protocol.NewReader(payload)
	var out handshakePacket
	var err error
	if out.index, err = r.Int(); err != nil {
		t.Fatalf("handshake index: %v", err)
	}
	if out.name, err = r.String(); err != nil {
		t.Fatalf("handshake name: %v", err)
	}
	if out.present, err = r.Byte(); err != nil {
		t.Fatalf("handshake appearance flag: %v", err)
	}
	if out.gender, err = r.Byte(); err != nil {
		t.Fatalf("handshake gender: %v", err)
	}
	readBytes := func() []int {
		t.Helper()
		count, err := r.Byte()
		if err != nil {
			t.Fatalf("handshake count: %v", err)
		}
		values := make([]int, count)
		for i := range values {
			b, err := r.Byte()
			if err != nil {
				t.Fatalf("handshake value: %v", err)
			}
			values[i] = int(b)
		}
		return values
	}
	readShorts := func() []int {
		t.Helper()
		count, err := r.Byte()
		if err != nil {
			t.Fatalf("handshake count: %v", err)
		}
		values := make([]int, count)
		for i := range values {
			s, err := r.Short()
			if err != nil {
				t.Fatalf("handshake value: %v", err)
			}
			values[i] = int(int16(s))
		}
		return values
	}
	out.colors = readBytes()
	out.kits = readShorts()
	out.equip = readShorts()
	out.icons = readBytes()
	if _, err = r.String(); err != nil { // empty string
		t.Fatalf("handshake empty string: %v", err)
	}
	if out.admin, err = r.Byte(); err != nil {
		t.Fatalf("handshake admin: %v", err)
	}
	if out.members, err = r.Byte(); err != nil {
		t.Fatalf("handshake members: %v", err)
	}
	if r.Remaining() != 0 {
		t.Fatalf("handshake has %d trailing bytes", r.Remaining())
	}
	return out
}

func parseVarbit(t *testing.T, payload []byte) (id int, value int32) {
	t.Helper()
	r := protocol.NewReader(payload)
	rawID, err := r.Short()
	if err != nil {
		t.Fatalf("varbit id: %v", err)
	}
	value, err = r.Int()
	if err != nil {
		t.Fatalf("varbit value: %v", err)
	}
	return int(rawID), value
}

func parseVarp(t *testing.T, opcode byte, payload []byte) (id, value int) {
	t.Helper()
	r := protocol.NewReader(payload)
	rawID, err := r.Short()
	if err != nil {
		t.Fatalf("varp id: %v", err)
	}
	switch opcode {
	case protocol.ServerVarpSmall:
		b, err := r.Byte()
		if err != nil {
			t.Fatalf("varp value: %v", err)
		}
		return int(rawID), int(b)
	case protocol.ServerVarpLarge:
		v, err := r.Int()
		if err != nil {
			t.Fatalf("varp value: %v", err)
		}
		return int(rawID), int(v)
	default:
		t.Fatalf("not a varp opcode: %d", opcode)
		return 0, 0
	}
}

type skillRow struct {
	id, xp, base, virtual, boost, current int
}

func parseSkills(t *testing.T, payload []byte) ([]skillRow, int, int) {
	t.Helper()
	if len(payload) < 1 {
		t.Fatal("empty skills payload")
	}
	count := int(payload[0])
	rows := make([]skillRow, count)
	offset := 1
	for i := range rows {
		if offset+9 > len(payload) {
			t.Fatalf("skills payload truncated at row %d", i)
		}
		rows[i] = skillRow{
			id:      int(payload[offset]),
			xp:      int(int32(binary.BigEndian.Uint32(payload[offset+1 : offset+5]))),
			base:    int(payload[offset+5]),
			virtual: int(payload[offset+6]),
			boost:   int(payload[offset+7]) - 128,
			current: int(payload[offset+8]),
		}
		offset += 9
	}
	// The encoder allocates 10 bytes per skill but writes 9, so the
	// payload ends with one zero slack byte per skill AFTER the
	// total/combat trailer; the trailer sits at 1 + 9*count.
	total := int(binary.BigEndian.Uint16(payload[offset : offset+2]))
	combat := int(payload[offset+2])
	return rows, total, combat
}

func parseInventorySnapshot(t *testing.T, payload []byte) []protocol.ItemSlot {
	t.Helper()
	r := protocol.NewReader(payload)
	count, err := r.Short()
	if err != nil {
		t.Fatalf("inventory count: %v", err)
	}
	slots := make([]protocol.ItemSlot, count)
	for i := range slots {
		slot, err := r.Short()
		if err != nil {
			t.Fatalf("slot %d index: %v", i, err)
		}
		idPlus, err := r.Short()
		if err != nil {
			t.Fatalf("slot %d id: %v", i, err)
		}
		qtyByte, err := r.Byte()
		if err != nil {
			t.Fatalf("slot %d quantity: %v", i, err)
		}
		qty := int(qtyByte)
		if qtyByte == 255 {
			q, err := r.Int()
			if err != nil {
				t.Fatalf("slot %d large quantity: %v", i, err)
			}
			qty = int(q)
		}
		slots[i] = protocol.ItemSlot{Slot: int(slot), ItemID: int(int16(idPlus)) - 1, Quantity: qty}
	}
	return slots
}

// parseRunScript reads a WIDGET_RUN_SCRIPT payload: script id, then the
// argument list. It returns the script id and the decoded int/string
// arguments in order.
func parseRunScript(t *testing.T, payload []byte) (int32, []any) {
	t.Helper()
	r := protocol.NewReader(payload)
	id, err := r.Int()
	if err != nil {
		t.Fatalf("run-script id: %v", err)
	}
	count, err := r.Byte()
	if err != nil {
		t.Fatalf("run-script arg count: %v", err)
	}
	args := make([]any, 0, count)
	for i := 0; i < int(count); i++ {
		tag, err := r.Byte()
		if err != nil {
			t.Fatalf("run-script arg tag: %v", err)
		}
		switch tag {
		case 0:
			s, err := r.String()
			if err != nil {
				t.Fatalf("run-script string arg: %v", err)
			}
			args = append(args, s)
		case 1:
			v, err := r.Int()
			if err != nil {
				t.Fatalf("run-script int arg: %v", err)
			}
			args = append(args, v)
		case 2:
			t.Fatal("run-script bigint arg: not expected in bootstrap tests")
		default:
			t.Fatalf("run-script arg tag %d", tag)
		}
	}
	return id, args
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBootstrapFreshCharacterSequence(t *testing.T) {
	f := newConnFixture(t)
	f.world.nextIndex = 1
	f.world.cycle = 42
	// A brand-new account: no seeded save, so the bootstrap uses the
	// fresh-character defaults throughout.
	f.loginAndEnter(t, "Carol", "secret")
	packets := flushedBootstrap(t, f)

	// 1. HANDSHAKE first: index, username, male defaults, no admin,
	// members world (nil BootstrapWorld defaults).
	if len(packets) == 0 || packets[0].Opcode != protocol.ServerHandshake {
		t.Fatalf("first packet opcode = %v, want HANDSHAKE", packets[0].Opcode)
	}
	hs := parseHandshake(t, packets[0].Payload)
	if hs.index != 1 || hs.name != "Carol" || hs.present != 1 || hs.gender != 0 {
		t.Errorf("handshake = %+v, want index 1 / Carol / present / male", hs)
	}
	if !equalInts(hs.colors, []int{2, 14, 5, 4, 0}) {
		t.Errorf("handshake colors = %v, want [2 14 5 4 0]", hs.colors)
	}
	if !equalInts(hs.kits, []int{3, 14, 18, 26, 34, 38, 42}) {
		t.Errorf("handshake kits = %v, want male defaults", hs.kits)
	}
	if len(hs.equip) != 14 {
		t.Fatalf("handshake equip length = %d, want 14", len(hs.equip))
	}
	for i, id := range hs.equip {
		if id != -1 {
			t.Errorf("handshake equip[%d] = %d, want -1 (nothing worn)", i, id)
		}
	}
	if len(hs.icons) != 0 || hs.admin != 0 || hs.members != 1 {
		t.Errorf("handshake icons/admin/members = %v/%d/%d, want []/0/1", hs.icons, hs.admin, hs.members)
	}

	// 2. Default animations, byte-exact.
	if packets[1].Opcode != protocol.ServerAnim || !bytes.Equal(packets[1].Payload, mustPayload(t, protocol.EncodeDefaultAnimations())) {
		t.Errorf("second packet is not the default animations packet")
	}

	// 3. Gameframe bootstrap, byte-exact against the encoder, with the
	// world cycle as the map clock.
	wantGameframe := protocol.EncodeGameframeBootstrap("Carol", 161, false, "REGULAR", true, 42)
	cursor := 2
	if len(packets) < cursor+len(wantGameframe) {
		t.Fatalf("packets = %d, want at least %d", len(packets), cursor+len(wantGameframe))
	}
	for i, want := range wantGameframe {
		got := packets[cursor+i]
		framed, err := protocol.EncodePacket(got.Opcode, got.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(framed, want) {
			t.Errorf("gameframe packet %d mismatch: got opcode %d", i, got.Opcode)
		}
	}
	cursor += len(wantGameframe)
	// The root packet inside the gameframe names 161.
	rootSet := false
	for _, p := range packets[2:cursor] {
		if p.Opcode == protocol.ServerWidgetSetRoot && len(p.Payload) == 2 &&
			int(binary.BigEndian.Uint16(p.Payload)) == 161 {
			rootSet = true
		}
	}
	if !rootSet {
		t.Error("gameframe does not set root 161")
	}
	// Script 876 carries the map clock (42), 0, the name, and REGULAR.
	found876 := false
	for _, p := range packets[2:cursor] {
		if p.Opcode != protocol.ServerWidgetRunScript {
			continue
		}
		id, args := parseRunScript(t, p.Payload)
		if id == 876 {
			found876 = true
			if len(args) != 4 || args[0] != int32(42) || args[1] != int32(0) || args[2] != "Carol" || args[3] != "REGULAR" {
				t.Errorf("script 876 args = %v, want [42 0 Carol REGULAR]", args)
			}
		}
	}
	if !found876 {
		t.Error("gameframe has no script 876 packet")
	}

	// 4a. Spell-unlock varbits, then varps, then varbit 4070 = 0.
	for _, pair := range spellUnlockVarbits {
		if packets[cursor].Opcode != protocol.ServerVarbit {
			t.Fatalf("packet %d opcode = %d, want VARBIT", cursor, packets[cursor].Opcode)
		}
		id, value := parseVarbit(t, packets[cursor].Payload)
		if id != pair[0] || value != int32(pair[1]) {
			t.Errorf("spell varbit = (%d, %d), want (%d, %d)", id, value, pair[0], pair[1])
		}
		cursor++
	}
	for _, pair := range spellUnlockVarps {
		id, value := parseVarp(t, packets[cursor].Opcode, packets[cursor].Payload)
		if id != pair[0] || value != pair[1] {
			t.Errorf("spell varp = (%d, %d), want (%d, %d)", id, value, pair[0], pair[1])
		}
		cursor++
	}
	if packets[cursor].Opcode != protocol.ServerVarbit {
		t.Fatalf("packet %d opcode = %d, want VARBIT 4070", cursor, packets[cursor].Opcode)
	}
	id, value := parseVarbit(t, packets[cursor].Payload)
	if id != 4070 || value != 0 {
		t.Errorf("spellbook varbit = (%d, %d), want (4070, 0)", id, value)
	}
	cursor++

	// 4a continued: the 218 mount + tab setup.
	if packets[cursor].Opcode != protocol.ServerWidgetOpenSub {
		t.Fatalf("packet %d opcode = %d, want WIDGET_OPEN_SUB", cursor, packets[cursor].Opcode)
	}
	mount := packets[cursor].Payload
	if len(mount) < 6 || int(binary.BigEndian.Uint32(mount[:4])) != (161<<16)|82 ||
		int(binary.BigEndian.Uint16(mount[4:6])) != 218 {
		t.Errorf("spellbook mount payload = %v, want uid (161<<16)|82 group 218", mount[:6])
	}
	cursor++
	wantSetup := protocol.EncodeGameframeTabSetup(218, 161)
	for i, want := range wantSetup {
		framed, err := protocol.EncodePacket(packets[cursor+i].Opcode, packets[cursor+i].Payload)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(framed, want) {
			t.Errorf("tab setup packet %d mismatch", i)
		}
	}
	cursor += len(wantSetup)

	// 4b. Script 828 with the members flag (1).
	if packets[cursor].Opcode != protocol.ServerWidgetRunScript {
		t.Fatalf("packet %d opcode = %d, want WIDGET_RUN_SCRIPT 828", cursor, packets[cursor].Opcode)
	}
	scriptID, args := parseRunScript(t, packets[cursor].Payload)
	if scriptID != 828 || len(args) != 1 || args[0] != int32(1) {
		t.Errorf("script 828 = (%d, %v), want (828, [1])", scriptID, args)
	}
	cursor++

	// 4c. Inventory snapshot: 28 empty slots. No equipment packet for
	// a browser client.
	if packets[cursor].Opcode != protocol.ServerInventorySnapshot {
		t.Fatalf("packet %d opcode = %d, want INVENTORY_SNAPSHOT", cursor, packets[cursor].Opcode)
	}
	slots := parseInventorySnapshot(t, packets[cursor].Payload)
	if len(slots) != 28 {
		t.Fatalf("inventory slots = %d, want 28", len(slots))
	}
	for i, s := range slots {
		if s.Slot != i || s.ItemID != -1 || s.Quantity != 0 {
			t.Errorf("inventory slot %d = %+v, want empty", i, s)
		}
	}
	cursor++

	// 4e. Skills snapshot: 24 skills, Hitpoints 10/1184, total 33,
	// combat 3. Then run energy: 10000 units, running, weight 0.
	if packets[cursor].Opcode != protocol.ServerSkillsSnapshot {
		t.Fatalf("packet %d opcode = %d, want SKILLS_SNAPSHOT", cursor, packets[cursor].Opcode)
	}
	rows, total, combat := parseSkills(t, packets[cursor].Payload)
	if len(rows) != 24 {
		t.Fatalf("skill rows = %d, want 24", len(rows))
	}
	wantIDs := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 22, 21, 23}
	for i, row := range rows {
		if row.id != wantIDs[i] {
			t.Errorf("skill row %d id = %d, want %d", i, row.id, wantIDs[i])
		}
		wantXP, wantLevel := 0, 1
		if i == persist.HitpointsIndex {
			wantXP, wantLevel = 1184, 10
		}
		if row.xp != wantXP || row.base != wantLevel || row.virtual != wantLevel || row.boost != 0 || row.current != wantLevel {
			t.Errorf("skill row %d = %+v, want xp %d level %d", i, row, wantXP, wantLevel)
		}
	}
	if total != 33 || combat != 3 {
		t.Errorf("skills total/combat = %d/%d, want 33/3", total, combat)
	}
	cursor++
	if packets[cursor].Opcode != protocol.ServerRunEnergy {
		t.Fatalf("packet %d opcode = %d, want RUN_ENERGY", cursor, packets[cursor].Opcode)
	}
	wantEnergy := []byte{100, 1, 0x27, 0x10, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(packets[cursor].Payload, wantEnergy) {
		t.Errorf("run energy = %v, want %v", packets[cursor].Payload, wantEnergy)
	}
	cursor++

	// 5. Quick prayers: empty mask, not enabled. Nothing follows.
	if len(packets) != cursor+2 {
		t.Fatalf("packets = %d, want %d (trailing packets after quick prayers)", len(packets), cursor+2)
	}
	id, value = parseVarbit(t, packets[cursor].Payload)
	if packets[cursor].Opcode != protocol.ServerVarbit || id != 4102 || value != 0 {
		t.Errorf("quick-prayer mask = opcode %d (%d, %d), want VARBIT (4102, 0)", packets[cursor].Opcode, id, value)
	}
	id, value = parseVarbit(t, packets[cursor+1].Payload)
	if packets[cursor+1].Opcode != protocol.ServerVarbit || id != 4103 || value != 0 {
		t.Errorf("quick-prayer state = opcode %d (%d, %d), want VARBIT (4103, 0)", packets[cursor+1].Opcode, id, value)
	}
}

// mustPayload reframes an encoded packet and returns its payload, for
// comparing encoder output against split packets.
func mustPayload(t *testing.T, framed []byte) []byte {
	t.Helper()
	packets := splitServerPackets(t, framed)
	if len(packets) != 1 {
		t.Fatalf("framed packet split into %d packets", len(packets))
	}
	return packets[0].Payload
}

func TestBootstrapWithSave(t *testing.T) {
	f := newConnFixture(t)
	f.world.nextIndex = 1
	f.world.cycle = 7
	f.login.seedAccount(t, "Alice", "secret")
	save, err := f.login.store.Load("Alice", persist.ProfileMain)
	if err != nil || save == nil {
		t.Fatalf("seeded save: (%v, %v)", save, err)
	}
	skills := persist.DefaultSkills()
	skills.Experience[0] = 13034431 // Attack 99
	skills.MaxLevel[0] = 99
	skills.Level[0] = 99
	*save = persist.PlayerSave{
		DisplayName:          save.DisplayName,
		PasswordHashWithSalt: save.PasswordHashWithSalt,
		Rights:               persist.Rights{ID: 2, SpriteID: 619},
		SpellBook:            persist.SpellBook{InterfaceID: persist.SpellbookAncientInterfaceID},
		SkullTimer:           5,
		SkullType:            persist.SkullType{IconID: 1},
		Running:              false,
		RunEnergy:            50.5,
		Inventory: []persist.Item{
			{ID: 995, Amount: 1000},
			{ID: -1, Amount: 0},
			{ID: -1, Amount: 0},
			{ID: -1, Amount: 0},
			{ID: -1, Amount: 0},
			{ID: 4151, Amount: 1},
		},
		Equipment: []persist.Item{
			{ID: 1163, Amount: 1},
			{ID: -1, Amount: 0},
			{ID: -1, Amount: 0},
			{ID: 1127, Amount: 1},
		},
		Appearance: []int{1, 48, 57, 65, 68, 77, 80, -1, 7, 14, 5, 4, 0},
		Skills:     skills,
		QuickPrayers: []*persist.QuickPrayer{
			{ConfigID: 5609, PrayerID: 0},
			nil, nil, nil, nil,
			{ConfigID: 5610, PrayerID: 5},
		},
		Attributes: map[string]json.RawMessage{
			"client-layout-root":   json.RawMessage("548"),
			"xp-drops:enabled":     json.RawMessage("true"),
			"appearance:pronouns":  json.RawMessage("2"),
			"unrelated:attribute":  json.RawMessage(`"ignored"`),
			"client-layout:unused": json.RawMessage("161"),
		},
	}
	f.loginAndEnter(t, "Alice", "secret")
	packets := flushedBootstrap(t, f)

	hs := parseHandshake(t, packets[0].Payload)
	if hs.gender != 1 {
		t.Errorf("handshake gender = %d, want 1 (female save)", hs.gender)
	}
	if !equalInts(hs.colors, []int{7, 14, 5, 4, 0}) {
		t.Errorf("handshake colors = %v, want [7 14 5 4 0]", hs.colors)
	}
	if !equalInts(hs.kits, []int{48, -1, 57, 65, 68, 77, 80}) {
		t.Errorf("handshake kits = %v, want female defaults", hs.kits)
	}
	if hs.equip[0] != 1163 || hs.equip[3] != 1127 || hs.equip[1] != -1 {
		t.Errorf("handshake equip = %v, want 1163@0, 1127@3", hs.equip[:4])
	}
	if hs.admin != 1 || hs.members != 1 {
		t.Errorf("handshake admin/members = %d/%d, want 1/1 (rights id 2)", hs.admin, hs.members)
	}

	// Saved layout 548 wins; XP drops enabled mounts group 122.
	wantGameframe := protocol.EncodeGameframeBootstrap("Alice", 548, true, "REGULAR", true, 7)
	if len(packets) < 2+len(wantGameframe) {
		t.Fatalf("packets = %d, want at least %d", len(packets), 2+len(wantGameframe))
	}
	for i, want := range wantGameframe {
		framed, err := protocol.EncodePacket(packets[2+i].Opcode, packets[2+i].Payload)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(framed, want) {
			t.Errorf("gameframe packet %d mismatch", i)
		}
	}
	cursor := 2 + len(wantGameframe)
	// Skip the spell-unlock varbits/varps.
	cursor += len(spellUnlockVarbits) + len(spellUnlockVarps)
	id, value := parseVarbit(t, packets[cursor].Payload)
	if id != 4070 || value != 1 {
		t.Errorf("spellbook varbit = (%d, %d), want (4070, 1) for Ancient", id, value)
	}
	cursor++
	cursor++ // 218 mount
	cursor += len(protocol.EncodeGameframeTabSetup(218, 548))
	cursor++ // script 828

	// Inventory: slot 0 coins (large quantity), slot 5 whip, rest empty.
	slots := parseInventorySnapshot(t, packets[cursor].Payload)
	if slots[0].ItemID != 995 || slots[0].Quantity != 1000 {
		t.Errorf("inventory slot 0 = %+v, want 995x1000", slots[0])
	}
	if slots[5].ItemID != 4151 || slots[5].Quantity != 1 {
		t.Errorf("inventory slot 5 = %+v, want 4151x1", slots[5])
	}
	if slots[1].ItemID != -1 || len(slots) != 28 {
		t.Errorf("inventory slots = %d, slot 1 = %+v", len(slots), slots[1])
	}
	cursor++

	// Skills: Attack 99, total 131, combat 35.
	rows, total, combat := parseSkills(t, packets[cursor].Payload)
	if rows[0].xp != 13034431 || rows[0].base != 99 || rows[0].current != 99 {
		t.Errorf("attack row = %+v, want 99/13034431", rows[0])
	}
	if rows[persist.HitpointsIndex].xp != 1184 {
		t.Errorf("hitpoints xp = %d, want 1184", rows[persist.HitpointsIndex].xp)
	}
	if total != 131 || combat != 35 {
		t.Errorf("skills total/combat = %d/%d, want 131/35", total, combat)
	}
	cursor++

	// Run energy: floor(50.5*100) = 5050 units, not running.
	wantEnergy := []byte{50, 0, 0x13, 0xba, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(packets[cursor].Payload, wantEnergy) {
		t.Errorf("run energy = %v, want %v", packets[cursor].Payload, wantEnergy)
	}
	cursor++

	// Quick prayers: prayers 0 and 5 selected -> slots 0 and 3 -> mask 9.
	id, value = parseVarbit(t, packets[cursor].Payload)
	if id != 4102 || value != 9 {
		t.Errorf("quick-prayer mask = (%d, %d), want (4102, 9)", id, value)
	}
	id, value = parseVarbit(t, packets[cursor+1].Payload)
	if id != 4103 || value != 0 {
		t.Errorf("quick-prayer state = (%d, %d), want (4103, 0)", id, value)
	}
	if len(packets) != cursor+2 {
		t.Errorf("packets = %d, want %d", len(packets), cursor+2)
	}
	// No native-client equipment packet for a browser handshake.
	for _, p := range packets {
		if p.Opcode == protocol.ServerWidgetRunScript {
			scriptID, _ := parseRunScript(t, p.Payload)
			if scriptID == -1 {
				t.Error("browser client received the native equipment (94) packet")
			}
		}
	}
}

func TestBootstrapAppearanceSkullAndPronouns(t *testing.T) {
	// The handshake's short appearance form does not carry head icons
	// or pronouns, so assert them on the derived struct directly.
	save := &persist.PlayerSave{
		SkullTimer: 3,
		SkullType:  persist.SkullType{IconID: 1},
		Appearance: []int{0, 3, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0},
		Attributes: map[string]json.RawMessage{
			"appearance:pronouns": json.RawMessage("2"),
		},
	}
	ap := bootstrapAppearance(save)
	if ap.HeadIcons == nil || ap.HeadIcons.Skull != 1 || ap.HeadIcons.Prayer != -1 {
		t.Errorf("head icons = %+v, want skull 1 / prayer -1", ap.HeadIcons)
	}
	if ap.Pronouns == nil || *ap.Pronouns != 2 {
		t.Errorf("pronouns = %v, want 2", ap.Pronouns)
	}
	if ap.Hidden || ap.NpcTransformationID != nil {
		t.Errorf("hidden/transformation = %v/%v, want false/nil", ap.Hidden, ap.NpcTransformationID)
	}

	// No skull timer: no skull icon. No save: male defaults, pronouns 0.
	save.SkullTimer = 0
	if got := bootstrapAppearance(save).HeadIcons.Skull; got != -1 {
		t.Errorf("skull with timer 0 = %d, want -1", got)
	}
	fresh := bootstrapAppearance(nil)
	if fresh.Gender != 0 || fresh.Pronouns == nil || *fresh.Pronouns != 0 {
		t.Errorf("fresh appearance = gender %d pronouns %v, want 0/0", fresh.Gender, fresh.Pronouns)
	}
	if fresh.HeadIcons == nil || fresh.HeadIcons.Skull != -1 {
		t.Errorf("fresh head icons = %+v, want skull -1", fresh.HeadIcons)
	}
}

func TestBootstrapMobileAndNativeClient(t *testing.T) {
	f := newConnFixture(t)
	f.world.nextIndex = 1
	f.world.cycle = 3
	frame := clientFrame(protocol.ClientLogin, loginPayload("Dave", "secret", 242, 0))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("login: %v", err)
	}
	frame = clientFrame(protocol.ClientHandshake, handshakePayload("Dave", 1, protocol.HandshakeCapabilityNativeClient, true))
	if err := f.conn.HandleFrame(frame); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if !f.conn.Entered() {
		t.Fatalf("did not enter: %q", f.conn.LoginFailure())
	}
	packets := flushedBootstrap(t, f)

	// Mobile root 601, with the mobile orbs group (897) mount.
	wantGameframe := protocol.EncodeGameframeBootstrap("Dave", 601, false, "REGULAR", true, 3)
	for i, want := range wantGameframe {
		framed, err := protocol.EncodePacket(packets[2+i].Opcode, packets[2+i].Payload)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(framed, want) {
			t.Errorf("gameframe packet %d mismatch", i)
		}
	}
	// The native client gets the worn inventory (94) as a run-script
	// -1 packet with capacity 14 and zero (filtered) slots.
	found := false
	for _, p := range packets {
		if p.Opcode != protocol.ServerWidgetRunScript {
			continue
		}
		scriptID, _ := parseRunScript(t, p.Payload)
		if scriptID != -1 {
			continue
		}
		found = true
		// After the script id + empty args + two empty var sections,
		// the inventories section: count 1, id 94, capacity 14, 0 slots.
		r := protocol.NewReader(p.Payload)
		if _, err := r.Int(); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Byte(); err != nil { // arg count 0
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ { // varp/varbit counts
			count, err := r.Short()
			if err != nil || count != 0 {
				t.Fatalf("run-script -1 var section %d: count %d err %v", i, count, err)
			}
		}
		invCount, err := r.Byte()
		if err != nil || invCount != 1 {
			t.Fatalf("run-script -1 inventory count = %d, err %v", invCount, err)
		}
		invID, err := r.Short()
		if err != nil || invID != 94 {
			t.Fatalf("run-script -1 inventory id = %d, err %v", invID, err)
		}
		capacity, err := r.Short()
		if err != nil || capacity != 14 {
			t.Fatalf("run-script -1 capacity = %d, err %v", capacity, err)
		}
		slotCount, err := r.Short()
		if err != nil || slotCount != 0 {
			t.Fatalf("run-script -1 slot count = %d, err %v", slotCount, err)
		}
	}
	if !found {
		t.Error("native client received no run-script -1 (worn inventory) packet")
	}
}

func TestBootstrapFreeToPlayAndDevUsername(t *testing.T) {
	f := newConnFixture(t)
	f.login.deps.BootstrapWorld = &BootstrapWorld{
		MembersWorld: false,
		TypeLabel:    "REGULAR",
		DevUsernames: []string{" carol ", "other"},
	}
	f.world.nextIndex = 1
	f.loginAndEnter(t, "Carol", "secret")
	packets := flushedBootstrap(t, f)
	hs := parseHandshake(t, packets[0].Payload)
	if hs.admin != 1 {
		t.Errorf("handshake admin = %d, want 1 (DEV_USERNAME match, case-insensitive)", hs.admin)
	}
	if hs.members != 0 {
		t.Errorf("handshake members = %d, want 0 (free-to-play world)", hs.members)
	}
	for _, p := range packets {
		if p.Opcode != protocol.ServerWidgetRunScript {
			continue
		}
		scriptID, args := parseRunScript(t, p.Payload)
		if scriptID == 828 && (len(args) != 1 || args[0] != int32(0)) {
			t.Errorf("script 828 args = %v, want [0] on a free-to-play world", args)
		}
	}
}

func TestBootstrapIdempotent(t *testing.T) {
	f := newConnFixture(t)
	f.loginAndEnter(t, "Eve", "secret")
	first := flushedBootstrap(t, f)
	if err := f.conn.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap after enter: %v", err)
	}
	sentBefore := len(f.ch.sentBytes())
	if err := f.conn.Session().Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(f.ch.sentBytes()) != sentBefore {
		t.Error("second Bootstrap sent packets")
	}
	if len(first) == 0 {
		t.Error("no bootstrap packets on first flush")
	}
}

func TestBootstrapMapClockZeroBeforeFirstTick(t *testing.T) {
	// The fake world reports cycle 0 (no tick has run), which is what
	// TS sends too: World.processCycle starts at 0.
	f := newConnFixture(t)
	f.loginAndEnter(t, "Frank", "secret")
	packets := flushedBootstrap(t, f)
	for _, p := range packets {
		if p.Opcode != protocol.ServerWidgetRunScript {
			continue
		}
		scriptID, args := parseRunScript(t, p.Payload)
		if scriptID == 876 {
			if len(args) != 4 || args[0] != int32(0) {
				t.Errorf("script 876 args = %v, want map clock 0", args)
			}
			return
		}
	}
	t.Error("no script 876 packet")
}

func TestSanitizeLook(t *testing.T) {
	// A nil array yields the male defaults.
	if got := sanitizeLook(nil); got != defaultMaleLook {
		t.Errorf("sanitizeLook(nil) = %v, want %v", got, defaultMaleLook)
	}
	// Female gender selects the female defaults and ranges.
	female := sanitizeLook([]int{1})
	if female != defaultFemaleLook {
		t.Errorf("sanitizeLook([1]) = %v, want %v", female, defaultFemaleLook)
	}
	// Out-of-range kits clamp; the -1 jaw is kept; an unknown skin
	// (13) resets to the default instead of clamping.
	got := sanitizeLook([]int{0, 99, 18, 26, 34, 38, 42, -1, 2, 14, 5, 4, 13})
	if got[lookHead] != 8 {
		t.Errorf("head kit = %d, want clamped 8", got[lookHead])
	}
	if got[lookBeard] != -1 {
		t.Errorf("beard kit = %d, want -1 kept", got[lookBeard])
	}
	if got[lookSkinColour] != 0 {
		t.Errorf("skin colour = %d, want default 0", got[lookSkinColour])
	}
	// Colours clamp into their palettes.
	got = sanitizeLook([]int{0, 3, 18, 26, 34, 38, 42, 14, 99, -5, 5, 4, 12})
	if got[lookHairColour] != 29 || got[lookTorsoCol] != 0 || got[lookSkinColour] != 12 {
		t.Errorf("colours = %v, want hair 29 torso 0 skin 12", got[8:])
	}
	// A non-0/1 gender reads as male.
	if got := sanitizeLook([]int{7}); got[lookGender] != 0 {
		t.Errorf("gender = %d, want 0", got[lookGender])
	}
}

func TestLevelForExperience(t *testing.T) {
	cases := []struct {
		exp, level int
	}{
		{0, 1}, {82, 1}, {83, 2}, {1154, 10}, {1184, 10}, {1358, 11},
		{13034431, 99}, {13034432, 99}, {200000000, 99},
	}
	for _, tc := range cases {
		if got := levelForExperience(tc.exp); got != tc.level {
			t.Errorf("levelForExperience(%d) = %d, want %d", tc.exp, got, tc.level)
		}
	}
	if len(experienceTable) != 99 || experienceTable[0] != 0 || experienceTable[98] != 13034431 {
		t.Errorf("experience table malformed: len %d, first %d, last %d",
			len(experienceTable), experienceTable[0], experienceTable[98])
	}
}

func TestBootstrapSkillsSaveVsFresh(t *testing.T) {
	// No save: the SkillManager constructor defaults (Hitpoints 1184).
	views, total, combat := bootstrapSkills(nil)
	if views[persist.HitpointsIndex].XP != 1184 || total != 33 || combat != 3 {
		t.Errorf("fresh skills: hp xp %d total %d combat %d, want 1184/33/3",
			views[persist.HitpointsIndex].XP, total, combat)
	}
	// A save with empty skill arrays goes through setSkills: Hitpoints
	// keeps at least 1154 XP (getExperienceForLevel(10)).
	views, _, _ = bootstrapSkills(&persist.PlayerSave{})
	if views[persist.HitpointsIndex].XP != 1154 {
		t.Errorf("bare-save hitpoints xp = %d, want 1154", views[persist.HitpointsIndex].XP)
	}
	// Experience implies the base level even when the stored max
	// level lags (getMaxLevel's floor).
	save := &persist.PlayerSave{Skills: persist.Skills{
		Level:      make([]int, persist.SkillCount),
		MaxLevel:   make([]int, persist.SkillCount),
		Experience: make([]int, persist.SkillCount),
	}}
	for i := range save.Skills.Level {
		save.Skills.Level[i] = 1
		save.Skills.MaxLevel[i] = 1
	}
	save.Skills.Experience[0] = 13034431
	save.Skills.MaxLevel[0] = 1
	views, _, _ = bootstrapSkills(save)
	if views[0].BaseLevel != 99 || views[0].CurrentLevel != 1 || views[0].Boost != -98 {
		t.Errorf("attack row = %+v, want base 99 current 1 boost -98", views[0])
	}
	// Combat level extremes clamp to 3..126.
	maxed := make([]int, persist.SkillCount)
	for i := range maxed {
		maxed[i] = 99
	}
	if got := combatLevel(maxed); got != 126 {
		t.Errorf("combatLevel(maxed) = %d, want 126", got)
	}
	minimal := make([]int, persist.SkillCount)
	for i := range minimal {
		minimal[i] = 1
	}
	minimal[persist.HitpointsIndex] = 10
	if got := combatLevel(minimal); got != 3 {
		t.Errorf("combatLevel(fresh) = %d, want 3", got)
	}
}

func TestQuickPrayerMask(t *testing.T) {
	if got := quickPrayerMask(nil); got != 0 {
		t.Errorf("quickPrayerMask(nil) = %d, want 0", got)
	}
	save := &persist.PlayerSave{QuickPrayers: make([]*persist.QuickPrayer, 30)}
	save.QuickPrayers[0] = &persist.QuickPrayer{}
	save.QuickPrayers[28] = &persist.QuickPrayer{} // slot 27
	save.QuickPrayers[29] = &persist.QuickPrayer{} // no slot (Zeal): ignored
	if got := quickPrayerMask(save); got != 1+(1<<27) {
		t.Errorf("quickPrayerMask = %d, want %d", got, 1+(1<<27))
	}
}

func TestResolveGameframeRoot(t *testing.T) {
	attrs := func(pairs ...string) map[string]json.RawMessage {
		m := map[string]json.RawMessage{}
		for i := 0; i < len(pairs); i += 2 {
			m[pairs[i]] = json.RawMessage(pairs[i+1])
		}
		return m
	}
	cases := []struct {
		name       string
		clientType byte
		attrs      map[string]json.RawMessage
		want       int
	}{
		{"desktop default", 0, nil, 161},
		{"mobile wins", 1, attrs("client-layout-root", "548"), 601},
		{"saved 548", 0, attrs("client-layout-root", "548"), 548},
		{"saved 164", 0, attrs("client-layout-root", "164"), 164},
		{"saved 161", 0, attrs("client-layout-root", "161"), 161},
		{"unknown root falls back", 0, attrs("client-layout-root", "999"), 161},
		{"string root parses", 0, attrs("client-layout-root", `"548"`), 548},
		{"non-numeric falls back", 0, attrs("client-layout-root", `"abc"`), 161},
		{"clientType 10 is not mobile", 10, nil, 161},
	}
	for _, tc := range cases {
		if got := resolveGameframeRoot(tc.clientType, tc.attrs); got != tc.want {
			t.Errorf("%s: root = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestBootstrapAdmin(t *testing.T) {
	admin := &persist.PlayerSave{Rights: persist.Rights{ID: 2}}
	if !bootstrapAdmin(admin, "Alice", nil) {
		t.Error("rights id 2 is not admin")
	}
	mod := &persist.PlayerSave{Rights: persist.Rights{ID: 1}}
	if bootstrapAdmin(mod, "Alice", nil) {
		t.Error("rights id 1 (moderator) is admin")
	}
	if bootstrapAdmin(nil, "Alice", nil) {
		t.Error("no save is admin without a dev username")
	}
	if !bootstrapAdmin(nil, "Alice", []string{"alice"}) {
		t.Error("dev username match (case-insensitive) is not admin")
	}
	if bootstrapAdmin(nil, "Alice", []string{"", "  ", "bob"}) {
		t.Error("empty dev entries match")
	}
	// A dev username grants admin even when the save has no rights.
	if !bootstrapAdmin(mod, "Alice", []string{"ALICE"}) {
		t.Error("dev username does not override save rights")
	}
}

func TestRunEnergyState(t *testing.T) {
	units, running := runEnergyState(nil)
	if units != 10000 || !running {
		t.Errorf("fresh run energy = (%d, %v), want (10000, true)", units, running)
	}
	units, running = runEnergyState(&persist.PlayerSave{RunEnergy: 99.99, Running: true})
	if units != 9999 || !running {
		t.Errorf("99.99%% run energy = (%d, %v), want (9999, true)", units, running)
	}
	units, _ = runEnergyState(&persist.PlayerSave{RunEnergy: 150})
	if units != 10000 {
		t.Errorf("150%% run energy = %d, want clamped 10000", units)
	}
	units, _ = runEnergyState(&persist.PlayerSave{RunEnergy: -5})
	if units != 0 {
		t.Errorf("-5%% run energy = %d, want clamped 0", units)
	}
}
