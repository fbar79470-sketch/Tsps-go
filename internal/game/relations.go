package game

import (
	"strconv"
	"strings"
	"sync"
)

// Player relations: the port of PlayerRelations
// (game/model/PlayerRelations.ts, 383 lines at commit bfd81e3) plus the
// Player wiring it needs (LongUsername, Relations()).
//
// WHAT IS PORTED, WHOLE: friend/ignore lists as int64 name-hashes,
// friend ranks, the friends-chat channel fields, private-chat status
// and the public/trade chat modes with the exact setChatModes clamping,
// the three gates (canReceivePublicChatFrom, isVisibleTo,
// canReceivePrivateMessageFrom), updateLists presence broadcast,
// add/delete flows with the exact TS refusal/notice strings, the
// onLogin sends, the private-message id counter, and message() —
// including message(), although no live TS caller uses it (see Message).
// Friend/ignore caps, rank ranges, and every string below are copied
// from the TS source; each method cites its lines.
//
// INT64 HASHES. TS stores name hashes as bigint; Go stores them as
// int64. Every hash fits (37^12 < 2^63; see names.go), so the port is
// exact with no big-int arithmetic.
//
// NOT PORTED (plugin content, out of scope): FriendsChatManager.js and
// the Clans plugin — channels, joins, membership, and the friends-chat
// / clan snapshot sends. This file is the engine API only: the lists,
// the modes, the gates, and the presence broadcast. The friends-chat
// channel FIELDS (name/owner/ranks) are engine state on PlayerRelations
// and are ported; what joins or displays a channel is content.
//
// THREADING. Relations state is tick-goroutine-owned, like the rest of
// Player state: every engine call site runs on the tick goroutine
// (turn-packet listeners, the logout path, updateLists peer updates)
// or before the player is visible to the tick goroutine (entry-time
// LoadSaveState, OnLogin from the login path). No mutex guards the
// state itself — TS runs one event loop and needs none, and Go invents
// no locking TS doesn't need. Content calling relations methods from
// login-goroutine hooks before the player is queued is race-free for
// the same reason other entry-time writes are; content must not mutate
// relations concurrently with the tick from another goroutine, the same
// rule as for inventory and skills. The one exception is the Relations()
// pointer allocation, serialized by relationsAllocMu following the
// chatIconStore/attribute-store precedent (chat.go, attributes.go), so
// a zero-value Player's first Relations() cannot tear the pointer.
// The package-level seams below (RelationsWorld, ShownNameFunc) are
// set once at boot, before ticking starts, following the
// AreaMethodCaller precedent (area.go), and read without a lock.
//
// SEAMS. Three seams, all defined here; another worker implements or
// wires each one:
//   - RelationsSender: the optional session interface for relations
//     sends (ChatMessageSender precedent, message.go). Relations
//     methods degrade silently when the player's session lacks it.
//   - RelationsWorld: the world view updateLists and the add/delete
//     flows iterate and look up through, installed with
//     SetRelationsWorld. NewWorldRelations adapts a *World. Nil
//     degrades updateLists to self-only sends.
//   - ShownNameFunc: the DisplayNames lookup behind the shownName in
//     refusal/notice strings, installed with SetShownNameFunc.
//     Identity by default (no display-name entry).
//
// SAVE API (contract C3). LoadSaveState/SnapshotSaveState move the
// whole relations state to and from save-shaped values; see
// RelationsSaveState. Normalization (positive, unique, capped) lives
// in persist (PlayerSave.normalizeRelationListToBigInt,
// PlayerSave.ts:482-508); the load/snapshot methods re-apply it
// defensively so malformed input can never corrupt live state.

// MaxFriends is the friend-list cap: MAX_FRIENDS
// (PlayerRelations.ts:7). AddFriend refuses past it.
const MaxFriends = 200

// MaxIgnores is the ignore-list cap: MAX_IGNORES
// (PlayerRelations.ts:8). AddIgnore refuses past it.
const MaxIgnores = 100

// publicModeAutoChat is the public-chat mode that shows autochat:
// PUBLIC_MODE_AUTOCHAT (PlayerRelations.ts:9). Mode 4 accepts every
// public-chat line, including autochat; see CanReceivePublicChatFrom.
const publicModeAutoChat = 4

// PrivateChatStatus is the private-chat presence setting: the
// PrivateChatStatus enum (PlayerRelations.ts:381-383). The numeric
// values are the TS enum order and matter: setChatModes clamps the
// client's private-mode byte to 0..2 and casts it to this type, and
// SendPrivateStatus maps the status back to a chat-options byte
// (ON 0, FRIENDS_ONLY 1, OFF 2).
type PrivateChatStatus int

const (
	// PrivateChatOn: presence visible to everyone, private messages
	// from anyone not ignored.
	PrivateChatOn PrivateChatStatus = iota
	// PrivateChatFriendsOnly: presence visible to friends only, and
	// only friends' private messages arrive.
	PrivateChatFriendsOnly
	// PrivateChatOff: invisible; no private messages arrive, and
	// updateLists reports offline.
	PrivateChatOff
)

