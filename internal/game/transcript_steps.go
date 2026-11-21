package game

import (
	"regexp"
	"sort"
	"strings"

	"tsps-go/internal/definition"
)

// This file is the live playback runner: startDialogue, flatten,
// presentMenu, resolveJump, and the overhead-actor machinery
// (NpcDialogues.plugin.js:510-1200). It builds chains through the
// Worker A contract (see the banner in transcript.go) and starts
// them through the player's DialogueManager.
//
// CONTRACT NOTES (Worker A's dialogue_*.go verified; see the
// banner in transcript.go):
//   - Entry construction happens only in the newNPCLine /
//     newPlayerLine / newStatement / newItemStatement /
//     newDoubleItemStatement / newAction helpers below.
//   - Chains start only in runner.startChain, which passes the
//     playback's owner token (TS startDialogues(builder, owner)).
//   - Manager identity checks use manager.GetCurrent() +
//     sameDialogue, which compares Dialogue interface values:
//     entries must be pointer types (or otherwise comparable),
//     as TS compares object identity.
//   - Explicit playback's owner token is the runner itself (one
//     fresh token per StartDialogue call, as TS mints one frozen
//     object per startDialogue). The manager scopes chain
//     entries to it; only closeChatboxFor / presentOwned have no
//     Go equivalent (DialogueSender has no closeChatboxFor), so
//     cancel() issues the unscoped CloseChatbox removal and
//     seam presentations (menus, held sends) are unscoped — see
//     runner.cancel.

// DialogueActor is the overhead-speech surface: the Mobile
// methods the actor-availability rules need. *Player and *NPC
// implement it through the embedded Mobile plus the two
// unexported helpers below; the unexported methods keep the
// interface closed to the game package (TS actors are always
// Mobile instances too).
type DialogueActor interface {
	ForceChat(message string)
	IsRegistered() bool
	Location() Location
	// dialogueVisible is NPC.isVisible (NPC.ts:704). Players
	// have NO isVisible method in TS, so an explicitly bound
	// cast containing the player fails the availability check
	// (actor?.isVisible?.() === true is false for undefined):
	// dialogueVisible on *Player returns false, exactly as TS
	// behaves. Non-explicit player lines never consult it
	// (actorAvailable has no visibility term).
	dialogueVisible() bool
	dialogueIndex() int
}

func (p *Player) dialogueVisible() bool { return false }
func (p *Player) dialogueIndex() int    { return p.Index }
func (n *NPC) dialogueVisible() bool    { return n.Visible }
func (n *NPC) dialogueIndex() int       { return n.Index }

// heldDialogue is the optional SendHeld surface
// (NpcDialogue.sendHeld, NpcDialogue.ts): the line shown without
// its "Click here to continue". PlayerDialogue has no such
// method in TS, so the hold path type-asserts and no-ops for
// player lines (TS held?.sendHeld?.()).
type heldDialogue interface {
	SendHeld(player *Player)
}

// sameDialogue compares two Dialogue entries by identity, as TS
// compares object references. It never panics: an uncomparable
// entry type degrades to "different" (the identity checks then
// behave as if a replacement dialogue owned the chatbox, which
// is the safe direction — playback stops rather than stomping
// another chain).
func sameDialogue(a, b Dialogue) (same bool) {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	defer func() {
		if recover() != nil {
			same = false
		}
	}()
	return a == b
}

// StartDialogue plays steps as a conversation for event's player,
// through the player's DialogueManager. It ports startDialogue
// (NpcDialogues.plugin.js:588-1200) with branches (the record's
// named branches for call steps) and a resolved context (nil is
// allowed: page indexes are then empty and jump fallbacks
// degrade to the queued continuation). It returns the scoped
// cancellation handle: for explicit (trusted custom-event)
// playback it stops this conversation when its chain still owns
// the chatbox; for ordinary Talk-to playback it is a no-op
// returning false, as TS cancel() is owner-gated. A nil player
// or a missing DialogueManager plays nothing and returns a
// no-op handle (TS always has a manager; a nil manager means
// Worker A's wiring is absent).
func (pb *DialoguePlayback) StartDialogue(event TranscriptEvent, steps []definition.DialogueStep, branches map[string][]definition.DialogueStep, context *TranscriptContext) func() bool {
	noop := func() bool { return false }
	if event.Player == nil {
		return noop
	}
	manager := event.Player.GetDialogueManager()
	if manager == nil {
		pb.logf("[npc-dialogues] no dialogue manager for %s; conversation dropped", event.Player.GetDisplayName())
		return noop
	}
	ctx := context
	if ctx == nil {
		ctx = &TranscriptContext{}
	}
	if ctx.SpeakerIDByName == nil && pb.deps.Data != nil {
		copied := *ctx
		copied.SpeakerIDByName = pb.deps.Data.SpeakerIDByName
		ctx = &copied
	}
	r := &runner{
		pb:            pb,
		event:         event,
		manager:       manager,
		branches:      branches,
		context:       ctx,
		recordsByText: make(map[string][]*optionRecord),
		optionRecords: make(map[*definition.DialogueOptionStep]*optionRecord),
		warnedMenus:   make(map[*definition.DialogueStep]bool),
	}
	// Scoped cancellation is an opt-in contract of trusted named
	// playback (NpcDialogues.plugin.js:580-583): only explicit
	// playback gets an owner token (TS Object.freeze({})). The
	// runner itself is the token: one fresh value per
	// StartDialogue call, comparable by identity, opaque to the
	// manager. Ordinary Talk-to playback stays unowned.
	if event.Explicit {
		r.owner = r
	}
	r.run(wrapPlaySteps(steps), nil)
	return r.cancel
}

// runner is one conversation's live state: the jump counter, the
// menus seen so far, the ordered option records, and the
// played/cancelled flags. It is created per StartDialogue call
// and kept alive by the chain/menu closures until the
// conversation ends or is cancelled.
type runner struct {
	pb       *DialoguePlayback
	event    TranscriptEvent
	manager  *DialogueManager
	branches map[string][]definition.DialogueStep
	context  *TranscriptContext

	jumps         int
	currentMenu   *transcriptMenu
	menuHistory   []*transcriptMenu
	records       []*optionRecord
	recordsByText map[string][]*optionRecord
	optionRecords map[*definition.DialogueOptionStep]*optionRecord
	currentRecord *optionRecord
	lastChoice    string
	hasLastChoice bool
	warnedMenus   map[*definition.DialogueStep]bool

	playedAny bool
	cancelled bool
	// owner is this playback's chatbox-owner token (the runner
	// itself) for explicit playback, or nil for ordinary
	// Talk-to playback (see StartDialogue).
	owner any
}

// transcriptMenu is one shown menu: the choice step, the steps
// that follow the menu in its branch (rest), and the option
// record the menu was reached through.
type transcriptMenu struct {
	step   *definition.DialogueStep
	rest   []playStep
	record *optionRecord
}

// optionRecord is one recorded menu/random option, in the order
// options were shown: the record powering the wiki "jump above"
// shortcut (NpcDialogues.plugin.js:614-628). Random-option
// records carry id -1 and the alternative list.
type optionRecord struct {
	id            int
	text          string
	steps         []definition.DialogueStep
	randomOptions []definition.DialogueOptionStep
	randomIndex   int
	hasRandom     bool
}

// recordOption returns the record for an option node, creating
// it on first sight (TS recordOption's WeakMap). Identity is the
// option node's pointer: options re-presented from the same menu
// object share a record, while options rebuilt by a fresh
// flatten get fresh records, matching the TS spread-copy
// behavior closely enough for the jump fallbacks (which also
// compare by signature).
func (r *runner) recordOption(option *definition.DialogueOptionStep) *optionRecord {
	if record, ok := r.optionRecords[option]; ok {
		return record
	}
	steps := option.Steps
	if steps == nil {
		steps = []definition.DialogueStep{}
	}
	record := &optionRecord{id: len(r.records), text: option.Text, steps: steps}
	r.optionRecords[option] = record
	r.records = append(r.records, record)
	key := normText(record.text)
	r.recordsByText[key] = append(r.recordsByText[key], record)
	return record
}

func (r *runner) cancel() bool {
	// cancel ports TS cancel() (NpcDialogues.plugin.js:591-598):
	// false without an owner, without even setting the flag;
	// otherwise whether the manager's chain is still this
	// playback's, the owned chatbox is closed, and the manager
	// resets if it still owns the chain afterwards (closing
	// emits synchronous interface hooks which may install a new
	// chain).
	// DEVIATION: TS closes with sender.closeChatboxFor(owner),
	// which closes only a chatbox mounted under this owner and
	// reports whether it closed one. DialogueSender has no
	// closeChatboxFor, so Go issues the unscoped CloseChatbox
	// removal (sendInterfaceRemoval) and only when this
	// playback still owns the manager's chain — closing
	// unconditionally could clobber another chain's chatbox,
	// which the scoped TS close never does. The return value is
	// therefore ownsChain alone: the TS `|| closed` term (an
	// owned menu/held box mounted after the chain moved on)
	// cannot be observed in Go.
	if r.cancelled || r.owner == nil {
		return false
	}
	r.cancelled = true
	ownsChain := r.manager.GetOwner() == r.owner
	if ownsChain {
		r.close()
		// Closing emits synchronous interface hooks which may
		// install a new chain.
		if r.manager.GetOwner() == r.owner {
			r.manager.Reset()
		}
	}
	return ownsChain
}

