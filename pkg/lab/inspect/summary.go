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
	noEdit := 0 // consecutive calls that changed no file, tracked for the longest streak
	calls := 0
	for i := range steps {
		st := &steps[i]
		// The convergence-guard nudges can ride on any kind of step, so read them
		// before the kind switch sends the non-calls away.
		if lbl := guardNudge(st.Text); lbl != "" {
			s.GuardNudges = appendUnique(s.GuardNudges, lbl)
		}
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
		calls++
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
			cmd := argField(st.Text, "command", "cmd", "script")
			switch {
			case isSearchCmd(cmd):
				st.Act = "search"
				s.Searches++
			case isShellEdit(cmd):
				// An edit made through the shell (apply_patch, sed -i, a heredoc into a
				// source file) is a real edit, invisible to a write-tool count. Bucket it
				// as an edit so the phase view and the streak see the fix, and keep the
				// tally of how many edits came this way.
				st.Act = "edit"
				s.Edits++
				s.ShellEdits++
				edited = true
			default:
				s.Shells++
				if isInstall(st.Text) {
					s.Installs++
				}
				if edited && isVerify(st.Text) {
					s.Verified = true
				}
			}
			// Reading git history is orthogonal to how the command bucketed: a `git log`
			// is a shell command that also digs in the past, and a run that keeps
			// digging is the archaeology runaway. Count it wherever it landed.
			if isHistoryCmd(cmd) {
				s.HistoryReads++
				if isHistoryProbe(cmd) {
					s.HistoryProbes++
				}
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
		// The longest run of calls that changed nothing is the size of an
		// investigation runaway: an edit resets it, anything else extends it.
		if st.Act == "edit" {
			noEdit = 0
		} else {
			noEdit++
			if noEdit > s.NoEditStreak {
				s.NoEditStreak = noEdit
			}
		}
	}
	// A run that made calls but never changed a file is pure investigation, the
	// runaway shape; flag it plainly rather than leaving a reader to infer it from
	// a zero in the edit column.
	s.ZeroEdits = calls > 0 && s.Edits == 0
	// A tool that ships a deeper read of its own behavior gets the last word.
	if notes := notesFor(tool); notes != nil {
		s.Notes = notes(prof, steps)
	}
	return s
}
