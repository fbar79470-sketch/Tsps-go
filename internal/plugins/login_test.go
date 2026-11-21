package plugins

import (
	"strings"
	"testing"
)

func TestCreateLoginExtensionNoFactory(t *testing.T) {
	h, _ := newTestHub()
	if got := h.CreateLoginExtension(nil); got != nil {
		t.Fatalf("CreateLoginExtension with no factory = %v, want nil", got)
	}
	if got := h.CreateLoginExtension(&LoginConnection{}); got != nil {
		t.Fatalf("CreateLoginExtension with no factory = %v, want nil", got)
	}
	var nilHub *Hub
	if nilHub.CreateLoginExtension(nil) != nil {
		t.Fatal("nil hub CreateLoginExtension is non-nil")
	}
}

func TestRegisterLoginExtensionFirstWins(t *testing.T) {
	h, _ := newTestHub()
	firstCalled := false
	newApi(h, "First").RegisterLoginExtension(func(*LoginConnection) *LoginExtension {
		firstCalled = true
		return &LoginExtension{}
	})
	// A second registration, by another plugin, panics with the TS message.
	assertPanicsWith(t, func() {
		newApi(h, "Second").RegisterLoginExtension(func(*LoginConnection) *LoginExtension {
			return &LoginExtension{}
		})
	}, "A login extension is already registered")
	// The same plugin re-registering also panics (the slot is global, not
	// per-plugin, as in TS).
	assertPanicsWith(t, func() {
		newApi(h, "First").RegisterLoginExtension(func(*LoginConnection) *LoginExtension {
			return &LoginExtension{}
		})
	}, "A login extension is already registered")
	// The first factory still owns the slot.
	if got := h.CreateLoginExtension(nil); got == nil || !firstCalled {
		t.Fatalf("CreateLoginExtension = %v (firstCalled=%v), want the first factory's extension", got, firstCalled)
	}
}

func TestRegisterLoginExtensionNilFactoryPanics(t *testing.T) {
	h, _ := newTestHub()
	assertPanicsWith(t, func() {
		newApi(h, "p").RegisterLoginExtension(nil)
	}, "Login extension must be a function")
}

func TestCreateLoginExtensionFailClosed(t *testing.T) {
	h, _ := newTestHub()
	sentinel := "extension-boom"
	newApi(h, "p").RegisterLoginExtension(func(*LoginConnection) *LoginExtension {
		panic(sentinel)
	})
	// No recover: the factory panic propagates out of CreateLoginExtension.
	assertPanicsWith(t, func() {
		h.CreateLoginExtension(nil)
	}, sentinel)
}

func TestCreateLoginExtensionFactoryReturnsNil(t *testing.T) {
	h, _ := newTestHub()
	newApi(h, "p").RegisterLoginExtension(func(*LoginConnection) *LoginExtension { return nil })
	if got := h.CreateLoginExtension(nil); got != nil {
		t.Fatalf("declining factory yielded %v, want nil", got)
	}
}

func TestCreateLoginExtensionConnectionApi(t *testing.T) {
	h, _ := newTestHub()
	var seen *LoginConnection
	newApi(h, "Owner").RegisterLoginExtension(func(conn *LoginConnection) *LoginExtension {
		seen = conn
		return &LoginExtension{}
	})
	h.CreateLoginExtension(&LoginConnection{})
	if seen == nil {
		t.Fatal("factory received a nil connection")
	}
	if seen.Api() == nil || seen.Api().Name() != "Owner" {
		t.Fatalf("connection Api name = %q, want the registering plugin Owner", seen.Api().Name())
	}
	// A nil connection is tolerated: the factory still gets an Api.
	seen = nil
	h.CreateLoginExtension(nil)
	if seen == nil || seen.Api() == nil || seen.Api().Name() != "Owner" {
		t.Fatal("nil-connection creation did not supply the registering plugin's Api")
	}
}

func TestRegisterLoginExtensionPostLoadIgnored(t *testing.T) {
	h, buf := newTestHub()
	h.Load()
	// Post-load registration is ignored with a warning — it must NOT panic,
	// even though the slot is empty and the factory is valid.
	newApi(h, "Late").RegisterLoginExtension(func(*LoginConnection) *LoginExtension {
		return &LoginExtension{}
	})
	if !strings.Contains(buf.String(), "already loaded") {
		t.Fatalf("log = %q, want a post-load warning", buf.String())
	}
	if got := h.CreateLoginExtension(nil); got != nil {
		t.Fatalf("post-load registration took the slot: %v", got)
	}
}
