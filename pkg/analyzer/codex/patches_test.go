package codex

import (
	"strings"
	"testing"
)

// A rollout with two apply_patch writes yields two patches, each with its files
// and op read from the envelope markers, and the raw body kept intact.
const patchRollout = `
{"timestamp":"2026-07-07T15:50:23.001Z","type":"session_meta","payload":{"session_id":"s","cwd":"/work","cli_version":"0.144.1"}}
{"timestamp":"2026-07-07T15:50:24.000Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","call_id":"c1","input":"*** Begin Patch\n*** Update File: /work/control/xferfcn.py\n@@\n-def zpk(zeros, poles, gain, dt=None, **kwargs):\n+def zpk(zeros, poles, gain, *args, **kwargs):\n*** End Patch","internal_chat_message_metadata_passthrough":{"turn_id":"t1"}}}
{"timestamp":"2026-07-07T15:50:25.000Z","type":"response_item","payload":{"type":"exec_command","name":"exec_command","call_id":"c2","input":"ls","internal_chat_message_metadata_passthrough":{"turn_id":"t1"}}}
{"timestamp":"2026-07-07T15:50:26.000Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","call_id":"c3","input":"*** Begin Patch\n*** Add File: /work/newmod.py\n+print('hi')\n*** Delete File: /work/old.py\n*** End Patch","internal_chat_message_metadata_passthrough":{"turn_id":"t1"}}}
`

func TestPatches(t *testing.T) {
	r, err := ParseRollout(strings.NewReader(patchRollout))
	if err != nil {
		t.Fatalf("ParseRollout: %v", err)
	}
	ps := r.Patches()
	if len(ps) != 2 {
		t.Fatalf("patches = %d, want 2 (the exec item is not a write)", len(ps))
	}
	if len(ps[0].Files) != 1 || ps[0].Files[0] != "/work/control/xferfcn.py" || ps[0].Ops[0] != "update" {
		t.Errorf("patch 0 files/ops = %v/%v, want one update of xferfcn.py", ps[0].Files, ps[0].Ops)
	}
	if !strings.Contains(ps[0].Body, "*args, **kwargs") {
		t.Errorf("patch 0 body did not keep the diff: %q", ps[0].Body)
	}
	if len(ps[1].Files) != 2 || ps[1].Ops[0] != "add" || ps[1].Ops[1] != "delete" {
		t.Errorf("patch 1 files/ops = %v/%v, want add then delete", ps[1].Files, ps[1].Ops)
	}
}

func TestPatchFilesEmpty(t *testing.T) {
	files, ops := patchFiles("not a patch at all")
	if files != nil || ops != nil {
		t.Errorf("no markers should yield no files/ops, got %v/%v", files, ops)
	}
}
