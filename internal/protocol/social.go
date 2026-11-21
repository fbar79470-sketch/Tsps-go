package protocol

import "encoding/binary"

// Social encoders: the friends/ignore/clan/chat-filter sends.
//
// TWO FAMILIES live in this file, and they must not be confused:
//
//   - The NATIVE family (EncodeChatFilterSettings,
//     EncodeFriendsChatSnapshot, and the five clan encoders) goes
//     through encodeServerPacket (ClientProtocol.ts) with real
//     ServerPacketId opcodes (121/122/124-128) and length-table
//     framing, via mustEncode like every other encoder in this
//     package. EncodeChatFilterSettings has a live producer (the
//     session chat-options sender, used by PlayerRelations
//     sendPrivateStatus). The snapshot and clan encoders have NO
//     producers in Go core this phase — the Phase 13 merge/
//     animation precedent: in TS they are produced only by
//     plugin content (FriendsChatManager.sendSnapshot,
//     FriendsChatManager.js:431; the clans plugin through
//     PluginCoreApi, PluginManager.ts:2698-2702), and no content
//     port exists yet. They exist so content can send through
//     them plus Session.SendPacket when it lands.
//
//   - The LEGACY family (EncodeFriendStatus, EncodeFriend,
//     EncodeDeleteFriend, EncodeAddIgnore, EncodeDeleteIgnore,
//     EncodeShowClanChatOptions) ports PacketSender methods that
//     build a PacketBuilder (PacketSender.ts:778-813, 397-401)
//     and hand it to session.write — which is a NO-OP in TS at
//     bfd81e3 (PlayerSession.write, PlayerSession.ts:228-230:
//     "Unported callers remain no-ops until they use a native
//     packet encoder"; BotPlayerSession.write is likewise
//     empty). These bytes NEVER reach a client in TS, and the
//     opcodes are legacy OSRS ids, NOT ServerPacketId values:
//     50/51/115 collide with INVENTORY_SNAPSHOT/INVENTORY_SLOT/
//     WIDGET_SET_PLAYER_HEAD in ServerPackets.ts, and 214/215/
//     221 have no ServerPacketId entry at all. They are
//     therefore NOT in the server length table (opcodes.go
//     stays a verbatim transcription of ServerPackets.ts), they
//     are NOT framed with mustEncode/EncodePacket (which would
//     misframe 50/51/115 under the wrong table entries and
//     reject 214/215/221 outright), and they must NEVER be sent
//     with Session.SendPacket: opcode 50 on the wire would be
//     parsed by the client as an inventory snapshot and desync
//     the stream. The encoders below reproduce the PacketBuilder
//     FIXED framing exactly (one opcode byte, no length prefix,
//     PacketBuilder's default PacketType.FIXED) as pure layout
//     functions with byte-vector tests, so the legacy layout is
//     on record; the session senders for them are deliberate
//     no-ops mirroring TS write(). The live presence path is
//     the friends-chat snapshot (opcode 121), sent by content.

// Legacy PacketBuilder opcodes for the friend/ignore/clan-option
// sends (PacketSender.ts:778-813, 397-401). These are NOT
// ServerPacketId values and MUST NOT be added to
// serverPacketLengths: see the file header. They exist only so
// the legacy encoders below name the opcode they emit.
const (
	// LegacyFriendStatusOpcode is the sendFriendStatus opcode (221).
	LegacyFriendStatusOpcode = 221
	// LegacyFriendOpcode is the sendFriend opcode (50). It
	// collides with ServerInventorySnapshot (also 50) in the
	// native table; the two are unrelated packet families.
	LegacyFriendOpcode = 50
	// LegacyDeleteFriendOpcode is the sendDeleteFriend opcode (51).
	// It collides with ServerInventorySlot (also 51).
	LegacyDeleteFriendOpcode = 51
	// LegacyAddIgnoreOpcode is the sendAddIgnore opcode (214).
	LegacyAddIgnoreOpcode = 214
	// LegacyDeleteIgnoreOpcode is the sendDeleteIgnore opcode (215).
	LegacyDeleteIgnoreOpcode = 215
	// LegacyShowClanChatOptionsOpcode is the sendShowClanChatOptions
	// opcode (115). It collides with ServerWidgetSetPlayerHead
	// (also 115).
	LegacyShowClanChatOptionsOpcode = 115
)

