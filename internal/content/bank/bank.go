// Package bank is the Go port of the BankBooths plugin
// (server/plugins/objects/BankBooths.plugin.js): the bank
// interaction layer — opening the bank from bank booths, bank
// chests, and Banker NPCs, the banker dialogue action, and the
// legacy-button / interface-action routing into the bank core
// flows.
//
// Bank *storage* already exists (internal/game/bank.go, Phase
// 9: tab containers, Rearrange, placeholder support, the
// `banks` save Map). This package owns only the interaction
// layer, following the shopkeepers precedent (name-based
// bindings + hub families):
//
//   - OnObjectInteractionNamed("Bank booth", {"Bank", "Use"}),
//     ("Bank chest", {"Use"}), ("Bank Chest", {"Use"}).
//   - OnNpcInteraction("Banker", {"Talk-to", "Bank"}).
//   - OnCustomEvent("npc-dialogue:action", bankerDialogueAction).
//   - OnInterfaceActionClick (bankInterfaceAction + the widget
//     (group, child) dispatch into Worker B's handleBankWidget).
//   - OnButtonClick (bankButton → handleBankButton, action 0).
//     The hub family is Worker A's internal/plugins/buttons.go:
//     Hub.EmitButtonClick runs onButtonClick hooks, then
//     onButton hooks (PHASE46_PLAN.md §4).
//
// The actual open sequence and the button/widget flows live in
// Worker B's bank core (game.BankOpen / BankHandleButton /
// BankHandleWidgetAction, PHASE46_PLAN.md §4). This package
// calls them through the Flows seam (InstallFlows, installed
// from cmd/server/bankwiring.go — the coordinator calls
// installBankFlows from buildServer) and NEVER reimplements
// them: with no flow installed the open/button calls fall
// through (return false) instead of claiming a click the bank
// cannot answer yet.
//
// Explicitly OUT of this package (PHASE46_PLAN.md §1; each is a
// gap, not a silent omission): the platinum-token exchange, the
// rune-pouch-note exchange, banknote un-noting on a banker
// (all three need onItemOnNpc / onItemOnObject, which are
// table-only for the noted/unnoted filters the plugin uses),
// BankDepositBooth (interface 192, a separate deposit-only
// piece), "Collect" (Grand Exchange, not ported), Bank PIN, and
// BankSpace.
package bank

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// BankSettingsInterfaceID is the bank settings interface id
// (BANK_SETTINGS_INTERFACE_ID, BankBooths.plugin.js).
const BankSettingsInterfaceID = 32500

// BankTabSelectStart is the first tab-select button id
// (BANK_TAB_SELECT_START, BankBooths.plugin.js): tab-select
// buttons are 50070 + 4*tab.
const BankTabSelectStart = 50070

// BankOpenDebounceTicks is the open debounce window
// (BANK_OPEN_DEBOUNCE_TICKS, BankBooths.plugin.js): a second
// open within 1 tick of the last is swallowed.
const BankOpenDebounceTicks = 1

// bankMainButtonIDs is BANK_MAIN_BUTTON_IDS
// (BankBooths.plugin.js): the legacy button ids the bank
// answers on its main interface (12).
var bankMainButtonIDs = map[int]bool{
	50013: true, // show settings menu
	5386:  true, // note on
	5387:  true, // note off
	8130:  true, // swap
	8131:  true, // insert
	50004: true, // deposit inventory
	50007: true, // deposit worn
	5384:  true, // close
	50001: true, // close
	50010: true, // search
}

// bankSettingsButtonIDs is BANK_SETTINGS_BUTTON_IDS
// (BankBooths.plugin.js): the legacy button ids the bank
// answers on its settings interface (32500).
var bankSettingsButtonIDs = map[int]bool{
	32503: true, // close settings
	32512: true, // re-open bank
	32513: true, // placeholder toggle
}

// lastBankOpenTickAttribute is the per-player last-open tick
// (the Go form of the TS module-level WeakMap `lastBankOpenAt`
// in BankBooths.plugin.js): a transient player attribute, not
// persisted — the cooking precedent for a transient per-player
// tick (cooking:leaping-fish-cut-tick, leaping_fish.go). It dies
// with the player, as the WeakMap entry does, and needs no
// package-level map (which Go cannot weak-key).
const lastBankOpenTickAttribute = "bank:last-open-tick"

