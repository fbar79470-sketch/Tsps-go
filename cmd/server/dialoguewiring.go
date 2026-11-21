package main

import (
	"fmt"
	"log"

	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
)

// Dialogue wiring: the cmd/server adapter between the NPC-option /
// dialogue-continue listeners (internal/listeners), the transcript
// playback engine (internal/game), the definition Store, the world
// TaskManager, and the plugin hub (internal/plugins). It follows the
// Phase 13 object-listener precedent exactly: narrow func fields and
// lookup interfaces on the caller side, translation adapters here,
// where all sides are importable. Game never imports plugins;
// listeners never import definition or plugins.
//
// The coordinator wires it in two places in server.go (both edits
// are the coordinator's — this file only defines the adapters):
//
//   - In buildServer, after loadGameDefinitions:
//     NewDialoguePlayback over LoadDialogueData, stored on adapter
//     fields (playback, npcDefs, npcNameResolver), plus
//     SubscribeDialogueStart for the npc-dialogue:start custom
//     event. A dialogue-data load failure fails the boot, as the
//     TS loader's throw fails the plugin registration.
//   - In AddPlayer: the NPC-name resolver on the entering
//     player's DialogueManager (before queueing, so no tick can
//     send a chathead before the resolver exists), and
//     InstallDialogueListeners in the Deps block.
//
// A nil hub, nil playback, nil registry, or nil store degrades
// each adapter to its documented no-op: NPC clicks drop with
// their unwired reasons, resume packets drop with
// ReasonDialogueUnwired, and variant/condition emits run the
// playback defaults, exactly as the examine and chat seams
// degrade.

// talkToOption is the lower-cased menu text the transcript
// playback answers: NpcDialogues subscribes its Talk-to handler
// through onAnyNpcInteraction({"Talk-to": ...})
// (NpcDialogues.plugin.js:1393-1403), and the listener lower-cases
// option text before dispatch. Any other option falls through to
// the interaction emit.
const talkToOption = "talk-to"

