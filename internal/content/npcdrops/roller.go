package npcdrops

import (
	"log"
	"math"
	"math/rand"
	"strings"
)

// RNG is the roller's randomness source: math/rand in production
// (the unseeded global, like TS Math.random — NpcDrops.plugin.js
// uses Math.random everywhere), an injectable *rand.Rand (or any
// implementation) in tests so distributions are verified exactly over
// seeded runs. *rand.Rand satisfies this interface.
type RNG interface {
	Float64() float64
	Intn(n int) int
}

// QuestVerdictFunc answers the npc-drops:quest-item question for one
// row (NpcDrops.plugin.js:974-999, questItemVerdicts): whether the
// player meets the quest condition gating the drop of itemID, whose
// requirement the loader parsed as questKey/questMode (e.g.
// "quest_ragandboneman1", "RequiresDuring"). It returns
// (allowed, answered): answered=false means no plugin answered, and
// the row rolls normally. A nil func is never answered.
//
// The verdict is keyed by item id, exactly as TS keys its verdict map
// — the first requirement seen for an item id wins.
type QuestVerdictFunc func(itemID int, questKey, questMode string) (allowed, answered bool)

// Roller is the drop-table roller. It holds the parsed shared tables
// and the injectable seams; RollTable is the per-kill entry point.
//
//   - Shared: the 13 parsed shared subtables by name (rareDrop, gem,
//     herb, seed, ...). A missing name rolls nothing.
//   - RNG: nil means the math/rand global (unseeded, like TS).
//   - QuestVerdict: nil means no quest plugin answers (see
//     QuestVerdictFunc).
//   - LegendsQuestComplete: nil means the player has NOT completed
//     Legends' Quest. There is no quest system in Go yet, so this is
//     a stub defaulting to false (plan §2 item 6); Worker C wires it
//     to the quest system when it lands. It is read at roll time, so
//     a per-kill closure may be assigned before each RollTable call.
//   - Log: nil means log.Printf with an "[npcdrops] " prefix.
type Roller struct {
	Shared               map[string]*SharedTable
	RNG                  RNG
	QuestVerdict         QuestVerdictFunc
	LegendsQuestComplete func() bool
	Log                  func(format string, args ...any)
}

// NewRoller returns a Roller over the given shared tables. The map is
// used read-only; pass nil when no shared tables are loaded (shared
// refs then roll nothing).
func NewRoller(shared map[string]*SharedTable) *Roller {
	return &Roller{Shared: shared}
}

func (r *Roller) rng() RNG {
	if r.RNG != nil {
		return r.RNG
	}
	return globalRNG{}
}

// globalRNG is the math/rand global, the production source (TS
// Math.random equivalent).
type globalRNG struct{}

func (globalRNG) Float64() float64 { return rand.Float64() }
func (globalRNG) Intn(n int) int   { return rand.Intn(n) }

func (r *Roller) logf(format string, args ...any) {
	if r.Log != nil {
		r.Log(format, args...)
		return
	}
	log.Printf("[npcdrops] "+format, args...)
}

func (r *Roller) legendsComplete() bool {
	return r.LegendsQuestComplete != nil && r.LegendsQuestComplete()
}

