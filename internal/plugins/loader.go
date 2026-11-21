package plugins

import (
	"sort"
	"strings"
)

// loadCandidate is one plugin working its way through Hub.Load,
// mirroring PluginLoadCandidate (PluginManager.ts:178-184): the
// plugin value, its effective name and ID, its normalized
// dependencies, and — for the deferred re-queue — the Api its
// Register call received. accepted holds the config declarations
// checkConfigDeclarations kept for this candidate (nil when the
// plugin declares no config).
type loadCandidate struct {
	plugin    Plugin
	name      string
	id        string
	dependsOn []string
	deferred  bool
	api       *Api
	accepted  map[string]ConfigDeclaration
}

// effectiveNameAndID resolves the name and ID a candidate loads
// under. Name is trimmed and falls back to ID when blank (the TS
// fallback to the file name when plugin.name is missing,
// PluginManager.ts:2448-2452); ID is trimmed and defaults to Name.
// Both blank yields two blank strings, which collectCandidates skips
// with a warning — a corner TS cannot hit, because a file always has
// a name.
func effectiveNameAndID(p Plugin) (name, id string) {
	name = strings.TrimSpace(p.Name)
	id = strings.TrimSpace(p.ID)
	if id == "" {
		id = name
	}
	if name == "" {
		name = id
	}
	return name, id
}

// collectCandidates ports the pre-collection steps of
// loadFromDirectory (PluginManager.ts:725-748): the filename-switch
// pre-filter, then candidate collection.
//
// The pre-filter drops a plugin whose ID switch is explicitly false
// BEFORE its exported name is considered — TS filters files by
// filename before evaluation, so an exported-name switch cannot
// rescue a file the filename switch turned off, and Go preserves
// that quirk. Pre-filtered plugins count as disabled. A plugin with
// a nil Register is then skipped with a warning (the missing
// register(api) export check) but does NOT count as disabled, and
// counts toward enabledCount: TS counts files that passed the
// filename filter, including ones collection then skips, in the
// "no valid plugins loaded" check.
//
// It returns the collected candidates, the pre-filter disabled count,
// and the count of plugins that passed the pre-filter.
func (h *Hub) collectCandidates() (candidates []*loadCandidate, disabledCount, enabledCount int) {
	for _, p := range h.plugins {
		name, id := effectiveNameAndID(p)
		if name == "" {
			h.logf("[plugins] skipped plugin with no name or id")
			continue
		}
		if enabled, ok := h.switches[NormalizeName(id)]; ok && !enabled {
			disabledCount++
			continue
		}
		enabledCount++
		if p.Register == nil {
			h.logf("[plugins] skipped %s: missing register(api) export", id)
			continue
		}
		candidates = append(candidates, &loadCandidate{
			plugin:    p,
			name:      name,
			id:        id,
			dependsOn: h.normalizeDependencies(name, id, p.DependsOn),
		})
	}
	return candidates, disabledCount, enabledCount
}

