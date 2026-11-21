package main

import "tsps-go/internal/game"

// Sound wiring (phase 21): the cmd/server adapter for the
// sound/projectile broadcast seam (internal/game sounds.go /
// projectile.go). It follows the installRelationsSeams
// precedent (socialwiring.go): a narrow install, called
// once at boot, that hands a game-layer seam the world's
// live player list.
//
// One entry point, wired by buildServer in server.go (the
// coordinator adds the call; this file must not edit
// server.go):
//
//   - InstallSoundWiring, once in buildServer, before
//     ticking starts: installs the network-player source
//     game.PlayAreaSound, game.Projectile.SendProjectile,
//     and the Sounds area sends iterate (World.getPlayers
//     / World.forEachNetworkPlayer, World.ts:159-169,
//     232-234) from the world's player list, in slot
//     order.
//
// NIL SAFETY. A nil world installs nothing: broadcasts
// keep the unwired behavior (no players, no sends) and
// direct sends are unaffected.
//
// NO OTHER WIRING. The session senders need no install:
// content and the game layer reach them through the
// SoundSender / ProjectileSender extensions on the
// player's attached session. The hub Core members
// (Sound/Sounds/Projectile) need no install either: they
// are stateless delegates over internal/game. Picker
// wiring (equipment/NPC-combat sounds at equip/combat
// call sites) is NOT installed here: no Go call site
// exists yet, and the missing sites are reported as gaps
// with this phase.

// InstallSoundWiring installs the network-player source
// broadcasts iterate: the world's registered players, in
// slot order (MobileList.All). Call once at boot, before
// ticking starts: the source is read on every broadcast
// once the engine runs. A nil world is a no-op.
func InstallSoundWiring(world *game.World) {
	if world == nil {
		return
	}
	game.SetNetworkPlayerSource(func() []*game.Player {
		return world.Players().All()
	})
}
