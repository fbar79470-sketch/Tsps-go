package listeners

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// chatPayload builds a CHAT packet payload: the kind byte, the
// latin1 text plus NUL terminator, and optional trailing style
// bytes. All fixture text is ASCII, so latin1 and UTF-8 agree.
func chatPayload(kind byte, text string, style ...byte) []byte {
	payload := []byte{kind}
	payload = append(payload, []byte(text)...)
	payload = append(payload, 0)
	return append(payload, style...)
}

// fakeDelivery records broadcast copies: the recipients, in
// delivery order, and the message each received.
type fakeDelivery struct {
	recipients []*game.Player
	messages   []PublicChatMessage
}

func (d *fakeDelivery) DeliverPublicChat(recipient *game.Player, msg PublicChatMessage) {
	d.recipients = append(d.recipients, recipient)
	d.messages = append(d.messages, msg)
}

func (d *fakeDelivery) names() []string {
	names := make([]string, len(d.recipients))
	for i, r := range d.recipients {
		names[i] = r.Username
	}
	return names
}

// capturingSession is a SessionPort + ChatMessageSender test
// double that records the game messages Player.SendMessage sends
// it, so refusal-line tests can assert the exact text without a
// real session.
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

// fakeCommands records command dispatches: the player and the raw
// command text each call received.
type fakeCommands struct {
	players []*game.Player
	raws    []string
}

func (c *fakeCommands) DispatchCommand(player *game.Player, rawCommand string) {
	c.players = append(c.players, player)
	c.raws = append(c.raws, rawCommand)
}

// chatWorld builds a world with three registered players: a
// speaker at the spawn, a near player 2 tiles away (inside the
// default 15-tile view distance), and a far player 100 tiles away
// (outside both the view distance and the bucket radius). Buckets
// are rebuilt and every player's local list is updated, as the
// update_players_npcs phase does, so the speaker's local list
// holds the near player only.
func chatWorld(t *testing.T) (*game.World, *game.Player, *game.Player, *game.Player) {
	t.Helper()
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	w := game.NewWorld(spawn)
	add := func(name string, loc game.Location) *game.Player {
		p := game.NewPlayer(name, loc)
		if _, ok := w.Players().Add(p); !ok {
			t.Fatalf("add of player %s failed", name)
		}
		return p
	}
	speaker := add("speaker", spawn)
	near := add("near", game.Location{X: 3091, Y: 3524, Z: 0})
	far := add("far", game.Location{X: 3189, Y: 3524, Z: 0})
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(speaker)
	w.UpdateLocalPlayers(near)
	w.UpdateLocalPlayers(far)
	return w, speaker, near, far
}

func forcedChatText(p *game.Player) string {
	if got := p.GetForcedChat(); got != nil {
		return *got
	}
	return ""
}

