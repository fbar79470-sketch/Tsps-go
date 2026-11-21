package session

import "tsps-go/internal/protocol"

// Camera senders: the session side of PacketSender's camera
// sends (PacketSender.ts:1111-1214), porting
// sendCameraShake, sendCameraMoveTo, sendCameraLookAt,
// sendCameraRotateBy, sendCameraRotateToCoordinate,
// sendCameraMoveArc, sendCameraUnlock, sendCameraSkybox,
// sendCameraTranslate, sendCameraTarget, and
// sendCameraReset. All eleven go through sendClientPacket
// in TS — the LIVE queued path — and queue real frames
// here through SendPacket.
//
// CAMERA STATE GAP. PacketSender tracks the last
// server-requested camera mode (follow/target/controlled,
// PacketSender.ts:185-199) and exposes getCameraState /
// restoreCameraState so content can suspend and resume a
// camera scene; the senders below update it (moveTo/
// lookAt/rotateBy/rotateToCoordinate/moveArc/translate set
// controlled; target sets target — or controlled for a
// timed tile glide; reset sets follow; shake/unlock/
// skybox leave it alone). Go has no equivalent: the state
// lives on PacketSender in TS, and the Session struct
// (session.go) cannot gain the field in this phase. These
// senders are therefore stateless: they queue exactly the
// frames TS queues, but nothing records the mode, and
// getCameraState/restoreCameraState have no Go form. The
// coordinator snippet to close the gap (add a cameraState
// field to Session, port the CameraState type and the
// three methods, and set the state in each sender per the
// transition list above) is reported with this phase; no
// frame behavior is missing, only the suspend/resume
// bookkeeping.
//
// INVALID ARGUMENTS. The encoders that validate (moveTo/
// lookAt/rotateBy/rotateToCoordinate/moveArc/skybox/
// translate) return errors mirroring the TS RangeError
// throws; these senders drop the frame on error. TS would
// throw into the caller; a Go sender that panicked would
// break the tick, and no TS caller depends on the throw
// (the encoders are called with validated content values).
// The encoder tests pin the ranges.

// SendCameraShake queues a camera shake, porting
// PacketSender.sendCameraShake (PacketSender.ts:1111-1115).
// Shake is additive and does not change the camera mode.
func (s *Session) SendCameraShake(slot, randomAmplitude, sineAmplitude, sineFrequency int) {
	s.SendPacket(protocol.EncodeCameraShake(slot, randomAmplitude, sineAmplitude, sineFrequency))
}

// SendCameraMoveTo queues a camera move-to, porting
// PacketSender.sendCameraMoveTo (PacketSender.ts:1117-1121).
// TS defaults motion to true (the instant form); Go
// callers pass the motion explicitly.
func (s *Session) SendCameraMoveTo(x, y, height int, motion protocol.CameraMotion) {
	if b, err := protocol.EncodeCameraMoveTo(x, y, height, motion); err == nil {
		s.SendPacket(b)
	}
}

// SendCameraLookAt queues a camera look-at, porting
// PacketSender.sendCameraLookAt (PacketSender.ts:1123-1127).
func (s *Session) SendCameraLookAt(x, y, height int, motion protocol.CameraMotion) {
	if b, err := protocol.EncodeCameraLookAt(x, y, height, motion); err == nil {
		s.SendPacket(b)
	}
}

// SendCameraRotateBy queues a camera rotate-by, porting
// PacketSender.sendCameraRotateBy (PacketSender.ts:1129-1133).
// TS defaults the easing to linear; Go callers pass it.
func (s *Session) SendCameraRotateBy(pitch, yaw, cycles int, easing protocol.CameraEasing) {
	if b, err := protocol.EncodeCameraRotateBy(pitch, yaw, cycles, easing); err == nil {
		s.SendPacket(b)
	}
}

// SendCameraRotateToCoordinate queues a camera
// rotate-to-coordinate, porting
// PacketSender.sendCameraRotateToCoordinate
// (PacketSender.ts:1135-1139).
func (s *Session) SendCameraRotateToCoordinate(x, y, height, cycles int, easing protocol.CameraEasing) {
	if b, err := protocol.EncodeCameraRotateToCoordinate(x, y, height, cycles, easing); err == nil {
		s.SendPacket(b)
	}
}

// SendCameraMoveArc queues a camera move-arc, porting
// PacketSender.sendCameraMoveArc (PacketSender.ts:1142-1147).
func (s *Session) SendCameraMoveArc(centreX, centreY, x, y, height, cycles int, easing protocol.CameraEasing, ignoreTerrain bool) {
	if b, err := protocol.EncodeCameraMoveArc(centreX, centreY, x, y, height, cycles, easing, ignoreTerrain); err == nil {
		s.SendPacket(b)
	}
}

// SendCameraUnlock queues a camera unlock, porting
// PacketSender.sendCameraUnlock (PacketSender.ts:1149-1153).
// It changes only the scripted pitch constraint, not the
// camera mode.
func (s *Session) SendCameraUnlock(unlock bool) {
	s.SendPacket(protocol.EncodeCameraUnlock(unlock))
}

// SendCameraSkybox queues a skybox model, porting
// PacketSender.sendCameraSkybox (PacketSender.ts:1156-1159).
// -1 clears the skybox; cam_reset leaves it in place.
func (s *Session) SendCameraSkybox(model int) {
	if b, err := protocol.EncodeCameraSkybox(model); err == nil {
		s.SendPacket(b)
	}
}

// SendCameraTranslate queues a camera translation, porting
// PacketSender.sendCameraTranslate (PacketSender.ts:1161-1165).
func (s *Session) SendCameraTranslate(dx, dy int) {
	if b, err := protocol.EncodeCameraTranslate(dx, dy); err == nil {
		s.SendPacket(b)
	}
}

// SendCameraTarget queues a camera target, porting
// PacketSender.sendCameraTarget (PacketSender.ts:1198-1208).
// The target follows a tile, a player, an NPC, or a world
// entity (a boat) until SendCameraReset.
func (s *Session) SendCameraTarget(target protocol.CameraTarget) {
	s.SendPacket(protocol.EncodeCameraTarget(target))
}

// SendCameraReset queues a camera reset, porting
// PacketSender.sendCameraReset (PacketSender.ts:1210-1214).
// It puts the camera back on the player and stops any shake.
func (s *Session) SendCameraReset() {
	s.SendPacket(protocol.EncodeCameraReset())
}