// legacyPacket frames one legacy PacketBuilder packet: the opcode
// byte followed by the payload, with no length prefix
// (PacketType.FIXED, the PacketBuilder constructor default,
// PacketBuilder.ts:28-36). It deliberately bypasses EncodePacket:
// the legacy opcodes have no length-table entries (see the file
// header).
func legacyPacket(opcode byte, payload []byte) []byte {
	out := make([]byte, 0, 1+len(payload))
	out = append(out, opcode)
	return append(out, payload...)
}

// legacyLong renders a name hash as PacketBuilder.putLong does
// (PacketBuilder.ts:366-389): the value reduced mod 2^64
// (toUnsignedLong, BigInt.asUintN(64)) and written big-endian.
// Name hashes are non-negative and below 37^12, so the reduction
// is a no-op for every real hash; the uint64 conversion
// reproduces it exactly for any int64.
func legacyLong(value int64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, uint64(value))
	return out
}

// EncodeFriendStatus encodes a friend-status send (sendFriendStatus,
// PacketSender.ts:778-783): the legacy opcode 221 plus one status
// byte, masked to 8 bits as PacketBuilder.put masks (PacketBuilder
// .ts:269-270). PlayerRelations.updateLists sends status 2
// (PlayerRelations.ts:188). Legacy framing: see the file header.
// It has no producer that sends it; TS drops it in write().
func EncodeFriendStatus(status int) []byte {
	return legacyPacket(LegacyFriendStatusOpcode, []byte{byte(status & 0xff)})
}

// EncodeFriend encodes a friend-presence send (sendFriend,
// PacketSender.ts:785-792): the legacy opcode 50, the 8-byte
// big-endian name hash, and the world byte, where a non-zero
// world is sent as world+9 and zero stays zero. The world byte
// is masked to 8 bits as PacketBuilder.put masks. Legacy framing:
// see the file header. It has no producer that sends it; TS
// drops it in write().
func EncodeFriend(nameHash int64, world int) []byte {
	if world != 0 {
		world += 9
	}
	payload := legacyLong(nameHash)
	payload = append(payload, byte(world&0xff))
	return legacyPacket(LegacyFriendOpcode, payload)
}

// EncodeDeleteFriend encodes a friend-removal send
// (sendDeleteFriend, PacketSender.ts:794-799): the legacy opcode
// 51 plus the 8-byte big-endian name hash. Legacy framing: see
// the file header. It has no producer that sends it; TS drops it
// in write().
func EncodeDeleteFriend(nameHash int64) []byte {
	return legacyPacket(LegacyDeleteFriendOpcode, legacyLong(nameHash))
}

// EncodeAddIgnore encodes an ignore-add send (sendAddIgnore,
// PacketSender.ts:801-806): the legacy opcode 214 plus the 8-byte
// big-endian name hash. Legacy framing: see the file header. It
// has no producer that sends it; TS drops it in write().
func EncodeAddIgnore(nameHash int64) []byte {
	return legacyPacket(LegacyAddIgnoreOpcode, legacyLong(nameHash))
}

// EncodeDeleteIgnore encodes an ignore-removal send
// (sendDeleteIgnore, PacketSender.ts:808-813): the legacy opcode
// 215 plus the 8-byte big-endian name hash. Legacy framing: see
// the file header. It has no producer that sends it; TS drops it
// in write().
func EncodeDeleteIgnore(nameHash int64) []byte {
	return legacyPacket(LegacyDeleteIgnoreOpcode, legacyLong(nameHash))
}

// EncodeShowClanChatOptions encodes the clan-chat-options send
// (sendShowClanChatOptions, PacketSender.ts:397-401): the legacy
// opcode 115 plus one byte, 1 when the right-click options show
// and 0 when they do not. Legacy framing: see the file header.
// It has no producer that sends it; TS drops it in write().
func EncodeShowClanChatOptions(show bool) []byte {
	value := byte(0)
	if show {
		value = 1
	}
	return legacyPacket(LegacyShowClanChatOptionsOpcode, []byte{value})
}