func TestNormalizeChatText(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello", "hello"},
		{"  hello  ", "hello"},
		{"<hello>", "hello"},
		{"<<::x>>", "::x"},
		{"a<b>c", "abc"},
		{"<>", ""},
		{"", ""},
		{"   ", ""},
		{"\u00a0hi\u00a0", "hi"}, // NBSP is JS-trim whitespace.
		{"\t\n hi \r\n", "hi"},
		{strings.Repeat("a", 80), strings.Repeat("a", 80)},
		{strings.Repeat("a", 81), strings.Repeat("a", 80)},
		{strings.Repeat("a", 200), strings.Repeat("a", 80)},
		// The slice applies after the strip and trim: brackets
		// and spaces do not consume the 80 characters.
		{"  <" + strings.Repeat("b", 81) + ">  ", strings.Repeat("b", 80)},
	}
	for _, tc := range cases {
		if got := normalizeChatText(tc.in); got != tc.want {
			t.Errorf("normalizeChatText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestChatIconPrefix(t *testing.T) {
	if got := chatIconPrefix(nil); got != "" {
		t.Errorf("chatIconPrefix(nil) = %q, want empty", got)
	}
	if got := chatIconPrefix([]int{}); got != "" {
		t.Errorf("chatIconPrefix([]) = %q, want empty", got)
	}
	if got := chatIconPrefix([]int{1}); got != "<img=1>" {
		t.Errorf("chatIconPrefix([1]) = %q, want <img=1>", got)
	}
	if got := chatIconPrefix([]int{1, 2, 255}); got != "<img=1><img=2><img=255>" {
		t.Errorf("chatIconPrefix([1 2 255]) = %q", got)
	}
}

func TestBlockedWord(t *testing.T) {
	// Every entry of Misc.BLOCKED_WORDS, as a substring.
	for _, word := range blockedWords {
		if !blockedWord("see " + word + " here") {
			t.Errorf("blockedWord(%q): false, want true", word)
		}
	}
	for _, text := range []string{"hello", "com", "org", ".co", "trade", "@crx"} {
		// "@crx" contains "@cr" and IS blocked; it is listed
		// separately below. The rest must pass.
		if text == "@crx" {
			continue
		}
		if blockedWord(text) {
			t.Errorf("blockedWord(%q): true, want false", text)
		}
	}
	// Substring matching is case-sensitive (TS String.includes).
	if blockedWord("visit EVIL.COM") {
		t.Error("blockedWord upper-case .COM: true, want false")
	}
	// "@cr" matches anywhere, including inside a longer token.
	if !blockedWord("@crx") {
		t.Error("blockedWord(@crx): false, want true")
	}
}

func TestChatTypeFor(t *testing.T) {
	if got := chatTypeFor(false, false); got != nil {
		t.Errorf("ordinary chat type = %d, want nil", *got)
	}
	if got := chatTypeFor(false, true); got != nil {
		t.Errorf("ordinary chat with icons type = %d, want nil", *got)
	}
	if got := chatTypeFor(true, false); got == nil || *got != protocol.ChatTypeAutotyper {
		t.Errorf("autochat type = %v, want 90", got)
	}
	if got := chatTypeFor(true, true); got == nil || *got != protocol.ChatTypeModAutotyper {
		t.Errorf("autochat with icons type = %v, want 91", got)
	}
}

func TestHandleChatBroadcast(t *testing.T) {
	w, speaker, near, far := chatWorld(t)
	delivery := &fakeDelivery{}
	deps := Deps{Chat: ChatHooks{
		Delivery:      delivery,
		NearbyPlayers: w.NearbyPlayersForUpdate,
	}}
	got := Handle(speaker, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hello")}, 7, deps)
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("Handle = %+v, want implemented", got)
	}
	// Overhead text: set on the speaker only, with the flag.
	if forcedChatText(speaker) != "hello" {
		t.Fatalf("speaker forced chat = %q, want \"hello\"", forcedChatText(speaker))
	}
	if !speaker.Flags.Has(game.FlagForcedChat) {
		t.Fatal("speaker FlagForcedChat unset")
	}
	if forcedChatText(near) != "" || forcedChatText(far) != "" {
		t.Fatal("forceChat set on a non-speaker")
	}
	// Broadcast: speaker first, then the near player, exactly
	// once each (the near player is in both the local list and
	// the nearby supplement; dedup by index collapses them).
	// The far player receives nothing.
	if gotNames, want := delivery.names(), []string{"speaker", "near"}; !equalStrings(gotNames, want) {
		t.Fatalf("recipients = %v, want %v", gotNames, want)
	}
	msg := delivery.messages[0]
	if msg.Text != "hello" || msg.From != "speaker" || msg.SpeakerIndex != speaker.Index {
		t.Fatalf("message = %+v, want text hello from speaker index %d", msg, speaker.Index)
	}
	if msg.AutoChat || msg.ChatType != nil || msg.Style != nil {
		t.Fatalf("message = %+v, want no autochat/type/style", msg)
	}
	if delivery.messages[1] != msg {
		t.Fatalf("second copy = %+v, want identical to first %+v", delivery.messages[1], msg)
	}
}

func TestHandleChatBroadcastLocalListOnly(t *testing.T) {
	// With no NearbyPlayers hook the supplement term is empty:
	// the same recipients arrive through the local list alone.
	_, speaker, _, far := chatWorld(t)
	delivery := &fakeDelivery{}
	got := Handle(speaker, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hi")}, 7, Deps{Chat: ChatHooks{Delivery: delivery}})
	if !got.Implemented {
		t.Fatalf("Handle = %+v, want implemented", got)
	}
	if gotNames, want := delivery.names(), []string{"speaker", "near"}; !equalStrings(gotNames, want) {
		t.Fatalf("recipients = %v, want %v", gotNames, want)
	}
	if forcedChatText(far) != "" {
		t.Fatal("forceChat set on the far player")
	}
}

func TestHandleChatNoDeliveryStillForceChats(t *testing.T) {
	// With no Delivery wired, the overhead text is still set;
	// only the chatbox frames are missing (the unwired-seam
	// degradation). The outcome is still Implemented: state
	// changed.
	p := testPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hello")}, 7, Deps{})
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("Handle = %+v, want implemented", got)
	}
	if forcedChatText(p) != "hello" {
		t.Fatalf("forced chat = %q, want \"hello\"", forcedChatText(p))
	}
}

func TestHandleChatDisplayName(t *testing.T) {
	p := testPlayer()
	p.DisplayName = "Chatty"
	delivery := &fakeDelivery{}
	Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hi")}, 7, Deps{Chat: ChatHooks{Delivery: delivery}})
	if len(delivery.messages) != 1 || delivery.messages[0].From != "Chatty" {
		t.Fatalf("messages = %+v, want one copy from Chatty", delivery.messages)
	}
}

func TestHandleChatStylePassthrough(t *testing.T) {
	p := testPlayer()
	delivery := &fakeDelivery{}
	Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hi", 2, 1)}, 7, Deps{Chat: ChatHooks{Delivery: delivery}})
	if len(delivery.messages) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(delivery.messages))
	}
	style := delivery.messages[0].Style
	if style == nil || style.Colour != 2 || style.Effect != 1 || style.Pattern != nil {
		t.Fatalf("style = %+v, want colour 2 effect 1", style)
	}
	// A pattern style passes through whole.
	delivery = &fakeDelivery{}
	Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hi", 13, 0, 7)}, 7, Deps{Chat: ChatHooks{Delivery: delivery}})
	style = delivery.messages[0].Style
	if style == nil || style.Colour != 13 || len(style.Pattern) != 1 || style.Pattern[0] != 7 {
		t.Fatalf("pattern style = %+v, want colour 13 pattern [7]", style)
	}
	// An invalid style decodes to nil and stays nil.
	delivery = &fakeDelivery{}
	Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hi", 21, 0)}, 7, Deps{Chat: ChatHooks{Delivery: delivery}})
	if delivery.messages[0].Style != nil {
		t.Fatalf("style = %+v, want nil", delivery.messages[0].Style)
	}
}