// dialoguePlaybackDeps builds the TranscriptDeps for a playback
// engine over the loaded dialogue data, the definition Store, the
// world (for the tick scheduler), and the hub (for custom-event
// emits and the variant/condition hook families). Nil-tolerant:
// a nil store leaves the name seams answering ""/"null" (the
// DEFAULT shape), a nil hub leaves the emits unhandled (the
// no-hooks defaults), a nil world leaves Schedule unset (the
// playback runs continuations immediately), a nil prompts
// registry leaves PresentMenu unset (menus reach the
// unavailable path), and a nil shops opener leaves
// FindShops/OpenShop unset (the playback degrades them to its
// documented fallbacks: shop steps never open).
func dialoguePlaybackDeps(data *definition.DialogueData, store *definition.Store, world *game.World, hub *plugins.Hub, prompts *game.MultiChatboxPrompt, logf func(format string, args ...any), shops game.ShopOpener) game.TranscriptDeps {
	if logf == nil {
		logf = log.Printf
	}
	deps := game.TranscriptDeps{
		Data: data,
		Logf: logf,
		// CloseChatbox is the unscoped chatbox removal the
		// playback's cancel uses when it still owns the chain
		// (NpcDialogues.plugin.js:591-598, via closeChatboxFor;
		// the owner-scoped form has no Go equivalent — the
		// session DialogueSender exposes only the unscoped
		// removal — so cancel checks ownership itself before
		// calling this). A session-less player has nothing to
		// close.
		CloseChatbox: func(player *game.Player) {
			if player == nil {
				return
			}
			if remover, ok := player.Session.(interface{ SendInterfaceRemoval() }); ok && remover != nil {
				remover.SendInterfaceRemoval()
			}
		},
		// SendHeldStatement is StatementDialogue.send(player,
		// text, false): the message box without "Click here to
		// continue" (StatementDialogue.ts:44-54), shown while a
		// held scene plays out.
		SendHeldStatement: func(player *game.Player, text string) {
			game.SendStatementDialogue(player, text, false)
		},
	}
	if shops != nil {
		// FindShops/OpenShop are the dialogue shop steps
		// (shopName / open_shop, NpcDialogues.plugin.js:
		// 569-576 + 1155-1164), bound through the ShopOpener
		// seam (see game/shop_dialogue.go). A nil opener
		// leaves them unset and shop steps fall back to
		// their documented no-shop behavior.
		deps.FindShops, deps.OpenShop = game.ShopDialogueSeams(shops)
	}
	if prompts != nil {
		// PresentMenu is api.sendMultiChatboxPrompt
		// (NpcDialogues.plugin.js:846-848): choice steps present
		// through the MultiChatboxPrompt registry, under the
		// NpcDialogues plugin name TS passes. The resume
		// dispatch checks the same registry before the
		// dialogue manager (see dialogueAccess.Resume).
		deps.PresentMenu = func(player *game.Player, prompt string, options []game.MenuOption) bool {
			return prompts.ShowPrompt("NpcDialogues", player, prompt, options)
		}
	}
	if store != nil {
		deps.NPCName = func(npc *game.NPC, player *game.Player) string {
			_ = player // No transform resolution; see NpcForPlayer below.
			if npc == nil {
				return ""
			}
			return store.NPC(npc.ID).Name()
		}
	}
	if world != nil {
		tasks := world.Tasks()
		deps.Schedule = func(player *game.Player, ticks int, fn func()) {
			// Overhead continuations and held advances run
			// through the TaskManager (TS CountdownTask via
			// TaskManager.submit), keyed by the player so a
			// logout can cancel them with the owner's other
			// tasks. Tick-goroutine-only: playback runs on
			// ticks, and TaskManager is otherwise
			// tick-goroutine-only (see autosaveTask).
			task := game.NewTask(ticks, func(cycle int64) bool {
				_ = cycle
				fn()
				return true // one-shot: done after firing.
			})
			task.Name = "npc-dialogue-continue"
			task.Key = player
			tasks.Schedule(task)
		}
	}
	if hub != nil {
		deps.EmitCustomEvent = hub.EmitCustomEvent
		deps.VariantHandlers = game.NewNpcDialogueVariantRegistry(logf)
		deps.VariantHandlers.Register(func(event game.NpcDialogueVariantEvent) (game.NpcDialogueVariantChoice, bool) {
			choice := hub.EmitNpcDialogueVariant(translateVariantContext(event, store))
			if choice == nil {
				return game.NpcDialogueVariantChoice{}, false
			}
			// A page-qualified answer carries its page; a
			// bare-name answer leaves Page "" (the string
			// form). The playback treats "" as "no page
			// requested", matching TS's undefined.
			return game.NpcDialogueVariantChoice{Variant: choice.Variant, Page: choice.Page}, true
		})
		deps.ConditionHandlers = game.NewNpcDialogueConditionRegistry(logf)
		deps.ConditionHandlers.Register(func(query game.NpcDialogueConditionQuery) (bool, bool) {
			answer := hub.EmitNpcDialogueCondition(translateConditionEvent(query, store))
			if answer == nil {
				return false, false
			}
			return *answer, true
		})
	}
	return deps
}

// translateVariantContext builds the hub's NpcDialogueContext
// from the playback's variant event. The definition resolves
// through the Store when the event carries one (DefinitionID !=
// 0; item transcripts carry 0 and no definition, as TS's
// optional definition); *definition.NpcDefinition satisfies
// plugins.NpcDefinition structurally. Pages copy across
// directly: both shapes are {page, variants}.
func translateVariantContext(event game.NpcDialogueVariantEvent, store *definition.Store) *plugins.NpcDialogueContext {
	ctx := &plugins.NpcDialogueContext{
		Player: event.Player,
		NPC:    event.NPC,
		NpcID:  event.NPCID,
	}
	if store != nil && event.DefinitionID != 0 {
		ctx.Definition = store.NPC(event.DefinitionID)
	}
	for _, page := range event.Pages {
		ctx.Pages = append(ctx.Pages, plugins.NpcDialoguePage{Page: page.Page, Variants: page.Variants})
	}
	return ctx
}

