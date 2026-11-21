package protocol

import (
	"encoding/binary"
	"fmt"
)

// Trade encoders and the trade-action decoder: encodeTradeRequest /
// encodeTradeOpen / encodeTradeUpdate / encodeTradeClose and the
// TradePartyView payload (ClientProtocol.ts:2113-2151), plus the
// TRADE_ACTION decode branch (ClientProtocol.ts:973-978, opcode
// 180). Trading (game/content/Trading.ts) is the only producer of
// the outbound packets and the only consumer of the inbound one.
//
// Wire shapes, all through encodeServerPacket (ClientProtocol.ts:
// 1053-1071): TRADE_REQUEST (154) and TRADE_CLOSE (157) carry a
// 1-byte length prefix (-1 in the length table); TRADE_OPEN (155)
// and TRADE_UPDATE (156) carry a 2-byte length prefix (-2).
// Strings are the latin1 NUL-terminated string() helper
// (ClientProtocol.ts:1096-1098), shared here as latin1Bytes.

// TradeStage is the trade window stage encodeTrade carries
// (ClientProtocol.ts:2137-2139): "offer" encodes as byte 0, anything
// else ("confirm") as byte 1.
type TradeStage int

const (
	// TradeStageOffer is the offer stage (byte 0).
	TradeStageOffer TradeStage = iota
	// TradeStageConfirm is the confirm stage (byte 1).
	TradeStageConfirm
)

// TradeOfferView is one offered stack in a TradePartyView
// (ClientProtocol.ts:2113): the offer-container slot, the item id,
// and the quantity.
type TradeOfferView struct {
	Slot     int
	ItemID   int
	Quantity int
}

// TradePartyView is one side of a trade window
// (ClientProtocol.ts:2113): the player's index and display name,
// whether each stage is accepted, and the offered stacks in slot
// order. A PlayerID of -1 and an empty Name are the TS absent
// defaults (playerId ?? -1, name ?? "").
type TradePartyView struct {
	PlayerID        int
	Name            string
	Accepted        bool
	ConfirmAccepted bool
	Offers          []TradeOfferView
}

// encodeTradeParty renders one party (ClientProtocol.ts:2115-2127):
// the player id as a signed 16-bit value, the latin1 name, the
// accepted byte, the confirm-accepted byte, the offer count byte,
// then one 8-byte offer payload per stack (slot u16, item id u16,
// quantity int32). The header is split around the name exactly as
// TS splits it (header.subarray(0, 2), string(name),
// header.subarray(2)).
func encodeTradeParty(party TradePartyView) []byte {
	payload := make([]byte, 0, 5+len(party.Name)+1+len(party.Offers)*8)
	id := make([]byte, 2)
	binary.BigEndian.PutUint16(id, uint16(int16(party.PlayerID)))
	payload = append(payload, id...)
	payload = append(payload, latin1Bytes(party.Name)...)
	if party.Accepted {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}
	if party.ConfirmAccepted {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}
	payload = append(payload, byte(len(party.Offers)))
	for _, offer := range party.Offers {
		slot := make([]byte, 8)
		binary.BigEndian.PutUint16(slot[0:2], uint16(offer.Slot&0xffff))
		binary.BigEndian.PutUint16(slot[2:4], uint16(offer.ItemID&0xffff))
		binary.BigEndian.PutUint32(slot[4:8], uint32(offer.Quantity))
		payload = append(payload, slot...)
	}
	return payload
}

// EncodeTradeRequest encodes a trade-request notice
// (encodeTradeRequest, ClientProtocol.ts:2131-2135): TRADE_REQUEST
// (opcode 154), the requester's index as u16 followed by the
// latin1 display name. The client shows "<name> wishes to trade
// with you." and makes it clickable; the companion chat line is a
// separate CHAT_MESSAGE packet (see the game-layer TradeSender).
func EncodeTradeRequest(playerID int, name string) []byte {
	payload := make([]byte, 2)
	binary.BigEndian.PutUint16(payload, uint16(playerID&0xffff))
	payload = append(payload, latin1Bytes(name)...)
	return mustEncode(ServerTradeRequest, payload)
}

