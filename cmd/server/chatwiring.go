package main

import (
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
)

// Chat wiring: the broadcast-delivery and examine adapters the
// phase 12 listeners need from the rest of the server, plus the
// object-examine plugin-event adapter. The coordinator (buildServer
// in server.go) installs them on the world adapter; the chat
// listener and examine routes reach them through listeners.Deps.

// chatDelivery sends one broadcast copy of a public-chat line to
// one recipient: the Go form of
// recipient.getPacketSender().sendPublicChat(...)
// (PacketSender.ts:843-848: encodeChatMessage("public", message,
// from, "", playerId, chatType, style)). A recipient with no
// session attached receives nothing: no client is attached, and
// TS sends through the session likewise.
type chatDelivery struct{}

func (chatDelivery) DeliverPublicChat(recipient *game.Player, msg listeners.PublicChatMessage) {
	if recipient == nil || recipient.Session == nil {
		return
	}
	out := protocol.OutgoingChatMessage{
		Kind:     protocol.ChatKindPublic,
		Text:     msg.Text,
		From:     msg.From,
		PlayerID: msg.SpeakerIndex,
		Style:    msg.Style,
	}
	if msg.ChatType != nil {
		out.ChatType = *msg.ChatType
		out.HasChatType = true
	}
	recipient.Session.SendPacket(protocol.EncodeChatMessage(out))
}

var _ listeners.ChatDelivery = chatDelivery{}

// storeExamineSource implements listeners.ExamineSource over the
// definition Store. It is constructed only with a non-nil store
// (buildServer always builds one); the nil guards return the
// no-definition answers rather than panicking a fixture.
type storeExamineSource struct {
	store *definition.Store
}

var _ listeners.ExamineSource = storeExamineSource{}
var _ listeners.ExaminePriceSource = storeExamineSource{}

// NpcExamine is NpcDefinition.getExamine
// (NpcDefinition.ts:152-160) via Store.NPC, which returns the
// DEFAULT shape for unknown ids — the generic line, as TS forId
// falls back to DEFAULT.
func (s storeExamineSource) NpcExamine(id int) string {
	if s.store == nil {
		return "It's nothing special."
	}
	return s.store.NPC(id).Examine()
}

// ObjectExamine is the examine_object dispatch rule
// (NetworkBuilder.ts:1298-1306). When the id has no recorded
// examine text, the text is read from the variant the player sees
// (Store.ObjectForPlayer); the variant's text is used only when
// the variant has recorded text of its own, else the base
// definition's examine. An unknown id answers the generic line:
// TS forId yields undefined there and the `?.getExamine() ??
// "It's nothing special."` fallback applies. The variant
// resolution runs with a nil var reader (the varp store is
// per-session and the examine source has no per-player reader
// wired), so the default (last) transform is used — the
// documented Store.ObjectForPlayer degradation.
func (s storeExamineSource) ObjectExamine(id int) string {
	if s.store == nil {
		return "It's nothing special."
	}
	def, ok := s.store.Object(id)
	if !ok {
		return "It's nothing special."
	}
	var seen *definition.ObjectDefinition
	if _, hasText := s.store.ObjectExamineText(id); !hasText {
		if variant, ok := s.store.ObjectForPlayer(id, nil); ok {
			seen = variant
		}
	}
	if seen != nil {
		if text, hasText := s.store.ObjectExamineText(seen.ID()); hasText && text != "" {
			return seen.Examine()
		}
	}
	return def.Examine()
}

// ItemExamine is examineItem's text (ExaminePrice.ts:71-75): the
// item's examine, or its name when the examine is empty. The
// price line is a separate send through ExaminePriceValues
// below (sendExaminePrice, ExaminePrice.ts:55-69).
func (s storeExamineSource) ItemExamine(id int) string {
	if s.store == nil {
		return ""
	}
	def := s.store.Item(id)
	if examine := def.Examine(); examine != "" {
		return examine
	}
	return def.Name()
}

// ExaminePriceValues is the examine price line's value source
// (ExaminePrice.ts:42-55) over the definition Store: the
// un-noted name plus the GE/high-alch/low-alch prices, via
// game.ExaminePricesFor. A nil store reports not-ok. The line
// itself stays settings-gated (see game/examine_price.go): with
// no VarbitReader attached to the session, no line is sent —
// the accessors are wired, the settings gate is the remaining
// gap.
func (s storeExamineSource) ExaminePriceValues(itemID int) (string, game.ExaminePrices, bool) {
	if s.store == nil {
		return "", game.ExaminePrices{}, false
	}
	return game.ExaminePricesFor(itemID, storeItemDefinitions{store: s.store})
}

// objectExamineIntercept adapts the hub's world:object-examine
// custom event to the examine listener's intercept shape: one
// object id in, whether a plugin marked the event handled out.
// A nil hub yields an intercept that never intercepts (the
// no-hooks default).
func objectExamineIntercept(hub *plugins.Hub) func(*game.Player, int) bool {
	return func(p *game.Player, id int) bool {
		if hub == nil {
			return false
		}
		event := &plugins.ObjectExamineEvent{Player: p, ObjectID: id}
		hub.EmitCustomEvent("world:object-examine", event)
		return event.Handled
	}
}