// Flows is the Worker B bank-core seam (PHASE46_PLAN.md §4).
// Every field is a Worker B flow this package delegates to and
// must not reimplement:
//
//   - IsOpen answers Bank.isOpen (status == BANKING &&
//     interface id == 12). The coordinator wires
//     game.BankIsOpen here (landed, Worker A,
//     internal/game/bankstate.go); with no flow installed
//     bankIsOpen reads it directly.
//   - Open runs the Bank.open sequence (script 917, mount 12 +
//     15 with the varp/varbit bundle, flags ranges, varbit
//     12393 = 1, "Bank of <world>" string, sendBankRefresh,
//     CONTAINER_OPEN sound, bank:opened).
//   - HandleButton runs Bank.handleButton (the legacy button
//     route, Bank.ts:1098-1196).
//   - HandleWidget runs Bank.handleWidgetAction (the
//     cache-native widget route, Bank.ts:877-1032), dispatched
//     by (group, child).
//
// A nil field means the flow is not installed: callers fall
// through (return false) instead of claiming the click. The
// coordinator installs the real flows from
// cmd/server/bankwiring.go (installBankFlows, called from
// buildServer); tests install stubs through InstallFlows.
type Flows struct {
	IsOpen       func(p *game.Player) bool
	Open         func(p *game.Player) bool
	HandleButton func(p *game.Player, buttonID, action int) bool
	HandleWidget func(ev *plugins.InterfaceActionClickEvent) bool
}

// flows is the installed Worker B seam (see Flows). Tests
// install stubs through InstallFlows and restore the zero
// value when done.
var flows Flows

// InstallFlows installs the Worker B bank-core flows (see
// Flows). It is the coordinator's seam: called from
// cmd/server/bankwiring.go once Worker B's flows land. Calling
// it with the zero Flows uninstalls them.
func InstallFlows(f Flows) {
	flows = f
}

// Plugin is the registered BankBooths plugin value: name and
// description from the plugin file
// (BankBooths.plugin.js).
var Plugin = plugins.Plugin{
	Name:        "BankBooths",
	Description: "Opens the bank from bank booths, bank chests and Banker NPCs, and handles the bank tab and settings buttons.",
	Register: func(api *plugins.Api) {
		api.OnObjectInteractionNamed("Bank booth", map[string]plugins.ObjectInteractionHandler{
			"Bank": func(ev *plugins.ObjectInteractionEvent) bool { return openBank(api, ev.Player) },
			"Use":  func(ev *plugins.ObjectInteractionEvent) bool { return openBank(api, ev.Player) },
		})
		api.OnObjectInteractionNamed("Bank chest", map[string]plugins.ObjectInteractionHandler{
			"Use": func(ev *plugins.ObjectInteractionEvent) bool { return openBank(api, ev.Player) },
		})
		api.OnObjectInteractionNamed("Bank Chest", map[string]plugins.ObjectInteractionHandler{
			"Use": func(ev *plugins.ObjectInteractionEvent) bool { return openBank(api, ev.Player) },
		})
		api.OnNpcInteraction("Banker", map[string]plugins.NpcInteractionHandler{
			"Talk-to": func(ev *plugins.NpcInteractionEvent) bool { return talkToBanker(api, ev) },
			"Bank":    func(ev *plugins.NpcInteractionEvent) bool { return openBank(api, ev.Player) },
		})
		api.OnCustomEvent(plugins.NpcDialogueActionEventName, func(payload any) {
			bankerDialogueAction(api, payload)
		})
		api.OnButtonClick(func(ev *plugins.ButtonClickEvent) {
			if HandleBankButton(ev.Player, ev.ButtonID) {
				ev.Handled = true
			}
		})
		api.OnInterfaceActionClick(handleInterfaceActionClick)
	},
}

func init() {
	plugins.Register(Plugin)
}

// currentCycle is the current process cycle (TS
// World.getProcessCycle()): Api.Core().Cycle(), installed by
// the cmd/server adapter from world.ProcessCycle
// (pluginwiring.go: hub.SetCycleSource(world.ProcessCycle)) —
// nil-safe, 0 with no source (the cooking currentCycle
// precedent, leaping_fish.go:146-154).
func currentCycle(api *plugins.Api) int64 {
	if api == nil {
		return 0
	}
	return api.Core().Cycle()
}

// lastBankOpenTick reads the player's last-open tick: the
// stored int64, or -1 when the player never opened the bank
// (the TS `?? -Infinity` read, clamped to -1 — any tick >= 0
// is more than the 1-tick debounce away from it).
func lastBankOpenTick(player *game.Player) (int64, bool) {
	if player == nil {
		return -1, false
	}
	switch v := player.GetAttribute(lastBankOpenTickAttribute).(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return -1, false
	}
}

// bankIsOpen answers Bank.isOpen for this package (PHASE46_PLAN.md
// §4: status == BANKING && interface id == 12). With a Worker B
// flow installed it delegates; without one it reads the landed
// Worker A seam game.BankIsOpen (internal/game/bankstate.go),
// which is the contract itself over the real Player status
// field (status.go) — no interface-only approximation is
// needed anymore.
func bankIsOpen(player *game.Player) bool {
	if player == nil {
		return false
	}
	if flows.IsOpen != nil {
		return flows.IsOpen(player)
	}
	return game.BankIsOpen(player)
}