// encodeTrade renders a TRADE_OPEN or TRADE_UPDATE packet
// (ClientProtocol.ts:2137-2142): the session id string, the stage
// byte, the info string, then the two party views (self first).
func encodeTrade(opcode byte, sessionID string, stage TradeStage, self, other TradePartyView, info string) []byte {
	payload := append([]byte(nil), latin1Bytes(sessionID)...)
	if stage == TradeStageOffer {
		payload = append(payload, 0)
	} else {
		payload = append(payload, 1)
	}
	payload = append(payload, latin1Bytes(info)...)
	payload = append(payload, encodeTradeParty(self)...)
	payload = append(payload, encodeTradeParty(other)...)
	return mustEncode(opcode, payload)
}

// EncodeTradeOpen encodes a trade-window open
// (encodeTradeOpen, ClientProtocol.ts:2144-2146): TRADE_OPEN
// (opcode 155). info defaults to "" in TS; callers pass "".
func EncodeTradeOpen(sessionID string, stage TradeStage, self, other TradePartyView, info string) []byte {
	return encodeTrade(ServerTradeOpen, sessionID, stage, self, other, info)
}

// EncodeTradeUpdate encodes a trade-window state refresh
// (encodeTradeUpdate, ClientProtocol.ts:2148-2150): TRADE_UPDATE
// (opcode 156). info defaults to "" in TS; callers pass "".
func EncodeTradeUpdate(sessionID string, stage TradeStage, self, other TradePartyView, info string) []byte {
	return encodeTrade(ServerTradeUpdate, sessionID, stage, self, other, info)
}

// EncodeTradeClose encodes a trade-window close
// (encodeTradeClose, ClientProtocol.ts:2152-2154): TRADE_CLOSE
// (opcode 157), the latin1 reason string ("" by default in TS).
func EncodeTradeClose(reason string) []byte {
	return mustEncode(ServerTradeClose, latin1Bytes(reason))
}

// TradeActionKind is the TRADE_ACTION action byte
// (ClientProtocol.ts:974): the index into ["offer", "remove",
// "accept", "decline", "confirm_accept", "confirm_decline"].
type TradeActionKind int

const (
	// TradeActionOffer moves items from the inventory into the
	// offer container.
	TradeActionOffer TradeActionKind = iota
	// TradeActionRemove moves items from the offer container
	// back into the inventory.
	TradeActionRemove
	// TradeActionAccept accepts the offer screen.
	TradeActionAccept
	// TradeActionDecline declines the offer screen.
	TradeActionDecline
	// TradeActionConfirmAccept accepts the confirm screen.
	TradeActionConfirmAccept
	// TradeActionConfirmDecline declines the confirm screen.
	TradeActionConfirmDecline
)

// TradeAction is a decoded TRADE_ACTION packet
// (ClientProtocol.ts:973-978): the action, the clicked slot, the
// quantity, and the item id when the packet carries one
// (unsignedItemId maps 0xffff to -1, which TS drops to
// undefined; HasItemID carries that distinction).
type TradeAction struct {
	Action    TradeActionKind
	Slot      int
	Quantity  int
	ItemID    int
	HasItemID bool
}

// DecodeTradeAction decodes a TRADE_ACTION payload (opcode 180,
// ClientProtocol.ts:973-978): the action byte, the slot short,
// the quantity int, and the item-id short. An action byte outside
// 0..5 is an error: TS returns a "raw" packet for it (the decode
// branch yields no trade_action), and no trade_action exists to
// dispatch, so the Go form is a decode failure, which the
// listener layer drops as malformed rather than routing.
func DecodeTradeAction(payload []byte) (TradeAction, error) {
	r := NewReader(payload)
	action, err := r.Byte()
	if err != nil {
		return TradeAction{}, err
	}
	if action > byte(TradeActionConfirmDecline) {
		return TradeAction{}, fmt.Errorf("protocol: unknown trade action %d", action)
	}
	slot, err := r.Short()
	if err != nil {
		return TradeAction{}, err
	}
	quantity, err := r.Int()
	if err != nil {
		return TradeAction{}, err
	}
	rawItemID, err := r.Short()
	if err != nil {
		return TradeAction{}, err
	}
	out := TradeAction{Action: TradeActionKind(action), Slot: int(slot), Quantity: int(quantity)}
	if id := unsignedItemID(rawItemID); id >= 0 {
		out.ItemID = id
		out.HasItemID = true
	}
	return out, nil
}
