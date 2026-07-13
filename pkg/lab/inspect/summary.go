package inspect

// Analyze reads the step list into a summary: what kind of move each call was,
// which files were touched, and the marks a run leaves when it repeats itself,
// bootstraps its environment, leaves the repo, hits errors, or checks its own
// work. Every judgment is keyword-based on the call name and arguments, which
// keeps it tool-agnostic: one tool's `read` and another's `view_file` land in the
// same bucket. It also tags each call step with its action bucket, so the
// walkthrough can render the move without re-classifying it.
func Analyze(tool string, prof ToolProfile, steps []Step) *RunSummary {
	s := &RunSummary{}
	seen := map[string]bool{}
	edited := false
	for i := range steps {
		st := &steps[i]
		switch st.Kind {
		case "assistant":
			s.Thoughts++
			continue
		case "result":
			if resultIsError(st.Text) {
				s.Errors++
			}
			continue
		case "call":
			// handled below
		default:
			continue
		}
		st.Act = prof.Classify(st.Name)
		switch st.Act {
		case "read":
			s.Reads++
			if f := argPath(st.Text); f != "" {
				s.FilesRead = appendUnique(s.FilesRead, f)
			}
		case "search":
			s.Searches++
		case "edit":
			s.Edits++
			edited = true
			// A fix belongs in the source, not the tests, and the grader resets the
			// test tree before it grades, so an edit to a test file is worth keeping
			// apart from the source change rather than piling both into one list.
			if f := argPath(st.Text); f != "" {
				if testPathRe.MatchString(f) {
					s.TestEdits = appendUnique(s.TestEdits, f)
				} else {
					s.FilesEdit = appendUnique(s.FilesEdit, f)
				}
			}
		case "shell":
			// A tool with no dedicated search runs its greps and finds through the
			// shell; count those as the searches they are, and keep only the real
			// commands as shell, so a bash-only agent's summary is not one big
			// undifferentiated pile of shell calls.
			if cmd := argField(st.Text, "command", "cmd", "script"); isSearchCmd(cmd) {
				st.Act = "search"
				s.Searches++
				break
			}
			s.Shells++
			if isInstall(st.Text) {
				s.Installs++
			}
			if edited && isVerify(st.Text) {
				s.Verified = true
			}
		case "plan":
			s.Plans++
		default:
			s.Other++
		}
		// Network activity cuts across the buckets: a web tool lands in "other" and a
		// curl lands in "shell", but both leave the repo, which is the move worth
		// seeing on a task graded only on local files. Count it once, orthogonally,
		// and remember the hosts so a run that went looking for the answer online is
		// visible without re-reading the raw trace.
		if isNetworkTool(st.Name) || (st.Act == "shell" && isNetworkCmd(argField(st.Text, "command", "cmd", "script"))) {
			s.Fetches++
			if h := urlHost(fetchURL(*st)); h != "" {
				s.FetchHosts = appendUnique(s.FetchHosts, h)
			}
		}
		if seen[st.Name+st.Text] {
			s.Repeated++
		}
		seen[st.Name+st.Text] = true
	}
	// A tool that ships a deeper read of its own behavior gets the last word.
	if notes := notesFor(tool); notes != nil {
		s.Notes = notes(prof, steps)
	}
	return s
}
