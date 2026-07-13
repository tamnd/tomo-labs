package codex

import (
	"encoding/json"
	"regexp"
)

// Codex runs shell commands through a small JavaScript shim rather than a plain
// argv. A custom_tool_call named "exec" carries an Input like
//
//	const r = await tools.exec_command({"cmd":"pytest -q","workdir":"/w"});
//	text(JSON.stringify(r));
//
// so the command the model actually ran is the "cmd" field of the object passed
// to tools.exec_command, not the whole Input string. Older or differently shaped
// traces put the command in a function_call whose Arguments is a JSON object with
// a "command" array. Commands pulls the real command line out of either shape so
// the rest of the analyzer, the leak audit above all, reads what the run did and
// not the wrapper around it.

// execCmdRe pulls the first string argument object out of a tools.exec_command or
// tools.shell call in the JS shim. It is deliberately loose about whitespace so a
// reformatted shim still matches, and it stops at the object's closing brace.
var execCmdRe = regexp.MustCompile(`tools\.(?:exec_command|shell)\(\s*(\{.*?\})\s*\)`)

// Command returns the shell command an exec tool call ran, or empty if the item
// is not one or carries no command. It first tries the JS shim's exec_command
// object, then falls back to a function_call whose Arguments is {"command": [...]}
// or {"cmd": "..."}. write_stdin and other non-shell calls yield empty.
func (it ResponseItem) Command() string {
	if !it.IsToolCall() {
		return ""
	}
	if it.Input != "" {
		if cmd := commandFromShim(it.Input); cmd != "" {
			return cmd
		}
	}
	if it.Arguments != "" {
		if cmd := commandFromArgs(it.Arguments); cmd != "" {
			return cmd
		}
	}
	return ""
}

// commandFromShim extracts the cmd string from a tools.exec_command JS-shim body.
// It parses the object argument as JSON, so an escaped command is unescaped the
// same way Codex escaped it, rather than read raw with the backslashes still in.
func commandFromShim(input string) string {
	m := execCmdRe.FindStringSubmatch(input)
	if m == nil {
		return ""
	}
	var obj struct {
		Cmd     string   `json:"cmd"`
		Command []string `json:"command"`
	}
	if json.Unmarshal([]byte(m[1]), &obj) != nil {
		return ""
	}
	if obj.Cmd != "" {
		return obj.Cmd
	}
	return joinArgv(obj.Command)
}

// commandFromArgs extracts the command from a function_call Arguments JSON, which
// is either a "cmd" string or a "command" argv array. A leading shell -c is kept
// as-is, since the leak audit reads the whole line anyway.
func commandFromArgs(args string) string {
	var obj struct {
		Cmd     string   `json:"cmd"`
		Command []string `json:"command"`
	}
	if json.Unmarshal([]byte(args), &obj) != nil {
		return ""
	}
	if obj.Cmd != "" {
		return obj.Cmd
	}
	return joinArgv(obj.Command)
}

// joinArgv turns a ["bash","-lc","pytest -q"] argv into a single line. When the
// last element is the script of a shell -c, that script is the command that
// matters, so it is returned alone; otherwise the parts are space-joined.
func joinArgv(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	if len(argv) >= 2 {
		switch argv[len(argv)-2] {
		case "-c", "-lc", "-lic", "-ic":
			return argv[len(argv)-1]
		}
	}
	out := argv[0]
	for _, a := range argv[1:] {
		out += " " + a
	}
	return out
}

// Commands returns every shell command the run issued, in order, skipping tool
// calls that ran no command (apply_patch, write_stdin). The leak audit reads this
// list; a report can print it to show exactly what the run did in the shell.
func (r *Rollout) Commands() []string {
	var cmds []string
	for _, it := range r.Items {
		if c := it.Command(); c != "" {
			cmds = append(cmds, c)
		}
	}
	return cmds
}
