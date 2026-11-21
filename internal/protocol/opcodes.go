package protocol

// Opcode constants and packet-length tables, transcribed verbatim from the
// three TS table files. Length values: -1 = 1-byte length prefix, -2 = 2-byte
// big-endian length prefix, >= 0 = fixed length, no prefix.
//
// TS constant names are preserved with a Client/Native/Server prefix to avoid
// collisions between the tables (e.g. RESUME_PAUSEBUTTON exists in both
// client tables, and IF_BUTTOND exists in both the high-level and native
// tables with different opcodes).

// Client opcodes: high-level protocol (ClientPackets.ts, ClientPacketId
// enum). The header comment there notes these use opcode range 180+,
// separate from the OSRS-style native opcodes.
const (
	ClientHello                = 200
	ClientPing                 = 201
	ClientHandshake            = 202
	ClientLogout               = 203
	ClientLogin                = 204
	ClientPluginMessage        = 205
	ClientLoginForget          = 207
	ClientCharacterSelect      = 206
	ClientCharacterCreate      = 208
	ClientCharacterDelete      = 209
	ClientWalk                 = 210
	ClientFace                 = 211
	ClientTeleport             = 212
	ClientPathfind             = 213
	ClientMovementInput        = 215
	ClientLocInteract          = 231
	ClientGroundItemAction     = 232
	ClientInteract             = 233
	ClientInteractStop         = 234
	ClientInventoryUse         = 240
	ClientInventoryUseOn       = 241
	ClientInventoryMove        = 242
	ClientBankDepositInventory = 243
	ClientBankDepositEquipment = 244
	ClientBankMove             = 245
	ClientItemSpawnerSearch    = 246
	ClientWidget               = 250
	ClientWidgetAction         = 251
	ClientResumePausebutton    = 252
	ClientIfButtond            = 253
	ClientEmote                = 254
	ClientDebug                = 255
	ClientTradeAction          = 180
	ClientClanChannelAction    = 181
	ClientChat                 = 190
	ClientVarpTransmit         = 191
	ClientResumeCountdialog    = 192
	ClientResumeNamedialog     = 193
	ClientResumeStringdialog   = 194
	ClientMapEdit              = 195
	ClientFriendsChatAction    = 196
	ClientPrivateMessage       = 197
	ClientChatFilter           = 198
	ClientReportAbuse          = 199
)

// clientPacketLengths is CLIENT_PACKET_LENGTHS from ClientPackets.ts,
// verbatim (44 entries). Comments quote the TS file's own layout notes.
var clientPacketLengths = map[byte]int{
	ClientHello:                -1,
	ClientPing:                 4, // time(4)
	ClientHandshake:            -1,
	ClientLogout:               0,
	ClientPluginMessage:        -2,
	ClientLogin:                -2, // username + password (variable)
	ClientLoginForget:          -1, // username + token (variable)
	ClientCharacterSelect:      -1, // name
	ClientCharacterCreate:      -1, // name
	ClientCharacterDelete:      -1, // name + confirmName
	ClientWalk:                 5,  // x(2) + y(2) + flags(1)
	ClientFace:                 -1,
	ClientTeleport:             5, // x(2) + y(2) + level(1)
	ClientPathfind:             -1,
	ClientMovementInput:        12, // seq(4), dx/dy(2 each), yaw(2), duration(1), flags(1)
	ClientLocInteract:          -1,
	ClientGroundItemAction:     -1,
	ClientInteract:             -1,
	ClientInteractStop:         0,
	ClientInventoryUse:         -1,
	ClientInventoryUseOn:       -1,
	ClientInventoryMove:        4, // from(2) + to(2)
	ClientBankDepositInventory: 0,
	ClientBankDepositEquipment: 0,
	ClientBankMove:             -1,
	ClientItemSpawnerSearch:    -1,
	ClientWidget:               -1,
	ClientWidgetAction:         -1,
	ClientResumePausebutton:    6,  // widgetId(4) + childIndex(2)
	ClientIfButtond:            16, // target item(2) + target widget(4) + source item(2) + source slot(2) + source widget(4) + target slot(2)
	ClientEmote:                3,  // index(2) + loop(1)
	ClientTradeAction:          -1,
	ClientClanChannelAction:    -1,
	ClientChat:                 -1,
	ClientVarpTransmit:         6,  // varpId(2) + value(4)
	ClientResumeCountdialog:    4,  // value(4)
	ClientResumeNamedialog:     -1, // value(string)
	ClientResumeStringdialog:   -1, // value(string)
	ClientMapEdit:              -1, // action(1) + tile(4) + level/type/rotation/id(var)
	ClientFriendsChatAction:    -1,
	ClientPrivateMessage:       -1,
	ClientChatFilter:           3,
	ClientReportAbuse:          -1, // name(string) + rule(1) + mute(1)
	ClientDebug:                -2,
}

