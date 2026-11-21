package main

import (
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// Ground-item wiring: the cmd/server adapter between the game
// layer's GroundItemManager and the session layer's per-session
// ground-item sends. It ports the PacketSender side of
// ItemOnGroundManager.perform (PacketSender.ts:1424-1476) as glue:
// the manager fans operations out through the GroundItemSender
// interface, and this adapter converts game views to protocol
// views and reaches the session's GroundItemSync. The game layer
// never imports protocol and the session layer never imports
// game; this package imports both, as the flush adapter does.
//
// The manager is constructed in buildServer (after the definition
// load, so the definition-backed sources see the loaded Store),
// spawns apply through applyGroundItemSpawns before the engine
// starts, the per-tick pass runs behind WorldHooks.GroundItems,
// and each entering session gets its visible-item source closure
// in AddPlayer (see installGroundItemSource).

// groundItemSender implements game.GroundItemSender by reaching
// the player's session through the SessionPort interface (the
// appearanceBlock pattern in syncsource.go): a session-less
// player, or a SessionPort that is not a *session.Session, sends
// nothing. Stateless: one shared instance serves the manager.
type groundItemSender struct{}

// SendGroundItemDelta converts the game views to protocol views
// and sends one delta packet through the player's session sync
// state. A nil sync (a zero-value Session) drops the delta: all
// GroundItemSync methods are nil-safe.
func (groundItemSender) SendGroundItemDelta(player *game.Player, upserts []game.GroundItemView, removes []int) {
	if player == nil || player.Session == nil {
		return
	}
	s, ok := player.Session.(interface {
		GroundItems() *session.GroundItemSync
	})
	if !ok {
		return
	}
	sync := s.GroundItems()
	if sync == nil {
		return
	}
	views := make([]protocol.GroundItemView, len(upserts))
	for i, v := range upserts {
		views[i] = convertGroundItemView(v)
	}
	sync.SendGroundItemDelta(views, removes)
}

// convertGroundItemView converts one game-layer view to its
// protocol form, field for field. Tick values are int64 in the
// game view and int in the protocol view; the encoder truncates
// to int32 exactly as the TS `| 0` conversions do.
func convertGroundItemView(v game.GroundItemView) protocol.GroundItemView {
	return protocol.GroundItemView{
		EnabledOps:       v.EnabledOps,
		ID:               v.ID,
		ItemID:           v.ItemID,
		Quantity:         v.Quantity,
		X:                v.X,
		Y:                v.Y,
		Level:            v.Level,
		CreatedTick:      int(v.CreatedTick),
		PrivateUntilTick: int(v.PrivateUntilTick),
		ExpiresTick:      int(v.ExpiresTick),
		OwnerID:          v.OwnerID,
		IsPrivate:        v.IsPrivate,
		Ownership:        v.Ownership,
	}
}

// newGroundItemManager constructs the world's ground-item manager
// with every seam wired: the world's player list, the send
// adapter, the world's task scheduler (respawn tasks), the world
// cycle, and the definition-backed stackable/tradeable/dropable
// sources from the loaded Store. A nil Store (a load that
// produced none) leaves the definition sources nil, and the
// manager degrades as documented on GroundItemManagerConfig
// (no merges, public-visibility tradeable term false).
func newGroundItemManager(world *game.World, store *definition.Store) *game.GroundItemManager {
	cfg := game.GroundItemManagerConfig{
		Players:   func() []*game.Player { return world.Players().All() },
		Sender:    groundItemSender{},
		Scheduler: world.Tasks(),
		Cycle:     world.ProcessCycle,
	}
	if store != nil {
		cfg.Stackable = func(itemID int) bool { return store.Item(itemID).IsStackable() }
		cfg.TradeableDropable = func(itemID int) (bool, bool) {
			def := store.Item(itemID)
			return def.IsTradeable(), def.IsDropable()
		}
	}
	return game.NewGroundItemManager(cfg)
}

// applyGroundItemSpawns registers every loaded ground-item spawn
// with the manager (GroundItemSpawnDefinitionLoader.load,
// GroundItemSpawnDefinitionLoader.ts:30-81, reached through
// GameBuilder.loadStartupData). It runs before the engine starts,
// so the boot-time CREATE fan-out reaches zero players, as in TS
// (no player is logged in during GameBuilder). A nil manager or
// store applies nothing.
func applyGroundItemSpawns(mgr *game.GroundItemManager, store *definition.Store) {
	if mgr == nil || store == nil {
		return
	}
	for _, rec := range store.GroundItems() {
		mgr.ApplySpawn(rec)
	}
}

// installGroundItemSource installs the session's visible-item
// source: the FlushTick region-change block reads the stacks the
// owner can see, converted to protocol views, through it. A nil
// manager yields an empty source. It runs at AddPlayer time,
// before the player is queued, so the first FlushTick already
// sends the visible snapshot.
func installGroundItemSource(sess *session.Session, mgr *game.GroundItemManager, world *game.World, owner *game.Player) {
	sync := sess.GroundItems()
	if sync == nil {
		return
	}
	sync.SetGroundItemSource(func() []protocol.GroundItemView {
		if mgr == nil {
			return nil
		}
		views := mgr.VisibleViews(owner, world.ProcessCycle())
		out := make([]protocol.GroundItemView, len(views))
		for i, v := range views {
			out[i] = convertGroundItemView(v)
		}
		return out
	})
}
