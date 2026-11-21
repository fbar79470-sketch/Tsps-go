package plugins

import "errors"

// Login extensions: a plugin can extend an authenticated login without
// owning credentials or socket cleanup (PluginLoginExtension,
// PluginTypes.ts:33-40). The mechanism here is the single-slot registration
// plus fail-closed creation, mirroring PluginManager.createLoginExtension
// (PluginManager.ts:794-798). There is NO production call site this phase:
// session login has no extension seam, so nothing calls
// CreateLoginExtension. Mechanism + tests only; the gap is documented on
// LoginExtension below.
//
// Single-slot semantics, grounded in createApi (PluginManager.ts:3196-3200):
// the slot is FIRST-WINS, not last-wins. A second registration — by any
// plugin, including the same plugin re-registering in its deferred pass —
// throws "A login extension is already registered", and a non-function
// factory throws "Login extension must be a function". In Go both are
// panics, which the loader's register recovery turns into a failed plugin
// initialization (mirroring registerPluginCandidate,
// PluginManager.ts:2570-2590). Registration after Load() completes is
// ignored with a warning instead of panicking: the registration window is
// closed, so there is nothing to contest.

// LoginConnection is the connection a login-extension factory receives. TS
// PluginLoginConnection (PluginTypes.ts:21-31) carries the plugin's Api
// plus socket/persistence operations (send, pending, name reservation,
// reject); only the Api can be honestly typed in this package — the rest
// belong to the session login path, which this package must not import and
// which has no extension seam yet. When that seam lands, the session-owned
// operations arrive here as an interface supplied by the adapter; until
// then, LoginConnection carries the Api alone, and CreateLoginExtension
// fills it with the registering plugin's Api, mirroring
// createLoginExtension's createApi(entry.pluginName)
// (PluginManager.ts:797).
type LoginConnection struct {
	api *Api
}

// Api returns the registering plugin's Api, or nil when the connection has
// none (only possible for a zero-value LoginConnection, which no production
// path constructs).
func (c *LoginConnection) Api() *Api {
	if c == nil {
		return nil
	}
	return c.api
}

// LoginExtension is the extension a factory returns. TS PluginLoginExtension
// (PluginTypes.ts:33-40) has four optional callbacks — authenticated,
// accepted, packet, entered — each taking session/persistence types
// (PluginLoginPending, ClientMessage) that cannot be honestly typed in this
// package today: the session login path has no extension seam, and this
// package must not import internal/session or internal/persist. This type is
// therefore an intentional placeholder carrying no callbacks: the factory
// mechanism (single slot, first-wins, fail-closed creation) is real and
// tested, but no callback can fire until the seam exists. When it does, the
// callbacks arrive here as optional func fields on this struct (the Go form
// of TS optional methods), typed against the seam's adapter types — not
// before. A factory returns nil to decline (TS undefined).
type LoginExtension struct{}

// LoginExtensionFactory builds a login extension for a connection, or
// returns nil to decline. It mirrors the factory signature in
// registerLoginExtension (PluginTypes.ts:1002): a function of the login
// connection returning a PluginLoginExtension or undefined.
type LoginExtensionFactory func(conn *LoginConnection) *LoginExtension

// loginExtensionEntry is the single slot's content: the registering
// plugin's name (for the Api handed to the factory) plus the factory.
type loginExtensionEntry struct {
	pluginName string
	factory    LoginExtensionFactory
}

// setLoginExtension stores the factory unless the slot is already occupied,
// reporting whether the slot was occupied. The check-and-store is atomic
// under the registry lock.
func (r *hookRegistry) setLoginExtension(pluginName string, factory LoginExtensionFactory) (occupied bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loginExt != nil {
		return true
	}
	r.loginExt = &loginExtensionEntry{pluginName: pluginName, factory: factory}
	return false
}

// loginExtensionSnapshot returns the slot's entry, or nil when no factory
// is registered. The factory is always invoked outside the registry lock
// (see CreateLoginExtension).
func (r *hookRegistry) loginExtensionSnapshot() *loginExtensionEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loginExt
}

// RegisterLoginExtension registers the hub's single login-extension factory,
// mirroring registerLoginExtension (PluginTypes.ts:1002;
// PluginManager.ts:3196-3200). The slot is first-wins: a nil factory panics
// with "Login extension must be a function", and a second registration
// panics with "A login extension is already registered" — both mirroring
// the TS throws, which the loader's register recovery converts into a
// failed plugin initialization. Registration after Load() completes is
// ignored with a warning (the window check runs first, so a post-load
// registration never panics). A nil Api or a Hub without a registry is a
// silent no-op.
func (a *Api) RegisterLoginExtension(factory LoginExtensionFactory) {
	if a == nil || a.hub == nil || a.hub.hooks == nil {
		return
	}
	if a.hub.loaded() {
		if logger := a.hub.Logger(); logger != nil {
			logger.Printf("[plugins] %s: registerLoginExtension ignored (hub already loaded)", a.name)
		}
		return
	}
	if factory == nil {
		panic(errors.New("Login extension must be a function"))
	}
	if a.hub.hooks.setLoginExtension(a.name, factory) {
		panic(errors.New("A login extension is already registered"))
	}
}

// CreateLoginExtension calls the registered factory with a connection
// carrying the registering plugin's Api, and returns the extension — or nil
// when no factory is registered. It mirrors createLoginExtension
// (PluginManager.ts:794-798), including the fail-closed rule: there is NO
// recover anywhere on this path, so a factory panic propagates to the
// caller (in TS the comment reads "Authentication extensions fail closed;
// ordinary hook error suppression is unsafe here"). A nil connection is
// tolerated (a fresh connection carrying only the Api is passed); a nil Hub
// or a Hub without a registry returns nil. There is no production call site
// this phase: session login has no extension seam.
func (h *Hub) CreateLoginExtension(conn *LoginConnection) *LoginExtension {
	if h == nil || h.hooks == nil {
		return nil
	}
	entry := h.hooks.loginExtensionSnapshot()
	if entry == nil {
		return nil
	}
	c := LoginConnection{}
	if conn != nil {
		c = *conn
	}
	c.api = newApi(h, entry.pluginName)
	return entry.factory(&c)
}
