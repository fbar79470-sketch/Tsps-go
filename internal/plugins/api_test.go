package plugins

import (
	"bytes"
	"fmt"
	"log"
	"slices"
	"strings"
	"testing"

	"tsps-go/internal/persist"
)

// newTestHub builds a Hub with no plugins and a captured logger.
func newTestHub() (*Hub, *bytes.Buffer) {
	var buf bytes.Buffer
	return NewHub(nil, HubOptions{Logger: log.New(&buf, "", 0)}), &buf
}

// newTestHubWithConfig builds a Hub whose plugin config map holds cfg.
func newTestHubWithConfig(cfg map[string]any) (*Hub, *bytes.Buffer) {
	var buf bytes.Buffer
	return NewHub(nil, HubOptions{Logger: log.New(&buf, "", 0), PluginConfig: cfg}), &buf
}

// assertPanicsWith fails unless fn panics with a value whose formatted form
// contains want.
func assertPanicsWith(t *testing.T, fn func(), want string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q, got no panic", want)
		}
		if got := fmt.Sprint(r); !strings.Contains(got, want) {
			t.Fatalf("panic = %q, want it to contain %q", got, want)
		}
	}()
	fn()
}

func TestNewApiBasics(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Alpha")
	if got := a.Name(); got != "Alpha" {
		t.Fatalf("Name() = %q, want %q", got, "Alpha")
	}
	if a.Core() == nil {
		t.Fatal("Core() is nil, want the hub's shared Core")
	}
	if got := a.Core().Hub(); got != h {
		t.Fatalf("Core().Hub() = %p, want hub %p", got, h)
	}
	// One shared instance per hub: a second Api sees the same Core.
	b := newApi(h, "Beta")
	if a.Core() != b.Core() {
		t.Fatal("two Apis on one hub have different Core instances, want one shared instance")
	}
}

func TestNilApiSafety(t *testing.T) {
	var a *Api
	if got := a.Name(); got != "" {
		t.Fatalf("nil Api Name() = %q, want empty", got)
	}
	if a.Core() != nil {
		t.Fatal("nil Api Core() is non-nil, want nil")
	}
	if got := a.GetPluginConfig("k", 7); got != 7 {
		t.Fatalf("nil Api GetPluginConfig = %v, want default 7", got)
	}
	a.EmitCustomEvent("x:y", nil) // must not panic
	a.OnPlayerLogin(nil)
	a.RegisterLoginExtension(nil)
	if got := newApi(nil, "NoHub").GetPluginConfig("k", 9); got != 9 {
		t.Fatalf("nil-hub Api GetPluginConfig = %v, want default 9", got)
	}
	if newApi(nil, "NoHub").Core() != nil {
		t.Fatal("nil-hub Api Core() is non-nil, want nil")
	}
}

func TestApiGetPluginConfig(t *testing.T) {
	h, _ := newTestHubWithConfig(map[string]any{
		"MyPlugin:allowSkip": true,
		"league:rate":        2,
		"Other:key":          "v",
	})
	api := newApi(h, "MyPlugin")
	if got := api.GetPluginConfig("allowSkip", false); got != true {
		t.Fatalf("bare key = %v, want true (prefixed with owning plugin)", got)
	}
	if got := api.GetPluginConfig("missing", "d"); got != "d" {
		t.Fatalf("missing key = %v, want default", got)
	}
	if got := api.GetPluginConfig("Other:key", "d"); got != "v" {
		t.Fatalf("full key = %v, want v (used as written)", got)
	}
	if got := api.GetPluginConfig("", "d"); got != "d" {
		t.Fatalf("empty key = %v, want default", got)
	}
	// Legacy namespace mapping: LeagueMode reads the "league:" prefix.
	league := newApi(h, "LeagueMode")
	if got := league.GetPluginConfig("rate", 0); got != 2 {
		t.Fatalf("legacy key = %v, want 2 (leaguemode -> league)", got)
	}
}

