package shopkeepers

import (
	"bytes"
	"log"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// fakeShopDefinition is a plugins.ShopDefinition over explicit
// trader rows, for binding tests.
type fakeShopDefinition struct {
	id      int
	name    string
	traders []plugins.ShopNpcInteraction
}

func (d *fakeShopDefinition) ID() int                                 { return d.id }
func (d *fakeShopDefinition) Name() string                            { return d.name }
func (d *fakeShopDefinition) Currency() string                        { return "" }
func (d *fakeShopDefinition) OriginalStock() []plugins.ShopStockEntry { return nil }
func (d *fakeShopDefinition) DefaultRestockTicks() int                { return 4 }
func (d *fakeShopDefinition) DefaultDestockTicks() int                { return 4 }
func (d *fakeShopDefinition) SoldItemDestockTicks() int               { return 100 }
func (d *fakeShopDefinition) Source() string                          { return "core" }
func (d *fakeShopDefinition) SalesPolicy() plugins.ShopSalesPolicy {
	return plugins.ShopSalesPolicy{}
}
func (d *fakeShopDefinition) NpcInteractions() []plugins.ShopNpcInteraction {
	return append([]plugins.ShopNpcInteraction(nil), d.traders...)
}

func fakeShop(id int, name string, traders ...plugins.ShopNpcInteraction) *fakeShopDefinition {
	return &fakeShopDefinition{id: id, name: name, traders: traders}
}

// fakeCurrencyRegistry records currency registrations, for the
// Register path: the Go form of the ShopManager currency map.
type fakeCurrencyRegistry struct {
	named map[string]plugins.ShopCurrencyHandler
	items []fakeItemCurrency
}

type fakeItemCurrency struct {
	id   int
	opts plugins.ShopItemCurrencyOptions
}

func (r *fakeCurrencyRegistry) RegisterCurrency(name string, handler plugins.ShopCurrencyHandler) {
	if r.named == nil {
		r.named = make(map[string]plugins.ShopCurrencyHandler)
	}
	r.named[name] = handler
}

func (r *fakeCurrencyRegistry) RegisterItemCurrency(itemID int, opts plugins.ShopItemCurrencyOptions) {
	r.items = append(r.items, fakeItemCurrency{id: itemID, opts: opts})
}

// fakeShopManager records ShopManagerAPI opens, for the emit path.
type fakeShopManager struct {
	opened []int
}

func (f *fakeShopManager) Open(player *game.Player, shopID int, resetScroll bool) bool {
	f.opened = append(f.opened, shopID)
	return true
}
func (f *fakeShopManager) Close(player *game.Player) {}
func (f *fakeShopManager) HasShop(shopID int) bool   { return true }
func (f *fakeShopManager) GetOpenShop(player *game.Player) *plugins.OpenShopView {
	return nil
}
func (f *fakeShopManager) Reload() int                                  { return 0 }
func (f *fakeShopManager) AddShopStock(shopID, itemID, amount int) bool { return false }
func (f *fakeShopManager) RemoveShop(shopID int)                        {}
func (f *fakeShopManager) ShareStock(source plugins.ShopStockSource)    {}

// testHub builds a hub with only Shopkeepers loaded, the fake
// currency registry installed BEFORE Load (so the plugin's
// Register flushes straight into it), and returns both. The
// plugin's Register stashes pluginAPI, so InstallBindings works
// on the returned hub.
func testHub(t *testing.T) (*plugins.Hub, *fakeCurrencyRegistry) {
	t.Helper()
	var buf bytes.Buffer
	reg := &fakeCurrencyRegistry{}
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{Logger: log.New(&buf, "", 0)})
	hub.SetShopCurrencyRegistry(reg)
	hub.Load()
	if len(hub.Loaded()) != 1 || hub.Loaded()[0] != "Shopkeepers" {
		t.Fatalf("loaded = %v, want [Shopkeepers] (log: %s)", hub.Loaded(), buf.String())
	}
	return hub, reg
}

func testActions(labels map[int][]string) func(int) []string {
	return func(npcID int) []string { return labels[npcID] }
}

