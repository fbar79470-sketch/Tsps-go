package plugins

import "tsps-go/internal/game"

// Sound / Sounds Core members: the typed Go form of
// core.Sound and core.Sounds (PluginTypes.ts:1639-1640;
// PluginManager.ts:2692-2693, getCoreApi). In TS both are
// `any` — the Sound class and the Sounds statics object.
// In Go they are typed: SoundClass mirrors the Sound
// constructor + registries, SoundsService mirrors the
// Sounds send path, both delegating to internal/game
// (sounds.go), where the behavior lives.
//
// The 161 named sounds (Sound.LEVEL_UP et al.) are NOT
// duplicated here: they are game package vars
// (game.SoundLevelUp, ...), which content imports from
// internal/game directly, as it already imports game
// types from event payloads. The Core surface is the
// constructor and the registries; the table itself has
// one home.
//
// NO HOOK FAMILIES. No on*/emit* family exists for sound
// in TS: PluginTypes.ts and PluginManager.ts contain no
// onSound/emitSound family (verified by grep at bfd81e3),
// and no camera surface exists either — "camera" appears
// nowhere in PluginTypes.ts or PluginManager.ts, so no
// camera Core member is ported and none is invented. The
// camera senders are engine-side (internal/session
// /camera_sender.go); content that needs them goes
// through a future PluginApi member only if TS gains one.
// This file therefore adds NO hookRegistry methods and NO
// Api members: the firable-family count (44/108) and the
// PluginApi member count (19/95) are unchanged. The Core
// count moves 4 -> 6 with Sound and Sounds (Projectile,
// in projectiles.go, makes 7); the coordinator records
// the counts in core.go/hooks.go/doc.go, which this phase
// must not edit — core.go's NOT-mirrored list still names
// `Sound`, `Sounds`, and `Projectile` until the
// coordinator updates it.

// SoundClass mirrors core.Sound: the Sound class
// (game/Sound.ts). It is stateless; the zero value is
// ready. Content constructs sounds and resolves known
// sounds through it; the named statics live in
// internal/game (see the file header).
type SoundClass struct{}

// New constructs a Sound: new Sound(id, volume, delay,
// loopType) (Sound.ts:258-264). The argument order is
// part of the port: delay precedes loopType.
func (SoundClass) New(id, volume, delay, loopType int) game.Sound {
	return game.NewSound(id, volume, delay, loopType)
}

// ResolveKnownSound resolves a sound by id or name, as
// Sounds.resolveKnownSound does (Sounds.ts:131-152); see
// game.ResolveKnownSound for the normalization rules.
func (SoundClass) ResolveKnownSound(token any) (game.Sound, bool) {
	return game.ResolveKnownSound(token)
}

// KnownSoundNames returns the known sound names, sorted
// (knownSoundNames, Sounds.ts:154-156).
func (SoundClass) KnownSoundNames() []string {
	return game.KnownSoundNames()
}

// SoundsService mirrors core.Sounds: the Sounds statics
// (game/Sounds.ts). It is stateless; the zero value is
// ready. All methods delegate to internal/game, where the
// send path, the area-matching rule, and the bot-guard
// degeneration are documented.
type SoundsService struct{}

// SendSound sends a sound for an entity: a player hears a
// private synth; anything else with a location becomes an
// area sound (Sounds.sendSound, Sounds.ts:44-68). See
// game.SendSound.
func (SoundsService) SendSound(entity any, sound game.Sound) {
	game.SendSound(entity, sound)
}

// SendSoundEffect sends a sound direct to one player,
// unguarded (Sounds.sendSoundEffect, Sounds.ts:70-72). See
// game.SendSoundEffect.
func (SoundsService) SendSoundEffect(player *game.Player, soundID, loopType, delay, volume int) {
	game.SendSoundEffect(player, soundID, loopType, delay, volume)
}

// PlayAreaSoundAt plays a sound at an entity's own tile
// with the given radius (Sounds.playAreaSoundAt,
// Sounds.ts:79-94). See game.PlayAreaSoundAt.
func (SoundsService) PlayAreaSoundAt(entity any, sound game.Sound, radius int) {
	game.PlayAreaSoundAt(entity, sound, radius)
}

// PlayAreaSound broadcasts a positioned sound
// (Sounds.playAreaSound, Sounds.ts:96-127). See
// game.PlayAreaSound.
func (SoundsService) PlayAreaSound(opts game.PlayAreaSoundOptions) {
	game.PlayAreaSound(opts)
}

// Sound returns the Sound class surface: the core.Sound
// member (PluginTypes.ts:1639). Safe on a nil Core: the
// surface is stateless.
func (c *Core) Sound() SoundClass {
	return SoundClass{}
}

// Sounds returns the Sounds send surface: the core.Sounds
// member (PluginTypes.ts:1640). Safe on a nil Core: the
// surface is stateless.
func (c *Core) Sounds() SoundsService {
	return SoundsService{}
}