func TestNilHandlersIgnoredSilently(t *testing.T) {
	h, buf := newTestHub()
	api := newApi(h, "Alpha")
	api.OnServerStartup(nil)
	api.OnServerShutdown(nil)
	api.OnPlayerLogin(nil)
	api.OnPlayerLogout(nil)
	api.OnPlayerDisconnect(nil)
	api.OnPlayerProcess(nil)
	api.OnCustomEvent("x:y", nil)
	api.OnCustomEvent("bad-name", func(any) {})
	if buf.Len() != 0 {
		t.Fatalf("nil/invalid registrations logged %q, want silence", buf.String())
	}
	// Nothing was stored: emits run no hooks and log nothing.
	h.EmitServerStartup(&ServerLifecycleEvent{Timestamp: 1})
	h.EmitCustomEvent("x:y", nil)
	if buf.Len() != 0 {
		t.Fatalf("emits after nil registrations logged %q, want silence", buf.String())
	}
}

func TestPostLoadRegistrationIgnored(t *testing.T) {
	h, buf := newTestHub()
	h.Load()
	if !h.loaded() {
		t.Fatal("hub is not loaded after Load(); Load() must call emitPluginsLoaded (plan contract)")
	}
	api := newApi(h, "Late")
	called := false
	api.OnPlayerLogin(func(*PlayerLoginEvent) { called = true })
	api.OnCustomEvent("x:y", func(any) { called = true })
	if !strings.Contains(buf.String(), "already loaded") {
		t.Fatalf("log = %q, want a post-load warning", buf.String())
	}
	h.EmitPlayerLogin(&PlayerLoginEvent{Player: testPlayer("alice"), Username: "alice"})
	h.EmitCustomEvent("x:y", nil)
	if called {
		t.Fatal("post-load registration ran, want it ignored")
	}
}

// assertPanics fails unless fn panics, with any value. It is used where
// the panic's message comes from another package (the persist registry)
// and this package must not pin its wording; use assertPanicsWith when
// the message is this package's own.
func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a panic, got none")
		}
	}()
	fn()
}

// persistentKeyRegistered reports whether key is in the live persistent
// registry. PersistentAttributeKeys returns a sorted slice, so membership
// is a linear scan; the registry is small and this is test-only.
func persistentKeyRegistered(key string) bool {
	return slices.Contains(persist.PersistentAttributeKeys(), key)
}

// PersistAttribute tests share the process-global persist registry, so
// each resets it first and uses a uniquely named key: a key registered
// by one test must never satisfy another test's assertion.
func TestApiPersistAttributeRegistersKeyWithoutDefault(t *testing.T) {
	persist.ResetAttributeRegistryForTest()
	h, _ := newTestHub()
	a := newApi(h, "Alpha")
	const key = "phase11-test:persist-no-default"
	a.PersistAttribute(key)
	if !persistentKeyRegistered(key) {
		t.Fatalf("key %q is not registered after PersistAttribute", key)
	}
	if def, ok := persist.AttributeDefaultJSON(key); ok {
		t.Fatalf("AttributeDefaultJSON(%q) = (%q, true), want no default", key, def)
	}
}

func TestApiPersistAttributeRegistersKeyWithDefault(t *testing.T) {
	persist.ResetAttributeRegistryForTest()
	h, _ := newTestHub()
	a := newApi(h, "Alpha")
	const intKey = "phase11-test:persist-int-default"
	const strKey = "phase11-test:persist-string-default"
	a.PersistAttribute(intKey, 42)
	a.PersistAttribute(strKey, "hello")
	for _, key := range []string{intKey, strKey} {
		if !persistentKeyRegistered(key) {
			t.Fatalf("key %q is not registered after PersistAttribute with default", key)
		}
	}
	if got, ok := persist.AttributeDefaultJSON(intKey); !ok || got != "42" {
		t.Fatalf("AttributeDefaultJSON(%q) = (%q, %v), want (\"42\", true)", intKey, got, ok)
	}
	if got, ok := persist.AttributeDefaultJSON(strKey); !ok || got != `"hello"` {
		t.Fatalf("AttributeDefaultJSON(%q) = (%q, %v), want (`\"hello\"`, true)", strKey, got, ok)
	}
}