// RollTable rolls one kill of table and returns the drops, in TS
// rollTable order (NpcDrops.plugin.js:1001-1088):
//
//  1. Always drops: every always:true entry resolves once per kill,
//     never repeated by rolls.
//  2. Ordered chain: if the table carries a reward_chain, the main
//     roll is SKIPPED with a log line (fail closed — TS throws when
//     no npc-drops:reward-state provider answers;
//     OrderedRewards.NpcDrops.js select). Always drops and tertiary
//     still resolve. No state provider is built in this phase.
//     Otherwise rollOnce runs table.Rolls times (1 when absent).
//  3. Tertiary groups: each exclusive group picks at most one outcome.
//  4. Tertiary: each entry rolled independently, with the 1/1
//     conditional rule and the Looting-bag Wilderness gate.
//  5. Post-filter: drops whose quest verdict is false are removed,
//     then drops whose id is in ctx.HandledItemIDs are removed.
//
// A nil table rolls nothing. The returned drops are valid per
// validDrop (NpcDrops.plugin.js:1161-1168) except for the note-id swap
// and ground placement, which are Worker C's job. Deliberately NOT
// implemented in this phase (plan §2): the boss rare multiplier
// (at default config 1 every entry is in the main pool, so no boost
// logic is exact), Ring of Wealth (nothing rows stay in the
// rareDrop/gem/megaRare pools), tertiary guarantee rules and chance
// rules (no providers exist in Go — all multipliers are 1), and
// location filtering (drop-locations.json is not imported).
func (r *Roller) RollTable(table *Table, ctx KillContext) []Drop {
	if table == nil {
		return nil
	}
	verdicts := r.questVerdicts(table)

	var drops []Drop
	for i := range table.Entries {
		if table.Entries[i].Always {
			drops = append(drops, r.resolveEntry(&table.Entries[i], ctx)...)
		}
	}

	if table.RewardChain != nil {
		r.logf("ordered reward chain on table %q: no reward-state provider; main roll skipped (fail closed)", table.ID)
	} else {
		rolls := table.Rolls
		if rolls < 2 {
			rolls = 1
		}
		for roll := 0; roll < rolls; roll++ {
			drops = append(drops, r.rollOnce(table, ctx, roll == 0)...)
		}
	}

	for gi := range table.TertiaryGroups {
		g := &table.TertiaryGroups[gi]
		if g.Type != "exclusive" {
			r.logf("unsupported tertiary group type %q on table %q; group skipped", g.Type, table.ID)
			continue
		}
		if picked := r.exclusiveGroupPick(g); picked != nil {
			drops = append(drops, r.resolveEntry(picked, ctx)...)
		}
	}

	for i := range table.Tertiary {
		e := &table.Tertiary[i]
		// A quest plugin refusing the row suppresses it outright.
		if v, ok := verdicts[e.ItemID]; ok && v.answered && !v.allowed {
			continue
		}
		// A tertiary at 1/1 is a conditional drop, not a guaranteed
		// one: the Wiki writes "Always" for things that always drop
		// *given* a condition (an active clue step, a quest), and
		// the condition only exists as free text in notes. Skip it
		// unless a quest plugin answered allowed:true above.
		// (NpcDrops.plugin.js:1033-1041.)
		if v, ok := verdicts[e.ItemID]; !(ok && v.answered && v.allowed) && e.OutOf <= 1 {
			continue
		}
		// Looting-bag rows drop only in the Wilderness, never from
		// Aviansie / Abyssal demon / Black chinchompa
		// (NpcDrops.plugin.js:55-56, :1043).
		if e.Name == LootingBagName && !r.lootingBagDrops(ctx) {
			continue
		}
		if r.hits(e, 1) {
			drops = append(drops, r.resolveEntry(e, ctx)...)
		}
	}

	// Filtering after the roll keeps the other rows' published
	// weights intact; a refused row is an empty slot instead
	// (NpcDrops.plugin.js:1079-1084).
	if len(verdicts) > 0 {
		kept := drops[:0:0]
		for _, d := range drops {
			if v, ok := verdicts[d.ItemID]; ok && v.answered && !v.allowed {
				continue
			}
			kept = append(kept, d)
		}
		drops = kept
	}
	if len(ctx.HandledItemIDs) > 0 {
		kept := drops[:0:0]
		for _, d := range drops {
			if ctx.HandledItemIDs[d.ItemID] {
				continue
			}
			kept = append(kept, d)
		}
		drops = kept
	}
	return drops
}

// questVerdict is one answered-or-not quest verdict, keyed by item id.
type questVerdict struct {
	answered bool
	allowed  bool
}

// questVerdicts asks the QuestVerdict func once per distinct item id
// carrying a quest requirement, over entries then tertiary
// (NpcDrops.plugin.js:974-999). Entries without a requirement get no
// map entry (an undefined verdict: the row rolls normally).
func (r *Roller) questVerdicts(table *Table) map[int]questVerdict {
	verdicts := make(map[int]questVerdict)
	ask := func(e *Entry) {
		if e.QuestKey == "" {
			return
		}
		if _, ok := verdicts[e.ItemID]; ok {
			return
		}
		verdicts[e.ItemID] = questVerdict{}
		if r.QuestVerdict == nil {
			return
		}
		allowed, answered := r.QuestVerdict(e.ItemID, e.QuestKey, e.QuestMode)
		verdicts[e.ItemID] = questVerdict{answered: answered, allowed: allowed}
	}
	for i := range table.Entries {
		ask(&table.Entries[i])
	}
	for i := range table.Tertiary {
		ask(&table.Tertiary[i])
	}
	return verdicts
}

