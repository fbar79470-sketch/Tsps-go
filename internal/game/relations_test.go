package game

import (
	"reflect"
	"strconv"
	"testing"
)

// Relations test doubles: a session recording every relations send
// (embedding fakeSession for SessionPort + ChatMessageSender), and a
// fake world implementing RelationsWorld over a fixed player list.

type privateMessageCall struct {
	sender  *Player
	message []byte
	size    int
}

type relationsFakeSession struct {
	*fakeSession
	friendStatus   []int
	friends        [][2]int64 // [name hash, world]
	deletedFriends []int64
	addedIgnores   []int64
	deletedIgnores []int64
	chatOptions    [][3]int // [public, private, trade]
	privateMsgs    []privateMessageCall
}

func newRelationsFakeSession() *relationsFakeSession {
	return &relationsFakeSession{fakeSession: newFakeSession()}
}

func (f *relationsFakeSession) SendFriendStatus(status int) {
	f.friendStatus = append(f.friendStatus, status)
}

func (f *relationsFakeSession) SendFriend(name int64, world int) {
	f.friends = append(f.friends, [2]int64{name, int64(world)})
}

func (f *relationsFakeSession) SendDeleteFriend(name int64) {
	f.deletedFriends = append(f.deletedFriends, name)
}

func (f *relationsFakeSession) SendAddIgnore(name int64) {
	f.addedIgnores = append(f.addedIgnores, name)
}

func (f *relationsFakeSession) SendDeleteIgnore(name int64) {
	f.deletedIgnores = append(f.deletedIgnores, name)
}

func (f *relationsFakeSession) SendChatOptions(publicChat, privateChat, tradeChat int) {
	f.chatOptions = append(f.chatOptions, [3]int{publicChat, privateChat, tradeChat})
}

func (f *relationsFakeSession) SendPrivateMessage(sender *Player, message []byte, size int) {
	f.privateMsgs = append(f.privateMsgs, privateMessageCall{sender: sender, message: append([]byte(nil), message...), size: size})
}

type fakeRelationsWorld struct {
	players []*Player
}

func (w *fakeRelationsWorld) RelationsPlayers() []*Player { return w.players }

func (w *fakeRelationsWorld) RelationsPlayerByName(name string) *Player {
	needle := formatNameForProtocol(formatDisplayName(name))
	for _, p := range w.players {
		if p != nil && formatNameForProtocol(p.Username) == needle {
			return p
		}
	}
	return nil
}

// newRelationsPlayer builds a player with its long username set from
// the username (as the login path sets it) and a recording relations
// session attached.
func newRelationsPlayer(username string) (*Player, *relationsFakeSession) {
	p := NewPlayer(username, Location{X: 3089, Y: 3524, Z: 0})
	p.LongUsername = StringToLong(username)
	s := newRelationsFakeSession()
	p.Session = s
	return p, s
}

// installRelationsWorld installs a fake world for the test and
// restores the previous one on cleanup.
func installRelationsWorld(t *testing.T, players ...*Player) {
	t.Helper()
	prev := relationsWorld
	SetRelationsWorld(&fakeRelationsWorld{players: players})
	t.Cleanup(func() { SetRelationsWorld(prev) })
}

// installNoRelationsWorld uninstalls the world seam for the test,
// restoring the previous one on cleanup.
func installNoRelationsWorld(t *testing.T) {
	t.Helper()
	prev := relationsWorld
	SetRelationsWorld(nil)
	t.Cleanup(func() { SetRelationsWorld(prev) })
}

// installShownName installs a shown-name lookup for the test and
// restores the previous one on cleanup.
func installShownName(t *testing.T, f func(string) string) {
	t.Helper()
	prev := shownNameFunc
	SetShownNameFunc(f)
	t.Cleanup(func() { SetShownNameFunc(prev) })
}

func TestRelationsDefaultsAndIDs(t *testing.T) {
	p, _ := newRelationsPlayer("alice")
	r := p.Relations()
	if r.GetStatus() != PrivateChatOn {
		t.Errorf("GetStatus = %v, want ON", r.GetStatus())
	}
	if r.GetPublicChatMode() != 0 || r.GetTradeChatMode() != 0 {
		t.Errorf("modes = %d/%d, want 0/0", r.GetPublicChatMode(), r.GetTradeChatMode())
	}
	if len(r.GetFriendList()) != 0 || len(r.GetIgnoreList()) != 0 {
		t.Error("fresh lists non-empty")
	}
	if len(r.GetFriendRanks()) != 0 {
		t.Errorf("GetFriendRanks = %v, want empty", r.GetFriendRanks())
	}
	if r.GetFriendsChatChannelName() != "" || r.GetFriendsChatLastOwner() != "" {
		t.Error("fresh channel fields non-empty")
	}
	if r.GetFriendsChatEntryRank() != -1 || r.GetFriendsChatTalkRank() != -1 || r.GetFriendsChatKickRank() != 2 {
		t.Errorf("channel ranks = %d/%d/%d, want -1/-1/2",
			r.GetFriendsChatEntryRank(), r.GetFriendsChatTalkRank(), r.GetFriendsChatKickRank())
	}
	// The private-message id counter starts at 1 and advances.
	if got := r.GetPrivateMessageId(); got != 1 {
		t.Errorf("first GetPrivateMessageId = %d, want 1", got)
	}
	if got := r.GetPrivateMessageId(); got != 2 {
		t.Errorf("second GetPrivateMessageId = %d, want 2", got)
	}
	if got := r.GetPrivateMessageId(); got != 3 {
		t.Errorf("third GetPrivateMessageId = %d, want 3", got)
	}
	if r.SetPrivateMessageId(10) != r {
		t.Error("SetPrivateMessageId did not return the relations object")
	}
	if got := r.GetPrivateMessageId(); got != 10 {
		t.Errorf("GetPrivateMessageId after set(10) = %d, want 10", got)
	}
	if got := r.GetPrivateMessageId(); got != 11 {
		t.Errorf("next GetPrivateMessageId = %d, want 11", got)
	}
}

func TestRelationsLazyAllocation(t *testing.T) {
	// NewPlayer constructs eagerly; repeated access returns the
	// same object.
	p, _ := newRelationsPlayer("alice")
	if p.Relations() != p.Relations() {
		t.Error("Relations() returned different objects")
	}
	// A zero-value Player gets one lazily, holding the player.
	var zp Player
	rel := zp.Relations()
	if rel == nil {
		t.Fatal("zero-value Player Relations() = nil")
	}
	if rel != zp.Relations() {
		t.Error("lazy Relations() returned different objects")
	}
	if rel.GetStatus() != PrivateChatOn {
		t.Errorf("lazy relations status = %v, want ON", rel.GetStatus())
	}
	// The lazy object holds zp: a self-add (by zp's hash) no-ops.
	zp.LongUsername = 5
	rel.AddFriend(5)
	if len(rel.GetFriendList()) != 0 {
		t.Error("lazy relations self-add added to the list")
	}
	// A nil player has no relations.
	var np *Player
	if np.Relations() != nil {
		t.Error("nil player Relations() non-nil")
	}
	if np.GetLongUsername() != 0 {
		t.Errorf("nil player GetLongUsername = %d, want 0", np.GetLongUsername())
	}
}

func TestLongUsernameField(t *testing.T) {
	p, _ := newRelationsPlayer("alice")
	if p.GetLongUsername() != StringToLong("alice") {
		t.Errorf("GetLongUsername = %d, want hash of alice", p.GetLongUsername())
	}
	// Default 0 until the login path sets it (Player.ts:151).
	fresh := NewPlayer("unset", Location{X: 3089, Y: 3524, Z: 0})
	if fresh.GetLongUsername() != 0 {
		t.Errorf("fresh GetLongUsername = %d, want 0", fresh.GetLongUsername())
	}
	fresh.LongUsername = 42
	if fresh.GetLongUsername() != 42 {
		t.Errorf("GetLongUsername after field set = %d, want 42", fresh.GetLongUsername())
	}
}

