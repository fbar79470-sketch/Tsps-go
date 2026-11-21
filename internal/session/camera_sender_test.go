package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/protocol"
)

func TestCameraSendersQueueFrames(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendCameraShake(1, 100, 200, 300)
	s.SendCameraMoveTo(3200, 3200, 100, protocol.CameraMotionBool(true))
	s.SendCameraLookAt(3200, 3200, 100, protocol.CameraMotionRates(10, 20, false))
	s.SendCameraRotateBy(10, 20, 30, protocol.CameraEasingLinear)
	s.SendCameraRotateToCoordinate(3200, 3200, 100, 30, protocol.CameraEasingLinear)
	s.SendCameraMoveArc(3200, 3200, 3210, 3210, 0, 50, protocol.CameraEasingLinear, false)
	s.SendCameraUnlock(true)
	s.SendCameraSkybox(5)
	s.SendCameraTranslate(10, -10)
	s.SendCameraTarget(protocol.CameraTarget{Kind: protocol.CameraTargetPlayer, Index: 7})
	s.SendCameraReset()

	packets := queuedPackets(t, s)
	if len(packets) != 11 {
		t.Fatalf("queued packets = %d, want 11", len(packets))
	}
	moveTo, _ := protocol.EncodeCameraMoveTo(3200, 3200, 100, protocol.CameraMotionBool(true))
	lookAt, _ := protocol.EncodeCameraLookAt(3200, 3200, 100, protocol.CameraMotionRates(10, 20, false))
	rotateBy, _ := protocol.EncodeCameraRotateBy(10, 20, 30, protocol.CameraEasingLinear)
	rotateTo, _ := protocol.EncodeCameraRotateToCoordinate(3200, 3200, 100, 30, protocol.CameraEasingLinear)
	moveArc, _ := protocol.EncodeCameraMoveArc(3200, 3200, 3210, 3210, 0, 50, protocol.CameraEasingLinear, false)
	skybox, _ := protocol.EncodeCameraSkybox(5)
	translate, _ := protocol.EncodeCameraTranslate(10, -10)
	want := [][]byte{
		protocol.EncodeCameraShake(1, 100, 200, 300),
		moveTo,
		lookAt,
		rotateBy,
		rotateTo,
		moveArc,
		protocol.EncodeCameraUnlock(true),
		skybox,
		translate,
		protocol.EncodeCameraTarget(protocol.CameraTarget{Kind: protocol.CameraTargetPlayer, Index: 7}),
		protocol.EncodeCameraReset(),
	}
	for i, w := range want {
		if !bytes.Equal(packets[i], w) {
			t.Errorf("packet %d = %x, want %x", i, packets[i], w)
		}
		if packets[i][0] != protocol.ServerCameraControl {
			t.Errorf("packet %d opcode = %d, want 137", i, packets[i][0])
		}
	}
}

func TestCameraSendersDropInvalid(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// Out-of-range values the encoders reject queue nothing.
	s.SendCameraMoveTo(-1, 0, 0, protocol.CameraMotionRates(1, 1, false))
	s.SendCameraRotateBy(0, 0, 0, protocol.CameraEasingLinear)
	s.SendCameraSkybox(-2)
	s.SendCameraTranslate(0x80000000, 0)
	s.SendCameraMoveArc(0, 0, 0, 0, 0, 1, "bogus", false)
	if packets := queuedPackets(t, s); len(packets) != 0 {
		t.Errorf("invalid sends queued %d packets, want 0", len(packets))
	}
}