// EncodeChatFilterSettings encodes the chat-filter settings send
// (encodeChatFilterSettings, ClientProtocol.ts:2181-2183):
// CHAT_FILTER_SETTINGS (opcode 122, fixed length 3, no length
// prefix): the public, private, and trade mode bytes, each
// masked to 8 bits. PacketSender.sendChatOptions
// (PacketSender.ts:365-371) sends it through sendClientPacket —
// this is the one live relations send. It panics only if the
// table is violated, which the fixed 3-byte payload cannot do.
func EncodeChatFilterSettings(publicMode, privateMode, tradeMode int) []byte {
	return mustEncode(ServerChatFilterSettings, []byte{
		byte(publicMode & 0xff), byte(privateMode & 0xff), byte(tradeMode & 0xff),
	})
}

// FriendsChatChannelMember is one member of a friends-chat channel
// (the channel.members entries, FriendsChatSnapshot,
// ClientProtocol.ts:286-303).
type FriendsChatChannelMember struct {
	Name  string
	World int
	Rank  int
}

// FriendsChatChannel is the channel block of a friends-chat
// snapshot (FriendsChatSnapshot.channel, ClientProtocol.ts:
// 287-293). MinKickRank is the minimum rank that may kick;
// LocalRank is the viewing player's rank in the channel.
type FriendsChatChannel struct {
	Name        string
	Owner       string
	MinKickRank int
	LocalRank   int
	Members     []FriendsChatChannelMember
}

// FriendsChatFriend is one friend entry of a friends-chat snapshot
// (FriendsChatSnapshot.friends, ClientProtocol.ts:294-300).
// PreviousName is the friend's most recent held display name
// ("" when none).
type FriendsChatFriend struct {
	Name         string
	PreviousName string
	World        int
	Rank         int
	IsOnline     bool
}

// FriendsChatIgnore is one ignore entry of a friends-chat snapshot
// (FriendsChatSnapshot.ignores, ClientProtocol.ts:301).
type FriendsChatIgnore struct {
	Name         string
	PreviousName string
}

// FriendsChatSnapshot is one encodeFriendsChatSnapshot call's
// argument (ClientProtocol.ts:286-303). A nil Channel means the
// player is in no channel (the channel-present byte is 0).
type FriendsChatSnapshot struct {
	Channel *FriendsChatChannel
	Friends []FriendsChatFriend
	Ignores []FriendsChatIgnore
}

// EncodeFriendsChatSnapshot encodes a friends-chat snapshot
// (encodeFriendsChatSnapshot, ClientProtocol.ts:1218-1256):
// FRIENDS_CHAT_UPDATE (opcode 121, var-short), with the payload
// laid out as the channel-present byte, the channel block when
// present (name, owner, min-kick-rank and local-rank bytes,
// member count u16, then per member the name, world u16, and
// rank byte), the friend count u16 and per-friend entries (name,
// previous name, world u16, rank byte, online byte), and the
// ignore count u16 and per-ignore entries (name, previous name).
// Strings are latin1 NUL-terminated; counts and worlds are
// big-endian u16 masked to 16 bits; ranks are masked to 8 bits.
// It panics when the framed payload would exceed 65535 bytes,
// mirroring the TS encodeServerPacket throw. It has NO producer
// in Go core (see the file header): content sends it.
func EncodeFriendsChatSnapshot(snapshot FriendsChatSnapshot) []byte {
	payload := make([]byte, 0, 64)
	u16 := func(value int) {
		var buf [2]byte
		binary.BigEndian.PutUint16(buf[:], uint16(value&0xffff))
		payload = append(payload, buf[:]...)
	}
	if snapshot.Channel == nil {
		payload = append(payload, 0)
	} else {
		channel := snapshot.Channel
		payload = append(payload, 1)
		payload = append(payload, latin1Bytes(channel.Name)...)
		payload = append(payload, latin1Bytes(channel.Owner)...)
		payload = append(payload, byte(channel.MinKickRank&0xff), byte(channel.LocalRank&0xff))
		u16(len(channel.Members))
		for _, member := range channel.Members {
			payload = append(payload, latin1Bytes(member.Name)...)
			u16(member.World)
			payload = append(payload, byte(member.Rank&0xff))
		}
	}
	u16(len(snapshot.Friends))
	for _, friend := range snapshot.Friends {
		payload = append(payload, latin1Bytes(friend.Name)...)
		payload = append(payload, latin1Bytes(friend.PreviousName)...)
		u16(friend.World)
		online := byte(0)
		if friend.IsOnline {
			online = 1
		}
		payload = append(payload, byte(friend.Rank&0xff), online)
	}
	u16(len(snapshot.Ignores))
	for _, ignored := range snapshot.Ignores {
		payload = append(payload, latin1Bytes(ignored.Name)...)
		payload = append(payload, latin1Bytes(ignored.PreviousName)...)
	}
	return mustEncode(ServerFriendsChatUpdate, payload)
}

