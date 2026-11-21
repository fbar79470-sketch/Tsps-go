// Package dateminutes is the Go port of the DateMinutes plugin
// (server/plugins/world/DateMinutes.plugin.js), the Phase 10 proof
// content piece: the smallest real TS plugin whose every dependency
// already exists in Go. It sends the client the date_minutes varp
// (3078) — whole minutes since 1970 — on login and each new minute,
// so home and minigame teleport cooldown timers can display (cache
// scripts compare it with the minute of the last home (892) and
// minigame (888) teleport).
//
// The port is faithful: same plugin name and description, same two
// hook registrations, same dedup rule (skip when the minute equals
// the last minute sent to that player), same send through the
// sendConfig seam (Player.SendVarp, porting
// PacketSender.sendConfig, PacketSender.ts:355-359). The
// dateMinutes helper is ported from
// server/plugins/combat/HomeTeleportSequence.js:194-196.
//
// Two deliberate deviations, both forced by the language change:
//
//   - Dedup state: TS keeps a WeakMap keyed by player, so entries
//     vanish with the player. Go has no weak map, so the state is a
//     mutex-guarded map keyed by *game.Player (never by player
//     index — indexes are reused), cleaned up by an extra
//     OnPlayerDisconnect registration the TS plugin does not have.
//     The mutex is Go-only: login emits run on the login goroutine
//     while process/disconnect emits run on the tick goroutine.
//   - Dedup lives here, in the content piece, exactly where TS
//     keeps it (the WeakMap) — not in the session's varp store.
package dateminutes

import (
	"sync"
	"time"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// DateMinutesVarp is the date_minutes varp id (3078).
const DateMinutesVarp = 3078

// dateMinutes returns whole minutes since 1970 for the given epoch
// milliseconds (dateMinutes, HomeTeleportSequence.js:194-196:
// Math.floor(nowMs / 60_000)).
func dateMinutes(nowMs int64) int {
	return int(nowMs / 60_000)
}

// piece holds the plugin's dedup state: the last minute sent to
// each player. One instance serves the registered Plugin value;
// tests build their own with a fake clock.
type piece struct {
	mu       sync.Mutex
	lastSent map[*game.Player]int
	now      func() int64
}

func newPiece(now func() int64) *piece {
	if now == nil {
		now = func() int64 { return time.Now().UnixMilli() }
	}
	return &piece{lastSent: make(map[*game.Player]int), now: now}
}

// send sends the current minute to p unless it equals the last
// minute sent to p (sendDateMinutes, DateMinutes.plugin.js:15-20).
func (s *piece) send(p *game.Player) {
	minute := dateMinutes(s.now())
	s.mu.Lock()
	last, ok := s.lastSent[p]
	if ok && last == minute {
		s.mu.Unlock()
		return
	}
	s.lastSent[p] = minute
	s.mu.Unlock()
	p.SendVarp(DateMinutesVarp, minute)
}

// forget drops p's dedup entry when p leaves the world: the Go
// stand-in for the TS WeakMap, which drops entries with the
// player. Without it the map would retain every player that ever
// logged in.
func (s *piece) forget(p *game.Player) {
	s.mu.Lock()
	delete(s.lastSent, p)
	s.mu.Unlock()
}

func (s *piece) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "DateMinutes",
		Description: "Sends the client the current minute since 1970 on login and each new minute, so home and minigame teleport cooldown timers can display.",
		Register: func(api *plugins.Api) {
			api.OnPlayerLogin(func(e *plugins.PlayerLoginEvent) { s.send(e.Player) })
			api.OnPlayerProcess(func(e *plugins.PlayerProcessEvent) { s.send(e.Player) })
			// Disconnect cleanup only; see the package comment.
			api.OnPlayerDisconnect(func(e *plugins.PlayerDisconnectEvent) { s.forget(e.Player) })
		},
	}
}

// Plugin is the registered DateMinutes plugin value.
var Plugin = newPiece(nil).plugin()

func init() {
	plugins.Register(Plugin)
}