func TestComputeBindingsUniqueAmbiguousAndCustom(t *testing.T) {
	defaults := []plugins.ShopDefinition{
		fakeShop(1102, "Edgeville General Store", plugins.ShopNpcInteraction{NPCIDs: []int{2821, 2822}, OptionSlot: 3}),
		fakeShop(1020, "Armour Shop (Jatizso)", plugins.ShopNpcInteraction{NPCIDs: []int{1943}, OptionSlot: 3}),
		fakeShop(1497, "Armour Shop", plugins.ShopNpcInteraction{NPCIDs: []int{1943}, OptionSlot: 3}),
		fakeShop(1461, "Ranging Guild Ticket Exchange", plugins.ShopNpcInteraction{NPCIDs: []int{6071}, OptionSlot: 1}),
	}
	customs := []plugins.ShopDefinition{
		fakeShop(9000, "Custom Store", plugins.ShopNpcInteraction{NPCIDs: []int{2821}, OptionSlot: 3}),
	}
	b := ComputeBindings(defaults, customs)
	// Unique: one shop per key.
	if got := b.Unique[TraderKey{NPCID: 2821, Slot: 3}]; got != 1102 {
		t.Errorf("Unique[2821:3] = %d, want 1102", got)
	}
	if got := b.Unique[TraderKey{NPCID: 2822, Slot: 3}]; got != 1102 {
		t.Errorf("Unique[2822:3] = %d, want 1102", got)
	}
	if got := b.Unique[TraderKey{NPCID: 6071, Slot: 1}]; got != 1461 {
		t.Errorf("Unique[6071:1] = %d, want 1461", got)
	}
	// Ambiguous: 1943:3 is claimed by two different shops, so
	// it is absent from Unique and present in Shared, in
	// first-seen order (TS defaultShopkeepers null + sharedTraders).
	if _, ok := b.Unique[TraderKey{NPCID: 1943, Slot: 3}]; ok {
		t.Error("Unique[1943:3]: want absent (ambiguous)")
	}
	shared := b.Shared[TraderKey{NPCID: 1943, Slot: 3}]
	if len(shared) != 2 || shared[0].ID != 1020 || shared[1].ID != 1497 {
		t.Fatalf("Shared[1943:3] = %+v, want [1020 1497]", shared)
	}
	if shared[0].Name != "Armour Shop (Jatizso)" || shared[1].Name != "Armour Shop" {
		t.Errorf("Shared[1943:3] names = %q %q, want the two shop names", shared[0].Name, shared[1].Name)
	}
	// Custom: the custom shop wins the key at emit time.
	if got := b.Custom[TraderKey{NPCID: 2821, Slot: 3}]; got != 9000 {
		t.Errorf("Custom[2821:3] = %d, want 9000", got)
	}
	if len(b.Custom) != 1 {
		t.Errorf("len(Custom) = %d, want 1", len(b.Custom))
	}
}

func TestComputeBindingsSameShopDuplicateIsShared(t *testing.T) {
	// TS sharedTraders appends per key appearance, so a shop
	// listing the same key twice counts as shared — while
	// defaultShopkeepers keeps the id (same shop, not ambiguous).
	shop := fakeShop(1102, "Edgeville General Store",
		plugins.ShopNpcInteraction{NPCIDs: []int{2821}, OptionSlot: 3},
		plugins.ShopNpcInteraction{NPCIDs: []int{2821}, OptionSlot: 3},
	)
	b := ComputeBindings([]plugins.ShopDefinition{shop}, nil)
	if got := b.Unique[TraderKey{NPCID: 2821, Slot: 3}]; got != 1102 {
		t.Errorf("Unique[2821:3] = %d, want 1102", got)
	}
	if shared := b.Shared[TraderKey{NPCID: 2821, Slot: 3}]; len(shared) != 2 {
		t.Errorf("Shared[2821:3] has %d entries, want 2 (per-occurrence list)", len(shared))
	}
}

