package fishing

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Sub-unit test fixtures (hand-built only; no live cache).
// Fixture ids live in a range far from the real numeric
// harpoon ids (21031+), so a test can tell a resolved id
// from a hard-coded one.

// testSession captures game messages and sounds for
// fishing sub-unit tests (a SessionPort with the chat and
// sound extensions).
type testSession struct {
	gameMessages []string
	sounds       []int
}

func (s *testSession) SendPacket(packet []byte) {}
func (s *testSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *testSession) SendDestination(x, y int)      {}
func (s *testSession) SendVarp(id, value int)        {}
func (s *testSession) FlushTick(cycle int64) error   { return nil }
func (s *testSession) Close(code int, reason string) {}
func (s *testSession) IsOpen() bool                  { return true }
func (s *testSession) SendGameMessage(message string) {
	s.gameMessages = append(s.gameMessages, message)
}
func (s *testSession) SendTypedGameMessage(chatType int, message, from string) {}
func (s *testSession) SendSoundEffect(soundID, loopType, delay, volume int) {
	s.sounds = append(s.sounds, soundID)
}
func (s *testSession) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
}

var (
	_ game.SessionPort       = (*testSession)(nil)
	_ game.ChatMessageSender = (*testSession)(nil)
	_ game.SoundSender       = (*testSession)(nil)
)

// stackableDefs is a minimal game.ItemDefinitionSource
// marking the given ids stackable: shards, flakes and
// karambwanji stack in the cache, and with no source
// installed the container treats every item as
// non-stackable, so DeleteNumber would clear a whole stack
// instead of subtracting.
type stackableDefs struct{ stackable map[int]bool }

func (s stackableDefs) IsStackable(id int) bool      { return s.stackable[id] }
func (s stackableDefs) IsNoted(id int) bool          { return false }
func (s stackableDefs) WeightGrams(id int) int       { return 0 }
func (s stackableDefs) EquipmentSlot(id int) int     { return -1 }
func (s stackableDefs) IsDoubleHanded(id int) bool   { return false }
func (s stackableDefs) Requirements(id int) []int    { return nil }
func (s stackableDefs) CombatRequirement(id int) int { return 0 }
func (s stackableDefs) IsTradeable(id int) bool      { return false }
func (s stackableDefs) IsSellable(id int) bool       { return false }
func (s stackableDefs) IsDropable(id int) bool       { return false }
func (s stackableDefs) UnNote(id int) int            { return id }
func (s stackableDefs) PlaceholderID(id int) int     { return -1 }
func (s stackableDefs) ItemName(id int) string       { return "" }

// namedDefs is an ItemDefinitionSource test double naming
// items by id: ownsClueBottle scans inventory + bank by
// display name, so the test names the clue forms it plants.
type namedDefs struct {
	stackableDefs
	names map[int]string
}

func (s namedDefs) ItemName(id int) string { return s.names[id] }

func newTestPlayer(name string, x, y int) *game.Player {
	p := game.NewPlayer(name, game.Location{X: x, Y: y, Z: 0})
	p.SetRegistered(true)
	return p
}

func setLevel(p *game.Player, skill, level int) {
	p.Skills().SetCurrentLevel(skill, level, true)
	p.Skills().SetMaxLevel(skill, level)
}

// fixtureIDs returns deterministic fake resolved ids for
// every FishingIDs field.
func fixtureIDs() *FishingIDs {
	return &FishingIDs{
		SpiritFlakes:             6001,
		RadasBlessing1:           6002,
		RadasBlessing2:           6003,
		RadasBlessing3:           6004,
		RadasBlessing4:           6005,
		ClueBottleBeginner:       6010,
		ClueBottleEasy:           6011,
		ClueBottleMedium:         6012,
		ClueBottleHard:           6013,
		ClueBottleElite:          6014,
		AnglerHat:                6020,
		AnglerTop:                6021,
		AnglerWaders:             6022,
		AnglerBoots:              6023,
		SpiritAnglerHeadband:     6024,
		SpiritAnglerTop:          6025,
		SpiritAnglerWaders:       6026,
		SpiritAnglerBoots:        6027,
		CrystalShard:             6030,
		SmoulderingStone:         6031,
		DragonHarpoon:            6032,
		IceGloves:                6033,
		SmithsGlovesI:            6034,
		ElvenSignet:              6040,
		ElvenSignet2:             6041,
		ElvenSignet3:             6042,
		CelestialSignet:          6043,
		CelestialSignetUncharged: 6044,
		ImbuedRings:              []int{6050, 6051, 6052, 6053, 6054, 6055, 6056, 6057},
		KarambwanVessel:          6060,
		KarambwanVesselBaited:    6061,
		RawKarambwanji:           6062,
	}
}

// emittedEvent is one captured custom-event emit.
type emittedEvent struct {
	name    string
	payload any
}

// newTestFishing builds a fishing instance over fixture
// tables and ids on a members world, with a capturing emit
// func. Tests that need answered queries replace f.emit.
func newTestFishing(t *testing.T) (*fishing, *[]emittedEvent) {
	t.Helper()
	var emitted []emittedEvent
	f := &fishing{tables: loadFixture(t), ids: fixtureIDs(), membersWorld: true}
	f.emit = func(name string, payload any) {
		emitted = append(emitted, emittedEvent{name: name, payload: payload})
	}
	return f, &emitted
}

// hubWith registers f's sub-unit attach calls on a fresh hub
// and loads it, for registration round-trip tests.
func hubWith(t *testing.T, register func(api *plugins.Api)) *plugins.Hub {
	t.Helper()
	hub := plugins.NewHub([]plugins.Plugin{{
		Name:        "FishingTest",
		Description: "Test plugin for the fishing sub-unit registration.",
		Register:    register,
	}}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	return hub
}