func TestHandleChatAutoChat(t *testing.T) {
	w, speaker, near, _ := chatWorld(t)
	delivery := &fakeDelivery{}
	got := Handle(speaker, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(3, "buying gf")}, 7, Deps{Chat: ChatHooks{
		Delivery:      delivery,
		NearbyPlayers: w.NearbyPlayersForUpdate,
	}})
	if !got.Implemented {
		t.Fatalf("Handle = %+v, want implemented", got)
	}
	// The mode-0 relations gate rejects autochat for every other
	// recipient: only the speaker receives their own line, typed
	// AUTOTYPER (90). The overhead text is still set.
	if gotNames, want := delivery.names(), []string{"speaker"}; !equalStrings(gotNames, want) {
		t.Fatalf("recipients = %v, want %v", gotNames, want)
	}
	msg := delivery.messages[0]
	if !msg.AutoChat || msg.ChatType == nil || *msg.ChatType != protocol.ChatTypeAutotyper {
		t.Fatalf("message = %+v, want autochat type 90", msg)
	}
	if forcedChatText(speaker) != "buying gf" {
		t.Fatalf("forced chat = %q, want the autochat text", forcedChatText(speaker))
	}
	if forcedChatText(near) != "" {
		t.Fatal("forceChat set on the near player")
	}
}

func TestHandleChatCommandDispatch(t *testing.T) {
	p := testPlayer()
	commands := &fakeCommands{}
	deps := Deps{Chat: ChatHooks{Commands: commands}}
	cases := []struct {
		payload string
		wantRaw string
	}{
		{"::players", "players"},
		{"::", ""},
		{"  ::x", "x"},
		{"<::x>", "x"},
		{"<<::x>>", "x"},
		// Interior spacing survives; leading/trailing whitespace
		// was already removed by normalization, before the
		// prefix strip (the dispatcher's own sanitize then
		// runs on the remainder).
		{"::item  1351 ", "item  1351"},
	}
	for _, tc := range cases {
		commands.players, commands.raws = nil, nil
		got := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, tc.payload)}, 7, deps)
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Errorf("Handle(%q) = %+v, want implemented", tc.payload, got)
		}
		if len(commands.raws) != 1 || commands.raws[0] != tc.wantRaw {
			t.Errorf("Handle(%q): raws = %q, want [%q]", tc.payload, commands.raws, tc.wantRaw)
		}
		if len(commands.players) == 1 && commands.players[0] != p {
			t.Errorf("Handle(%q): dispatched to the wrong player", tc.payload)
		}
		if forcedChatText(p) != "" {
			t.Errorf("Handle(%q): forced chat set to %q, want none", tc.payload, forcedChatText(p))
		}
		p.ResetUpdating()
	}
	// Non-command text never reaches the dispatcher.
	commands.raws = nil
	Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hello")}, 7, deps)
	if len(commands.raws) != 0 {
		t.Fatalf("plain chat dispatched commands: %q", commands.raws)
	}
	Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, ":players")}, 7, deps)
	Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "x::y")}, 7, deps)
	if len(commands.raws) != 0 {
		t.Fatalf("non-prefix text dispatched commands: %q", commands.raws)
	}
}

