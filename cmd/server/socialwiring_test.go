package main

import (
	"log"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/persist"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

func TestEntryLoadsRelations(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	// The unparseable entry drops in the parse; the
	// non-positive entry and the duplicate drop in
	// LoadSaveState's re-normalization (a store-loaded save
	// would already be normalized by normalizeRelationList;
	// the harness passes the save directly, so the defensive
	// pass is what runs here).
	save.Friends = []string{"123", "456", "abc", "-5", "123"}
	save.Ignores = []string{"789"}
	// Only ranks for LISTED friends survive the load.
	save.FriendRanks = map[string]int{"123": 3, "999": 5}
	save.FriendsChatChannelName = "My Channel"
	save.FriendsChatLastOwner = "alice"
	save.FriendsChatEntryRank = 0
	save.FriendsChatTalkRank = 1
	save.FriendsChatKickRank = 3

	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", Save: save, PasswordHash: "h"})

	if p.LongUsername != session.NameToLong("Alice") {
		t.Fatalf("LongUsername = %d, want %d", p.LongUsername, session.NameToLong("Alice"))
	}
	got := p.Relations().SnapshotSaveState()
	want := game.RelationsSaveState{
		Friends:                []int64{123, 456},
		Ignores:                []int64{789},
		FriendRanks:            map[string]int{"123": 3},
		FriendsChatChannelName: "My Channel",
		FriendsChatLastOwner:   "alice",
		FriendsChatEntryRank:   0,
		FriendsChatTalkRank:    1,
		FriendsChatKickRank:    3,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("relations snapshot = %+v, want %+v", got, want)
	}
}

func TestEntryNewAccountRelationsDefaults(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Bob", Profile: "main", PasswordHash: "h"})

	if p.LongUsername != session.NameToLong("Bob") {
		t.Fatalf("LongUsername = %d, want %d", p.LongUsername, session.NameToLong("Bob"))
	}
	got := p.Relations().SnapshotSaveState()
	want := game.RelationsSaveState{
		Friends:              []int64{},
		Ignores:              []int64{},
		FriendRanks:          map[string]int{},
		FriendsChatEntryRank: -1,
		FriendsChatTalkRank:  -1,
		FriendsChatKickRank:  2,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("new-account relations snapshot = %+v, want %+v", got, want)
	}
}

func TestSnapshotWritesRelations(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.Friends = []string{"111"}
	save.FriendsChatChannelName = "Old"
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", Save: save, PasswordHash: "h"})

	// Mutate live state through the relations API, then
	// snapshot: the owned fields overwrite the carried ones.
	p.Relations().AddFriend(12345)
	p.Relations().AddIgnore(67890)
	p.Relations().SetFriendsChatChannelName("New Channel")
	p.Relations().SetFriendsChatLastOwner("alice")
	p.Relations().SetFriendsChatRanks(0, 1, 3)

	carried, _ := h.coord.carriedSave(p)
	snap, err := h.coord.buildSnapshot(p, carried)
	if err != nil {
		t.Fatalf("buildSnapshot: %v", err)
	}
	if !reflect.DeepEqual(snap.Friends, []string{"111", "12345"}) {
		t.Fatalf("snapshot Friends = %v, want [111 12345]", snap.Friends)
	}
	if !reflect.DeepEqual(snap.Ignores, []string{"67890"}) {
		t.Fatalf("snapshot Ignores = %v, want [67890]", snap.Ignores)
	}
	if !reflect.DeepEqual(snap.FriendRanks, map[string]int{"12345": 0}) {
		t.Fatalf("snapshot FriendRanks = %v, want {12345:0}", snap.FriendRanks)
	}
	if snap.FriendsChatChannelName != "New Channel" || snap.FriendsChatLastOwner != "alice" {
		t.Fatalf("snapshot channel = %q/%q, want New Channel/alice", snap.FriendsChatChannelName, snap.FriendsChatLastOwner)
	}
	if snap.FriendsChatEntryRank != 0 || snap.FriendsChatTalkRank != 1 || snap.FriendsChatKickRank != 3 {
		t.Fatalf("snapshot ranks = %d/%d/%d, want 0/1/3", snap.FriendsChatEntryRank, snap.FriendsChatTalkRank, snap.FriendsChatKickRank)
	}
}

func TestDisconnectSaveWritesRelations(t *testing.T) {
	// The full removal path: mutate relations, remove the
	// player (UpdateLists(false) runs in onPlayerDisconnect),
	// and the disconnect save carries the live relations state.
	h := newManualHarness(t, 1000, 500)
	p, index := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	p.Relations().AddFriend(12345)
	p.Relations().SetFriendsChatChannelName("Chan")

	h.remove(t, index)

	stored, err := h.store.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("Load after disconnect: %v", err)
	}
	if !reflect.DeepEqual(stored.Friends, []string{"12345"}) {
		t.Fatalf("stored Friends = %v, want [12345]", stored.Friends)
	}
	if stored.FriendsChatChannelName != "Chan" {
		t.Fatalf("stored channel = %q, want Chan", stored.FriendsChatChannelName)
	}
	// And a relogin loads it back.
	p2, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", Save: stored, PasswordHash: "h"})
	if got := p2.Relations().GetFriendList(); !reflect.DeepEqual(got, []int64{12345}) {
		t.Fatalf("relogin friend list = %v, want [12345]", got)
	}
}

