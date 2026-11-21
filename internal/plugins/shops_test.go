package plugins

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// Shop-family tests: the registration methods, emits, and gate
// contracts in shops.go, grounded in PluginManager.ts (emitCanShop
// 1534-1546, emitCanStockItem 1682-1685 via firstAnswer 1639-1652,
// registration guards 3767-3779 and 3838-3840), PluginTypes.ts
// (event shapes 364-368, 471-475; currency members 1338-1346),
// and ShopManager.ts (registerCurrency 105-110, registerItemCurrency
// 112-138). Fixtures only: newTestHub and testPlayer come from
// api_test.go / hooks_test.go.

func TestOnCanShopFirstAnswer(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Gate")
	calls := 0
	a.OnCanShop(func(ev *CanShopEvent) {
		calls++
		// Silence: no answer.
	})
	a.OnCanShop(func(ev *CanShopEvent) {
		calls++
		ev.Allow = BoolPtr(false)
	})
	a.OnCanShop(func(ev *CanShopEvent) {
		calls++
		ev.Allow = BoolPtr(true)
	})
	p := testPlayer("alice")
	got := h.EmitCanShop(p, 7, true)
	if got == nil || *got != false {
		t.Fatalf("EmitCanShop = %v, want false (first answer wins)", got)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (third hook never runs)", calls)
	}
	// No answer at all yields nil.
	h2, _ := newTestHub()
	newApi(h2, "Silent").OnCanShop(func(*CanShopEvent) {})
	if got := h2.EmitCanShop(p, 7, true); got != nil {
		t.Fatalf("silent EmitCanShop = %v, want nil", got)
	}
	// Zero hooks yields nil.
	h3, _ := newTestHub()
	if got := h3.EmitCanShop(p, 7, true); got != nil {
		t.Fatalf("zero-hook EmitCanShop = %v, want nil", got)
	}
}

func TestOnCanShopWrapperAndShopID(t *testing.T) {
	h, _ := newTestHub()
	seen := -1
	seenHas := false
	newApi(h, "Watcher").OnCanShop(func(ev *CanShopEvent) {
		seen, seenHas = ev.ShopID, ev.HasShopID
	})
	p := testPlayer("alice")
	h.EmitCanShop(p, 42, true)
	if seen != 42 || !seenHas {
		t.Fatalf("shop = %d/%v, want 42/true", seen, seenHas)
	}
	// The null-shopId form reaches the handler: TS passes shopId
	// null through, and the wrapper checks the player only.
	h.EmitCanShop(p, 0, false)
	if seen != 0 || seenHas {
		t.Fatalf("null shop = %d/%v, want 0/false", seen, seenHas)
	}
	// A nil player runs no handler (the registration wrapper),
	// and the emit yields nil.
	seen = -1
	if got := h.EmitCanShop(nil, 42, true); got != nil {
		t.Fatalf("nil-player EmitCanShop = %v, want nil", got)
	}
	if seen != -1 {
		t.Fatalf("nil-player emit ran the handler (shop %d)", seen)
	}
	// Nil hub / nil handler: no panic, no registration.
	var nilHub *Hub
	if got := nilHub.EmitCanShop(p, 1, true); got != nil {
		t.Fatalf("nil-hub EmitCanShop = %v, want nil", got)
	}
	newApi(h, "Nil").OnCanShop(nil)
}

func TestOnCanStockItemFirstAnswerNoWrapper(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Stock")
	calls := 0
	a.OnCanStockItem(func(ev *CanStockItemEvent) {
		calls++
		if ev.ShopID != 3 || ev.ItemID != 995 {
			t.Errorf("event = shop %d item %d, want 3/995", ev.ShopID, ev.ItemID)
		}
		// Silence.
	})
	a.OnCanStockItem(func(ev *CanStockItemEvent) {
		calls++
		ev.Allow = BoolPtr(true)
	})
	a.OnCanStockItem(func(ev *CanStockItemEvent) {
		calls++
		ev.Allow = BoolPtr(false)
	})
	got := h.EmitCanStockItem(3, 995)
	if got == nil || *got != true {
		t.Fatalf("EmitCanStockItem = %v, want true", got)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (third hook never runs)", calls)
	}
	// Silence and zero hooks yield nil (the entry is kept).
	h2, _ := newTestHub()
	newApi(h2, "Silent").OnCanStockItem(func(*CanStockItemEvent) {})
	if got := h2.EmitCanStockItem(3, 995); got != nil {
		t.Fatalf("silent EmitCanStockItem = %v, want nil", got)
	}
	var nilHub *Hub
	if got := nilHub.EmitCanStockItem(3, 995); got != nil {
		t.Fatalf("nil-hub EmitCanStockItem = %v, want nil", got)
	}
	newApi(h, "Nil").OnCanStockItem(nil)
}

