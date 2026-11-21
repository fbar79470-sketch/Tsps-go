package protocol

import (
	"errors"
	"testing"
)

func TestDecodeClanChannelAction(t *testing.T) {
	payload := func(action, value byte, name string) []byte {
		out := []byte{action, value}
		out = append(out, name...)
		return append(out, 0)
	}
	for _, tc := range []struct {
		action byte
		want   string
	}{
		{0, ClanChannelActionKick},
		{1, ClanChannelActionBan},
		{2, ClanChannelActionMute},
	} {
		got, err := DecodeClanChannelAction(payload(tc.action, 7, "bob"))
		if err != nil {
			t.Fatalf("action %d: unexpected error: %v", tc.action, err)
		}
		if got.Action != tc.want || got.Value != 7 || got.Name != "bob" {
			t.Errorf("action %d: got %+v, want {%s 7 bob}", tc.action, got, tc.want)
		}
	}
	// Trailing bytes are ignored, as in TS.
	if got, err := DecodeClanChannelAction(append(payload(0, 1, "x"), 99)); err != nil || got.Name != "x" {
		t.Errorf("trailing bytes: got %+v, err %v", got, err)
	}
	// An unknown action is a decode failure (the TS "raw" return):
	// the DecodeTradeAction precedent.
	if _, err := DecodeClanChannelAction(payload(3, 7, "bob")); err == nil {
		t.Error("unknown action 3: got no error, want an error")
	}
	// Reads run before the action check: a truncated payload
	// errors as truncated even with an unknown action byte.
	if _, err := DecodeClanChannelAction([]byte{9}); !errors.Is(err, ErrUnexpectedEnd) {
		t.Errorf("truncated unknown action: err %v, want ErrUnexpectedEnd", err)
	}
	if _, err := DecodeClanChannelAction(nil); !errors.Is(err, ErrUnexpectedEnd) {
		t.Errorf("empty payload: err %v, want ErrUnexpectedEnd", err)
	}
	if _, err := DecodeClanChannelAction([]byte{0, 7, 'b', 'o', 'b'}); !errors.Is(err, ErrUnterminatedString) {
		t.Errorf("unterminated name: err %v, want ErrUnterminatedString", err)
	}
}

func TestDecodeFriendsChatAction(t *testing.T) {
	payload := func(code byte, name string, rank byte) []byte {
		out := []byte{code}
		out = append(out, name...)
		out = append(out, 0, rank)
		return out
	}
	cases := []struct {
		code byte
		name string
		rank byte
		want FriendsChatAction
	}{
		{0, "alice", 5, FriendsChatAction{Action: FriendsChatActionJoin, Name: "alice"}},
		{1, "alice", 5, FriendsChatAction{Action: FriendsChatActionLeave}},
		{2, "alice", 5, FriendsChatAction{Action: FriendsChatActionKick, Name: "alice"}},
		{3, "alice", 5, FriendsChatAction{Action: FriendsChatActionAddFriend, Name: "alice"}},
		{4, "alice", 5, FriendsChatAction{Action: FriendsChatActionRemoveFriend, Name: "alice"}},
		{5, "alice", 6, FriendsChatAction{Action: FriendsChatActionSetFriendRank, Name: "alice", Rank: 6}},
		{6, "alice", 5, FriendsChatAction{Action: FriendsChatActionAddIgnore, Name: "alice"}},
		{7, "alice", 5, FriendsChatAction{Action: FriendsChatActionRemoveIgnore, Name: "alice"}},
	}
	for _, tc := range cases {
		got, err := DecodeFriendsChatAction(payload(tc.code, tc.name, tc.rank))
		if err != nil {
			t.Fatalf("code %d: unexpected error: %v", tc.code, err)
		}
		if got != tc.want {
			t.Errorf("code %d: got %+v, want %+v", tc.code, got, tc.want)
		}
	}
	// The rank byte is signed: 0xff is -1, 0x80 is -128.
	got, err := DecodeFriendsChatAction(payload(5, "alice", 0xff))
	if err != nil || got.Rank != -1 {
		t.Errorf("rank 0xff: got %+v, err %v, want rank -1", got, err)
	}
	got, err = DecodeFriendsChatAction(payload(5, "alice", 0x80))
	if err != nil || got.Rank != -128 {
		t.Errorf("rank 0x80: got %+v, err %v, want rank -128", got, err)
	}
	// An unknown action is a decode failure (the TS "raw" return).
	if _, err := DecodeFriendsChatAction(payload(8, "alice", 0)); err == nil {
		t.Error("unknown action 8: got no error, want an error")
	}
	// Reads run before the action check: a truncated payload
	// errors as truncated even with an unknown action code.
	if _, err := DecodeFriendsChatAction([]byte{9}); !errors.Is(err, ErrUnterminatedString) {
		t.Errorf("truncated unknown action: err %v, want ErrUnterminatedString", err)
	}
	if _, err := DecodeFriendsChatAction(nil); !errors.Is(err, ErrUnexpectedEnd) {
		t.Errorf("empty payload: err %v, want ErrUnexpectedEnd", err)
	}
	if _, err := DecodeFriendsChatAction([]byte{0, 'a', 0}); !errors.Is(err, ErrUnexpectedEnd) {
		t.Errorf("missing rank byte: err %v, want ErrUnexpectedEnd", err)
	}
}

