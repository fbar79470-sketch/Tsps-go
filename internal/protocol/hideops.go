package protocol

// EncodeHideNpcOps encodes the HIDE_NPC_OPS packet (opcode 148, fixed
// length 1): one boolean byte, 1 when hidden (encodeHideNpcOps,
// ClientProtocol.ts lines 1770-1772). While hidden, NPC menus keep
// Examine but drop Use, Cast, every action and Attack. flushClient sends
// hidden=false on the initial sync so a world switch cannot inherit the
// previous world's cutscene state (PlayerSession.ts:461).
func EncodeHideNpcOps(hidden bool) []byte {
	payload := []byte{0}
	if hidden {
		payload[0] = 1
	}
	return mustEncode(ServerHideNpcOps, payload)
}

// EncodeHideObjOps encodes the HIDE_OBJ_OPS packet (opcode 149, fixed
// length 1): one boolean byte, 1 when hidden (encodeHideObjOps,
// ClientProtocol.ts lines 1779-1781). While hidden, ground-item menus
// drop Use, Cast and every ground action (including the Take fallback);
// Examine stays only when no spell is selected. flushClient sends
// hidden=false on the initial sync, next to the NPC packet
// (PlayerSession.ts:462).
func EncodeHideObjOps(hidden bool) []byte {
	payload := []byte{0}
	if hidden {
		payload[0] = 1
	}
	return mustEncode(ServerHideObjOps, payload)
}
