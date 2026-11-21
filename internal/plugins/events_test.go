package plugins

import (
	"strings"
	"testing"
)

func TestCustomEventNameValidationSubscribePath(t *testing.T) {
	invalid := []string{"", "no-colon", "a:b:c", "a: b", "a :b", ":b", "a:", "a:b ", " a:b", "a\t:b", "a:b\n"}
	h, buf := newTestHub()
	api := newApi(h, "p")
	called := 0
	for _, name := range invalid {
		api.OnCustomEvent(name, func(any) { called++ })
	}
	// Emitting any of the invalid names must reach nothing.
	for _, name := range invalid {
		h.EmitCustomEvent(name, nil)
	}
	if called != 0 {
		t.Fatalf("invalid subscriptions ran %d times, want 0", called)
	}
	if buf.Len() != 0 {
		t.Fatalf("invalid names logged %q, want silence", buf.String())
	}
}

func TestCustomEventNameValidationEmitPath(t *testing.T) {
	h, _ := newTestHub()
	called := 0
	newApi(h, "p").OnCustomEvent("x:y", func(any) { called++ })
	for _, name := range []string{"", "x", "x:y:z", "x: y", "x:y "} {
		h.EmitCustomEvent(name, nil)
	}
	if called != 0 {
		t.Fatalf("invalid emits ran the handler %d times, want 0", called)
	}
	h.EmitCustomEvent("x:y", 42)
	if called != 1 {
		t.Fatalf("valid emit ran the handler %d times, want 1", called)
	}
}

func TestCustomEventOrderingPayloadAndIsolation(t *testing.T) {
	h, buf := newTestHub()
	var got []any
	newApi(h, "first").OnCustomEvent("game:event", func(p any) { got = append(got, p) })
	newApi(h, "boom").OnCustomEvent("game:event", func(any) { panic("event-boom") })
	newApi(h, "third").OnCustomEvent("game:event", func(p any) { got = append(got, p) })
	h.EmitCustomEvent("game:event", "payload")
	if len(got) != 2 || got[0] != "payload" || got[1] != "payload" {
		t.Fatalf("payloads = %v, want two payload deliveries in order", got)
	}
	if want := "[plugins] custom_event:game:event hook failed (boom)"; !strings.Contains(buf.String(), want) {
		t.Fatalf("log = %q, want %q", buf.String(), want)
	}
	// A different event name does not reach these subscribers.
	h.EmitCustomEvent("game:other", "payload")
	if len(got) != 2 {
		t.Fatalf("other event reached subscribers: %v", got)
	}
}

func TestApiEmitCustomEvent(t *testing.T) {
	h, _ := newTestHub()
	seen := 0
	newApi(h, "sub").OnCustomEvent("a:b", func(any) { seen++ })
	newApi(h, "pub").EmitCustomEvent("a:b", nil)
	newApi(h, "pub").EmitCustomEvent("invalid", nil)
	if seen != 1 {
		t.Fatalf("Api.EmitCustomEvent delivered %d times, want 1", seen)
	}
}

func TestPluginsLoadedEventViaLoad(t *testing.T) {
	// Register through a real plugin so Load() drives the whole path:
	// registration passes, then the plugins:loaded emit, then the seal.
	hub := NewHub([]Plugin{{
		Name: "Alpha",
		Register: func(api *Api) {
			api.OnCustomEvent("plugins:loaded", func(payload any) {
				ev, ok := payload.(PluginsLoadedEvent)
				if !ok {
					t.Errorf("plugins:loaded payload = %T, want PluginsLoadedEvent", payload)
					return
				}
				if len(ev.Names) != 1 || ev.Names[0] != "Alpha" {
					t.Errorf("plugins:loaded Names = %v, want [Alpha]", ev.Names)
				}
			})
		},
	}}, HubOptions{})
	hub.Load()
	if got := hub.Loaded(); len(got) != 1 || got[0] != "Alpha" {
		t.Fatalf("Loaded() = %v, want [Alpha]", got)
	}
	if !hub.loaded() {
		t.Fatal("hub is not sealed after Load()")
	}
}

func TestEmitPluginsLoadedDirectly(t *testing.T) {
	h, _ := newTestHub()
	calls := 0
	var names []string
	newApi(h, "p").OnCustomEvent("plugins:loaded", func(payload any) {
		calls++
		names = payload.(PluginsLoadedEvent).Names
	})
	h.emitPluginsLoaded()
	if calls != 1 {
		t.Fatalf("plugins:loaded fired %d times, want 1", calls)
	}
	if len(names) != 0 {
		t.Fatalf("Names = %v, want empty for a hub with no plugins", names)
	}
	// Idempotent: a second call neither re-fires nor unseals.
	h.emitPluginsLoaded()
	if calls != 1 {
		t.Fatalf("plugins:loaded fired %d times after a second call, want 1", calls)
	}
	if !h.loaded() {
		t.Fatal("hub is not sealed after emitPluginsLoaded")
	}
}

func TestPluginsLoadedHandlerMayRegister(t *testing.T) {
	// The seal happens after the emit, so a plugins:loaded handler can
	// still register — matching TS, where registration during the load
	// passes is allowed.
	h, _ := newTestHub()
	ran := false
	newApi(h, "p").OnCustomEvent("plugins:loaded", func(any) {
		newApi(h, "p").OnPlayerLogin(func(*PlayerLoginEvent) { ran = true })
	})
	h.emitPluginsLoaded()
	h.EmitPlayerLogin(&PlayerLoginEvent{Player: testPlayer("alice"), Username: "alice"})
	if !ran {
		t.Fatal("hook registered from a plugins:loaded handler did not run")
	}
}

// TestSubscribeCustomEventEngineSubscription covers the coordinator's
// engine-internal subscription path (SubscribeCustomEvent): it
// delivers emits, runs in subscription order with content handlers,
// and ignores invalid names, nil handlers, and nil hubs without
// panicking.
func TestSubscribeCustomEventEngineSubscription(t *testing.T) {
	h, _ := newTestHub()
	var order []string
	newApi(h, "p").OnCustomEvent("x:y", func(any) { order = append(order, "content") })
	h.SubscribeCustomEvent("x:y", func(any) { order = append(order, "engine") })
	h.EmitCustomEvent("x:y", nil)
	if len(order) != 2 || order[0] != "content" || order[1] != "engine" {
		t.Fatalf("order = %v, want [content engine]", order)
	}
	// Invalid names and nil handlers are ignored silently.
	h.SubscribeCustomEvent("no-colon", func(any) { order = append(order, "bad") })
	h.SubscribeCustomEvent("x:y", nil)
	h.EmitCustomEvent("x:y", nil)
	if len(order) != 4 {
		t.Fatalf("order = %v, want exactly two more deliveries", order)
	}
	// A nil hub never panics.
	var nilHub *Hub
	nilHub.SubscribeCustomEvent("x:y", func(any) { t.Error("nil hub handler ran") })
}