func TestSetChatModesClamping(t *testing.T) {
	cases := []struct {
		in                    [3]int
		wantPublic, wantTrade int
		wantStatus            PrivateChatStatus
	}{
		{[3]int{0, 0, 0}, 0, 0, PrivateChatOn},
		{[3]int{1, 1, 1}, 1, 1, PrivateChatFriendsOnly},
		{[3]int{2, 2, 2}, 2, 2, PrivateChatOff},
		{[3]int{3, 0, 0}, 3, 0, PrivateChatOn},
		{[3]int{4, 0, 0}, 4, 0, PrivateChatOn},
		// Clamps: public 0..4, private 0..2, trade 0..2.
		{[3]int{5, 5, 5}, 4, 2, PrivateChatOff},
		{[3]int{-1, -1, -1}, 0, 0, PrivateChatOn},
		{[3]int{99, -99, 99}, 4, 2, PrivateChatOn},
	}
	for _, tc := range cases {
		p, s := newRelationsPlayer("alice")
		p.Relations().SetChatModes(tc.in[0], tc.in[1], tc.in[2])
		r := p.Relations()
		if r.GetPublicChatMode() != tc.wantPublic || r.GetTradeChatMode() != tc.wantTrade || r.GetStatus() != tc.wantStatus {
			t.Errorf("SetChatModes(%v) = public %d trade %d status %v, want %d/%d/%v",
				tc.in, r.GetPublicChatMode(), r.GetTradeChatMode(), r.GetStatus(),
				tc.wantPublic, tc.wantTrade, tc.wantStatus)
		}
		// setChatModes never broadcasts (TS passes update=false).
		if len(s.friendStatus) != 0 || len(s.friends) != 0 {
			t.Errorf("SetChatModes(%v) sent presence traffic: %+v", tc.in, s)
		}
	}
}

func TestSetStatus(t *testing.T) {
	installNoRelationsWorld(t)
	p, s := newRelationsPlayer("alice")
	r := p.Relations()
	if r.SetStatus(PrivateChatFriendsOnly, false) != r {
		t.Error("SetStatus did not return the relations object")
	}
	if r.GetStatus() != PrivateChatFriendsOnly {
		t.Errorf("GetStatus = %v, want FRIENDS_ONLY", r.GetStatus())
	}
	if len(s.friendStatus) != 0 {
		t.Error("SetStatus(update=false) broadcast")
	}
	r.SetStatus(PrivateChatOff, true)
	if r.GetStatus() != PrivateChatOff {
		t.Errorf("GetStatus = %v, want OFF", r.GetStatus())
	}
	// update=true broadcasts: the self friend-status send.
	if len(s.friendStatus) != 1 || s.friendStatus[0] != 2 {
		t.Errorf("friendStatus = %v, want [2]", s.friendStatus)
	}
}

func TestFriendRank(t *testing.T) {
	p, _ := newRelationsPlayer("alice")
	bob := StringToLong("bob")
	r := p.Relations()
	if r.GetFriendRank(bob) != 0 {
		t.Errorf("GetFriendRank(non-friend) = %d, want 0", r.GetFriendRank(bob))
	}
	// False when not a friend, storing nothing.
	if r.SetFriendRank(bob, 3) {
		t.Error("SetFriendRank(non-friend) = true, want false")
	}
	if r.GetFriendRank(bob) != 0 {
		t.Errorf("GetFriendRank after refused set = %d, want 0", r.GetFriendRank(bob))
	}
	r.LoadSaveState(RelationsSaveState{Friends: []int64{bob}})
	if !r.SetFriendRank(bob, 3) {
		t.Error("SetFriendRank(friend) = false, want true")
	}
	if r.GetFriendRank(bob) != 3 {
		t.Errorf("GetFriendRank = %d, want 3", r.GetFriendRank(bob))
	}
	// Clamped to 0..6.
	r.SetFriendRank(bob, 99)
	if r.GetFriendRank(bob) != 6 {
		t.Errorf("GetFriendRank after set(99) = %d, want 6", r.GetFriendRank(bob))
	}
	r.SetFriendRank(bob, -3)
	if r.GetFriendRank(bob) != 0 {
		t.Errorf("GetFriendRank after set(-3) = %d, want 0", r.GetFriendRank(bob))
	}
}

func TestFriendRanksLoadAndGet(t *testing.T) {
	p, _ := newRelationsPlayer("alice")
	bob, carol := StringToLong("bob"), StringToLong("carol")
	dave := StringToLong("dave")
	r := p.Relations()
	r.LoadSaveState(RelationsSaveState{Friends: []int64{bob, carol}})
	r.LoadFriendRanks(map[string]int{
		strconv.FormatInt(bob, 10):   2,
		strconv.FormatInt(carol, 10): 9, // clamped to 6
		strconv.FormatInt(dave, 10):  5, // unlisted: dropped
		"not-a-hash":                 4, // unparseable: skipped
		"":                           4, // unparseable: skipped
		"9999999999999999999999":     4, // overflow: skipped
		"0x10":                       4, // hex: rejected (see LoadFriendRanks)
	})
	got := r.GetFriendRanks()
	want := map[string]int{
		strconv.FormatInt(bob, 10):   2,
		strconv.FormatInt(carol, 10): 6,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetFriendRanks = %v, want %v", got, want)
	}
	// Keys with surrounding whitespace and a leading "+" parse,
	// as TS BigInt does.
	r.LoadFriendRanks(map[string]int{
		"  " + strconv.FormatInt(bob, 10) + " ": 1,
		"+" + strconv.FormatInt(carol, 10):      3,
	})
	if r.GetFriendRank(bob) != 1 || r.GetFriendRank(carol) != 3 {
		t.Errorf("ranks after whitespace/+ load = %d/%d, want 1/3", r.GetFriendRank(bob), r.GetFriendRank(carol))
	}
	// A nil map clears and stores nothing (TS null/undefined).
	r.LoadFriendRanks(nil)
	if len(r.GetFriendRanks()) != 0 {
		t.Errorf("GetFriendRanks after nil load = %v, want empty", r.GetFriendRanks())
	}
	// getFriendRanks omits ranks whose friend left the list, and
	// rebuildRelationSets (via UpdateLists) prunes them.
	r.LoadSaveState(RelationsSaveState{Friends: []int64{bob}})
	r.SetFriendRank(bob, 4)
	r.LoadSaveState(RelationsSaveState{Friends: []int64{carol}})
	if len(r.GetFriendRanks()) != 0 {
		t.Errorf("GetFriendRanks after friend removed = %v, want empty", r.GetFriendRanks())
	}
}

func TestFriendsChatFields(t *testing.T) {
	p, _ := newRelationsPlayer("alice")
	r := p.Relations()
	r.SetFriendsChatChannelName("Cool Channel")
	r.SetFriendsChatLastOwner("alice")
	if r.GetFriendsChatChannelName() != "Cool Channel" || r.GetFriendsChatLastOwner() != "alice" {
		t.Errorf("channel fields = %q/%q, want Cool Channel/alice",
			r.GetFriendsChatChannelName(), r.GetFriendsChatLastOwner())
	}
	cases := []struct {
		in                            [3]int
		wantEntry, wantTalk, wantKick int
	}{
		{[3]int{0, 0, 2}, 0, 0, 2},
		{[3]int{7, 7, 7}, 7, 7, 7},
		{[3]int{-1, -1, 2}, -1, -1, 2},
		// Entry/talk clamp -1..7, kick clamps 2..7.
		{[3]int{-5, 9, 0}, -1, 7, 2},
		{[3]int{99, -99, 99}, 7, -1, 7},
	}
	for _, tc := range cases {
		r.SetFriendsChatRanks(tc.in[0], tc.in[1], tc.in[2])
		if r.GetFriendsChatEntryRank() != tc.wantEntry || r.GetFriendsChatTalkRank() != tc.wantTalk || r.GetFriendsChatKickRank() != tc.wantKick {
			t.Errorf("SetFriendsChatRanks(%v) = %d/%d/%d, want %d/%d/%d", tc.in,
				r.GetFriendsChatEntryRank(), r.GetFriendsChatTalkRank(), r.GetFriendsChatKickRank(),
				tc.wantEntry, tc.wantTalk, tc.wantKick)
		}
	}
}