func (r *runner) close() {
	if r.pb.deps.CloseChatbox != nil {
		r.pb.deps.CloseChatbox(r.event.Player)
	}
}

func (r *runner) unavailable() {
	r.close()
	if !r.playedAny {
		r.event.Player.SendMessage(UnavailableMessage)
	}
}

func (r *runner) schedule(ticks int, fn func()) {
	if r.pb.deps.Schedule != nil {
		r.pb.deps.Schedule(r.event.Player, ticks, fn)
		return
	}
	// No tick scheduler installed: continue immediately (see
	// TranscriptDeps.Schedule). The conversation completes;
	// only the overhead cadence is lost.
	fn()
}

func (r *runner) startChain(builder *DialogueChainBuilder) {
	if !r.cancelled {
		// TS startDialogues(builder, owner): the owner is nil
		// for ordinary playback, the token for explicit
		// playback. The manager scopes visible entries to it
		// via WithChatboxOwner.
		r.manager.StartDialogues(builder, r.owner)
	}
}

func (r *runner) emit(name string, payload any) { r.pb.emit(name, payload) }

// resolveCondition answers one condition (text + anchor id)
// through the condition hooks, then the built-in combat/skill
// fallbacks (NpcDialogues.plugin.js:632-636). It returns
// answered=false for TS null.
func (r *runner) resolveCondition(text, stepID string) (bool, bool) {
	if r.pb.deps.ConditionHandlers != nil {
		if answer, ok := r.pb.deps.ConditionHandlers.Emit(NpcDialogueConditionQuery{
			Player: r.event.Player, NPC: r.event.NPC, NPCID: r.event.NPCID,
			DefinitionName: r.event.DefinitionName, DefinitionID: r.event.DefinitionID,
			Pages: r.context.Pages, Text: text, StepID: stepID,
		}); ok {
			return answer, true
		}
	}
	if answer, ok := combatLevelCondition(r.event.Player, text); ok {
		return answer, true
	}
	if answer, ok := skillLevelCondition(r.event.Player, text); ok {
		return answer, true
	}
	return false, false
}

// --- Chain entry construction (Worker A contract surface) ------
//
// Every DialogueChainBuilder entry this file creates goes through
// these helpers, so a constructor-signature drift is fixed here
// and nowhere else.

func (r *runner) newNPCLine(index, npcID int, text, speakerName string, hasSpeakerName bool, expression DialogueExpression, groupID int) Dialogue {
	entry := NewNpcDialogue(index, npcID, text).WithExpression(expression).WithGroupID(groupID)
	// TS passes speakerName possibly undefined (NpcDialogues.
	// plugin.js:1018): undefined resolves through the NPC
	// definition at send time, while an explicit "" wins over
	// the definition name (?? semantics). WithSpeakerName is
	// therefore applied only when a name was actually given.
	if hasSpeakerName {
		entry.WithSpeakerName(speakerName)
	}
	return entry
}

func (r *runner) newPlayerLine(index int, text string, expression DialogueExpression) Dialogue {
	return NewPlayerDialogue(index, text).WithExpression(expression)
}

func (r *runner) newStatement(index int, text string) Dialogue {
	return NewStatementDialogue(index, text)
}

func (r *runner) newItemStatement(index, itemID int, text string) Dialogue {
	return NewItemStatementDialogue(index, itemID, text)
}

func (r *runner) newDoubleItemStatement(index, first, second int, text string) Dialogue {
	return NewDoubleItemStatementDialogue(index, first, second, text)
}

func (r *runner) newAction(index int, execute func(), selfAdvancing bool) Dialogue {
	return NewActionDialogue(index, DialogueActionFunc(execute)).WithSelfAdvancing(selfAdvancing)
}

// resolveExpression ports the expression resolution in run()
// (NpcDialogues.plugin.js:1017): request.expression ?? step
// expression, valid only when an integer >= 0, else the
// constructors' CALM default.
func resolveExpression(request *int, step definition.DialogueStep) DialogueExpression {
	var animation float64
	hasAnimation := false
	if request != nil {
		animation, hasAnimation = float64(*request), true
	} else if step.HasExpression {
		animation, hasAnimation = step.Expression, true
	}
	if hasAnimation {
		if value, ok := dialogueExpressionInt(animation); ok && value >= 0 {
			return DialogueExpression(value)
		}
	}
	return DialogueExpressionCalm
}

// --- Flattening --------------------------------------------------

// playKind distinguishes original transcript steps from the
// synthetic steps flatten produces.
type playKind int

const (
	playOriginal playKind = iota
	// playEnd is flatten's {type: "end"} for an unreplayable
	// jump: stop rather than leak into the next step.
	playEnd
	// playGoMenu replays a menu already seen ("shows
	// other/previous/initial options").
	playGoMenu
	// playJumpTo is an expanded jump target, resolved when the
	// conversation reaches it so its conditions see what
	// happened before it.
	playJumpTo
	// playConditionRun is a run of sibling conditions left for
	// the live runner to resolve when reached.
	playConditionRun
	// playConditionChosen marks a branch chosen at flatten
	// time (eager mode): its emit runs when reached.
	playConditionChosen
)

// playStep is one flattened queue entry: either an original
// transcript step or a synthetic step with its payload.
type playStep struct {
	kind playKind
	step definition.DialogueStep

	menu       *transcriptMenu
	via        string
	jumpSteps  []definition.DialogueStep
	run        []definition.DialogueStep
	chosenID   string
	chosenText string
}

func wrapPlaySteps(steps []definition.DialogueStep) []playStep {
	out := make([]playStep, 0, len(steps))
	for _, step := range steps {
		out = append(out, playStep{kind: playOriginal, step: step})
	}
	return out
}

