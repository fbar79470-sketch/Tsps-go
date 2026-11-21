package protocol

import "encoding/binary"

// ShopSlotView is one stock row in a SHOP_OPEN packet
// (ShopSlotView, ClientProtocol.ts:2086): the display slot, the
// item id, the quantity on display, the shop's normal stock of
// it, and the per-unit buy and sell prices. TS leaves
// defaultQuantity/priceEach/sellPrice optional, defaulting to
// the quantity / 0 / 0 in encodeShopSlotPayload; the Go struct
// carries plain ints because the only production caller
// (ShopManager.openInterface) always supplies all three, so no
// behavior differs. Callers building a view by hand must set
// DefaultQuantity explicitly: it is written as given, never
// defaulted to Quantity here.
type ShopSlotView struct {
	Slot            int
	ItemID          int
	Quantity        int
	DefaultQuantity int
	PriceEach       int
	SellPrice       int
}

// encodeShopSlotPayload renders one stock row (encodeShopSlotPayload,
// ClientProtocol.ts:2088-2096): a fixed 20-byte payload — slot
// u16, item id u16, quantity i32, default quantity i32, price
// each i32, sell price i32, all big-endian. Slot and item id are
// masked to 16 bits and the 32-bit fields are truncated to int32,
// as TS masks with & 0xffff / | 0.
func encodeShopSlotPayload(slot ShopSlotView) []byte {
	payload := make([]byte, 20)
	binary.BigEndian.PutUint16(payload[0:2], uint16(slot.Slot&0xffff))
	binary.BigEndian.PutUint16(payload[2:4], uint16(slot.ItemID&0xffff))
	binary.BigEndian.PutUint32(payload[4:8], uint32(int32(slot.Quantity)))
	binary.BigEndian.PutUint32(payload[8:12], uint32(int32(slot.DefaultQuantity)))
	binary.BigEndian.PutUint32(payload[12:16], uint32(int32(slot.PriceEach)))
	binary.BigEndian.PutUint32(payload[16:20], uint32(int32(slot.SellPrice)))
	return payload
}

// EncodeShopOpen encodes a shop opening (encodeShopOpen,
// ClientProtocol.ts:2099-2107): SHOP_OPEN (opcode 150, var-short),
// carrying the latin1 NUL-terminated shop id and title, a 7-byte
// header (currency item id u16, general-store flag, buy mode,
// sell mode, stock count u16), and one 20-byte payload per stock
// row. The currency item id and stock count are masked to 16
// bits; the mode bytes are written as single bytes, as TS writes
// them into a Buffer (values outside 0-255 wrap). It panics when
// the framed payload would exceed 65535 bytes, mirroring the TS
// encodeServerPacket throw; only a stock of ~3,200 rows can
// reach that (shops cap at 1,000 rows).
func EncodeShopOpen(id, name string, currencyItemID int, general bool, buyMode, sellMode int, stock []ShopSlotView) []byte {
	payload := make([]byte, 0, len(id)+len(name)+9+20*len(stock))
	payload = append(payload, latin1Bytes(id)...)
	payload = append(payload, latin1Bytes(name)...)
	header := make([]byte, 7)
	binary.BigEndian.PutUint16(header[0:2], uint16(currencyItemID&0xffff))
	if general {
		header[2] = 1
	}
	header[3] = byte(buyMode)
	header[4] = byte(sellMode)
	binary.BigEndian.PutUint16(header[5:7], uint16(len(stock)&0xffff))
	payload = append(payload, header...)
	for _, slot := range stock {
		payload = append(payload, encodeShopSlotPayload(slot)...)
	}
	return mustEncode(ServerShopOpen, payload)
}

// EncodeShopClose encodes a shop closing (encodeShopClose,
// ClientProtocol.ts:2109-2111): SHOP_CLOSE (opcode 152) with an
// empty payload. The length-table entry for opcode 152 is 0, so
// mustEncode cannot fail.
func EncodeShopClose() []byte {
	return mustEncode(ServerShopClose, nil)
}
