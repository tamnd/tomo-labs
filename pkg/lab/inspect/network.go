package inspect

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// testPathRe matches a path that lives in a test tree or is named like a test
// file, so a run that edits one (when the task said to fix the source, not the
// tests) can be split out. It is deliberately broad: any tests/ or test/
// directory segment, or a file named test_*.py / *_test.* .
var testPathRe = regexp.MustCompile(`(^|/)tests?/|(^|/)test_[^/]*$|_test\.[a-z]+$|\.test\.[a-z]+$`)

// argPath pulls a file path out of a call's JSON arguments, trying the field
// names different tools use, so the summary can name the files a run touched.
func argPath(args string) string {
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return ""
	}
	for _, k := range []string{"path", "file", "file_path", "filePath", "filename", "fileName", "filepath", "target_file"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// argField pulls the first present string field out of a call's JSON arguments,
// used to name the pattern a search ran or the command a shell executed.
func argField(args string, keys ...string) string {
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// isInstall reports whether a shell command is bootstrapping the environment
// rather than doing the task, so a run that pays that tax is called out. With the
// prepared env in place this should be rare, and a spike is worth seeing.
func isInstall(cmd string) bool {
	return containsAny(strings.ToLower(cmd),
		"pip install", "pip3 install", "uv pip", "apt-get install", "apt install",
		"npm install", "npm i ", "yarn add", "poetry install", "conda install",
		"break-system-packages")
}

// isVerify reports whether a command is the agent checking its own work: running
// the tests, or at least confirming the edited file still parses.
func isVerify(cmd string) bool {
	return containsAny(strings.ToLower(cmd),
		"pytest", "unittest", "python -m test", "go test", "npm test", "npm run test",
		"ast.parse", "py_compile", "tox", "make test", "./run_tests")
}

// isSearchCmd reports whether a shell command is really a search: a grep, find,
// or listing whose point is to locate code, not to change or test it. A bash-only
// agent has no other way to search, so recognizing these keeps its summary honest
// about how much of its work was orienting itself. It strips leading `cd … &&`
// prefixes and matches on the first real command word.
func isSearchCmd(cmd string) bool {
	tok := firstWord(cmd)
	switch tok {
	case "grep", "egrep", "fgrep", "rg", "ag", "ack", "find", "fd", "ls", "locate", "tree", "glob":
		return true
	}
	return false
}

// isNetworkTool reports whether a call name is a web tool: a fetch, a web search,
// or a browser move. These are the named ways an agent leaves the repo, as
// opposed to a curl smuggled through the shell, which isNetworkCmd catches.
func isNetworkTool(name string) bool {
	n := strings.ToLower(name)
	switch n {
	case "fetch", "webfetch", "web_fetch", "websearch", "web_search", "browse", "browser", "url_fetch":
		return true
	}
	// A few tools prefix or suffix these, e.g. web.run or fetch_url; keep the match
	// broad enough to catch them without swallowing fetch_file, which reads locally.
	if strings.Contains(n, "file") {
		return false
	}
	return containsAny(n, "webfetch", "websearch", "web_", "fetch_url", "url_fetch")
}

// isNetworkCmd reports whether a shell command reaches out over the network: a
// curl, wget, or a raw http client. It reads the first real command word after
// any leading `cd … &&`, the same way isSearchCmd does, so a run that fetches
// through the shell is counted next to one that uses a web tool.
func isNetworkCmd(cmd string) bool {
	switch firstWord(cmd) {
	case "curl", "wget", "http", "https", "httpie", "xh", "lynx", "w3m", "aria2c":
		return true
	}
	return false
}

// firstWord returns the first real command word of a shell command, stripping any
// leading `cd … &&` prefixes so the command that actually runs is what is tested.
func firstWord(cmd string) string {
	c := strings.TrimSpace(strings.ToLower(cmd))
	for strings.HasPrefix(c, "cd ") {
		i := strings.Index(c, "&&")
		if i < 0 {
			break
		}
		c = strings.TrimSpace(c[i+2:])
	}
	if i := strings.IndexAny(c, " \t"); i >= 0 {
		return c[:i]
	}
	return c
}

// fetchURL pulls the URL a network call went to, from the web tool's url field
// or from the first http(s) token in a shell command, so the host can be named.
func fetchURL(s Step) string {
	if u := argField(s.Text, "url", "uri", "link", "href", "target"); u != "" {
		return u
	}
	if cmd := argField(s.Text, "command", "cmd", "script"); cmd != "" {
		return firstURL(cmd)
	}
	return firstURL(s.Text)
}

// urlRe matches an http(s) URL, used to lift the target out of a shell command
// that has no structured url field.
var urlRe = regexp.MustCompile(`https?://[^\s"'` + "`" + `)]+`)

// firstURL returns the first http(s) URL in a string, or empty if none.
func firstURL(s string) string { return urlRe.FindString(s) }

// urlHost reduces a URL to its host, so distinct pages on the same site collapse
// to one entry and a run that hammered one host is not listed a hundred times. It
// is deliberately forgiving: no scheme, a port, or a trailing path all still
// yield the bare host, and a string with no host yields empty.
func urlHost(u string) string {
	if u == "" {
		return ""
	}
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.LastIndex(u, "@"); i >= 0 {
		u = u[i+1:]
	}
	if i := strings.Index(u, ":"); i >= 0 {
		u = u[:i]
	}
	return u
}

// resultIsError reports whether a tool result carried a failure the agent had to
// work around, so a run that fought errors reads differently from one that did
// not. It looks for the markers tools and languages print on failure.
func resultIsError(out string) bool {
	return containsAny(out, "Traceback (most recent call last)", "fatal:", "ERROR:", "command not found", "No such file", "Error:")
}

// -- small text helpers, kept local so the summary reads cleanly ----------------

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func appendUnique(xs []string, x string) []string {
	for _, e := range xs {
		if e == x {
			return xs
		}
	}
	return append(xs, x)
}

func countNoun(n int, singular, plural string) string {
	if n == 1 {
		return strconv.Itoa(n) + " " + singular
	}
	return strconv.Itoa(n) + " " + plural
}

// comma groups an integer with thousands separators for readability.
func comma(n int) string {
	if n < 0 {
		return "-" + comma(-n)
	}
	s := strconv.Itoa(n)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

// joinList renders a list as prose: "a", "a and b", "a, b, and c".
func joinList(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	case 2:
		return xs[0] + " and " + xs[1]
	default:
		return strings.Join(xs[:len(xs)-1], ", ") + ", and " + xs[len(xs)-1]
	}
}
