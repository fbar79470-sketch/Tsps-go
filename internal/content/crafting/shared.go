// Shared.Crafting.js (234 lines, commit bfd81e3): the shared
// helpers every Crafting unit uses — the level / actions /
// interval queries, the XP grant, the members filter, the spam
// channel, the next-tick runner, the skillmulti make-many menu,
// and the make-many loop.
//
// THE LOOP SHAPE (coordinator correction, 2026-11-21): do NOT
// use StartSkillable/StopSkillable for the make-many loop — no
// skill port uses that slot. Crafting follows the mining /
// smithing shape: a per-plugin sessions map plus ONE global
// 1-tick TaskManager task (ensureTask / processCraftingTick,
// the smithing sessions.go precedent). Only StopSkillable
// (walk-away) is live engine behaviour. The per-session tick
// counting is Shared.startMaking's exactly, counted from the
// tick the player picks the item: a start animation (and sound)
// on startTick, the first item on firstTick, then one every
// interval ticks, each with the animation replayed. The session
// is mirrored under the "crafting:task" attribute (TS stores the
// live task there; Go stores the session) so stopCrafting keeps
// the TS-visible contract.
//
// CreationMenu modes, grounded from TS (worker 1, 2026-11-21):
// leather/combine 0, gems cut 12, amethyst 0, glassblowing 0,
// pottery shape 0 / fire 9, spinning 16, weaving 35,
// battlestaves 0, snelm 0. Jewellery does NOT use CreationMenu
// (cache interfaces 446/6), so the 18-item CreationMenu cap
// never applies to gold (32) / silver (20). Pass the mode
// through MakeMenuOptions.Mode exactly as TS passes it to
// skillmulti.
//
// XP INTEGER TRUNCATION (gap 27a): the JSON pays float XP
// (13.8, 32.5, 6.3); Go SkillManager.AddExperiences takes an
// int, so addCraftingXp truncates at the grant site (13.8 →
// 13). The skills:produced emit still carries the float XP, as
// TS emits it. No rounding is invented.
package crafting