// translateConditionEvent builds the hub's
// NpcDialogueConditionEvent from the playback's condition query.
// HasStepID is the non-empty test: the export's anchor ids are
// non-empty strings, and the playback passes "" when the step
// carries none.
func translateConditionEvent(query game.NpcDialogueConditionQuery, store *definition.Store) *plugins.NpcDialogueConditionEvent {
	ctx := translateVariantContext(game.NpcDialogueVariantEvent{
		Player:         query.Player,
		NPC:            query.NPC,
		NPCID:          query.NPCID,
		DefinitionName: query.DefinitionName,
		DefinitionID:   query.DefinitionID,
		Pages:          query.Pages,
	}, store)
	return plugins.NewNpcDialogueConditionEvent(*ctx, query.Text, query.StepID, query.StepID != "")
}

// loadDialoguePlayback loads the dialogue catalogue from the
// definitions directory and builds the playback engine over it.
// It ports the NpcDialogues load() call site: the catalogue is
// read from the definitions directory (not the definition source
// registry — TS reads these files directly), with the Store's
// NPC names feeding the speaker index's cache-name pass. A load
// failure fails the boot: npc-dialogues.json is required, and TS
// fails the plugin registration when its read throws. The
// retired-NPC successor map stays empty: retired-npcs.json is
// plugin-private data (plugins/npcs/data/), never copied into
// the repo, and no approved read path exists for it yet —
// successor ids play no transcript until one does (documented
// gap; 12 successors upstream).
func loadDialoguePlayback(dir string, store *definition.Store, world *game.World, hub *plugins.Hub, prompts *game.MultiChatboxPrompt, logf func(format string, args ...any), shops game.ShopOpener) (*game.DialoguePlayback, error) {
	opts := definition.DialogueLoadOptions{}
	if store != nil {
		opts.NPCName = func(id int) string { return store.NPC(id).Name() }
	}
	data, err := definition.LoadDialogueData(dir, opts)
	if err != nil {
		return nil, fmt.Errorf("loading dialogue data: %w", err)
	}
	return game.NewDialoguePlayback(dialoguePlaybackDeps(data, store, world, hub, prompts, logf, shops)), nil
}

// SubscribeDialogueStart subscribes the playback's named-variant
// entry point to the "npc-dialogue:start" custom event, porting
// the NpcDialogues subscription (NpcDialogues.plugin.js:1378).
// Content emits *game.StartNamedRequest (the {player, npc?,
// npcId, variant, select?, exact?, handled} request shape, with
// cancel written back); any other payload shape is ignored —
// the hub passes payloads opaquely, and only the request type
// carries the fields the entry point reads. A nil hub or nil
// playback subscribes nothing.
func SubscribeDialogueStart(hub *plugins.Hub, playback *game.DialoguePlayback) {
	if hub == nil || playback == nil {
		return
	}
	hub.SubscribeCustomEvent(plugins.NpcDialogueStartEventName, func(payload any) {
		req, ok := payload.(*game.StartNamedRequest)
		if !ok || req == nil {
			return
		}
		playback.StartNamedVariant(req)
	})
}

// npcListLookup adapts the world's NPC MobileList to
// listeners.NpcLookup: World.getNpcs().get(index). A miss (empty
// slot, out-of-range index) returns nil. A nil list never finds
// anything.
type npcListLookup struct {
	list *game.MobileList[*game.NPC]
}

var _ listeners.NpcLookup = npcListLookup{}

// Find is MobileList.Get: the slot's NPC, or nil on a miss.
func (l npcListLookup) Find(index int) *game.NPC {
	if l.list == nil {
		return nil
	}
	npc, ok := l.list.Get(index)
	if !ok {
		return nil
	}
	return npc
}

// storeNpcDefinitions implements listeners.NpcDefinitionSource
// over a *definition.Store. *definition.NpcDefinition satisfies
// the listeners.NpcDefinition interface structurally (same
// accessor set), so the adapter only translates the lookup
// signature and the resolved test. A nil Store is never wrapped
// — NewNpcDefinitionSource returns nil instead, and the
// listener drops with its unwired reason.
type storeNpcDefinitions struct {
	store *definition.Store
}