// RelationsSender is the optional SessionPort extension for relations
// sends: the session side of the PlayerRelations send methods. The
// session package implements it; the game layer must never import the
// protocol package, so the encoding lives there, as with
// ChatMessageSender (message.go) and SendVarp/SendRunEnergy
// (player.go). A nil player, a nil session, or a session without this
// extension drops the send silently: session-less and test players
// have no client to inform.
//
// Method-for-method, the TS surface is PacketSender
// (net/packet/PacketSender.ts): sendFriendStatus (line 778),
// sendFriend (785), sendDeleteFriend (794), sendAddIgnore (801),
// sendDeleteIgnore (809), sendChatOptions (365), sendPrivateMessage
// (850). The Go signatures carry the same arguments the TS methods
// take, except SendPrivateMessage, whose TS target is `any` and is
// typed here as the sending *Player (the session reads its display
// name and index, as PacketSender.ts:850-858 does).
type RelationsSender interface {
	// SendFriendStatus queues the friend-list status packet
	// (PacketSender.sendFriendStatus, PacketSender.ts:778-783,
	// opcode 221). Relations passes 2, unconditionally, from
	// UpdateLists, as TS does (PlayerRelations.ts:188).
	SendFriendStatus(status int)
	// SendFriend queues a friend add/update packet
	// (PacketSender.sendFriend, PacketSender.ts:785-791, opcode
	// 50). world is 0 (offline) or 1 (online) as relations
	// computes it; the session maps a nonzero world to world+9,
	// as TS does.
	SendFriend(name int64, world int)
	// SendDeleteFriend queues a friend removal packet
	// (PacketSender.sendDeleteFriend, PacketSender.ts:794-798,
	// opcode 51).
	SendDeleteFriend(name int64)
	// SendAddIgnore queues an ignore add packet
	// (PacketSender.sendAddIgnore, PacketSender.ts:801-805,
	// opcode 214).
	SendAddIgnore(name int64)
	// SendDeleteIgnore queues an ignore removal packet
	// (PacketSender.sendDeleteIgnore, PacketSender.ts:809-813,
	// opcode 215).
	SendDeleteIgnore(name int64)
	// SendChatOptions queues the chat-filter settings packet
	// (PacketSender.sendChatOptions, PacketSender.ts:365-373,
	// encodeChatFilterSettings). privateChat is the mapped byte
	// (ON 0, FRIENDS_ONLY 1, OFF 2), computed by SendPrivateStatus,
	// not the raw status.
	SendChatOptions(publicChat, privateChat, tradeChat int)
	// SendPrivateMessage queues a private message for THIS player
	// (the session's owner) from sender
	// (PacketSender.sendPrivateMessage, PacketSender.ts:850-858,
	// encodeChatMessage "private_in"). message is the packed
	// payload and size its authoritative byte count, passed
	// through verbatim from Message; the session unpacks the text
	// (Misc.textUnpack, lowercased then ucFirst) as TS does.
	SendPrivateMessage(sender *Player, message []byte, size int)
}

// RelationsWorld is the world view PlayerRelations needs: the
// registered players to broadcast presence to, and the by-name lookup
// for the peer updateLists calls in the add/delete flows. It is the Go
// form of the two static World accesses in PlayerRelations.ts:
// World.getPlayers() (World.ts:232-234) and World.getPlayerByName
// (World.ts:135-138).
type RelationsWorld interface {
	// RelationsPlayers returns the registered players, in slot
	// order: World.getPlayers() (World.ts:232-234). The caller
	// skips nil entries, as TS updateLists does
	// (PlayerRelations.ts:192-194).
	RelationsPlayers() []*Player
	// RelationsPlayerByName returns the registered player for a
	// formatted name, or nil: World.getPlayerByName
	// (World.ts:135-138). The needle is normalized with
	// formatNameForProtocol(formatDisplayName(name)) and each
	// candidate with formatNameForProtocol(username); the first
	// match in slot order wins, as in TS search. Callers pass the
	// FormatName(LongToString(hash)) form add/delete compute.
	RelationsPlayerByName(name string) *Player
}

// relationsWorld is the installed RelationsWorld. It is set once at
// boot via SetRelationsWorld, before the world starts ticking,
// following the AreaMethodCaller precedent (area.go). Nil means no
// world is installed: updateLists degrades to self-only sends and the
// add/delete flows skip their peer updates.
var relationsWorld RelationsWorld

// SetRelationsWorld installs the RelationsWorld relations methods
// iterate and look up through. The coordinator calls it once at boot
// with NewWorldRelations(world); passing nil uninstalls (tests use
// this to restore the no-world degradation). It must not be called
// while the world is ticking: reads are unsynchronized, as with
// SetAreaMethodCaller.
func SetRelationsWorld(world RelationsWorld) {
	relationsWorld = world
}

// worldRelations adapts a *World to RelationsWorld. It is constructed
// through NewWorldRelations; a nil World yields no players and no
// lookup matches.
type worldRelations struct {
	world *World
}

// NewWorldRelations returns the RelationsWorld backed by w: iteration
// over w.Players().All() (slot order, empty slots skipped, as TS
// getPlayers iteration) and the normalized by-name lookup of
// World.getPlayerByName (World.ts:135-138), implemented over the same
// iteration with the local formatNameForProtocol/formatDisplayName
// ports (grounditems.go) — World.PlayerByName is deliberately NOT used:
// it compares usernames exactly, and its normalization is NOT STARTED.
// A nil World is tolerated: iteration is empty and lookup is nil.
func NewWorldRelations(w *World) RelationsWorld {
	return worldRelations{world: w}
}

// RelationsPlayers returns w's registered players in slot order.
func (s worldRelations) RelationsPlayers() []*Player {
	if s.world == nil || s.world.players == nil {
		return nil
	}
	return s.world.players.All()
}

// RelationsPlayerByName returns the first registered player whose
// protocol-form username matches the normalized needle, or nil. See
// RelationsWorld.RelationsPlayerByName for the normalization rule.
func (s worldRelations) RelationsPlayerByName(name string) *Player {
	needle := formatNameForProtocol(formatDisplayName(name))
	for _, p := range s.RelationsPlayers() {
		if p != nil && formatNameForProtocol(p.Username) == needle {
			return p
		}
	}
	return nil
}

// shownNameFunc resolves the name a listed account shows in a
// refusal/notice string: the DisplayNames lookup behind
// PlayerRelations.shownName (PlayerRelations.ts:31-33), which returns
// DisplayNames.entryOf(username)?.name ?? username
// (DisplayNames.ts:87, entryOf). It receives the formatted name
// (FormatName output) and returns the display name for it, or the
// input unchanged when the account has no entry. Nil means no
// registry is installed and every lookup misses: the identity
// default, which is exactly the TS fallback for an account without
// an entry. Set once at boot via SetShownNameFunc, before ticking
// starts; reads are unsynchronized, as with relationsWorld.
var shownNameFunc func(username string) string

// SetShownNameFunc installs the display-name lookup relations messages
// use for shown names. The coordinator overrides the identity default
// with the persist.DisplayNames registry:
//
//	game.SetShownNameFunc(func(username string) string {
//		if entry, ok := displayNames.EntryOf(username); ok {
//			return entry.Name
//		}
//		return username
//	})
//
// EntryOf keys its argument (persist.Key), as TS entryOf keys via
// DisplayNames.key, so the formatted name relations passes resolves
// the same entry TS resolves. Passing nil restores the identity
// default. It must not be called while the world is ticking.
func SetShownNameFunc(f func(username string) string) {
	shownNameFunc = f
}