// ClanSettingsMember is one member of a clan's persistent state
// (ClanSettingsSnapshot.members, ClientProtocol.ts:1263).
type ClanSettingsMember struct {
	Name        string
	Rank        int
	Muted       bool
	ExtraInfo   int
	JoinRuneDay int
}

// ClanSetting is one clan setting entry
// (ClanSettingsSnapshot.settings, ClientProtocol.ts:1265).
type ClanSetting struct {
	ID    int
	Value int
}

// ClanSettingsSnapshot is one clan's persistent state, as
// ClanSettingsFull carries it (ClientProtocol.ts:1260-1266).
type ClanSettingsSnapshot struct {
	UpdateNum            int
	CreationTime         int
	Name                 string
	AllowGuests          bool
	TalkRank             int
	KickRank             int
	OwnerSlot            int
	ReplacementOwnerSlot int
	Members              []ClanSettingsMember
	Banned               []string
	Settings             []ClanSetting
}

// ClanChannelUser is one user of a clan's live channel
// (ClanChannelUser, ClientProtocol.ts:1279-1283).
type ClanChannelUser struct {
	Name  string
	Rank  int
	World int
}

// ClanChannelSnapshot is a clan's live channel, as ClanChannelFull
// carries it (ClientProtocol.ts:1271-1277).
type ClanChannelSnapshot struct {
	Key       int
	UpdateNum int
	Name      string
	TalkRank  int
	KickRank  int
	Users     []ClanChannelUser
}

// ClanChannelDeltaEvent is one clan-channel delta event
// (ClanChannelDeltaEvent, ClientProtocol.ts:1285-1287). Type is
// ClanChannelDeltaAdd (carrying User) or ClanChannelDeltaDelete
// (carrying Index). Any other Type encodes as a delete, exactly
// as the TS encoder's else branch does.
type ClanChannelDeltaEvent struct {
	Type  string
	User  ClanChannelUser
	Index int
}

// Clan channel delta event types (ClanChannelDeltaEvent,
// ClientProtocol.ts:1285-1287).
const (
	ClanChannelDeltaAdd    = "add"
	ClanChannelDeltaDelete = "delete"
)

// Clan settings delta update types (ClanSettingsDeltaUpdate,
// ClientProtocol.ts:1377-1383).
const (
	ClanSettingsDeltaAddMember       = "add_member"
	ClanSettingsDeltaDeleteMember    = "delete_member"
	ClanSettingsDeltaMemberRank      = "member_rank"
	ClanSettingsDeltaMemberMuted     = "member_muted"
	ClanSettingsDeltaMemberExtraInfo = "member_extra_info"
	ClanSettingsDeltaIntSetting      = "int_setting"
)

// ClanSettingsDeltaUpdate is one clan-settings delta update
// (ClanSettingsDeltaUpdate, ClientProtocol.ts:1377-1383). Only
// the fields of the update's Type are read: add_member reads
// Name and JoinRuneDay; delete_member reads Index; member_rank
// reads Index and Rank; member_muted reads Index and Muted;
// member_extra_info reads Index, Value, StartBit, and EndBit;
// int_setting reads Setting and Value. Any Type other than the
// five named ones encodes as an int_setting, exactly as the TS
// encoder's final else branch does.
type ClanSettingsDeltaUpdate struct {
	Type        string
	Name        string
	JoinRuneDay int
	Index       int
	Rank        int
	Muted       bool
	Value       int
	StartBit    int
	EndBit      int
	Setting     int
}

// clanI32 renders a value as the TS i32 helper does
// (ClientProtocol.ts:1301-1305): a signed 32-bit big-endian
// integer, truncated to 32 bits as `value | 0` truncates.
func clanI32(value int) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, uint32(int32(value)))
	return out
}

// clanU16 renders a value as the TS u16 helper does
// (ClientProtocol.ts:1295-1299): an unsigned 16-bit big-endian
// integer masked to 16 bits.
func clanU16(value int) []byte {
	out := make([]byte, 2)
	binary.BigEndian.PutUint16(out, uint16(value&0xffff))
	return out
}