import (
	"math/rand/v2"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// CraftingTaskAttribute is CRAFTING_TASK_ATTRIBUTE
// (Shared.Crafting.js:13): the attribute the active make-many
// session is stored under. TS stores the live Task; Go stores
// the *craftSession. It is NOT a persisted key — it holds live
// state, cleared on stop.
const CraftingTaskAttribute = "crafting:task"

// ThreadUsesAttribute is the persisted thread counter
// (Leather.Crafting.js): persistAttribute("crafting:thread-uses",
// 0) — a reel of thread lasts 5 items.
const ThreadUsesAttribute = "crafting:thread-uses"

// spamChatType is the chat channel for recipe lines: the spam
// channel (protocol.ChatTypeSpam, 105 — the game package cannot
// import protocol, so the value is spelled out with its
// provenance, as fletching/cooking spell it out).
const spamChatType = 105

// ---- the shared instance ----

// crafting is the plugin's per-registration state: the TS
// module-level state (api, the indexed tables) as struct fields,
// per the Phase 21 race fix. One instance serves the registered
// Plugin value; tests build their own with newCrafting.
type crafting struct {
	tables *Tables
	// api is the Register-time Api, held so the hub surfaces
	// that install after plugin Load (TaskManager) can be
	// resolved lazily at use time through taskManager. Nil in
	// unit tests, which set tasks directly.
	api  *plugins.Api
	emit func(name string, payload any)
	// tasks is the directly-set TaskManager override (unit
	// tests set it); production leaves it nil and resolves
	// through the api instead — a Register-time capture would
	// read nil, because the TaskManager installs after Load.
	tasks *game.TaskManager
	// membersWorld defaults true: the TS no-world fixture path
	// allows everything, and the dev world is a members world.
	// cmd/server installs the real flag through
	// SetMembersWorld before hub Load.
	membersWorld bool
	// objectNameResolver resolves an object's display name by
	// id (the smithing SetObjectNameResolver precedent).
	// Wiring installs it from the cache definitions; without
	// it the item-on-object router cannot match furnace/wheel/
	// loom/oven names and degrades to a no-op (gap 27, the
	// Phase 43 live-found gap — do NOT repeat it).
	objectNameResolver func(id int) string
	// sessions maps each crafting player to their active
	// make-many session (the mining/smithing sessions-map
	// shape). The global 1-tick task advances them all.
	sessions map[*game.Player]*craftSession
	// pottery maps each crafting player to their active
	// bespoke pottery task (pottery.go): it is not a
	// craftSession, so it lives beside the sessions map, and
	// stopCrafting stops it too (a new loop, or an
	// interaction clear, ends a shaping/firing run, as TS's
	// stopCrafting stops the task stored under
	// "crafting:task").
	pottery map[*game.Player]*potteryTask
	// randFloat rolls [0, 1) for the gem-crush, firing-crack
	// and jeweller's-chisel double-cut rolls (math/rand/v2
	// Float64 by default; tests override it — the cooking
	// randFloat precedent).
	randFloat func() float64
	// taskScheduled guards ensureTask: the 1-tick global task
	// schedules once, on the first startMaking.
	taskScheduled bool
	// tick is the global crafting task's cycle counter (the
	// smithing tick field): sessions read it through the
	// process call, never the wall clock.
	tick int64
}

// newCrafting builds a crafting instance over tables (nil tables
// degrade every handler to a no-op; see Register in
// crafting.go).
func newCrafting(tables *Tables) *crafting {
	return &crafting{
		tables:       tables,
		membersWorld: true,
		sessions:     make(map[*game.Player]*craftSession),
		pottery:      make(map[*game.Player]*potteryTask),
		randFloat:    rand.Float64,
	}
}

// shared is the instance behind Plugin; SetTables,
// SetMembersWorld and SetObjectNameResolver operate on it.
var shared = newCrafting(nil)

// SetTables installs the resolved tables for the registered
// Plugin value. cmd/server calls it BEFORE hub Load: Register
// captures the shared instance, so a call before Load reaches
// every handler. With no tables installed, every handler
// degrades to a no-op. (The cooking SetTables precedent.)
func SetTables(t *Tables) {
	shared.tables = t
}

// TablesLoaded reports whether the tables are installed.
func TablesLoaded() bool { return shared.tables != nil }

// SetMembersWorld installs the members-world flag for the
// registered Plugin value (TS: WorldDefinition.
// isMembersWorld()). cmd/server calls it before hub Load with
// the world definition's flag; it defaults true.
func SetMembersWorld(members bool) { shared.membersWorld = members }

// MembersWorld reports the installed members-world flag (the
// shared instance's view, for tests).
func MembersWorld() bool { return shared.membersWorld }

// SetObjectNameResolver installs the object-id → display-name
// lookup for the item-on-object router. Wiring installs it from
// the cache definitions; nil clears it.
func SetObjectNameResolver(fn func(id int) string) { shared.objectNameResolver = fn }

// ObjectNameResolverInstalled reports whether the object-name
// resolver is installed — the Phase 43 live-found gap made
// test-visible: item-on-object routing is a no-op without it.
func ObjectNameResolverInstalled() bool { return shared.objectNameResolver != nil }

// objectNameOf resolves an object's display name, or "" when
// unknown (no resolver installed).
func (c *crafting) objectNameOf(id int) string {
	if c == nil || c.objectNameResolver == nil {
		return ""
	}
	return c.objectNameResolver(id)
}

// emitEvent emits a custom event through the captured emit
// func; a nil emit (before Register) drops the emit, which is
// the no-hooks default for every query.
func (c *crafting) emitEvent(name string, payload any) {
	if c == nil || c.emit == nil {
		return
	}
	c.emit(name, payload)
}

// taskManager resolves the world's TaskManager lazily: the hub
// installs it after plugin Load (nil until installed, per the
// hub contract — cmd/server installs it from the built world),
// so a Register-time capture reads nil in production. The live
// Api value wins when present; the directly-set tasks field is
// the unit-test override. (The cooking/fletching taskManager
// pattern, Phase 32 hub-contract lesson: lazy resolution,
// Schedule only on the tick goroutine.)
func (c *crafting) taskManager() *game.TaskManager {
	if c == nil {
		return nil
	}
	if c.api != nil {
		if tm := c.api.GetTaskManager(); tm != nil {
			return tm
		}
	}
	return c.tasks
}

// ---- level, actions, interval (Shared.Crafting.js) ----

// craftingLevel is the Crafting level the skill checks: the
// current level, raised by content ("skills:level-boost", a
// league relic's invisible +12). No Go answerer exists, so the
// boost defaults 0 (fail-closed, documented at the emit site —
// gap 27c). Do NOT invent a boost.
func (c *crafting) craftingLevel(player *game.Player) int {
	if player == nil || player.Skills() == nil {
		return 0
	}
	request := &LevelBoostRequest{Player: player, Skill: game.SkillCrafting, Boost: 0}
	c.emitEvent(LevelBoostEventName, request)
	boost := request.Boost
	if boost < 0 {
		boost = 0
	}
	return player.Skills().CurrentLevel(game.SkillCrafting) + boost
}

// actionsPerCycle is how many items one cycle makes
// ("skills:actions-per-cycle", a league relic's all at once); 1
// unless content says more. No Go answerer exists, so the
// default is 1 (fail-closed, documented at the emit site —
// gap 27c).
func (c *crafting) actionsPerCycle(player *game.Player, kind string) int {
	request := &ActionsPerCycleRequest{Player: player, Skill: game.SkillCrafting, Kind: kind, Actions: 1}
	c.emitEvent(ActionsPerCycleEventName, request)
	if request.Actions < 1 {
		return 1
	}
	return request.Actions
}

// actionInterval is the ticks between a loop's items: `ticks`
// unless content makes it faster ("skills:action-interval", a
// league relic's 2-tick pace), never under 1
// (SkillActions.js actionInterval, via
// skillactions.ActionInterval with saving 0 — Crafting has no
// tool saving). No Go answerer exists, so the tabled interval
// applies (gap 27c).
func (c *crafting) actionInterval(player *game.Player, kind string, ticks int) int {
	return skillactions.ActionInterval(c.emitEvent, player, game.SkillCrafting, kind, ticks, 0)
}

// produced emits "skills:produced" for a product made, with its
// amount and XP (Shared.Crafting.js produced): `kind` names the
// loop as "skills:actions-per-cycle" does; `materials` ([id,
// amount] pairs) are what one product used, where known. No Go
// consumer exists — emit anyway, harmless (gap 27c).
func (c *crafting) produced(player *game.Player, itemID, amount int, xp float64, kind string, materials [][2]int) {
	c.emitEvent(SkillsProducedEventName, &ProducedEvent{
		Player: player, Skill: game.SkillCrafting, ItemID: itemID,
		Amount: amount, XP: xp, Kind: kind, Materials: materials,
	})
}

// addCraftingXp grants Crafting XP (Shared.Crafting.js
// addCraftingXp): a no-op when xp <= 0. Fractional XP truncates
// at the grant site (int(xp)) — see the file header (gap 27a).
func (c *crafting) addCraftingXp(player *game.Player, xp float64) {
	if player == nil || xp <= 0 {
		return
	}
	if skills := player.Skills(); skills != nil {
		skills.AddExperiences(game.SkillCrafting, int(xp))
	}
}

// isMembersWorld reports whether this world is a members world
// (Shared.Crafting.js isMembersWorld).
func (c *crafting) isMembersWorld() bool {
	if c == nil {
		return true
	}
	return c.membersWorld
}

// available filters recipes to the ones this world offers:
// members recipes only on members worlds
// (Shared.Crafting.js available). T is any recipe type carrying
// its members flag; membersOf reads it. The input slice is not
// mutated.
func available[T any](recipes []T, membersOf func(T) bool, membersWorld bool) []T {
	var out []T
	for _, recipe := range recipes {
		if membersWorld || !membersOf(recipe) {
			out = append(out, recipe)
		}
	}
	return out
}

// availableFor is the method-shaped form of available over a
// crafting instance's members-world flag, for the units. (It is
// a package-level generic function because Go methods cannot
// declare type parameters.)
func availableFor[T any](c *crafting, recipes []T, membersOf func(T) bool) []T {
	return available(recipes, membersOf, c.isMembersWorld())
}

// ---- messages, articles (Shared.Crafting.js) ----

// withArticle is "a beer glass", "an unpowered staff orb"
// (Shared.Crafting.js withArticle): "an" before a vowel, else
// "a". Unlike cooking's helper it takes no explicit article —
// TS Shared has none; recipes that need one carry it in their
// `made` field instead.
func withArticle(name string) string {
	if len(name) > 0 && strings.ContainsRune("aeiouAEIOU", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}

// spam sends a SPAM-channel chat line (Shared.Crafting.js spam).
func (c *crafting) spam(player *game.Player, text string) {
	if player == nil {
		return
	}
	player.SendTypedMessage(spamChatType, text, "")
}

// animate plays the animation and the sound, both optional
// (Shared.Crafting.js animate). A zero Sound plays none
// (game.SendSound drops it).
func animate(player *game.Player, animation int, sound game.Sound) {
	if player == nil {
		return
	}
	if animation != 0 {
		player.PerformAnimation(animation, 0)
	}
	game.SendSound(player, sound)
}

// roll is one [0, 1) roll for the gem-crush, firing-crack
// and jeweller's-chisel double-cut chances (Shared's
// Math.random). A nil randFloat (a hand-built instance that
// skipped newCrafting) falls back to math/rand/v2.
func (c *crafting) roll() float64 {
	if c == nil || c.randFloat == nil {
		return rand.Float64()
	}
	return c.randFloat()
}

// ---- next tick (Shared.Crafting.js nextTick) ----

// nextTick runs fn on the tick after the click (Shared.Crafting
// .js nextTick): the one-tick delay live gives a single
// item-on-item action (a chisel on a gem too low to cut, a ball
// of wool on an amulet). The task is keyed by the player's
// index, as combat and other actions cancel a player's tasks by
// it; it runs fn only while the player is still registered. A
// nil TaskManager schedules nothing (the caller retries on the
// next click, the fletching startFletching shape).
func (c *crafting) nextTick(player *game.Player, fn func()) {
	if player == nil || fn == nil {
		return
	}
	tm := c.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		if player.IsRegistered() {
			fn()
		}
		return true
	})
	task.Name = "CraftingNextTick"
	task.Key = player.Index
	tm.Schedule(task)
}

