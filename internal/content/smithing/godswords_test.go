package smithing

import (
	"encoding/json"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// dialogueText reads the current statement or item-statement
// text, for the forge / dismantle boxes.
func dialogueText(p *game.Player) (string, bool) {
	dm := p.GetDialogueManager()
	if dm == nil || !dm.IsActive() {
		return "", false
	}
	type texter interface{ GetText() string }
	if t, ok := dm.GetCurrent().(texter); ok {
		return t.GetText(), true
	}
	return "", false
}

// ---- combinations ----

func TestGodswordCombinationFor(t *testing.T) {
	cases := []struct {
		name  string
		held  []int
		into  int
		found bool
	}{
		{"shards 1+2", []int{godswordShard1ID, godswordShard2ID}, godswordShards12ID, true},
		{"shards 1+3", []int{godswordShard1ID, godswordShard3ID}, godswordShards13ID, true},
		{"shards 2+3", []int{godswordShard2ID, godswordShard3ID}, godswordShards23ID, true},
		{"pair 1&2 + shard 3", []int{godswordShards12ID, godswordShard3ID}, godswordBladeID, true},
		{"pair 1&3 + shard 2", []int{godswordShards13ID, godswordShard2ID}, godswordBladeID, true},
		{"pair 2&3 + shard 1", []int{godswordShards23ID, godswordShard1ID}, godswordBladeID, true},
		{"duplicate shard", []int{godswordShard1ID, godswordShard1ID}, 0, false},
		{"overlapping pair", []int{godswordShards12ID, godswordShard2ID}, 0, false},
		{"single shard", []int{godswordShard1ID}, 0, false},
		{"blade alone", []int{godswordBladeID}, 0, false},
		{"three shards pick a pair", []int{godswordShard1ID, godswordShard2ID, godswordShard3ID}, godswordShards12ID, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _ = newTestSmithing(t)
			p, _ := newTestPlayer("combo")
			for _, id := range tc.held {
				give(p, id, 1)
			}
			combo := godswordCombinationFor(p)
			if !tc.found {
				if combo != nil {
					t.Errorf("combination = %+v, want none", combo)
				}
				return
			}
			if combo == nil {
				t.Fatal("combination = nil, want one")
			}
			if combo.into != tc.into {
				t.Errorf("into = %d, want %d", combo.into, tc.into)
			}
		})
	}
}

// ---- join by hand ----

func TestJoinGodswordByHand(t *testing.T) {
	// Every hilt + blade makes its godsword, no skill needed.
	for hilt, sword := range godswordHilts() {
		t.Run("hilt", func(t *testing.T) {
			s, _ := newTestSmithing(t)
			p, _ := newTestPlayer("join")
			give(p, godswordBladeID, 1)
			give(p, hilt, 1)
			s.joinGodswordByHand(&plugins.PluginItemOnItemEvent{Player: p, UsedItemID: godswordBladeID, UsedWithItemID: hilt})
			if !p.Inventory().Contains(sword) {
				t.Errorf("godsword %d not made from blade + hilt %d", sword, hilt)
			}
			if p.Inventory().Contains(godswordBladeID) || p.Inventory().Contains(hilt) {
				t.Error("blade or hilt left after the join")
			}
		})
	}

	// Two pieces cannot join by hand: the message, claimed.
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("join")
	give(p, godswordShard1ID, 1)
	give(p, godswordShard2ID, 1)
	ev := &plugins.PluginItemOnItemEvent{Player: p, UsedItemID: godswordShard1ID, UsedWithItemID: godswordShard2ID}
	s.joinGodswordByHand(ev)
	if !ev.Handled {
		t.Error("piece+piece not claimed")
	}
	if !hasMessage(sess, "can't be joined together like that") {
		t.Errorf("messages = %v, want the by-hand line", sess.messages)
	}
	if p.Inventory().Contains(godswordShards12ID) {
		t.Error("pieces joined by hand, want the anvil message only")
	}

	// A hilt on a shard is not a join.
	ev = &plugins.PluginItemOnItemEvent{Player: p, UsedItemID: armadylHiltID, UsedWithItemID: godswordShard1ID}
	s.joinGodswordByHand(ev)
	if ev.Handled {
		t.Error("hilt+shard claimed, want no join")
	}
}

// ---- forge gates ----