func TestApiPersistAttributeInvalidKeysPanic(t *testing.T) {
	persist.ResetAttributeRegistryForTest()
	h, _ := newTestHub()
	a := newApi(h, "Alpha")
	for _, key := range []string{"", " ", " padded", "padded ", "  padded  "} {
		assertPanics(t, func() { a.PersistAttribute(key) })
		if persistentKeyRegistered(key) {
			t.Fatalf("invalid key %q was registered despite the panic", key)
		}
	}
}

func TestApiPersistAttributeTooManyDefaultsPanics(t *testing.T) {
	persist.ResetAttributeRegistryForTest()
	h, _ := newTestHub()
	a := newApi(h, "Alpha")
	const key = "phase11-test:persist-too-many-defaults"
	assertPanicsWith(t, func() { a.PersistAttribute(key, 1, 2) }, "at most one default value")
	if persistentKeyRegistered(key) {
		t.Fatalf("key %q was registered despite the arity panic", key)
	}
}

func TestApiPersistAttributeAfterLoadStillRegisters(t *testing.T) {
	persist.ResetAttributeRegistryForTest()
	h, _ := newTestHub()
	h.Load() // seals the hub: hook registration is refused from here on.
	a := newApi(h, "Late")
	const key = "phase11-test:persist-after-load"
	a.PersistAttribute(key, 0)
	if !persistentKeyRegistered(key) {
		t.Fatalf("key %q is not registered: PersistAttribute must have no load-phase gate", key)
	}
	if got, ok := persist.AttributeDefaultJSON(key); !ok || got != "0" {
		t.Fatalf("AttributeDefaultJSON(%q) = (%q, %v), want (\"0\", true)", key, got, ok)
	}
}

func TestApiPersistAttributeNilSafety(t *testing.T) {
	persist.ResetAttributeRegistryForTest()
	var a *Api
	a.PersistAttribute("phase11-test:persist-nil-api")                       // must not panic
	a.PersistAttribute("phase11-test:persist-nil-api", 1, 2)                 // arity panic suppressed: nil wins
	newApi(nil, "NoHub").PersistAttribute("phase11-test:persist-nil-hub", 1) // must not panic
	for _, key := range []string{"phase11-test:persist-nil-api", "phase11-test:persist-nil-hub"} {
		if persistentKeyRegistered(key) {
			t.Fatalf("key %q was registered by a nil/no-hub Api call", key)
		}
	}
}

func TestPersistAttributeBadKeyDropsPluginOnLoad(t *testing.T) {
	persist.ResetAttributeRegistryForTest()
	var buf bytes.Buffer
	h := NewHub([]Plugin{
		{
			Name: "BadPersist",
			Register: func(api *Api) {
				api.PersistAttribute("  padded  ")
			},
		},
		{
			Name: "GoodPersist",
			Register: func(api *Api) {
				api.PersistAttribute("phase11-test:persist-good-plugin", 7)
			},
		},
	}, HubOptions{Logger: log.New(&buf, "", 0)})
	h.Load()
	if got := h.Loaded(); len(got) != 1 || got[0] != "GoodPersist" {
		t.Fatalf("Loaded() = %v, want [GoodPersist]: the bad-key plugin must be dropped, the good one kept", got)
	}
	if !strings.Contains(buf.String(), "[plugins] failed to initialize BadPersist") {
		t.Fatalf("log %q does not contain the failed-to-initialize line for BadPersist", buf.String())
	}
	if !persistentKeyRegistered("phase11-test:persist-good-plugin") {
		t.Fatal("the surviving plugin's key was not registered")
	}
}
