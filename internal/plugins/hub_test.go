package plugins

import (
	"bytes"
	"io"
	"log"
	"reflect"
	"strings"
	"testing"
)

// newLoaderTestHub builds a Hub whose log output lands in a buffer with no
// flags, so tests can assert on message shapes. It honors an explicit
// Logger in opts.
func newLoaderTestHub(t *testing.T, plugins []Plugin, opts HubOptions) (*Hub, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	if opts.Logger == nil {
		opts.Logger = log.New(&buf, "", 0)
	}
	return NewHub(plugins, opts), &buf
}

func TestNewHubCopiesInputsAndNormalizesSwitches(t *testing.T) {
	switches := map[string]bool{"My Plugin": false}
	config := map[string]any{"k": "v"}
	list := []Plugin{{Name: "My Plugin", Register: func(*Api) {}}}
	hub, _ := newLoaderTestHub(t, list, HubOptions{Switches: switches, PluginConfig: config})

	// Mutating the inputs after NewHub must not affect the hub.
	switches["My Plugin"] = true
	switches["other"] = true
	config["k"] = "changed"
	list[0].Name = "Renamed"

	hub.Load()
	// The raw switch key "My Plugin" normalized to "myplugin" and
	// disabled the plugin (ID defaults to Name) at the pre-filter.
	if got := hub.Loaded(); len(got) != 0 {
		t.Errorf("Loaded = %v, want empty: raw switch keys must be normalized", got)
	}
	if got := hub.GetPluginConfig("k", "def"); got != "v" {
		t.Errorf("PluginConfig = %v, want the snapshot value v", got)
	}
}

func TestLoadIdempotent(t *testing.T) {
	calls := 0
	hub, log := newLoaderTestHub(t, []Plugin{{Name: "Once", Register: func(*Api) { calls++ }}}, HubOptions{})
	hub.Load()
	hub.Load()
	hub.Load()
	if calls != 1 {
		t.Errorf("Register called %d times, want 1", calls)
	}
	if n := strings.Count(log.String(), "active=1 disabled=0"); n != 1 {
		t.Errorf("counts log appears %d times, want 1:\n%s", n, log.String())
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Once"}) {
		t.Errorf("Loaded = %v", got)
	}
}

func TestLoadCountsLog(t *testing.T) {
	no := false
	noop := func(*Api) {}
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Active", Register: noop},
		{Name: "Off", Register: noop, Enabled: &no},
		{Name: "MembersOnly", Members: true, Register: noop},
	}, HubOptions{MembersWorld: false})
	hub.Load()
	if !strings.Contains(log.String(), "active=1 disabled=1 members-only=1") {
		t.Errorf("counts log wrong:\n%s", log.String())
	}

	// On a members world the members plugin loads and the suffix
	// disappears.
	hub, log = newLoaderTestHub(t, []Plugin{
		{Name: "Active", Register: noop},
		{Name: "MembersOnly", Members: true, Register: noop},
	}, HubOptions{MembersWorld: true})
	hub.Load()
	if !strings.Contains(log.String(), "active=2 disabled=0") || strings.Contains(log.String(), "members-only") {
		t.Errorf("counts log wrong:\n%s", log.String())
	}
}

func TestLoadEmptyHub(t *testing.T) {
	hub, log := newLoaderTestHub(t, nil, HubOptions{})
	hub.Load()
	if got := hub.Loaded(); len(got) != 0 {
		t.Errorf("Loaded = %v", got)
	}
	// No discovery early-return in Go: the pipeline still runs.
	if !strings.Contains(log.String(), "active=0 disabled=0") {
		t.Errorf("counts log wrong:\n%s", log.String())
	}
	if strings.Contains(log.String(), "no valid plugins loaded") {
		t.Errorf("empty hub must not warn about no valid plugins:\n%s", log.String())
	}
}

func TestLoadNilLoggerUsesDefault(t *testing.T) {
	old := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(old)
	hub := NewHub([]Plugin{{Name: "X", Register: func(*Api) {}}}, HubOptions{})
	hub.Load() // must not panic with a nil Logger
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"X"}) {
		t.Errorf("Loaded = %v", got)
	}
}

func TestLoadEmitsPluginsLoaded(t *testing.T) {
	var got []any
	calls := 0
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "Listener", Register: func(api *Api) {
			api.OnCustomEvent("plugins:loaded", func(payload any) {
				calls++
				got = append(got, payload)
			})
		}},
		{Name: "Other", Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	if calls != 1 {
		t.Fatalf("plugins:loaded handler called %d times, want 1", calls)
	}
	if len(got) != 1 || got[0] == nil {
		t.Fatalf("plugins:loaded payload = %v, want one non-nil payload", got)
	}
	// The payload shape (PluginsLoadedEvent{Names}) is the hooks
	// worker's contract to test; the loader proves the emit fires
	// exactly once, after registration.
}

