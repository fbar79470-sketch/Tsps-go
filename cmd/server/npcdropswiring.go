package main

import (
	"regexp"

	"tsps-go/internal/content/npcdrops"
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
)

// NpcDrops wiring (Phase 50): the coordinator's adapter between the
// definition loader (Worker A, internal/definition) and the roller +
// plugin (Workers B/C, internal/content/npcdrops). The loader parses
// into NpcDropTable / NpcDropEntry shapes; the roller rolls over
// npcdrops.Table / Entry shapes. The two shape families are
// field-for-field mirrors (the roller package's types.go documents
// the mapping), so this file converts one to the other at the seam
// instead of forcing either side to depend on the other's types.
//
// Install order (buildServer): installNpcDropsWiring runs after
// loadGameDefinitions, once the Store exists, and before the engine
// starts. The plugin itself is inert until these seams are installed
// (its TableSource / Roller are nil until then — kills roll nothing),
// and the install only matters before ticking starts.
//
// Reload: the table source converts from the live Store on every
// call, so it is always fresh across a ::reloaddrops reload. The
// roller holds the shared-table map built here; the reloader refills
// that same map in place after a reload, so the running roller sees
// the new shared tables without being replaced (the plugin resolves
// its seams lazily at kill time — see the npcDrops comment in
// npcdrops.go — so no reinstall is needed).

// questRequirementNote is the TS quest-requirement note shape
// (NpcDrops.plugin.js:70):
// "Quest requirement: WikiQuestDropRequirement(questKey=..., mode=...)".
// The loader keeps entry notes as raw strings; the converter parses
// the quest key/mode out of them for the roller's questVerdicts.
var questRequirementNote = regexp.MustCompile(
	`^Quest requirement: WikiQuestDropRequirement\(questKey=([^,]+), mode=([A-Za-z]+)\)$`)

// parseQuestRequirement returns the (questKey, questMode) parsed from
// an entry's notes, or ("", "") when no note carries a quest
// requirement.
func parseQuestRequirement(notes []string) (string, string) {
	for _, note := range notes {
		if m := questRequirementNote.FindStringSubmatch(note); m != nil {
			return m[1], m[2]
		}
	}
	return "", ""
}

func convertNpcDropEntry(def definition.NpcDropEntry) npcdrops.Entry {
	itemID := 0
	if def.HasItemID {
		itemID = def.ItemID
	}
	questKey, questMode := parseQuestRequirement(def.Notes)
	out := npcdrops.Entry{
		ItemID:            itemID,
		Name:              def.Name,
		Quantity:          def.Quantity,
		Weight:            float64(def.Weight),
		OutOf:             float64(def.OutOf),
		Rarity:            def.Rarity,
		Section:           def.Section,
		Always:            def.Always,
		Noted:             def.Noted,
		SeparateRoll:      def.SeparateRoll,
		PreRoll:           def.PreRoll,
		OncePerKill:       def.OncePerKill,
		Nothing:           def.Nothing,
		SharedTable:       def.SharedTable,
		RollGroup:         def.RollGroup,
		GroupAccessWeight: float64(def.GroupAccessWeight),
		GroupAccessOutOf:  float64(def.GroupAccessOutOf),
		ClueScrollBox:     def.ClueScrollBox,
		QuestKey:          questKey,
		QuestMode:         questMode,
		QuantitySelection: def.QuantitySelection,
	}
	for _, alt := range def.QuantityAlternatives {
		converted := npcdrops.QuantityAlternative{Quantity: alt.Quantity}
		if alt.Condition != nil {
			converted.Condition = &npcdrops.QuantityCondition{
				Type:     alt.Condition.Type,
				Monster:  alt.Condition.Monster,
				Location: alt.Condition.Location,
			}
		}
		out.QuantityAlternatives = append(out.QuantityAlternatives, converted)
	}
	for _, w := range def.RollCountWeights {
		out.RollCountWeights = append(out.RollCountWeights, npcdrops.RollCount{
			Rolls:  w.Rolls,
			Weight: w.Weight,
		})
	}
	for _, b := range def.BonusDrops {
		out.BonusDrops = append(out.BonusDrops, npcdrops.Entry{
			ItemID:   b.ItemID,
			Name:     b.Name,
			Quantity: b.Quantity,
		})
	}
	return out
}

func convertNpcDropEntries(defs []definition.NpcDropEntry) []npcdrops.Entry {
	if len(defs) == 0 {
		return nil
	}
	out := make([]npcdrops.Entry, 0, len(defs))
	for _, def := range defs {
		out = append(out, convertNpcDropEntry(def))
	}
	return out
}

func convertNpcDropTable(def *definition.NpcDropTable) *npcdrops.Table {
	if def == nil {
		return nil
	}
	out := &npcdrops.Table{
		ID:          def.ID,
		Label:       def.Label,
		MainMaxRoll: float64(def.MainMaxRoll),
		Rolls:       def.Rolls,
		Entries:     convertNpcDropEntries(def.Entries),
		Tertiary:    convertNpcDropEntries(def.Tertiary),
	}
	if def.RewardChain != nil {
		// Presence-only: the ordered path is fail-closed in this
		// phase (plan §2 item 3); the stage shape belongs to the
		// follow-up that builds the state provider.
		out.RewardChain = &npcdrops.RewardChainInfo{
			Type:        def.RewardChain.Type,
			StateDomain: def.RewardChain.StateDomain,
		}
	}
	for _, g := range def.TertiaryGroups {
		out.TertiaryGroups = append(out.TertiaryGroups, npcdrops.TertiaryGroup{
			Type: g.Type,
			SourceAccess: npcdrops.AccessRate{
				Weight: float64(g.SourceAccess.Weight),
				OutOf:  float64(g.SourceAccess.OutOf),
			},
			Entries: convertNpcDropEntries(g.Entries),
		})
	}
	return out
}

