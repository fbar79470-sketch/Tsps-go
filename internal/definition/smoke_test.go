package definition

import (
	"testing"
)

func TestFakeCacheSmoke(t *testing.T) {
	defs, gamevals := newFakeCache(t, fakeCacheSpec{
		npcs: map[int][]byte{
			0: npcEntry("Goblin", npcAction(1, "Attack"), npcCombatLevel(2), npcSpawnDirection(3)),
			1: npcEntry("Man", npcIdleSeq(10)),
		},
		objs: map[int][]byte{
			0: objEntry("null"),
			1: objEntry("Coins", objStackable(), objPrice(1), objTradable()),
			2: objEntry("Bronze sword", objPrice(10), objWearPos(3), objWeight(2), objTradable(), objParams([2]int{0, 5}, [2]int{14, 4})),
		},
		locs: map[int][]byte{
			0: locEntry("Tree", locSize(2, 2), locAction(0, "Chop down")),
		},
		seqNames: map[int]string{10: "human_ready", 11: "human_unarmedblock", 12: "goblin_death"},
	})
	counts := defs.GetCounts()
	if counts.NPCs != 2 || counts.Items != 3 || counts.Objects != 1 {
		t.Fatalf("counts = %+v", counts)
	}
	if got := defs.GetNpc(0).Name; got != "Goblin" {
		t.Fatalf("npc 0 name = %q", got)
	}
	if got := defs.GetNpc(0).SpawnDirection; got != 3 {
		t.Fatalf("spawn direction = %d", got)
	}
	if got := defs.GetItem(2).Price; got != 10 {
		t.Fatalf("item price = %d", got)
	}
	if !defs.HasItem(1) || defs.HasItem(0) {
		t.Fatalf("HasItem wrong")
	}
	names, err := gamevals.NamesOf(7)
	if err != nil {
		t.Fatal(err)
	}
	if names[12] != "goblin_death" {
		t.Fatalf("seq names = %v", names)
	}
	seqNames, err := defs.GetSeqNames()
	if err != nil {
		t.Fatal(err)
	}
	if seqNames[11] != "human_unarmedblock" {
		t.Fatalf("cache seq names = %v", seqNames)
	}
}