func TestSocialEmitAdapter(t *testing.T) {
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "Watcher",
		Register: func(api *plugins.Api) {
			api.OnSocialPacket(func(e *plugins.SocialPacketEvent) {
				if e.Packet.Kind == plugins.SocialPacketChatFilter {
					e.Handled = true
				}
			})
		},
	}}, plugins.HubOptions{Logger: log.Default()})
	hub.Load()

	emit := socialEmitAdapter(hub)
	if emit == nil {
		t.Fatal("socialEmitAdapter(non-nil hub) = nil")
	}
	p := game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})
	pkt := game.SocialPacket{Kind: game.SocialPacketChatFilter, PublicMode: 0, PrivateMode: 1, TradeMode: 2}
	if !emit(p, pkt) {
		t.Error("emit(chat_filter) = false, want true (hook claims it)")
	}
	if emit(p, game.SocialPacket{Kind: game.SocialPacketReportAbuse, Name: "x"}) {
		t.Error("emit(report_abuse) = true, want false")
	}
	// The adapter passes the packet struct through unchanged:
	// a hook observing the event sees the exact packet.
	var seen *game.SocialPacket
	hub2 := plugins.NewHub([]plugins.Plugin{{
		Name: "Observer",
		Register: func(api *plugins.Api) {
			api.OnSocialPacket(func(e *plugins.SocialPacketEvent) {
				seen = &e.Packet
			})
		},
	}}, plugins.HubOptions{Logger: log.Default()})
	hub2.Load()
	socialEmitAdapter(hub2)(p, pkt)
	if seen == nil || !reflect.DeepEqual(*seen, pkt) {
		t.Fatalf("observed packet = %+v, want %+v", seen, pkt)
	}
	if socialEmitAdapter(nil) != nil {
		t.Error("socialEmitAdapter(nil hub) != nil")
	}
	var deps listeners.Deps
	installSocialListeners(&deps, nil)
	if deps.SocialEmit != nil {
		t.Error("installSocialListeners(nil hub) installed an emit")
	}
	installSocialListeners(&deps, hub)
	if deps.SocialEmit == nil {
		t.Error("installSocialListeners(hub) left SocialEmit nil")
	}
}

func TestShownNameFunc(t *testing.T) {
	if got := shownNameFunc(nil)("Some Name"); got != "Some Name" {
		t.Fatalf("nil registry shownName = %q, want input unchanged", got)
	}
	names, err := persist.LoadDisplayNames(filepath.Join(t.TempDir(), "display-names.json"))
	if err != nil {
		t.Fatalf("LoadDisplayNames: %v", err)
	}
	if err := names.Set("alice", "Alice Cooper", time.Now(), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	fn := shownNameFunc(names)
	if got := fn("alice"); got != "Alice Cooper" {
		t.Fatalf("shownName(alice) = %q, want Alice Cooper", got)
	}
	// No entry: the INPUT is returned unchanged — not the
	// formatted login name (NameOf would return "Bob Smith"
	// for "bob_smith"; shownName returns "bob_smith").
	if got := fn("bob_smith"); got != "bob_smith" {
		t.Fatalf("shownName(bob_smith) = %q, want input unchanged", got)
	}
}

func TestParseFormatRelationHashes(t *testing.T) {
	if got := parseRelationHashes([]string{"123", "abc", "-5", "123", ""}); !reflect.DeepEqual(got, []int64{123, -5, 123}) {
		t.Fatalf("parseRelationHashes = %v, want [123 -5 123]", got)
	}
	if got := parseRelationHashes(nil); got == nil || len(got) != 0 {
		t.Fatalf("parseRelationHashes(nil) = %v, want empty non-nil", got)
	}
	if got := formatRelationHashes([]int64{123, 456}); !reflect.DeepEqual(got, []string{"123", "456"}) {
		t.Fatalf("formatRelationHashes = %v, want [123 456]", got)
	}
	if got := formatRelationHashes(nil); got == nil || len(got) != 0 {
		t.Fatalf("formatRelationHashes(nil) = %v, want empty non-nil", got)
	}
	// Round trip through the save string form.
	if got := formatRelationHashes(parseRelationHashes([]string{"42"})); !reflect.DeepEqual(got, []string{"42"}) {
		t.Fatalf("round trip = %v, want [42]", got)
	}
}