// shownName returns the name username shows in a message: the
// installed lookup's answer, or username unchanged with no lookup
// installed. PlayerRelations.shownName (PlayerRelations.ts:31-33).
func shownName(username string) string {
	if shownNameFunc != nil {
		return shownNameFunc(username)
	}
	return username
}

// PlayerRelations is one player's friend/ignore lists, friend ranks,
// friends-chat channel fields, and chat-filter state: the port of
// PlayerRelations (game/model/PlayerRelations.ts). Values must be
// built with NewPlayerRelations (or reached through Player.Relations,
// which builds one); the zero value has nil maps and must not be used.
//
// The field defaults are the TS field initializers
// (PlayerRelations.ts:10-23): status ON, public/trade modes 0, empty
// lists, empty ranks, empty channel name/owner, entry/talk ranks -1,
// kick rank 2, private-message id starting at 1.
type PlayerRelations struct {
	status           PrivateChatStatus
	publicChatMode   int
	tradeChatMode    int
	friendList       []int64
	ignoreList       []int64
	friendSet        map[int64]struct{}
	ignoreSet        map[int64]struct{}
	friendRanks      map[int64]int
	channelName      string
	lastOwner        string
	entryRank        int
	talkRank         int
	kickRank         int
	privateMessageID int
	player           *Player
}

// NewPlayerRelations returns the relations object for player, holding
// the player pointer as the TS constructor does
// (PlayerRelations.ts:26-28). The player may be nil in tests; send
// methods then degrade silently and gates read the player's hash as 0.
func NewPlayerRelations(player *Player) *PlayerRelations {
	return &PlayerRelations{
		status:           PrivateChatOn,
		friendSet:        make(map[int64]struct{}),
		ignoreSet:        make(map[int64]struct{}),
		friendRanks:      make(map[int64]int),
		entryRank:        -1,
		talkRank:         -1,
		kickRank:         2,
		privateMessageID: 1,
		player:           player,
	}
}

// relationsAllocMu serializes lazy allocation of PlayerRelations
// objects and every load of a Player's relations pointer, mirroring
// chatIconAllocMu in chat.go. See the file header for why only the
// allocation, not the state, is locked.
var relationsAllocMu sync.Mutex

// Relations returns p's relations object: Player.getRelations
// (Player.ts:1152-1154), backed by the eagerly constructed field
// (Player.ts:117, `relations = new PlayerRelations(this)`, built by
// NewPlayer). It is never nil for a non-nil player: a zero-value
// Player, which NewPlayer never built, gets one allocated lazily on
// first access, following the chatIconStore precedent — the allocated
// object holds p, as the TS constructor requires. A nil player has no
// relations and returns nil.
func (p *Player) Relations() *PlayerRelations {
	if p == nil {
		return nil
	}
	relationsAllocMu.Lock()
	defer relationsAllocMu.Unlock()
	if p.relations == nil {
		p.relations = NewPlayerRelations(p)
	}
	return p.relations
}

// GetLongUsername returns the player's base-37 name hash:
// Player.getLongUsername (Player.ts:976-978). The login path sets the
// LongUsername field at enterWorld (NetworkBuilder.ts:1662,
// setLongUsername with Misc.stringToLongBigInt); until then it reads
// 0, the TS field default (Player.ts:151). A nil player reads 0.
func (p *Player) GetLongUsername() int64 {
	if p == nil {
		return 0
	}
	return p.LongUsername
}

// playerHash returns the owning player's long username, or 0 when the
// relations object or its player is nil.
func (r *PlayerRelations) playerHash() int64 {
	if r == nil || r.player == nil {
		return 0
	}
	return r.player.GetLongUsername()
}

// sender returns the RelationsSender for the owning player's session,
// or false when there is no player, no session, or the session does
// not implement the interface. Every relations send degrades silently
// through this helper, following SendMessage (message.go).
func (r *PlayerRelations) sender() (RelationsSender, bool) {
	if r == nil || r.player == nil {
		return nil, false
	}
	sender, ok := r.player.Session.(RelationsSender)
	return sender, ok
}

// sendFriendStatus sends the friend-list status packet to the owning
// player, or drops it when no RelationsSender is attached.
func (r *PlayerRelations) sendFriendStatus(status int) {
	if sender, ok := r.sender(); ok {
		sender.SendFriendStatus(status)
	}
}

// sendFriend sends a friend add/update packet to the owning player, or
// drops it when no RelationsSender is attached. world is 0/1; see
// RelationsSender.SendFriend.
func (r *PlayerRelations) sendFriend(name int64, world int) {
	if sender, ok := r.sender(); ok {
		sender.SendFriend(name, world)
	}
}

// sendDeleteFriend sends a friend removal packet to the owning player,
// or drops it when no RelationsSender is attached.
func (r *PlayerRelations) sendDeleteFriend(name int64) {
	if sender, ok := r.sender(); ok {
		sender.SendDeleteFriend(name)
	}
}

// sendAddIgnore sends an ignore add packet to the owning player, or
// drops it when no RelationsSender is attached.
func (r *PlayerRelations) sendAddIgnore(name int64) {
	if sender, ok := r.sender(); ok {
		sender.SendAddIgnore(name)
	}
}

// sendDeleteIgnore sends an ignore removal packet to the owning player,
// or drops it when no RelationsSender is attached.
func (r *PlayerRelations) sendDeleteIgnore(name int64) {
	if sender, ok := r.sender(); ok {
		sender.SendDeleteIgnore(name)
	}
}

// sendChatOptions sends the chat-filter settings packet to the owning
// player, or drops it when no RelationsSender is attached.
func (r *PlayerRelations) sendChatOptions(publicChat, privateChat, tradeChat int) {
	if sender, ok := r.sender(); ok {
		sender.SendChatOptions(publicChat, privateChat, tradeChat)
	}
}

// deliverPrivateMessage delivers a private message from sender to the
// owning player, or drops it when no RelationsSender is attached. It is
// called on the RECIPIENT's relations object; see Message.
func (r *PlayerRelations) deliverPrivateMessage(sender *Player, message []byte, size int) {
	if s, ok := r.sender(); ok {
		s.SendPrivateMessage(sender, message, size)
	}
}

