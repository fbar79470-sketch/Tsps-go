package protocol

import (
	"bytes"
	"testing"
)

// cameraBytes runs an error-returning camera encoder and fails
// the test on error, returning the framed bytes.
func cameraBytes(t *testing.T, b []byte, err error) []byte {
	t.Helper()
	if err != nil {
		t.Fatalf("camera encoder error: %v", err)
	}
	return b
}

func TestEncodeCameraShake(t *testing.T) {
	got := EncodeCameraShake(1, 100, 200, 300)
	want := []byte{137, 8, 3, 1, 0, 100, 0, 200, 1, 44}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeCameraShake = %x, want %x", got, want)
	}
}

func TestEncodeCameraMoveToForms(t *testing.T) {
	// Instant (boolean) form: discriminator 1, 8 bytes.
	b, err := EncodeCameraMoveTo(3200, 3200, 100, CameraMotionBool(true))
	got := cameraBytes(t, b, err)
	want := []byte{137, 8, 1, 0x0c, 0x80, 0x0c, 0x80, 0, 100, 1}
	if !bytes.Equal(got, want) {
		t.Errorf("MoveTo instant = %x, want %x", got, want)
	}
	b, err = EncodeCameraMoveTo(3200, 3200, 100, CameraMotionBool(false))
	got = cameraBytes(t, b, err)
	if got[len(got)-1] != 0 {
		t.Errorf("MoveTo instant false last byte = %d, want 0", got[len(got)-1])
	}
	// Rate form: discriminator 1+3 = 4, 10 bytes.
	b, err = EncodeCameraMoveTo(3200, 3200, 100, CameraMotionRates(10, 20, true))
	got = cameraBytes(t, b, err)
	want = []byte{137, 10, 4, 0x0c, 0x80, 0x0c, 0x80, 0, 100, 10, 20, 1}
	if !bytes.Equal(got, want) {
		t.Errorf("MoveTo rate = %x, want %x", got, want)
	}
	// Timed form: discriminator 1+7 = 8, 11 bytes, easing
	// ease_out_cubic = index 2, flags heightRelative|trackTarget.
	b, err = EncodeCameraMoveTo(3200, 3200, 100, CameraMotionCycles(50, CameraEasingEaseOutCubic, true, false, true))
	got = cameraBytes(t, b, err)
	want = []byte{137, 11, 8, 0x0c, 0x80, 0x0c, 0x80, 0, 100, 0, 50, 2, 5}
	if !bytes.Equal(got, want) {
		t.Errorf("MoveTo timed = %x, want %x", got, want)
	}
}

func TestEncodeCameraLookAtDiscriminators(t *testing.T) {
	b, err := EncodeCameraLookAt(3200, 3200, 0, CameraMotionBool(true))
	got := cameraBytes(t, b, err)
	if got[2] != 2 {
		t.Errorf("LookAt instant discriminator = %d, want 2", got[2])
	}
	b, err = EncodeCameraLookAt(3200, 3200, 0, CameraMotionRates(1, 2, false))
	got = cameraBytes(t, b, err)
	if got[2] != 5 {
		t.Errorf("LookAt rate discriminator = %d, want 5", got[2])
	}
	b, err = EncodeCameraLookAt(3200, 3200, 0, CameraMotionCycles(10, CameraEasingLinear, false, true, false))
	got = cameraBytes(t, b, err)
	if got[2] != 9 || got[len(got)-1] != 2 {
		t.Errorf("LookAt timed = %x, want discriminator 9 flags 2", got)
	}
}

func TestEncodeCameraRotateToCoordinate(t *testing.T) {
	b, err := EncodeCameraRotateToCoordinate(3200, 3200, 100, 30, CameraEasingLinear)
	got := cameraBytes(t, b, err)
	// Distinct timed discriminator 13, not the look-at 9.
	want := []byte{137, 11, 13, 0x0c, 0x80, 0x0c, 0x80, 0, 100, 0, 30, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("RotateToCoordinate = %x, want %x", got, want)
	}
}

func TestEncodeCameraRotateBy(t *testing.T) {
	b, err := EncodeCameraRotateBy(-100, 200, 60, CameraEasingEaseInSine)
	got := cameraBytes(t, b, err)
	want := []byte{137, 8, 10, 0xff, 0x9c, 0, 200, 0, 60, 4}
	if !bytes.Equal(got, want) {
		t.Errorf("RotateBy = %x, want %x", got, want)
	}
}

func TestEncodeCameraMoveArc(t *testing.T) {
	b, err := EncodeCameraMoveArc(3200, 3200, 3210, 3210, -50, 100, CameraEasingLinear, true)
	got := cameraBytes(t, b, err)
	// ignoreTerrain writes flag 2, not 1.
	want := []byte{137, 15, 11, 0x0c, 0x80, 0x0c, 0x80, 0x0c, 0x8a, 0x0c, 0x8a, 0xff, 0xce, 0, 100, 0, 2}
	if !bytes.Equal(got, want) {
		t.Errorf("MoveArc = %x, want %x", got, want)
	}
	b, err = EncodeCameraMoveArc(0, 0, 0, 0, 0, 1, CameraEasingLinear, false)
	got = cameraBytes(t, b, err)
	if got[len(got)-1] != 0 {
		t.Errorf("MoveArc no-ignore flag = %d, want 0", got[len(got)-1])
	}
}

