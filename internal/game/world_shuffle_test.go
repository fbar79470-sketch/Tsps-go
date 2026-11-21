package game

import "testing"

// TestShuffleKeepsDuellersInPlace pins the duelling exception in
// shufflePlayerProcessOrder (World.ts:985-998): players in a duel
// keep their slots across reshuffles, while everyone else still
// moves.
func TestShuffleKeepsDuellersInPlace(t *testing.T) {
	w := newTestWorld()
	players := make([]*Player, 10)
	for i := range players {
		players[i] = NewPlayer("p", w.Spawn())
	}
	players[2].Dueling().SetState(DuelStateInDuel)
	players[7].Dueling().SetState(DuelStateStartingDuel)
	w.processOrder = append([]*Player(nil), players...)

	moved := false
	for round := 0; round < 100; round++ {
		w.SeedRand(int64(round))
		w.shufflePlayerProcessOrder()
		if w.processOrder[2] != players[2] || w.processOrder[7] != players[7] {
			t.Fatalf("round %d: duelling player moved: slots 2,7 = %v, %v", round, w.processOrder[2] == players[2], w.processOrder[7] == players[7])
		}
		for i, p := range w.processOrder {
			if i != 2 && i != 7 && p != players[i] {
				moved = true
			}
		}
		// Restore the pre-shuffle order for the next round so
		// every round shuffles the same starting order.
		w.processOrder = append([]*Player(nil), players...)
	}
	if !moved {
		t.Fatal("non-duelling players never moved across 100 shuffles")
	}
}
