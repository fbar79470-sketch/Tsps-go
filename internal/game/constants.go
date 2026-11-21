package game

// Capacities and thresholds quoted from the TypeScript sources. These are
// literals here (not loaded from config) because the values are fixed by the
// TS sources cited; the coordinator may rewire them to config later, but the
// defaults must stay equal to the cited values.
const (
	// TickRateMs is the main game loop rate.
	// GameConstants.ts: GAME_ENGINE_PROCESSING_CYCLE_RATE = 600.
	TickRateMs = 600

	// QueueDrainCap bounds how many queued adds/removes one World.Process
	// cycle drains from each queue.
	// GameConstants.ts: QUEUED_LOOP_THRESHOLD = 150.
	QueueDrainCap = 150

	// MaxPlayers is the capacity of the world player list.
	// World.ts:33: MAX_PLAYERS = 2048.
	MaxPlayers = 2048

	// MaxNPCs is the capacity of the world NPC list. NPC indices are 16 bits
	// on the wire with 0xffff terminating the list, so indices run 1-65534.
	// World.ts:36: MAX_NPCS = 65534.
	MaxNPCs = 65534

	// MaxLocalPlayers is the per-player local player list limit.
	// World.ts:741: MAX_LOCAL_PLAYERS = 255.
	MaxLocalPlayers = 255

	// MaxNewLocalPlayersPerCycle caps new locals added per tick.
	// World.ts:742 (adjacent to MAX_LOCAL_PLAYERS).
	MaxNewLocalPlayersPerCycle = 25

	// MaxLocalNPCs is the per-player local NPC list limit.
	// World.ts:743: MAX_LOCAL_NPCS = 255.
	MaxLocalNPCs = 255

	// UpdateBucketRadius is the default bucket-collection radius, in
	// 8x8 chunks, and the floor for the NPC nearby radius.
	// World.ts:44: UPDATE_BUCKET_RADIUS = 2.
	UpdateBucketRadius = 2

	// PreferredViewDistance is the view distance a player's local
	// player list regrows toward when the view is not saturated.
	// Player.ts:213: PREFERRED_VIEW_DISTANCE = 15.
	PreferredViewDistance = 15

	// PreferredLocalPlayers is the local-player count at or above
	// which the view distance shrinks instead of regrowing.
	// Player.ts:214: PREFERRED_LOCAL_PLAYERS = 250.
	PreferredLocalPlayers = 250

	// ViewDistanceRegrowCycles is the number of consecutive
	// unsaturated cycles before the view distance regrows by one.
	// Player.ts:215: VIEW_DISTANCE_REGROW_CYCLES = 10.
	ViewDistanceRegrowCycles = 10

	// NPCViewDistance is the default distance at which NPCs are seen.
	// Player.ts:1079: NPC_VIEW_DISTANCE = 15.
	NPCViewDistance = 15

	// MaxNPCViewDistance is the widest NPC view distance (NPC
	// positions are sent as signed 8-bit offsets from the player).
	// Player.ts:1081: MAX_NPC_VIEW_DISTANCE = 127.
	MaxNPCViewDistance = 127

	// MaxFixedViewDistance is the widest fixed (spectator) player
	// view distance. TS uses the literal 64 in setFixedViewDistance
	// (Player.ts:1074); it has no named constant there.
	MaxFixedViewDistance = 64

	// PacketQueueLimit caps queued inbound turn packets per player.
	// NetworkConstants.ts: PACKET_QUEUE_LIMIT = 240.
	PacketQueueLimit = 240

	// LogoutCombatTicks is how long after the last hit the logout button
	// stays refused ("10 seconds" per rsprox captures).
	// Player.ts:672: LOGOUT_COMBAT_TICKS = 16.
	LogoutCombatTicks = 16

	// LogoutHoldTicks is how long a player removed in combat stays in the
	// world before being forced out (60 seconds, OSRS Wiki).
	// Player.ts:674: LOGOUT_HOLD_TICKS = 100.
	LogoutHoldTicks = 100

	// SlowTaskWarnMs is the per-tick duration above which a task tick is
	// reported through TaskManager.OnSlowTask.
	// TaskManager.ts: SLOW_TASK_WARN_MS = 50.
	SlowTaskWarnMs = 50

	// SlowTaskLogCooldownMs throttles slow-task reports per task name.
	// TaskManager.ts: SLOW_TASK_LOG_COOLDOWN_MS = 3000.
	SlowTaskLogCooldownMs = 3000
)