func TestEncodeCameraUnlockSkyboxTranslate(t *testing.T) {
	if got := EncodeCameraUnlock(true); !bytes.Equal(got, []byte{137, 2, 6, 1}) {
		t.Errorf("Unlock true = %x", got)
	}
	if got := EncodeCameraUnlock(false); !bytes.Equal(got, []byte{137, 2, 6, 0}) {
		t.Errorf("Unlock false = %x", got)
	}
	b, err := EncodeCameraSkybox(-1)
	got := cameraBytes(t, b, err)
	if !bytes.Equal(got, []byte{137, 5, 14, 0xff, 0xff, 0xff, 0xff}) {
		t.Errorf("Skybox -1 = %x", got)
	}
	b, err = EncodeCameraSkybox(12345)
	got = cameraBytes(t, b, err)
	if !bytes.Equal(got, []byte{137, 5, 14, 0, 0, 0x30, 0x39}) {
		t.Errorf("Skybox 12345 = %x", got)
	}
	b, err = EncodeCameraTranslate(-1000, 2000)
	got = cameraBytes(t, b, err)
	if !bytes.Equal(got, []byte{137, 9, 7, 0xff, 0xff, 0xfc, 0x18, 0, 0, 0x07, 0xd0}) {
		t.Errorf("Translate = %x", got)
	}
}

func TestEncodeCameraTarget(t *testing.T) {
	cases := []struct {
		name   string
		target CameraTarget
		want   []byte
	}{
		{"player", CameraTarget{Kind: CameraTargetPlayer, Index: 7}, []byte{137, 4, 12, 1, 0, 7}},
		{"npc", CameraTarget{Kind: CameraTargetNPC, Index: 3}, []byte{137, 4, 12, 2, 0, 3}},
		{"worldEntity", CameraTarget{Kind: CameraTargetWorldEntity, Index: 1}, []byte{137, 4, 12, 3, 0, 1}},
		{"tile", CameraTarget{Kind: CameraTargetTile, X: 3200, Y: 3200, Level: 5}, []byte{137, 7, 12, 0, 0x0c, 0x80, 0x0c, 0x80, 1}},
		{"tileGlide", CameraTarget{Kind: CameraTargetTile, X: 3200, Y: 3200, Glide: true, Cycles: 40}, []byte{137, 9, 12, 4, 0x0c, 0x80, 0x0c, 0x80, 0, 0, 40}},
		{"tileGlideNegative", CameraTarget{Kind: CameraTargetTile, X: 1, Y: 2, Glide: true, Cycles: -9}, []byte{137, 9, 12, 4, 0, 1, 0, 2, 0, 0, 0}},
	}
	for _, tc := range cases {
		if got := EncodeCameraTarget(tc.target); !bytes.Equal(got, tc.want) {
			t.Errorf("Target %s = %x, want %x", tc.name, got, tc.want)
		}
	}
}

func TestEncodeCameraReset(t *testing.T) {
	if got := EncodeCameraReset(); !bytes.Equal(got, []byte{137, 1, 0}) {
		t.Errorf("Reset = %x, want [137 1 0]", got)
	}
}

func TestEncodeCameraValidation(t *testing.T) {
	// Captured forms validate; the instant form only masks.
	if _, err := EncodeCameraMoveTo(-1, 0, 0, CameraMotionRates(1, 1, false)); err == nil {
		t.Error("MoveTo rate x=-1: want error")
	}
	if _, err := EncodeCameraMoveTo(0, 0, 0, CameraMotionRates(256, 1, false)); err == nil {
		t.Error("MoveTo rate 256: want error")
	}
	if _, err := EncodeCameraMoveTo(0, 0, 0, CameraMotionCycles(0, CameraEasingLinear, false, false, false)); err == nil {
		t.Error("MoveTo cycles 0: want error")
	}
	if _, err := EncodeCameraMoveTo(0, 0, 0, CameraMotionCycles(1, "bogus", false, false, false)); err == nil {
		t.Error("MoveTo bad easing: want error")
	}
	if _, err := EncodeCameraMoveTo(-1, 0, 0, CameraMotionBool(true)); err != nil {
		t.Errorf("MoveTo instant x=-1: want no error, got %v", err)
	}
	if _, err := EncodeCameraRotateBy(0, 0, 0, CameraEasingLinear); err == nil {
		t.Error("RotateBy cycles 0: want error")
	}
	if _, err := EncodeCameraRotateBy(32768, 0, 1, CameraEasingLinear); err == nil {
		t.Error("RotateBy pitch 32768: want error")
	}
	if _, err := EncodeCameraMoveArc(0, 0, 0, 0, 0, 1, "bogus", false); err == nil {
		t.Error("MoveArc bad easing: want error")
	}
	if _, err := EncodeCameraSkybox(-2); err == nil {
		t.Error("Skybox -2: want error")
	}
	if _, err := EncodeCameraSkybox(0x80000000); err == nil {
		t.Error("Skybox 0x80000000: want error")
	}
	if _, err := EncodeCameraTranslate(0x80000000, 0); err == nil {
		t.Error("Translate 0x80000000: want error")
	}
	if _, err := EncodeCameraTranslate(-0x80000000, 0); err == nil {
		t.Error("Translate -0x80000000: want error (abs exceeds 0x7fffffff)")
	}
	if _, err := EncodeCameraRotateToCoordinate(0, 0, 0, 0, CameraEasingLinear); err == nil {
		t.Error("RotateToCoordinate cycles 0: want error")
	}
}