// GetPrivateMessageId returns the next private-message id and advances
// the counter: getPrivateMessageId (PlayerRelations.ts:35-37), which
// returns privateMessageId++. The counter starts at 1. A nil relations
// object reads 0 and advances nothing.
func (r *PlayerRelations) GetPrivateMessageId() int {
	if r == nil {
		return 0
	}
	id := r.privateMessageID
	r.privateMessageID++
	return id
}

// SetPrivateMessageId replaces the private-message id counter and
// returns the relations object: setPrivateMessageId
// (PlayerRelations.ts:39-42). A nil receiver is a no-op returning nil.
func (r *PlayerRelations) SetPrivateMessageId(id int) *PlayerRelations {
	if r == nil {
		return nil
	}
	r.privateMessageID = id
	return r
}

// SetStatus replaces the private-chat status and, when update is true,
// broadcasts the presence change with UpdateLists(true): setStatus
// (PlayerRelations.ts:44-50). It returns the relations object. A nil
// receiver is a no-op returning nil.
func (r *PlayerRelations) SetStatus(status PrivateChatStatus, update bool) *PlayerRelations {
	if r == nil {
		return nil
	}
	r.status = status
	if update {
		r.UpdateLists(true)
	}
	return r
}

// GetStatus returns the private-chat status: getStatus
// (PlayerRelations.ts:52-54). A nil relations object reads ON, the TS
// field default.
func (r *PlayerRelations) GetStatus() PrivateChatStatus {
	if r == nil {
		return PrivateChatOn
	}
	return r.status
}

// SetChatModes replaces the public, private, and trade chat modes with
// the exact clamping of setChatModes (PlayerRelations.ts:60-64):
// public is clamped to 0..4 (PUBLIC_MODE_AUTOCHAT), trade to 0..2, and
// private to 0..2 and cast to PrivateChatStatus. The status change
// does NOT broadcast (TS passes update=false); presence only changes
// on the next UpdateLists. Go ints are already integral, so the TS
// `| 0` truncation step has no equivalent here. A nil receiver is a
// no-op.
func (r *PlayerRelations) SetChatModes(publicMode, privateMode, tradeMode int) {
	if r == nil {
		return
	}
	r.publicChatMode = min(max(publicMode, 0), publicModeAutoChat)
	r.tradeChatMode = min(max(tradeMode, 0), 2)
	r.status = PrivateChatStatus(min(max(privateMode, 0), 2))
}

// GetPublicChatMode returns the public-chat mode (0 Show standard, 1
// Show friends, 2 Show none, 3 Hide, 4 Show autochat):
// getPublicChatMode (PlayerRelations.ts:66-68). A nil relations object
// reads 0.
func (r *PlayerRelations) GetPublicChatMode() int {
	if r == nil {
		return 0
	}
	return r.publicChatMode
}

// GetTradeChatMode returns the trade-chat mode (0..2):
// getTradeChatMode (PlayerRelations.ts:70-72). A nil relations object
// reads 0.
func (r *PlayerRelations) GetTradeChatMode() int {
	if r == nil {
		return 0
	}
	return r.tradeChatMode
}

// GetFriendList returns a copy of the friend list, in insertion order:
// getFriendList (PlayerRelations.ts:74-76). TS returns the live array,
// which PlayerSave mutates in place on load; Go callers must not mutate
// relations state through the accessor, so this returns a copy —
// wholesale replacement goes through LoadSaveState, and single entries
// through AddFriend/DeleteFriend. A nil relations object returns nil.
func (r *PlayerRelations) GetFriendList() []int64 {
	if r == nil {
		return nil
	}
	return append([]int64(nil), r.friendList...)
}

// GetIgnoreList returns a copy of the ignore list, in insertion order:
// getIgnoreList (PlayerRelations.ts:78-80). Same copy rule as
// GetFriendList: replacement goes through LoadSaveState. A nil
// relations object returns nil.
func (r *PlayerRelations) GetIgnoreList() []int64 {
	if r == nil {
		return nil
	}
	return append([]int64(nil), r.ignoreList...)
}

// HasFriend reports whether username is on the friend list: hasFriend
// (PlayerRelations.ts:82-84), a set lookup. A nil relations object
// reports false.
func (r *PlayerRelations) HasFriend(username int64) bool {
	if r == nil {
		return false
	}
	_, ok := r.friendSet[username]
	return ok
}

// HasIgnore reports whether username is on the ignore list: hasIgnore
// (PlayerRelations.ts:86-88), a set lookup. A nil relations object
// reports false.
func (r *PlayerRelations) HasIgnore(username int64) bool {
	if r == nil {
		return false
	}
	_, ok := r.ignoreSet[username]
	return ok
}

// CanReceivePublicChatFrom reports whether the owning player's client
// accepts a public-chat line from other: canReceivePublicChatFrom
// (PlayerRelations.ts:90-96), with the exact semantics. Ignored
// senders never deliver, in any mode. Mode 4 (Show autochat) accepts
// everything, including autochat. Autochat is otherwise rejected.
// Modes 0 (Show standard) and 1 (Show friends, friends only) accept
// standard chat; modes 2/3 accept nothing. A nil sender never
// delivers, and a nil relations object receives nothing. The method
// does not exempt the speaker: the broadcast loop applies the self
// exemption (ChatPacketListener.ts:63 skips the gate for recipient ==
// speaker).
func (r *PlayerRelations) CanReceivePublicChatFrom(other *Player, autoChat bool) bool {
	if r == nil || other == nil || r.HasIgnore(other.GetLongUsername()) {
		return false
	}
	if r.publicChatMode == publicModeAutoChat {
		return true
	}
	if autoChat {
		return false
	}
	return r.publicChatMode == 0 || (r.publicChatMode == 1 && r.HasFriend(other.GetLongUsername()))
}

// IsVisibleTo reports whether the owning player shows as online to
// other: isVisibleTo (PlayerRelations.ts:99-102). Presence is
// controlled by private chat, independently of the ignore list (the TS
// comment at line 98): OFF is invisible to everyone, ON is visible to
// everyone, FRIENDS_ONLY is visible to friends only. A nil other, or a
// nil relations object, is never visible-to.
func (r *PlayerRelations) IsVisibleTo(other *Player) bool {
	if r == nil || other == nil {
		return false
	}
	return r.status != PrivateChatOff &&
		(r.status == PrivateChatOn || r.HasFriend(other.GetLongUsername()))
}

