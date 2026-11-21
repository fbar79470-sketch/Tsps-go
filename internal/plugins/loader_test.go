package plugins

import (
	"reflect"
	"strings"
	"testing"
)

func recorder(order *[]string, name string) func(*Api) {
	return func(*Api) { *order = append(*order, name) }
}

func TestDependencyOrderingDiamond(t *testing.T) {
	var order []string
	mk := func(name string, deps ...string) Plugin {
		return Plugin{Name: name, DependsOn: deps, Register: recorder(&order, name)}
	}
	hub, _ := newLoaderTestHub(t, []Plugin{
		mk("D", "B", "C"),
		mk("B", "A"),
		mk("C", "A"),
		mk("A"),
	}, HubOptions{})
	hub.Load()
	if !reflect.DeepEqual(order, []string{"A", "B", "C", "D"}) {
		t.Errorf("register order = %v, want A,B,C,D", order)
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"A", "B", "C", "D"}) {
		t.Errorf("Loaded = %v", got)
	}
}

func TestDependencySamePassCounts(t *testing.T) {
	// A dependency loaded earlier in the SAME pass satisfies a
	// later candidate: registration order follows list order when
	// dependencies precede their dependents.
	var order []string
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "A", Register: recorder(&order, "A")},
		{Name: "B", DependsOn: []string{"A"}, Register: recorder(&order, "B")},
		{Name: "C", DependsOn: []string{"B"}, Register: recorder(&order, "C")},
	}, HubOptions{})
	hub.Load()
	if !reflect.DeepEqual(order, []string{"A", "B", "C"}) {
		t.Errorf("register order = %v", order)
	}
}

func TestDependencyMissing(t *testing.T) {
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Needy", DependsOn: []string{"Ghost", "AlsoGhost"}, Register: func(*Api) {}},
		{Name: "Fine", Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	if !strings.Contains(log.String(), "[plugins] skipped Needy: unresolved dependsOn [Ghost, AlsoGhost]") {
		t.Errorf("missing unresolved warning:\n%s", log.String())
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Fine"}) {
		t.Errorf("Loaded = %v", got)
	}
}

func TestDependencyCycle(t *testing.T) {
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "A", DependsOn: []string{"B"}, Register: func(*Api) {}},
		{Name: "B", DependsOn: []string{"A"}, Register: func(*Api) {}},
		{Name: "C", Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	out := log.String()
	if !strings.Contains(out, "[plugins] skipped A: unresolved dependsOn [B]") ||
		!strings.Contains(out, "[plugins] skipped B: unresolved dependsOn [A]") {
		t.Errorf("missing cycle warnings:\n%s", out)
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"C"}) {
		t.Errorf("Loaded = %v", got)
	}
}

func TestDependencySelfDupAndBlankDropped(t *testing.T) {
	var order []string
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Base", Register: recorder(&order, "Base")},
		{Name: "Self", ID: "self-id", DependsOn: []string{"", "  ", "Self", "Base", "Base", " Base "}, Register: recorder(&order, "Self")},
	}, HubOptions{})
	hub.Load()
	out := log.String()
	if n := strings.Count(out, "ignored invalid dependency value in self-id"); n != 2 {
		t.Errorf("expected 2 blank-dependency warnings, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "Self ignored self dependency in self-id") {
		t.Errorf("missing self-dependency warning:\n%s", out)
	}
	if !reflect.DeepEqual(order, []string{"Base", "Self"}) {
		t.Errorf("register order = %v: Self must load once Base loads", order)
	}
}

func TestDependencyMatchingIsExact(t *testing.T) {
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Base", Register: func(*Api) {}},
		{Name: "WrongCase", DependsOn: []string{"base"}, Register: func(*Api) {}},
		{Name: "Padded", DependsOn: []string{"  Base  "}, Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	// "base" does not match "Base": no normalization in matching.
	if !strings.Contains(log.String(), "[plugins] skipped WrongCase: unresolved dependsOn [base]") {
		t.Errorf("case-differing dependency must stay unresolved:\n%s", log.String())
	}
	// Surrounding whitespace IS trimmed by normalization.
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Base", "Padded"}) {
		t.Errorf("Loaded = %v, want Base and Padded", got)
	}
}