// ---- the make-many menu (Shared.Crafting.js openMakeMenu) ----

// MakeMenuItem is one menu entry (Shared.Crafting.js
// openMakeMenu's items): the item id and its label. An empty
// Label falls back to the cache item name (player.ItemName),
// exactly as TS defaults labels to itemName(itemId).
type MakeMenuItem struct {
	ItemID int
	Label  string
}

// MakeMenuOptions is openMakeMenu's argument
// (Shared.Crafting.js openMakeMenu): the title, the skillmulti
// mode (CreationMenu modes, grounded from TS — see the file
// header: leather/combine 0, gems cut 12, amethyst 0,
// glassblowing 0, pottery shape 0 / fire 9, spinning 16,
// weaving 35, battlestaves 0, snelm 0), the items in menu order,
// the most that can be chosen (MaxAmount <= 0 means the menu
// maximum, 28 — TS callers always pass a count), and Choose,
// which runs with the chosen entry and amount.
//
// Jewellery does NOT use this menu (cache interfaces 446/6),
// so the 18-item CreationMenu cap never applies to gold (32) /
// silver (20).
type MakeMenuOptions struct {
	Title     string
	Mode      int
	Items     []MakeMenuItem
	MaxAmount int
	Choose    func(item MakeMenuItem, amount int)
}