func TestLabelsFor(t *testing.T) {
	b := Bindings{
		Custom: map[TraderKey]int{{NPCID: 2821, Slot: 3}: 9000},
		Unique: map[TraderKey]int{
			{NPCID: 2821, Slot: 3}: 1102, // custom-overridden: no label
			{NPCID: 2822, Slot: 3}: 1102,
			{NPCID: 6071, Slot: 1}: 1461,
		},
		Shared: map[TraderKey][]ShopRef{
			{NPCID: 1943, Slot: 3}: {{ID: 1020, Name: "Armour Shop (Jatizso)"}, {ID: 1497, Name: "Armour Shop"}},
		},
	}
	actions := testActions(map[int][]string{
		2821: {"Talk-to", "Attack", "Trade", "", ""},
		2822: {"Talk-to", "Attack", "Trade", "", ""},
		6071: {"Exchange", "", "", "", ""},
		1943: {"Talk-to", "Attack", "Trade", "", ""},
		9999: {"Custom-only", "", "", "", ""},
	})
	labels := LabelsFor(b, actions)
	for _, want := range []string{"Trade", "Exchange"} {
		if !labels[want] {
			t.Errorf("labels missing %q: %v", want, labels)
		}
	}
	if labels["Custom-only"] {
		t.Error("labels has Custom-only: a custom-only NPC must contribute no label")
	}
	// A nil actions source and an unknown NPC yield nothing.
	if got := LabelsFor(b, nil); len(got) != 0 {
		t.Errorf("LabelsFor(nil actions) = %v, want empty", got)
	}
	b2 := Bindings{Unique: map[TraderKey]int{{NPCID: 12345, Slot: 3}: 1102}}
	if got := LabelsFor(b2, actions); len(got) != 0 {
		t.Errorf("LabelsFor(unknown NPC) = %v, want empty", got)
	}
}

func TestRegisterCurrencies(t *testing.T) {
	_, reg := testHub(t)
	if len(reg.items) != 19 {
		t.Fatalf("item currencies = %d, want 19 (ITEM_CURRENCIES)", len(reg.items))
	}
	byID := make(map[int]plugins.ShopItemCurrencyOptions, len(reg.items))
	for _, entry := range reg.items {
		byID[entry.id] = entry.opts
	}
	// Tokkul, the currency the live leg exercises.
	if _, ok := byID[6529]; !ok {
		t.Error("item currencies missing Tokkul 6529")
	}
	// Trading sticks carry the unit word.
	sticks, ok := byID[6306]
	if !ok {
		t.Fatal("item currencies missing trading sticks 6306")
	}
	if !sticks.HasUnit || sticks.Unit != "trade sticks" {
		t.Errorf("6306 unit = %q hasUnit=%v, want \"trade sticks\"/true", sticks.Unit, sticks.HasUnit)
	}
	// Aliases ride on the options.
	if got := byID[2996].Aliases; len(got) != 1 || got[0] != "Agility arena ticket (discontinued)" {
		t.Errorf("2996 aliases = %v, want [Agility arena ticket (discontinued)]", got)
	}
	if got := byID[12746].Aliases; len(got) != 1 || got[0] != "Archaic emblem" {
		t.Errorf("12746 aliases = %v, want [Archaic emblem]", got)
	}
	if got := byID[4278].Aliases; len(got) != 1 || got[0] != "ecto-tokens" {
		t.Errorf("4278 aliases = %v, want [ecto-tokens]", got)
	}
	// Point currencies: 13, registered under the trimmed
	// UPPERCASED keys (TS registerPointCurrency uppercases at
	// the call site; the registry stores keys verbatim).
	if len(reg.named) != 13 {
		t.Fatalf("point currencies = %d, want 13 (POINT_CURRENCIES)", len(reg.named))
	}
	for _, want := range []string{"BOUNTY HUNTER POINTS", "DEADMAN POINTS", "EVENT POINTS", "HONOUR POINTS", "LAST MAN STANDING POINTS", "LEAGUE POINTS", "MAHOGANY HOMES POINTS", "NMZ", "PVP ARENA POINTS", "SPEEDRUN POINTS", "TITHE", "VOLCANIC MINE POINTS", "ZEAL TOKENS"} {
		h, ok := reg.named[want]
		if !ok {
			t.Errorf("point currencies missing %q", want)
			continue
		}
		if h.Name == "" || h.Amount == nil || h.Add == nil || h.Remove == nil {
			t.Errorf("handler %q is incomplete: %+v", want, h)
		}
	}
	if _, ok := reg.named["Bounty Hunter Points"]; ok {
		t.Error("point currencies registered a mixed-case key: TS uppercases at the call site")
	}
}