func TestPublicChatGateTruthTable(t *testing.T) {
	// Receiver modes x autoChat x sender relation, on the
	// relations method itself.
	receiver, _ := newRelationsPlayer("alice")
	sender, _ := newRelationsPlayer("bob")
	stranger, _ := newRelationsPlayer("carol")
	r := receiver.Relations()
	type relation int
	const (
		neither relation = iota
		friend
		ignored
	)
	// want[mode][autoChat][relation], per PlayerRelations.ts:90-96.
	want := map[int]map[bool]map[relation]bool{
		0: {false: {neither: true, friend: true, ignored: false}, true: {neither: false, friend: false, ignored: false}},
		1: {false: {neither: false, friend: true, ignored: false}, true: {neither: false, friend: false, ignored: false}},
		2: {false: {neither: false, friend: false, ignored: false}, true: {neither: false, friend: false, ignored: false}},
		3: {false: {neither: false, friend: false, ignored: false}, true: {neither: false, friend: false, ignored: false}},
		// Mode 4 accepts everything, including autochat — except
		// from ignored senders, who never deliver.
		4: {false: {neither: true, friend: true, ignored: false}, true: {neither: true, friend: true, ignored: false}},
	}
	for mode := 0; mode <= 4; mode++ {
		for _, autoChat := range []bool{false, true} {
			for _, rel := range []relation{neither, friend, ignored} {
				r.LoadSaveState(RelationsSaveState{})
				switch rel {
				case friend:
					r.LoadSaveState(RelationsSaveState{Friends: []int64{sender.GetLongUsername()}})
				case ignored:
					r.LoadSaveState(RelationsSaveState{Ignores: []int64{sender.GetLongUsername()}})
				}
				r.SetChatModes(mode, 0, 0)
				if got := r.CanReceivePublicChatFrom(sender, autoChat); got != want[mode][autoChat][rel] {
					t.Errorf("mode %d autoChat=%v relation=%d: got %v, want %v",
						mode, autoChat, rel, got, want[mode][autoChat][rel])
				}
			}
		}
	}
	// A nil sender never delivers, in any mode (including 4).
	r.LoadSaveState(RelationsSaveState{})
	r.SetChatModes(4, 0, 0)
	if r.CanReceivePublicChatFrom(nil, false) || r.CanReceivePublicChatFrom(nil, true) {
		t.Error("nil sender received in mode 4")
	}
	// The stranger/friend distinction is by hash, not identity:
	// the stranger is neither friended nor ignored.
	r.SetChatModes(1, 0, 0)
	r.LoadSaveState(RelationsSaveState{Friends: []int64{sender.GetLongUsername()}})
	if r.CanReceivePublicChatFrom(stranger, false) {
		t.Error("stranger received in mode 1")
	}
}

func TestVisibilityAndPrivateMessageGates(t *testing.T) {
	receiver, _ := newRelationsPlayer("alice")
	sender, _ := newRelationsPlayer("bob")
	r := receiver.Relations()
	senderHash := sender.GetLongUsername()

	// IsVisibleTo: ON visible to all, FRIENDS_ONLY to friends,
	// OFF to none — independently of the ignore list.
	// CanReceivePrivateMessageFrom: visible AND not ignored.
	cases := []struct {
		status      PrivateChatStatus
		friend      bool
		ignored     bool
		wantVisible bool
		wantPM      bool
	}{
		{PrivateChatOn, false, false, true, true},
		{PrivateChatOn, true, false, true, true},
		{PrivateChatOn, false, true, true, false}, // presence ignores the ignore list
		{PrivateChatOn, true, true, true, false},  // friend+ignored: visible, but no PM
		{PrivateChatFriendsOnly, false, false, false, false},
		{PrivateChatFriendsOnly, true, false, true, true},
		{PrivateChatFriendsOnly, false, true, false, false},
		{PrivateChatFriendsOnly, true, true, true, false},
		{PrivateChatOff, false, false, false, false},
		{PrivateChatOff, true, false, false, false},
		{PrivateChatOff, false, true, false, false},
		{PrivateChatOff, true, true, false, false},
	}
	for _, tc := range cases {
		state := RelationsSaveState{}
		if tc.friend {
			state.Friends = []int64{senderHash}
		}
		if tc.ignored {
			state.Ignores = []int64{senderHash}
		}
		r.LoadSaveState(state)
		r.SetStatus(tc.status, false)
		if got := r.IsVisibleTo(sender); got != tc.wantVisible {
			t.Errorf("status %v friend=%v ignored=%v: IsVisibleTo = %v, want %v",
				tc.status, tc.friend, tc.ignored, got, tc.wantVisible)
		}
		if got := r.CanReceivePrivateMessageFrom(sender); got != tc.wantPM {
			t.Errorf("status %v friend=%v ignored=%v: CanReceivePrivateMessageFrom = %v, want %v",
				tc.status, tc.friend, tc.ignored, got, tc.wantPM)
		}
	}
	// Nil other: never visible, never receivable.
	r.SetStatus(PrivateChatOn, false)
	if r.IsVisibleTo(nil) || r.CanReceivePrivateMessageFrom(nil) {
		t.Error("nil other visible/receivable")
	}
}

func TestPlayerCanReceivePublicChatFromLive(t *testing.T) {
	// The Player-level gate delegates to live relations (chat.go):
	// the Phase 12 degeneration (!autoChat) is gone.
	p, _ := newRelationsPlayer("alice")
	other, _ := newRelationsPlayer("bob")
	// Fresh-player defaults still agree with the old degeneration.
	if !p.CanReceivePublicChatFrom(other, false) {
		t.Error("fresh standard chat rejected")
	}
	if p.CanReceivePublicChatFrom(other, true) {
		t.Error("fresh autochat received")
	}
	// Mode 1: friends only.
	p.Relations().SetChatModes(1, 0, 0)
	if p.CanReceivePublicChatFrom(other, false) {
		t.Error("non-friend received in mode 1")
	}
	p.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{other.GetLongUsername()}})
	if !p.CanReceivePublicChatFrom(other, false) {
		t.Error("friend rejected in mode 1")
	}
	// Ignored: never, even in mode 4.
	p.Relations().LoadSaveState(RelationsSaveState{Ignores: []int64{other.GetLongUsername()}})
	p.Relations().SetChatModes(4, 0, 0)
	if p.CanReceivePublicChatFrom(other, false) || p.CanReceivePublicChatFrom(other, true) {
		t.Error("ignored sender received in mode 4")
	}
	// Mode 4: autochat from a stranger arrives.
	p.Relations().LoadSaveState(RelationsSaveState{})
	if !p.CanReceivePublicChatFrom(other, true) {
		t.Error("autochat rejected in mode 4")
	}
	// Nil receiver and nil sender never deliver.
	var np *Player
	if np.CanReceivePublicChatFrom(other, false) {
		t.Error("nil receiver received")
	}
	if p.CanReceivePublicChatFrom(nil, false) {
		t.Error("nil sender received")
	}
}