// openMakeMenu builds a CreationMenu (title, item ids, labels
// defaulting to cache item names) and sends it
// (Shared.Crafting.js openMakeMenu). The menu is sent through
// the session's CreationMenuSender (the fletching openMenu
// precedent); script 2046 travels on opcode 110, NOT 170, and
// the mount must NOT use SendChatboxInterface (Phase 34
// corrections). A session without the sender opens nothing.
func (c *crafting) openMakeMenu(player *game.Player, opts MakeMenuOptions) {
	if player == nil || len(opts.Items) == 0 || opts.Choose == nil {
		return
	}
	byItem := make(map[int]MakeMenuItem, len(opts.Items))
	var itemIDs []int
	var labels []string
	for _, item := range opts.Items {
		byItem[item.ItemID] = item
		itemIDs = append(itemIDs, item.ItemID)
		label := item.Label
		if label == "" {
			label = player.ItemName(item.ItemID)
		}
		labels = append(labels, label)
	}
	menuOpts := game.CreationMenuOptions{Labels: labels, Mode: opts.Mode}
	if opts.MaxAmount > 0 {
		maxAmount := opts.MaxAmount
		menuOpts.MaxAmount = &maxAmount
	}
	choose := opts.Choose
	menu := game.NewCreationMenu(opts.Title, itemIDs, func(itemID, amount int) {
		if item, ok := byItem[itemID]; ok {
			choose(item, amount)
		}
	}, menuOpts)
	c.sendCreationMenu(player, menu)
}

// sendCreationMenu opens a menu on the client through the
// session's CreationMenuSender (the cooking sendCreationMenu /
// fletching openMenu precedent): a session without the sender
// opens nothing.
func (c *crafting) sendCreationMenu(player *game.Player, menu *game.CreationMenu) {
	if player == nil || menu == nil || player.Session == nil {
		return
	}
	sender, ok := player.Session.(game.CreationMenuSender)
	if !ok {
		return
	}
	lastItem, hasLastItem := player.CreationMenuLastItem()
	sender.SendCreationMenu(menu, player.ItemName, lastItem, hasLastItem)
}