func concatPlaySteps(parts ...[]playStep) []playStep {
	n := 0
	for _, part := range parts {
		n += len(part)
	}
	out := make([]playStep, 0, n)
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

// jumpTarget is resolveJump's result: no target (fall through),
// "end", a menu replay, or an expanded step list.
type jumpTarget struct {
	kind  playKind // playEnd, playGoMenu, playJumpTo, or playOriginal for "none"
	menu  *transcriptMenu
	via   string
	steps []definition.DialogueStep
}

func noJumpTarget() jumpTarget { return jumpTarget{kind: playOriginal} }

// flattenOpts carries flatten's callbacks, mirroring the opts
// object (NpcDialogues.plugin.js:500-502). Stopped is shared
// across the recursive calls: an "end" jump stops the whole
// flatten, not just the current level.
type flattenOpts struct {
	resolveCondition func(text, stepID string) (bool, bool)
	resolveJump      func(step definition.DialogueStep, prev, next *definition.DialogueStep) jumpTarget
	lazyConditions   bool
	wrapBranch       func(chosen definition.DialogueStep, branch []playStep) []playStep
	stopped          *bool
}

// flatten ports flatten (NpcDialogues.plugin.js:500-559): a
// transcript becomes a linear play queue. Conditions resolve
// through resolveCondition (first true wins; else first
// unknown; all-false skips the guarded content); lazyConditions
// leaves runs for the live runner; jumps resolve through
// resolveJump (unresolved jumps fall through).
func flatten(steps []playStep, opts flattenOpts) []playStep {
	steps = nestOrphanConditions(steps)
	resolveCondition := opts.resolveCondition
	if resolveCondition == nil {
		resolveCondition = func(string, string) (bool, bool) { return false, false }
	}
	resolveJump := opts.resolveJump
	if resolveJump == nil {
		resolveJump = func(definition.DialogueStep, *definition.DialogueStep, *definition.DialogueStep) jumpTarget {
			return noJumpTarget()
		}
	}
	wrapBranch := opts.wrapBranch
	if wrapBranch == nil {
		wrapBranch = func(_ definition.DialogueStep, branch []playStep) []playStep { return branch }
	}
	stopped := opts.stopped
	if stopped == nil {
		stopped = new(bool)
	}

	var out []playStep
	for position := 0; position < len(steps); position++ {
		if *stopped {
			break
		}
		entry := steps[position]
		if entry.kind == playOriginal && entry.step.Type == "jump" {
			step := entry.step
			var target jumpTarget
			if step.HasSteps {
				// Some wiki exports inline the target body
				// on a "continue" jump; prefer it so nested
				// quest branches do not stop at the jump
				// marker.
				target = jumpTarget{kind: playJumpTo, steps: step.Steps}
			} else {
				var prev, next *definition.DialogueStep
				if position > 0 && steps[position-1].kind == playOriginal {
					prev = &steps[position-1].step
				}
				if position+1 < len(steps) && steps[position+1].kind == playOriginal {
					next = &steps[position+1].step
				}
				target = resolveJump(step, prev, next)
			}
			switch target.kind {
			case playEnd:
				out = append(out, playStep{kind: playEnd})
				*stopped = true
			case playGoMenu:
				out = append(out, playStep{kind: playGoMenu, menu: target.menu, via: target.via})
			case playJumpTo:
				out = append(out, playStep{kind: playJumpTo, jumpSteps: target.steps})
			}
			if *stopped {
				break
			}
			continue
		}
		if entry.kind != playOriginal || entry.step.Type != "condition" {
			out = append(out, entry)
			continue
		}
		runStart := position
		for position+1 < len(steps) && steps[position+1].kind == playOriginal && steps[position+1].step.Type == "condition" {
			position++
		}
		run := steps[runStart : position+1]
		if opts.lazyConditions {
			raw := make([]definition.DialogueStep, 0, len(run))
			for _, item := range run {
				raw = append(raw, item.step)
			}
			out = append(out, playStep{kind: playConditionRun, run: raw})
			continue
		}
		chosen, ok := chooseCondition(run, resolveCondition)
		if !ok {
			continue
		}
		branch := flatten(wrapPlaySteps(chosen.step.Steps), opts)
		out = append(out, wrapBranch(chosen.step, branch)...)
		if *stopped {
			break
		}
		if continues(chosen.step.Steps) {
			rest := make([]playStep, 0, len(run)-chosen.index-1)
			rest = append(rest, run[chosen.index+1:]...)
			out = append(out, flatten(rest, opts)...)
			if *stopped {
				break
			}
		}
	}
	return out
}

// chosenCondition is chooseCondition's result: the winning run
// entry, its index, and whether its answer was an explicit true
// (as opposed to the first-unknown fallback).
type chosenCondition struct {
	index    int
	step     definition.DialogueStep
	answer   bool
	answered bool
}

// chooseCondition ports chooseCondition
// (NpcDialogues.plugin.js:477-488): first-true (else
// first-unknown, else none) pick for a run of sibling conditions.
func chooseCondition(run []playStep, resolveCondition func(text, stepID string) (bool, bool)) (chosenCondition, bool) {
	type answer struct {
		value    bool
		answered bool
	}
	answers := make([]answer, len(run))
	for i, entry := range run {
		value, answered := resolveCondition(entry.step.Text, entry.step.ID)
		answers[i] = answer{value: value, answered: answered}
	}
	index := -1
	for i, a := range answers {
		if a.answered && a.value {
			index = i
			break
		}
	}
	if index == -1 {
		for i, a := range answers {
			if !a.answered {
				index = i
				break
			}
		}
		if index == -1 {
			// Every condition answered false: the guarded
			// content does not apply.
			return chosenCondition{}, false
		}
	}
	return chosenCondition{index: index, step: run[index].step, answer: answers[index].value, answered: answers[index].answered}, true
}

// nestOrphanConditions ports nestOrphanConditions
// (NpcDialogues.plugin.js:420-441): the parser sometimes emits a
// guard as an empty condition followed by the text as siblings,
// so the following run of non-condition steps is attached to the
// empty condition. Without this the text plays unconditionally.
// It recurses into options first (an option-bearing step's own
// nested steps are NOT recursed, matching the TS if/else), then
// into nested steps.
func nestOrphanConditions(steps []playStep) []playStep {
	if len(steps) == 0 {
		return []playStep{}
	}
	var flat []playStep
	for i := 0; i < len(steps); i++ {
		entry := steps[i]
		if entry.kind == playOriginal && entry.step.Type == "condition" && len(entry.step.Steps) == 0 {
			var branch []definition.DialogueStep
			j := i + 1
			for j < len(steps) {
				next := steps[j]
				if next.kind == playOriginal && next.step.Type == "condition" {
					break
				}
				if next.kind != playOriginal {
					break
				}
				branch = append(branch, next.step)
				j++
			}
			if len(branch) > 0 {
				step := entry.step
				step.Steps = branch
				step.HasSteps = true
				flat = append(flat, playStep{kind: playOriginal, step: step})
				i = j - 1
				continue
			}
		}
		flat = append(flat, entry)
	}
	out := make([]playStep, 0, len(flat))
	for _, entry := range flat {
		switch entry.kind {
		case playOriginal:
			step := entry.step
			if len(step.Options) > 0 {
				options := make([]definition.DialogueOptionStep, len(step.Options))
				for i, option := range step.Options {
					nested := nestOrphanConditions(wrapPlaySteps(option.Steps))
					nestedSteps := make([]definition.DialogueStep, 0, len(nested))
					for _, item := range nested {
						nestedSteps = append(nestedSteps, item.step)
					}
					option.Steps = nestedSteps
					options[i] = option
				}
				step.Options = options
				out = append(out, playStep{kind: playOriginal, step: step})
			} else if len(step.Steps) > 0 {
				nested := nestOrphanConditions(wrapPlaySteps(step.Steps))
				nestedSteps := make([]definition.DialogueStep, 0, len(nested))
				for _, item := range nested {
					nestedSteps = append(nestedSteps, item.step)
				}
				step.Steps = nestedSteps
				out = append(out, playStep{kind: playOriginal, step: step})
			} else {
				out = append(out, entry)
			}
		case playJumpTo:
			// TS spreads jump_to steps through the same map:
			// a jump target's nested steps are nested too.
			nested := nestOrphanConditions(wrapPlaySteps(entry.jumpSteps))
			nestedSteps := make([]definition.DialogueStep, 0, len(nested))
			for _, item := range nested {
				nestedSteps = append(nestedSteps, item.step)
			}
			entry.jumpSteps = nestedSteps
			out = append(out, entry)
		default:
			out = append(out, entry)
		}
	}
	return out
}

// --- The live runner ------------------------------------------------

// flattenOptions returns the flatten options run() uses: live
// condition/jump resolution, lazy conditions, and the
// condition_chosen bookkeeping splice
// (NpcDialogues.plugin.js:767-768).
func (r *runner) flattenOptions(stopped *bool) flattenOpts {
	return flattenOpts{
		resolveCondition: r.resolveCondition,
		resolveJump:      r.resolveJump,
		lazyConditions:   true,
		wrapBranch: func(chosen definition.DialogueStep, branch []playStep) []playStep {
			marker := playStep{kind: playConditionChosen, chosenID: chosen.ID, chosenText: chosen.Text}
			return append([]playStep{marker}, branch...)
		},
		stopped: stopped,
	}
}

// run plays one flattened queue. It ports run()
// (NpcDialogues.plugin.js:853-1200): speech lines accumulate
// into one chain; jumps, condition runs, menus, and action steps
// each terminate the current chain with an ActionDialogue that
// continues the conversation when reached. currentRecord is the
// option record the queue belongs to (for "jump above"
// resolution), or nil at the top level.
func (r *runner) run(steps []playStep, currentRecord *optionRecord) {
	if r.cancelled {
		return
	}
	previous := r.currentRecord
	r.currentRecord = currentRecord
	stopped := false
	queue := flatten(steps, r.flattenOptions(&stopped))
	r.currentRecord = previous

	chain := NewDialogueChainBuilder()
	index := 0
	for position := range queue {
		entry := queue[position]
		after := queue[position+1:]
		rest := concatPlaySteps(wrapPlaySteps(nestedStepsOf(entry)), after)

		switch entry.kind {
		case playJumpTo:
			afterCopy := append([]playStep(nil), after...)
			jumpSteps := entry.jumpSteps
			chain.Add(r.newAction(index, func() {
				// Jumps that only ever lead to more jumps
				// would never show anything.
				r.jumps++
				if r.jumps > MaxTranscriptJumps {
					r.close()
					return
				}
				r.run(concatPlaySteps(wrapPlaySteps(jumpSteps), afterCopy), currentRecord)
			}, false))
			index++
			r.startChain(chain)
			return
		case playConditionRun:
			afterCopy := append([]playStep(nil), after...)
			run := entry.run
			chain.Add(r.newAction(index, func() {
				// Judged now, not when the queue was built:
				// the choices/actions before this point
				// have run and may have changed what the
				// guards observe.
				raw := wrapPlaySteps(run)
				chosen, ok := chooseCondition(raw, r.resolveCondition)
				if !ok {
					r.run(afterCopy, currentRecord)
					return
				}
				// A branch selected by an explicit true
				// answer keeps the sibling conditions
				// after it until the run actually ends:
				// wiki tconds continue in order. An
				// unknown fallback does not, so unrelated
				// unknown branches stay unselected.
				var tail []definition.DialogueStep
				if continues(chosen.step.Steps) || (chosen.answered && chosen.answer && isLinearConditionBranch(chosen.step.Steps)) {
					tail = run[chosen.index+1:]
				}
				head := []playStep{{kind: playConditionChosen, chosenID: chosen.step.ID, chosenText: chosen.step.Text}}
				r.run(concatPlaySteps(head, wrapPlaySteps(chosen.step.Steps), wrapPlaySteps(tail), afterCopy), currentRecord)
			}, false))
			index++
			r.startChain(chain)
			return
		case playConditionChosen:
			chosenID, chosenText := entry.chosenID, entry.chosenText
			restCopy := append([]playStep(nil), rest...)
			chain.Add(r.newAction(index, func() {
				r.emit("npc-dialogue:condition", &NpcDialogueConditionEvent{
					Player: r.event.Player, NPC: r.event.NPC, NPCID: r.event.NPCID,
					DefinitionName: r.event.DefinitionName, DefinitionID: r.event.DefinitionID,
					Text: chosenText, StepID: chosenID,
				})
				r.run(restCopy, currentRecord)
			}, false))
			index++
			r.startChain(chain)
			return
		case playGoMenu:
			// A menu jump abandons this option; the menu
			// already owns its continuation. A parser-split
			// "shows other options" jump immediately
			// before a choice really means "show that
			// choice"; replaying the current menu forever
			// would never reach it.
			if position+1 < len(queue) && queue[position+1].kind == playOriginal && queue[position+1].step.Type == "choice" {
				continue
			}
			menu, via := entry.menu, entry.via
			chain.Add(r.newAction(index, func() {
				r.presentMenu(menu, 0, nil, via)
			}, false))
			index++
			r.startChain(chain)
			return
		}

		step := entry.step
		if entry.kind == playEnd {
			step = definition.DialogueStep{Type: "end"}
		}
		typedLine := entry.kind == playOriginal && step.Type == "line" && step.HasText
		if step.Hook == "" && (step.HasNPC || step.HasPlayer || typedLine) {
			if done := r.addSpeechLine(chain, &index, step, typedLine, rest, currentRecord); done {
				return
			}
			continue
		}

		// Every other step becomes one ActionDialogue that
		// runs when reached, then starts the chain: the tail
		// below handles hooks, messages, actions, menus,
		// random picks, and the terminal markers.
		restCopy := append([]playStep(nil), rest...)
		stepCopy := step
		chain.Add(r.newAction(index, func() {
			r.executeTailStep(stepCopy, restCopy, currentRecord)
		}, false))
		index++
		r.startChain(chain)
		return
	}
	chain.Add(r.newAction(index, func() { r.close() }, false))
	r.startChain(chain)
}

// nestedStepsOf returns the nested body a queue entry carries
// into its continuation: an original step's steps, or a jump
// target's steps. Synthetic entries carry none here (their
// branches compute their own continuations).
func nestedStepsOf(entry playStep) []definition.DialogueStep {
	switch entry.kind {
	case playOriginal:
		return entry.step.Steps
	case playJumpTo:
		return entry.jumpSteps
	default:
		return nil
	}
}

// addSpeechLine adds one speech line (or overhead line) to the
// chain, emitting npc-dialogue:line first. It returns true when
// the caller must stop building the chain (the line's
// continuation action started it, or a skipped line recursed
// into run directly), false to keep accumulating lines.
func (r *runner) addSpeechLine(chain *DialogueChainBuilder, index *int, step definition.DialogueStep, typedLine bool, rest []playStep, currentRecord *optionRecord) bool {
	player := r.event.Player
	isPlayer := step.HasPlayer || (typedLine && typedPlayerSpeaker.MatchString(step.Speaker))
	speech := step.NPCText
	if step.HasPlayer {
		speech = step.PlayerText
	} else if typedLine {
		speech = step.Text
	}
	// Let a plugin skip a line through the mutable payload, e.g.
	// a hand-over line for an item the player is no longer
	// carrying.
	request := &NpcDialogueLineEvent{
		Player: player, NPC: r.event.NPC, NPCID: r.event.NPCID,
		DefinitionName: r.event.DefinitionName, DefinitionID: r.event.DefinitionID,
		Step: step, Text: speech, Pages: r.context.Pages,
	}
	r.emit("npc-dialogue:line", request)
	if request.Skip {
		if len(step.Steps) > 0 {
			r.run(rest, currentRecord)
			return true
		}
		return false
	}
	// `text` is mutable too, so a plugin can fill in the wiki's
	// "[number]"-style blanks.
	formatted := FormatPlayerText(request.Text, player)
	if step.Presentation == "overhead" {
		r.addOverheadLine(chain, index, step, typedLine, request, formatted, rest, currentRecord)
		return true
	}
	r.playedAny = true
	// Pages as the chatbox holds them: four lines of font 497 in
	// 380 pixels (ChatTextLayout), or the character fallback.
	pages := r.pb.pageText(formatted)
	// A typed line spoken by someone other than the NPC being
	// talked to (a paired NPC, a cutscene actor) gets that
	// speaker's head. An item's own transcript has no NPC; a
	// named speaker still gets their head.
	speakerID := r.event.DefinitionID
	if request.SpeakerID != nil {
		speakerID = *request.SpeakerID
	} else if !isPlayer && typedLine && step.Speaker != "" && step.Speaker != r.event.DefinitionName {
		if id, ok := r.context.SpeakerIDByName[step.Speaker]; ok {
			speakerID = id
		}
	}
	if request.Before != nil {
		before := request.Before
		chain.Add(r.newAction(*index, func() {
			current := r.manager.GetCurrent()
			before()
			if current != nil && !sameDialogue(r.manager.GetCurrent(), current) {
				return
			}
			r.manager.Advance()
		}, false))
		*index++
	}
	expression := resolveExpression(request.Expression, step)
	// TS speakerName (NpcDialogues.plugin.js:1018):
	// request.speakerName ?? (typedLine ? step.speaker :
	// undefined). Only a defined value overrides the definition
	// name at send time (see newNPCLine); an absent speaker on
	// a typed line resolves through the definition.
	speakerName, hasSpeakerName := "", false
	if request.SpeakerName != nil {
		speakerName, hasSpeakerName = *request.SpeakerName, true
	} else if typedLine && step.HasSpeaker {
		speakerName, hasSpeakerName = step.Speaker, true
	}
	side := step.Side
	if request.HasSide {
		side = request.Side
	}
	groupID := NpcDialogueGroupLeft
	if side == "right" {
		groupID = NpcDialogueGroupRight
	}
	// `holdTicks` (set by a line handler): the line shows first
	// without "Click here to continue", which comes that many
	// ticks later (a cutscene's camera moving, as live). The
	// hold action is added before the pages exist, so it reads
	// the `held` variable at execute time, exactly as TS's
	// `let held` closure does.
	var held Dialogue
	var hold Dialogue
	if request.HoldTicks > 0 {
		holdTicks := request.HoldTicks
		holdEntry := r.newAction(*index, func() {
			if heldDialogue, ok := held.(heldDialogue); ok && heldDialogue != nil {
				heldDialogue.SendHeld(player)
			}
			r.schedule(holdTicks, func() {
				if sameDialogue(r.manager.GetCurrent(), hold) {
					r.manager.Advance()
				}
			})
		}, true)
		hold = holdEntry
		chain.Add(holdEntry)
		*index++
	}
	for _, text := range pages {
		var dialogue Dialogue
		if isPlayer {
			dialogue = r.newPlayerLine(*index, text, expression)
		} else {
			dialogue = r.newNPCLine(*index, speakerID, text, speakerName, hasSpeakerName, expression, groupID)
		}
		if held == nil {
			held = dialogue
		}
		chain.Add(dialogue)
		*index++
	}
	// `after` runs once the player continues past the line,
	// before the conversation carries on: a teleport the wiki
	// only narrates, say.
	if len(step.Steps) > 0 || request.After != nil {
		after := request.After
		restCopy := append([]playStep(nil), rest...)
		chain.Add(r.newAction(*index, func() {
			current := r.manager.GetCurrent()
			if after != nil {
				after()
			}
			if current != nil && !sameDialogue(r.manager.GetCurrent(), current) {
				return
			}
			r.run(restCopy, currentRecord)
		}, false))
		*index++
		r.startChain(chain)
		return true
	}
	return false
}

// addOverheadLine adds an overhead (force-chat) line: an optional
// before action, then a self-advancing action that speaks the
// line overhead, waits OVERHEAD_CONTINUATION_TICKS, and
// continues. It starts the chain and the caller must stop
// building.
func (r *runner) addOverheadLine(chain *DialogueChainBuilder, index *int, step definition.DialogueStep, typedLine bool, request *NpcDialogueLineEvent, formatted string, rest []playStep, currentRecord *optionRecord) {
	player := r.event.Player
	if request.Before != nil {
		before := request.Before
		chain.Add(r.newAction(*index, func() {
			current := r.manager.GetCurrent()
			before()
			if current != nil && !sameDialogue(r.manager.GetCurrent(), current) {
				return
			}
			r.manager.Advance()
		}, false))
		*index++
	}
	var overheadAction Dialogue
	restCopy := append([]playStep(nil), rest...)
	overheadAction = r.newAction(*index, func() {
		if !sameDialogue(r.manager.GetCurrent(), overheadAction) {
			return
		}
		// Resolve here, after before and its identity check:
		// ordinary speech earlier in the chain must already
		// be showing before a miss stops it.
		isExplicitCast := request.Actors != nil
		actors := r.overheadActors(step, typedLine, request.Actors)
		actorsAvailable := func() bool {
			if actors == nil {
				return false
			}
			for _, actor := range actors {
				if isExplicitCast {
					if !r.explicitlyBoundActorAvailable(actor) {
						return false
					}
				} else if !r.actorAvailable(actor) {
					return false
				}
			}
			return true
		}
		// A speaker out of view has nobody to say it over,
		// and the steps after the line may be ones the quest
		// can't go on without. Unless a quest bound its own
		// cast, the line goes to chat marked as theirs and
		// the chain goes on.
		outOfView := actors == nil && !isExplicitCast && player.IsRegistered()
		if !outOfView && !actorsAvailable() {
			r.unavailable()
			r.manager.Reset()
			return
		}
		r.playedAny = true
		if outOfView {
			name := r.event.DefinitionName
			if typedLine {
				name = step.Speaker
			}
			if name != "" {
				player.SendMessage(name + ": " + formatted)
			} else {
				player.SendMessage(formatted)
			}
		} else {
			for _, actor := range actors {
				actor.ForceChat(formatted)
			}
		}
		r.schedule(OverheadContinuationTicks, func() {
			// A replacement dialogue owns the chat now;
			// never close or continue it.
			if !sameDialogue(r.manager.GetCurrent(), overheadAction) {
				return
			}
			// The out-of-view line has no actor to
			// recheck, so the player is rechecked alone.
			if outOfView {
				if !player.IsRegistered() {
					r.unavailable()
					r.manager.Reset()
					return
				}
			} else if !actorsAvailable() {
				r.unavailable()
				r.manager.Reset()
				return
			}
			if request.After != nil {
				request.After()
			}
			if !sameDialogue(r.manager.GetCurrent(), overheadAction) {
				return
			}
			r.run(restCopy, currentRecord)
			// run([]) plays the normal close action; clear
			// it so the final overhead leaves no active
			// ActionDialogue behind.
			if len(restCopy) == 0 {
				r.manager.Reset()
			}
		})
	}, true)
	chain.Add(overheadAction)
	*index++
	r.startChain(chain)
}

// executeTailStep runs one non-speech queue step when its chain
// entry is reached. It ports the tail ActionDialogue body
// (NpcDialogues.plugin.js:1042-1191) in TS order: quest hooks,
// message steps, generic action steps, terminal markers, calls,
// menus, random picks, shop/slayer actions, and the unknown-step
// fallback.
func (r *runner) executeTailStep(step definition.DialogueStep, rest []playStep, currentRecord *optionRecord) {
	player := r.event.Player
	// Action steps can carry a quest slug; emit it, then
	// continue the branch.
	if step.Hook != "" {
		hook := &NpcDialogueHookEvent{
			Player: player, NPC: r.event.NPC, NPCID: r.event.NPCID,
			DefinitionName: r.event.DefinitionName, DefinitionID: r.event.DefinitionID,
			Hook: step.Hook, Quest: step.Quest, Action: step.Action,
		}
		current := r.manager.GetCurrent()
		r.emit("npc-dialogue:hook", hook)
		if current != nil && !sameDialogue(r.manager.GetCurrent(), current) {
			return
		}
		if hook.End {
			r.close()
			return
		}
		r.run(rest, currentRecord)
		return
	}
	if step.Type == "message" {
		r.executeMessageStep(step, rest, currentRecord)
		return
	}
	action := &NpcDialogueActionEvent{
		Player: player, NPC: r.event.NPC, NPCID: r.event.NPCID,
		DefinitionName: r.event.DefinitionName, DefinitionID: r.event.DefinitionID,
		Step: step, Pages: r.context.Pages,
		Text: step.Text, Action: step.Action, Target: step.Target, StepID: step.ID,
	}
	current := r.manager.GetCurrent()
	r.emit("npc-dialogue:action", action)
	// Quest actions may start a different conversation (for
	// example, after a cutscene action). Do not let this
	// transcript's remaining tail replace it.
	if current != nil && !sameDialogue(r.manager.GetCurrent(), current) {
		return
	}
	// A handler can also hand back `steps` to play first (a
	// story picked from another page).
	if action.Handled {
		if action.End {
			r.close()
			return
		}
		r.run(concatPlaySteps(wrapPlaySteps(action.Steps), rest), currentRecord)
		return
	}
	if step.Type == "end" {
		r.close()
		return
	}
	// Wiki markers for content this server does not implement.
	if step.Type == "unavailable" || step.Type == "reference" {
		r.unavailable()
		return
	}
	if step.Type == "call" {
		if branch, ok := r.branches[step.Branch]; ok {
			r.run(concatPlaySteps(wrapPlaySteps(branch), rest), currentRecord)
			return
		}
		// A call to a missing branch falls through to the
		// unknown-step fallback below (TS: no branch match,
		// no other branch matches, unavailable()).
	}
	if step.Type == "choice" {
		stepCopy := step
		r.presentMenu(&transcriptMenu{step: &stepCopy, rest: rest, record: currentRecord}, 0, nil, "")
		return
	}
	if step.Type == "random" {
		r.executeRandomStep(step, rest, currentRecord)
		return
	}
	if step.Type == "action" && step.Action == "open_shop" {
		if r.executeShopStep(step) {
			return
		}
	}
	if step.Type == "action" && step.Action == "slayer_assignment" {
		request := &SlayerAssignmentEvent{
			Player: player, Master: step.Target, NPCID: r.event.NPCID,
			DefinitionID: r.event.DefinitionID, NPCName: r.event.DefinitionName,
		}
		r.emit("slayer:assignment", request)
		if request.Line != "" {
			line := []definition.DialogueStep{{NPCText: request.Line, HasNPC: true}}
			r.run(concatPlaySteps(wrapPlaySteps(line), rest), currentRecord)
			return
		}
	}
	if step.Type == "action" && step.Action == "slayer_task_tip" {
		request := &SlayerTaskTipEvent{Player: player, Master: step.Target}
		r.emit("slayer:task-tip", request)
		if request.Line != "" {
			line := []definition.DialogueStep{{NPCText: request.Line, HasNPC: true}}
			r.run(concatPlaySteps(wrapPlaySteps(line), rest), currentRecord)
			return
		}
	}
	if step.Type == "action" {
		// The step was already emitted with its id above. An
		// unhandled prose direction ("The player lights a
		// tinderbox.") has no further contract.
		r.run(rest, currentRecord)
		return
	}
	// Unknown step type with no executable contract. Stop
	// safely.
	r.unavailable()
}

// executeMessageStep ports the message-step branch
// (NpcDialogues.plugin.js:1066-1103): the kind:"message" event,
// the replacement-dialogue check, the item/statement box path,
// and the plain chat-message path.
func (r *runner) executeMessageStep(step definition.DialogueStep, rest []playStep, currentRecord *optionRecord) {
	player := r.event.Player
	// Item hand-outs are also `message` steps; let quests hook
	// their id.
	message := &NpcDialogueActionEvent{
		Player: player, NPC: r.event.NPC, NPCID: r.event.NPCID,
		DefinitionName: r.event.DefinitionName, DefinitionID: r.event.DefinitionID,
		Step: step, Pages: r.context.Pages,
		Text: step.Text, Target: step.Target, StepID: step.ID, Kind: "message",
	}
	previous := r.manager.GetCurrent()
	r.emit("npc-dialogue:action", message)
	replacement := r.manager.GetCurrent()
	if replacement != nil && !sameDialogue(replacement, previous) {
		if !message.Handled {
			r.event.Player.SendMessage(FormatPlayerText(message.Text, player))
		}
		return
	}
	if message.End {
		r.close()
		return
	}
	r.playedAny = true
	var first, second int
	hasFirst, hasSecond := false, false
	if message.Box != nil {
		if len(message.Box.Items) > 0 {
			first, hasFirst = message.Box.Items[0], true
		}
		if len(message.Box.Items) > 1 {
			second, hasSecond = message.Box.Items[1], true
		}
	}
	if hasFirst || (message.Box != nil && message.Box.Statement) {
		text := FormatPlayerText(message.Text, player)
		box := NewDialogueChainBuilder()
		// `box.holdTicks` (statement boxes): the box first
		// shows that many ticks without "Click here to
		// continue" while a scene plays out, as live sends
		// it; then it gains its continue, or with
		// `box.advance` the conversation moves on by itself.
		holdTicks := 0
		if message.Box.Statement && message.Box.HoldTicks > 0 {
			holdTicks = message.Box.HoldTicks
		}
		boxIndex := 0
		if holdTicks > 0 {
			var held Dialogue
			held = r.newAction(boxIndex, func() {
				if r.pb.deps.SendHeldStatement != nil {
					r.pb.deps.SendHeldStatement(player, text)
				}
				r.schedule(holdTicks, func() {
					if sameDialogue(r.manager.GetCurrent(), held) {
						r.manager.Advance()
					}
				})
			}, true)
			box.Add(held)
			boxIndex++
		}
		if holdTicks == 0 || !message.Box.Advance {
			if message.Box.Statement {
				box.Add(r.newStatement(boxIndex, text))
			} else if !hasSecond {
				box.Add(r.newItemStatement(boxIndex, first, text))
			} else {
				box.Add(r.newDoubleItemStatement(boxIndex, first, second, text))
			}
			boxIndex++
		}
		after := message.After
		restCopy := append([]playStep(nil), rest...)
		box.Add(r.newAction(boxIndex, func() {
			if after != nil {
				after()
			}
			r.run(restCopy, currentRecord)
		}, false))
		r.startChain(box)
		return
	}
	if !message.Handled {
		// NOTE: the plain path re-reads the step's text, not
		// the (possibly rewritten) message text — exactly as
		// TS does (only the replacement path above uses
		// message.text).
		player.SendMessage(FormatPlayerText(step.Text, player))
	}
	r.run(rest, currentRecord)
}

// executeRandomStep ports the random-step branch
// (NpcDialogues.plugin.js:1131-1148): guarded alternatives are
// left out once a plugin answers their condition false, an
// empty random continues with whatever follows, and a hooked
// alternative is unavailable.
func (r *runner) executeRandomStep(step definition.DialogueStep, rest []playStep, currentRecord *optionRecord) {
	var options []definition.DialogueOptionStep
	for _, option := range step.Options {
		if option.Condition == "" {
			options = append(options, option)
			continue
		}
		conditionID := option.ConditionID
		if conditionID == "" {
			conditionID = option.ID
		}
		if answer, answered := r.resolveCondition(option.Condition, conditionID); !answered || answer {
			options = append(options, option)
		}
	}
	if len(options) == 0 {
		r.run(rest, currentRecord)
		return
	}
	index := r.pb.random(len(options))
	option := options[index]
	if option.Hook != "" {
		r.unavailable()
		return
	}
	// The record lets an "above" jump see the other random
	// alternatives ({{tact|above}} = "same as the alternative
	// above").
	steps := option.Steps
	if steps == nil {
		steps = []definition.DialogueStep{}
	}
	record := &optionRecord{
		id: -1, text: option.Text, steps: steps,
		randomOptions: options, randomIndex: index, hasRandom: true,
	}
	r.run(concatPlaySteps(wrapPlaySteps(option.Steps), rest), record)
}

// executeShopStep ports the open_shop branch
// (NpcDialogues.plugin.js:1149-1158): the npc-dialogue:shop
// version-picking event is emitted and the shop is resolved by
// name (preferring the core shop when a name has several), then
// opened through the installed OpenShop seam (milestone 7). It
// returns true only when a shop actually opened; with no
// installed seam, or an unresolvable name, the branch falls
// through to the next step.
func (r *runner) executeShopStep(step definition.DialogueStep) bool {
	target := r.shopName(step)
	var named []TranscriptShop
	if r.pb.deps.FindShops != nil {
		named = r.pb.deps.FindShops(target)
	}
	// A skill master's shop also has its skillcape copies under
	// its name: open the shops.json shop, which that plugin
	// turns into the copy for a player with 99.
	shops := named
	if len(named) > 1 {
		shops = nil
		for _, shop := range named {
			if shop.Source == "core" {
				shops = append(shops, shop)
			}
		}
	}
	if len(shops) == 1 {
		r.close()
		if r.pb.deps.OpenShop != nil && r.pb.deps.OpenShop(r.event.Player, shops[0].ID) {
			return true
		}
	}
	return false
}

// shopName ports shopName (NpcDialogues.plugin.js:578-584): a
// shop page split into versions lists them in step.shops;
// "npc-dialogue:shop" lets a plugin pick one by game state, else
// the Default version opens. Without a shops list the target
// opens directly.
func (r *runner) shopName(step definition.DialogueStep) string {
	if !step.HasShops {
		return step.Target
	}
	request := &NpcDialogueShopEvent{
		Player: r.event.Player, NPCID: r.event.NPCID,
		Target: step.Target, Shops: step.Shops,
	}
	r.emit("npc-dialogue:shop", request)
	if request.Shop != nil {
		return *request.Shop
	}
	for _, name := range step.Shops {
		if strings.HasSuffix(name, " (Default)") {
			return name
		}
	}
	return ""
}

// --- Menus ---------------------------------------------------------

// presentMenu ports presentMenu (NpcDialogues.plugin.js:776-851):
// the npc-dialogue:menu notice, the warning path, the guarded
// option filter, 5-option pagination with "More...", the
// single-option "Goodbye.", and the empty-menu continuation.
// `via` is how a menu jump reached it ("other", or "" normally).
func (r *runner) presentMenu(menu *transcriptMenu, offset int, tail []playStep, via string) {
	if r.cancelled {
		return
	}
	player := r.event.Player
	step := menu.step
	r.currentMenu = menu
	history := make([]NpcDialogueMenuHistory, 0, len(r.menuHistory))
	for _, shown := range r.menuHistory {
		history = append(history, NpcDialogueMenuHistory{Step: *shown.step})
	}
	prompt := step.Prompt
	if prompt == "" {
		prompt = "Select an option"
	}
	notice := &NpcDialogueMenuEvent{
		Player: player, NPCID: r.event.NPCID,
		DefinitionName: r.event.DefinitionName, DefinitionID: r.event.DefinitionID,
		Prompt: prompt, Options: step.Options, History: history, Via: via,
	}
	if r.hasLastChoice {
		notice.Chosen = r.lastChoice
	}
	r.emit("npc-dialogue:menu", notice)
	// A plugin asked the question itself.
	if notice.Handled {
		return
	}
	if notice.Warning != "" && !r.warnedMenus[step] {
		r.warnedMenus[step] = true
		warning := notice.Warning
		tailCopy := append([]playStep(nil), tail...)
		chain := NewDialogueChainBuilder()
		chain.Add(r.newStatement(0, warning))
		chain.Add(r.newAction(1, func() {
			r.presentMenu(menu, offset, tailCopy, via)
		}, false))
		r.startChain(chain)
		return
	}
	if !menuHistoryContains(r.menuHistory, menu) {
		r.menuHistory = append(r.menuHistory, menu)
	}
	// An option the wiki guards is hidden only once a plugin
	// answers its condition false; unanswered ones still show.
	offered := notice.Options
	if offered == nil {
		offered = step.Options
	}
	// An option borrowed from an earlier menu continues as it
	// did there, with that menu's own steps.
	ownMenu := func(option *definition.DialogueOptionStep) *transcriptMenu {
		if optionBelongsToMenu(option, step, offered) {
			return menu
		}
		for _, shown := range r.menuHistory {
			if optionBelongsToMenu(option, shown.step, shown.step.Options) {
				return shown
			}
		}
		return menu
	}
	var options []*definition.DialogueOptionStep
	for i := range offered {
		option := &offered[i]
		if option.Condition != "" {
			conditionID := option.ConditionID
			if conditionID == "" {
				conditionID = option.ID
			}
			if answer, answered := r.resolveCondition(option.Condition, conditionID); answered && !answer {
				continue
			}
		}
		options = append(options, option)
	}
	more := len(options)-offset > 5
	visibleCount := 5
	if more {
		visibleCount = 4
	}
	var visible []*definition.DialogueOptionStep
	if offset < len(options) {
		end := offset + visibleCount
		if end > len(options) {
			end = len(options)
		}
		visible = options[offset:end]
	}
	// Record every option when the prompt is shown (not on
	// selection) so that a nested option's "jump above" can
	// find its unselected sibling by text.
	for _, option := range visible {
		r.recordOption(option)
	}
	var pairs []MenuOption
	for _, option := range visible {
		option := option
		// A dump-gap option can carry blank text (the wording
		// lives in its condition or its first player line);
		// fall back so the branch stays reachable.
		label := strings.TrimSpace(FormatPlayerText(option.Text, player))
		if label == "" {
			label = strings.TrimSpace(stripConditionPrefix(option.Condition))
		}
		if label == "" {
			for _, entry := range option.Steps {
				if entry.HasPlayer {
					label = entry.PlayerText
					break
				}
			}
		}
		if label == "" {
			label = "Continue."
		}
		pairs = append(pairs, MenuOption{Label: label, Select: func() {
			if r.cancelled {
				return
			}
			// The guard was answered when the menu was
			// drawn; worn gear can come off while it stays
			// open, so ask again and draw the menu afresh
			// if it no longer holds.
			if option.Condition != "" {
				conditionID := option.ConditionID
				if conditionID == "" {
					conditionID = option.ID
				}
				if answer, answered := r.resolveCondition(option.Condition, conditionID); answered && !answer {
					r.presentMenu(menu, offset, tail, "")
					return
				}
			}
			// Quest-gated choices carry a slug; let the
			// owning quest run its action, then play the
			// branch.
			if option.Hook != "" {
				hook := &NpcDialogueHookEvent{
					Player: player, NPC: r.event.NPC, NPCID: r.event.NPCID,
					DefinitionName: r.event.DefinitionName, DefinitionID: r.event.DefinitionID,
					Hook: option.Hook, Quest: option.Quest, Option: option.Text,
				}
				r.emit("npc-dialogue:hook", hook)
				// The native option prompt is already
				// closed; preserve any replacement UI.
				if hook.End {
					return
				}
			}
			choice := &NpcDialogueChoiceEvent{
				Player: player, NPC: r.event.NPC, NPCID: r.event.NPCID,
				DefinitionName: r.event.DefinitionName, DefinitionID: r.event.DefinitionID,
				Option: option.Text, StepID: option.ID, Pages: r.context.Pages,
			}
			r.emit("npc-dialogue:choice", choice)
			if choice.End {
				return
			}
			own := ownMenu(option)
			r.currentMenu = own
			r.lastChoice = option.Text
			r.hasLastChoice = true
			var rest []playStep
			if own == menu {
				rest = menu.rest
			} else {
				rest = own.rest
			}
			r.run(concatPlaySteps(wrapPlaySteps(option.Steps), rest, tail), r.recordOption(option))
		}})
	}
	if more {
		tailCopy := append([]playStep(nil), tail...)
		pairs = append(pairs, MenuOption{Label: "More...", Select: func() {
			r.presentMenu(menu, offset+4, tailCopy, via)
		}})
	}
	if len(visible) == 1 {
		pairs = append(pairs, MenuOption{Label: "Goodbye.", Select: func() { r.close() }})
	}
	// A parsed menu with no options is a wiki-export gap;
	// continue the branch instead of silently closing the chat.
	if len(pairs) == 0 {
		r.run(menu.rest, menu.record)
		return
	}
	r.playedAny = true
	r.manager.Reset()
	presented := false
	if r.pb.deps.PresentMenu != nil {
		presented = r.pb.deps.PresentMenu(player, FormatPlayerText(notice.Prompt, player), pairs)
	}
	if !presented {
		r.unavailable()
	}
}

func menuHistoryContains(history []*transcriptMenu, menu *transcriptMenu) bool {
	for _, shown := range history {
		if shown == menu {
			return true
		}
	}
	return false
}

// optionBelongsToMenu reports whether an option node belongs to
// a menu's own option list. TS uses reference equality
// (Array.includes); Go option values are copied across the
// emit/present boundary, so a handler-borrowed option is
// matched structurally (text, ids, guard, hook, branch length),
// with a pointer fast path when the offered slice IS the menu's
// own slice.
func optionBelongsToMenu(option *definition.DialogueOptionStep, step *definition.DialogueStep, offered []definition.DialogueOptionStep) bool {
	if step == nil {
		return false
	}
	if len(offered) > 0 && len(step.Options) == len(offered) && &offered[0] == &step.Options[0] {
		for i := range step.Options {
			if option == &step.Options[i] {
				return true
			}
		}
	}
	for i := range step.Options {
		own := &step.Options[i]
		if own == option {
			return true
		}
		if own.Text == option.Text && own.ID == option.ID &&
			own.Condition == option.Condition && own.ConditionID == option.ConditionID &&
			own.Hook == option.Hook && own.Quest == option.Quest &&
			len(own.Steps) == len(option.Steps) {
			return true
		}
	}
	return false
}

var conditionPrefixPattern = regexp.MustCompile(`(?i)^if\s+.*?:\s*`)

// stripConditionPrefix removes a leading "If ...:" from a guard
// used as a fallback menu label
// (NpcDialogues.plugin.js:813: .replace(/^if\s+.*?:\s*/i, "")).
func stripConditionPrefix(condition string) string {
	return conditionPrefixPattern.ReplaceAllString(condition, "")
}

// --- Jump resolution ---------------------------------------------

// resolveMenuJump ports resolveMenuJump
// (NpcDialogues.plugin.js:638-650): "shows other/previous/initial
// options" replays a menu already shown. "previous2/3" walk
// further back, "initial" is the first menu of the conversation.
func (r *runner) resolveMenuJump(reference string) jumpTarget {
	lower := strings.ToLower(reference)
	current := r.currentMenu
	history := r.menuHistory
	if strings.HasPrefix(lower, "other") {
		if current == nil {
			return jumpTarget{kind: playEnd}
		}
		return jumpTarget{kind: playGoMenu, menu: current, via: "other"}
	}
	if strings.HasPrefix(lower, "initial") {
		var target *transcriptMenu
		if len(history) > 0 {
			target = history[0]
		} else if r.context.PageInitialMenu != nil {
			target = &transcriptMenu{step: r.context.PageInitialMenu}
		}
		if target == nil {
			return jumpTarget{kind: playEnd}
		}
		return jumpTarget{kind: playGoMenu, menu: target}
	}
	// "previous" with an optional number suffix. TS takes
	// Number(match[1]) whenever the capture is a non-empty
	// string — including "0", which is truthy — so back is the
	// parsed number whenever the suffix is all digits.
	rest := strings.TrimPrefix(lower, "previous")
	back := 1
	if rest != "" {
		n := 0
		allDigits := true
		for _, ch := range rest {
			if ch < '0' || ch > '9' {
				allDigits = false
				break
			}
			n = n*10 + int(ch-'0')
		}
		if allDigits {
			back = n
		}
	}
	index := len(history) - 1
	if current != nil {
		index = -1
		for i, shown := range history {
			if shown == current {
				index = i
				break
			}
		}
	}
	// "previous" on the first menu means the menu the option
	// came from; only walk further back when the conversation
	// actually has earlier menus.
	var target *transcriptMenu
	if index-back >= 0 {
		target = history[index-back]
	} else if current != nil {
		target = current
	} else if len(history) > 0 {
		target = history[len(history)-1]
	}
	if target == nil {
		return jumpTarget{kind: playEnd}
	}
	return jumpTarget{kind: playGoMenu, menu: target}
}

// resolveJump ports resolveJump (NpcDialogues.plugin.js:652-766):
// wiki jump references ("above", "below", "other", "previous",
// "initial", "continue") resolve to a menu replay, an expanded
// step list, "end", or null (fall through to the queued
// continuation). Wiki jump ids lost their targets in the dump,
// so most of the function is the similarity fallback chain, in
// TS order.
func (r *runner) resolveJump(step definition.DialogueStep, previous, nextStep *definition.DialogueStep) jumpTarget {
	reference := strings.ToLower(step.Reference)
	if strings.HasPrefix(reference, "other") || strings.HasPrefix(reference, "previous") || strings.HasPrefix(reference, "initial") {
		return r.resolveMenuJump(reference)
	}
	// The player asks one of the page's questions again: carry
	// on as that question does, without saying it twice.
	if (strings.HasPrefix(reference, "above") || strings.HasPrefix(reference, "below")) && previous != nil && previous.HasPlayer {
		question := normText(previous.PlayerText)
		asked := realBody(r.context.PageOptions[question])
		if asked == nil && len(r.recordsByText[question]) > 0 {
			asked = realBody(r.recordsByText[question][0].steps)
		}
		if asked != nil {
			repeatsLine := len(asked) > 0 && asked[0].HasPlayer && normText(asked[0].PlayerText) == normText(previous.PlayerText)
			candidate := asked
			if repeatsLine {
				candidate = asked[1:]
			}
			if body := realBody(candidate); body != nil {
				return jumpTarget{kind: playJumpTo, steps: body}
			}
			return jumpTarget{kind: playJumpTo, steps: asked}
		}
	}
	// After an NPC line, "same as above" carries on as that
	// line does where the page first has it.
	if (strings.HasPrefix(reference, "above") || strings.HasPrefix(reference, "below")) && previous != nil && previous.HasNPC {
		if lines, ok := r.context.PageLines[normText(previous.NPCText)]; ok && len(lines) > 1 {
			after := lines[1:]
			if !jumpOnly(after) && !sameJumpStep(after[0], step) {
				return jumpTarget{kind: playJumpTo, steps: after}
			}
		}
	}
	// Prefer the queued continuation to guessing another
	// historical option. Explicit inline targets and the
	// matching question/line continuations above retain
	// priority; null lets flatten continue into the queued menu.
	if strings.HasPrefix(reference, "below") && nextStep != nil && nextStep.Type == "choice" {
		return noJumpTarget()
	}
	current := r.currentRecord
	// True when a candidate would replay the branch the jump is
	// in. A wiki jump whose target was lost falls back to "the
	// most similar option", which can be the very option
	// containing the jump; resolving to "end" beats looping.
	sameBranch := func(candidate []definition.DialogueStep) bool {
		if candidate == nil || current == nil {
			return true
		}
		if sameStepSlice(candidate, current.steps) {
			return true
		}
		mine := branchSignature(current.steps)
		return len(mine) > 0 && branchSignature(candidate) == mine
	}
	// A random alternative ending "same as above" repeats the
	// previous alternative's continuation, not an unrelated
	// menu elsewhere on the page.
	if strings.HasPrefix(reference, "above") && current != nil && current.hasRandom {
		if current.randomIndex > 0 && current.randomIndex-1 < len(current.randomOptions) {
			continuation := realBody(current.randomOptions[current.randomIndex-1].Steps)
			for continuation != nil && len(continuation) > 0 {
				first := continuation[0]
				if first.HasPlayer || first.HasNPC || first.Type == "line" {
					continuation = continuation[1:]
					continue
				}
				break
			}
			continuation = realBody(continuation)
			if len(continuation) > 0 {
				return jumpTarget{kind: playJumpTo, steps: continuation}
			}
		}
	}
	key := ""
	if current != nil {
		key = normText(current.text)
	}
	// A jump never leads back into the branch it is in: that
	// replays the branch forever.
	elsewhere := func(steps []definition.DialogueStep) []definition.DialogueStep {
		if steps == nil {
			return nil
		}
		if current != nil && sameStepSlice(steps, current.steps) {
			return nil
		}
		return steps
	}
	// The same option text (normalized) shown earlier, anywhere
	// on the page.
	var sameText *optionRecord
	if current != nil {
		for _, record := range r.recordsByText[key] {
			if !sameStepSlice(record.steps, current.steps) {
				sameText = record
				break
			}
		}
	}
	var fromPage []definition.DialogueStep
	if key != "" {
		fromPage = elsewhere(r.context.PageOptions[key])
	}
	// Near text: a page option whose normalized text contains
	// (or is contained by) this one.
	var near []definition.DialogueStep
	if len(key) >= 8 {
		type nearCandidate struct {
			steps []definition.DialogueStep
			diff  int
		}
		var candidates []nearCandidate
		for _, option := range r.context.PageOptionList {
			if len(option.Key) < 8 || option.Key == key {
				continue
			}
			if !strings.Contains(option.Key, key) && !strings.Contains(key, option.Key) {
				continue
			}
			diff := len(option.Key) - len(key)
			if diff < 0 {
				diff = -diff
			}
			candidates = append(candidates, nearCandidate{steps: realBody(option.Steps), diff: diff})
		}
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].diff < candidates[j].diff })
		for _, candidate := range candidates {
			if candidate.steps != nil {
				near = candidate.steps
				break
			}
		}
	}
	// Same question reworded: best word overlap on the page,
	// above a confidence floor.
	var similar []definition.DialogueStep
	if len(key) >= 8 && current != nil {
		target := transcriptWords(current.text)
		bestScore := 0.0
		for _, option := range r.context.PageOptionList {
			steps := realBody(option.Steps)
			if steps == nil {
				continue
			}
			score := wordSimilarity(target, option.Words)
			if score > bestScore {
				bestScore, similar = score, steps
			}
		}
		if bestScore < 0.5 {
			similar = nil
		}
	}
	// "Same as above" can point at an inline line rather than
	// an option.
	var fromLine []definition.DialogueStep
	if len(key) >= 8 {
		fromLine = realBody(r.context.PageLines[key])
	}
	if strings.HasPrefix(reference, "below") {
		var next *optionRecord
		if current != nil {
			if current.id+1 >= 0 && current.id+1 < len(r.records) {
				next = r.records[current.id+1]
			}
		} else if len(r.records) > 0 {
			next = r.records[0]
		}
		var sameTextSteps []definition.DialogueStep
		if sameText != nil {
			sameTextSteps = realBody(sameText.steps)
		}
		var nextSteps []definition.DialogueStep
		if next != nil {
			nextSteps = realBody(next.steps)
		}
		for _, candidate := range [][]definition.DialogueStep{sameTextSteps, realBody(fromPage), near, similar, fromLine, nextSteps} {
			if candidate != nil && !sameBranch(candidate) {
				return jumpTarget{kind: playJumpTo, steps: candidate}
			}
		}
		if current == nil && r.context.PageMenuBefore != nil {
			return jumpTarget{kind: playGoMenu, menu: &transcriptMenu{step: r.context.PageMenuBefore}}
		}
		if len(r.records) > 0 {
			if body := realBody(r.records[len(r.records)-1].steps); body != nil {
				return jumpTarget{kind: playJumpTo, steps: body}
			}
		}
		return jumpTarget{kind: playEnd}
	}
	if !strings.HasPrefix(reference, "above") {
		return noJumpTarget()
	}
	if current == nil {
		// "same as above" outside a menu continues into the
		// menu above on the page.
		if r.context.PageMenuBefore != nil {
			return jumpTarget{kind: playGoMenu, menu: &transcriptMenu{step: r.context.PageMenuBefore}}
		}
		if len(r.records) > 0 {
			if body := realBody(r.records[len(r.records)-1].steps); body != nil {
				return jumpTarget{kind: playJumpTo, steps: body}
			}
		}
		return jumpTarget{kind: playEnd}
	}
	var prev *optionRecord
	if current.id > 0 && current.id-1 < len(r.records) {
		prev = r.records[current.id-1]
	}
	var sameTextSteps []definition.DialogueStep
	if sameText != nil {
		sameTextSteps = realBody(sameText.steps)
	}
	var prevSteps []definition.DialogueStep
	if prev != nil {
		prevSteps = realBody(prev.steps)
	}
	for _, candidate := range [][]definition.DialogueStep{sameTextSteps, realBody(fromPage), near, similar, fromLine, prevSteps} {
		if candidate != nil && !sameBranch(candidate) {
			return jumpTarget{kind: playJumpTo, steps: candidate}
		}
	}
	return jumpTarget{kind: playEnd}
}