// CanReceivePrivateMessageFrom reports whether the owning player
// accepts a private message from other: canReceivePrivateMessageFrom
// (PlayerRelations.ts:104-106) — visible to the sender, and not
// ignoring them. A nil other, or a nil relations object, cannot
// receive.
func (r *PlayerRelations) CanReceivePrivateMessageFrom(other *Player) bool {
	return r.IsVisibleTo(other) && !r.HasIgnore(otherHash(other))
}

// otherHash returns other's long username, or 0 for a nil other. It
// exists so CanReceivePrivateMessageFrom can call HasIgnore without a
// nil dereference; IsVisibleTo already rejected the nil case, so the
// value is never consulted for a nil other.
func otherHash(other *Player) int64 {
	if other == nil {
		return 0
	}
	return other.GetLongUsername()
}

// GetFriendRank returns username's friend rank, or 0 when no rank is
// stored: getFriendRank (PlayerRelations.ts:108-110). Unlike
// GetFriendRanks, it does not check list membership — the ranks map is
// kept pruned to listed friends by every path that changes the list,
// so the two agree. A nil relations object reads 0.
func (r *PlayerRelations) GetFriendRank(username int64) int {
	if r == nil {
		return 0
	}
	return r.friendRanks[username]
}

// SetFriendRank sets username's friend rank, clamped to 0..6, and
// reports whether it did: setFriendRank (PlayerRelations.ts:112-116).
// It returns false, storing nothing, when username is not a friend. A
// nil receiver returns false.
func (r *PlayerRelations) SetFriendRank(username int64, rank int) bool {
	if r == nil || !r.HasFriend(username) {
		return false
	}
	r.friendRanks[username] = min(max(rank, 0), 6)
	return true
}

// GetFriendRanks returns the stored friend ranks, keyed by the
// decimal string of the name hash: getFriendRanks
// (PlayerRelations.ts:118-124). Only ranks for listed friends survive
// — a rank whose friend left the list is omitted (and pruned by
// rebuildRelationSets). The string keys are the TS Record<string,
// number> shape, matching the save's friendRanks field directly. The
// result is a fresh non-nil map. A nil relations object returns an
// empty map.
func (r *PlayerRelations) GetFriendRanks() map[string]int {
	ranks := make(map[string]int)
	if r == nil {
		return ranks
	}
	for username, rank := range r.friendRanks {
		if r.HasFriend(username) {
			ranks[strconv.FormatInt(username, 10)] = rank
		}
	}
	return ranks
}

// LoadFriendRanks replaces the stored friend ranks from save-shaped
// values: loadFriendRanks (PlayerRelations.ts:126-137). The map is
// cleared first; then each entry is kept only when its key parses as
// a name hash, the hash is on the friend list, and — implicitly, since
// Go ints are always finite — the rank is finite. Kept ranks are
// clamped to 0..6. A nil map clears and stores nothing, as TS does for
// null/undefined. Keys are decimal strings (the save shape); a key
// that does not parse as a base-10 int64 is skipped, as TS skips keys
// BigInt() rejects — except that TS BigInt also accepts hexadecimal
// ("0x10"), which this port rejects: normalizeRelationListToStrings
// never produces such keys, so no save carries one. Callers must load
// the friend list first (LoadSaveState does): ranks for unlisted
// friends are dropped. A nil receiver is a no-op.
func (r *PlayerRelations) LoadFriendRanks(ranks map[string]int) {
	if r == nil {
		return
	}
	r.friendRanks = make(map[int64]int)
	if ranks == nil {
		return
	}
	for rawName, rawRank := range ranks {
		username, err := strconv.ParseInt(strings.TrimSpace(rawName), 10, 64)
		if err != nil {
			continue
		}
		if r.HasFriend(username) {
			r.friendRanks[username] = min(max(rawRank, 0), 6)
		}
	}
}

// GetFriendsChatChannelName returns the friends-chat channel name:
// getFriendsChatChannelName (PlayerRelations.ts:139-141). A nil
// relations object reads "".
func (r *PlayerRelations) GetFriendsChatChannelName() string {
	if r == nil {
		return ""
	}
	return r.channelName
}

// SetFriendsChatChannelName replaces the friends-chat channel name:
// setFriendsChatChannelName (PlayerRelations.ts:143-145). A nil
// receiver is a no-op.
func (r *PlayerRelations) SetFriendsChatChannelName(name string) {
	if r == nil {
		return
	}
	r.channelName = name
}

// GetFriendsChatLastOwner returns the last friends-chat channel owner:
// getFriendsChatLastOwner (PlayerRelations.ts:147-149). A nil relations
// object reads "".
func (r *PlayerRelations) GetFriendsChatLastOwner() string {
	if r == nil {
		return ""
	}
	return r.lastOwner
}

// SetFriendsChatLastOwner replaces the last friends-chat channel
// owner: setFriendsChatLastOwner (PlayerRelations.ts:151-153). A nil
// receiver is a no-op.
func (r *PlayerRelations) SetFriendsChatLastOwner(owner string) {
	if r == nil {
		return
	}
	r.lastOwner = owner
}

// GetFriendsChatEntryRank returns the friends-chat entry rank:
// getFriendsChatEntryRank (PlayerRelations.ts:155-157). A nil relations
// object reads -1, the TS field default.
func (r *PlayerRelations) GetFriendsChatEntryRank() int {
	if r == nil {
		return -1
	}
	return r.entryRank
}

// GetFriendsChatTalkRank returns the friends-chat talk rank:
// getFriendsChatTalkRank (PlayerRelations.ts:159-161). A nil relations
// object reads -1, the TS field default.
func (r *PlayerRelations) GetFriendsChatTalkRank() int {
	if r == nil {
		return -1
	}
	return r.talkRank
}

// GetFriendsChatKickRank returns the friends-chat kick rank:
// getFriendsChatKickRank (PlayerRelations.ts:163-165). A nil relations
// object reads 2, the TS field default.
func (r *PlayerRelations) GetFriendsChatKickRank() int {
	if r == nil {
		return 2
	}
	return r.kickRank
}