// clanChannelUserBytes renders one channel user as
// clanChannelUserBytes does (ClientProtocol.ts:1307-1309): the
// latin1 NUL-terminated name, the rank byte, and the world u16.
func clanChannelUserBytes(user ClanChannelUser) []byte {
	out := latin1Bytes(user.Name)
	out = append(out, byte(user.Rank&0xff))
	return append(out, clanU16(user.World)...)
}

// EncodeClanSettingsFull encodes a clan's full persistent state
// (encodeClanSettingsFull, ClientProtocol.ts:1312-1335):
// CLAN_SETTINGS_FULL (opcode 124, var-short). The payload starts
// with the clan-type byte and the clan-present byte; a nil clan
// means the player has none of that type and sends nothing more.
// Otherwise: updateNum i32, creationTime i32, name, the
// allow-guests byte, talk-rank and kick-rank bytes, owner slot
// u16, replacement-owner slot u16, member count u16, per-member
// entries (name, rank byte, muted byte, extraInfo i32,
// joinRuneDay u16), banned count u16, banned names, settings
// count u16, and per-setting entries (id u16, value i32). It
// panics when the framed payload would exceed 65535 bytes,
// mirroring the TS encodeServerPacket throw. It has NO producer
// in Go core (see the file header): content sends it.
func EncodeClanSettingsFull(clanType int, clan *ClanSettingsSnapshot) []byte {
	present := byte(0)
	if clan != nil {
		present = 1
	}
	payload := []byte{byte(clanType & 0xff), present}
	if clan != nil {
		payload = append(payload, clanI32(clan.UpdateNum)...)
		payload = append(payload, clanI32(clan.CreationTime)...)
		payload = append(payload, latin1Bytes(clan.Name)...)
		guests := byte(0)
		if clan.AllowGuests {
			guests = 1
		}
		payload = append(payload, guests, byte(clan.TalkRank&0xff), byte(clan.KickRank&0xff))
		payload = append(payload, clanU16(clan.OwnerSlot)...)
		payload = append(payload, clanU16(clan.ReplacementOwnerSlot)...)
		payload = append(payload, clanU16(len(clan.Members))...)
		for _, member := range clan.Members {
			payload = append(payload, latin1Bytes(member.Name)...)
			muted := byte(0)
			if member.Muted {
				muted = 1
			}
			payload = append(payload, byte(member.Rank&0xff), muted)
			payload = append(payload, clanI32(member.ExtraInfo)...)
			payload = append(payload, clanU16(member.JoinRuneDay)...)
		}
		payload = append(payload, clanU16(len(clan.Banned))...)
		for _, name := range clan.Banned {
			payload = append(payload, latin1Bytes(name)...)
		}
		payload = append(payload, clanU16(len(clan.Settings))...)
		for _, setting := range clan.Settings {
			payload = append(payload, clanU16(setting.ID)...)
			payload = append(payload, clanI32(setting.Value)...)
		}
	}
	return mustEncode(ServerClanSettingsFull, payload)
}

// EncodeClanChannelFull encodes a clan's full live channel
// (encodeClanChannelFull, ClientProtocol.ts:1338-1352):
// CLAN_CHANNEL_FULL (opcode 125, var-short). The payload starts
// with the clan-type byte and the channel-present byte; a nil
// channel is ClanChannelFull(Leave) and sends nothing more.
// Otherwise: key i32, updateNum i32, name, talk-rank and
// kick-rank bytes, user count u16, and per-user entries (name,
// rank byte, world u16). It panics when the framed payload
// would exceed 65535 bytes, mirroring the TS encodeServerPacket
// throw. It has NO producer in Go core (see the file header):
// content sends it.
func EncodeClanChannelFull(clanType int, channel *ClanChannelSnapshot) []byte {
	present := byte(0)
	if channel != nil {
		present = 1
	}
	payload := []byte{byte(clanType & 0xff), present}
	if channel != nil {
		payload = append(payload, clanI32(channel.Key)...)
		payload = append(payload, clanI32(channel.UpdateNum)...)
		payload = append(payload, latin1Bytes(channel.Name)...)
		payload = append(payload, byte(channel.TalkRank&0xff), byte(channel.KickRank&0xff))
		payload = append(payload, clanU16(len(channel.Users))...)
		for _, user := range channel.Users {
			payload = append(payload, clanChannelUserBytes(user)...)
		}
	}
	return mustEncode(ServerClanChannelFull, payload)
}