func TestPointCurrencyHandler(t *testing.T) {
	_, reg := testHub(t)
	handler := reg.named["BOUNTY HUNTER POINTS"]
	if handler.Name != "Bounty Hunter Points" {
		t.Fatalf("handler.Name = %q, want \"Bounty Hunter Points\"", handler.Name)
	}
	player := game.NewPlayer("alice", game.Location{X: 3090, Y: 3519, Z: 0})
	if got := handler.Amount(player); got != 0 {
		t.Fatalf("fresh Amount = %d, want 0", got)
	}
	handler.Add(player, 5)
	if got := handler.Amount(player); got != 5 {
		t.Fatalf("after Add(5): Amount = %d, want 5", got)
	}
	// A negative add is clamped to 0 (TS Math.max(0, value)).
	handler.Add(player, -3)
	if got := handler.Amount(player); got != 5 {
		t.Errorf("after Add(-3): Amount = %d, want 5", got)
	}
	handler.Remove(player, 2)
	if got := handler.Amount(player); got != 3 {
		t.Errorf("after Remove(2): Amount = %d, want 3", got)
	}
	handler.Remove(player, 10)
	if got := handler.Amount(player); got != 0 {
		t.Errorf("after Remove(10): Amount = %d, want 0 (clamped)", got)
	}
	// TS does NOT clamp the removed amount: removing a negative
	// value adds to the balance (amount - floor(value)).
	handler.Remove(player, -4)
	if got := handler.Amount(player); got != 4 {
		t.Errorf("after Remove(-4): Amount = %d, want 4 (TS unclamped-negative quirk)", got)
	}
	// Stored values coerce like TS Number(): a JSON float floors,
	// a numeric string parses, anything else is 0.
	player.SetAttribute("shop-currency:Bounty Hunter Points", 7.9)
	if got := handler.Amount(player); got != 7 {
		t.Errorf("stored 7.9: Amount = %d, want 7", got)
	}
	player.SetAttribute("shop-currency:Bounty Hunter Points", "12")
	if got := handler.Amount(player); got != 12 {
		t.Errorf("stored \"12\": Amount = %d, want 12", got)
	}
	player.SetAttribute("shop-currency:Bounty Hunter Points", "abc")
	if got := handler.Amount(player); got != 0 {
		t.Errorf("stored \"abc\": Amount = %d, want 0", got)
	}
	player.SetAttribute("shop-currency:Bounty Hunter Points", nil)
	if got := handler.Amount(player); got != 0 {
		t.Errorf("stored nil: Amount = %d, want 0", got)
	}
	if got := handler.Amount(nil); got != 0 {
		t.Errorf("nil player Amount = %d, want 0", got)
	}
	// Aliases register under their own uppercased keys.
	RegisterPointCurrency(pluginAPI, "Test Points", "TP", "  ", "")
	if _, ok := reg.named["TEST POINTS"]; !ok {
		t.Error("alias registration missing TEST POINTS")
	}
	if _, ok := reg.named["TP"]; !ok {
		t.Error("alias registration missing TP")
	}
	if _, ok := reg.named["  "]; ok {
		t.Error("blank alias key must be skipped")
	}
}

func TestInstallBindingsOpenShop(t *testing.T) {
	hub, _ := testHub(t)
	manager := &fakeShopManager{}
	hub.SetShopManager(manager)
	defaults := []plugins.ShopDefinition{
		fakeShop(1102, "Edgeville General Store", plugins.ShopNpcInteraction{NPCIDs: []int{2821, 2822}, OptionSlot: 3}),
	}
	actions := testActions(map[int][]string{
		2821: {"Talk-to", "Attack", "Trade", "", ""},
		2822: {"Talk-to", "Attack", "Trade", "", ""},
	})
	if bound := InstallBindings(hub, defaults, nil, actions); bound != 2 {
		t.Fatalf("InstallBindings bound = %d, want 2 keys", bound)
	}
	player := game.NewPlayer("alice", game.Location{X: 3090, Y: 3519, Z: 0})
	npc := game.NewNPC(2821, game.Location{X: 3090, Y: 3519, Z: 0})
	event := &plugins.NpcInteractionEvent{Player: player, NPC: npc, NpcID: 2821, ClickType: 3, Location: npc.Location(), DefinitionName: "Shop keeper", Action: "Trade"}
	if !hub.EmitNpcInteraction(event) {
		t.Fatal("Trade click on 2821: want handled")
	}
	if len(manager.opened) != 1 || manager.opened[0] != 1102 {
		t.Fatalf("opened = %v, want [1102]", manager.opened)
	}
	// Talk-to (slot 1) is not a shop option: the click hook is
	// slot 3 only and the any-hook has no "Talk-to" label.
	manager.opened = nil
	if hub.EmitNpcInteraction(&plugins.NpcInteractionEvent{Player: player, NPC: npc, NpcID: 2821, ClickType: 1, Location: npc.Location(), DefinitionName: "Shop keeper", Action: "Talk-to"}) {
		t.Error("Talk-to click: want unhandled")
	}
	if len(manager.opened) != 0 {
		t.Errorf("opened = %v after Talk-to, want none", manager.opened)
	}
}