func TestShopRegistrationAfterLoadIgnored(t *testing.T) {
	h, buf := newTestHub()
	h.Load()
	newApi(h, "Late").OnCanShop(func(*CanShopEvent) {})
	newApi(h, "Late").OnCanStockItem(func(*CanStockItemEvent) {})
	if !strings.Contains(buf.String(), "onCanShop") || !strings.Contains(buf.String(), "onCanStockItem") {
		t.Fatalf("log = %q, want post-load refusals for both shop families", buf.String())
	}
	if got := h.EmitCanShop(testPlayer("alice"), 1, true); got != nil {
		t.Fatalf("post-load EmitCanShop = %v, want nil", got)
	}
}

// fakeShopCurrencyRegistry records currency registrations in order.
type fakeShopCurrencyRegistry struct {
	named []string
	items []int
	opts  []ShopItemCurrencyOptions
}

func (f *fakeShopCurrencyRegistry) RegisterCurrency(name string, handler ShopCurrencyHandler) {
	f.named = append(f.named, name)
}

func (f *fakeShopCurrencyRegistry) RegisterItemCurrency(itemID int, opts ShopItemCurrencyOptions) {
	f.items = append(f.items, itemID)
	f.opts = append(f.opts, opts)
}

func coinHandler() ShopCurrencyHandler {
	return ShopCurrencyHandler{
		Name:   "Coins",
		Amount: func(*game.Player) int { return 0 },
		Add:    func(*game.Player, int) {},
		Remove: func(*game.Player, int) {},
	}
}

func TestRegisterShopCurrencyQueuesUntilInstalled(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Currency")
	// Registered before installation: queued, applied in order
	// at install.
	a.RegisterShopCurrency("HONOUR POINTS", coinHandler())
	a.RegisterItemShopCurrency(995, ShopItemCurrencyOptions{})
	a.RegisterShopCurrency("TOKENS", coinHandler())
	reg := &fakeShopCurrencyRegistry{}
	h.SetShopCurrencyRegistry(reg)
	if len(reg.named) != 2 || reg.named[0] != "HONOUR POINTS" || reg.named[1] != "TOKENS" {
		t.Fatalf("named = %v, want [HONOUR POINTS TOKENS] in order", reg.named)
	}
	if len(reg.items) != 1 || reg.items[0] != 995 {
		t.Fatalf("items = %v, want [995]", reg.items)
	}
	// Registered after installation: direct.
	a.RegisterShopCurrency("LATE", coinHandler())
	if len(reg.named) != 3 || reg.named[2] != "LATE" {
		t.Fatalf("named after direct = %v, want LATE appended", reg.named)
	}
	if h.ShopCurrencyRegistry() != ShopCurrencyRegistry(reg) {
		t.Fatal("ShopCurrencyRegistry getter must return the installed registry")
	}
}

func TestRegisterShopCurrencyNilInstallKeepsQueue(t *testing.T) {
	h, _ := newTestHub()
	newApi(h, "Currency").RegisterShopCurrency("QUEUED", coinHandler())
	h.SetShopCurrencyRegistry(nil)
	if h.ShopCurrencyRegistry() != nil {
		t.Fatal("nil install must clear the registry")
	}
	reg := &fakeShopCurrencyRegistry{}
	h.SetShopCurrencyRegistry(reg)
	if len(reg.named) != 1 || reg.named[0] != "QUEUED" {
		t.Fatalf("named = %v, want [QUEUED] flushed by the later install", reg.named)
	}
}

