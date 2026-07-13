package codex

import "strings"

// Patch is one apply_patch a run made: the files it touched and the raw patch
// body. The body is exactly what Codex sent, in its apply_patch envelope, so a
// reader sees the precise change rather than a count of edits.
//
// This is the part of a rollout worth learning from. A run that solved a task
// solved it in these diffs, and reading them next to what tomo did on the same
// task is how a real fix teaches a better one.
type Patch struct {
	Files []string // the files the patch adds, updates, or deletes, in order
	Ops   []string // the op per file, parallel to Files: add, update, or delete
	Body  string   // the raw apply_patch body, *** Begin Patch to *** End Patch
}

// Patches returns every apply_patch a run made, in order. Codex writes through
// apply_patch as a custom_tool_call whose input is the patch body, so each write
// item yields one Patch with its files pulled from the *** Update/Add/Delete
// File markers.
func (r *Rollout) Patches() []Patch {
	var out []Patch
	for _, it := range r.Items {
		if !it.IsWrite() {
			continue
		}
		body := it.Input
		if body == "" {
			// A function_call apply_patch carries the body as a JSON arguments
			// string; the custom_tool_call form carries it plain in Input.
			body = it.Arguments
		}
		files, ops := patchFiles(body)
		out = append(out, Patch{Files: files, Ops: ops, Body: body})
	}
	return out
}

// patchFiles reads the file markers out of an apply_patch body. The envelope
// names each file on its own line, one of "*** Update File: path", "*** Add
// File: path", or "*** Delete File: path", so a scan of those lines gives the
// files and the operation on each without applying the patch.
func patchFiles(body string) (files, ops []string) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "*** Update File:"):
			files = append(files, strings.TrimSpace(line[len("*** Update File:"):]))
			ops = append(ops, "update")
		case strings.HasPrefix(line, "*** Add File:"):
			files = append(files, strings.TrimSpace(line[len("*** Add File:"):]))
			ops = append(ops, "add")
		case strings.HasPrefix(line, "*** Delete File:"):
			files = append(files, strings.TrimSpace(line[len("*** Delete File:"):]))
			ops = append(ops, "delete")
		}
	}
	return files, ops
}