func TestForgeGodswordGates(t *testing.T) {
	// No fitting pair: the need-two box.
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("gates")
	give(p, godswordShard1ID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 80)
	s.forgeGodsword(p)
	if text, ok := statementText(p); !ok || text != godswordNeedTwoMessage {
		t.Errorf("box = %q, want the need-two line", text)
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session started with no pair")
	}

	// No hammer.
	p, _ = newTestPlayer("gates")
	give(p, godswordShard1ID, 1)
	give(p, godswordShard2ID, 1)
	setLevel(p, game.SkillSmithing, 80)
	s.forgeGodsword(p)
	if text, ok := statementText(p); !ok || text != godswordNoHammerMessage {
		t.Errorf("box = %q, want the hammer line", text)
	}

	// Level 79.
	p, _ = newTestPlayer("gates")
	give(p, godswordShard1ID, 1)
	give(p, godswordShard2ID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 79)
	s.forgeGodsword(p)
	if text, ok := statementText(p); !ok || text != "You need a Smithing level of 80 to repair the Godsword." {
		t.Errorf("box = %q, want the level-80 line", text)
	}
}

// ---- forge timeline ----

func TestForgeGodswordTimeline(t *testing.T) {
	s, emitted := newTestSmithing(t)
	p, _ := newTestPlayer("forge")
	give(p, godswordShard1ID, 1)
	give(p, godswordShard2ID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 80)
	s.forgeGodsword(p)

	// Tick 1: the start box; pieces still held.
	driveTicks(s, 1)
	if text, ok := statementText(p); !ok || text != godswordStartMessage {
		t.Errorf("after tick 1: box = %q, want the start line", text)
	}
	if !p.Inventory().Contains(godswordShard1ID) {
		t.Error("after tick 1: shard gone too early")
	}

	// Tick 7: the pair is the result, 100 XP, the done box,
	// the smithing:success emit, and the session ends.
	driveTicks(s, 6)
	if p.Inventory().Contains(godswordShard1ID) || p.Inventory().Contains(godswordShard2ID) {
		t.Error("after tick 7: shards left")
	}
	if got := p.Inventory().GetAmount(godswordShards12ID); got != 1 {
		t.Errorf("pairs = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 100 {
		t.Errorf("smithing XP = %d, want 100", got)
	}
	if text, ok := dialogueText(p); !ok || text != godswordDoneMessage {
		t.Errorf("after tick 7: box = %q, want the done line", text)
	}
	if !hasEvent(emitted, "smithing:success") {
		t.Error("no smithing:success emit")
	}
	if _, ok := s.sessions[p]; ok {
		t.Error("session still active after the forge")
	}
}

func TestForgeGodswordBlade(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("forge")
	give(p, godswordShards12ID, 1)
	give(p, godswordShard3ID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 80)
	s.forgeGodsword(p)
	driveTicks(s, 7)
	if got := p.Inventory().GetAmount(godswordBladeID); got != 1 {
		t.Errorf("blades = %d, want 1", got)
	}
}

// ---- entry points ----

func TestTryGodswordClaim(t *testing.T) {
	_, _ = newTestSmithing(t)
	p, _ := newTestPlayer("claim")
	give(p, godswordShard1ID, 1)
	give(p, godswordShard2ID, 1)
	give(p, fxHammer, 1)
	setLevel(p, game.SkillSmithing, 80)
	t.Cleanup(func() { shared.stopSession(p, false) })
	if !TryGodsword(p) {
		t.Error("TryGodsword = false with two pieces held")
	}
	if _, ok := shared.sessions[p]; !ok {
		t.Error("no godsword session on the shared instance")
	}

	p2, _ := newTestPlayer("claim2")
	if TryGodsword(p2) {
		t.Error("TryGodsword = true with no pieces held")
	}

	// A piece used on an anvil claims; a bar does not.
	p3, _ := newTestPlayer("claim3")
	give(p3, godswordShard1ID, 1)
	give(p3, godswordShard2ID, 1)
	give(p3, fxHammer, 1)
	setLevel(p3, game.SkillSmithing, 80)
	t.Cleanup(func() { shared.stopSession(p3, false) })
	ev := &plugins.ItemOnObjectEvent{Player: p3, ItemID: godswordShard1ID, ObjectID: 1}
	if !TryGodswordOnAnvil(ev) {
		t.Error("TryGodswordOnAnvil = false for a shard")
	}
	if !ev.Handled {
		t.Error("shard on anvil not marked handled")
	}
	ev = &plugins.ItemOnObjectEvent{Player: p3, ItemID: fxIronBar, ObjectID: 1}
	if TryGodswordOnAnvil(ev) {
		t.Error("TryGodswordOnAnvil = true for a bar")
	}
}

// ---- dismantle ----

func dismantleEvent(p *game.Player, swordID int) *plugins.PluginItemActionEvent {
	return &plugins.PluginItemActionEvent{Player: p, ItemID: swordID, Option: "Dismantle"}
}

func TestDismantleGodswordInventory(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("dismantle")
	give(p, armadylGodswordID, 1)
	if !s.dismantleGodsword(dismantleEvent(p, armadylGodswordID)) {
		t.Fatal("dismantle not claimed")
	}
	// The prompt, then Yes splits it: the blade in place, the
	// hilt added.
	dm := p.GetDialogueManager()
	if !dm.IsActive() {
		t.Fatal("no dismantle prompt")
	}
	if !dm.HandleResume(game.OptionsWidgetID, 1) {
		t.Fatal("HandleResume(Yes) failed")
	}
	if p.Inventory().Contains(armadylGodswordID) {
		t.Error("godsword left after Yes")
	}
	if got := p.Inventory().GetAmount(godswordBladeID); got != 1 {
		t.Errorf("blades = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(armadylHiltID); got != 1 {
		t.Errorf("hilts = %d, want 1", got)
	}
	if !hasMessage(sess, "You dismantle the godsword.") {
		t.Errorf("messages = %v, want the dismantled line", sess.messages)
	}
}

func TestDismantleGodswordNo(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("dismantle")
	give(p, bandosGodswordID, 1)
	s.dismantleGodsword(dismantleEvent(p, bandosGodswordID))
	if !p.GetDialogueManager().HandleResume(game.OptionsWidgetID, 2) {
		t.Fatal("HandleResume(No) failed")
	}
	if !p.Inventory().Contains(bandosGodswordID) {
		t.Error("godsword gone after No")
	}
	if p.Inventory().Contains(godswordBladeID) || p.Inventory().Contains(bandosHiltID) {
		t.Error("parts appeared after No")
	}
}

func TestDismantleGodswordWielded(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("dismantle")
	wear(p, game.WeaponSlot, saradominGodswordID)
	s.dismantleGodsword(dismantleEvent(p, saradominGodswordID))
	if !p.GetDialogueManager().HandleResume(game.OptionsWidgetID, 1) {
		t.Fatal("HandleResume(Yes) failed")
	}
	if weapon := p.Equipment().ItemAt(game.WeaponSlot); weapon.IsValid() {
		t.Errorf("weapon slot still holds %d after dismantle", weapon.ID())
	}
	if got := p.Inventory().GetAmount(godswordBladeID); got != 1 {
		t.Errorf("blades = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(saradominHiltID); got != 1 {
		t.Errorf("hilts = %d, want 1", got)
	}
}

func TestDismantleGodswordNoSpace(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("dismantle")
	give(p, zamorakGodswordID, 1)
	for i := 0; i < 27; i++ {
		give(p, fxLogs, 1)
	}
	if got := p.Inventory().GetFreeSlots(); got != 0 {
		t.Fatalf("free slots = %d, want 0", got)
	}
	s.dismantleGodsword(dismantleEvent(p, zamorakGodswordID))
	if !p.GetDialogueManager().HandleResume(game.OptionsWidgetID, 1) {
		t.Fatal("HandleResume(Yes) failed")
	}
	if !p.Inventory().Contains(zamorakGodswordID) {
		t.Error("godsword split with no free slot")
	}
	if !hasMessage(sess, "You don't have enough inventory space to do that.") {
		t.Errorf("messages = %v, want the no-space line", sess.messages)
	}
}

func TestDismantleGodswordRestricted(t *testing.T) {
	s, _ := newTestSmithing(t)
	p, sess := newTestPlayer("dismantle")
	p.Inventory().Add(game.NewItemWithMeta(ancientGodswordID, 1, map[string]json.RawMessage{
		game.ItemPresetMeta: json.RawMessage("true"),
	}), true)
	if !s.dismantleGodsword(dismantleEvent(p, ancientGodswordID)) {
		t.Fatal("dismantle not claimed for a preset copy")
	}
	if p.GetDialogueManager().IsActive() {
		t.Error("prompt shown for a preset copy, want the refusal at once")
	}
	if !hasMessage(sess, "Preset items cannot be used for this.") {
		t.Errorf("messages = %v, want the preset line", sess.messages)
	}
	if !p.Inventory().Contains(ancientGodswordID) {
		t.Error("preset godsword gone after the refusal")
	}
}
