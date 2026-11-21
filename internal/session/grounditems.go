package session

import (
	"sync"

	"tsps-go/internal/config"
	"tsps-go/internal/protocol"
)

// Per-session ground-item sends: the PacketSender ground-item methods
// (PacketSender.ts:1424-1476) in Go form. In TS the serial and the
// send methods live on PacketSender, one per player; in Go they live
// on GroundItemSync, one per session, because the Session struct
// itself is owned by the coordinator's flush integration (this file
// must not edit session.go). The coordinator creates one
// GroundItemSync per entering session, installs the visible-item
// source on it, and calls it from two places: the FlushTick
// region-change block (SendVisibleGroundItems) and the cmd/server
// GroundItemSender adapter (SendGroundItemDelta). All sends go
// through Session.SendPacket and join the pending queue like any
// other queued packet.
//
// Thread-safety: the serial and the source are guarded by the sync's
// own mutex, and SendPacket locks internally, so every method here
// is safe from the tick goroutine (the FlushTick and manager paths)
// and from any other goroutine. SendVisibleGroundItems snapshots
// the source under the mutex and calls it without holding it: the
// source reads world state, and holding this mutex across that call
// would invert the lock order against world code that sends while
// holding a world lock (the same snapshot discipline FlushTick uses
// for its FlushSource).

// GroundItemsPerPacket is the stacks per ground-item packet:
// GROUND_ITEMS_PER_PACKET (PacketSender.ts:172),
// floor((MAX_GAME_MESSAGE_BYTES - 16) / GROUND_ITEM_BYTES).
// MAX_GAME_MESSAGE_BYTES is config.MaxFrameBytes (64 KiB,
// BinaryChannel.ts:5); the -16 reserves the packet framing and the
// delta remove section so one packet always fits one game message.
// (65536 - 16) / 34 = 1927.
const GroundItemsPerPacket = (config.MaxFrameBytes - 16) / protocol.GroundItemBytes

// GroundItemSource supplies the stacks visible to one player right
// now, as protocol views. The cmd/server adapter installs a closure
// that reads the world's GroundItemManager.VisibleViews for the
// session's player and converts the game views to protocol views.
// It runs on the tick goroutine, from FlushTick's region-change
// block.
type GroundItemSource func() []protocol.GroundItemView

// GroundItemSync is one session's ground-item send state: the serial
// and the visible-item source. Construct it with NewGroundItemSync;
// the zero value sends nothing (no session attached).
type GroundItemSync struct {
	session *Session

	mu     sync.Mutex
	serial int
	source GroundItemSource
}

// NewGroundItemSync returns the ground-item send state for s. The
// serial starts at 0, as groundItemSerial does
// (PacketSender.ts:220); the first packet sent carries serial 1
// (every send pre-increments, as TS ++groundItemSerial does).
func NewGroundItemSync(s *Session) *GroundItemSync {
	return &GroundItemSync{session: s}
}

// SetGroundItemSource installs the visible-item source the
// region-change block reads. A nil source clears it, and
// SendVisibleGroundItems then sends nothing. The coordinator
// installs the adapter closure at AddPlayer time.
func (g *GroundItemSync) SetGroundItemSource(src GroundItemSource) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.source = src
}

// GroundItemSource returns the installed visible-item source, or nil
// when none is installed. The coordinator's FlushTick region-change
// call reads the source through this getter (or calls
// SendVisibleGroundItems, which reads it internally).
func (g *GroundItemSync) GroundItemSource() GroundItemSource {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.source
}

// SendGroundItems sends the full visible snapshot, chunked per
// PacketSender.sendGroundItems (PacketSender.ts:1451-1458): the
// first chunk goes as a full GROUND_ITEMS packet and the rest as
// GROUND_ITEMS_DELTA packets, each with its own pre-incremented
// serial. The first packet is always sent, even for an empty
// snapshot (TS slices unconditionally). Chunk boundaries follow the
// TS loop exactly: chunks of GroundItemsPerPacket starting at 0,
// N, 2N, ....
func (g *GroundItemSync) SendGroundItems(stacks []protocol.GroundItemView) {
	if g == nil || g.session == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	first := GroundItemsPerPacket
	if len(stacks) < first {
		first = len(stacks)
	}
	g.serial++
	g.session.SendPacket(protocol.EncodeGroundItems(g.serial, stacks[:first]))
	for i := GroundItemsPerPacket; i < len(stacks); i += GroundItemsPerPacket {
		end := i + GroundItemsPerPacket
		if end > len(stacks) {
			end = len(stacks)
		}
		g.serial++
		g.session.SendPacket(protocol.EncodeGroundItemsDelta(g.serial, stacks[i:end], nil))
	}
}

// SendGroundItemDelta sends one GROUND_ITEMS_DELTA packet with a
// pre-incremented serial, as PacketSender alter/delete/
// createGroundItem do (PacketSender.ts:1424-1449): upserts carry
// full views, removes carry stack ids. The manager's CREATE/ALTER
// fan-out calls this with one upsert and no removes; DELETE calls
// it with no upserts and one remove.
func (g *GroundItemSync) SendGroundItemDelta(upserts []protocol.GroundItemView, removes []int) {
	if g == nil || g.session == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.serial++
	g.session.SendPacket(protocol.EncodeGroundItemsDelta(g.serial, upserts, removes))
}

// SendVisibleGroundItems sends the current visible snapshot through
// the installed source, for the FlushTick region-change block
// (ItemOnGroundManager.onRegionChange,
// ItemOnGroundManager.ts:34-38, reached from
// PlayerSession.ts:384-390). With no source installed it sends
// nothing.
func (g *GroundItemSync) SendVisibleGroundItems() {
	src := g.GroundItemSource()
	if src == nil {
		return
	}
	g.SendGroundItems(src())
}

// GroundItems returns this session's ground-item send state, or nil
// for a zero-value Session built without NewSession (all
// GroundItemSync methods are nil-safe). The cmd/server adapter
// reaches the delta sends through it.
func (s *Session) GroundItems() *GroundItemSync {
	return s.groundItems
}
