package persist

import (
	"encoding/json"
	"reflect"
	"testing"
)

// strptr is the package helper in playersave.go.
func intptr(i int) *int { return &i }

func TestWithOwnedRelationsOverwrite(t *testing.T) {
	base := applyTestSave()
	clone := base.WithOwned(OwnedState{
		Position:               *base.Position,
		Running:                base.Running,
		RunEnergyUnits:         7750,
		Friends:                []string{"111", "222"},
		Ignores:                []string{"333"},
		FriendRanks:            map[string]int{"111": 6},
		FriendsChatChannelName: strptr("new-channel"),
		FriendsChatLastOwner:   strptr("new-owner"),
		FriendsChatEntryRank:   intptr(-1),
		FriendsChatTalkRank:    intptr(0),
		FriendsChatKickRank:    intptr(7),
	})
	if !reflect.DeepEqual(clone.Friends, []string{"111", "222"}) ||
		!reflect.DeepEqual(clone.Ignores, []string{"333"}) ||
		!reflect.DeepEqual(clone.FriendRanks, map[string]int{"111": 6}) ||
		clone.FriendsChatChannelName != "new-channel" ||
		clone.FriendsChatLastOwner != "new-owner" ||
		clone.FriendsChatEntryRank != -1 ||
		clone.FriendsChatTalkRank != 0 ||
		clone.FriendsChatKickRank != 7 {
		t.Fatalf("relations not overwritten: %+v", clone)
	}
	// The base is untouched.
	if !reflect.DeepEqual(base.Friends, []string{"12345"}) || base.FriendsChatChannelName != "channel" {
		t.Fatal("base mutated by WithOwned relations overwrite")
	}
}

func TestWithOwnedRelationsNilPreserves(t *testing.T) {
	// A partial OwnedState — no relations fields supplied —
	// preserves every carried relations field, including the
	// channel scalars (nil pointers preserve).
	base := applyTestSave()
	clone := base.WithOwned(OwnedState{
		Position: *base.Position, Running: base.Running, RunEnergyUnits: 7750,
	})
	if !reflect.DeepEqual(clone.Friends, base.Friends) ||
		!reflect.DeepEqual(clone.Ignores, base.Ignores) ||
		!reflect.DeepEqual(clone.FriendRanks, base.FriendRanks) ||
		clone.FriendsChatChannelName != base.FriendsChatChannelName ||
		clone.FriendsChatLastOwner != base.FriendsChatLastOwner ||
		clone.FriendsChatEntryRank != base.FriendsChatEntryRank ||
		clone.FriendsChatTalkRank != base.FriendsChatTalkRank ||
		clone.FriendsChatKickRank != base.FriendsChatKickRank {
		t.Fatal("partial OwnedState must preserve carried relations fields")
	}
}

func TestWithOwnedRelationsEmptyOverwrites(t *testing.T) {
	// Non-nil but empty relations state overwrites outright:
	// empty means "the player holds no friends", not preserve.
	// Likewise, pointers at "" and 0 overwrite with "" and 0.
	base := applyTestSave()
	clone := base.WithOwned(OwnedState{
		Position:               *base.Position,
		Running:                base.Running,
		RunEnergyUnits:         7750,
		Friends:                []string{},
		Ignores:                []string{},
		FriendRanks:            map[string]int{},
		FriendsChatChannelName: strptr(""),
		FriendsChatLastOwner:   strptr(""),
		FriendsChatEntryRank:   intptr(0),
		FriendsChatTalkRank:    intptr(0),
		FriendsChatKickRank:    intptr(0),
	})
	// Empty slices clone to nil (cloneStrings); load-time
	// normalization restores [] on the next load, so only the
	// length matters here. The map clones to a non-nil empty map.
	if len(clone.Friends) != 0 ||
		len(clone.Ignores) != 0 ||
		clone.FriendRanks == nil || len(clone.FriendRanks) != 0 ||
		clone.FriendsChatChannelName != "" ||
		clone.FriendsChatLastOwner != "" ||
		clone.FriendsChatEntryRank != 0 ||
		clone.FriendsChatTalkRank != 0 ||
		clone.FriendsChatKickRank != 0 {
		t.Fatalf("empty relations state did not overwrite: friends=%v ignores=%v ranks=%v channel=%q owner=%q ranks=%d/%d/%d",
			clone.Friends, clone.Ignores, clone.FriendRanks,
			clone.FriendsChatChannelName, clone.FriendsChatLastOwner,
			clone.FriendsChatEntryRank, clone.FriendsChatTalkRank, clone.FriendsChatKickRank)
	}
}