// lootingBagDrops is the Wilderness gate for Looting-bag rows
// (NpcDrops.plugin.js:97-101, lootingBagDrops).
func (r *Roller) lootingBagDrops(ctx KillContext) bool {
	return ctx.InWilderness && !lootingBagExcludedNPCs[ctx.NPCName]
}

// rollOnce is one main-table roll (NpcDrops.plugin.js:867-955).
// first is true only on the first of a table's rolls: pre-roll and
// separate-roll entries marked once_per_kill roll then and only then.
//
// Classification per roll, in TS order (pre_roll wins over
// separate_roll — the Corporeal Beast sigils carry both flags and go
// to the pre-roll group):
//   - always: skipped (RollTable handles them).
//   - pre_roll: independent hits() chance; FIRST hit wins and
//     replaces the main roll. Groups (roll_group): one access roll,
//     then exactly one weighted item from the group.
//   - separate_roll: independent hits() each; groups resolve to
//     exactly one weighted item via rollWeighted (no access roll).
//   - everything else with finite weight > 0: the main pool. The
//     boss-boosted pool is EMPTY in this phase (plan §2 item 7:
//     at multiplier 1 TS puts every entry in main), so every such
//     entry lands in main and exactly one weighted main roll happens
//     via rollWeighted(main, table.MainMaxRoll) when no pre-roll hit.
func (r *Roller) rollOnce(table *Table, ctx KillContext, first bool) []Drop {
	var drops []Drop
	var main []*Entry
	var preGroups, sepGroups [][]*Entry
	preGroupIndex := map[string]int{}
	sepGroupIndex := map[string]int{}
	preRollHit := false

	// Group collection keeps first-seen order (TS iterates the Maps
	// in insertion order).
	collect := func(groups *[][]*Entry, index map[string]int, name string, e *Entry) {
		if i, ok := index[name]; ok {
			(*groups)[i] = append((*groups)[i], e)
			return
		}
		index[name] = len(*groups)
		*groups = append(*groups, []*Entry{e})
	}

	for i := range table.Entries {
		e := &table.Entries[i]
		switch {
		case e.Always:
			continue
		case e.PreRoll:
			if e.OncePerKill && !first {
				continue
			}
			if e.RollGroup != "" {
				collect(&preGroups, preGroupIndex, e.RollGroup, e)
			} else if !preRollHit && r.hits(e, 1) {
				drops = append(drops, r.resolveEntry(e, ctx)...)
				preRollHit = true
			}
		case e.SeparateRoll:
			if e.OncePerKill && !first {
				continue
			}
			if e.RollGroup != "" {
				collect(&sepGroups, sepGroupIndex, e.RollGroup, e)
			} else if r.hits(e, 1) {
				drops = append(drops, r.resolveEntry(e, ctx)...)
			}
		default:
			if isFiniteWeight(e.Weight) && e.Weight > 0 {
				main = append(main, e)
			}
		}
	}

	// Weighted pre-roll groups: one access roll, then exactly one
	// weighted item. A hit replaces the main roll, like the source's
	// "1/585 chance of hitting the sigil drop table"
	// (NpcDrops.plugin.js:916-935).
	for _, group := range preGroups {
		if preRollHit {
			break
		}
		head := group[0]
		accessOutOf := head.GroupAccessOutOf
		accessWeight := head.GroupAccessWeight
		if !(accessOutOf > 0) || !(accessWeight > 0) || head.OutOf <= 0 {
			continue
		}
		totalWeight := 0.0
		for _, e := range group {
			totalWeight += e.Weight
		}
		boostedAccess := accessWeight * totalWeight / head.OutOf
		if r.rng().Float64()*accessOutOf >= math.Min(accessOutOf, boostedAccess) {
			continue
		}
		preRollHit = true
		if picked := r.rollWeighted(group, totalWeight); picked != nil {
			drops = append(drops, r.resolveEntry(picked, ctx)...)
		}
	}

	// Separate-roll groups always resolve to exactly one weighted
	// item (NpcDrops.plugin.js:937-940). A roll past the last slot
	// is a miss.
	for _, group := range sepGroups {
		if picked := r.rollWeighted(group, group[0].OutOf); picked != nil {
			drops = append(drops, r.resolveEntry(picked, ctx)...)
		}
	}

	if !preRollHit && len(main) > 0 {
		if picked := r.rollWeighted(main, table.MainMaxRoll); picked != nil {
			drops = append(drops, r.resolveEntry(picked, ctx)...)
		}
	}
	return drops
}