func TestAddFriend(t *testing.T) {
	t.Run("success with peer online", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, fb := newRelationsPlayer("bob")
		installRelationsWorld(t, a, b)
		a.Relations().AddFriend(b.GetLongUsername())
		r := a.Relations()
		if !r.HasFriend(b.GetLongUsername()) || len(r.GetFriendList()) != 1 {
			t.Errorf("friend list = %v, want [bob]", r.GetFriendList())
		}
		if r.GetFriendRank(b.GetLongUsername()) != 0 {
			t.Errorf("new friend rank = %d, want 0", r.GetFriendRank(b.GetLongUsername()))
		}
		// SendAddFriend (world 0), then (bob,1) from each
		// UpdateLists: a's own and b's peer broadcast.
		wantFriends := [][2]int64{
			{b.GetLongUsername(), 0},
			{b.GetLongUsername(), 1},
			{b.GetLongUsername(), 1},
		}
		if !reflect.DeepEqual(fa.friends, wantFriends) {
			t.Errorf("a friends sends = %v, want %v", fa.friends, wantFriends)
		}
		if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
			t.Errorf("a friendStatus = %v, want [2]", fa.friendStatus)
		}
		// The peer broadcast: b's own UpdateLists(true) sends
		// b its friend status.
		if !reflect.DeepEqual(fb.friendStatus, []int{2}) {
			t.Errorf("b friendStatus = %v, want [2]", fb.friendStatus)
		}
		if len(fa.messages) != 0 {
			t.Errorf("messages = %v, want none", fa.messages)
		}
	})

	t.Run("peer offline", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		installRelationsWorld(t, a) // b not registered
		a.Relations().AddFriend(b.GetLongUsername())
		if !a.Relations().HasFriend(b.GetLongUsername()) {
			t.Error("friend not added when peer offline")
		}
		// Only the add send and the self friend status: no peer
		// update, and no (bob,1) refresh (b is not iterated).
		wantFriends := [][2]int64{{b.GetLongUsername(), 0}}
		if !reflect.DeepEqual(fa.friends, wantFriends) {
			t.Errorf("friends sends = %v, want %v", fa.friends, wantFriends)
		}
		if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
			t.Errorf("friendStatus = %v, want [2]", fa.friendStatus)
		}
	})

	t.Run("no world installed", func(t *testing.T) {
		installNoRelationsWorld(t)
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		a.Relations().AddFriend(b.GetLongUsername())
		if !a.Relations().HasFriend(b.GetLongUsername()) {
			t.Error("friend not added with no world")
		}
		if !reflect.DeepEqual(fa.friends, [][2]int64{{b.GetLongUsername(), 0}}) {
			t.Errorf("friends sends = %v, want only the add", fa.friends)
		}
	})

	t.Run("self add is a silent no-op", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		installRelationsWorld(t, a)
		a.Relations().AddFriend(a.GetLongUsername())
		if len(a.Relations().GetFriendList()) != 0 {
			t.Error("self-add added to the list")
		}
		if len(fa.messages) != 0 || len(fa.friends) != 0 || len(fa.friendStatus) != 0 {
			t.Errorf("self-add produced traffic: messages=%v friends=%v status=%v",
				fa.messages, fa.friends, fa.friendStatus)
		}
	})

	t.Run("duplicate notices", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		installRelationsWorld(t, a, b)
		a.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{b.GetLongUsername()}})
		a.Relations().AddFriend(b.GetLongUsername())
		if len(a.Relations().GetFriendList()) != 1 {
			t.Errorf("list = %v, want single entry", a.Relations().GetFriendList())
		}
		if !reflect.DeepEqual(fa.messages, []string{"Bob is already on your friends list!"}) {
			t.Errorf("messages = %v, want the duplicate notice", fa.messages)
		}
		if len(fa.friends) != 0 {
			t.Errorf("duplicate add sent friends traffic: %v", fa.friends)
		}
	})

	t.Run("duplicate uses shown name", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		installShownName(t, func(username string) string { return "Sir " + username })
		a.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{b.GetLongUsername()}})
		a.Relations().AddFriend(b.GetLongUsername())
		if !reflect.DeepEqual(fa.messages, []string{"Sir Bob is already on your friends list!"}) {
			t.Errorf("messages = %v, want the shown-name notice", fa.messages)
		}
	})

	t.Run("ignored first", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		a.Relations().LoadSaveState(RelationsSaveState{Ignores: []int64{b.GetLongUsername()}})
		a.Relations().AddFriend(b.GetLongUsername())
		if a.Relations().HasFriend(b.GetLongUsername()) {
			t.Error("ignored player added as friend")
		}
		if !reflect.DeepEqual(fa.messages, []string{"Please remove Bob from your ignore list first."}) {
			t.Errorf("messages = %v, want the ignore-first refusal", fa.messages)
		}
	})

	t.Run("ignore check precedes duplicate check", func(t *testing.T) {
		// Both lists holding the hash is impossible through the
		// API but reachable through a save; TS checks ignore
		// first (PlayerRelations.ts:276-283).
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		a.Relations().LoadSaveState(RelationsSaveState{
			Friends: []int64{b.GetLongUsername()},
			Ignores: []int64{b.GetLongUsername()},
		})
		a.Relations().AddFriend(b.GetLongUsername())
		if !reflect.DeepEqual(fa.messages, []string{"Please remove Bob from your ignore list first."}) {
			t.Errorf("messages = %v, want the ignore-first refusal", fa.messages)
		}
	})

	t.Run("full list refuses", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		friends := make([]int64, 0, MaxFriends)
		for i := int64(1); i <= MaxFriends; i++ {
			friends = append(friends, i)
		}
		a.Relations().LoadSaveState(RelationsSaveState{Friends: friends})
		a.Relations().AddFriend(999999)
		if len(a.Relations().GetFriendList()) != MaxFriends {
			t.Errorf("list length = %d, want %d", len(a.Relations().GetFriendList()), MaxFriends)
		}
		if !reflect.DeepEqual(fa.messages, []string{"Your friend list is full!"}) {
			t.Errorf("messages = %v, want the full refusal", fa.messages)
		}
	})
}

func TestDeleteFriend(t *testing.T) {
	t.Run("present removes and broadcasts", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, fb := newRelationsPlayer("bob")
		installRelationsWorld(t, a, b)
		a.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{b.GetLongUsername()}})
		a.Relations().SetFriendRank(b.GetLongUsername(), 3)
		a.Relations().DeleteFriend(b.GetLongUsername())
		r := a.Relations()
		if len(r.GetFriendList()) != 0 || r.HasFriend(b.GetLongUsername()) {
			t.Errorf("list = %v, want empty", r.GetFriendList())
		}
		if r.GetFriendRank(b.GetLongUsername()) != 0 || len(r.GetFriendRanks()) != 0 {
			t.Error("rank survived deletion")
		}
		if !reflect.DeepEqual(fa.deletedFriends, []int64{b.GetLongUsername()}) {
			t.Errorf("deletedFriends = %v, want [bob]", fa.deletedFriends)
		}
		if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
			t.Errorf("a friendStatus = %v, want [2]", fa.friendStatus)
		}
		// The peer broadcasts with UpdateLists(false): its own
		// friend status goes out.
		if !reflect.DeepEqual(fb.friendStatus, []int{2}) {
			t.Errorf("b friendStatus = %v, want [2]", fb.friendStatus)
		}
		if len(fa.messages) != 0 {
			t.Errorf("messages = %v, want none", fa.messages)
		}
	})

	t.Run("absent notices", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		a.Relations().DeleteFriend(b.GetLongUsername())
		if !reflect.DeepEqual(fa.messages, []string{"This player is not on your friends list!"}) {
			t.Errorf("messages = %v, want the not-listed notice", fa.messages)
		}
		if len(fa.deletedFriends) != 0 || len(fa.friendStatus) != 0 {
			t.Error("absent delete sent traffic")
		}
	})

	t.Run("self delete is a silent no-op", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		a.Relations().DeleteFriend(a.GetLongUsername())
		if len(fa.messages) != 0 || len(fa.deletedFriends) != 0 {
			t.Errorf("self-delete produced traffic: %v %v", fa.messages, fa.deletedFriends)
		}
	})
}

func TestAddIgnore(t *testing.T) {
	t.Run("success with peer online", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, fb := newRelationsPlayer("bob")
		installRelationsWorld(t, a, b)
		a.Relations().AddIgnore(b.GetLongUsername())
		if !a.Relations().HasIgnore(b.GetLongUsername()) {
			t.Error("ignore not added")
		}
		if !reflect.DeepEqual(fa.addedIgnores, []int64{b.GetLongUsername()}) {
			t.Errorf("addedIgnores = %v, want [bob]", fa.addedIgnores)
		}
		if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
			t.Errorf("a friendStatus = %v, want [2]", fa.friendStatus)
		}
		if !reflect.DeepEqual(fb.friendStatus, []int{2}) {
			t.Errorf("b friendStatus = %v, want [2]", fb.friendStatus)
		}
	})

	t.Run("friend first", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		a.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{b.GetLongUsername()}})
		a.Relations().AddIgnore(b.GetLongUsername())
		if a.Relations().HasIgnore(b.GetLongUsername()) {
			t.Error("friend added to ignore list")
		}
		if !reflect.DeepEqual(fa.messages, []string{"Please remove Bob from your friend list first."}) {
			t.Errorf("messages = %v, want the friend-first refusal", fa.messages)
		}
	})

	t.Run("duplicate notices", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		a.Relations().LoadSaveState(RelationsSaveState{Ignores: []int64{b.GetLongUsername()}})
		a.Relations().AddIgnore(b.GetLongUsername())
		if len(a.Relations().GetIgnoreList()) != 1 {
			t.Errorf("list = %v, want single entry", a.Relations().GetIgnoreList())
		}
		if !reflect.DeepEqual(fa.messages, []string{"Bob is already on your ignore list!"}) {
			t.Errorf("messages = %v, want the duplicate notice", fa.messages)
		}
	})

	t.Run("full list refuses", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		ignores := make([]int64, 0, MaxIgnores)
		for i := int64(1); i <= MaxIgnores; i++ {
			ignores = append(ignores, i)
		}
		a.Relations().LoadSaveState(RelationsSaveState{Ignores: ignores})
		a.Relations().AddIgnore(999999)
		if len(a.Relations().GetIgnoreList()) != MaxIgnores {
			t.Errorf("list length = %d, want %d", len(a.Relations().GetIgnoreList()), MaxIgnores)
		}
		if !reflect.DeepEqual(fa.messages, []string{"Your ignore list is full!"}) {
			t.Errorf("messages = %v, want the full refusal", fa.messages)
		}
	})

	t.Run("self add is a silent no-op", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		a.Relations().AddIgnore(a.GetLongUsername())
		if len(a.Relations().GetIgnoreList()) != 0 || len(fa.messages) != 0 {
			t.Error("self-ignore produced state or traffic")
		}
	})
}

