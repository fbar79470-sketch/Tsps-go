package plugins

import (
	"log"
	"testing"
)

func TestCoreLogger(t *testing.T) {
	h, buf := newTestHub()
	_ = buf
	core := newApi(h, "p").Core()
	if core.Logger() == nil {
		t.Fatal("Core.Logger() is nil")
	}
	// The hub logger is the injected one: writing through Core lands in
	// the captured buffer.
	core.Logger().Print("via-core")
	if buf.Len() == 0 {
		t.Fatal("Core.Logger() did not write to the hub's injected logger")
	}
	var nilCore *Core
	if nilCore.Logger() == nil {
		t.Fatal("nil Core Logger() is nil, want log.Default() fallback")
	}
	if nilCore.Logger() != log.Default() {
		t.Fatal("nil Core Logger() is not log.Default()")
	}
	if nilCore.Hub() != nil {
		t.Fatal("nil Core Hub() is non-nil")
	}
	if nilCore.Cycle() != 0 {
		t.Fatal("nil Core Cycle() is non-zero")
	}
}

func TestCoreConfigGet(t *testing.T) {
	h, _ := newTestHubWithConfig(map[string]any{"ns:key": 42})
	cfg := newApi(h, "p").Core().Config()
	if got := cfg.Get("ns:key", 0); got != 42 {
		t.Fatalf("Config.Get full key = %v, want 42", got)
	}
	if got := cfg.Get("ns:missing", 7); got != 7 {
		t.Fatalf("Config.Get missing key = %v, want default 7", got)
	}
	if got := cfg.Get("", 7); got != 7 {
		t.Fatalf("Config.Get empty key = %v, want default 7", got)
	}
	// No per-plugin prefixing on the shared Core: a bare key reads the
	// bare key, which is unset here.
	if got := cfg.Get("key", 7); got != 7 {
		t.Fatalf("Config.Get bare key = %v, want default 7 (no prefixing)", got)
	}
	if got := (CoreConfig{}).Get("ns:key", 7); got != 7 {
		t.Fatalf("zero CoreConfig Get = %v, want default 7", got)
	}
}

func TestCoreCycleSeam(t *testing.T) {
	h, _ := newTestHub()
	core := newApi(h, "p").Core()
	if got := core.Cycle(); got != 0 {
		t.Fatalf("Cycle() with no source = %d, want 0", got)
	}
	h.SetCycleSource(func() int64 { return 77 })
	if got := core.Cycle(); got != 77 {
		t.Fatalf("Cycle() after SetCycleSource = %d, want 77", got)
	}
	// The seam is shared: a second Api's Core sees the same source.
	if got := newApi(h, "q").Core().Cycle(); got != 77 {
		t.Fatalf("second Api's Cycle() = %d, want 77", got)
	}
	h.SetCycleSource(nil)
	if got := core.Cycle(); got != 0 {
		t.Fatalf("Cycle() after clearing the source = %d, want 0", got)
	}
	// Nil-hub and bare-hub calls are no-ops, not panics.
	var nilHub *Hub
	nilHub.SetCycleSource(func() int64 { return 1 })
	(&Hub{}).SetCycleSource(func() int64 { return 1 })
}