func TestInstallBindingsSharedTraderNeedsSubscriber(t *testing.T) {
	hub, _ := testHub(t)
	manager := &fakeShopManager{}
	hub.SetShopManager(manager)
	defaults := []plugins.ShopDefinition{
		fakeShop(1020, "Armour Shop (Jatizso)", plugins.ShopNpcInteraction{NPCIDs: []int{1943}, OptionSlot: 3}),
		fakeShop(1497, "Armour Shop", plugins.ShopNpcInteraction{NPCIDs: []int{1943}, OptionSlot: 3}),
	}
	actions := testActions(map[int][]string{
		1943: {"Talk-to", "Attack", "Trade", "", ""},
	})
	if bound := InstallBindings(hub, defaults, nil, actions); bound != 1 {
		t.Fatalf("InstallBindings bound = %d, want 1 key", bound)
	}
	player := game.NewPlayer("alice", game.Location{X: 3090, Y: 3519, Z: 0})
	npc := game.NewNPC(1943, game.Location{X: 3090, Y: 3519, Z: 0})
	event := &plugins.NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1943, ClickType: 3, Location: npc.Location(), DefinitionName: "Armourer", Action: "Trade"}
	// Neither version ends " (Default)" and no subscriber picks
	// one, so the click is unhandled — exactly as in TS.
	if hub.EmitNpcInteraction(event) {
		t.Fatal("shared trader with no subscriber: want unhandled")
	}
	if len(manager.opened) != 0 {
		t.Fatalf("opened = %v, want none", manager.opened)
	}
	// A subscriber that picks a version opens it.
	hub.SubscribeCustomEvent(plugins.NpcDialogueShopEventName, func(payload any) {
		request, ok := payload.(*game.NpcDialogueShopEvent)
		if !ok {
			return
		}
		if len(request.Shops) > 0 {
			pick := request.Shops[0]
			request.Shop = &pick
		}
	})
	if !hub.EmitNpcInteraction(&plugins.NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1943, ClickType: 3, Location: npc.Location(), DefinitionName: "Armourer", Action: "Trade"}) {
		t.Fatal("shared trader with a picking subscriber: want handled")
	}
	if len(manager.opened) != 1 || manager.opened[0] != 1020 {
		t.Fatalf("opened = %v, want [1020]", manager.opened)
	}
}