// openBank is openBank (BankBooths.plugin.js): the 1-tick open
// debounce, the already-open skip, the bot shortcut, and the
// delegation to Worker B's Bank.open flow. It returns true when
// the click is claimed (debounced, already open, bot-marked, or
// opened) and false when the bank cannot be opened yet — with
// no Open flow installed the click falls through instead of
// being swallowed by a half-open.
func openBank(api *plugins.Api, player *game.Player) bool {
	if player == nil {
		return false
	}
	now := currentCycle(api)
	if last, ok := lastBankOpenTick(player); ok && now-last < BankOpenDebounceTicks {
		return true
	}
	player.SetAttribute(lastBankOpenTickAttribute, now)

	if bankIsOpen(player) {
		return true
	}

	if player.IsPlayerBot() {
		// Bots do not need full interface/container refresh
		// work just to use bank operations. They only need
		// banking state + interface marker (the status half
		// runs on the landed Worker A seam, Player.SetStatus
		// in internal/game/status.go).
		player.SetStatus(game.PlayerStatusBanking)
		player.SetEnteredSyntaxAction(nil)
		player.SetInterfaceId(game.BankMainInterfaceID)
		return true
	}

	// The open sequence itself (script 917, mounts, flags,
	// refresh, sound, bank:opened) is Worker B's game.BankOpen,
	// reached through the Flows seam (installed from
	// cmd/server/bankwiring.go) — never reimplemented here.
	if flows.Open != nil {
		return flows.Open(player)
	}
	return false
}

// talkToBanker is talkToBanker (BankBooths.plugin.js): emit
// npc-dialogue:start with variant "standard-dialogue" and
// report whether the transcript claimed it. The Go equivalent
// request type is *game.StartNamedRequest
// (internal/game/transcript.go:1160; subscribed by the
// coordinator in cmd/server/dialoguewiring.go:
// SubscribeDialogueStart). The banker transcript itself is
// absent from Go (the npc-dialogues.json stub; NPC dialogs are
// queued after Bank, PHASE46_PLAN.md §6), so until that import
// lands the emit runs but nothing plays — the handler is live,
// the leg is data-bound.
func talkToBanker(api *plugins.Api, event *plugins.NpcInteractionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	req := &game.StartNamedRequest{
		Player:         event.Player,
		NPC:            event.NPC,
		NPCID:          event.NpcID,
		HasNPCID:       event.NpcID != 0 || event.NPC != nil,
		DefinitionName: event.DefinitionName,
		Variant:        "standard-dialogue",
		HasVariant:     true,
	}
	if api != nil {
		api.EmitCustomEvent(plugins.NpcDialogueStartEventName, req)
	}
	return req.Handled
}

// bankerDialogueAction is bankerDialogueAction
// (BankBooths.plugin.js): "I'd like to access my bank account,
// please." — an npc-dialogue:action step with action
// "open_interface" and target "Bank", from any NPC that banks.
// On fire it claims the step, resets the dialogue manager
// (changing the current dialogue also tells transcript playback
// not to close the bank), and opens the bank.
//
// The NPC check is the definition name only: the Go event
// (*game.NpcDialogueActionEvent) carries DefinitionName but no
// action list, so the TS alternative — the definition "has a
// 'Bank' action" — is unobservable from this payload (the
// game.NPCDefinition interface exposes no actions). A banker
// whose definition is not named exactly "Banker" does not fire
// until the event carries its actions (NPC-dialogs phase gap).
func bankerDialogueAction(api *plugins.Api, payload any) {
	ev, ok := payload.(*game.NpcDialogueActionEvent)
	if !ok || ev == nil || ev.Handled {
		return
	}
	if ev.Action != "open_interface" || ev.Target != "Bank" {
		return
	}
	if ev.DefinitionName != "Banker" {
		return
	}
	if ev.Player == nil {
		return
	}
	ev.Handled = true
	if dm := ev.Player.GetDialogueManager(); dm != nil {
		dm.Reset()
	}
	openBank(api, ev.Player)
}

// isBankTabSelectButton is isBankTabSelectButton
// (BankBooths.plugin.js): buttonId >= 50070, (buttonId-50070)
// % 4 == 0, tab 0..10 (TOTAL_BANK_TABS, game.TotalBankTabs).
func isBankTabSelectButton(buttonID int) bool {
	if buttonID < BankTabSelectStart {
		return false
	}
	offset := buttonID - BankTabSelectStart
	if offset%4 != 0 {
		return false
	}
	tab := offset / 4
	return tab >= 0 && tab < game.TotalBankTabs
}