func TestDependencyOnDisabledPluginUnresolved(t *testing.T) {
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Off", Register: func(*Api) {}},
		{Name: "Needy", DependsOn: []string{"Off"}, Register: func(*Api) {}},
	}, HubOptions{Switches: map[string]bool{"off": false}})
	hub.Load()
	if !strings.Contains(log.String(), "[plugins] skipped Needy: unresolved dependsOn [Off]") {
		t.Errorf("missing unresolved warning:\n%s", log.String())
	}
	if got := hub.Loaded(); len(got) != 0 {
		t.Errorf("Loaded = %v", got)
	}
}

func TestDeferredOrderingByID(t *testing.T) {
	var order []string
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "X", ID: "b-id", Register: recorder(&order, "regX"), RegisterDeferred: recorder(&order, "defX")},
		{Name: "Y", ID: "a-id", Register: recorder(&order, "regY"), RegisterDeferred: recorder(&order, "defY")},
	}, HubOptions{})
	hub.Load()
	// Registers run in list order in pass 1; deferreds run in the
	// next pass sorted by ID.
	if !reflect.DeepEqual(order, []string{"regX", "regY", "defY", "defX"}) {
		t.Errorf("call order = %v, want regX,regY,defY,defX", order)
	}
}

func TestDeferredRunsInNextPass(t *testing.T) {
	var order []string
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "P", Register: recorder(&order, "regP"), RegisterDeferred: recorder(&order, "defP")},
		{Name: "Q", DependsOn: []string{"P"}, Register: recorder(&order, "regQ")},
	}, HubOptions{})
	hub.Load()
	// Q's Register runs in pass 1 (P is already loaded); P's
	// deferred waits for pass 2.
	if !reflect.DeepEqual(order, []string{"regP", "regQ", "defP"}) {
		t.Errorf("call order = %v", order)
	}
}

func TestDeferredReceivesSameApi(t *testing.T) {
	var regApi, defApi *Api
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "P", Register: func(api *Api) { regApi = api }, RegisterDeferred: func(api *Api) { defApi = api }},
	}, HubOptions{})
	hub.Load()
	if regApi == nil || defApi == nil {
		t.Fatalf("apis = %v, %v", regApi, defApi)
	}
	if regApi != defApi {
		t.Error("RegisterDeferred must receive the same Api as Register")
	}
}

func TestDeferredSortReordersWaitingCandidates(t *testing.T) {
	// TS sorts the WHOLE pending list after appending deferred
	// candidates, not just the deferred ones. Here W waits on L
	// while P (deferred, smaller ID) registers in pass 1; after
	// the sort, P's deferred entry precedes W, so defP runs before
	// regW in pass 2. Without the whole-list sort, W would run
	// first (it was pending before the deferred entry existed).
	var order []string
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "W", ID: "z-id", DependsOn: []string{"L"}, Register: recorder(&order, "regW")},
		{Name: "P", ID: "a-id", Register: recorder(&order, "regP"), RegisterDeferred: recorder(&order, "defP")},
		{Name: "L", ID: "m-id", Register: recorder(&order, "regL")},
	}, HubOptions{})
	hub.Load()
	if !reflect.DeepEqual(order, []string{"regP", "regL", "defP", "regW"}) {
		t.Errorf("call order = %v, want regP,regL,defP,regW", order)
	}
}