// ---- the make-many loop (Shared.Crafting.js startMaking) ----

// craftSession is one player's active make-many loop (the
// smithSession shape, holding Shared.startMaking's state).
type craftSession struct {
	kind      string
	remaining int
	// startTick is the tick the anim/sound plays on; nil is
	// TS's null (no opening play — gems/amethyst pass null).
	startTick *int
	firstTick int
	pace      int
	animation int
	sound     game.Sound
	onAnimate func()
	has       func() bool
	make      func() bool
	anchor    *game.Location
	// resetAnimation resets the animation on stop, when set.
	resetAnimation bool

	startLocation game.Location
	ticks         int
}

// MakeOptions is startMaking's argument
// (Shared.Crafting.js startMaking). Amount is how many to make;
// StartTick is the tick the anim/sound plays on — nil means TS
// null (no opening play), non-nil 0 means play immediately at
// the choice; FirstTick is the tick the first item lands on;
// Interval is the ticks between items (lowered by content
// through skills:action-interval); Animation is 0 for none;
// Sound is the zero Sound for none (build it with
// game.NewSound(id, volume, delay, loopType)); OnAnimate runs
// after the anim/sound on each play, when set; Has says whether
// another item can be made; Make makes one and returns false
// when it could not; Anchor, when set, stops the loop once the
// player is more than 2 tiles from it (a furnace or wheel the
// menu was opened at); ResetAnimation resets the animation on
// stop; Kind names the loop for the actions-per-cycle /
// action-interval queries.
type MakeOptions struct {
	Amount         int
	StartTick      *int
	FirstTick      int
	Interval       int
	Animation      int
	Sound          game.Sound
	OnAnimate      func()
	Has            func() bool
	Make           func() bool
	Anchor         *game.Location
	ResetAnimation bool
	Kind           string
}

// startMaking starts the make-many loop (Shared.Crafting.js
// startMaking): it stops any running loop first, requires
// has() to start, snapshots the start location, plays the
// anim/sound at once when startTick is 0, and registers the
// session for the global 1-tick task. The task stops the loop
// when the player is unregistered, dead, moving, force-moved,
// off the start location, has an interface open, is >2 tiles
// from anchor, or has() fails. Tick counting, counted from the
// tick the player picks the item: tick N plays the anim+sound
// at startTick (and replays it on every make tick except when
// it equals startTick); the first item lands at firstTick, then
// one every interval ticks ((tick - firstTick) % pace == 0);
// each cycle makes actionsPerCycle items via make() until
// remaining hits 0 or make() returns false. A nil TaskManager
// schedules nothing and the loop never starts (the caller
// retries on the next click, the fletching startFletching
// shape) — except that has() was already checked, so the
// start is a silent no-op.
func (c *crafting) startMaking(player *game.Player, opts MakeOptions) {
	if player == nil || opts.Has == nil || opts.Make == nil {
		return
	}
	c.stopCrafting(player)
	if !opts.Has() {
		return
	}
	pace := c.actionInterval(player, opts.Kind, opts.Interval)
	sess := &craftSession{
		kind:           opts.Kind,
		remaining:      opts.Amount,
		startTick:      opts.StartTick,
		firstTick:      opts.FirstTick,
		pace:           pace,
		animation:      opts.Animation,
		sound:          opts.Sound,
		onAnimate:      opts.OnAnimate,
		has:            opts.Has,
		make:           opts.Make,
		anchor:         opts.Anchor,
		resetAnimation: opts.ResetAnimation,
		startLocation:  player.Location(),
	}
	c.sessions[player] = sess
	player.SetAttribute(CraftingTaskAttribute, sess)
	if opts.StartTick != nil && *opts.StartTick == 0 {
		c.playSession(player, sess)
	}
	c.ensureTask()
}

// playSession plays a session's anim/sound and its onAnimate
// hook (Shared.Crafting.js startMaking's play()).
func (c *crafting) playSession(player *game.Player, sess *craftSession) {
	if sess == nil {
		return
	}
	animate(player, sess.animation, sess.sound)
	if sess.onAnimate != nil {
		sess.onAnimate()
	}
}

