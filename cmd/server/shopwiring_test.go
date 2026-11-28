package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// shopFixtureDir writes the minimal definitions directory the
// loader needs (see chatwiring_test.go's TestStoreExamineSource),
// with shops.json holding two shops named "Test Shop" (ids 7 and
// 8, in that order) plus one "Other Shop", and returns the
// loaded Store.
func shopFixtureStore(t *testing.T) *definition.Store {
	t.Helper()
	dir := t.TempDir()
	shops := `[
		{"id": 7, "name": "Test Shop", "currency": "coins",
		 "originalStock": [{"id": 995, "amount": 10, "restockTicks": 5, "price": 100}, {"id": 2, "amount": 3}],
		 "generalStore": true, "sellsAt": 1000, "buysAt": 500, "changePer": 10},
		{"id": 8, "name": "Test Shop", "currency": "tokkul", "originalStock": []},
		{"id": 9, "name": "Other Shop", "originalStock": [{"id": 3, "amount": 1}]}
	]`
	for name, content := range map[string]string{
		"interface-layouts.json": `{}`, "object-spawns.json": `[]`,
		"ground-items.json": `[]`, "npc-combat-defs.json": `{}`,
		"npc-animations.json": `{}`, "shops.json": shops,
		"npc-drops.json":    `{"tables": {}, "npcs": {}}`,
		"custom-shops.json": `[]`, "npc-spawns.json": `[]`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	store, err := definition.Load(dir, definition.LoadOptions{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

func TestConvertShopDefinitions(t *testing.T) {
	store := shopFixtureStore(t)
	defs := convertShopDefinitions(store)
	if len(defs) != 3 {
		t.Fatalf("converted %d shops, want 3", len(defs))
	}
	first := defs[0]
	if first.ID != 7 || first.Name != "Test Shop" || first.Currency != "COINS" {
		t.Errorf("first = %+v, want id 7 Test Shop COINS", first)
	}
	if first.Source != "core" {
		t.Errorf("first.Source = %q, want core", first.Source)
	}
	if len(first.OriginalStock) != 2 {
		t.Fatalf("first stock = %d rows, want 2", len(first.OriginalStock))
	}
	row := first.OriginalStock[0]
	if row.ID != 995 || row.Amount != 10 || row.RestockTicks == nil || *row.RestockTicks != 5 || row.Price == nil || *row.Price != 100 {
		t.Errorf("first stock row = %+v, want id 995 amount 10 restock 5 price 100", row)
	}
	if row := first.OriginalStock[1]; row.ID != 2 || row.Amount != 3 || row.RestockTicks != nil || row.Price != nil {
		t.Errorf("second stock row = %+v, want id 2 amount 3 with nil ticks/price", row)
	}
	policy := first.SalesPolicy
	if policy.GeneralStore == nil || !*policy.GeneralStore ||
		policy.SellsAt == nil || *policy.SellsAt != 1000 ||
		policy.BuysAt == nil || *policy.BuysAt != 500 ||
		policy.ChangePer == nil || *policy.ChangePer != 10 {
		t.Errorf("policy = %+v, want generalStore sellsAt 1000 buysAt 500 changePer 10", policy)
	}
	if defs[1].ID != 8 || defs[2].ID != 9 {
		t.Errorf("order = %d, %d, want 7, 8, 9", defs[0].ID, defs[1].ID)
	}
	if got := convertShopDefinitions(nil); got != nil {
		t.Errorf("convertShopDefinitions(nil) = %v, want nil", got)
	}
}

func TestNewShopManagerInitializesFromStore(t *testing.T) {
	manager := newShopManager(shopFixtureStore(t), nil)
	if !manager.HasShop(7) || !manager.HasShop(8) || !manager.HasShop(9) {
		t.Error("manager missing a fixture shop")
	}
	if manager.HasShop(10) {
		t.Error("manager has unloaded shop 10")
	}
	empty := newShopManager(nil, nil)
	if empty.HasShop(7) {
		t.Error("nil-store manager has shops")
	}
}

func TestShopOpenerFindShops(t *testing.T) {
	opener := shopOpener{manager: newShopManager(nil, nil), store: shopFixtureStore(t)}
	found := opener.FindShops("Test Shop")
	if len(found) != 2 || found[0].ID != 7 || found[1].ID != 8 {
		t.Errorf("FindShops = %+v, want ids 7, 8 in load order", found)
	}
	if got := opener.FindShops("test shop"); got != nil {
		t.Errorf("FindShops case-insensitive = %v, want nil (exact match)", got)
	}
	if got := opener.FindShops(""); got != nil {
		t.Errorf("FindShops(\"\") = %v, want nil", got)
	}
	if got := opener.FindShops("No Such Shop"); got != nil {
		t.Errorf("FindShops(unknown) = %v, want nil", got)
	}
	if got := (shopOpener{}).FindShops("Test Shop"); got != nil {
		t.Errorf("nil-store FindShops = %v, want nil", got)
	}
}

func TestShopOpenerOpenShop(t *testing.T) {
	manager := newShopManager(shopFixtureStore(t), nil)
	opener := shopOpener{manager: manager, store: shopFixtureStore(t)}
	p := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	if !opener.OpenShop(p, 7) {
		t.Fatal("OpenShop(7) = false")
	}
	if open := manager.GetOpenShop(p); open == nil {
		t.Error("no open shop after OpenShop")
	}
	if (shopOpener{}).OpenShop(p, 7) {
		t.Error("nil-manager OpenShop = true")
	}
	if opener.OpenShop(nil, 7) {
		t.Error("nil-player OpenShop = true")
	}
}

// recordingSessionPort captures SendPacket frames for sender
// tests. It implements game.SessionPort and deliberately does
// NOT implement game.ChatMessageSender, so SendMessage is a
// no-op through it.
type recordingSessionPort struct {
	packets [][]byte
}

func (s *recordingSessionPort) SendPacket(packet []byte) {
	s.packets = append(s.packets, append([]byte(nil), packet...))
}
func (s *recordingSessionPort) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *recordingSessionPort) SendDestination(x, y int)      {}
func (s *recordingSessionPort) SendVarp(id, value int)        {}
func (s *recordingSessionPort) FlushTick(cycle int64) error   { return nil }
func (s *recordingSessionPort) Close(code int, reason string) {}
func (s *recordingSessionPort) IsOpen() bool                  { return true }

func TestShopSenderFrames(t *testing.T) {
	rec := &recordingSessionPort{}
	p := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	p.Session = rec
	view := game.ShopOpenView{
		ShopID: 7, Title: "Test Shop", CurrencyItemID: 995,
		GeneralStore: true, QuantityMode: 1,
		TargetUID: game.DefaultShopTargetUID, Opening: true,
		Stock: []game.ShopSlotView{{Slot: 0, ItemID: 2, Quantity: 5, DefaultQuantity: 5, PriceEach: 100, SellPrice: 85}},
	}
	shopSender{}.SendShopOpen(p, view)
	want := [][]byte{
		protocol.EncodeVarbit(6348, 1),
		protocol.EncodeWidgetOpenSub(game.DefaultShopTargetUID, 300, 0, protocol.WidgetOpenSubOptions{}),
		protocol.EncodeWidgetOpenSub((161<<16)|79, 301, 1, protocol.WidgetOpenSubOptions{}),
		protocol.EncodeWidgetRunScript(1074, []protocol.ScriptArg{
			protocol.ScriptArgInt(516), protocol.ScriptArgString("Test Shop"),
			protocol.ScriptArgInt(995), protocol.ScriptArgInt(0), protocol.ScriptArgInt(1),
		}, nil, nil, nil),
		protocol.EncodeWidgetSetFlagsRange((300<<16)|16, 0, 299, 1662),
		protocol.EncodeWidgetRunScript(149, []protocol.ScriptArg{
			protocol.ScriptArgInt(301 << 16), protocol.ScriptArgInt(93),
			protocol.ScriptArgInt(4), protocol.ScriptArgInt(7),
			protocol.ScriptArgInt(0), protocol.ScriptArgInt(-1),
			protocol.ScriptArgString("Value"), protocol.ScriptArgString("Sell 1"),
			protocol.ScriptArgString("Sell 5"), protocol.ScriptArgString("Sell 10"),
			protocol.ScriptArgString("Sell 50"),
		}, nil, nil, nil),
		protocol.EncodeWidgetSetFlagsRange(301<<16, 0, 27, 1086),
		protocol.EncodeShopOpen("7", "Test Shop", 995, true, 1, 1, []protocol.ShopSlotView{
			{Slot: 0, ItemID: 2, Quantity: 5, DefaultQuantity: 5, PriceEach: 100, SellPrice: 85},
		}),
	}
	if len(rec.packets) != len(want) {
		t.Fatalf("sent %d frames, want %d", len(rec.packets), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(rec.packets[i], want[i]) {
			t.Errorf("frame %d = %v, want %v", i, rec.packets[i], want[i])
		}
	}
	if rec.packets[len(rec.packets)-1][0] != 150 {
		t.Errorf("last frame opcode = %d, want 150 (shop open)", rec.packets[len(rec.packets)-1][0])
	}

	rec.packets = nil
	shopSender{}.SendShopClose(p)
	if len(rec.packets) != 1 || !reflect.DeepEqual(rec.packets[0], protocol.EncodeShopClose()) {
		t.Errorf("close frames = %v, want one EncodeShopClose", rec.packets)
	}

	// Nil player/session send nothing and never panic.
	shopSender{}.SendShopOpen(nil, view)
	shopSender{}.SendShopOpen(game.NewPlayer("nosession", game.Location{}), view)
	shopSender{}.SendShopClose(nil)
	shopSender{}.SendEnterAmountPrompt(p, "How many?", func(int) {})
}

func TestInstallShopHooks(t *testing.T) {
	var opened *game.ShopOpenedEvent
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "ShopProbe",
		Register: func(api *plugins.Api) {
			api.OnCustomEvent("shop:opened", func(payload any) {
				opened = payload.(*game.ShopOpenedEvent)
			})
			api.OnCanShop(func(e *plugins.CanShopEvent) {
				e.Allow = plugins.BoolPtr(false)
			})
		},
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()
	manager := game.NewShopManager(nil)
	installShopHooks(manager, hub)
	if manager.Hooks.CanShop == nil {
		t.Fatal("CanShop seam unset")
	}
	if ans := manager.Hooks.CanShop(game.NewPlayer("alice", game.Location{}), 7); ans == nil || *ans {
		t.Errorf("CanShop = %v, want false", ans)
	}
	event := &game.ShopOpenedEvent{}
	manager.Hooks.OnShopOpened(event)
	if opened != event {
		t.Error("shop:opened custom event did not fire with the payload")
	}
	// Nil hub leaves the seam inert.
	plain := game.NewShopManager(nil)
	installShopHooks(plain, nil)
	if plain.Hooks.CanShop != nil || plain.Hooks.OnShopOpened != nil {
		t.Error("nil hub installed hooks")
	}
}

func TestInstallCanStockItemHook(t *testing.T) {
	saved := definition.CanStockItemHook
	defer func() { definition.CanStockItemHook = saved }()
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "StockProbe",
		Register: func(api *plugins.Api) {
			api.OnCanStockItem(func(e *plugins.CanStockItemEvent) {
				if e.ShopID == 7 && e.ItemID == 995 {
					e.Allow = plugins.BoolPtr(false)
				}
			})
		},
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()
	installCanStockItemHook(hub)
	if definition.CanStockItemHook == nil {
		t.Fatal("hook unset")
	}
	if definition.CanStockItemHook(7, 995) {
		t.Error("vetoed entry kept")
	}
	if !definition.CanStockItemHook(7, 2) || !definition.CanStockItemHook(8, 995) {
		t.Error("unvetoed entry dropped")
	}
	// Nil hub leaves the loader hook untouched.
	sentinel := func(shopID, itemID int) bool { return shopID == 1 }
	definition.CanStockItemHook = sentinel
	installCanStockItemHook(nil)
	if !definition.CanStockItemHook(1, 0) || definition.CanStockItemHook(2, 0) {
		t.Error("nil hub changed the loader hook")
	}
}

func TestInstallTradeHooks(t *testing.T) {
	saved := game.TradeHookSeam
	defer func() { game.TradeHookSeam = saved }()
	var completed *plugins.TradeCompletedEvent
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "TradeProbe",
		Register: func(api *plugins.Api) {
			api.OnCanTrade(func(e *plugins.CanTradeEvent) {
				e.Allow = plugins.BoolPtr(false)
			})
			api.OnTradeRequest(func(e *plugins.TradeRequestEvent) {
				e.Handled = true
			})
			api.OnTradeCompleted(func(e *plugins.TradeCompletedEvent) {
				completed = e
			})
			api.OnCustomEvent("trade:item-eligibility", func(payload any) {
				payload.(*game.TradeItemEligibilityRequest).Allowed = true
			})
		},
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()
	installTradeHooks(hub, nil, nil, discardLogger())
	seam := game.TradeHookSeam
	p := game.NewPlayer("alice", game.Location{})
	q := game.NewPlayer("bob", game.Location{})
	if seam.EmitCanTrade == nil || seam.EmitTradeRequest == nil || seam.EmitTradeCompleted == nil || seam.ItemEligibility == nil {
		t.Fatal("trade hook seam incomplete")
	}
	if ans := seam.EmitCanTrade(p, q); ans == nil || *ans {
		t.Errorf("EmitCanTrade = %v, want false", ans)
	}
	req := &game.TradeRequestEvent{Player: p, Target: q}
	if !seam.EmitTradeRequest(req) || !req.Handled {
		t.Errorf("EmitTradeRequest = handled %v, want true with Handled copied back", req.Handled)
	}
	seam.EmitTradeCompleted(&game.TradeCompletedEvent{Player: p, Partner: q, Received: []game.Item{game.NewItem(995, 10)}})
	if completed == nil || completed.Player != p || len(completed.Received) != 1 {
		t.Errorf("EmitTradeCompleted delivered %+v", completed)
	}
	if !seam.ItemEligibility(game.NewItem(995, 1)) {
		t.Error("ItemEligibility = false, want true from the custom event")
	}
	// GetVarbit is wired unconditionally (Phase 18): a session-less
	// player reads 0, the fresh-store default.
	if seam.GetVarbit == nil {
		t.Error("GetVarbit nil, want the session-store reader wired")
	} else if got := seam.GetVarbit(p, 13130); got != 0 {
		t.Errorf("GetVarbit(session-less player) = %d, want 0", got)
	}
	if seam.DropGroundItem != nil || seam.SavePlayer != nil {
		t.Error("nil manager/saves wired DropGroundItem/SavePlayer")
	}
}

func TestInstallTradeHooksGetVarbitReadsSessionStore(t *testing.T) {
	saved := game.TradeHookSeam
	defer func() { game.TradeHookSeam = saved }()
	installTradeHooks(nil, nil, nil, discardLogger())
	seam := game.TradeHookSeam
	if seam.GetVarbit == nil {
		t.Fatal("GetVarbit nil, want the session-store reader wired")
	}
	// Nil-safe: no player, no session, a session without the
	// VarbitReader extension, and a session with no resolver all
	// read 0, the fresh-store default.
	if got := seam.GetVarbit(nil, 13130); got != 0 {
		t.Errorf("GetVarbit(nil) = %d, want 0", got)
	}
	sessionless := game.NewPlayer("alice", game.Location{})
	if got := seam.GetVarbit(sessionless, 13130); got != 0 {
		t.Errorf("GetVarbit(session-less) = %d, want 0", got)
	}
	stubbed := game.NewPlayer("bob", game.Location{})
	stubbed.Session = &packetRecordingSession{}
	if got := seam.GetVarbit(stubbed, 13130); got != 0 {
		t.Errorf("GetVarbit(no-extension session) = %d, want 0", got)
	}
	unresolved := game.NewPlayer("carol", game.Location{})
	unresolved.Session = session.NewSession(session.NewHeadlessChannel(""))
	if got := seam.GetVarbit(unresolved, 13130); got != 0 {
		t.Errorf("GetVarbit(no-resolver session) = %d, want 0", got)
	}
	// The live rule: varbit 13130 (Trading.ts:50) reads the
	// player's own session store through a fixture definition
	// (backing varp 3000, bit 0) — unset 0, set 1 — and an
	// unknown varbit id reads 0.
	sess := session.NewSession(session.NewHeadlessChannel(""))
	sess.SetVarbitResolver(func(id int) (int, int, int, bool) {
		if id == 13130 {
			return 3000, 0, 0, true
		}
		return 0, 0, 0, false
	})
	p := game.NewPlayer("dave", game.Location{})
	p.Session = sess
	if got := seam.GetVarbit(p, 13130); got != 0 {
		t.Errorf("GetVarbit(unset 13130) = %d, want 0", got)
	}
	sess.SendVarp(3000, 1)
	if got := seam.GetVarbit(p, 13130); got != 1 {
		t.Errorf("GetVarbit(set 13130) = %d, want 1", got)
	}
	if got := seam.GetVarbit(p, 99999); got != 0 {
		t.Errorf("GetVarbit(unknown id) = %d, want 0", got)
	}
}

func TestTradeItemMetaToGround(t *testing.T) {
	if got := tradeItemMetaToGround(nil); got != nil {
		t.Errorf("nil meta = %v, want nil", got)
	}
	got := tradeItemMetaToGround(map[string]json.RawMessage{
		"n": json.RawMessage(`42`),
		"s": json.RawMessage(`"x"`),
		"b": json.RawMessage(`{bad`),
	})
	if got["n"] != json.Number("42") || got["s"] != "x" || got["b"] != "{bad" {
		t.Errorf("meta = %#v, want number/string/raw fallback", got)
	}
}

func TestShopCurrencyRegistryAdapter(t *testing.T) {
	manager := game.NewShopManager(nil)
	adapter := shopCurrencyRegistryAdapter{manager: manager}
	p := game.NewPlayer("alice", game.Location{})
	// Direct currency: the handler's Amount/Add/Remove map
	// through field for field.
	balance := 0
	adapter.RegisterCurrency("PROBE", plugins.ShopCurrencyHandler{
		Amount: func(player *game.Player) int { return balance },
		Add:    func(player *game.Player, amount int) { balance += amount },
		Remove: func(player *game.Player, amount int) { balance -= amount },
		Name:   "Probe",
	})
	balance = 42
	if got := manager.CurrencyAmount(p, "PROBE"); got != 42 {
		t.Errorf("CurrencyAmount(PROBE) = %d, want 42", got)
	}
	// Item currency, no name: the no-cache DEFAULT item name
	// is "", so the handler registers under "Item <id>".
	p.Inventory().Adds(1234, 7)
	adapter.RegisterItemCurrency(1234, plugins.ShopItemCurrencyOptions{Aliases: []string{"myalias", "  ", "MyAlias2"}})
	if got := manager.CurrencyAmount(p, "ITEM 1234"); got != 7 {
		t.Errorf("CurrencyAmount(ITEM 1234) = %d, want 7", got)
	}
	if got := manager.CurrencyAmount(p, "MYALIAS"); got != 7 {
		t.Errorf("CurrencyAmount(MYALIAS) = %d, want 7 (trimmed + uppercased alias)", got)
	}
	if got := manager.CurrencyAmount(p, "MYALIAS2"); got != 7 {
		t.Errorf("CurrencyAmount(MYALIAS2) = %d, want 7", got)
	}
	// Explicit empty name: TS registers under no key — only
	// the aliases. The fallback name must NOT resolve.
	p.Inventory().Adds(2345, 3)
	adapter.RegisterItemCurrency(2345, plugins.ShopItemCurrencyOptions{
		Name: " ", HasName: true, Aliases: []string{"otheralias"},
	})
	// (A whitespace name is non-empty: it registers under the
	// trimmed-uppercase key, which is blank and skipped — the
	// same observable outcome as the empty-name branch for
	// the name key, with the alias still working.)
	if got := manager.CurrencyAmount(p, "OTHERALIAS"); got != 3 {
		t.Errorf("CurrencyAmount(OTHERALIAS) = %d, want 3", got)
	}
	adapter.RegisterItemCurrency(3456, plugins.ShopItemCurrencyOptions{
		Name: "", HasName: true, Aliases: []string{"emptyalias"},
	})
	p.Inventory().Adds(3456, 5)
	if got := manager.CurrencyAmount(p, "EMPTYALIAS"); got != 5 {
		t.Errorf("CurrencyAmount(EMPTYALIAS) = %d, want 5", got)
	}
	if got := manager.CurrencyAmount(p, "ITEM 3456"); got != 0 {
		t.Errorf("CurrencyAmount(ITEM 3456) = %d, want 0 (explicit empty name registers no name key)", got)
	}
	if got := manager.CurrencyAmount(p, ""); got != 0 {
		t.Errorf("CurrencyAmount(\"\") = %d, want 0", got)
	}
	// Nil manager degrades silently.
	nilAdapter := shopCurrencyRegistryAdapter{}
	nilAdapter.RegisterCurrency("X", plugins.ShopCurrencyHandler{})
	nilAdapter.RegisterItemCurrency(1, plugins.ShopItemCurrencyOptions{})
}

func TestTradeStateOrdinalsAligned(t *testing.T) {
	pairs := [][2]int{
		{int(game.TradeStateNone), int(plugins.TradeStateNone)},
		{int(game.TradeStateRequestedTrade), int(plugins.TradeStateRequested)},
		{int(game.TradeStateTradeScreen), int(plugins.TradeStateScreen)},
		{int(game.TradeStateAcceptedTradeScreen), int(plugins.TradeStateAcceptedScreen)},
		{int(game.TradeStateConfirmScreen), int(plugins.TradeStateConfirm)},
		{int(game.TradeStateAcceptedConfirmScreen), int(plugins.TradeStateAcceptedConfirm)},
	}
	for _, pair := range pairs {
		if pair[0] != pair[1] {
			t.Errorf("ordinal mismatch: game %d vs plugins %d", pair[0], pair[1])
		}
	}
	state, ok := tradeStateReader(game.NewPlayer("alice", game.Location{}))
	if !ok || state != plugins.TradeStateNone {
		t.Errorf("tradeStateReader = (%v, %v), want (None, true)", state, ok)
	}
	if _, ok := tradeStateReader(nil); ok {
		t.Error("tradeStateReader(nil) ok, want false")
	}
}

func TestInstallShopHubSurfaces(t *testing.T) {
	var core *plugins.Core
	hub := plugins.NewHub([]plugins.Plugin{{
		Name:     "CoreProbe",
		Register: func(api *plugins.Api) { core = api.Core() },
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()
	if core == nil {
		t.Fatal("probe did not capture Core")
	}
	if core.ShopManager() != nil || core.ShopDefinition() != nil {
		t.Fatal("surfaces installed before installShopHubSurfaces")
	}
	store := shopFixtureStore(t)
	manager := newShopManager(store, nil)
	installShopHubSurfaces(hub, manager, store)
	if api := core.ShopManager(); api == nil || !api.HasShop(7) {
		t.Errorf("Core.ShopManager = %v, want a manager holding shop 7", api)
	}
	registry := core.ShopDefinition()
	if registry == nil {
		t.Fatal("Core.ShopDefinition nil after install")
	}
	def, ok := registry.ForID(7)
	if !ok || def.Name() != "Test Shop" {
		t.Errorf("ForID(7) = %+v, %v, want Test Shop", def, ok)
	}
	if all := registry.All(); len(all) != 3 {
		t.Errorf("All() = %d defs, want 3", len(all))
	}
	state, ok := core.TradeState(game.NewPlayer("alice", game.Location{}))
	if !ok || state != plugins.TradeStateNone {
		t.Errorf("Core.TradeState = (%v, %v), want (None, true)", state, ok)
	}
	// Nil hub installs nothing and never panics.
	installShopHubSurfaces(nil, manager, store)
}

func TestNpcInteractionSource(t *testing.T) {
	registry := definition.NewRegistry()
	err := registry.Register("npc_interactions", "TestPlugin", definition.Source{
		Load: func() (any, error) {
			return []any{map[string]any{
				"npcId": float64(5),
				"firstClick": map[string]any{
					"method": "shops.open",
					"args":   map[string]any{"shopId": float64(3)},
				},
			}}, nil
		},
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	dir := t.TempDir()
	for name, content := range map[string]string{
		"interface-layouts.json": `{}`, "object-spawns.json": `[]`,
		"ground-items.json": `[]`, "npc-combat-defs.json": `{}`,
		"npc-animations.json": `{}`, "shops.json": `[]`,
		"npc-drops.json":    `{"tables": {}, "npcs": {}}`,
		"custom-shops.json": `[]`, "npc-spawns.json": `[]`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	store, err := definition.Load(dir, definition.LoadOptions{Sources: registry, Logf: discardLogger().Printf})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	src := npcInteractionSource{store: store}
	action, ok := src.InteractionAction(5, 1)
	if !ok || action.Method != "shops.open" || action.Args["shopId"] != float64(3) {
		t.Errorf("InteractionAction(5, 1) = %+v, %v, want shops.open shopId 3", action, ok)
	}
	if _, ok := src.InteractionAction(5, 2); ok {
		t.Error("InteractionAction(5, 2) ok, want false")
	}
	if _, ok := src.InteractionAction(9, 1); ok {
		t.Error("InteractionAction(9, 1) ok, want false")
	}
	if _, ok := (npcInteractionSource{}).InteractionAction(5, 1); ok {
		t.Error("nil-store InteractionAction ok, want false")
	}
}

func TestShopWidgetHandlerAdapter(t *testing.T) {
	manager := newShopManager(shopFixtureStore(t), nil)
	adapter := shopWidgetHandlerAdapter{manager: manager}
	p := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	p.Session = &recordingSessionPort{}
	// No open shop: the widget branch declines.
	if adapter.HandleWidgetAction(p, listeners.ShopWidgetPacket{GroupID: 300, ChildID: 8, ButtonNum: 1}) {
		t.Error("widget action handled with no open shop")
	}
	if !manager.Open(p, 7, true, game.DefaultShopTargetUID) {
		t.Fatal("open failed")
	}
	// Quantity button (child 8 = mode 1) claims the click.
	if !adapter.HandleWidgetAction(p, listeners.ShopWidgetPacket{GroupID: 300, ChildID: 8, ButtonNum: 1}) {
		t.Error("quantity button not handled with an open shop")
	}
	if got := manager.QuantityMode(p); got != 1 {
		t.Errorf("QuantityMode = %d, want 1", got)
	}
	// A Value container action on the open shop's first slot
	// claims the packet (the value text goes nowhere: the
	// recording port is not a ChatMessageSender).
	if !adapter.HandleItemContainerAction(p, listeners.ShopItemContainerAction{
		Kind: listeners.ShopContainerActionValue, ContainerID: 3900, Slot: 0, ItemID: 995, Amount: 1,
	}) {
		t.Error("value container action not handled")
	}
	// Nil manager declines everything.
	nilAdapter := shopWidgetHandlerAdapter{}
	if nilAdapter.HandleWidgetAction(p, listeners.ShopWidgetPacket{GroupID: 300}) {
		t.Error("nil-manager widget action handled")
	}
	if nilAdapter.HandleItemContainerAction(p, listeners.ShopItemContainerAction{}) {
		t.Error("nil-manager container action handled")
	}
}

func TestInstallShopListeners(t *testing.T) {
	store := shopFixtureStore(t)
	manager := newShopManager(store, nil)
	deps := listeners.Deps{}
	InstallShopListeners(&deps, store, manager)
	if deps.Shop == nil {
		t.Error("deps.Shop unset")
	}
	if deps.NpcHooks.Interactions == nil {
		t.Error("deps.NpcHooks.Interactions unset")
	}
	if deps.NpcHooks.OpenShop == nil {
		t.Error("deps.NpcHooks.OpenShop unset")
	}
	// Nil manager leaves Shop/OpenShop unset; nil store leaves
	// Interactions unset; nil deps is a no-op.
	partial := listeners.Deps{}
	InstallShopListeners(&partial, store, nil)
	if partial.Shop != nil || partial.NpcHooks.OpenShop != nil {
		t.Error("nil manager installed shop seams")
	}
	if partial.NpcHooks.Interactions == nil {
		t.Error("nil manager cleared Interactions")
	}
	nostore := listeners.Deps{}
	InstallShopListeners(&nostore, nil, manager)
	if nostore.NpcHooks.Interactions != nil {
		t.Error("nil store installed Interactions")
	}
	InstallShopListeners(nil, store, manager)
}