// isBankButtonForInterface is isBankButtonForInterface
// (BankBooths.plugin.js): the settings ids only on interface
// 32500, the main ids + tab-select only on interface 12,
// nothing elsewhere.
func isBankButtonForInterface(interfaceID, buttonID int) bool {
	if interfaceID == BankSettingsInterfaceID {
		return bankSettingsButtonIDs[buttonID]
	}
	if interfaceID == game.BankMainInterfaceID {
		return bankMainButtonIDs[buttonID] || isBankTabSelectButton(buttonID)
	}
	return false
}

// handleButtonFlow runs Worker B's Bank.handleButton through
// the Flows seam (game.BankHandleButton, installed from
// cmd/server/bankwiring.go), or falls through (false) when the
// flow is not installed.
func handleButtonFlow(player *game.Player, buttonID, action int) bool {
	if flows.HandleButton != nil {
		return flows.HandleButton(player, buttonID, action)
	}
	return false
}

// HandleBankButton is handleBankButton (BankBooths.plugin.js):
// the onButtonClick route — gate on the bank button sets for
// the player's open interface, then Worker B's handleButton
// with action 0. Exported for tests and for the coordinator;
// the registered onButtonClick handler above calls it (and
// claims the event when it returns true, as TS bankButton
// does).
func HandleBankButton(player *game.Player, buttonID int) bool {
	if player == nil {
		return false
	}
	if !isBankButtonForInterface(player.GetInterfaceId(), buttonID) {
		return false
	}
	return handleButtonFlow(player, buttonID, 0)
}

// HandleBankInterfaceAction is handleBankInterfaceAction
// (BankBooths.plugin.js): the onInterfaceActionClick route —
// the same gate, then Worker B's handleButton with the click's
// action. Exported for tests and for the coordinator; the
// registered interface-action handler below calls it.
func HandleBankInterfaceAction(player *game.Player, buttonID, action int) bool {
	if player == nil {
		return false
	}
	if !isBankButtonForInterface(player.GetInterfaceId(), buttonID) {
		return false
	}
	return handleButtonFlow(player, buttonID, action)
}

// isBankWidgetTarget reports whether (groupID, childID) is one
// of the bank's cache-native widget routes (PHASE46_PLAN.md
// §2a, Bank.handleWidgetAction, Bank.ts:877-1032): the main
// frame controls and grid on group 12, the side inventory and
// worn views on group 15 (child 3 or >= 0x8000 for inventory,
// child 4 for worn).
func isBankWidgetTarget(groupID, childID int) bool {
	switch groupID {
	case game.BankMainInterfaceID: // 12
		switch childID {
		case 2, 10, 12, 23, 25, 29, 31, 33, 35, 37, 40, 42, 45, 47, 49:
			return true
		}
		return childID >= 68 && childID <= 78
	case 15:
		return childID == 3 || childID == 4 || childID >= 0x8000
	default:
		return false
	}
}

// handleInterfaceActionClick is the registered
// onInterfaceActionClick handler (TS bankInterfaceAction +
// the §4 interface-action routing, PHASE46_PLAN.md): first the
// legacy-button gate (handleBankInterfaceAction, exactly as TS
// registers it on this family), then the widget (group, child)
// dispatch into Worker B's handleBankWidget. The widget leg
// only runs while the bank is open; with no HandleWidget flow
// installed the click falls through unclaimed.
//
// NOTE on the legacy-button leg: the ids it gates (50013 et
// al.) are the simple-click button ids — in Go they arrive on
// the onButtonClick family (Worker A), not this hub, whose
// ButtonID is the (group<<16)|child widget uid (the smithing
// OnInterfaceActionButton precedent, anvil.go). The gate is
// kept here because TS registers bankInterfaceAction on this
// family too; it claims nothing a widget uid could match.
func handleInterfaceActionClick(ev *plugins.InterfaceActionClickEvent) {
	if ev == nil || ev.Player == nil {
		return
	}
	if HandleBankInterfaceAction(ev.Player, ev.ButtonID, ev.Action) {
		ev.Handled = true
		return
	}
	if !ev.HasGroupID || !ev.HasChildID {
		return
	}
	if !isBankWidgetTarget(ev.GroupID, ev.ChildID) {
		return
	}
	if !bankIsOpen(ev.Player) {
		return
	}
	// The (group, child) dispatch itself (grid ops,
	// side-inventory ops, side-worn ops, tab bar, search, frame
	// controls) is Worker B's game.BankHandleWidgetAction,
	// reached through the Flows seam (installed from
	// cmd/server/bankwiring.go, which translates the event into
	// a game.BankWidgetPacket); this package only routes to it.
	if flows.HandleWidget != nil && flows.HandleWidget(ev) {
		ev.Handled = true
	}
}
