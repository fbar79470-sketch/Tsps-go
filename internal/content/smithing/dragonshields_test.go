package smithing

import (
	"encoding/json"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// answerQuest installs a "quest:is-complete" answerer on the
// test emit, so the dragon forge can be driven past its gate.
func answerQuest(s *smithing, emitted *[]emittedEvent, complete bool) {
	s.emit = func(name string, payload any) {
		if req, ok := payload.(*questStateRequest); ok && name == "quest:is-complete" {
			req.Complete = complete
		}
		*emitted = append(*emitted, emittedEvent{name: name, payload: payload})
	}
}

// ---- dragon sq shield ----

func TestRepairSqShieldTimeline(t *testing.T) {
	s, emitted := newTestSmithing(t)
	p, _ := newTestPlayer("sq")
	give(p, dragonShieldLeftHalfID, 1)
	give(p, dragonShieldRightHalfID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 60)
	source := &anvilSource{objectID: 1234, location: p.Location()}
	s.repairSqShield(p, source)

	// The start box; continuing it starts the timeline.
	if text, ok := statementText(p); !ok || text != dragonSqShieldStartMessage {
		t.Fatalf("start box = %q, want the repair line", text)
	}
	p.GetDialogueManager().Advance()

	// Tick 7: the done box; continuing it gives the shield.
	driveTicks(s, 7)
	if text, ok := statementText(p); !ok || text != dragonSqShieldDoneMessage {
		t.Fatalf("after tick 7: box = %q, want the done line", text)
	}
	p.GetDialogueManager().Advance()
	if got := p.Inventory().GetAmount(dragonSqShieldID); got != 1 {
		t.Errorf("sq shields = %d, want 1", got)
	}
	if p.Inventory().Contains(dragonShieldLeftHalfID) || p.Inventory().Contains(dragonShieldRightHalfID) {
		t.Error("halves left after the repair")
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 75 {
		t.Errorf("smithing XP = %d, want 75", got)
	}
	// The success emit carries the anvil provenance.
	found := false
	for _, e := range *emitted {
		if e.name != "smithing:success" {
			continue
		}
		if ev, ok := e.payload.(*smithingSuccessEvent); ok && ev.StationObjectID == 1234 && ev.Source == "anvil" {
			found = true
		}
	}
	if !found {
		t.Error("no smithing:success with the anvil source")
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after the repair")
	}
}

func TestRepairSqShieldGates(t *testing.T) {
	// Missing the other half.
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("sq")
	give(p, dragonShieldLeftHalfID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 60)
	s.repairSqShield(p, nil)
	if text, ok := statementText(p); !ok || text != dragonSqShieldOtherHalfMessage {
		t.Errorf("box = %q, want the other-half line", text)
	}

	// No hammer.
	p, _ = newTestPlayer("sq")
	give(p, dragonShieldLeftHalfID, 1)
	give(p, dragonShieldRightHalfID, 1)
	setLevel(p, game.SkillSmithing, 60)
	s.repairSqShield(p, nil)
	if text, ok := statementText(p); !ok || text != dragonSqShieldNoHammerMessage {
		t.Errorf("box = %q, want the hammer line", text)
	}

	// Level 59.
	p, _ = newTestPlayer("sq")
	give(p, dragonShieldLeftHalfID, 1)
	give(p, dragonShieldRightHalfID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 59)
	s.repairSqShield(p, nil)
	if text, ok := statementText(p); !ok || text != "You need a Smithing level of 60 to repair this shield." {
		t.Errorf("box = %q, want the level-60 line", text)
	}
}

func TestHalvesByHand(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("halves")
	ev := &plugins.PluginItemOnItemEvent{Player: p, UsedItemID: dragonShieldLeftHalfID, UsedWithItemID: dragonShieldRightHalfID}
	s.halvesByHand(ev)
	if !ev.Handled {
		t.Error("halves by hand not claimed")
	}
	if !hasMessage(sess, "repair the shield at an anvil") {
		t.Errorf("messages = %v, want the anvil line", sess.messages)
	}

	// A half on anything else falls through.
	ev = &plugins.PluginItemOnItemEvent{Player: p, UsedItemID: dragonShieldLeftHalfID, UsedWithItemID: fxIronBar}
	s.halvesByHand(ev)
	if ev.Handled {
		t.Error("half+bar claimed, want no join")
	}
}

// ---- visage shields ----

func TestAttachVisageTimeline(t *testing.T) {
	for _, tc := range []struct {
		name   string
		visage int
		result int
	}{
		{"draconic visage", draconicVisageID, dragonfireShieldUnchID},
		{"skeletal visage", skeletalVisageID, dragonfireWardUnchID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newTestSmithing(t)
			p, _ := newTestPlayer("visage")
			give(p, tc.visage, 1)
			give(p, antiDragonShieldID, 1)
			give(p, fxHammer, 1)
			setLevel(p, game.SkillSmithing, 90)
			shield := visageShieldFor(p)
			if shield == nil || shield.result != tc.result {
				t.Fatalf("visageShieldFor = %+v, want result %d", shield, tc.result)
			}
			s.attachVisage(p, shield)

			// The item box; continuing it starts the timeline.
			if text, ok := dialogueText(p); !ok || text == "" {
				t.Fatal("no visage start box")
			}
			p.GetDialogueManager().Advance()

			// Tick 9: the shield and the done box, 2,000 XP.
			driveTicks(s, 9)
			if got := p.Inventory().GetAmount(tc.result); got != 1 {
				t.Errorf("shields = %d, want 1", got)
			}
			if p.Inventory().Contains(tc.visage) || p.Inventory().Contains(antiDragonShieldID) {
				t.Error("visage or anti-dragon shield left after the attach")
			}
			if got := p.Skills().Experience(game.SkillSmithing); got != 2000 {
				t.Errorf("smithing XP = %d, want 2000", got)
			}
			if text, ok := dialogueText(p); !ok || text != shield.done {
				t.Errorf("after tick 9: box = %q, want the done line", text)
			}
		})
	}
}

func TestAttachVisageGates(t *testing.T) {
	// No hammer.
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("visage")
	give(p, draconicVisageID, 1)
	give(p, antiDragonShieldID, 1)
	setLevel(p, game.SkillSmithing, 90)
	s.attachVisage(p, visageShieldFor(p))
	if text, ok := statementText(p); !ok || text != "You'll need a hammer if you wish to construct a dragonfire shield." {
		t.Errorf("box = %q, want the hammer line", text)
	}

	// Level 89.
	p, _ = newTestPlayer("visage")
	give(p, draconicVisageID, 1)
	give(p, antiDragonShieldID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 89)
	s.attachVisage(p, visageShieldFor(p))
	if text, ok := statementText(p); !ok || text != "You need a Smithing level of at least 90 to construct a dragonfire shield." {
		t.Errorf("box = %q, want the level-90 line", text)
	}
}

func TestVisageShieldPresetOnly(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("visage")
	p.Inventory().Add(game.NewItemWithMeta(draconicVisageID, 1, map[string]json.RawMessage{
		game.ItemPresetMeta: json.RawMessage("true"),
	}), true)
	give(p, antiDragonShieldID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 90)
	ev := &plugins.ItemOnObjectEvent{Player: p, ItemID: draconicVisageID, ObjectID: 1}
	if !s.tryDragonShieldOnAnvil(ev) {
		t.Fatal("visage on anvil not claimed")
	}
	if text, ok := statementText(p); !ok || text != presetItemMessage {
		t.Errorf("box = %q, want the preset line", text)
	}
	if hasMessage(sess, "dragonfire shield") {
		t.Errorf("messages = %v, want no forge", sess.messages)
	}
}

// ---- dragon forge ----

func TestDragonForgeQuestGateFailClosed(t *testing.T) {
	s, emitted := newTestSmithing(t)
	p, sess := newTestPlayer("forge")
	give(p, dragonSqShieldID, 1)
	give(p, dragonMetalSliceID, 1)
	give(p, dragonMetalShardID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 75)
	ev := &plugins.ItemOnObjectEvent{Player: p, ItemID: dragonMetalSliceID, ObjectID: DragonForgeLocID}
	if !s.forgeAtDragonForge(ev) {
		t.Fatal("forge not claimed for a dragon metal slice")
	}
	if !ev.Handled {
		t.Error("forge use not marked handled")
	}
	if !hasMessage(sess, "to use the dragon forge.") {
		t.Errorf("messages = %v, want the Dragon Slayer II line", sess.messages)
	}
	if !hasEvent(emitted, "quest:is-complete") {
		t.Error("no quest:is-complete emit")
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("forge session started behind the locked gate")
	}
	if p.Inventory().Contains(dragonKiteshieldID) {
		t.Error("kiteshield forged behind the locked gate")
	}
}

func TestDragonForgeKiteshield(t *testing.T) {
	s, emitted := newTestSmithing(t)
	answerQuest(s, emitted, true)
	p, _ := newTestPlayer("forge")
	give(p, dragonSqShieldID, 1)
	give(p, dragonMetalSliceID, 1)
	give(p, dragonMetalShardID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 75)
	ev := &plugins.ItemOnObjectEvent{Player: p, ItemID: dragonMetalSliceID, ObjectID: DragonForgeLocID}
	if !s.forgeAtDragonForge(ev) {
		t.Fatal("forge not claimed")
	}
	// The 2-tick recipe gives the shield on the 3rd tick.
	driveTicks(s, 3)
	if got := p.Inventory().GetAmount(dragonKiteshieldID); got != 1 {
		t.Errorf("kiteshields = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 1000 {
		t.Errorf("smithing XP = %d, want 1000", got)
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after the forge")
	}
}

func TestDragonForgePlatebody(t *testing.T) {
	s, emitted := newTestSmithing(t)
	answerQuest(s, emitted, true)
	p, _ := newTestPlayer("forge")
	give(p, dragonChainbodyID, 1)
	give(p, dragonMetalLumpID, 1)
	give(p, dragonMetalShardID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 90)
	// The shard is in both recipes; the platebody is the only
	// one fully held, so it wins.
	ev := &plugins.ItemOnObjectEvent{Player: p, ItemID: dragonMetalShardID, ObjectID: DragonForgeLocID}
	if !s.forgeAtDragonForge(ev) {
		t.Fatal("forge not claimed")
	}
	driveTicks(s, 3)
	if got := p.Inventory().GetAmount(dragonPlatebodyID); got != 1 {
		t.Errorf("platebodies = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 2000 {
		t.Errorf("smithing XP = %d, want 2000", got)
	}
}

func TestDragonForgeMissingParts(t *testing.T) {
	s, emitted := newTestSmithing(t)
	answerQuest(s, emitted, true)
	p, _ := newTestPlayer("forge")
	give(p, dragonSqShieldID, 1)
	give(p, dragonMetalShardID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 75)
	ev := &plugins.ItemOnObjectEvent{Player: p, ItemID: dragonMetalShardID, ObjectID: DragonForgeLocID}
	if !s.forgeAtDragonForge(ev) {
		t.Fatal("forge not claimed")
	}
	if text, ok := statementText(p); !ok || text != "You need a dragon metal shard, a dragon metal slice and a dragon sq shield." {
		t.Errorf("box = %q, want the missing-parts line", text)
	}
}

// ---- wyvern shield ----

func TestWyvernShieldTimeline(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("wyvern")
	give(p, wyvernVisageID, 1)
	give(p, elementalShieldID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 66)
	setLevel(p, game.SkillMagic, 66)
	ev := &plugins.ItemOnObjectEvent{Player: p, ItemID: wyvernVisageID, ObjectID: ShieldMachineLocID}
	if !s.useOnShieldMachine(ev) {
		t.Fatal("shield machine use not claimed")
	}
	if !ev.Handled {
		t.Error("shield machine use not marked handled")
	}
	// Tick 4: the shield, 2,000 XP in each skill, and Magic
	// drained to 0.
	driveTicks(s, 4)
	if got := p.Inventory().GetAmount(wyvernShieldUnchID); got != 1 {
		t.Errorf("wyvern shields = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 2000 {
		t.Errorf("smithing XP = %d, want 2000", got)
	}
	if got := p.Skills().Experience(game.SkillMagic); got != 2000 {
		t.Errorf("magic XP = %d, want 2000", got)
	}
	if got := p.Skills().CurrentLevel(game.SkillMagic); got != 0 {
		t.Errorf("magic level = %d, want 0 (drained)", got)
	}
	if !hasMessage(sess, "you have crafted the wyvern visage") {
		t.Errorf("messages = %v, want the done line", sess.messages)
	}
}

func TestWyvernShieldGates(t *testing.T) {
	// Missing the elemental shield.
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("wyvern")
	give(p, wyvernVisageID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 66)
	setLevel(p, game.SkillMagic, 66)
	ev := &plugins.ItemOnObjectEvent{Player: p, ItemID: wyvernVisageID, ObjectID: ShieldMachineLocID}
	s.useOnShieldMachine(ev)
	if text, ok := statementText(p); !ok || text != wyvernShieldNeedPartsMessage {
		t.Errorf("box = %q, want the need-parts line", text)
	}

	// Magic 65 (Smithing fine).
	p, _ = newTestPlayer("wyvern")
	give(p, wyvernVisageID, 1)
	give(p, elementalShieldID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 66)
	setLevel(p, game.SkillMagic, 65)
	s.useOnShieldMachine(&plugins.ItemOnObjectEvent{Player: p, ItemID: wyvernVisageID, ObjectID: ShieldMachineLocID})
	if text, ok := statementText(p); !ok || text != wyvernShieldLevelMessage {
		t.Errorf("box = %q, want the level-66 line", text)
	}

	// A non-wyvern item on the machine falls through.
	ev = &plugins.ItemOnObjectEvent{Player: p, ItemID: fxIronBar, ObjectID: ShieldMachineLocID}
	if s.useOnShieldMachine(ev) {
		t.Error("iron bar on the shield machine claimed")
	}
}

// ---- entry points ----

func TestTryDragonShieldClaim(t *testing.T) {
	_, _ = newTestSmithing(t)
	p, _ := newTestPlayer("claim")
	give(p, draconicVisageID, 1)
	give(p, antiDragonShieldID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 90)
	t.Cleanup(func() { shared.stopSession(p, false) })
	if !TryDragonShield(p) {
		t.Error("TryDragonShield = false with a visage held")
	}

	p2, _ := newTestPlayer("claim2")
	if TryDragonShield(p2) {
		t.Error("TryDragonShield = true with no visage held")
	}
}
