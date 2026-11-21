package main

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Command wiring: the hub's command registry plus the dispatch
// seams the registry needs from the rest of the server. The
// registry itself is created lazily by the hub (Hub.Commands) so
// plugin Register functions can register commands during Load;
// this file installs everything Load cannot know, after the hub
// is built.
//
// The coordinator calls wireCommandRegistry from buildServer
// (server.go), after newPluginHub and before serving — the call
// site is the coordinator's: this worker owns no other cmd/server
// file. It returns the registry so the coordinator can hand it
// to the chat listener seam (Deps.Chat.Commands accepts a
// *game.CommandRegistry directly: DispatchCommand satisfies
// listeners.CommandDispatcher) and the social-packet intercept
// adapter below to Deps.Chat.SocialIntercept.
//
// Seams, and who supplies each:
//
//   - messenger: the engine's command replies (rights denial,
//     unknown command) go through player.sendMessage in TS
//     (Mobile.sendMessage, Mobile.ts:999-1006). The coordinator
//     supplies func(p *game.Player, msg string) {
//     p.SendMessage(msg) } once the phase 12 sendMessage work
//     lands; a nil messenger drops the replies (the registry
//     still dispatches and logs unhandled commands).
//   - rightsOf: the command rights gate reads the player's
//     PlayerRights id (PluginManager.playerHasCommandRights,
//     PluginManager.ts:2120-2130). The Go Player carries no
//     rights; the coordinator supplies the carried save's
//     Rights.ID (PlayerSave.applyToPlayer loads rights onto the
//     player, PlayerSave.ts:638). A nil source reads every
//     player as game.RightsNone: unrestricted commands run,
//     restricted commands deny — the fail-closed degenerate
//     behavior while rights are not live state.
//
// The world.json "commands:permissions" map is parsed from the
// hub's plugin config here (Hub.CommandPermissions) and
// installed on the registry; the onCommand any-command hooks
// are wired to the hub's emit (Hub.EmitCommandHooks), closing
// the emitCommand loop the registry's Emit runs.
func wireCommandRegistry(hub *plugins.Hub, messenger func(*game.Player, string), rightsOf func(*game.Player) int) *game.CommandRegistry {
	if hub == nil {
		return nil
	}
	reg := hub.Commands()
	if reg == nil {
		return nil
	}
	reg.SetPermissions(hub.CommandPermissions())
	reg.SetMessenger(messenger)
	reg.SetRightsSource(rightsOf)
	reg.SetCommandHooks(hub.EmitCommandHooks)
	return reg
}

// socialPacketIntercept adapts the hub's onSocialPacket emit to
// the chat listener's SocialIntercept hook shape
// (listeners.ChatHooks): one public-chat line in, whether a
// plugin marked it handled out. A nil hub yields an intercept
// that never intercepts (the no-hooks default).
func socialPacketIntercept(hub *plugins.Hub) func(*game.Player, string, bool) bool {
	return func(p *game.Player, text string, autoChat bool) bool {
		if hub == nil {
			return false
		}
		return hub.EmitSocialPacket(&plugins.SocialPacketEvent{
			Player: p,
			Packet: game.SocialPacket{
				Kind:     plugins.SocialPacketPublicChat,
				Text:     text,
				AutoChat: autoChat,
			},
		})
	}
}