// rollWeighted is one weighted roll among entries out of outOf
// (NpcDrops.plugin.js:847-865): total is outOf when it is a positive
// integer, else the sum of the weights (an entry's own out_of is NOT
// the main-roll denominator). One uniform draw, walk cumulative
// weights, first entry where the roll goes below zero wins. A roll
// past the last slot is a miss (nil, no drop) — the empty part of the
// pool. Chance-rule multipliers would grow the pool; the multiplier
// is 1 in this phase (no providers), so no growth happens.
func (r *Roller) rollWeighted(entries []*Entry, outOf float64) *Entry {
	if len(entries) == 0 {
		return nil
	}
	total := outOf
	if !(total > 0) || total != math.Trunc(total) {
		total = 0
		for _, e := range entries {
			total += e.Weight
		}
	}
	roll := r.rng().Float64() * math.Max(1, total)
	for _, e := range entries {
		roll -= e.Weight
		if roll < 0 {
			return e
		}
	}
	return nil
}

// hits is an independent weight/out_of chance, multiplier times as
// likely, capped at 1 (NpcDrops.plugin.js:616-624). Fractional
// weights are allowed; a non-finite or non-positive weight counts as
// 1, exactly as TS does. A non-integer or non-positive out_of never
// hits.
func (r *Roller) hits(e *Entry, multiplier float64) bool {
	outOf := e.OutOf
	if outOf <= 0 || outOf != math.Trunc(outOf) {
		return false
	}
	weight := e.Weight
	if !(weight > 0) || math.IsNaN(weight) || math.IsInf(weight, 0) {
		weight = 1
	}
	return r.rng().Float64()*outOf < math.Min(outOf, weight*multiplier)
}

// resolveEntry resolves one rolled entry to drops
// (NpcDrops.plugin.js:816-845): nothing drops nothing; a shared_table
// entry rolls that subtable (repeated per RollCounts); anything
// without an item id drops nothing; otherwise the item plus its
// bonus_drops, each at a rolled quantity.
func (r *Roller) resolveEntry(e *Entry, ctx KillContext) []Drop {
	if e == nil || e.Nothing {
		return nil
	}
	if e.SharedTable != "" {
		var drops []Drop
		for n := pickRollCount(e.RollCountWeights, r.rng()); n > 0; n-- {
			drops = append(drops, r.rollSharedTable(e.SharedTable, ctx, 0)...)
		}
		return drops
	}
	// No item id: nothing to drop. (Survey: the only item-less
	// entries are the shared_table/nothing shapes handled above; a
	// row with item_id 0 is malformed data and drops nothing here
	// rather than producing an invalid drop downstream.)
	if e.ItemID <= 0 {
		return nil
	}
	drops := []Drop{{ItemID: e.ItemID, Amount: r.rollQuantity(e, ctx), Noted: e.Noted}}
	for i := range e.BonusDrops {
		b := &e.BonusDrops[i]
		if b.ItemID <= 0 {
			continue
		}
		drops = append(drops, Drop{ItemID: b.ItemID, Amount: r.rollQuantity(b, ctx), Noted: b.Noted})
	}
	return drops
}

// rollQuantity rolls an entry's quantity
// (NpcDrops.plugin.js:600-614): the chosen alternative's quantity,
// else the entry's own [min, max]; uniform integer in range. The
// draw is always consumed, even for a fixed quantity, exactly as TS
// consumes randomInt(max-min+1) unconditionally.
func (r *Roller) rollQuantity(e *Entry, ctx KillContext) int {
	qty := e.Quantity
	if alt := r.chooseAlternative(e, ctx); alt != nil {
		qty = alt.Quantity
	}
	return rollRange(qty, r.rng())
}