// sameJumpStep approximates TS's after[0] !== step identity check
// for the "same as above" line continuation: with value-type
// steps there is no object identity to compare, so a jump step
// with the same reference and anchor id as the jump being
// resolved counts as the same step. Distinct jumps sharing both
// are vanishingly rare in the dump.
func sameJumpStep(candidate, step definition.DialogueStep) bool {
	return candidate.Type == "jump" && step.Type == "jump" &&
		candidate.Reference == step.Reference && candidate.ID == step.ID
}

// sameStepSlice reports whether two step slices share the same
// backing array at the same offset: the Go approximation of TS
// array-object identity (===). Copies made by nesting defeat it
// (the signature comparison in sameBranch is the real guard);
// direct sub-slices of record data compare equal, as in TS.
func sameStepSlice(a, b []definition.DialogueStep) bool {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return false
	}
	return &a[0] == &b[0]
}

// branchSignature is the speech signature sameBranch compares:
// the branch's spoken lines, in order, tagged by speaker
// (NpcDialogues.plugin.js:683-689).
func branchSignature(steps []definition.DialogueStep) string {
	var tokens []string
	for _, step := range steps {
		switch {
		case step.HasPlayer:
			tokens = append(tokens, "p:"+step.PlayerText)
		case step.HasNPC:
			tokens = append(tokens, "n:"+step.NPCText)
		case step.Type == "line" && step.HasText:
			tokens = append(tokens, "l:"+step.Text)
		}
	}
	return strings.Join(tokens, "|")
}