// SetFriendsChatRanks replaces the friends-chat entry, talk, and kick
// ranks with the exact clamping of setFriendsChatRanks
// (PlayerRelations.ts:167-171): entry and talk clamp to -1..7, kick to
// 2..7. A nil receiver is a no-op.
func (r *PlayerRelations) SetFriendsChatRanks(entryRank, talkRank, kickRank int) {
	if r == nil {
		return
	}
	r.entryRank = min(max(entryRank, -1), 7)
	r.talkRank = min(max(talkRank, -1), 7)
	r.kickRank = min(max(kickRank, 2), 7)
}

// rebuildRelationSets rebuilds the friend/ignore sets from the lists
// and drops ranks for friends no longer listed: rebuildRelationSets
// (PlayerRelations.ts:173-179). It does not dedupe the lists — TS
// rebuilds the sets only, and list hygiene is the loaders' job (see
// normalizeRelationList). A nil receiver is a no-op.
func (r *PlayerRelations) rebuildRelationSets() {
	if r == nil {
		return
	}
	r.friendSet = make(map[int64]struct{}, len(r.friendList))
	for _, username := range r.friendList {
		r.friendSet[username] = struct{}{}
	}
	r.ignoreSet = make(map[int64]struct{}, len(r.ignoreList))
	for _, username := range r.ignoreList {
		r.ignoreSet[username] = struct{}{}
	}
	for username := range r.friendRanks {
		if _, ok := r.friendSet[username]; !ok {
			delete(r.friendRanks, username)
		}
	}
}

// UpdateLists broadcasts the owning player's presence to every
// registered player who lists them, and refreshes the owning player's
// view of every registered player they list: updateLists
// (PlayerRelations.ts:181-219). It returns the relations object.
//
// The order is TS-exact: rebuild the sets; force online=false when the
// status is OFF; send friend status 2 to self; then, for each other
// player in slot order — if other lists the owner, send other the
// owner's hash with world 1 when the owner is visible to other, else
// 0; if the owner lists other, send the owner other's hash with world
// 1 when other is visible to the owner, else 0. Nil world entries are
// skipped, and the owner is not excluded from the iteration (TS does
// not exclude self either; a self hash can only reach a list through
// a save, since AddFriend refuses self-adds).
//
// With no RelationsWorld installed (relationsWorld == nil), only the
// self send happens: the friend-status packet still goes out, and no
// peer traffic is produced. A nil receiver is a no-op returning nil.
func (r *PlayerRelations) UpdateLists(online bool) *PlayerRelations {
	if r == nil {
		return nil
	}
	r.rebuildRelationSets()
	if r.status == PrivateChatOff {
		online = false
	}
	r.sendFriendStatus(2)
	if relationsWorld == nil {
		return r
	}
	selfHash := r.playerHash()
	for _, other := range relationsWorld.RelationsPlayers() {
		if other == nil {
			continue
		}
		otherRelations := other.Relations()
		if otherRelations.HasFriend(selfHash) {
			temporaryOnlineStatus := online
			if !r.IsVisibleTo(other) {
				temporaryOnlineStatus = false
			}
			otherRelations.sendFriend(selfHash, boolToWorld(temporaryOnlineStatus))
		}
		if r.HasFriend(other.GetLongUsername()) {
			otherVisibleToPlayer := true
			if !otherRelations.IsVisibleTo(r.player) {
				otherVisibleToPlayer = false
			}
			r.sendFriend(other.GetLongUsername(), boolToWorld(otherVisibleToPlayer))
		}
	}
	return r
}

// boolToWorld maps an online flag to the sendFriend world argument: 1
// online, 0 offline (PlayerRelations.ts:200-203, 211-214).
func boolToWorld(online bool) int {
	if online {
		return 1
	}
	return 0
}

// SendPrivateStatus sends the owning player's chat-filter settings to
// their client: sendPrivateStatus (PlayerRelations.ts:221-224). The
// private byte maps the status: OFF 2, FRIENDS_ONLY 1, ON 0. A nil
// receiver, or a session without RelationsSender, sends nothing.
func (r *PlayerRelations) SendPrivateStatus() {
	if r == nil {
		return
	}
	privateChat := 0
	switch r.status {
	case PrivateChatOff:
		privateChat = 2
	case PrivateChatFriendsOnly:
		privateChat = 1
	}
	r.sendChatOptions(r.publicChatMode, privateChat, r.tradeChatMode)
}

// SendFriends sends every friend on the list, each as offline (world
// 0), skipping zero hashes: sendFriends (PlayerRelations.ts:226-232).
// A nil receiver, or a session without RelationsSender, sends nothing.
func (r *PlayerRelations) SendFriends() {
	if r == nil {
		return
	}
	for _, l := range r.friendList {
		if l != 0 {
			r.sendFriend(l, 0)
		}
	}
}

// SendIgnores sends every ignore on the list, skipping zero hashes:
// sendIgnores (PlayerRelations.ts:234-240). A nil receiver, or a
// session without RelationsSender, sends nothing.
func (r *PlayerRelations) SendIgnores() {
	if r == nil {
		return
	}
	for _, l := range r.ignoreList {
		if l != 0 {
			r.sendAddIgnore(l)
		}
	}
}

// SendAddFriend sends one friend add (world 0) to the owning player:
// sendAddFriend (PlayerRelations.ts:242-244). A nil receiver, or a
// session without RelationsSender, sends nothing.
func (r *PlayerRelations) SendAddFriend(name int64) {
	r.sendFriend(name, 0)
}

// SendDeleteFriend sends one friend removal to the owning player:
// sendDeleteFriend (PlayerRelations.ts:246-248). A nil receiver, or a
// session without RelationsSender, sends nothing.
func (r *PlayerRelations) SendDeleteFriend(name int64) {
	r.sendDeleteFriend(name)
}

// SendAddIgnore sends one ignore add to the owning player:
// sendAddIgnore (PlayerRelations.ts:250-252). A nil receiver, or a
// session without RelationsSender, sends nothing.
func (r *PlayerRelations) SendAddIgnore(name int64) {
	r.sendAddIgnore(name)
}

// SendDeleteIgnore sends one ignore removal to the owning player:
// sendDeleteIgnore (PlayerRelations.ts:254-256). A nil receiver, or a
// session without RelationsSender, sends nothing.
func (r *PlayerRelations) SendDeleteIgnore(name int64) {
	r.sendDeleteIgnore(name)
}