var _ listeners.NpcDefinitionSource = storeNpcDefinitions{}

// NewNpcDefinitionSource returns the NPC-definition source for
// store, or nil when store is nil.
func NewNpcDefinitionSource(store *definition.Store) listeners.NpcDefinitionSource {
	if store == nil {
		return nil
	}
	return storeNpcDefinitions{store: store}
}

// NpcForPlayer is NPC.getCurrentDefinition(player): the
// definition the player sees for id. TRANSFORM GAP: Go has no
// NPC transform/var resolution, so the source answers the base
// definition; a transform NPC reads its base actions. Resolved
// means the Store hydrated a real definition for the id: the
// DEFAULT shape (id -1, name "null") reports absent, and the
// listener treats the option as absent and proceeds, as TS does
// with a null definition. In NoCache mode every id is absent.
func (s storeNpcDefinitions) NpcForPlayer(id int, player *game.Player) (listeners.NpcDefinition, bool) {
	_ = player // No transform resolution; see above.
	def := s.store.NPC(id)
	if def.ID() != id {
		return nil, false
	}
	return def, true
}

// npcNameResolver returns the NPC-name lookup for the entering
// player's DialogueManager: NpcDefinition.forId(id).getName()
// (NpcDialogue.ts:35) over the Store. A nil store yields nil,
// and the manager answers the DEFAULT name ("null") unwired.
func npcNameResolver(store *definition.Store) func(int) string {
	if store == nil {
		return nil
	}
	return func(id int) string { return store.NPC(id).Name() }
}

// dialogueAccess implements listeners.DialogueAccess over
// Player.GetDialogueManager(). Current returns the manager's
// current entry as a comparable identity token: entries are
// pointers, and the manager returns a nil interface when
// inactive, so == comparison is the TS object-identity check.
// A nil prompts registry degrades Resume to the manager-only
// path (menus never present, so no prompt can be pending).
type dialogueAccess struct {
	prompts *game.MultiChatboxPrompt
}

var _ listeners.DialogueAccess = dialogueAccess{}

// Resume is the dialogue_continue dispatch target: the
// interface-action-click pre-branch runs BEFORE the manager's
// handleResume (NetworkBuilder.ts:939-951, where
// emitInterfaceActionClick checks the MultiChatboxPrompt first,
// PluginManager.ts:2089). A pending prompt consumes the resume
// and the manager never sees it; otherwise the resume goes to
// handleResume.
func (d dialogueAccess) Resume(p *game.Player, widgetID int32, childIndex int) bool {
	if p == nil {
		return false
	}
	if d.prompts != nil && d.prompts.HandleInterfaceActionClick(p, int(widgetID), childIndex, int(uint32(widgetID)>>16)) {
		return true
	}
	return p.GetDialogueManager().HandleResume(int(widgetID), childIndex)
}

// IsActive is DialogueManager.isActive.
func (dialogueAccess) IsActive(p *game.Player) bool {
	if p == nil {
		return false
	}
	return p.GetDialogueManager().IsActive()
}

// Current is DialogueManager.getCurrent, as an identity token.
func (dialogueAccess) Current(p *game.Player) any {
	if p == nil {
		return nil
	}
	return p.GetDialogueManager().GetCurrent()
}

// HandleOption is DialogueManager.handleOption, with the option
// as the zero-based DialogueOption ordinal (buttonNum - 1 on the
// widget path).
func (dialogueAccess) HandleOption(p *game.Player, option int) {
	if p == nil {
		return
	}
	p.GetDialogueManager().HandleOption(game.DialogueOption(option))
}

