package woodcutting

import "testing"

// TestGuildDataTables pins the transcribed ents + shrine tables
// (guilddata.go; woodcutting-guild.json:23-67): every value the
// units read, hand-checked against the JSON lines cited in the
// table comments. Cache resolution of every id is the temporary
// probe's job (zz_temp_ent_probe_test.go, deleted after use),
// not this test's.
func TestGuildDataTables(t *testing.T) {
	// Ents (JSON:23-48).
	if got := Ents.TrunkID; got != 9474 {
		t.Errorf("Ents.TrunkID = %d, want 9474", got)
	}
	if got := Ents.TransformTicks; got != 4 {
		t.Errorf("Ents.TransformTicks = %d, want 4", got)
	}
	if got := Ents.TrunkTicks; got != 101 {
		t.Errorf("Ents.TrunkTicks = %d, want 101", got)
	}
	if got := Ents.ChopTicks; got != 3 {
		t.Errorf("Ents.ChopTicks = %d, want 3", got)
	}
	if got := Ents.XP; got != 25 {
		t.Errorf("Ents.XP = %d, want 25", got)
	}
	guild, ok := Ents.NPCs[7234]
	if !ok || guild.Where != "Woodcutting Guild" || guild.LogsPerSuccess != 1 || guild.RespawnTicks != 50 {
		t.Errorf("Ents.NPCs[7234] = %+v, want guild/1/50", guild)
	}
	wild, ok := Ents.NPCs[6594]
	if !ok || wild.Where != "Wilderness" || wild.LogsPerSuccess != 2 || wild.RespawnTicks != 15 {
		t.Errorf("Ents.NPCs[6594] = %+v, want wilderness/2/15", wild)
	}
	if len(Ents.NPCs) != 2 {
		t.Errorf("len(Ents.NPCs) = %d, want 2", len(Ents.NPCs))
	}
	// Cut chance rows (JSON:33).
	wantRows := [8][2]int{{12, 37}, {18, 56}, {24, 75}, {27, 84}, {30, 94}, {36, 112}, {42, 131}, {45, 140}}
	for tier, want := range wantRows {
		if row := Ents.CutChance[tier]; !row.Has || row.Low != want[0] || row.High != want[1] {
			t.Errorf("Ents.CutChance[%d] = %+v, want [%d %d]", tier, row, want[0], want[1])
		}
	}
	// Logs (JSON:35-40).
	wantLogs := []EntLog{
		{ID: 1511, Name: "Logs", MinLevel: 1, MaxLevel: 44, Weight: 4},
		{ID: 1521, Name: "Oak logs", MinLevel: 15, MaxLevel: 99, Weight: 4},
		{ID: 1519, Name: "Willow logs", MinLevel: 30, MaxLevel: 99, Weight: 1},
		{ID: 1517, Name: "Maple logs", MinLevel: 45, MaxLevel: 99, Weight: 1},
		{ID: 1515, Name: "Yew logs", MinLevel: 61, MaxLevel: 99, Weight: 1},
		{ID: 1513, Name: "Magic logs", MinLevel: 75, MaxLevel: 99, Weight: 1, WeightPerAxeTier: 1},
	}
	if len(Ents.Logs) != len(wantLogs) {
		t.Fatalf("len(Ents.Logs) = %d, want %d", len(Ents.Logs), len(wantLogs))
	}
	for i, want := range wantLogs {
		if got := Ents.Logs[i]; got != want {
			t.Errorf("Ents.Logs[%d] = %+v, want %+v", i, got, want)
		}
	}
	if got := Ents.Messages.Swing; got != "You swing your axe at the Ent trunk." {
		t.Errorf("Ents.Messages.Swing = %q", got)
	}
	if got := Ents.Messages.NoAxe; got != "You don't have an axe which you can use." {
		t.Errorf("Ents.Messages.NoAxe = %q", got)
	}
	if got := Ents.Messages.Full; got != "Your inventory is too full to carry any more logs." {
		t.Errorf("Ents.Messages.Full = %q", got)
	}
	if got := Ents.Messages.Yield; got != "The ent carcass yields: {amount} x {log}" {
		t.Errorf("Ents.Messages.Yield = %q", got)
	}

	// Shrine (JSON:49-67).
	if got := Shrine.ID; got != 29088 {
		t.Errorf("Shrine.ID = %d, want 29088", got)
	}
	if got := Shrine.Anim; got != 3705 {
		t.Errorf("Shrine.Anim = %d, want 3705", got)
	}
	if got := Shrine.XP; got != 100 {
		t.Errorf("Shrine.XP = %d, want 100", got)
	}
	if got := Shrine.Nest; got != 22798 {
		t.Errorf("Shrine.Nest = %d, want 22798", got)
	}
	wantProj := ShrineProjectile{FromX: 1, FromY: 1, ToX: 2, ToY: 1, StartCycle: 0, EndCycle: 140, Angle: 50, StartHeight: 25, EndHeight: 112}
	if got := Shrine.Projectile; got != wantProj {
		t.Errorf("Shrine.Projectile = %+v, want %+v", got, wantProj)
	}
	wantEggs := map[int]ShrineEgg{
		5076: {Colour: "red", Projectile: 1308, Sound: 3047},
		5077: {Colour: "blue", Projectile: 1307, Sound: 1984},
		5078: {Colour: "green", Projectile: 1309, Sound: 1986},
	}
	for id, want := range wantEggs {
		if got, ok := Shrine.Eggs[id]; !ok || got != want {
			t.Errorf("Shrine.Eggs[%d] = %+v, want %+v", id, got, want)
		}
	}
	if got := Shrine.EvilChickenChance; got != 1200 {
		t.Errorf("Shrine.EvilChickenChance = %d, want 1200", got)
	}
	wantPieces := []int{20433, 20436, 20439, 20442}
	for i, want := range wantPieces {
		if got := Shrine.EvilChickenPieces[i]; got != want {
			t.Errorf("Shrine.EvilChickenPieces[%d] = %d, want %d", i, got, want)
		}
	}
	if got := Shrine.Messages.Offer; got != "You offer your bird's egg to the shrine and receive a reward." {
		t.Errorf("Shrine.Messages.Offer = %q", got)
	}
	if got := Shrine.Messages.CountOne; got != "You have made <col=ff0000>one</col> offering." {
		t.Errorf("Shrine.Messages.CountOne = %q", got)
	}
	if got := Shrine.Messages.Count; got != "You have made <col=ff0000>{count}</col> offerings." {
		t.Errorf("Shrine.Messages.Count = %q", got)
	}
	if ShrineOfferingsAttribute != "woodcutting-guild:shrine-offerings" {
		t.Errorf("ShrineOfferingsAttribute = %q", ShrineOfferingsAttribute)
	}
}
