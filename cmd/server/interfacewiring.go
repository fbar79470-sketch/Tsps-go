package main

import (
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// Interface runtime wiring (phase 18, Worker 1): installs the
// per-player interface deps on an entering session, binding the
// session's sender machinery (internal/session/interface_*.go)
// to the owning player, the world's ShopManager, the loaded
// definition Store (for configured-interface layouts), and the
// plugin hub (for the interface:closed / interface:changed /
// interface:opened custom events).
//
// COORDINATOR CALL SITE (this file must not edit server.go):
// in worldAdapter.AddPlayer, inside the `if ep.Session != nil`
// wiring block, alongside the other per-session installs
// (AttachSyncSource, SetMusicData, InstallObjectSync,
// installSkillsAppearanceWiring):
//
//	installInterfaceRuntime(ep.Session, p, a.store, a.shops, a.plugins)
//
// The call runs before QueueAddPlayer, so no tick can reach
// the session before its deps exist, and the player's
// EmitCustomEvent sink is installed before the login hooks
// fire (EmitPlayerLogin runs after entry, in the same
// function).
//
// NIL TOLERANCE. A nil session or player installs nothing. A
// nil hub leaves the emit sink and the closed/opened deps
// unset (events drop, as an unwired session does). A nil
// ShopManager leaves CloseShop unset (resetInterfaceState
// skips the shop close). A nil Store leaves layouts unset
// (every configured reference is unknown). A standalone
// adapter passes nils and the session degrades to
// send-and-track-only, exactly as the unwired behavior the
// session tests pin.
//
// DELIBERATELY UNSET (gaps, not oversights):
//   - GameframeRoot: the bootstrap's computed root is not
//     retained on the session; the 161 default applies.
//   - The varp store needs no wiring here: Worker 3's
//     SetVarbitResolver installs the resolver, and the
//     sender's SendVarp/SendVarbit calls record through the
//     store NewSession creates.
//
// SetStatusNone and EndBankSearch were deliberately unset
// until phase 46 (no PlayerStatus field / no bank search
// state existed); both are wired below now.
func installInterfaceRuntime(sess *session.Session, p *game.Player, store *definition.Store, shops *game.ShopManager, hub *plugins.Hub) {
	if sess == nil {
		return
	}
	if store != nil {
		sess.SetInterfaceLayouts(store)
	}
	if p == nil {
		return
	}
	if hub != nil {
		p.EmitCustomEvent = hub.EmitCustomEvent
	}
	deps := session.InterfaceDeps{
		GetInterfaceID:      p.GetInterfaceId,
		SetInterfaceID:      p.SetInterfaceId,
		HasInputPrompt:      p.HasInputPrompt,
		AbortEnteredActions: p.AbortEnteredActions,
		ResetDialogue: func() {
			if dm := p.GetDialogueManager(); dm != nil {
				dm.Reset()
			}
		},
		DialogueOwner: func() any {
			if dm := p.GetDialogueManager(); dm != nil {
				return dm.GetOwner()
			}
			return nil
		},
		SetDestroyPending: func() {
			p.SetAttribute("destroy-item:pending", -1)
		},
		SetCanChangeAppearance: func(can bool) {
			if app := p.Appearance(); app != nil {
				app.SetCanChangeAppearance(can)
			}
		},
		SetCreationMenu: p.SetCreationMenu,
		// Phase 46: the status field (status.go) and the bank
		// search state (bankstate.go) both exist now.
		SetStatusNone: p.SetStatusNone,
		EndBankSearch: func(closedInterfaceID int) {
			// PacketSender.ts:902-906: only the bank's own
			// close ends a bank search.
			if closedInterfaceID == game.BankMainInterfaceID {
				game.BankCloseSearch(p)
			}
		},
	}
	if hub != nil {
		deps.EmitInterfaceClosed = func(interfaceID int) {
			hub.EmitCustomEvent(game.InterfaceClosedEventName, game.InterfaceClosedEvent{
				Player:      p,
				InterfaceID: interfaceID,
			})
		}
		deps.EmitInterfaceOpened = func(groupID, targetUID, typ int) {
			hub.EmitCustomEvent(game.InterfaceOpenedEventName, game.InterfaceOpenedEvent{
				Player:    p,
				GroupID:   groupID,
				TargetUID: targetUID,
				Type:      typ,
			})
		}
	}
	if shops != nil {
		deps.CloseShop = func() { shops.Close(p) }
	}
	sess.SetInterfaceDeps(deps)
}