// warnUnknownSwitches ports the unknown-switch warnings
// (PluginManager.ts:749-758): every switch naming no known plugin is
// warned about. Known names are the normalized IDs of ALL registered
// plugins — including pre-filtered ones, as TS knows every discovered
// file name — plus the normalized exported names of collected
// candidates. A switch naming only the exported name of a plugin
// collection skipped (nil Register) is therefore unknown, matching
// TS, where that exported name was never evaluated. Warning order is
// sorted: TS iterates the switches map in world.json order, which a
// Go map cannot reproduce.
func (h *Hub) warnUnknownSwitches(candidates []*loadCandidate) {
	known := make(map[string]bool, len(h.plugins)+len(candidates))
	for _, p := range h.plugins {
		_, id := effectiveNameAndID(p)
		if id != "" {
			known[NormalizeName(id)] = true
		}
	}
	for _, c := range candidates {
		known[NormalizeName(c.name)] = true
	}
	var unknown []string
	for name := range h.switches {
		if !known[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	for _, name := range unknown {
		h.logf("[plugins] world.json names a plugin that does not exist: %s", name)
	}
}

// filterToLoad ports the second filter pass of loadFromDirectory
// (PluginManager.ts:759-777): full switch resolution (exported name,
// then ID, then legacy default, then the plugin's own Enabled), then
// members-only gating on free-to-play worlds. The enabled check runs
// first, so a switch-disabled members plugin counts as disabled, not
// members-only. It returns the plugins that will load, the
// members-only count, and the newly disabled count.
func (h *Hub) filterToLoad(candidates []*loadCandidate) (toLoad []*loadCandidate, membersOnlyCount, disabledCount int) {
	for _, c := range candidates {
		if !ResolveEnabled(h.switches, h.defaultEnabled, c.name, c.id, c.plugin.Enabled) {
			disabledCount++
			continue
		}
		// Members content stays unloaded on a free-to-play world.
		if !h.membersWorld && c.plugin.Members {
			membersOnlyCount++
			continue
		}
		toLoad = append(toLoad, c)
	}
	return toLoad, membersOnlyCount, disabledCount
}

// normalizeDependencies ports normalizePluginDependencies
// (PluginManager.ts:2476-2520): a nil list means no dependencies; a
// non-array input is impossible in Go (a nil slice is the only empty
// shape), so that branch degenerates away. Blank entries are dropped
// with a warning, self-dependencies are dropped with a warning, and
// duplicates are dropped keeping first order. Entries are trimmed
// before comparison, as TS trims each dep. The ID stands in for the
// TS file basename in the warning messages.
func (h *Hub) normalizeDependencies(pluginName, id string, dependsOn []string) []string {
	normalized := make([]string, 0, len(dependsOn))
	for _, dep := range dependsOn {
		dependencyName := strings.TrimSpace(dep)
		if dependencyName == "" {
			h.logf("[plugins] %s ignored invalid dependency value in %s", pluginName, id)
			continue
		}
		if dependencyName == pluginName {
			h.logf("[plugins] %s ignored self dependency in %s", pluginName, id)
			continue
		}
		duplicate := false
		for _, seen := range normalized {
			if seen == dependencyName {
				duplicate = true
				break
			}
		}
		if !duplicate {
			normalized = append(normalized, dependencyName)
		}
	}
	return normalized
}

// loadWithDependencies ports loadPluginCandidatesWithDependencies
// (PluginManager.ts:2522-2568): multi-pass registration. Each pass
// walks the pending list in order, registering every candidate whose
// dependencies have all loaded (a dependency loaded earlier in the
// SAME pass counts — loadedByName updates immediately); candidates
// with missing dependencies wait for a later pass. A candidate whose
// Register has a deferred half is re-queued with cleared dependencies
// and its Api, and the whole pending list — deferred and waiting
// alike — is re-sorted by ID (TS sorts by pluginPath; Go sorts by
// ID, the documented analogue, with Go string ordering rather than
// localeCompare, which differs only for non-ASCII names). The
// deferred function therefore runs in the NEXT pass, never the same
// one. A pass that registers nothing breaks the loop, and every
// remaining candidate is warned about with its still-missing
// dependencies.
//
// Dependency matching is by EXACT unnormalized name against loaded
// names, as TS matches against its loadedByName set of raw plugin
// names. A failed Register removes the candidate from pending (the
// pass still counts as progress) but never adds its name, so its
// dependents stay unresolved.
func (h *Hub) loadWithDependencies(candidates []*loadCandidate) {
	pending := append([]*loadCandidate(nil), candidates...)
	loadedByName := make(map[string]bool, len(candidates))

	for len(pending) > 0 {
		progress := false
		var deferred []*loadCandidate
		for i := 0; i < len(pending); i++ {
			candidate := pending[i]
			if len(missingDeps(candidate.dependsOn, loadedByName)) > 0 {
				continue
			}
			ok := h.registerCandidate(candidate)
			if ok && !candidate.deferred {
				loadedByName[candidate.name] = true
				if candidate.plugin.RegisterDeferred != nil {
					deferred = append(deferred, &loadCandidate{
						plugin:    candidate.plugin,
						name:      candidate.name,
						id:        candidate.id,
						deferred:  true,
						api:       candidate.api,
						dependsOn: nil,
					})
				}
			}
			pending = append(pending[:i], pending[i+1:]...)
			i--
			progress = true
		}
		if len(deferred) > 0 {
			pending = append(pending, deferred...)
			sort.SliceStable(pending, func(a, b int) bool { return pending[a].id < pending[b].id })
		}
		if !progress {
			break
		}
	}
	for _, unresolved := range pending {
		missing := missingDeps(unresolved.dependsOn, loadedByName)
		h.logf("[plugins] skipped %s: unresolved dependsOn [%s]", unresolved.name, strings.Join(missing, ", "))
	}
}

// missingDeps returns the dependencies not yet loaded, in declared
// order. Matching is exact and unnormalized.
func missingDeps(dependsOn []string, loadedByName map[string]bool) []string {
	var missing []string
	for _, dep := range dependsOn {
		if !loadedByName[dep] {
			missing = append(missing, dep)
		}
	}
	return missing
}

// registerCandidate ports registerPluginCandidate
// (PluginManager.ts:2570-2590): it runs one candidate's Register (or
// RegisterDeferred for a deferred re-queue, reusing the Api from the
// first pass) and reports success. A panic — the Go equivalent of a
// TS throw — is recovered and logged with the failed-to-initialize
// shape; the plugin is NOT added to the loaded names, so dependents
// stay unresolved. A failed RegisterDeferred logs the same way but
// the plugin stays loaded, because its name was added during the
// Register pass. Hooks a failing Register managed to attach before
// panicking stay attached, as in TS.
func (h *Hub) registerCandidate(c *loadCandidate) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			h.logf("[plugins] failed to initialize %s: %v", c.id, r)
			ok = false
		}
	}()
	if c.deferred {
		if c.plugin.RegisterDeferred != nil {
			c.plugin.RegisterDeferred(c.api)
		}
		return true
	}
	api := h.newApi(c.name)
	c.plugin.Register(api)
	c.api = api
	h.loadedNames = append(h.loadedNames, c.name)
	return true
}