// rollRange is a uniform integer in [min, max]; an absent quantity
// ([0,0]) rolls 1, matching TS's "no quantity -> 1" branch.
func rollRange(qty [2]int, rng RNG) int {
	if qty == [2]int{0, 0} {
		return 1
	}
	if qty[0] < 0 || qty[1] < qty[0] {
		return 1
	}
	return qty[0] + rng.Intn(qty[1]-qty[0]+1)
}

// chooseAlternative picks the quantity alternative for this kill
// (NpcDrops.plugin.js:589-598): "unknown" selection rolls uniformly
// among the listed quantities (an assumed distribution); otherwise
// the first alternative whose condition holds, else the first
// (default) alternative. Against the empty context every condition
// is false, so the base quantity wins.
func (r *Roller) chooseAlternative(e *Entry, ctx KillContext) *QuantityAlternative {
	alts := e.QuantityAlternatives
	if len(alts) == 0 {
		return nil
	}
	if e.QuantitySelection == "unknown" {
		return &alts[r.rng().Intn(len(alts))]
	}
	for i := 1; i < len(alts); i++ {
		if conditionHolds(alts[i].Condition, ctx) {
			return &alts[i]
		}
	}
	return &alts[0]
}

// conditionHolds reports whether a source-stated quantity condition
// holds for this kill (NpcDrops.plugin.js:565-586). An unknown
// condition type never applies (TS logs a one-time warning for it;
// the roller stays silent — the roll outcome is identical).
func conditionHolds(c *QuantityCondition, ctx KillContext) bool {
	if c == nil {
		return false
	}
	switch c.Type {
	case "perfect_kill":
		return ctx.PerfectKill
	case "bounty_task":
		return ctx.BountyTask != "" && strings.EqualFold(ctx.BountyTask, c.Monster)
	case "preferred_method":
		return c.Location != "" && ctx.PreferredMethodLocation == c.Location
	default:
		return false
	}
}

// pickRollCount is RollCounts.pick (RollCounts.NpcDrops.js:14-22):
// one weighted draw over [{rolls, weight}]; a row without valid
// weights rolls once.
func pickRollCount(weights []RollCount, rng RNG) int {
	if len(weights) == 0 {
		return 1
	}
	total := 0
	for _, w := range weights {
		if w.Rolls < 1 || w.Weight < 1 {
			return 1
		}
		total += w.Weight
	}
	roll := rng.Intn(total)
	for _, w := range weights {
		roll -= w.Weight
		if roll < 0 {
			return w.Rolls
		}
	}
	return weights[len(weights)-1].Rolls
}

// RollSharedTable rolls one shared subtable by name (the
// npc-drops:roll-shared shape, NpcDrops.plugin.js:772-814): one draw
// over the pool's cumulative weights; a roll past the last slot is a
// miss, a nothing hit is no drop, a ref recurses (depth cap 4).
// Ring of Wealth is NOT implemented: nothing rows stay in the
// rareDrop/gem/megaRare pools (OSRS no-RoW behaviour, plan §2 item
// 8). An unknown or empty table rolls nothing.
func (r *Roller) RollSharedTable(name string, ctx KillContext) []Drop {
	return r.rollSharedTable(name, ctx, 0)
}