// --- Overhead actors ---------------------------------------------

// sameActorContext ports sameActorContext
// (NpcDialogues.plugin.js:66-72): overhead actors only speak
// inside the player's own plane, area and owner-only context.
// DEGENERATIONS: private areas do not exist in Go (both sides
// read as "no private area", so the term is always true), and
// NPC ownership does not exist (no NPC is owner-only, so that
// term is always true). Only the plane check is live.
func (r *runner) sameActorContext(actor DialogueActor) bool {
	if actor == nil {
		return false
	}
	playerLoc := r.event.Player.Location()
	actorLoc := actor.Location()
	return playerLoc.Z == actorLoc.Z
}

// actorAvailable ports actorAvailable
// (NpcDialogues.plugin.js:74-77): a registered actor with
// forceChat, in the player's context (or the player themself).
// Every DialogueActor has ForceChat by construction, so the
// typeof check degenerates to non-nil.
func (r *runner) actorAvailable(actor DialogueActor) bool {
	if actor == nil || !actor.IsRegistered() || !r.event.Player.IsRegistered() {
		return false
	}
	if player, ok := actor.(*Player); ok && player == r.event.Player {
		return true
	}
	return r.sameActorContext(actor)
}

// explicitlyBoundActorAvailable ports explicitlyBoundActorAvailable
// (NpcDialogues.plugin.js:79-82): an explicitly bound actor must
// also be visible and in the player's local NPC list (or be the
// player — which fails the visibility term, since players have
// no isVisible; see DialogueActor).
func (r *runner) explicitlyBoundActorAvailable(actor DialogueActor) bool {
	if !r.actorAvailable(actor) || !actor.dialogueVisible() {
		return false
	}
	if player, ok := actor.(*Player); ok && player == r.event.Player {
		return true
	}
	npc, ok := actor.(*NPC)
	if !ok {
		return false
	}
	for _, local := range r.event.Player.LocalNPCs() {
		if local == npc {
			return true
		}
	}
	return false
}