func TestWithOwnedRelationsCloneIndependence(t *testing.T) {
	// The clone shares no mutable relations state with the
	// supplied OwnedState: mutating the state afterwards
	// leaves the clone alone.
	base := applyTestSave()
	state := OwnedState{
		Position: *base.Position, Running: base.Running, RunEnergyUnits: 7750,
		Friends:     []string{"111"},
		FriendRanks: map[string]int{"111": 1},
	}
	clone := base.WithOwned(state)
	state.Friends[0] = "999"
	state.FriendRanks["111"] = 9
	if clone.Friends[0] != "111" || clone.FriendRanks["111"] != 1 {
		t.Fatalf("clone aliases supplied relations state: %v %v", clone.Friends, clone.FriendRanks)
	}
	// Mutating the clone's relations must not affect the base
	// when the state preserved (nil): the clone is a deep copy.
	preserved := base.WithOwned(OwnedState{Position: *base.Position, Running: base.Running})
	preserved.Friends[0] = "999"
	preserved.FriendRanks["12345"] = 9
	if base.Friends[0] != "12345" || base.FriendRanks["12345"] != 3 {
		t.Fatal("mutating the preserved clone affected the base")
	}
}

func TestOwnedStateRelationsRoundTrip(t *testing.T) {
	// Extraction + WithOwned reproduces the save's relations
	// fields exactly: a snapshot built from extracted state is
	// a no-op for relations.
	base := applyTestSave()
	clone := base.WithOwned(base.OwnedState())
	if !reflect.DeepEqual(clone.Friends, base.Friends) ||
		!reflect.DeepEqual(clone.Ignores, base.Ignores) ||
		!reflect.DeepEqual(clone.FriendRanks, base.FriendRanks) ||
		clone.FriendsChatChannelName != base.FriendsChatChannelName ||
		clone.FriendsChatLastOwner != base.FriendsChatLastOwner ||
		clone.FriendsChatEntryRank != base.FriendsChatEntryRank ||
		clone.FriendsChatTalkRank != base.FriendsChatTalkRank ||
		clone.FriendsChatKickRank != base.FriendsChatKickRank {
		t.Fatal("OwnedState round-trip changed relations fields")
	}
	// Extraction copies: mutating extracted lists must not
	// affect the save.
	state := base.OwnedState()
	state.Friends[0] = "999"
	state.FriendRanks["12345"] = 9
	*state.FriendsChatEntryRank = 9
	if base.Friends[0] != "12345" || base.FriendRanks["12345"] != 3 || base.FriendsChatEntryRank != 1 {
		t.Fatal("OwnedState extraction aliases the save's relations fields")
	}
}

// TestUnmarshalRankDefaultsAbsent pins the TS getter defaults for
// saves that omit the friends-chat rank fields (PlayerSave.ts:438-447).
func TestUnmarshalRankDefaultsAbsent(t *testing.T) {
	var s PlayerSave
	if err := json.Unmarshal([]byte(`{"displayName":"x"}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.FriendsChatEntryRank != -1 || s.FriendsChatTalkRank != -1 || s.FriendsChatKickRank != 2 {
		t.Errorf("absent ranks = %d/%d/%d, want -1/-1/2", s.FriendsChatEntryRank, s.FriendsChatTalkRank, s.FriendsChatKickRank)
	}
	var nul PlayerSave
	if err := json.Unmarshal([]byte(`{"friendsChatEntryRank":null,"friendsChatTalkRank":null,"friendsChatKickRank":null}`), &nul); err != nil {
		t.Fatalf("unmarshal nulls: %v", err)
	}
	if nul.FriendsChatEntryRank != -1 || nul.FriendsChatTalkRank != -1 || nul.FriendsChatKickRank != 2 {
		t.Errorf("null ranks = %d/%d/%d, want -1/-1/2", nul.FriendsChatEntryRank, nul.FriendsChatTalkRank, nul.FriendsChatKickRank)
	}
	var explicit PlayerSave
	if err := json.Unmarshal([]byte(`{"friendsChatEntryRank":0,"friendsChatTalkRank":3,"friendsChatKickRank":4}`), &explicit); err != nil {
		t.Fatalf("unmarshal explicit: %v", err)
	}
	if explicit.FriendsChatEntryRank != 0 || explicit.FriendsChatTalkRank != 3 || explicit.FriendsChatKickRank != 4 {
		t.Errorf("explicit ranks = %d/%d/%d, want 0/3/4", explicit.FriendsChatEntryRank, explicit.FriendsChatTalkRank, explicit.FriendsChatKickRank)
	}
}