func TestRegisterShopCurrencyInvalidPanics(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "BadCurrency")
	// Empty name: the TS throw message.
	assertPanicsWith(t, func() { a.RegisterShopCurrency("", coinHandler()) },
		"Invalid shop currency registration: ")
	// A nil Amount/Add/Remove: the TS "not a function" throw.
	bad := coinHandler()
	bad.Amount = nil
	assertPanicsWith(t, func() { a.RegisterShopCurrency("X", bad) },
		"Invalid shop currency registration: X")
	bad = coinHandler()
	bad.Add = nil
	assertPanicsWith(t, func() { a.RegisterShopCurrency("X", bad) },
		"Invalid shop currency registration: X")
	bad = coinHandler()
	bad.Remove = nil
	assertPanicsWith(t, func() { a.RegisterShopCurrency("X", bad) },
		"Invalid shop currency registration: X")
	// Invalid item ids: the TS throw message.
	assertPanicsWith(t, func() { a.RegisterItemShopCurrency(0, ShopItemCurrencyOptions{}) },
		"Invalid item shop currency: 0")
	assertPanicsWith(t, func() { a.RegisterItemShopCurrency(-5, ShopItemCurrencyOptions{}) },
		"Invalid item shop currency: -5")
	// A failed registration queues nothing.
	reg := &fakeShopCurrencyRegistry{}
	h.SetShopCurrencyRegistry(reg)
	if len(reg.named) != 0 || len(reg.items) != 0 {
		t.Fatalf("registry = named %v items %v, want empty", reg.named, reg.items)
	}
}

func TestRegisterShopCurrencyAfterLoadStillQueues(t *testing.T) {
	// Currency members have no load-phase gate (the TS closures
	// have no phase check): registration after Load still
	// queues, like RegisterArea.
	h, _ := newTestHub()
	h.Load()
	newApi(h, "LateCurrency").RegisterShopCurrency("LATE", coinHandler())
	reg := &fakeShopCurrencyRegistry{}
	h.SetShopCurrencyRegistry(reg)
	if len(reg.named) != 1 || reg.named[0] != "LATE" {
		t.Fatalf("named = %v, want [LATE]", reg.named)
	}
}

func TestRegisterShopCurrencyNilApiNoOp(t *testing.T) {
	var nilApi *Api
	nilApi.RegisterShopCurrency("X", coinHandler())
	nilApi.RegisterItemShopCurrency(995, ShopItemCurrencyOptions{})
	var nilHub *Hub
	nilHub.SetShopCurrencyRegistry(&fakeShopCurrencyRegistry{})
	if nilHub.ShopCurrencyRegistry() != nil {
		t.Fatal("nil-hub registry getter must be nil")
	}
}

// fakeShopDefinition is a hand-built ShopDefinition for core-surface tests.
type fakeShopDefinition struct {
	id       int
	name     string
	currency string
	source   string
	stock    []ShopStockEntry
	policy   ShopSalesPolicy
}

func (d *fakeShopDefinition) ID() int                         { return d.id }
func (d *fakeShopDefinition) Name() string                    { return d.name }
func (d *fakeShopDefinition) Currency() string                { return d.currency }
func (d *fakeShopDefinition) OriginalStock() []ShopStockEntry { return d.stock }
func (d *fakeShopDefinition) DefaultRestockTicks() int        { return 4 }
func (d *fakeShopDefinition) DefaultDestockTicks() int        { return 4 }
func (d *fakeShopDefinition) SoldItemDestockTicks() int       { return 100 }
func (d *fakeShopDefinition) Source() string                  { return d.source }
func (d *fakeShopDefinition) SalesPolicy() ShopSalesPolicy    { return d.policy }

type fakeShopDefinitionRegistry struct {
	defs []*fakeShopDefinition
}

func (r *fakeShopDefinitionRegistry) ForID(id int) (ShopDefinition, bool) {
	for _, d := range r.defs {
		if d.id == id {
			return d, true
		}
	}
	return nil, false
}

func (r *fakeShopDefinitionRegistry) All() []ShopDefinition {
	out := make([]ShopDefinition, len(r.defs))
	for i, d := range r.defs {
		out[i] = d
	}
	return out
}

// fakeShopManagerAPI records ShopManagerAPI calls.
type fakeShopManagerAPI struct {
	opened    []int
	closed    int
	reloads   int
	shared    int
	openView  *OpenShopView
	openValue bool
}

func (f *fakeShopManagerAPI) Open(player *game.Player, shopID int, resetScroll bool) bool {
	f.opened = append(f.opened, shopID)
	return f.openValue
}
func (f *fakeShopManagerAPI) Close(player *game.Player) { f.closed++ }
func (f *fakeShopManagerAPI) HasShop(shopID int) bool   { return shopID == 7 }
func (f *fakeShopManagerAPI) GetOpenShop(player *game.Player) *OpenShopView {
	return f.openView
}
func (f *fakeShopManagerAPI) Reload() int { f.reloads++; return 3 }
func (f *fakeShopManagerAPI) AddShopStock(shopID, itemID, amount int) bool {
	return shopID == 7
}
func (f *fakeShopManagerAPI) RemoveShop(shopID int) {}
func (f *fakeShopManagerAPI) ShareStock(source ShopStockSource) {
	f.shared++
	_ = source
}