func TestLoadFailedInitRecovery(t *testing.T) {
	var order []string
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Boom", Register: func(*Api) { panic("kaboom") }},
		{Name: "Dependent", DependsOn: []string{"Boom"}, Register: func(*Api) { order = append(order, "Dependent") }},
		{Name: "Fine", Register: func(*Api) { order = append(order, "Fine") }},
	}, HubOptions{})
	hub.Load()
	out := log.String()
	if !strings.Contains(out, "[plugins] failed to initialize Boom") {
		t.Errorf("missing failed-init log:\n%s", out)
	}
	if !strings.Contains(out, "[plugins] skipped Dependent: unresolved dependsOn [Boom]") {
		t.Errorf("missing unresolved-dependent warning:\n%s", out)
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Fine"}) {
		t.Errorf("Loaded = %v, want only Fine", got)
	}
	if !reflect.DeepEqual(order, []string{"Fine"}) {
		t.Errorf("register order = %v", order)
	}
	if !strings.Contains(out, "active=1 disabled=0") {
		t.Errorf("failed init must not count as disabled:\n%s", out)
	}
}

func TestLoadFailedDeferredKeepsPluginLoaded(t *testing.T) {
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Flaky", ID: "flaky-id", Register: func(*Api) {}, RegisterDeferred: func(*Api) { panic("later boom") }},
		{Name: "Dependent", DependsOn: []string{"Flaky"}, Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	out := log.String()
	if !strings.Contains(out, "[plugins] failed to initialize flaky-id") {
		t.Errorf("failed deferred must log with the ID:\n%s", out)
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Flaky", "Dependent"}) {
		t.Errorf("Loaded = %v: a failed deferred must not un-load the plugin or block dependents", got)
	}
}

func TestLoadNilRegisterSkipped(t *testing.T) {
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "NoRegister", ID: "no-register"},
		{Name: "Fine", Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	if !strings.Contains(log.String(), "[plugins] skipped no-register: missing register(api) export") {
		t.Errorf("missing nil-Register warning:\n%s", log.String())
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Fine"}) {
		t.Errorf("Loaded = %v", got)
	}
	if !strings.Contains(log.String(), "active=1 disabled=0") {
		t.Errorf("nil-Register skip must not count as disabled:\n%s", log.String())
	}
}

func TestLoadNoValidPluginsWarning(t *testing.T) {
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Boom", Register: func(*Api) { panic("x") }},
	}, HubOptions{})
	hub.Load()
	if !strings.Contains(log.String(), "[plugins] no valid plugins loaded") {
		t.Errorf("missing no-valid-plugins warning:\n%s", log.String())
	}
}

func TestLoadNoValidPluginsWarningSkippedWhenAllDisabled(t *testing.T) {
	no := false
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Off", Enabled: &no, Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	// The plugin passed the pre-filter (plugin default, not a
	// switch) but was disabled in the second pass — TS warns here
	// too, because enabledPluginFiles was non-empty. This pins
	// that behavior rather than the all-switch-disabled case.
	if !strings.Contains(log.String(), "[plugins] no valid plugins loaded") {
		t.Errorf("missing no-valid-plugins warning:\n%s", log.String())
	}

	hub, log = newLoaderTestHub(t, []Plugin{
		{Name: "Off", Register: func(*Api) {}},
	}, HubOptions{Switches: map[string]bool{"off": false}})
	hub.Load()
	if strings.Contains(log.String(), "no valid plugins loaded") {
		t.Errorf("switch-disabled plugins must not trigger the warning:\n%s", log.String())
	}
}

func TestLoadLegacyDefaultLogsDeprecation(t *testing.T) {
	yes := true
	hub, log := newLoaderTestHub(t, []Plugin{{Name: "P", Register: func(*Api) {}}}, HubOptions{DefaultEnabled: &yes})
	hub.Load()
	if !strings.Contains(log.String(), "disabledPlugins is deprecated") {
		t.Errorf("missing deprecation warning:\n%s", log.String())
	}
	hub, log = newLoaderTestHub(t, []Plugin{{Name: "P", Register: func(*Api) {}}}, HubOptions{})
	hub.Load()
	if strings.Contains(log.String(), "disabledPlugins is deprecated") {
		t.Errorf("modern map must not warn:\n%s", log.String())
	}
}

func TestLoadedReturnsCopy(t *testing.T) {
	hub, _ := newLoaderTestHub(t, []Plugin{{Name: "A", Register: func(*Api) {}}}, HubOptions{})
	hub.Load()
	got := hub.Loaded()
	got[0] = "MUTATED"
	if fresh := hub.Loaded(); fresh[0] != "A" {
		t.Errorf("mutating Loaded() affected the hub: %v", fresh)
	}
}