// EncodeClanChannelDelta encodes clan-channel delta events
// (encodeClanChannelDelta, ClientProtocol.ts:1354-1370):
// CLAN_CHANNEL_DELTA (opcode 126, var-short). The payload is the
// clan-type byte, key i32, updateNum i32, event count u16, and
// per event either [1] plus the user bytes (an add) or [2] plus
// the index u16 (a delete; any non-add Type encodes as a
// delete, as in TS). It panics when the framed payload would
// exceed 65535 bytes, mirroring the TS encodeServerPacket throw.
// It has NO producer in Go core (see the file header): content
// sends it.
func EncodeClanChannelDelta(clanType, key, updateNum int, events []ClanChannelDeltaEvent) []byte {
	payload := []byte{byte(clanType & 0xff)}
	payload = append(payload, clanI32(key)...)
	payload = append(payload, clanI32(updateNum)...)
	payload = append(payload, clanU16(len(events))...)
	for _, event := range events {
		if event.Type == ClanChannelDeltaAdd {
			payload = append(payload, 1)
			payload = append(payload, clanChannelUserBytes(event.User)...)
		} else {
			payload = append(payload, 2)
			payload = append(payload, clanU16(event.Index)...)
		}
	}
	return mustEncode(ServerClanChannelDelta, payload)
}

// EncodeClanVar encodes a clan variable write (encodeClanVar,
// ClientProtocol.ts:1372-1374): CLAN_VAR (opcode 128, fixed
// length 6, no length prefix): the id u16 and the value i32.
// It panics only if the table is violated, which the fixed
// 6-byte payload cannot do. It has NO producer in Go core (see
// the file header): content sends it.
func EncodeClanVar(id, value int) []byte {
	payload := clanU16(id)
	payload = append(payload, clanI32(value)...)
	return mustEncode(ServerClanVar, payload)
}

// EncodeClanSettingsDelta encodes clan-settings delta updates
// (encodeClanSettingsDelta, ClientProtocol.ts:1385-1400):
// CLAN_SETTINGS_DELTA (opcode 127, var-short). The payload is
// the clan-type byte, updateNum i32, update count u16, and per
// update one of: [1] name + joinRuneDay u16 (add_member), [2]
// index u16 (delete_member), [3] index u16 + rank byte
// (member_rank), [4] index u16 + muted byte (member_muted), [5]
// index u16 + value i32 + startBit/endBit bytes
// (member_extra_info; the bit bytes are written unmasked, as
// TS writes them raw), or [6] setting u16 + value i32
// (int_setting; any Type other than the five named ones encodes
// here, as in TS). It panics when the framed payload would
// exceed 65535 bytes, mirroring the TS encodeServerPacket throw.
// It has NO producer in Go core (see the file header): content
// sends it.
func EncodeClanSettingsDelta(clanType, updateNum int, updates []ClanSettingsDeltaUpdate) []byte {
	payload := []byte{byte(clanType & 0xff)}
	payload = append(payload, clanI32(updateNum)...)
	payload = append(payload, clanU16(len(updates))...)
	for _, update := range updates {
		switch update.Type {
		case ClanSettingsDeltaAddMember:
			payload = append(payload, 1)
			payload = append(payload, latin1Bytes(update.Name)...)
			payload = append(payload, clanU16(update.JoinRuneDay)...)
		case ClanSettingsDeltaDeleteMember:
			payload = append(payload, 2)
			payload = append(payload, clanU16(update.Index)...)
		case ClanSettingsDeltaMemberRank:
			payload = append(payload, 3)
			payload = append(payload, clanU16(update.Index)...)
			payload = append(payload, byte(update.Rank&0xff))
		case ClanSettingsDeltaMemberMuted:
			payload = append(payload, 4)
			payload = append(payload, clanU16(update.Index)...)
			muted := byte(0)
			if update.Muted {
				muted = 1
			}
			payload = append(payload, muted)
		case ClanSettingsDeltaMemberExtraInfo:
			payload = append(payload, 5)
			payload = append(payload, clanU16(update.Index)...)
			payload = append(payload, clanI32(update.Value)...)
			payload = append(payload, byte(update.StartBit), byte(update.EndBit))
		default:
			payload = append(payload, 6)
			payload = append(payload, clanU16(update.Setting)...)
			payload = append(payload, clanI32(update.Value)...)
		}
	}
	return mustEncode(ServerClanSettingsDelta, payload)
}