func convertNpcDropSharedEntry(def definition.NpcDropSharedEntry) npcdrops.SharedEntry {
	itemID := 0
	if def.HasItemID {
		itemID = def.ItemID
	}
	return npcdrops.SharedEntry{
		ItemID:      itemID,
		Name:        def.Name,
		Quantity:    def.Quantity,
		Weight:      float64(def.Weight),
		OutOf:       float64(def.OutOf),
		Noted:       def.Noted,
		Nothing:     def.Nothing,
		SharedTable: def.SharedTable,
	}
}

// convertNpcDropSharedTables builds the roller's shared-table map
// from the loaded Store: flat subtables from
// Store.NpcDropSharedTable(name), the seed table from
// Store.NpcDropSeedTable(name) with its level-banded categories.
func convertNpcDropSharedTables(store *definition.Store) map[string]*npcdrops.SharedTable {
	shared := make(map[string]*npcdrops.SharedTable)
	if store == nil {
		return shared
	}
	for _, name := range store.NpcDropSharedTableNames() {
		if seed, ok := store.NpcDropSeedTable(name); ok && seed != nil {
			table := &npcdrops.SharedTable{
				Name:                  seed.Name,
				CombatLevelMultiplier: seed.Multiplier,
			}
			for _, cat := range seed.Categories {
				converted := npcdrops.SeedCategory{Minimum: cat.Minimum}
				if cat.HasMaximumExclusive {
					max := cat.MaximumExclusive
					converted.MaximumExclusive = &max
				}
				for _, e := range cat.Entries {
					converted.Entries = append(converted.Entries, convertNpcDropSharedEntry(e))
				}
				table.Categories = append(table.Categories, converted)
			}
			shared[name] = table
			continue
		}
		table := &npcdrops.SharedTable{Name: name}
		for _, e := range store.NpcDropSharedTable(name) {
			table.Entries = append(table.Entries, convertNpcDropSharedEntry(e))
		}
		shared[name] = table
	}
	return shared
}

// refillNpcDropSharedTables replaces the contents of the roller's
// shared-table map in place (same map reference, so the running
// roller sees the new tables after a reload).
func refillNpcDropSharedTables(shared map[string]*npcdrops.SharedTable, store *definition.Store) {
	for name := range shared {
		delete(shared, name)
	}
	for name, table := range convertNpcDropSharedTables(store) {
		shared[name] = table
	}
}

// npcDropTableSource is the plugin's TableSource seam over the loaded
// Store: tables are converted fresh on every call, so a
// ::reloaddrops reload is visible immediately. NPCName comes from the
// drop file's own npcs[].name — game.NPCDefinition has no Name().
type npcDropTableSource struct {
	store *definition.Store
}

func (s npcDropTableSource) TablesForNPC(npcID int) []*npcdrops.Table {
	if s.store == nil {
		return nil
	}
	tables := s.store.NpcDropTable(npcID)
	out := make([]*npcdrops.Table, 0, len(tables))
	for _, t := range tables {
		out = append(out, convertNpcDropTable(t))
	}
	return out
}

func (s npcDropTableSource) NPCName(npcID int) string {
	if s.store == nil {
		return ""
	}
	npc, ok := s.store.NpcDropNPC(npcID)
	if !ok || npc == nil {
		return ""
	}
	return npc.Name
}

// installNpcDropsWiring installs the NpcDrops content plugin's seams:
// the world's Wilderness on the plugin (for the Looting-bag gate —
// the roller reads KillContext.InWilderness, set from this checker at
// kill time), the table source over the loaded Store, the production
// roller over the loaded shared tables, and the reloader over the
// loader's ReloadNpcDrops for ::reloaddrops. It runs in buildServer
// after loadGameDefinitions (the Store does not exist until then) and
// before the engine starts; the Wilderness does not exist until
// newAreaManagerAndWilderness, and the handle is only read at kill
// time on the tick goroutine, so installing here is in time. A nil
// Wilderness reads as not in the Wilderness, so Looting-bag rows never
// drop without one. A nil Store installs only the Wilderness checker.
func installNpcDropsWiring(w *game.Wilderness, store *definition.Store) {
	if w != nil {
		npcdrops.SetWildernessChecker(w.IsInLocation)
	}
	if store == nil {
		return
	}
	shared := convertNpcDropSharedTables(store)
	npcdrops.SetTableSource(npcDropTableSource{store: store})
	npcdrops.SetRoller(npcdrops.NewRoller(shared))
	npcdrops.SetReloader(func() (int, int, error) {
		npcs, tables, err := store.ReloadNpcDrops()
		if err != nil {
			return 0, 0, err
		}
		refillNpcDropSharedTables(shared, store)
		return npcs, tables, nil
	})
}