func TestDeleteIgnore(t *testing.T) {
	t.Run("present removes and broadcasts when ON", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, fb := newRelationsPlayer("bob")
		installRelationsWorld(t, a, b)
		a.Relations().LoadSaveState(RelationsSaveState{Ignores: []int64{b.GetLongUsername()}})
		a.Relations().DeleteIgnore(b.GetLongUsername())
		if len(a.Relations().GetIgnoreList()) != 0 {
			t.Errorf("list = %v, want empty", a.Relations().GetIgnoreList())
		}
		if !reflect.DeepEqual(fa.deletedIgnores, []int64{b.GetLongUsername()}) {
			t.Errorf("deletedIgnores = %v, want [bob]", fa.deletedIgnores)
		}
		if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
			t.Errorf("a friendStatus = %v, want [2]", fa.friendStatus)
		}
		if !reflect.DeepEqual(fb.friendStatus, []int{2}) {
			t.Errorf("b friendStatus = %v, want [2]", fb.friendStatus)
		}
	})

	t.Run("peer update gated on status ON", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, fb := newRelationsPlayer("bob")
		installRelationsWorld(t, a, b)
		a.Relations().LoadSaveState(RelationsSaveState{Ignores: []int64{b.GetLongUsername()}})
		a.Relations().SetStatus(PrivateChatOff, false)
		a.Relations().DeleteIgnore(b.GetLongUsername())
		if len(a.Relations().GetIgnoreList()) != 0 {
			t.Error("ignore not removed while OFF")
		}
		if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
			t.Errorf("a friendStatus = %v, want [2]", fa.friendStatus)
		}
		// The peer hears nothing: DeleteIgnore's peer update
		// runs only when the status is ON.
		if len(fb.friendStatus) != 0 || len(fb.friends) != 0 {
			t.Errorf("peer heard traffic while OFF: status=%v friends=%v", fb.friendStatus, fb.friends)
		}
	})

	t.Run("absent notices", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, _ := newRelationsPlayer("bob")
		a.Relations().DeleteIgnore(b.GetLongUsername())
		if !reflect.DeepEqual(fa.messages, []string{"This player is not on your ignore list!"}) {
			t.Errorf("messages = %v, want the not-listed notice", fa.messages)
		}
	})

	t.Run("self delete is a silent no-op", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		a.Relations().DeleteIgnore(a.GetLongUsername())
		if len(fa.messages) != 0 || len(fa.deletedIgnores) != 0 {
			t.Error("self-delete produced traffic")
		}
	})
}

// updateListsFixture builds the broadcast-matrix world: b lists a, a
// lists c, d is unrelated. All statuses ON.
func updateListsFixture(t *testing.T) (a, b, c, d *Player, fa, fb, fc, fd *relationsFakeSession) {
	t.Helper()
	a, fa = newRelationsPlayer("alice")
	b, fb = newRelationsPlayer("bob")
	c, fc = newRelationsPlayer("carol")
	d, fd = newRelationsPlayer("dave")
	b.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{a.GetLongUsername()}})
	a.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{c.GetLongUsername()}})
	installRelationsWorld(t, a, b, c, d)
	return a, b, c, d, fa, fb, fc, fd
}

func TestUpdateListsBroadcast(t *testing.T) {
	t.Run("online both directions", func(t *testing.T) {
		a, b, c, d, fa, fb, fc, fd := updateListsFixture(t)
		a.Relations().UpdateLists(true)
		if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
			t.Errorf("a friendStatus = %v, want [2]", fa.friendStatus)
		}
		// b listed a and a is visible: b sees a online.
		if !reflect.DeepEqual(fb.friends, [][2]int64{{a.GetLongUsername(), 1}}) {
			t.Errorf("b friends = %v, want [(a,1)]", fb.friends)
		}
		// a listed c and c is visible: a sees c online.
		if !reflect.DeepEqual(fa.friends, [][2]int64{{c.GetLongUsername(), 1}}) {
			t.Errorf("a friends = %v, want [(c,1)]", fa.friends)
		}
		if len(fc.friends) != 0 || len(fd.friends) != 0 {
			t.Errorf("c/d heard friend traffic: %v %v", fc.friends, fd.friends)
		}
		if len(fb.friendStatus) != 0 || len(fc.friendStatus) != 0 || len(fd.friendStatus) != 0 {
			t.Error("non-owner received friend status")
		}
		_ = b
		_ = d
	})

	t.Run("owner OFF forces offline outward only", func(t *testing.T) {
		a, _, c, _, fa, fb, _, _ := updateListsFixture(t)
		a.Relations().SetStatus(PrivateChatOff, false)
		a.Relations().UpdateLists(true)
		// b sees a offline, but a still sees c online: the
		// forced-offline flag binds only the outward term.
		if !reflect.DeepEqual(fb.friends, [][2]int64{{a.GetLongUsername(), 0}}) {
			t.Errorf("b friends = %v, want [(a,0)]", fb.friends)
		}
		if !reflect.DeepEqual(fa.friends, [][2]int64{{c.GetLongUsername(), 1}}) {
			t.Errorf("a friends = %v, want [(c,1)]", fa.friends)
		}
		if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
			t.Errorf("a friendStatus = %v, want [2]", fa.friendStatus)
		}
	})

	t.Run("listed player OFF shows offline", func(t *testing.T) {
		a, _, c, _, fa, fb, _, _ := updateListsFixture(t)
		c.Relations().SetStatus(PrivateChatOff, false)
		a.Relations().UpdateLists(true)
		if !reflect.DeepEqual(fa.friends, [][2]int64{{c.GetLongUsername(), 0}}) {
			t.Errorf("a friends = %v, want [(c,0)]", fa.friends)
		}
		if !reflect.DeepEqual(fb.friends, [][2]int64{{a.GetLongUsername(), 1}}) {
			t.Errorf("b friends = %v, want [(a,1)]", fb.friends)
		}
	})

	t.Run("friends-only visibility is mutual-list aware", func(t *testing.T) {
		a, b, _, _, _, fb, _, _ := updateListsFixture(t)
		// a is FRIENDS_ONLY and does not list b: b sees a offline.
		a.Relations().SetStatus(PrivateChatFriendsOnly, false)
		a.Relations().UpdateLists(true)
		if !reflect.DeepEqual(fb.friends, [][2]int64{{a.GetLongUsername(), 0}}) {
			t.Errorf("b friends = %v, want [(a,0)]", fb.friends)
		}
		// a lists b too: b sees a online.
		a.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{
			StringToLong("carol"), b.GetLongUsername(),
		}})
		fb.friends = nil
		a.Relations().UpdateLists(true)
		if !reflect.DeepEqual(fb.friends, [][2]int64{{a.GetLongUsername(), 1}}) {
			t.Errorf("b friends after mutual list = %v, want [(a,1)]", fb.friends)
		}
	})

	t.Run("offline broadcast", func(t *testing.T) {
		a, _, _, _, fa, fb, _, _ := updateListsFixture(t)
		a.Relations().UpdateLists(false)
		if !reflect.DeepEqual(fb.friends, [][2]int64{{a.GetLongUsername(), 0}}) {
			t.Errorf("b friends = %v, want [(a,0)]", fb.friends)
		}
		// The inward term is unaffected by the online flag: a
		// still refreshes its view of c as online.
		if !reflect.DeepEqual(fa.friends, [][2]int64{{StringToLong("carol"), 1}}) {
			t.Errorf("a friends = %v, want [(carol,1)]", fa.friends)
		}
	})

	t.Run("nil world entry skipped", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		b, fb := newRelationsPlayer("bob")
		b.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{a.GetLongUsername()}})
		installRelationsWorld(t, nil, a, nil, b, nil)
		a.Relations().UpdateLists(true) // must not panic
		if !reflect.DeepEqual(fb.friends, [][2]int64{{a.GetLongUsername(), 1}}) {
			t.Errorf("b friends = %v, want [(a,1)]", fb.friends)
		}
		if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
			t.Errorf("a friendStatus = %v, want [2]", fa.friendStatus)
		}
	})

	t.Run("returns the relations object", func(t *testing.T) {
		a, _, _, _, _, _, _, _ := updateListsFixture(t)
		if a.Relations().UpdateLists(true) != a.Relations() {
			t.Error("UpdateLists did not return the relations object")
		}
	})
}

