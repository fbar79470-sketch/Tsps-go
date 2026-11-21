package woodcutting

import (
	"io"
	"log"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// InfernalAxe tests: charge defaults/clamp, the 1/3 burn
// roll, the burn's Firemaking grant + presentation, the
// empty → uncharged transition, and the Check messages
// (InfernalAxe.Woodcutting.js). Fixtures only; ids are the
// ItemIdentifiers.ts values verified against the real rev-242
// seed cache (probe, 2026-11-20).
//
// The burn's central case — no Firemaking answerer — is
// pinned explicitly: with the log-xp query unanswered the
// burn never fires, even on a winning roll with a charged
// axe held.

// chargedInfernal returns a charged infernal axe item with
// the given saved charges, always writing the meta (even 0
// or negative, so clamp cases are expressible).
func chargedInfernal(id, charges int) game.Item {
	item := game.NewItem(id, 1)
	item.SetMetaValue(infernalChargesMetaKey, chargesMetaValue(charges))
	return item
}

func TestInfernalCharges(t *testing.T) {
	cases := []struct {
		name string
		item game.Item
		want int
	}{
		{"saved 2500", chargedInfernal(13241, 2500), 2500},
		{"saved 1", chargedInfernal(13241, 1), 1},
		{"saved 0", chargedInfernal(13241, 0), 0},
		{"saved over max clamps", chargedInfernal(13241, 6000), 5000},
		{"saved negative clamps", chargedInfernal(13241, -5), 0},
		{"no meta is fully charged", game.NewItem(13241, 1), 5000},
		{"no meta uncharged is fully charged too", game.NewItem(13242, 1), 5000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := infernalCharges(c.item); got != c.want {
				t.Fatalf("infernalCharges = %d, want %d", got, c.want)
			}
		})
	}
}

// answerFiremaking installs an emit that answers the
// firemaking:log-xp query with xp and records every event
// name it sees. The multiplier query is left unanswered
// (defaults to 1) unless answerMultiplier is set.
func answerFiremaking(w *woodcutting, xp int, answerMultiplier float64) *[]string {
	var seen []string
	w.emit = func(name string, payload any) {
		seen = append(seen, name)
		switch req := payload.(type) {
		case *FiremakingLogXPRequest:
			req.XP = xp
			req.HasXP = true
		case *FiremakingXPMultiplierRequest:
			if answerMultiplier != 0 {
				req.Multiplier = answerMultiplier
			}
		}
	}
	return &seen
}

func TestTryBurnLogNoAnswererNeverBurns(t *testing.T) {
	w, emitted := newTestWoodcutting(t, loadFixtureTables(t))
	w.randInt = func(min, max int) int { return 0 } // winning roll
	p := newTestPlayer("alice", 3000, 3000)
	p.Equipment().SetItem(game.WeaponSlot, chargedInfernal(13241, 5000))
	before := p.Skills().Experience(game.SkillFiremaking)
	if w.tryBurnLog(p, 13241, 1511) {
		t.Fatal("the burn must never fire with no log-xp answerer")
	}
	// The query was still asked (the emit is wired, the
	// answer is what is missing), and nothing was granted.
	found := false
	for _, e := range *emitted {
		if e.name == firemakingLogXPEventName {
			found = true
		}
	}
	if !found {
		t.Fatal("the log-xp query must be emitted even when unanswered")
	}
	if got := p.Skills().Experience(game.SkillFiremaking); got != before {
		t.Fatalf("Firemaking XP = %d, want unchanged %d", got, before)
	}
	if p.Graphic() != nil {
		t.Fatalf("graphic = %+v, want none without a burn", p.Graphic())
	}
	if got := infernalCharges(p.Equipment().ItemAt(game.WeaponSlot)); got != 5000 {
		t.Fatalf("charges = %d, want 5000 (no charge spent)", got)
	}
}

func TestTryBurnLogRollMiss(t *testing.T) {
	w, emitted := newTestWoodcutting(t, loadFixtureTables(t))
	answerFiremaking(w, 40, 0)
	w.randInt = func(min, max int) int { return 1 } // losing roll
	p := newTestPlayer("alice", 3000, 3000)
	p.Equipment().SetItem(game.WeaponSlot, chargedInfernal(13241, 5000))
	if w.tryBurnLog(p, 13241, 1511) {
		t.Fatal("a losing roll must not burn")
	}
	// The roll misses before the query: no event is emitted.
	if len(*emitted) != 0 {
		t.Fatalf("emitted = %v, want no events on a losing roll", *emitted)
	}
}

