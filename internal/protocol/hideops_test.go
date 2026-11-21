package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeHideNpcOps(t *testing.T) {
	if n, ok := ServerPacketLength(ServerHideNpcOps); !ok || n != 1 {
		t.Fatalf("ServerPacketLength(HIDE_NPC_OPS) = %d, %v, want 1, true", n, ok)
	}
	// Byte-exact: opcode + one boolean byte, no length prefix (the
	// opcode's table length is the fixed length 1).
	if got := EncodeHideNpcOps(false); !bytes.Equal(got, []byte{ServerHideNpcOps, 0}) {
		t.Errorf("EncodeHideNpcOps(false) = %v, want [148 0]", got)
	}
	if got := EncodeHideNpcOps(true); !bytes.Equal(got, []byte{ServerHideNpcOps, 1}) {
		t.Errorf("EncodeHideNpcOps(true) = %v, want [148 1]", got)
	}
}

func TestEncodeHideObjOps(t *testing.T) {
	if n, ok := ServerPacketLength(ServerHideObjOps); !ok || n != 1 {
		t.Fatalf("ServerPacketLength(HIDE_OBJ_OPS) = %d, %v, want 1, true", n, ok)
	}
	if got := EncodeHideObjOps(false); !bytes.Equal(got, []byte{ServerHideObjOps, 0}) {
		t.Errorf("EncodeHideObjOps(false) = %v, want [149 0]", got)
	}
	if got := EncodeHideObjOps(true); !bytes.Equal(got, []byte{ServerHideObjOps, 1}) {
		t.Errorf("EncodeHideObjOps(true) = %v, want [149 1]", got)
	}
}
