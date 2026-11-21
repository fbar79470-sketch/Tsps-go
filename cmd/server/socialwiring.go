// Social wiring: the coordinator-side seams for friends/ignores,
// clans, and the social-packet dispatch (phase 19, migration
// milestone 9).
//
// WHAT LIVES HERE
//
//   - installRelationsSeams: the two package-level seams Worker 1's
//     relations port reads — the world (player iteration + lookup)
//     and the shown-name lookup. Installed once at boot, before
//     the engine starts, next to the other world-hook installs.
//   - socialEmitAdapter: Deps.SocialEmit over Hub.EmitSocialPacket,
//     installed per entering player in AddPlayer.
//   - applyEntryRelations: the enterWorld relations load
//     (NetworkBuilder.ts:1662 for the long username,
//     PlayerSave.ts:699-725 for the lists/ranks/channel fields),
//     run from prepareCarriedSave in both branches.
//   - The snapshot half lives in buildSnapshot (savewiring.go),
//     which feeds SnapshotSaveState into the WithOwned literal
//     through formatRelationHashes below.
//   - The logout half lives in onPlayerDisconnect (server.go),
//     which calls UpdateLists(false) after the disconnect hooks
//     and the shop/trade close, before the disconnect save —
//     the TS onLogout order (Player.ts:725-765).
//
// WHAT CORE DOES NOT DO (D1)
//
// Core never handles a social packet beyond the hub emit, and
// never calls relations OnLogin sends: in TS the FriendsList
// plugin calls onLogin (sending ignores, friends, and the
// private status) and owns add-friend / private-message /
// chat-filter behavior. The encoders (friends-chat snapshot,
// clan snapshots/deltas) and the session senders exist for that
// content; until it ports, the snapshot/clan encoders have no
// producers and the legacy presence sends are no-ops (see
// internal/protocol/social.go and internal/session
// /social_sender.go).

package main

import (
	"strconv"

	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/persist"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// installRelationsSeams installs the relations port's world and
// shown-name seams (game.SetRelationsWorld /
// game.SetShownNameFunc). The world seam iterates the live world
// player list and looks players up by normalized name through
// Worker 1's NewWorldRelations adapter; the shown-name seam
// answers the display-names registry entry's name, or the input
// username unchanged when the account has no entry — exactly
// PlayerRelations.shownName (PlayerRelations.ts:31-33:
// DisplayNames.entryOf(username)?.name ?? username). Note the
// fallback is the INPUT unchanged, not NameOf's formatted login
// name: the two differ for unregistered names, and TS uses the
// input. A nil registry degrades to the identity fallback, as a
// nil world degrades updateLists to self-only.
func installRelationsSeams(world *game.World, displayNames *persist.DisplayNames) {
	game.SetRelationsWorld(game.NewWorldRelations(world))
	game.SetShownNameFunc(shownNameFunc(displayNames))
}

// shownNameFunc builds the shown-name lookup over a display-names
// registry: the entry's name when the account has one, the input
// username unchanged otherwise (see installRelationsSeams). A
// nil registry yields the identity function.
func shownNameFunc(displayNames *persist.DisplayNames) func(string) string {
	return func(username string) string {
		if displayNames != nil {
			if entry, ok := displayNames.EntryOf(username); ok {
				return entry.Name
			}
		}
		return username
	}
}

// socialEmitAdapter adapts Hub.EmitSocialPacket to the listeners
// SocialEmit shape (listeners.Deps.SocialEmit): one social
// packet in, whether a hook marked it handled out. A nil hub
// yields a nil emit — the no-hooks default — so AddPlayer need
// not special-case a standalone adapter. The event is built
// with the game.SocialPacket directly (contract D5): the hub
// event's Packet field IS game.SocialPacket since Worker 3's
// unification, and the plugins package keeps only kind alias
// constants. The adapter does not import the chat listener's
// intercept shape: public_chat goes through
// socialPacketIntercept (commandwiring.go), which synthesizes
// its packet from the chat line.
func socialEmitAdapter(hub *plugins.Hub) func(*game.Player, game.SocialPacket) bool {
	if hub == nil {
		return nil
	}
	return func(p *game.Player, pkt game.SocialPacket) bool {
		return hub.EmitSocialPacket(&plugins.SocialPacketEvent{Player: p, Packet: pkt})
	}
}

// installSocialListeners wires the social-packet emit onto a
// Deps value. It is called from AddPlayer next to the chat
// wiring; the nil-hub case leaves SocialEmit nil.
func installSocialListeners(deps *listeners.Deps, hub *plugins.Hub) {
	deps.SocialEmit = socialEmitAdapter(hub)
}

// applyEntryRelations loads the carried save's relations state
// onto the entering player and sets the player's long username:
// the enterWorld equivalents (NetworkBuilder.ts:1662 sets the
// long username from the pending username's base-37 hash; the
// applyToPlayer relations block, PlayerSave.ts:699-725, loads
// the friend/ignore lists, the friend ranks, and the channel
// fields). It runs in both prepareCarriedSave branches, after
// the entry live state and the attributes, reading only the
// relations fields of the (already migrated) save. The long
// username uses session.NameToLong, the entry-path hash; the
// lists convert from the save's decimal-string hashes, skipping
// unparseable entries (entry loading already normalized the
// lists through normalizeRelationList, and LoadSaveState
// re-normalizes defensively, so a skip here is unreachable for
// a loaded save and guards only a hand-built one).
func applyEntryRelations(p *game.Player, carried *persist.PlayerSave) {
	p.LongUsername = session.NameToLong(p.Username)
	p.Relations().LoadSaveState(game.RelationsSaveState{
		Friends:                parseRelationHashes(carried.Friends),
		Ignores:                parseRelationHashes(carried.Ignores),
		FriendRanks:            carried.FriendRanks,
		FriendsChatChannelName: carried.FriendsChatChannelName,
		FriendsChatLastOwner:   carried.FriendsChatLastOwner,
		FriendsChatEntryRank:   carried.FriendsChatEntryRank,
		FriendsChatTalkRank:    carried.FriendsChatTalkRank,
		FriendsChatKickRank:    carried.FriendsChatKickRank,
	})
}

// parseRelationHashes converts the save's decimal-string hashes
// to int64 hashes, skipping entries that do not parse.
func parseRelationHashes(hashes []string) []int64 {
	out := make([]int64, 0, len(hashes))
	for _, h := range hashes {
		if v, err := strconv.ParseInt(h, 10, 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// formatRelationHashes converts live int64 hashes back to the
// save's decimal-string form for the WithOwned snapshot (the
// fromPlayer shape, PlayerSave.ts:819-830). The result is
// non-nil even when empty, matching the TS [] shape.
func formatRelationHashes(hashes []int64) []string {
	out := make([]string, 0, len(hashes))
	for _, h := range hashes {
		out = append(out, strconv.FormatInt(h, 10))
	}
	return out
}