// NpcHooks adapts the playback entry point and the hub's
// npc:operated emit to listeners.NpcHooks. A nil playback yields
// a StartDialogue that never starts (the click falls through to
// the interaction emit); a nil hub yields no operated delivery
// and no interaction emit. The route emit has no hub family yet
// and stays nil. The interaction emit runs the onNpcClick hooks
// first and the onNpcInteraction name hooks after them
// (Hub.EmitNpcInteraction, Phase 33 + Phase 35), at the head —
// exactly where TS runs NpcInteractionManager.handle relative
// to emitNpcInteraction. The adapter fills the hub event's
// DefinitionName + Action + Actions from the listener event's
// Definition (Name + Actions()[ClickType-1] + the full list);
// a missing definition or a click type outside the actions
// leaves them empty, so no name hook matches — the TS
// registerNpcActions handler reads the same lookup and
// returns without claiming. StartDialogue answers ONLY Talk-to clicks (see
// talkToOption): the static-binding position
// (NpcInteractionManager.handle, core.dialogues.start) has no
// registry in Go — bindings are plugin-registered in TS
// (registerPluginInteraction), and no binding surface is ported
// — so non-Talk-to clicks fall through exactly as they do with
// no static binding registered.
func NpcHooks(playback *game.DialoguePlayback, hub *plugins.Hub) listeners.NpcHooks {
	hooks := listeners.NpcHooks{}
	if playback != nil {
		hooks.StartDialogue = func(req *listeners.NpcDialogueRequest) bool {
			if req == nil || req.Player == nil || !req.HasOption || req.Option != talkToOption {
				return false
			}
			event := game.TranscriptEvent{
				Player: req.Player,
				NPC:    req.NPC,
				NPCID:  req.NpcID,
			}
			if req.Definition != nil {
				event.DefinitionName = req.Definition.Name()
				event.DefinitionID = req.Definition.ID()
			}
			return playback.StartTalkTo(event)
		}
	}
	if hub != nil {
		hooks.EmitNpcInteraction = func(ev *listeners.NpcInteractionEvent) bool {
			if ev == nil {
				return false
			}
			event := &plugins.NpcInteractionEvent{
				Player:    ev.Player,
				NPC:       ev.NPC,
				NpcID:     ev.NpcID,
				NpcIndex:  ev.NpcIndex,
				ClickType: ev.ClickType,
				Location:  ev.Location,
			}
			// The name match reads the definition's name and
			// the clicked option text (PluginManager.ts:
			// 3175-3178: definition.getName(), definition.
			// getActions()[clickType-1]). A nil definition
			// or a click type outside the actions leaves
			// both empty and no name hook matches.
			if ev.Definition != nil {
				event.DefinitionName = ev.Definition.Name()
				if actions := ev.Definition.Actions(); ev.ClickType >= 1 && ev.ClickType <= len(actions) {
					event.Action = actions[ev.ClickType-1]
				}
				event.Actions = ev.Definition.Actions()
			}
			return hub.EmitNpcInteraction(event)
		}
		hooks.EmitOperated = func(ev *listeners.NpcOperatedEvent) {
			hub.EmitNpcOperated(&plugins.NpcOperatedEvent{
				Player:    ev.Player,
				NPC:       ev.NPC,
				NpcID:     ev.NpcID,
				Option:    ev.Option,
				HasOption: ev.HasOption,
			})
		}
	}
	return hooks
}

// InstallDialogueListeners fills the NPC/dialogue seams on
// deps: the NPC lookup, the definition source, the playback +
// hub hook adapters, and the dialogue-runtime seam. A nil deps
// is a no-op; a nil list leaves deps.NPCs unset (the unwired
// drop); a nil defs clears deps.NpcDefs; a nil playback/hub
// yields the degrading hook set. The coordinator calls it once
// per entering player, in AddPlayer's Deps block, after
// InstallObjectListeners.
func InstallDialogueListeners(deps *listeners.Deps, playback *game.DialoguePlayback, hub *plugins.Hub, prompts *game.MultiChatboxPrompt, npcs *game.MobileList[*game.NPC], defs listeners.NpcDefinitionSource) {
	if deps == nil {
		return
	}
	if npcs != nil {
		deps.NPCs = npcListLookup{list: npcs}
	}
	deps.NpcDefs = defs
	deps.NpcHooks = NpcHooks(playback, hub)
	deps.Dialogue = dialogueAccess{prompts: prompts}
}
