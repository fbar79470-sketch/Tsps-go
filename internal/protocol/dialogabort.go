package protocol

// EncodeTriggerOnDialogAbort encodes the dialog-abort trigger
// (encodeTriggerOnDialogAbort, ClientProtocol.ts:2321-2323):
// TRIGGER_ONDIALOGABORT (opcode 123) with an empty payload. It
// runs every interface's onDialogAbort, closing chatbox prompts
// without answering them; PacketSender.abortInputPrompt sends it
// when a prompt is armed (PacketSender.ts:466-474). The
// length-table entry for opcode 123 is 0, so mustEncode cannot
// fail.
func EncodeTriggerOnDialogAbort() []byte {
	return mustEncode(ServerTriggerOndialogabort, nil)
}