func TestHandleChatCommandDropWithoutDispatcher(t *testing.T) {
	p := testPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "::players")}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonCommand}) {
		t.Fatalf("Handle = %+v, want ReasonCommand drop", got)
	}
	if forcedChatText(p) != "" {
		t.Fatal("forced chat set for a dropped command")
	}
}

func TestHandleChatCommandBeforeMute(t *testing.T) {
	// The "::" check runs BEFORE allowChat: a muted player can
	// still run commands, and no refusal line is sent.
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	commands := &fakeCommands{}
	deps := Deps{Chat: ChatHooks{
		Commands: commands,
		Muted:    func(username, host string) bool { return true },
	}}
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "::x")}, 7, deps)
	if !got.Implemented || len(commands.raws) != 1 {
		t.Fatalf("Handle = %+v, raws = %q, want a dispatch", got, commands.raws)
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}
}

func TestHandleChatMuted(t *testing.T) {
	p := testPlayer()
	p.RemoteAddr = "1.2.3.4"
	session := &capturingSession{}
	p.Session = session
	var seenUser, seenHost string
	delivery := &fakeDelivery{}
	deps := Deps{Chat: ChatHooks{
		Delivery: delivery,
		Muted: func(username, host string) bool {
			seenUser, seenHost = username, host
			return true
		},
	}}
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hello")}, 7, deps)
	if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
		t.Fatalf("Handle = %+v, want ReasonNoAction", got)
	}
	if seenUser != "testplayer" || seenHost != "1.2.3.4" {
		t.Fatalf("Muted called with (%q, %q), want (testplayer, 1.2.3.4)", seenUser, seenHost)
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "You are muted and cannot chat." {
		t.Fatalf("messages = %q, want the mute line", session.gameMessages)
	}
	if forcedChatText(p) != "" {
		t.Fatal("forced chat set for a muted player")
	}
	if len(delivery.recipients) != 0 {
		t.Fatalf("deliveries = %d, want 0", len(delivery.recipients))
	}
}

func TestHandleChatMutedWithoutSession(t *testing.T) {
	// With no session attached, Player.SendMessage drops the
	// refusal text silently; the gate still refuses.
	p := testPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hello")}, 7, Deps{Chat: ChatHooks{
		Muted: func(username, host string) bool { return true },
	}})
	if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
		t.Fatalf("Handle = %+v, want ReasonNoAction", got)
	}
	if forcedChatText(p) != "" {
		t.Fatal("forced chat set for a muted player")
	}
}

func TestHandleChatBlockedWord(t *testing.T) {
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	delivery := &fakeDelivery{}
	deps := Deps{Chat: ChatHooks{Delivery: delivery}}
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "visit evil.com")}, 7, deps)
	if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
		t.Fatalf("Handle = %+v, want ReasonNoAction", got)
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Your message did not make it past the filter." {
		t.Fatalf("messages = %q, want the filter line", session.gameMessages)
	}
	if forcedChatText(p) != "" || len(delivery.recipients) != 0 {
		t.Fatal("blocked text produced chat state or deliveries")
	}
	// The "<"-bearing blocked words can never match: normalization
	// strips angle brackets before the check runs.
	p.ResetUpdating()
	session.gameMessages = nil
	got = Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "<col=red>hi")}, 7, deps)
	if !got.Implemented {
		t.Fatalf("Handle = %+v, want implemented", got)
	}
	if forcedChatText(p) != "col=redhi" {
		t.Fatalf("forced chat = %q, want the stripped text", forcedChatText(p))
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}
}

func TestHandleChatEmptyText(t *testing.T) {
	p := testPlayer()
	for _, text := range []string{"", "   ", "<>", "  < >  "} {
		got := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, text)}, 7, Deps{})
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Errorf("Handle(%q) = %+v, want ReasonNoAction", text, got)
		}
		if forcedChatText(p) != "" {
			t.Errorf("Handle(%q): forced chat set", text)
		}
		p.ResetUpdating()
	}
}

