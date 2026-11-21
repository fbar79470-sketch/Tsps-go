package protocol

// Multi-combat icon: PacketSender.sendMultiIcon
// (PacketSender.ts:774-776). There is no dedicated
// multi-icon packet: the icon IS varbit 4605
// (VARBIT_MULTICOMBAT_AREA, PacketSender.ts:119), sent
// through sendVarbit, which sends encodeVarbit
// (ClientProtocol.ts:1413-1418). The value is clamped to
// 0/1 — `value === 0 ? 0 : 1` — so any nonzero value,
// including a negative one, sends 1.

// MulticombatAreaVarbit is VARBIT_MULTICOMBAT_AREA
// (PacketSender.ts:119): the varbit whose value draws the
// multi-combat icon (1) or hides it (0).
const MulticombatAreaVarbit = 4605

// EncodeMultiIcon encodes the multi-combat icon state:
// EncodeVarbit(MulticombatAreaVarbit, 0 or 1), clamping any
// nonzero value to 1 exactly as sendMultiIcon does
// (PacketSender.ts:774-776). The wire form is VARBIT
// (opcode 42, 6 bytes): varbit id as u16, value as int32.
func EncodeMultiIcon(value int) []byte {
	if value != 0 {
		value = 1
	}
	return EncodeVarbit(MulticombatAreaVarbit, value)
}