func TestInstallBindingsSharedTraderDefaultVersion(t *testing.T) {
	// tradeVersion picks request.shop ?? the first name ending
	// " (Default)" (Shopkeepers.plugin.js:155-171): with no
	// subscriber at all, TzHaar-Hur-Lek's Default version opens.
	hub, _ := testHub(t)
	manager := &fakeShopManager{}
	hub.SetShopManager(manager)
	defaults := []plugins.ShopDefinition{
		fakeShop(1468, "TzHaar-Hur-Lek's Ore and Gem Store (Default)", plugins.ShopNpcInteraction{NPCIDs: []int{2184}, OptionSlot: 3}),
		fakeShop(1469, "TzHaar-Hur-Lek's Ore and Gem Store (Karamja gloves)", plugins.ShopNpcInteraction{NPCIDs: []int{2184}, OptionSlot: 3}),
	}
	actions := testActions(map[int][]string{
		2184: {"Talk-to", "Attack", "Trade", "", ""},
	})
	if bound := InstallBindings(hub, defaults, nil, actions); bound != 1 {
		t.Fatalf("InstallBindings bound = %d, want 1 key", bound)
	}
	player := game.NewPlayer("alice", game.Location{X: 2464, Y: 5152, Z: 0})
	npc := game.NewNPC(2184, game.Location{X: 2464, Y: 5149, Z: 0})
	event := &plugins.NpcInteractionEvent{Player: player, NPC: npc, NpcID: 2184, ClickType: 3, Location: npc.Location(), DefinitionName: "TzHaar-Hur-Lek", Action: "Trade"}
	if !hub.EmitNpcInteraction(event) {
		t.Fatal("shared trader with a Default version and no subscriber: want handled")
	}
	if len(manager.opened) != 1 || manager.opened[0] != 1468 {
		t.Fatalf("opened = %v, want [1468] (the Default version)", manager.opened)
	}
	// A subscriber pick beats the Default.
	hub.SubscribeCustomEvent(plugins.NpcDialogueShopEventName, func(payload any) {
		request, ok := payload.(*game.NpcDialogueShopEvent)
		if !ok {
			return
		}
		pick := "TzHaar-Hur-Lek's Ore and Gem Store (Karamja gloves)"
		request.Shop = &pick
	})
	manager.opened = nil
	if !hub.EmitNpcInteraction(&plugins.NpcInteractionEvent{Player: player, NPC: npc, NpcID: 2184, ClickType: 3, Location: npc.Location(), DefinitionName: "TzHaar-Hur-Lek", Action: "Trade"}) {
		t.Fatal("shared trader with a picking subscriber: want handled")
	}
	if len(manager.opened) != 1 || manager.opened[0] != 1469 {
		t.Fatalf("opened = %v, want [1469] (the subscriber's pick)", manager.opened)
	}
}

func TestInstallBindingsCustomClaimsFirst(t *testing.T) {
	hub, _ := testHub(t)
	manager := &fakeShopManager{}
	hub.SetShopManager(manager)
	defaults := []plugins.ShopDefinition{
		fakeShop(1102, "Edgeville General Store", plugins.ShopNpcInteraction{NPCIDs: []int{2821}, OptionSlot: 3}),
	}
	customs := []plugins.ShopDefinition{
		fakeShop(9000, "Custom Store", plugins.ShopNpcInteraction{NPCIDs: []int{2821}, OptionSlot: 3}),
	}
	actions := testActions(map[int][]string{
		2821: {"Talk-to", "Attack", "Trade", "", ""},
	})
	if bound := InstallBindings(hub, defaults, customs, actions); bound != 1 {
		t.Fatalf("InstallBindings bound = %d, want 1 distinct key", bound)
	}
	player := game.NewPlayer("alice", game.Location{X: 3090, Y: 3519, Z: 0})
	npc := game.NewNPC(2821, game.Location{X: 3090, Y: 3519, Z: 0})
	event := &plugins.NpcInteractionEvent{Player: player, NPC: npc, NpcID: 2821, ClickType: 3, Location: npc.Location(), DefinitionName: "Shop keeper", Action: "Trade"}
	if !hub.EmitNpcInteraction(event) {
		t.Fatal("custom-bound Trade click: want handled")
	}
	if len(manager.opened) != 1 || manager.opened[0] != 9000 {
		t.Fatalf("opened = %v, want [9000] (custom claims before the any-hook)", manager.opened)
	}
}

func TestInstallBindingsNoPlugin(t *testing.T) {
	// InstallBindings is a no-op when the plugin never
	// registered (pluginAPI nil): a disabled plugin must not
	// install hooks. Save/restore the package-level api AFTER
	// testHub loads (Load stashes the api).
	hub, _ := testHub(t)
	old := pluginAPI
	pluginAPI = nil
	defer func() { pluginAPI = old }()
	if bound := InstallBindings(hub, nil, nil, nil); bound != 0 {
		t.Errorf("InstallBindings with nil api = %d, want 0", bound)
	}
	if hub.EmitNpcInteraction(&plugins.NpcInteractionEvent{Player: game.NewPlayer("alice", game.Location{}), NpcID: 2821, ClickType: 3, Action: "Trade"}) {
		t.Error("no bindings installed: want unhandled")
	}
}
