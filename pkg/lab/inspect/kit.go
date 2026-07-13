package inspect

// The tools sub-package writes each tool's own reading of its transcript. A notes
// function reads the same signals the generic analysis does, so this exposes the
// small vocabulary it needs as a stable surface: the file a call touched, whether
// that path is a test file, and the count-noun and substring helpers the note
// strings are built from. Keeping these here means a notes author never reaches
// into the package internals and the two stay decoupled.

// ArgPath returns the file path a call's JSON arguments name, or empty if none.
func ArgPath(args string) string { return argPath(args) }

// ArgField returns the first present string field among keys in a call's JSON
// arguments, used to name the command a shell ran or the pattern a search used.
func ArgField(args string, keys ...string) string { return argField(args, keys...) }

// IsTestPath reports whether a path lives in a test tree or is named like a test
// file, so a notes function can tell a source fix from a test edit.
func IsTestPath(path string) bool { return testPathRe.MatchString(path) }

// CountNoun renders a count with the matching noun: "1 file", "3 files".
func CountNoun(n int, singular, plural string) string { return countNoun(n, singular, plural) }

// ContainsAny reports whether s contains any of the given substrings.
func ContainsAny(s string, subs ...string) bool { return containsAny(s, subs...) }
