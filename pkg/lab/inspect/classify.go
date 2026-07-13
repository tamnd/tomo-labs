package inspect

import "strings"

// Classify buckets one call name into an action: the tool's own lexicon first,
// then the verb keywords agents tend to share, so a name the lexicon does not
// list (or an untuned tool) still lands somewhere reasonable.
func (p ToolProfile) Classify(name string) string {
	if b, ok := p.Lexicon[name]; ok {
		return b
	}
	n := strings.ToLower(name)
	switch {
	case containsAny(n, "read", "view", "open", "cat", "show", "fetch_file"):
		return "read"
	case containsAny(n, "grep", "search", "find", "glob", "ls", "list"):
		return "search"
	case containsAny(n, "edit", "write", "patch", "apply", "replace", "create", "insert", "modify"):
		return "edit"
	case containsAny(n, "bash", "shell", "exec", "run", "command", "terminal", "sh"):
		return "shell"
	case containsAny(n, "plan", "todo", "task"):
		return "plan"
	default:
		return "other"
	}
}