func TestTryBurnLogWithAnswerer(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	answerFiremaking(w, 40, 0)
	w.randInt = func(min, max int) int { return 0 } // winning roll
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Equipment().SetItem(game.WeaponSlot, chargedInfernal(13241, 5000))
	before := p.Skills().Experience(game.SkillFiremaking)
	if !w.tryBurnLog(p, 13241, 1511) {
		t.Fatal("the burn must fire with an answering log-xp query")
	}
	// Half the log's Firemaking XP (40 → 20), multiplier 1.
	if got := p.Skills().Experience(game.SkillFiremaking); got != before+20 {
		t.Fatalf("Firemaking XP = %d, want %d", got, before+20)
	}
	// Smokepuff graphic at height 150, no delay.
	g := p.Graphic()
	if g == nil || g.ID != 86 || g.Height != 150 || g.Delay != 0 {
		t.Fatalf("graphic = %+v, want 86/150/0", g)
	}
	// The fire sound (2596).
	if len(session.sounds) != 1 || session.sounds[0] != 2596 {
		t.Fatalf("sounds = %v, want [2596]", session.sounds)
	}
	// One charge spent, no message (the axe is not empty).
	if got := infernalCharges(p.Equipment().ItemAt(game.WeaponSlot)); got != 4999 {
		t.Fatalf("charges = %d, want 4999", got)
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none on a non-depleting burn", session.gameMessages)
	}
}

func TestTryBurnLogMultiplierAndOddXP(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	answerFiremaking(w, 41, 2) // odd XP, doubled by the outfit
	w.randInt = func(min, max int) int { return 0 }
	p := newTestPlayer("alice", 3000, 3000)
	p.Equipment().SetItem(game.WeaponSlot, chargedInfernal(13241, 5000))
	before := p.Skills().Experience(game.SkillFiremaking)
	if !w.tryBurnLog(p, 13241, 1511) {
		t.Fatal("the burn must fire")
	}
	// (41 / 2) * 2 = 41: the int truncation is on the half,
	// before the multiplier.
	if got := p.Skills().Experience(game.SkillFiremaking); got != before+41 {
		t.Fatalf("Firemaking XP = %d, want %d", got, before+41)
	}
}

func TestTryBurnLogZeroXPAnswer(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	answerFiremaking(w, 0, 0) // answered, but with no XP
	w.randInt = func(min, max int) int { return 0 }
	p := newTestPlayer("alice", 3000, 3000)
	p.Equipment().SetItem(game.WeaponSlot, chargedInfernal(13241, 5000))
	if w.tryBurnLog(p, 13241, 1511) {
		t.Fatal("an xp of 0 must not burn (TS: !(request.xp > 0))")
	}
	if got := infernalCharges(p.Equipment().ItemAt(game.WeaponSlot)); got != 5000 {
		t.Fatalf("charges = %d, want 5000 (no charge spent)", got)
	}
}

func TestTryBurnLogDepletion(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	answerFiremaking(w, 40, 0)
	w.randInt = func(min, max int) int { return 0 }
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	// Carried (not wielded) axe with one charge left.
	p.Inventory().SetItem(2, chargedInfernal(25066, 1))
	if !w.tryBurnLog(p, 25066, 1511) {
		t.Fatal("the burn must fire")
	}
	got := p.Inventory().ItemAt(2)
	if got.ID() != 25371 {
		t.Fatalf("axe id after depletion = %d, want 25371", got.ID())
	}
	if _, ok := got.GetMetaValue(infernalChargesMetaKey); ok {
		t.Fatal("depleted axe must have its meta cleared")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your infernal axe has run out of charges." {
		t.Fatalf("messages = %q, want the run-out line", session.gameMessages)
	}
}

func TestTryBurnLogWrongAxe(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	answerFiremaking(w, 40, 0)
	w.randInt = func(min, max int) int { return 0 }
	p := newTestPlayer("alice", 3000, 3000)
	p.Equipment().SetItem(game.WeaponSlot, chargedInfernal(13241, 5000))
	// A dragon axe id is not a charged infernal axe id.
	if w.tryBurnLog(p, 6739, 1511) {
		t.Fatal("a non-infernal axe id must not burn")
	}
	// An infernal axe id the player does not hold does not burn
	// either (the find fails after the query).
	if w.tryBurnLog(p, 25066, 1511) {
		t.Fatal("an unheld infernal axe id must not burn")
	}
}

func TestCheckInfernalAxe(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, chargedInfernal(13241, 5000))
	ev := &plugins.PluginItemActionEvent{
		Player: p, InterfaceID: 3214, ItemID: 13241, Slot: 0, ClickType: 3,
	}
	if !w.checkInfernalAxe(ev) {
		t.Fatal("Check must claim the click")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Infernal axe: 100.0% remaining." {
		t.Fatalf("messages = %q, want the 100.0%% line", session.gameMessages)
	}
	p.Inventory().SetItem(0, chargedInfernal(13241, 2500))
	if !w.checkInfernalAxe(ev) {
		t.Fatal("Check must claim the click")
	}
	if session.gameMessages[1] != "Infernal axe: 50.0% remaining." {
		t.Fatalf("messages = %q, want the 50.0%% line", session.gameMessages)
	}
	// The Wiki's example value: 3460/5000 = 69.2%.
	p.Inventory().SetItem(0, chargedInfernal(13241, 3460))
	if !w.checkInfernalAxe(ev) {
		t.Fatal("Check must claim the click")
	}
	if session.gameMessages[2] != "Infernal axe: 69.2% remaining." {
		t.Fatalf("messages = %q, want the 69.2%% line", session.gameMessages)
	}
	// An uncharged axe falls through: no claim, no message.
	p.Inventory().SetItem(0, game.NewItem(13242, 1))
	evUncharged := &plugins.PluginItemActionEvent{
		Player: p, InterfaceID: 3214, ItemID: 13242, Slot: 0, ClickType: 3,
	}
	if w.checkInfernalAxe(evUncharged) {
		t.Fatal("Check on an uncharged axe must fall through")
	}
	if len(session.gameMessages) != 3 {
		t.Fatalf("messages = %q, want no further message", session.gameMessages)
	}
}