// explicitlyBoundActorsAvailable ports explicitlyBoundActorsAvailable
// (NpcDialogues.plugin.js:84-87).
func (r *runner) explicitlyBoundActorsAvailable(actors []DialogueActor) bool {
	if len(actors) == 0 {
		return false
	}
	for _, actor := range actors {
		if !r.explicitlyBoundActorAvailable(actor) {
			return false
		}
	}
	return true
}

// currentNPCAvailable ports currentNpcAvailable
// (NpcDialogues.plugin.js:96-99): the current NPC is usable only
// if it is really this local, visible, registered actor with the
// expected name.
func (r *runner) currentNPCAvailable(npc *NPC, name string) bool {
	if name == "" || npc == nil {
		return false
	}
	if !r.actorAvailable(npc) || !npc.Visible {
		return false
	}
	if r.pb.npcName(npc, r.event.Player) != name {
		return false
	}
	for _, local := range r.event.Player.LocalNPCs() {
		if local == npc {
			return true
		}
	}
	return false
}

// nearestNamedNPC ports nearestNamedNpc
// (NpcDialogues.plugin.js:102-111): the nearest registered,
// visible exact-name local NPC; lowest index then local order
// breaks ties.
func (r *runner) nearestNamedNPC(name string) *NPC {
	if name == "" {
		return nil
	}
	type match struct {
		npc      *NPC
		distance int
		index    int
		order    int
	}
	var matches []match
	playerLoc := r.event.Player.Location()
	for order, npc := range r.event.Player.LocalNPCs() {
		if npc == nil || !npc.IsRegistered() || !npc.Visible {
			continue
		}
		if r.pb.npcName(npc, r.event.Player) != name {
			continue
		}
		if !r.sameActorContext(npc) {
			continue
		}
		matches = append(matches, match{
			npc:      npc,
			distance: playerLoc.Distance(npc.Location()),
			index:    npc.Index,
			order:    order,
		})
	}
	if len(matches) == 0 {
		return nil
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].distance != matches[j].distance {
			return matches[i].distance < matches[j].distance
		}
		if matches[i].index != matches[j].index {
			return matches[i].index < matches[j].index
		}
		return matches[i].order < matches[j].order
	})
	return matches[0].npc
}