// Native client opcodes: OSRS-style packets (NativeClientPackets.ts,
// ClientPacketId enum).
const (
	NativeIfButtond         = 1
	NativeOpLocT            = 2
	NativeOpPlayer5         = 6
	NativeExamineNpc        = 9
	NativeOpPlayer7         = 10
	NativeIfButton6         = 11
	NativeOpNpc2            = 12
	NativeIfButton          = 13
	NativeIfButton7         = 14
	NativeMoveGameclick     = 16
	NativeIfButton8         = 19
	NativeIfButton9         = 20
	NativeOpPlayer8         = 21
	NativeIfButton1         = 23
	NativeIfButton2         = 25
	NativeOpLoc2            = 28
	NativeIfTriggeroplocal  = 30
	NativeIfButton3         = 31
	NativeOpPlayerT         = 32
	NativeOpNpc3            = 34
	NativeOpNpcU            = 36
	NativeAppearanceSet     = 37
	NativeOpLoc4            = 38
	NativeOpLoc3            = 42
	NativeOpObj2            = 43
	NativeOpPlayer1         = 44
	NativeOpPlayer2         = 45
	NativeOpPlayer3         = 46
	NativeOpPlayer6         = 48
	NativeOpNpc5            = 50
	NativeOpLoc5            = 51
	NativeIfClose           = 55
	NativeOpObj4            = 56
	NativeOpNpc1            = 57
	NativeResumePausebutton = 62
	NativeIfButton4         = 63
	NativeOpPlayerU         = 65
	NativeIfButton5         = 69
	NativeOpNpc4            = 70
	NativeOpPlayer4         = 73
	NativeOpNpcT            = 75
	NativeOpNpc1Alt         = 76
	NativeOpObjU            = 79
	NativeOpObj5            = 82
	NativeIfButton10        = 84
	NativeExamineLoc        = 85
	NativeOpLocU            = 86
	NativeIfButtonSub       = 89
	NativeIfButtont         = 90
	NativeIfButtonX         = 91
	NativeOpLocTAlt         = 94
	NativeOpLoc1            = 96
	NativeOpObj1            = 102
	NativeOpObj3            = 103
	NativeExamineObj        = 104
	NativeWorldMapClick     = 105
	NativeSetHeading        = 214
	NativeOpWorldEntity6    = 225
	NativeOpWorldEntity5    = 224
	NativeOpWorldEntity2    = 221
	NativeOpWorldEntity4    = 223
	NativeOpWorldEntity3    = 222
	NativeOpWorldEntity1    = 220
)

// nativeClientPacketLengths is CLIENT_PACKET_LENGTHS from
// NativeClientPackets.ts, verbatim (63 entries).
//
// Note: the APPEARANCE_SET enum comment in that file says "len=13", but the
// length table says 20, and the decode branch (ClientProtocol.ts,
// APPEARANCE_SET case: gender byte + 7 signed shorts + 5 bytes) reads 20
// bytes. The table value 20 is used here.
var nativeClientPacketLengths = map[byte]int{
	NativeIfButtond:         16,
	NativeOpLocT:            15,
	NativeOpPlayer5:         3,
	NativeExamineNpc:        2,
	NativeOpPlayer7:         3,
	NativeIfButton6:         8,
	NativeOpNpc2:            3,
	NativeIfButton:          4,
	NativeIfButton7:         8,
	NativeMoveGameclick:     7,
	NativeWorldMapClick:     4,
	NativeIfButton8:         8,
	NativeIfButton9:         8,
	NativeOpPlayer8:         3,
	NativeIfButton1:         8,
	NativeIfButton2:         8,
	NativeOpLoc1:            7,
	NativeOpLoc2:            7,
	NativeIfButton3:         8,
	NativeOpPlayerT:         11,
	NativeOpNpc3:            3,
	NativeOpNpcU:            11,
	NativeOpLoc4:            7,
	NativeOpLoc3:            7,
	NativeOpObj2:            7,
	NativeOpPlayer1:         3,
	NativeOpPlayer2:         3,
	NativeOpPlayer3:         3,
	NativeOpPlayer6:         3,
	NativeOpNpc5:            3,
	NativeAppearanceSet:     20,
	NativeOpLoc5:            7,
	NativeIfClose:           0,
	NativeOpObj4:            7,
	NativeOpNpc1:            3,
	NativeResumePausebutton: 6,
	NativeSetHeading:        1,
	NativeIfButton4:         8,
	NativeOpPlayerU:         11,
	NativeIfButton5:         8,
	NativeOpNpc4:            3,
	NativeOpPlayer4:         3,
	NativeOpNpcT:            11,
	NativeOpNpc1Alt:         3,
	NativeOpObjU:            15,
	NativeOpObj5:            7,
	NativeIfButton10:        8,
	NativeIfTriggeroplocal:  -2,
	NativeExamineLoc:        2,
	NativeExamineObj:        6,
	NativeOpLocU:            15,
	NativeIfButtont:         16,
	NativeIfButtonSub:       10,
	NativeIfButtonX:         9,
	NativeOpLocTAlt:         15,
	NativeOpObj1:            7,
	NativeOpObj3:            7,
	NativeOpWorldEntity1:    3,
	NativeOpWorldEntity2:    3,
	NativeOpWorldEntity3:    3,
	NativeOpWorldEntity4:    3,
	NativeOpWorldEntity5:    3,
	NativeOpWorldEntity6:    2,
}