func TestUpdateListsNoWorld(t *testing.T) {
	installNoRelationsWorld(t)
	a, fa := newRelationsPlayer("alice")
	if a.Relations().UpdateLists(true) != a.Relations() {
		t.Error("UpdateLists did not return the relations object")
	}
	// Self-only degradation: the friend status goes out, nothing else.
	if !reflect.DeepEqual(fa.friendStatus, []int{2}) {
		t.Errorf("friendStatus = %v, want [2]", fa.friendStatus)
	}
	if len(fa.friends) != 0 {
		t.Errorf("friends = %v, want none", fa.friends)
	}
}

func TestOnLoginSends(t *testing.T) {
	a, fa := newRelationsPlayer("alice")
	b, c, d := StringToLong("bob"), StringToLong("carol"), StringToLong("dave")
	a.Relations().LoadSaveState(RelationsSaveState{
		Friends: []int64{b, c},
		Ignores: []int64{d},
	})
	a.Relations().SetChatModes(2, 1, 1)
	// A stale rank for an unlisted friend: OnLogin rebuilds the
	// sets and prunes it.
	a.Relations().friendRanks[StringToLong("erin")] = 3
	if a.Relations().OnLogin() != a.Relations() {
		t.Error("OnLogin did not return the relations object")
	}
	if len(a.Relations().GetFriendRanks()) != 0 {
		t.Errorf("stale rank survived OnLogin: %v", a.Relations().GetFriendRanks())
	}
	if !reflect.DeepEqual(fa.addedIgnores, []int64{d}) {
		t.Errorf("addedIgnores = %v, want [dave]", fa.addedIgnores)
	}
	wantFriends := [][2]int64{{b, 0}, {c, 0}}
	if !reflect.DeepEqual(fa.friends, wantFriends) {
		t.Errorf("friends = %v, want %v", fa.friends, wantFriends)
	}
	// Chat options: public 2, FRIENDS_ONLY maps to 1, trade 1.
	if !reflect.DeepEqual(fa.chatOptions, [][3]int{{2, 1, 1}}) {
		t.Errorf("chatOptions = %v, want [[2 1 1]]", fa.chatOptions)
	}
}

func TestSendFriendsIgnoresPrivateStatus(t *testing.T) {
	t.Run("send friends and ignores skip zero hashes", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		r := a.Relations()
		// Zero hashes cannot arrive through the API (loads
		// normalize them out); set the lists directly to pin
		// the TS `if (l)` guard.
		r.friendList = []int64{0, StringToLong("bob")}
		r.ignoreList = []int64{0, StringToLong("carol")}
		r.SendFriends()
		r.SendIgnores()
		if !reflect.DeepEqual(fa.friends, [][2]int64{{StringToLong("bob"), 0}}) {
			t.Errorf("friends = %v, want only bob", fa.friends)
		}
		if !reflect.DeepEqual(fa.addedIgnores, []int64{StringToLong("carol")}) {
			t.Errorf("addedIgnores = %v, want only carol", fa.addedIgnores)
		}
	})

	t.Run("private status maps the status byte", func(t *testing.T) {
		cases := []struct {
			status PrivateChatStatus
			want   int
		}{
			{PrivateChatOn, 0},
			{PrivateChatFriendsOnly, 1},
			{PrivateChatOff, 2},
		}
		for _, tc := range cases {
			a, fa := newRelationsPlayer("alice")
			a.Relations().SetChatModes(3, int(tc.status), 2)
			a.Relations().SendPrivateStatus()
			if !reflect.DeepEqual(fa.chatOptions, [][3]int{{3, tc.want, 2}}) {
				t.Errorf("status %v: chatOptions = %v, want [[3 %d 2]]", tc.status, fa.chatOptions, tc.want)
			}
		}
	})

	t.Run("single sends", func(t *testing.T) {
		a, fa := newRelationsPlayer("alice")
		r := a.Relations()
		r.SendAddFriend(11)
		r.SendDeleteFriend(12)
		r.SendAddIgnore(13)
		r.SendDeleteIgnore(14)
		if !reflect.DeepEqual(fa.friends, [][2]int64{{11, 0}}) {
			t.Errorf("friends = %v, want [(11,0)]", fa.friends)
		}
		if !reflect.DeepEqual(fa.deletedFriends, []int64{12}) {
			t.Errorf("deletedFriends = %v, want [12]", fa.deletedFriends)
		}
		if !reflect.DeepEqual(fa.addedIgnores, []int64{13}) {
			t.Errorf("addedIgnores = %v, want [13]", fa.addedIgnores)
		}
		if !reflect.DeepEqual(fa.deletedIgnores, []int64{14}) {
			t.Errorf("deletedIgnores = %v, want [14]", fa.deletedIgnores)
		}
	})

	t.Run("session without the sender extension drops silently", func(t *testing.T) {
		// A bare fakeSession implements SessionPort and
		// ChatMessageSender but not RelationsSender.
		p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
		p.Session = newFakeSession()
		r := p.Relations()
		r.LoadSaveState(RelationsSaveState{Friends: []int64{1}, Ignores: []int64{2}})
		r.UpdateLists(true)
		r.OnLogin()
		r.SendAddFriend(3)
		// No panic, no relations traffic possible; the only
		// assertion is survival. SendMessage still works
		// through ChatMessageSender.
		p.SendMessage("hi")
		if got := p.Session.(*fakeSession).messages; !reflect.DeepEqual(got, []string{"hi"}) {
			t.Errorf("messages = %v, want [hi]", got)
		}
	})

	t.Run("session-less player drops silently", func(t *testing.T) {
		p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
		r := p.Relations()
		r.LoadSaveState(RelationsSaveState{Friends: []int64{1}})
		r.UpdateLists(true)
		r.OnLogin()
		r.AddFriend(2)
		r.Message(NewPlayer("bob", Location{X: 3089, Y: 3524, Z: 0}), []byte("x"), 1)
		// Survival is the assertion: no session, no panic.
		if !r.HasFriend(2) {
			t.Error("session-less AddFriend did not add")
		}
	})
}