// overheadActors ports overheadActors
// (NpcDialogues.plugin.js:113-128): who speaks an overhead line.
// boundActors is the line event's Actors field: nil means unset
// (resolve from the step), non-nil means an explicit cast (all
// of it must be available, else nobody speaks).
func (r *runner) overheadActors(step definition.DialogueStep, typedLine bool, boundActors []DialogueActor) []DialogueActor {
	if boundActors != nil {
		if r.explicitlyBoundActorsAvailable(boundActors) {
			return boundActors
		}
		return nil
	}
	if step.HasPlayer {
		return []DialogueActor{r.event.Player}
	}
	if typedLine && playerSpeakerPattern.MatchString(step.Speaker) {
		return []DialogueActor{r.event.Player}
	}
	if step.HasNPC {
		name := r.event.DefinitionName
		var actor *NPC
		if r.currentNPCAvailable(r.event.NPC, name) {
			actor = r.event.NPC
		} else {
			actor = r.nearestNamedNPC(name)
		}
		if actor != nil {
			return []DialogueActor{actor}
		}
		return nil
	}
	if !typedLine {
		return nil
	}
	speaker := step.Speaker
	if speaker == "" {
		return nil
	}
	var actor *NPC
	if r.currentNPCAvailable(r.event.NPC, speaker) {
		actor = r.event.NPC
	} else {
		actor = r.nearestNamedNPC(speaker)
	}
	if actor != nil {
		return []DialogueActor{actor}
	}
	return nil
}