// ClientPacketLength returns the table length value for a client-to-server
// opcode, consulting the high-level table first and then the native table,
// exactly like clientPacketLength (ClientProtocol.ts lines 476-479). The
// second result is false for an unknown opcode. (The only opcode present in
// both tables is 62, RESUME_PAUSEBUTTON, with length 6 in both, so the
// precedence is unobservable in practice.)
func ClientPacketLength(opcode byte) (int, bool) {
	if length, ok := clientPacketLengths[opcode]; ok {
		return length, true
	}
	length, ok := nativeClientPacketLengths[opcode]
	return length, ok
}

// Server opcodes (ServerPackets.ts, ServerPacketId enum).
const (
	ServerWelcome                     = 0
	ServerTick                        = 1
	ServerHandshake                   = 2
	ServerLoginResponse               = 3
	ServerLogoutResponse              = 4
	ServerPathResponse                = 5
	ServerPluginMessage               = 6
	ServerCharacterList               = 7
	ServerCharacterResult             = 8
	ServerPlayerSync                  = 20
	ServerNpcInfo                     = 21
	ServerAnim                        = 22
	ServerMovementPosition            = 23
	ServerNpcAnimSpecific             = 24
	ServerNpcSpotanimSpecific         = 25
	ServerNpcHeadiconSpecific         = 26
	ServerVarpSmall                   = 40
	ServerVarpLarge                   = 41
	ServerVarbit                      = 42
	ServerVarpBatch                   = 43
	ServerVarpLong                    = 44
	ServerInventorySnapshot           = 50
	ServerInventorySlot               = 51
	ServerBankSnapshot                = 52
	ServerBankSlot                    = 53
	ServerInventoryStopTransmit       = 56
	ServerGroundItems                 = 54
	ServerGroundItemsDelta            = 55
	ServerSkillsSnapshot              = 70
	ServerSkillsDelta                 = 71
	ServerCombatState                 = 80
	ServerRunEnergy                   = 81
	ServerHitsplat                    = 82
	ServerSpotAnim                    = 83
	ServerProjectiles                 = 84
	ServerSpellResult                 = 85
	ServerDebugPacket                 = 86
	ServerDestination                 = 87
	ServerPlayerOption                = 88
	ServerHintArrow                   = 89
	ServerAttackTimer                 = 90
	ServerWidgetOpen                  = 100
	ServerWidgetClose                 = 101
	ServerWidgetSetRoot               = 102
	ServerWidgetOpenSub               = 103
	ServerWidgetCloseSub              = 104
	ServerWidgetSetText               = 105
	ServerWidgetSetHidden             = 106
	ServerWidgetSetItem               = 107
	ServerWidgetSetNpcHead            = 108
	ServerWidgetSetFlagsRange         = 109
	ServerWidgetRunScript             = 110
	ServerWidgetSetFlags              = 111
	ServerWidgetSetFlagsRangeV2       = 112
	ServerWidgetSetAngle              = 113
	ServerWidgetSetAnimation          = 114
	ServerWidgetSetPlayerHead         = 115
	ServerWidgetSetRotateSpeed        = 116
	ServerWidgetSetModel              = 117
	ServerWidgetSetPosition           = 118
	ServerWidgetSetColour             = 119
	ServerChatMessage                 = 120
	ServerFriendsChatUpdate           = 121
	ServerChatFilterSettings          = 122
	ServerTriggerOndialogabort        = 123
	ServerClanSettingsFull            = 124
	ServerClanChannelFull             = 125
	ServerClanChannelDelta            = 126
	ServerClanSettingsDelta           = 127
	ServerClanVar                     = 128
	ServerLocChange                   = 130
	ServerLocAddChange                = 134
	ServerLocDel                      = 135
	ServerLocAnim                     = 136
	ServerCameraControl               = 137
	ServerLocMerge                    = 138
	ServerSound                       = 131
	ServerPlayJingle                  = 132
	ServerPlaySong                    = 133
	ServerRebuildRegion               = 140
	ServerRebuildNormal               = 141
	ServerRebuildWorldEntity          = 142
	ServerWorldEntityInfo             = 143
	ServerRegionReplacement           = 144
	ServerSetInteractionMode          = 145
	ServerResetInteractionMode        = 146
	ServerRegionReplacementCompressed = 147
	ServerHideNpcOps                  = 148
	ServerHideObjOps                  = 149
	ServerShopOpen                    = 150
	ServerShopSlot                    = 151
	ServerShopClose                   = 152
	ServerShopMode                    = 153
	ServerTradeRequest                = 154
	ServerTradeOpen                   = 155
	ServerTradeUpdate                 = 156
	ServerTradeClose                  = 157
	ServerRunClientScript             = 170
	ServerSmithingOpen                = 180
	ServerSmithingMode                = 181
	ServerSmithingClose               = 182
	ServerCollectionLogSnapshot       = 190
	ServerNotification                = 200
	ServerGamemodeData                = 210
	ServerSystemUpdate                = 220
	ServerDebug                       = 250
)