func TestMessage(t *testing.T) {
	installNoRelationsWorld(t)
	payload := []byte{0x01, 0x02, 0x03}

	t.Run("delivered when receivable", func(t *testing.T) {
		s, fs := newRelationsPlayer("alice")
		r, fr := newRelationsPlayer("bob")
		s.Relations().Message(r, payload, 3)
		if len(fs.messages) != 0 {
			t.Errorf("sender messages = %v, want none", fs.messages)
		}
		if len(fr.privateMsgs) != 1 {
			t.Fatalf("recipient private messages = %d, want 1", len(fr.privateMsgs))
		}
		got := fr.privateMsgs[0]
		if got.sender != s || !reflect.DeepEqual(got.message, payload) || got.size != 3 {
			t.Errorf("delivery = %+v, want sender=s payload size 3", got)
		}
		if s.Relations().GetStatus() != PrivateChatOn {
			t.Errorf("sender status = %v, want ON", s.Relations().GetStatus())
		}
	})

	t.Run("ignored recipient reports offline", func(t *testing.T) {
		s, fs := newRelationsPlayer("alice")
		r, fr := newRelationsPlayer("bob")
		r.Relations().LoadSaveState(RelationsSaveState{Ignores: []int64{s.GetLongUsername()}})
		s.Relations().Message(r, payload, 3)
		if !reflect.DeepEqual(fs.messages, []string{"This player is currently offline."}) {
			t.Errorf("sender messages = %v, want the offline notice", fs.messages)
		}
		if len(fr.privateMsgs) != 0 {
			t.Errorf("recipient received %d messages, want 0", len(fr.privateMsgs))
		}
	})

	t.Run("OFF recipient reports offline", func(t *testing.T) {
		s, fs := newRelationsPlayer("alice")
		r, fr := newRelationsPlayer("bob")
		r.Relations().SetStatus(PrivateChatOff, false)
		s.Relations().Message(r, payload, 3)
		if !reflect.DeepEqual(fs.messages, []string{"This player is currently offline."}) {
			t.Errorf("sender messages = %v, want the offline notice", fs.messages)
		}
		if len(fr.privateMsgs) != 0 {
			t.Error("OFF recipient received a message")
		}
	})

	t.Run("friends-only recipient needs the sender listed", func(t *testing.T) {
		s, fs := newRelationsPlayer("alice")
		r, fr := newRelationsPlayer("bob")
		r.Relations().SetStatus(PrivateChatFriendsOnly, false)
		s.Relations().Message(r, payload, 3)
		if len(fr.privateMsgs) != 0 || len(fs.messages) != 1 {
			t.Error("unlisted sender delivered to FRIENDS_ONLY recipient")
		}
		r.Relations().LoadSaveState(RelationsSaveState{Friends: []int64{s.GetLongUsername()}})
		fs.messages = nil
		s.Relations().Message(r, payload, 3)
		if len(fr.privateMsgs) != 1 || len(fs.messages) != 0 {
			t.Error("listed sender not delivered to FRIENDS_ONLY recipient")
		}
	})

	t.Run("OFF sender is promoted to friends-only", func(t *testing.T) {
		s, fs := newRelationsPlayer("alice")
		r, fr := newRelationsPlayer("bob")
		s.Relations().SetStatus(PrivateChatOff, false)
		s.Relations().Message(r, payload, 3)
		if s.Relations().GetStatus() != PrivateChatFriendsOnly {
			t.Errorf("sender status = %v, want FRIENDS_ONLY", s.Relations().GetStatus())
		}
		// The promotion broadcasts (SetStatus update=true):
		// the self friend status goes out.
		if !reflect.DeepEqual(fs.friendStatus, []int{2}) {
			t.Errorf("sender friendStatus = %v, want [2]", fs.friendStatus)
		}
		if len(fr.privateMsgs) != 1 {
			t.Error("promoted sender's message not delivered")
		}
	})

	t.Run("friends-only sender stays", func(t *testing.T) {
		s, _ := newRelationsPlayer("alice")
		r, fr := newRelationsPlayer("bob")
		s.Relations().SetStatus(PrivateChatFriendsOnly, false)
		s.Relations().Message(r, payload, 3)
		if s.Relations().GetStatus() != PrivateChatFriendsOnly {
			t.Errorf("sender status = %v, want FRIENDS_ONLY", s.Relations().GetStatus())
		}
		if len(fr.privateMsgs) != 1 {
			t.Error("message not delivered")
		}
	})

	t.Run("nil friend reports offline", func(t *testing.T) {
		s, fs := newRelationsPlayer("alice")
		s.Relations().Message(nil, payload, 3)
		if !reflect.DeepEqual(fs.messages, []string{"This player is currently offline."}) {
			t.Errorf("sender messages = %v, want the offline notice", fs.messages)
		}
	})

	t.Run("size passes through verbatim", func(t *testing.T) {
		s, _ := newRelationsPlayer("alice")
		r, fr := newRelationsPlayer("bob")
		s.Relations().Message(r, []byte{0x01, 0x02}, 99)
		if len(fr.privateMsgs) != 1 || fr.privateMsgs[0].size != 99 {
			t.Errorf("delivered size = %+v, want 99", fr.privateMsgs)
		}
	})
}

func TestIsFriendWith(t *testing.T) {
	p, _ := newRelationsPlayer("alice")
	r := p.Relations()
	r.LoadSaveState(RelationsSaveState{Friends: []int64{StringToLong("bob")}})
	if !r.IsFriendWith("bob") || !r.IsFriendWith("BOB") || !r.IsFriendWith("bob_") {
		t.Error("IsFriendWith missed a name form hashing to bob")
	}
	if r.IsFriendWith("carol") || r.IsFriendWith("") {
		t.Error("IsFriendWith matched a non-friend")
	}
}

func TestGetFriendListCopies(t *testing.T) {
	p, _ := newRelationsPlayer("alice")
	r := p.Relations()
	r.LoadSaveState(RelationsSaveState{Friends: []int64{1, 2}, Ignores: []int64{3}})
	got := r.GetFriendList()
	got[0] = 99
	if r.GetFriendList()[0] != 1 {
		t.Error("mutating GetFriendList result mutated live state")
	}
	gotI := r.GetIgnoreList()
	gotI[0] = 99
	if r.GetIgnoreList()[0] != 3 {
		t.Error("mutating GetIgnoreList result mutated live state")
	}
}

func TestLoadSnapshotRoundTrip(t *testing.T) {
	t.Run("defensive normalization on load", func(t *testing.T) {
		p, _ := newRelationsPlayer("alice")
		r := p.Relations()
		r.LoadSaveState(RelationsSaveState{
			Friends: []int64{3, 1, 3, 0, -5, 2},
			Ignores: []int64{9, 9, 0},
			FriendRanks: map[string]int{
				"3":  2,
				"99": 5, // unlisted: dropped
				"1":  9, // clamped to 6
			},
			FriendsChatChannelName: "Chan",
			FriendsChatLastOwner:   "alice",
			FriendsChatEntryRank:   5,
			FriendsChatTalkRank:    -9, // clamped to -1
			FriendsChatKickRank:    0,  // clamped to 2
		})
		if !reflect.DeepEqual(r.GetFriendList(), []int64{3, 1, 2}) {
			t.Errorf("friends = %v, want [3 1 2]", r.GetFriendList())
		}
		if !reflect.DeepEqual(r.GetIgnoreList(), []int64{9}) {
			t.Errorf("ignores = %v, want [9]", r.GetIgnoreList())
		}
		if !reflect.DeepEqual(r.GetFriendRanks(), map[string]int{"3": 2, "1": 6}) {
			t.Errorf("ranks = %v, want {3:2 1:6}", r.GetFriendRanks())
		}
		if r.GetFriendsChatChannelName() != "Chan" || r.GetFriendsChatLastOwner() != "alice" {
			t.Error("channel fields not loaded")
		}
		if r.GetFriendsChatEntryRank() != 5 || r.GetFriendsChatTalkRank() != -1 || r.GetFriendsChatKickRank() != 2 {
			t.Errorf("channel ranks = %d/%d/%d, want 5/-1/2",
				r.GetFriendsChatEntryRank(), r.GetFriendsChatTalkRank(), r.GetFriendsChatKickRank())
		}
	})

	t.Run("caps on load", func(t *testing.T) {
		p, _ := newRelationsPlayer("alice")
		many := make([]int64, 0, 250)
		for i := int64(1); i <= 250; i++ {
			many = append(many, i)
		}
		p.Relations().LoadSaveState(RelationsSaveState{Friends: many, Ignores: many})
		if len(p.Relations().GetFriendList()) != MaxFriends {
			t.Errorf("friends length = %d, want %d", len(p.Relations().GetFriendList()), MaxFriends)
		}
		if len(p.Relations().GetIgnoreList()) != MaxIgnores {
			t.Errorf("ignores length = %d, want %d", len(p.Relations().GetIgnoreList()), MaxIgnores)
		}
	})

	t.Run("round trip", func(t *testing.T) {
		p, _ := newRelationsPlayer("alice")
		want := RelationsSaveState{
			Friends: []int64{5, 6, 7},
			Ignores: []int64{8},
			FriendRanks: map[string]int{
				"5": 1,
				"6": 6,
			},
			FriendsChatChannelName: "Chan",
			FriendsChatLastOwner:   "bob",
			FriendsChatEntryRank:   0,
			FriendsChatTalkRank:    3,
			FriendsChatKickRank:    4,
		}
		p.Relations().LoadSaveState(want)
		if got := p.Relations().SnapshotSaveState(); !reflect.DeepEqual(got, want) {
			t.Errorf("snapshot = %+v, want %+v", got, want)
		}
	})

	t.Run("snapshot copies", func(t *testing.T) {
		p, _ := newRelationsPlayer("alice")
		p.Relations().LoadSaveState(RelationsSaveState{
			Friends:     []int64{5},
			FriendRanks: map[string]int{"5": 1},
		})
		snap := p.Relations().SnapshotSaveState()
		snap.Friends[0] = 99
		snap.FriendRanks["5"] = 6
		snap.FriendRanks["6"] = 6
		if p.Relations().GetFriendList()[0] != 5 || p.Relations().GetFriendRank(5) != 1 {
			t.Error("mutating the snapshot mutated live state")
		}
	})

	t.Run("load replaces wholesale", func(t *testing.T) {
		p, _ := newRelationsPlayer("alice")
		r := p.Relations()
		r.LoadSaveState(RelationsSaveState{
			Friends: []int64{1, 2}, Ignores: []int64{3},
			FriendRanks:            map[string]int{"1": 2},
			FriendsChatChannelName: "Old",
		})
		r.LoadSaveState(RelationsSaveState{Friends: []int64{4}})
		got := r.SnapshotSaveState()
		if !reflect.DeepEqual(got.Friends, []int64{4}) || len(got.Ignores) != 0 ||
			len(got.FriendRanks) != 0 || got.FriendsChatChannelName != "" {
			t.Errorf("snapshot after replace = %+v, want only friends [4]", got)
		}
		// The channel ranks reset to the caller's values: a
		// zero RelationsSaveState carries 0/0/0, clamped on load
		// to 0/0/2 — the caller supplies the -1/-1/2 defaults
		// for a missing save, per PlayerSave.ts:438-447.
		if got.FriendsChatEntryRank != 0 || got.FriendsChatTalkRank != 0 || got.FriendsChatKickRank != 2 {
			t.Errorf("channel ranks after zero load = %d/%d/%d, want 0/0/2",
				got.FriendsChatEntryRank, got.FriendsChatTalkRank, got.FriendsChatKickRank)
		}
	})

	t.Run("fresh snapshot shape", func(t *testing.T) {
		p, _ := newRelationsPlayer("alice")
		got := p.Relations().SnapshotSaveState()
		if got.Friends == nil || got.Ignores == nil || got.FriendRanks == nil {
			t.Error("fresh snapshot has nil slices/map; want non-nil empty (TS []/{})")
		}
		if len(got.Friends) != 0 || len(got.Ignores) != 0 || len(got.FriendRanks) != 0 {
			t.Errorf("fresh snapshot = %+v, want empty", got)
		}
		if got.FriendsChatEntryRank != -1 || got.FriendsChatTalkRank != -1 || got.FriendsChatKickRank != 2 {
			t.Errorf("fresh channel ranks = %d/%d/%d, want -1/-1/2",
				got.FriendsChatEntryRank, got.FriendsChatTalkRank, got.FriendsChatKickRank)
		}
	})
}

