package game

import (
	"sync"
	"testing"
	"time"
)

// TestWorldConcurrentQueueing exercises the coordinator's thread-safety
// contract: Queue* methods, IsFull, and PlayerByName are called from
// network goroutines while the tick goroutine drains. Run with -race:
// any unsynchronized queue or list access fails the test.
func TestWorldConcurrentQueueing(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	// Tick in the background while queue operations race it.
	stop := make(chan struct{})
	var tickWg sync.WaitGroup
	tickWg.Add(1)
	go func() {
		defer tickWg.Done()
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				w.Process()
			}
		}
	}()

	const workers = 8
	const perWorker = 25
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				name := "player" + string(rune('a'+id)) + string(rune('0'+j%10)) + string(rune('0'+j/10))
				p := NewPlayer(name, Location{X: 3089, Y: 3524, Z: 0})
				w.QueueAddPlayer(p)
				_ = w.IsFull()
				_ = w.PlayerByName(name)
			}
		}(i)
	}
	wg.Wait()
	close(stop)
	tickWg.Wait()

	// Drain remaining queues on this goroutine.
	for i := 0; i < 5; i++ {
		w.Process()
	}
	if got := w.Players().Len(); got != workers*perWorker {
		t.Fatalf("players = %d, want %d", got, workers*perWorker)
	}
}

// TestWorldConcurrentLogoutRequests queues logouts from several
// goroutines while the tick loop answers them.
func TestWorldConcurrentLogoutRequests(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	players := make([]*Player, 20)
	for i := range players {
		players[i] = NewPlayer("p", Location{X: 3089, Y: 3524, Z: 0})
		// Distinct usernames: the add drain kicks same-name copies.
		players[i].Username = "logout" + string(rune('a'+i))
		w.QueueAddPlayer(players[i])
	}
	w.Process()
	if got := w.Players().Len(); got != len(players) {
		t.Fatalf("players after add = %d, want %d", got, len(players))
	}

	// Queue logouts concurrently with ticks. Sessions are nil, so
	// answerLogoutRequest skips them (session-not-connected guard);
	// remove them directly instead and assert the queue machinery
	// stays consistent under -race.
	stop := make(chan struct{})
	var tickWg sync.WaitGroup
	tickWg.Add(1)
	go func() {
		defer tickWg.Done()
		ticker := time.NewTicker(2 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				w.Process()
			}
		}
	}()
	var wg sync.WaitGroup
	for _, p := range players {
		wg.Add(1)
		go func(p *Player) {
			defer wg.Done()
			w.QueueRemovePlayer(p.Index)
			w.QueueLogout(p.Index)
		}(p)
	}
	wg.Wait()
	close(stop)
	tickWg.Wait()
	for i := 0; i < 5; i++ {
		w.Process()
	}
	if got := w.Players().Len(); got != 0 {
		t.Fatalf("players after concurrent removal = %d, want 0", got)
	}
}