// OnLogin runs the login relations sends — ignores, then friends, then
// the chat-filter settings — after rebuilding the sets: onLogin
// (PlayerRelations.ts:258-264). It returns the relations object. The TS
// signature takes a player parameter that its body never uses; the Go
// form drops it (the owning player is already held). A nil receiver is
// a no-op returning nil.
func (r *PlayerRelations) OnLogin() *PlayerRelations {
	if r == nil {
		return nil
	}
	r.rebuildRelationSets()
	r.SendIgnores()
	r.SendFriends()
	r.SendPrivateStatus()
	return r
}

// AddFriend adds username to the friend list, or refuses with the
// exact TS message: addFriend (PlayerRelations.ts:266-292). Self-adds
// return silently. A full list (200) refuses with "Your friend list is
// full!". A username on the ignore list refuses with "Please remove
// <shown> from your ignore list first." — the ignore check precedes the
// duplicate check, as in TS. A username already listed notices "<shown>
// is already on your friends list!" and adds nothing. Otherwise the
// username is appended with rank 0, the add is sent, presence is
// broadcast with UpdateLists(true), and the named player, when online,
// broadcasts too. <shown> is shownName(FormatName(LongToString(hash))):
// the listed account's display name when it has one, else the formatted
// name. A nil receiver is a no-op.
func (r *PlayerRelations) AddFriend(username int64) {
	if r == nil {
		return
	}
	name := FormatName(LongToString(username))
	if username == r.playerHash() {
		return
	}
	if len(r.friendList) >= MaxFriends {
		r.player.SendMessage("Your friend list is full!")
		return
	}
	if r.HasIgnore(username) {
		r.player.SendMessage("Please remove " + shownName(name) + " from your ignore list first.")
		return
	}
	if r.HasFriend(username) {
		r.player.SendMessage(shownName(name) + " is already on your friends list!")
		return
	}
	r.friendList = append(r.friendList, username)
	r.friendSet[username] = struct{}{}
	r.friendRanks[username] = 0
	r.SendAddFriend(username)
	r.UpdateLists(true)
	if relationsWorld != nil {
		if friend := relationsWorld.RelationsPlayerByName(name); friend != nil {
			friend.Relations().UpdateLists(true)
		}
	}
}

// IsFriendWith reports whether the named player is on the friend list:
// isFriendWith (PlayerRelations.ts:294-296), hashing the name with
// StringToLong (Misc.stringToLongBigInt). A nil relations object
// reports false.
func (r *PlayerRelations) IsFriendWith(player string) bool {
	return r.HasFriend(StringToLong(player))
}

// DeleteFriend removes username from the friend list, or notices that
// it is not listed: deleteFriend (PlayerRelations.ts:298-317). A
// self-delete returns silently. A listed username is spliced out, its
// set entry and rank dropped, the removal sent, presence broadcast
// with UpdateLists(false), and the named player, when online,
// broadcasts with UpdateLists(false) too. An unlisted username
// notices "This player is not on your friends list!". A nil receiver
// is a no-op.
func (r *PlayerRelations) DeleteFriend(username int64) {
	if r == nil {
		return
	}
	name := FormatName(LongToString(username))
	if username == r.playerHash() {
		return
	}
	if index := indexOfHash(r.friendList, username); index != -1 {
		r.friendList = spliceOut(r.friendList, index)
		delete(r.friendSet, username)
		delete(r.friendRanks, username)
		r.SendDeleteFriend(username)
		r.UpdateLists(false)
		if relationsWorld != nil {
			if unfriend := relationsWorld.RelationsPlayerByName(name); unfriend != nil {
				unfriend.Relations().UpdateLists(false)
			}
		}
		return
	}
	r.player.SendMessage("This player is not on your friends list!")
}

// AddIgnore adds username to the ignore list, or refuses with the exact
// TS message: addIgnore (PlayerRelations.ts:319-344). Self-adds return
// silently. A full list (100) refuses with "Your ignore list is full!".
// A username on the friend list refuses with "Please remove <shown>
// from your friend list first." — the friend check precedes the
// duplicate check, as in TS. A username already listed notices "<shown>
// is already on your ignore list!" and adds nothing. Otherwise the
// username is appended, the add is sent, presence is broadcast with
// UpdateLists(true), and the named player, when online, broadcasts
// too. <shown> is shownName(FormatName(LongToString(hash))), as in
// AddFriend. A nil receiver is a no-op.
func (r *PlayerRelations) AddIgnore(username int64) {
	if r == nil {
		return
	}
	name := FormatName(LongToString(username))
	if username == r.playerHash() {
		return
	}
	if len(r.ignoreList) >= MaxIgnores {
		r.player.SendMessage("Your ignore list is full!")
		return
	}
	if r.HasFriend(username) {
		r.player.SendMessage("Please remove " + shownName(name) + " from your friend list first.")
		return
	}
	if r.HasIgnore(username) {
		r.player.SendMessage(shownName(name) + " is already on your ignore list!")
		return
	}
	r.ignoreList = append(r.ignoreList, username)
	r.ignoreSet[username] = struct{}{}
	r.SendAddIgnore(username)
	r.UpdateLists(true)
	if relationsWorld != nil {
		if ignored := relationsWorld.RelationsPlayerByName(name); ignored != nil {
			ignored.Relations().UpdateLists(true)
		}
	}
}

// DeleteIgnore removes username from the ignore list, or notices that
// it is not listed: deleteIgnore (PlayerRelations.ts:346-366). A
// self-delete returns silently. A listed username is spliced out, the
// removal sent, and presence broadcast with UpdateLists(true); the
// named player, when online, broadcasts too — but only when the status
// is ON (TS gates the peer update on PrivateChatStatus.ON, line 357).
// An unlisted username notices "This player is not on your ignore
// list!". A nil receiver is a no-op.
func (r *PlayerRelations) DeleteIgnore(username int64) {
	if r == nil {
		return
	}
	name := FormatName(LongToString(username))
	if username == r.playerHash() {
		return
	}
	if index := indexOfHash(r.ignoreList, username); index != -1 {
		r.ignoreList = spliceOut(r.ignoreList, index)
		delete(r.ignoreSet, username)
		r.SendDeleteIgnore(username)
		r.UpdateLists(true)
		if r.status == PrivateChatOn && relationsWorld != nil {
			if ignored := relationsWorld.RelationsPlayerByName(name); ignored != nil {
				ignored.Relations().UpdateLists(true)
			}
		}
		return
	}
	r.player.SendMessage("This player is not on your ignore list!")
}

