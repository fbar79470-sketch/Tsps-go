package config

// Game constants, quoted from the TypeScript sources cited per constant.
// See ANALYSIS.md Appendix B for the same cheat-sheet.
const (
	// TickRateMs is the main game loop cycle rate in milliseconds.
	// GameConstants.GAME_ENGINE_PROCESSING_CYCLE_RATE
	// (game/GameConstants.ts).
	TickRateMs = 600

	// MaxPlayers is the capacity of the world player list.
	// World.MAX_PLAYERS (game/World.ts:33).
	MaxPlayers = 2048

	// MaxNPCs is the capacity of the world NPC list.
	// World.MAX_NPCS (game/World.ts:36).
	MaxNPCs = 65534

	// MaxLocalPlayers is the maximum number of other players one
	// player can observe. World.MAX_LOCAL_PLAYERS (game/World.ts:741).
	MaxLocalPlayers = 255

	// MaxLocalNPCs is the maximum number of NPCs one player can
	// observe. World.MAX_LOCAL_NPCS (game/World.ts:743).
	MaxLocalNPCs = 255

	// MaxNewLocalPlayersPerCycle caps how many new players may enter
	// a player's local list in one tick.
	// World.MAX_NEW_LOCAL_PLAYERS_PER_CYCLE (game/World.ts:742).
	MaxNewLocalPlayersPerCycle = 25

	// QueueDrainCap is the maximum number of queued add/remove entries
	// drained per tick per queue.
	// GameConstants.QUEUED_LOOP_THRESHOLD (game/GameConstants.ts).
	QueueDrainCap = 150

	// MaxFrameBytes is the largest single WebSocket game message.
	// MAX_GAME_MESSAGE_BYTES = 64 * 1024 (net/BinaryChannel.ts:3).
	MaxFrameBytes = 65536

	// PacketQueueLimit is the maximum number of queued inbound frames
	// / turn packets per connection; exceeding it closes the
	// connection. NetworkConstants.PACKET_QUEUE_LIMIT
	// (net/NetworkConstants.ts:14).
	PacketQueueLimit = 240

	// RunEnergyMaxUnits is OSRS run energy on its 0-10,000 unit scale;
	// the orb shows only the whole percentage.
	// GameConstants.RUN_ENERGY_MAX_UNITS (game/GameConstants.ts).
	RunEnergyMaxUnits = 10000
)