func TestChopSuccessBurnsInfernalLog(t *testing.T) {
	// The core-loop wiring: a successful chop with the
	// charged infernal axe burns the log when Firemaking
	// answers the log-xp query (chopSuccess, before
	// prepareLog — plugin.js:1410-1413). No log reaches the
	// inventory, Woodcutting XP still lands, and half the
	// Firemaking XP is granted. The session axe carries the
	// real id (13241); the fixture tables cannot resolve it,
	// so the session is built directly.
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	answerFiremaking(w, 40, 0)
	w.randInt = func(min, max int) int { return 0 } // winning roll
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	p.Equipment().SetItem(game.WeaponSlot, chargedInfernal(13241, 5000))
	normal := treeByName(loadFixtureTables(t), "normal tree")
	state := &session{
		tree:     normal,
		axe:      Axe{ID: 13241, Item: "infernal_axe"},
		objectID: normal.ObjectIDs[0],
		location: game.Location{X: 3201, Y: 3200, Z: 0},
	}
	fmBefore := p.Skills().Experience(game.SkillFiremaking)
	w.chopSuccess(p, state)
	if p.Inventory().Contains(normal.LogID) {
		t.Fatal("a burnt log must not reach the inventory")
	}
	if got := p.Skills().Experience(game.SkillFiremaking); got != fmBefore+20 {
		t.Errorf("Firemaking XP = %d, want %d", got, fmBefore+20)
	}
	if got := p.Skills().Experience(game.SkillWoodcutting); got != 25 {
		t.Errorf("Woodcutting XP = %d, want 25 (burn does not cut it)", got)
	}
	if got := infernalCharges(p.Equipment().ItemAt(game.WeaponSlot)); got != 4999 {
		t.Fatalf("charges after burn = %d, want 4999", got)
	}
	// Without a Firemaking answer the same chop delivers the
	// log and spends no charge.
	w2, _ := newTestWoodcutting(t, loadFixtureTables(t))
	w2.randInt = func(min, max int) int { return 0 }
	p2 := newTestPlayer("bob", 3200, 3200)
	setLevel(p2, game.SkillWoodcutting, 99)
	p2.Equipment().SetItem(game.WeaponSlot, chargedInfernal(13241, 5000))
	w2.chopSuccess(p2, state)
	if !p2.Inventory().Contains(normal.LogID) {
		t.Fatal("an unanswered burn must deliver the log")
	}
	if got := infernalCharges(p2.Equipment().ItemAt(game.WeaponSlot)); got != 5000 {
		t.Fatalf("charges = %d, want 5000 (no burn, no charge spent)", got)
	}
}

func TestInfernalAxeRegistration(t *testing.T) {
	w, _ := newTestWoodcutting(t, loadFixtureTables(t))
	hub := plugins.NewHub([]plugins.Plugin{{
		Name:     "Woodcutting",
		Register: func(api *plugins.Api) { w.registerInfernalAxe(api) },
	}}, plugins.HubOptions{Logger: log.New(io.Discard, "", 0)})
	hub.SetItemResolver(func(id int) (string, bool) {
		switch id {
		case 13241:
			return "Infernal axe", false
		case 25066, 30347:
			return "Infernal axe (or)", false
		default:
			return "", false
		}
	})
	hub.SetItemActionsResolver(func(id int) []string {
		if id == 13241 {
			return []string{"", "Wield", "Check", "Toggle", "Drop"}
		}
		if id == 25066 || id == 30347 {
			return []string{"Wield", "Check", "Revert", "Options", "Drop"}
		}
		return []string{"", "", "", "", "Drop"}
	})
	hub.Load()

	p := newTestPlayer("alice", 3000, 3000)
	session := &testSession{}
	p.Session = session
	p.Inventory().SetItem(0, chargedInfernal(25066, 1250))
	ev := &plugins.PluginItemActionEvent{
		Player: p, InterfaceID: 3214, ItemID: 25066, Slot: 0, ClickType: 2,
	}
	if !hub.EmitItemAction(ev) {
		t.Fatal("Check must claim through the hub")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Infernal axe: 25.0% remaining." {
		t.Fatalf("messages = %q, want the 25.0%% line", session.gameMessages)
	}
}