func TestSwitchPrecedenceFileVsExported(t *testing.T) {
	noop := func(*Api) {}
	// Stage-1 quirk: an ID switch of false drops the plugin before
	// the exported-name switch is ever considered.
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "Exported", ID: "file-name", Register: noop},
	}, HubOptions{Switches: map[string]bool{"filename": false, "exported": true}})
	hub.Load()
	if got := hub.Loaded(); len(got) != 0 {
		t.Errorf("ID switch false must win at the pre-filter: Loaded = %v", got)
	}

	// Stage 2: the exported-name switch beats the ID switch.
	hub, _ = newLoaderTestHub(t, []Plugin{
		{Name: "Exported", ID: "file-name", Register: noop},
	}, HubOptions{Switches: map[string]bool{"filename": true, "exported": false}})
	hub.Load()
	if got := hub.Loaded(); len(got) != 0 {
		t.Errorf("exported switch false must disable: Loaded = %v", got)
	}

	// The exported-name switch beats the plugin's own default.
	no := false
	hub, _ = newLoaderTestHub(t, []Plugin{
		{Name: "Exported", ID: "file-name", Enabled: &no, Register: noop},
	}, HubOptions{Switches: map[string]bool{"exported": true}})
	hub.Load()
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Exported"}) {
		t.Errorf("exported switch true must beat plugin default false: Loaded = %v", got)
	}
}

func TestMembersGatingCounts(t *testing.T) {
	no := false
	noop := func(*Api) {}
	// A switch-disabled members plugin counts as disabled, not
	// members-only: the enabled check runs first.
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Gated", Members: true, Enabled: &no, Register: noop},
	}, HubOptions{MembersWorld: false})
	hub.Load()
	if !strings.Contains(log.String(), "active=0 disabled=1") || strings.Contains(log.String(), "members-only") {
		t.Errorf("counts wrong:\n%s", log.String())
	}
}

func TestUnknownSwitchWarnings(t *testing.T) {
	noop := func(*Api) {}
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Known", ID: "known-id", Register: noop},
		{Name: "Hidden", ID: "hidden-id"}, // nil Register: name never a candidate
		{Name: "Off", Register: noop},
	}, HubOptions{Switches: map[string]bool{
		"known":      true,  // exported name of a candidate: known
		"hidden-id":  true,  // ID of a skipped plugin: known (all IDs are known)
		"off":        false, // pre-filtered plugin: still known
		"nosuchplug": true,  // unknown
		"Hidden":     true,  // exported name of a nil-Register plugin: unknown
	}})
	hub.Load()
	out := log.String()
	if !strings.Contains(out, "world.json names a plugin that does not exist: nosuchplug") {
		t.Errorf("missing unknown-switch warning:\n%s", out)
	}
	if !strings.Contains(out, "world.json names a plugin that does not exist: hidden") {
		t.Errorf("exported name of a skipped plugin must be unknown:\n%s", out)
	}
	for _, known := range []string{"known", "hiddenid", "off"} {
		if strings.Contains(out, "does not exist: "+known) {
			t.Errorf("known switch %q warned about:\n%s", known, out)
		}
	}
}

func TestBlankNameFallsBackToID(t *testing.T) {
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "  ", ID: "Fallback", Register: func(*Api) {}},
		{Name: "Dep", DependsOn: []string{"Fallback"}, Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Fallback", "Dep"}) {
		t.Errorf("Loaded = %v, want blank Name to fall back to ID", got)
	}
}

func TestBlankNameAndIDSkipped(t *testing.T) {
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: " ", ID: "  ", Register: func(*Api) {}},
		{Name: "Fine", Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	if !strings.Contains(log.String(), "skipped plugin with no name or id") {
		t.Errorf("missing blank-identity warning:\n%s", log.String())
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Fine"}) {
		t.Errorf("Loaded = %v", got)
	}
}

func TestDuplicateNamesBothLoad(t *testing.T) {
	// TS never deduplicates loadedPlugins; two files exporting the
	// same name both load. Go mirrors that.
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "Dup", ID: "dup-1", Register: func(*Api) {}},
		{Name: "Dup", ID: "dup-2", Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Dup", "Dup"}) {
		t.Errorf("Loaded = %v", got)
	}
}

func TestIndependentPluginsLoadInListOrder(t *testing.T) {
	var order []string
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "C", Register: recorder(&order, "C")},
		{Name: "A", Register: recorder(&order, "A")},
		{Name: "B", Register: recorder(&order, "B")},
	}, HubOptions{})
	hub.Load()
	if !reflect.DeepEqual(order, []string{"C", "A", "B"}) {
		t.Errorf("register order = %v, want list order", order)
	}
}