// serverPacketLengths is SERVER_PACKET_LENGTHS from ServerPackets.ts,
// verbatim (106 entries). Comments quote the TS file's own layout notes.
var serverPacketLengths = map[byte]int{
	ServerPluginMessage:               -2,
	ServerCharacterList:               -2,
	ServerCharacterResult:             -1,
	ServerWelcome:                     8, // tickMs(4) + serverTime(4)
	ServerTick:                        8, // tick(4) + time(4)
	ServerHandshake:                   -1,
	ServerCameraControl:               -1,
	ServerLoginResponse:               -1,
	ServerLogoutResponse:              -1,
	ServerPathResponse:                -1,
	ServerPlayerSync:                  -2,
	ServerMovementPosition:            18,
	ServerNpcInfo:                     -2,
	ServerNpcAnimSpecific:             5,
	ServerNpcSpotanimSpecific:         9,
	ServerNpcHeadiconSpecific:         7,
	ServerAnim:                        22, // 11 shorts for animation IDs
	ServerVarpSmall:                   3,  // varpId(2) + value(1)
	ServerVarpLarge:                   6,  // varpId(2) + value(4)
	ServerVarbit:                      6,  // varbitId(2) + value(4)
	ServerVarpBatch:                   -1,
	ServerVarpLong:                    10, // varpId(2) + value(8)
	ServerInventorySnapshot:           -2,
	ServerInventorySlot:               -1, // Quantity is encoded OSRS-style: 1 byte, or 255 + int (variable length).
	ServerBankSnapshot:                -2,
	ServerBankSlot:                    -1,
	ServerInventoryStopTransmit:       2,
	ServerGroundItems:                 -2,
	ServerGroundItemsDelta:            -2,
	ServerSkillsSnapshot:              -1,
	ServerSkillsDelta:                 -1,
	ServerCombatState:                 -1,
	ServerRunEnergy:                   10, // percent(1) + running(1) + units(2) + weight kg(2 signed) + stamina ticks(2) + stamina active(2)
	ServerHitsplat:                    -1,
	ServerSpotAnim:                    -1,
	ServerProjectiles:                 -2,
	ServerSpellResult:                 -2,
	ServerDebugPacket:                 -2,
	ServerPlayerOption:                -1,
	ServerDestination:                 4,  // worldX(2) + worldY(2)
	ServerHintArrow:                   7,  // type(1) + a(2) + b(2) + c(1) + d(1); type 0 clear, 1 npc, 2 tile, 3 player (encodeHintArrow)
	ServerAttackTimer:                 1,  // ticks until the next attack (0-255)
	ServerWidgetOpen:                  3,  // groupId(2) + modal(1)
	ServerWidgetClose:                 2,  // groupId(2)
	ServerWidgetSetRoot:               2,  // groupId(2)
	ServerWidgetOpenSub:               -2, // Custom open_sub payload can include initial varp/varbit/hidden UID state.
	ServerWidgetCloseSub:              4,  // targetUid(4)
	ServerWidgetSetText:               -2,
	ServerWidgetSetModel:              8,  // uid(4) + raw modelId(4)
	ServerWidgetSetPosition:           8,  // uid(4) + x(2) + y(2)
	ServerWidgetSetAngle:              10, // uid(4) + angleX(2) + angleY(2) + zoom(2)
	ServerWidgetSetRotateSpeed:        8,  // uid(4) + speedY(2, high) + speedX(2, low)
	ServerWidgetSetColour:             6,  // uid(4) + 15-bit rgb colour(2)
	ServerWidgetSetHidden:             5,  // uid(4) + hidden(1)
	ServerWidgetSetItem:               10, // uid(4) + itemId(2) + quantity(4)
	ServerWidgetSetNpcHead:            6,  // uid(4) + npcId(2)
	ServerWidgetSetFlagsRange:         12, // uid(4) + fromSlot(2) + toSlot(2) + flags(4)
	ServerWidgetSetFlagsRangeV2:       16, // uid(4) + fromSlot(2) + toSlot(2) + flags(4) + ops(4)
	ServerWidgetRunScript:             -2, // scriptId(4) + args (variable)
	ServerWidgetSetFlags:              8,  // uid(4) + flags(4)
	ServerWidgetSetAnimation:          6,  // uid(4) + animId(2)
	ServerWidgetSetPlayerHead:         4,  // uid(4)
	ServerChatMessage:                 -2, // var-short: tutorial/quest game messages exceed 255 bytes
	ServerFriendsChatUpdate:           -2,
	ServerChatFilterSettings:          3, // public(1) + private(1) + trade(1)
	ServerTriggerOndialogabort:        0,
	ServerClanSettingsFull:            -2,
	ServerClanChannelFull:             -2,
	ServerClanChannelDelta:            -2,
	ServerClanSettingsDelta:           -2,
	ServerClanVar:                     6,
	ServerLocChange:                   -1,
	ServerLocAddChange:                -1,
	ServerLocDel:                      -1,
	ServerLocMerge:                    18, // loc/tile/shape + player index + cycle interval + signed bounds
	ServerLocAnim:                     10, // locId(2) + x(2) + y(2) + level(1) + shape/rot(1) + animId(2)
	ServerSound:                       -1,
	ServerPlayJingle:                  5,  // jingleId(2) + delay(3, IME)
	ServerPlaySong:                    10, // trackId(2) + outDelay(2) + outDur(2) + inDelay(2) + inDur(2)
	ServerRebuildRegion:               -2,
	ServerRebuildNormal:               -2,
	ServerRebuildWorldEntity:          -2,
	ServerWorldEntityInfo:             -1, // count(1) + per-entity updates + new spawns
	ServerRegionReplacement:           -2,
	ServerRegionReplacementCompressed: -2, // regionId(2) + flags(1) + lengths(8) + deflated data
	ServerSetInteractionMode:          4,  // worldId(2) + tileMode(1) + entityMode(1)
	ServerResetInteractionMode:        2,  // worldId(2)
	ServerHideNpcOps:                  1,  // hidden(1)
	ServerHideObjOps:                  1,  // hidden(1)
	ServerShopOpen:                    -2,
	ServerShopSlot:                    -1,
	ServerShopClose:                   0,
	ServerShopMode:                    -1,
	ServerTradeRequest:                -1,
	ServerTradeOpen:                   -2,
	ServerTradeUpdate:                 -2,
	ServerTradeClose:                  -1,
	ServerRunClientScript:             -2,
	ServerSmithingOpen:                -2, // mode(1) + title(var) + options(var) + quantityMode(1) + customQuantity(4)
	ServerSmithingMode:                5,  // quantityMode(1) + customQuantity(4)
	ServerSmithingClose:               0,
	ServerCollectionLogSnapshot:       -2, // count(2) + slots(var)
	ServerNotification:                -1, // kind(1) + title(var) + message(var) + itemId(2) + quantity(4) + durationMs(2)
	ServerGamemodeData:                -2,
	ServerSystemUpdate:                4, // remainingCentis(4, big-endian)
	ServerDebug:                       -2,
}

// ServerPacketLength returns the table length value for a server-to-client
// opcode from SERVER_PACKET_LENGTHS (ServerPackets.ts). The second result is
// false for an unknown opcode.
func ServerPacketLength(opcode byte) (int, bool) {
	length, ok := serverPacketLengths[opcode]
	return length, ok
}