func TestDecodePrivateMessage(t *testing.T) {
	got, err := DecodePrivateMessage([]byte{'r', 'e', 'c', 0, 'h', 'i', 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Recipient != "rec" || got.Text != "hi" {
		t.Errorf("got %+v, want {rec hi}", got)
	}
	if _, err := DecodePrivateMessage([]byte{'r', 0}); !errors.Is(err, ErrUnterminatedString) {
		t.Errorf("missing text: err %v, want ErrUnterminatedString", err)
	}
	if _, err := DecodePrivateMessage(nil); !errors.Is(err, ErrUnterminatedString) {
		t.Errorf("empty payload: err %v, want ErrUnterminatedString", err)
	}
}

func TestDecodeChatFilter(t *testing.T) {
	got, err := DecodeChatFilter([]byte{0, 1, 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != (ChatFilter{PublicMode: 0, PrivateMode: 1, TradeMode: 2}) {
		t.Errorf("got %+v, want {0 1 2}", got)
	}
	if _, err := DecodeChatFilter([]byte{0, 1}); !errors.Is(err, ErrUnexpectedEnd) {
		t.Errorf("short payload: err %v, want ErrUnexpectedEnd", err)
	}
	// Fixed length 3: trailing bytes are an error.
	if _, err := DecodeChatFilter([]byte{0, 1, 2, 3}); err == nil {
		t.Error("trailing byte: got no error, want a trailing-bytes error")
	}
}

func TestDecodeReportAbuse(t *testing.T) {
	payload := func(name string, rule, mute byte) []byte {
		out := []byte(name)
		return append(out, 0, rule, mute)
	}
	got, err := DecodeReportAbuse(payload("mallory", 4, 1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "mallory" || got.RuleID != 4 || !got.Mute {
		t.Errorf("got %+v, want {mallory 4 true}", got)
	}
	// Mute is true only when the byte is exactly 1.
	for _, mute := range []byte{0, 2, 0xff} {
		got, err := DecodeReportAbuse(payload("n", 0, mute))
		if err != nil || got.Mute {
			t.Errorf("mute byte %d: got %+v, err %v, want Mute false", mute, got, err)
		}
	}
	if _, err := DecodeReportAbuse([]byte{'n', 0, 4}); !errors.Is(err, ErrUnexpectedEnd) {
		t.Errorf("missing mute byte: err %v, want ErrUnexpectedEnd", err)
	}
}