func TestCoreShopSurfaceUninstalled(t *testing.T) {
	h, _ := newTestHub()
	core := newApi(h, "Shopper").Core()
	if core == nil {
		t.Fatal("Core() must not be nil")
	}
	if core.ShopManager() != nil {
		t.Fatal("uninstalled ShopManager must be nil")
	}
	if core.ShopDefinition() != nil {
		t.Fatal("uninstalled ShopDefinition must be nil")
	}
	var nilCore *Core
	if nilCore.ShopManager() != nil || nilCore.ShopDefinition() != nil {
		t.Fatal("nil-Core shop accessors must be nil")
	}
}

func TestCoreShopSurfaceInstalled(t *testing.T) {
	h, _ := newTestHub()
	mgr := &fakeShopManagerAPI{openValue: true}
	reg := &fakeShopDefinitionRegistry{defs: []*fakeShopDefinition{
		{id: 7, name: "Bob's Shop", currency: "COINS", source: "core"},
	}}
	h.SetShopManager(mgr)
	h.SetShopDefinitionRegistry(reg)
	core := newApi(h, "Shopper").Core()
	if core.ShopManager() != ShopManagerAPI(mgr) {
		t.Fatal("Core.ShopManager must return the installed surface")
	}
	if core.ShopDefinition() != ShopDefinitionRegistry(reg) {
		t.Fatal("Core.ShopDefinition must return the installed registry")
	}
	// The surface forwards: open records, lookup resolves.
	p := testPlayer("alice")
	if !core.ShopManager().Open(p, 7, true) {
		t.Fatal("Open must forward the fake's true")
	}
	if len(mgr.opened) != 1 || mgr.opened[0] != 7 {
		t.Fatalf("opened = %v, want [7]", mgr.opened)
	}
	def, ok := core.ShopDefinition().ForID(7)
	if !ok || def.Name() != "Bob's Shop" || def.Source() != "core" {
		t.Fatalf("ForID(7) = %+v/%v, want Bob's Shop/core", def, ok)
	}
	if _, ok := core.ShopDefinition().ForID(99); ok {
		t.Fatal("ForID(99) must miss")
	}
	if len(core.ShopDefinition().All()) != 1 {
		t.Fatal("All must return the one definition")
	}
	// Clearing uninstalls.
	h.SetShopManager(nil)
	h.SetShopDefinitionRegistry(nil)
	if core.ShopManager() != nil || core.ShopDefinition() != nil {
		t.Fatal("nil installs must clear the surfaces")
	}
}

func TestShopEventNameConstants(t *testing.T) {
	// The exact TS strings (ShopManager.ts emit sites); content
	// subscribes through Api.OnCustomEvent with these names.
	want := map[string]string{
		"opened":        ShopOpenedEventName,
		"stock-visible": ShopStockVisibleEventName,
		"buy-limit":     ShopBuyLimitEventName,
		"delivery":      ShopDeliveryEventName,
		"purchase":      ShopPurchaseEventName,
		"keep-stock":    ShopKeepStockEventName,
		"sell-price":    ShopSellPriceEventName,
		"buy-price":     ShopBuyPriceEventName,
		"buys-back":     ShopBuysBackEventName,
		"can-sell":      ShopCanSellEventName,
	}
	for suffix, got := range want {
		if got != "shop:"+suffix {
			t.Fatalf("event name = %q, want %q", got, "shop:"+suffix)
		}
	}
}

func TestBoolPtr(t *testing.T) {
	if p := BoolPtr(true); p == nil || *p != true {
		t.Fatalf("BoolPtr(true) = %v, want true", p)
	}
	if p := BoolPtr(false); p == nil || *p != false {
		t.Fatalf("BoolPtr(false) = %v, want false", p)
	}
	// Fresh pointers: mutating one must not rewrite another.
	a, b := BoolPtr(true), BoolPtr(true)
	*a = false
	if *b != true {
		t.Fatal("BoolPtr must return a fresh pointer each call")
	}
}