func TestHandleChatSocialIntercept(t *testing.T) {
	p := testPlayer()
	delivery := &fakeDelivery{}
	var seenText string
	var seenAuto bool
	intercept := true
	deps := Deps{Chat: ChatHooks{
		Delivery: delivery,
		SocialIntercept: func(player *game.Player, text string, autoChat bool) bool {
			seenText, seenAuto = text, autoChat
			return intercept
		},
	}}
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hello")}, 7, deps)
	if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
		t.Fatalf("Handle = %+v, want ReasonNoAction", got)
	}
	if seenText != "hello" || seenAuto {
		t.Fatalf("intercept saw (%q, %v), want (hello, false)", seenText, seenAuto)
	}
	// An intercepted line sets neither forceChat nor deliveries:
	// the check runs before both.
	if forcedChatText(p) != "" || len(delivery.recipients) != 0 {
		t.Fatal("intercepted line produced chat state or deliveries")
	}
	// Unintercepted lines proceed normally.
	intercept = false
	got = Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(3, "auto")}, 7, deps)
	if !got.Implemented || seenText != "auto" || !seenAuto {
		t.Fatalf("Handle = %+v, intercept saw (%q, %v)", got, seenText, seenAuto)
	}
	if forcedChatText(p) != "auto" {
		t.Fatalf("forced chat = %q, want \"auto\"", forcedChatText(p))
	}
}

func TestHandleChatNearbySupplement(t *testing.T) {
	// Asymmetric local lists: the recipient renders the speaker
	// but the speaker does not render the recipient (the
	// 255-player-cap case). The supplement term includes the
	// recipient; a nearby player whose list does NOT include
	// the speaker stays excluded.
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	w := game.NewWorld(spawn)
	add := func(name string, loc game.Location) *game.Player {
		p := game.NewPlayer(name, loc)
		if _, ok := w.Players().Add(p); !ok {
			t.Fatalf("add of player %s failed", name)
		}
		return p
	}
	speaker := add("speaker", spawn)
	sees := add("sees", game.Location{X: 3090, Y: 3524, Z: 0})
	blind := add("blind", game.Location{X: 3091, Y: 3524, Z: 0})
	w.RebuildUpdateBuckets()
	// Only the "sees" player's list is updated: it renders the
	// speaker. The speaker's list stays empty (cap asymmetry),
	// and the "blind" player's list stays empty too.
	w.UpdateLocalPlayers(sees)
	if len(speaker.LocalPlayers()) != 0 {
		t.Fatalf("speaker locals = %d, want 0", len(speaker.LocalPlayers()))
	}
	if len(blind.LocalPlayers()) != 0 {
		t.Fatalf("blind locals = %d, want 0 (never updated)", len(blind.LocalPlayers()))
	}
	delivery := &fakeDelivery{}
	got := Handle(speaker, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hello")}, 7, Deps{Chat: ChatHooks{
		Delivery:      delivery,
		NearbyPlayers: w.NearbyPlayersForUpdate,
	}})
	if !got.Implemented {
		t.Fatalf("Handle = %+v, want implemented", got)
	}
	if gotNames, want := delivery.names(), []string{"speaker", "sees"}; !equalStrings(gotNames, want) {
		t.Fatalf("recipients = %v, want %v", gotNames, want)
	}
}

func TestHandleChatNearbyNilCandidatesSkipped(t *testing.T) {
	// A nil candidate in the nearby list is skipped, as TS skips
	// nil recipients in the send loop; delivery to real
	// recipients proceeds.
	_, speaker, near, _ := chatWorld(t)
	delivery := &fakeDelivery{}
	got := Handle(speaker, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hi")}, 7, Deps{Chat: ChatHooks{
		Delivery:      delivery,
		NearbyPlayers: func(speaker *game.Player) []*game.Player { return []*game.Player{nil, near, nil} },
	}})
	if !got.Implemented {
		t.Fatalf("Handle = %+v, want implemented", got)
	}
	if gotNames, want := delivery.names(), []string{"speaker", "near"}; !equalStrings(gotNames, want) {
		t.Fatalf("recipients = %v, want %v", gotNames, want)
	}
}

func TestHandleChatNilPlayer(t *testing.T) {
	got := Handle(nil, game.TurnPacket{Opcode: protocol.ClientChat, Payload: chatPayload(0, "hello")}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonNoPlayer}) {
		t.Fatalf("nil player public chat = %+v, want ReasonNoPlayer", got)
	}
	// Clan and friends-chat kinds drop with ReasonNoPlayer
	// after decode, as the `if (this.player)` guard requires:
	// the emit never runs without a player. Kind 4 carries a
	// clan-type byte before the text.
	got = Handle(nil, game.TurnPacket{Opcode: protocol.ClientChat, Payload: []byte{4, 0, 'c', 0}}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonNoPlayer}) {
		t.Fatalf("nil player clan chat = %+v, want ReasonNoPlayer", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