func (r *Roller) rollSharedTable(name string, ctx KillContext, depth int) []Drop {
	if depth > MaxSharedDepth {
		return nil
	}
	st := r.Shared[name]
	if st == nil {
		return nil
	}
	var pool []SharedEntry
	if len(st.Categories) > 0 {
		cat := selectSeedCategory(st.Categories, st.CombatLevelMultiplier, ctx.NPCCombatLevel, r.rng())
		if cat == nil {
			return nil
		}
		pool = cat.Entries
	} else {
		pool = st.Entries
	}
	if len(pool) == 0 {
		return nil
	}
	// No Ring of Wealth filtering in this phase: the full pool rolls.
	// outOf is entries[0].OutOf for the full pool (a filtered pool
	// would use the weight sum instead; no filtering exists here).
	outOf := pool[0].OutOf
	bound := int(outOf)
	if bound < 1 {
		bound = 1
	}
	// TS draws an integer here (randomInt), not a float
	// (NpcDrops.plugin.js:798-801).
	roll := float64(r.rng().Intn(bound))
	for i := range pool {
		e := &pool[i]
		roll -= e.Weight
		if roll < 0 {
			if e.Nothing {
				return nil
			}
			if e.SharedTable != "" {
				// Gem -> mega-rare Legends' Quest gate
				// (NpcDrops.plugin.js:743-770, :805-808): without
				// Legends' Quest the mega-rare slot is replaced by
				// a talisman — nature above ground, chaos
				// underground. The rare table's own mega-rare slot
				// needs no quest. The quest check defaults to
				// false (no quest system in Go yet).
				if name == "gem" && e.SharedTable == "megaRare" && !r.legendsComplete() {
					return []Drop{{ItemID: r.locationTalisman(ctx), Amount: 1}}
				}
				return r.rollSharedTable(e.SharedTable, ctx, depth+1)
			}
			if e.ItemID <= 0 {
				return nil
			}
			return []Drop{{ItemID: e.ItemID, Amount: rollRange(e.Quantity, r.rng()), Noted: e.Noted}}
		}
	}
	return nil
}

// locationTalisman is the Legends-gate talisman for this kill
// (NpcDrops.plugin.js:767-770): chaos underground (map y >= 6400),
// nature above ground.
func (r *Roller) locationTalisman(ctx KillContext) int {
	if ctx.MapY >= UndergroundMapY {
		return ChaosTalismanID
	}
	return NatureTalismanID
}

// selectSeedCategory picks the seed table's band
// (SeedCategories.NpcDrops.js:31-38): combat level x multiplier is
// rolled as randomInt(combatLevel * multiplier) and the first
// category whose maximum_exclusive exceeds the roll is used; an
// unknown combat level rolls the first (lowest) category. A
// multiplier below 1 or an empty category list is invalid data (the
// loader validates per validCategories) and rolls nothing.
func selectSeedCategory(categories []SeedCategory, multiplier, combatLevel int, rng RNG) *SeedCategory {
	if len(categories) == 0 || multiplier < 1 {
		return nil
	}
	if combatLevel < 1 {
		return &categories[0]
	}
	roll := rng.Intn(combatLevel * multiplier)
	for i := range categories {
		if categories[i].MaximumExclusive == nil || roll < *categories[i].MaximumExclusive {
			return &categories[i]
		}
	}
	return &categories[len(categories)-1]
}

// exclusiveGroupPick is OrderedRewards.group
// (OrderedRewards.NpcDrops.js, group): the entries' weight/out_of
// chances are the draw (a boost would scale them; the multiplier is
// 1 here). At most one outcome is picked; a roll past the total
// chance picks nothing. TS throws on an invalid rate or a base
// chance above one; the roller logs and skips the group instead — a
// kill must never crash on table data.
func (r *Roller) exclusiveGroupPick(g *TertiaryGroup) *Entry {
	if len(g.Entries) == 0 {
		return nil
	}
	chances := make([]float64, len(g.Entries))
	base := 0.0
	for i := range g.Entries {
		e := &g.Entries[i]
		if !validExclusiveRate(e.Weight, e.OutOf) {
			r.logf("invalid exclusive tertiary rate weight=%v out_of=%v for entry %q; group skipped", e.Weight, e.OutOf, e.Name)
			return nil
		}
		chances[i] = e.Weight / e.OutOf
		base += chances[i]
	}
	if base > 1 {
		r.logf("exclusive tertiary probabilities exceed one (%v); group skipped", base)
		return nil
	}
	total := math.Max(1, sumFloats(chances))
	roll := r.rng().Float64() * total
	for i := range g.Entries {
		roll -= chances[i]
		if roll < 0 {
			return &g.Entries[i]
		}
	}
	return nil
}

// validExclusiveRate is OrderedRewards.validRate: integer weight and
// out_of with 0 < weight <= out_of.
func validExclusiveRate(weight, outOf float64) bool {
	return weight == math.Trunc(weight) && outOf == math.Trunc(outOf) &&
		weight > 0 && weight <= outOf
}

func sumFloats(xs []float64) float64 {
	total := 0.0
	for _, x := range xs {
		total += x
	}
	return total
}

func isFiniteWeight(w float64) bool {
	return !math.IsNaN(w) && !math.IsInf(w, 0)
}