// Message delivers a private message from the owning player to friend:
// message (PlayerRelations.ts:368-376). When friend cannot receive a
// private message from the owner (offline, invisible, or ignoring),
// the owner is told "This player is currently offline." and nothing is
// delivered. When the owner's status is OFF, the owner is promoted to
// FRIENDS_ONLY with a presence broadcast (SetStatus with update=true)
// before delivery. Delivery goes to friend's session through the
// RelationsSender seam, carrying the packed payload and its size
// verbatim. A nil friend is treated as undeliverable (the offline
// notice); TS would throw on a null friend, but no live caller can
// pass one — see below. A nil receiver is a no-op.
//
// NO LIVE TS CALLER. A grep over the TS core and the plugin sources at
// commit bfd81e3 finds zero callers of PlayerRelations.message:
// private_message packets reach the FriendsList plugin's social-packet
// handler, which delivers through FriendsChatManager
// (FriendsList.plugin.js:118-123), not through this method. It is
// ported whole anyway, per the Phase 19 scope: it is engine API on
// PlayerRelations, and the Go friends-chat content may call it. The
// OFF-to-FRIENDS_ONLY promotion and the offline notice are its
// observable behaviors, and both are pinned by tests.
func (r *PlayerRelations) Message(friend *Player, message []byte, size int) {
	if r == nil {
		return
	}
	if friend == nil || !friend.Relations().CanReceivePrivateMessageFrom(r.player) {
		r.player.SendMessage("This player is currently offline.")
		return
	}
	if r.status == PrivateChatOff {
		r.SetStatus(PrivateChatFriendsOnly, true)
	}
	friend.Relations().deliverPrivateMessage(r.player, message, size)
}

// indexOfHash returns the first index of username in list, or -1.
// Array.indexOf in TS (PlayerRelations.ts:303, 349).
func indexOfHash(list []int64, username int64) int {
	for i, v := range list {
		if v == username {
			return i
		}
	}
	return -1
}

// RelationsSaveState is the save-shaped relations state moved by
// LoadSaveState/SnapshotSaveState (contract C3): the relations fields
// of PlayerSave.applyToPlayer/fromPlayer (PlayerSave.ts:699-725,
// 819-832). Friends and Ignores are name hashes as int64 slices —
// persist converts the save's decimal-string lists to and from this
// form, applying its normalization. FriendRanks keeps the save's
// Record<string, number> shape: decimal-string hash keys, matching
// GetFriendRanks/LoadFriendRanks and the save's friendRanks field
// directly. The channel ranks carry the PlayerSave getter defaults
// (-1/-1/2, PlayerSave.ts:438-447) when the save holds no value; the
// persist layer supplies those defaults before loading, and
// SetFriendsChatRanks clamps whatever arrives.
type RelationsSaveState struct {
	Friends                []int64
	Ignores                []int64
	FriendRanks            map[string]int
	FriendsChatChannelName string
	FriendsChatLastOwner   string
	FriendsChatEntryRank   int
	FriendsChatTalkRank    int
	FriendsChatKickRank    int
}

// LoadSaveState replaces the whole relations state from save-shaped
// values, in the applyToPlayer order (PlayerSave.ts:699-725): the
// friend and ignore lists are replaced wholesale, friend ranks are
// loaded (only ranks for listed friends survive), and the friends-chat
// channel name, last owner, and ranks are set. The caller passes
// already-normalized lists (persist's normalize step), but the load
// re-applies normalization defensively — positive, unique, capped,
// first-occurrence order — so duplicates, zeros, and over-long input
// can never corrupt live state. Sets are rebuilt from the replaced
// lists and stale ranks pruned, per rebuildRelationSets. A nil
// receiver is a no-op.
func (r *PlayerRelations) LoadSaveState(state RelationsSaveState) {
	if r == nil {
		return
	}
	r.friendList = normalizeRelationList(state.Friends, MaxFriends)
	r.ignoreList = normalizeRelationList(state.Ignores, MaxIgnores)
	r.rebuildRelationSets()
	r.LoadFriendRanks(state.FriendRanks)
	r.SetFriendsChatChannelName(state.FriendsChatChannelName)
	r.SetFriendsChatLastOwner(state.FriendsChatLastOwner)
	r.SetFriendsChatRanks(state.FriendsChatEntryRank, state.FriendsChatTalkRank, state.FriendsChatKickRank)
}

// SnapshotSaveState returns the whole relations state in save-shaped
// form, mirroring fromPlayer (PlayerSave.ts:819-832): normalized lists
// (positive, unique, capped), the listed-only friend ranks, and the
// friends-chat channel fields. The returned slices and map are fresh
// copies — mutating them does not affect live state — and are non-nil
// even when empty, matching the TS []/{} shapes. A nil relations
// object snapshots as a fresh player: empty lists and ranks, empty
// channel fields, entry/talk -1, kick 2.
func (r *PlayerRelations) SnapshotSaveState() RelationsSaveState {
	if r == nil {
		return RelationsSaveState{
			Friends:              []int64{},
			Ignores:              []int64{},
			FriendRanks:          map[string]int{},
			FriendsChatEntryRank: -1,
			FriendsChatTalkRank:  -1,
			FriendsChatKickRank:  2,
		}
	}
	return RelationsSaveState{
		Friends:                normalizeRelationList(r.friendList, MaxFriends),
		Ignores:                normalizeRelationList(r.ignoreList, MaxIgnores),
		FriendRanks:            r.GetFriendRanks(),
		FriendsChatChannelName: r.channelName,
		FriendsChatLastOwner:   r.lastOwner,
		FriendsChatEntryRank:   r.entryRank,
		FriendsChatTalkRank:    r.talkRank,
		FriendsChatKickRank:    r.kickRank,
	}
}

// normalizeRelationList returns values cleaned the way
// PlayerSave.normalizeRelationListToBigInt cleans them
// (PlayerSave.ts:482-508): non-positive values dropped, duplicates
// dropped keeping the first occurrence, order preserved, capped at
// max entries. It always returns a non-nil slice. Persist applies
// the same rule to the save's string lists; load and snapshot
// re-apply it here so either direction stays safe on malformed input.
func normalizeRelationList(values []int64, max int) []int64 {
	out := []int64{}
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= max {
			break
		}
	}
	return out
}