func TestRelationsWorldAdapter(t *testing.T) {
	t.Run("nil world", func(t *testing.T) {
		w := NewWorldRelations(nil)
		if w.RelationsPlayers() != nil {
			t.Errorf("nil-world players = %v, want nil", w.RelationsPlayers())
		}
		if w.RelationsPlayerByName("alice") != nil {
			t.Error("nil-world lookup matched")
		}
	})

	t.Run("real world iteration and normalized lookup", func(t *testing.T) {
		w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
		a, _ := newRelationsPlayer("alice_smith")
		b, _ := newRelationsPlayer("bob")
		if _, ok := w.Players().Add(a); !ok {
			t.Fatal("Add(a) failed")
		}
		if _, ok := w.Players().Add(b); !ok {
			t.Fatal("Add(b) failed")
		}
		adapter := NewWorldRelations(w)
		players := adapter.RelationsPlayers()
		if len(players) != 2 || players[0] != a || players[1] != b {
			t.Errorf("players = %v, want [a b] in slot order", players)
		}
		// World.getPlayerByName normalization: case, spaces,
		// and underscores do not matter.
		for _, needle := range []string{"Alice Smith", "ALICE_SMITH", "alice smith", "alice_smith"} {
			if got := adapter.RelationsPlayerByName(needle); got != a {
				t.Errorf("lookup(%q) = %v, want a", needle, got)
			}
		}
		if got := adapter.RelationsPlayerByName("Bob"); got != b {
			t.Errorf("lookup(Bob) = %v, want b", got)
		}
		if got := adapter.RelationsPlayerByName("nobody"); got != nil {
			t.Errorf("lookup(nobody) = %v, want nil", got)
		}
		if got := adapter.RelationsPlayerByName(""); got != nil {
			t.Errorf("lookup(empty) = %v, want nil", got)
		}
	})

	t.Run("installed adapter drives peer updates", func(t *testing.T) {
		w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
		a, _ := newRelationsPlayer("alice")
		b, fb := newRelationsPlayer("bob")
		w.Players().Add(a)
		w.Players().Add(b)
		prev := relationsWorld
		SetRelationsWorld(NewWorldRelations(w))
		t.Cleanup(func() { SetRelationsWorld(prev) })
		a.Relations().AddFriend(b.GetLongUsername())
		if !reflect.DeepEqual(fb.friendStatus, []int{2}) {
			t.Errorf("b friendStatus = %v, want [2] from the peer update", fb.friendStatus)
		}
	})
}

func TestShownNameSeam(t *testing.T) {
	// Default: identity (no registry installed).
	installShownName(t, nil)
	if got := shownName("Alice"); got != "Alice" {
		t.Errorf("default shownName = %q, want identity", got)
	}
	// Override: the DisplayNames adapter form.
	installShownName(t, func(username string) string {
		if username == "Bob" {
			return "King Bob"
		}
		return username
	})
	if got := shownName("Bob"); got != "King Bob" {
		t.Errorf("overridden shownName(Bob) = %q, want King Bob", got)
	}
	if got := shownName("Alice"); got != "Alice" {
		t.Errorf("overridden shownName(Alice) = %q, want Alice", got)
	}
}

func TestNilRelationsDegradation(t *testing.T) {
	// Every method tolerates a nil receiver (SendMessage precedent):
	// getters read defaults, sends and mutations no-op.
	var r *PlayerRelations
	p, _ := newRelationsPlayer("alice")
	if r.GetStatus() != PrivateChatOn {
		t.Errorf("nil GetStatus = %v, want ON", r.GetStatus())
	}
	if r.GetPublicChatMode() != 0 || r.GetTradeChatMode() != 0 {
		t.Error("nil modes non-zero")
	}
	if r.GetFriendList() != nil || r.GetIgnoreList() != nil {
		t.Error("nil lists non-nil")
	}
	if r.HasFriend(1) || r.HasIgnore(1) {
		t.Error("nil membership true")
	}
	if r.CanReceivePublicChatFrom(p, false) || r.IsVisibleTo(p) || r.CanReceivePrivateMessageFrom(p) {
		t.Error("nil gates true")
	}
	if r.GetFriendRank(1) != 0 || r.SetFriendRank(1, 3) {
		t.Error("nil rank read/set wrong")
	}
	if len(r.GetFriendRanks()) != 0 {
		t.Error("nil ranks non-empty")
	}
	if r.GetFriendsChatChannelName() != "" || r.GetFriendsChatLastOwner() != "" {
		t.Error("nil channel fields non-empty")
	}
	if r.GetFriendsChatEntryRank() != -1 || r.GetFriendsChatTalkRank() != -1 || r.GetFriendsChatKickRank() != 2 {
		t.Error("nil channel ranks not defaults")
	}
	if r.GetPrivateMessageId() != 0 {
		t.Errorf("nil GetPrivateMessageId = %d, want 0", r.GetPrivateMessageId())
	}
	if r.SetPrivateMessageId(5) != nil || r.SetStatus(PrivateChatOff, true) != nil ||
		r.UpdateLists(true) != nil || r.OnLogin() != nil {
		t.Error("nil chaining methods did not return nil")
	}
	// No-op calls must not panic.
	r.SetChatModes(1, 1, 1)
	r.SetFriendsChatChannelName("x")
	r.SetFriendsChatLastOwner("x")
	r.SetFriendsChatRanks(1, 1, 1)
	r.LoadFriendRanks(map[string]int{"1": 1})
	r.LoadSaveState(RelationsSaveState{Friends: []int64{1}})
	r.SendPrivateStatus()
	r.SendFriends()
	r.SendIgnores()
	r.SendAddFriend(1)
	r.SendDeleteFriend(1)
	r.SendAddIgnore(1)
	r.SendDeleteIgnore(1)
	r.AddFriend(1)
	r.DeleteFriend(1)
	r.AddIgnore(1)
	r.DeleteIgnore(1)
	r.Message(p, []byte("x"), 1)
	if r.IsFriendWith("alice") {
		t.Error("nil IsFriendWith true")
	}
	snap := r.SnapshotSaveState()
	if snap.FriendsChatEntryRank != -1 || snap.FriendsChatTalkRank != -1 || snap.FriendsChatKickRank != 2 {
		t.Errorf("nil snapshot ranks = %d/%d/%d, want -1/-1/2",
			snap.FriendsChatEntryRank, snap.FriendsChatTalkRank, snap.FriendsChatKickRank)
	}
}