// stopCrafting stops the player's active make-many loop
// (Shared.Crafting.js stopCrafting): the session is dropped
// and the "crafting:task" attribute cleared; the animation
// resets when the session asked for it. A player with no
// session is a no-op. A bespoke pottery task is stopped too
// (pottery.go): TS's stopCrafting stops whatever task the
// "crafting:task" attribute holds, and a pottery run is such
// a task.
func (c *crafting) stopCrafting(player *game.Player) {
	if player == nil {
		return
	}
	if task, ok := c.pottery[player]; ok {
		task.stop()
	}
	sess, ok := c.sessions[player]
	if !ok {
		player.SetAttribute(CraftingTaskAttribute, nil)
		return
	}
	delete(c.sessions, player)
	player.SetAttribute(CraftingTaskAttribute, nil)
	if sess.resetAnimation {
		player.PerformAnimation(game.ResetAnimationID, 0)
	}
}

// stopCraftingOnInteractionCleared stops the active loop when
// the player's interaction clears (Crafting.plugin.js
// stopCraftingOnInteractionCleared): walking away ends the
// make-many loop. A payload of another type is ignored.
func (c *crafting) stopCraftingOnInteractionCleared(payload any) {
	event, ok := payload.(*InteractionClearedEvent)
	if !ok || event == nil {
		return
	}
	c.stopCrafting(event.Player)
}

// advanceSession runs one session's tick (Shared.Crafting.js
// startMaking's CraftingTask.execute, driven by the global
// task): the stop set first, then the tick counting. It returns
// false when the session ends.
func (c *crafting) advanceSession(player *game.Player, sess *craftSession) bool {
	if sess == nil || sess.has == nil || sess.make == nil {
		return false
	}
	movement := player.MovementQueue()
	skills := player.Skills()
	if !player.IsRegistered() || skills == nil || skills.CurrentLevel(game.SkillHitpoints) <= 0 ||
		(movement != nil && (movement.Size() > 0 || movement.IsMoving())) || player.ForceMovement() != nil ||
		!player.Location().Equals(sess.startLocation) ||
		player.GetInterfaceId() != -1 || (sess.anchor != nil && player.Location().Distance(*sess.anchor) > 2) || !sess.has() {
		return false
	}
	tick := sess.ticks + 1
	sess.ticks = tick
	atStartTick := sess.startTick != nil && tick == *sess.startTick
	if atStartTick {
		c.playSession(player, sess)
	}
	if tick < sess.firstTick || (tick-sess.firstTick)%sess.pace != 0 {
		return true
	}
	if !atStartTick {
		c.playSession(player, sess)
	}
	// Content may make several items in the one cycle
	// ("skills:actions-per-cycle", by the loop's kind). The
	// TS loop is `if (!make() || --remaining <= 0 || !has())
	// stop` — the fused check below is the same order with
	// the decrement applied after it passes: make, then
	// remaining-after-decrement, then has().
	actions := c.actionsPerCycle(player, sess.kind)
	for done := 0; done < actions; done++ {
		if !sess.make() || sess.remaining-1 <= 0 || !sess.has() {
			return false
		}
		sess.remaining--
	}
	return true
}

// processCraftingTick runs one tick of the global CraftingTask
// (the smithing processSmithingTick shape): every session is
// advanced; a session that ends (or whose player is gone,
// dead, or walked off) is stopped, resetting the animation
// when the session asked for it.
func (c *crafting) processCraftingTick(cycle int64) {
	for player, sess := range c.sessions {
		if player == nil || !player.IsRegistered() || player.Skills() == nil ||
			player.Skills().CurrentLevel(game.SkillHitpoints) <= 0 {
			c.stopCrafting(player)
			continue
		}
		if !c.advanceSession(player, sess) {
			c.stopCrafting(player)
		}
	}
}

// ensureTask schedules the 1-tick global CraftingTask once, on
// the lazily-resolved TaskManager. Idempotent. A nil TaskManager
// schedules nothing and retries on the next call (the mining /
// smithing ensureTask shape).
func (c *crafting) ensureTask() {
	if c.taskScheduled {
		return
	}
	tm := c.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		c.tick++
		c.processCraftingTick(c.tick)
		return false
	})
	task.Name = "CraftingTask"
	tm.Schedule(task)
	c.taskScheduled = true
}

// ---- test seams ----

// setTick sets the session clock (tests drive ticks directly,
// the smithing setTick precedent).
func (c *crafting) setTick(tick int64) { c.tick = tick }

// sessionFor returns the player's active make-many session, or
// nil (tests assert on loop state).
func (c *crafting) sessionFor(player *game.Player) *craftSession {
	if c == nil {
		return nil
	}
	return c.sessions[player]
}
