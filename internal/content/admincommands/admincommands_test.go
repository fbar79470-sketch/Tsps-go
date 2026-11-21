package admincommands

import (
	"io"
	"log"
	"math"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// capturingSession is the SessionPort + ChatMessageSender test double
// from internal/listeners/chat_test.go: it records the game messages
// Player.SendMessage sends it.
type capturingSession struct {
	gameMessages []string
}

func (s *capturingSession) SendPacket(packet []byte) {}
func (s *capturingSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *capturingSession) SendDestination(x, y int)      {}
func (s *capturingSession) SendVarp(id, value int)        {}
func (s *capturingSession) FlushTick(cycle int64) error   { return nil }
func (s *capturingSession) Close(code int, reason string) {}
func (s *capturingSession) IsOpen() bool                  { return true }
func (s *capturingSession) SendGameMessage(message string) {
	s.gameMessages = append(s.gameMessages, message)
}
func (s *capturingSession) SendTypedGameMessage(chatType int, message, from string) {}

var (
	_ game.SessionPort       = (*capturingSession)(nil)
	_ game.ChatMessageSender = (*capturingSession)(nil)
)

// fakeDefs is a minimal ItemDefinitionSource: only the stackable set
// matters to the add path, so everything else reads the nil-source
// defaults (the testItemDefs precedent in internal/game/item_test.go).
// Coins (995) stack; every other id does not.
type fakeDefs struct {
	stackable map[int]bool
}

func (s *fakeDefs) IsStackable(id int) bool      { return s.stackable[id] }
func (s *fakeDefs) IsNoted(id int) bool          { return false }
func (s *fakeDefs) WeightGrams(id int) int       { return 0 }
func (s *fakeDefs) EquipmentSlot(id int) int     { return -1 }
func (s *fakeDefs) IsDoubleHanded(id int) bool   { return false }
func (s *fakeDefs) Requirements(id int) []int    { return nil }
func (s *fakeDefs) CombatRequirement(id int) int { return 0 }
func (s *fakeDefs) IsTradeable(id int) bool      { return false }
func (s *fakeDefs) IsSellable(id int) bool       { return false }
func (s *fakeDefs) IsDropable(id int) bool       { return false }
func (s *fakeDefs) UnNote(id int) int            { return id }
func (s *fakeDefs) PlaceholderID(id int) int     { return -1 }

var _ game.ItemDefinitionSource = (*fakeDefs)(nil)

// testPlayer builds a player with coins stackable and a capturing
// session attached, so handler messages are observable.
func testPlayer() (*game.Player, *capturingSession) {
	p := game.NewPlayer("alice", game.Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(&fakeDefs{stackable: map[int]bool{995: true}})
	session := &capturingSession{}
	p.Session = session
	return p, session
}

// testRegistry builds a hub holding only the AdminCommands plugin,
// loads it, and returns the registry with the engine seams installed
// the way cmd/server's wireCommandRegistry does: messenger =
// player.SendMessage, rights from the given source.
func testRegistry(t *testing.T, rightsOf func(*game.Player) int) *game.CommandRegistry {
	t.Helper()
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{
		Logger: log.New(io.Discard, "", 0),
	})
	hub.Load()
	reg := hub.Commands()
	if reg == nil {
		t.Fatal("hub has no command registry after Load")
	}
	reg.SetMessenger(func(p *game.Player, msg string) { p.SendMessage(msg) })
	reg.SetRightsSource(rightsOf)
	return reg
}

func adminRights(p *game.Player) int { return game.RightsAdministrator }
func noRights(p *game.Player) int    { return game.RightsNone }

// inventoryCounts returns id -> total amount across the player's
// inventory slots.
func inventoryCounts(p *game.Player) map[int]int {
	out := map[int]int{}
	for _, item := range p.Inventory().Items() {
		if item.ID() >= 0 && item.Amount() > 0 {
			out[item.ID()] += item.Amount()
		}
	}
	return out
}

// inventorySlots returns the ids of the occupied inventory slots, in
// slot order.
func inventorySlots(p *game.Player) []int {
	var out []int
	for _, item := range p.Inventory().Items() {
		if item.ID() >= 0 && item.Amount() > 0 {
			out = append(out, item.ID())
		}
	}
	return out
}

func TestParseIntArg(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"882", 882, true},
		{"1", 1, true},
		{"0", 0, true},
		{"12abc", 12, true},  // JS parseInt stops at the first non-digit
		{"0x10", 0, true},    // radix 10: "0" then stops at 'x'
		{"1.5", 1, true},     // stops at '.'
		{"1e3", 1, true},     // stops at 'e'
		{" 42", 42, true},    // leading whitespace skipped
		{"\t\n42", 42, true}, // other JS whitespace skipped
		{"+7", 7, true},
		{"-5", -5, true},
		{"-0", 0, true},
		{"010", 10, true}, // radix 10: no octal
		{"", 0, false},
		{"   ", 0, false},
		{"abc", 0, false},
		{"  abc", 0, false},
		{"+", 0, false},
		{"-", 0, false},
		{"+ 12", 0, false}, // sign must be followed by a digit
		{"--5", 0, false},
	}
	for _, tc := range cases {
		if got, ok := parseIntArg(tc.in); got != tc.want || ok != tc.ok {
			t.Errorf("parseIntArg(%q) = %d, %v; want %d, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParseIntArgSaturates(t *testing.T) {
	// JS parseInt returns a float here; Go saturates (see the package
	// comment). The saturated value is what the handler prints for
	// absurd ids.
	if got, ok := parseIntArg("99999999999999999999"); !ok || got != math.MaxInt64 {
		t.Fatalf("parseIntArg(huge) = %d, %v; want MaxInt64, true", got, ok)
	}
	if got, ok := parseIntArg("-99999999999999999999"); !ok || got != math.MinInt64 {
		t.Fatalf("parseIntArg(-huge) = %d, %v; want MinInt64, true", got, ok)
	}
}

func TestItemCommandDefaultAmount(t *testing.T) {
	reg := testRegistry(t, adminRights)
	p, session := testPlayer()
	if !reg.Execute(p, "item 882") {
		t.Fatal("Execute(item 882) returned false")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Spawned item 882 x1." {
		t.Fatalf("messages = %q; want [\"Spawned item 882 x1.\"]", session.gameMessages)
	}
	if got := inventoryCounts(p); got[882] != 1 || len(got) != 1 {
		t.Fatalf("inventory = %v; want {882: 1}", got)
	}
}

func TestItemCommandExplicitAmount(t *testing.T) {
	reg := testRegistry(t, adminRights)
	p, session := testPlayer()
	if !reg.Execute(p, "item 882 5") {
		t.Fatal("Execute(item 882 5) returned false")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Spawned item 882 x5." {
		t.Fatalf("messages = %q; want [\"Spawned item 882 x5.\"]", session.gameMessages)
	}
	// Bronze arrows are not stackable: one per slot, five slots.
	if got := inventorySlots(p); len(got) != 5 {
		t.Fatalf("occupied slots = %v; want 5 bronze-arrow slots", got)
	}
	if got := inventoryCounts(p); got[882] != 5 {
		t.Fatalf("inventory = %v; want 5 bronze arrows", got)
	}
}

func TestItemCommandStacksCoins(t *testing.T) {
	reg := testRegistry(t, adminRights)
	p, session := testPlayer()
	if !reg.Execute(p, "item 995 1000") {
		t.Fatal("Execute(item 995 1000) returned false")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Spawned item 995 x1000." {
		t.Fatalf("messages = %q; want [\"Spawned item 995 x1000.\"]", session.gameMessages)
	}
	slots := inventorySlots(p)
	if len(slots) != 1 || slots[0] != 995 {
		t.Fatalf("occupied slots = %v; want one coin slot", slots)
	}
	if got := inventoryCounts(p); got[995] != 1000 {
		t.Fatalf("inventory = %v; want 1000 coins in one stack", got)
	}
}

func TestItemCommandUsage(t *testing.T) {
	cases := []string{
		"item",         // missing id
		"item abc",     // unparsable id
		"item 882 abc", // unparsable amount
		"item 882 0",   // amount <= 0
		"item 882 -5",  // amount <= 0
		"item -1",      // id < 0
		"item -1 5",    // id < 0
		"item 882 -0",  // amount 0
	}
	for _, raw := range cases {
		reg := testRegistry(t, adminRights)
		p, session := testPlayer()
		if !reg.Execute(p, raw) {
			t.Fatalf("Execute(%q) returned false", raw)
		}
		if len(session.gameMessages) != 1 || session.gameMessages[0] != "Usage: ::item id [amount]" {
			t.Fatalf("Execute(%q) messages = %q; want [\"Usage: ::item id [amount]\"]", raw, session.gameMessages)
		}
		if got := inventoryCounts(p); len(got) != 0 {
			t.Fatalf("Execute(%q) inventory = %v; want empty", raw, got)
		}
	}
}

func TestItemCommandExtraPartsIgnored(t *testing.T) {
	reg := testRegistry(t, adminRights)
	p, session := testPlayer()
	if !reg.Execute(p, "item 882 5 extra junk") {
		t.Fatal("Execute(item 882 5 extra junk) returned false")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Spawned item 882 x5." {
		t.Fatalf("messages = %q; want [\"Spawned item 882 x5.\"]", session.gameMessages)
	}
	if got := inventoryCounts(p); got[882] != 5 {
		t.Fatalf("inventory = %v; want 5 bronze arrows", got)
	}
}

func TestItemCommandCapsAmount(t *testing.T) {
	reg := testRegistry(t, adminRights)
	p, session := testPlayer()
	if !reg.Execute(p, "item 995 99999999999999999999") {
		t.Fatal("Execute(cap amount) returned false")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Spawned item 995 x9007199254740991." {
		t.Fatalf("messages = %q; want [\"Spawned item 995 x9007199254740991.\"]", session.gameMessages)
	}
	// The message names the capped amount; the stack itself is capped
	// at MaxStackAmount by the container's add, exactly as in TS.
	if got := inventoryCounts(p); got[995] != game.MaxStackAmount {
		t.Fatalf("inventory = %v; want coins capped at MaxStackAmount %d", got, game.MaxStackAmount)
	}
}

func TestItemCommandUnknownID(t *testing.T) {
	reg := testRegistry(t, adminRights)
	p, session := testPlayer()
	if !reg.Execute(p, "item 999999 3") {
		t.Fatal("Execute(item 999999 3) returned false")
	}
	// No definition check in this command: the message prints and the
	// items land one per slot (unknown ids are non-stackable).
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Spawned item 999999 x3." {
		t.Fatalf("messages = %q; want [\"Spawned item 999999 x3.\"]", session.gameMessages)
	}
	if got := inventorySlots(p); len(got) != 3 || got[0] != 999999 || got[1] != 999999 || got[2] != 999999 {
		t.Fatalf("occupied slots = %v; want three slots of 999999", got)
	}
}

func TestItemCommandFullInventory(t *testing.T) {
	reg := testRegistry(t, adminRights)
	p, session := testPlayer()
	p.Inventory().Adds(882, 28)
	session.gameMessages = nil
	if !reg.Execute(p, "item 882") {
		t.Fatal("Execute(item 882) returned false")
	}
	// TS prints both lines: the container's couldn't-hold line, then
	// the handler's spawned line, in that order.
	want := []string{"You couldn't hold all those items.", "Spawned item 882 x1."}
	if len(session.gameMessages) != len(want) {
		t.Fatalf("messages = %q; want %q", session.gameMessages, want)
	}
	for i, msg := range want {
		if session.gameMessages[i] != msg {
			t.Fatalf("messages = %q; want %q", session.gameMessages, want)
		}
	}
	if got := inventoryCounts(p); got[882] != 28 {
		t.Fatalf("inventory = %v; want the original 28 arrows", got)
	}
}

func TestItemCommandRightsGate(t *testing.T) {
	// A player with no rights is denied with the registry's TS
	// denial text and gets nothing.
	reg := testRegistry(t, noRights)
	p, session := testPlayer()
	if !reg.Execute(p, "item 882") {
		t.Fatal("Execute(item 882) returned false for a non-admin")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "You do not have permission to use this command." {
		t.Fatalf("messages = %q; want the denial line", session.gameMessages)
	}
	if got := inventoryCounts(p); len(got) != 0 {
		t.Fatalf("inventory = %v; want empty for a denied command", got)
	}
}

func TestItemCommandRegistered(t *testing.T) {
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{
		Logger: log.New(io.Discard, "", 0),
	})
	hub.Load()
	reg := hub.Commands()
	if reg == nil {
		t.Fatal("hub has no command registry after Load")
	}
	admin, _ := testPlayer()
	plain, _ := testPlayer()
	reg.SetRightsSource(func(p *game.Player) int {
		if p == admin {
			return game.RightsAdministrator
		}
		return game.RightsNone
	})
	found := false
	for _, cmd := range reg.RegisteredCommands(admin) {
		if cmd.Command == "item" {
			found = true
			if cmd.Description != "Spawn an item" {
				t.Fatalf("item description = %q; want \"Spawn an item\"", cmd.Description)
			}
		}
	}
	if !found {
		t.Fatalf("RegisteredCommands(admin) = %v; want item listed", reg.RegisteredCommands(admin))
	}
	// The rights filter hides it from a player with no rights.
	for _, cmd := range reg.RegisteredCommands(plain) {
		if cmd.Command == "item" {
			t.Fatalf("RegisteredCommands(non-admin) = %v; want item hidden", reg.RegisteredCommands(plain))
		}
	}
}
