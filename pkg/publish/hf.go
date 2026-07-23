// Package publish mirrors a tomo-labs run to a Hugging Face dataset. It turns a
// run's captured trace into the Hub's Session Trace Simple Format, regenerates
// the dataset README and the analysis reports from every result on disk, and
// commits the lot to open-index/tomo-traces over the Hub's HTTP API with no
// Hugging Face SDK dependency.
//
// The HF client here is ported from the arctic-cli dataset publisher: a
// bearer-token HTTP client that creates a dataset repo, uploads large files
// through git-LFS, and posts a commit. It carries no SDK so the labs binary
// stays a single self-contained tool.
package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// hfEndpoint is the Hugging Face hub. Datasets live under /datasets/{repo}.
const hfEndpoint = "https://huggingface.co"

// lfsThreshold is the size above which a file goes through LFS rather than
// inline base64 in the commit. Every trace and report the publisher writes is
// far below this, so in practice the LFS path is dormant; it is kept because the
// arctic client already has it and a large future artifact would need it.
const lfsThreshold = 10 * 1024 * 1024

// HFClient talks to the Hugging Face dataset API over plain HTTP with a bearer
// token. It carries no SDK dependency.
type HFClient struct {
	Token string
	Repo  string // "namespace/name"
	HTTP  *http.Client

	// Message overrides the commit summary for the next commit when set; the
	// publisher sets it to a run headline before each UploadFiles. When empty,
	// postCommit falls back to a generic summary derived from the file set.
	Message string
}

// NewHFClient builds a client for repo authenticated with token.
func NewHFClient(token, repo string) *HFClient {
	return &HFClient{
		Token: token,
		Repo:  repo,
		HTTP:  &http.Client{Timeout: 30 * time.Minute},
	}
}

// HFOp is one file operation in a commit: add or update a file, or delete one.
// A file may be given inline as Content, or by LocalPath to read from disk; the
// publisher builds its ops with Content since it generates every file in memory.
type HFOp struct {
	LocalPath  string // source on disk, read when Content is nil and Delete is false
	Content    []byte // in-memory file body, used in preference to LocalPath
	PathInRepo string // destination path in the dataset repo
	Delete     bool
}

// hfError classifies a hub failure so the publish pipeline knows whether to
// retry in place, resume, back off, or stop.
type hfError struct {
	kind string // "corruption" | "transient" | "ratelimit" | "connreset" | "fatal"
	msg  string
}

func (e *hfError) Error() string { return e.msg }

// IsRateLimit reports whether err is a hub rate-limit response.
func IsRateLimit(err error) bool {
	var he *hfError
	return errors.As(err, &he) && he.kind == "ratelimit"
}

// IsFatal reports whether err is a hub failure no retry can fix (auth, a bad
// request), as opposed to a transient one worth retrying.
func IsFatal(err error) bool {
	var he *hfError
	return errors.As(err, &he) && he.kind == "fatal"
}

func classifyHTTP(status int, body string) error {
	switch {
	case status == 429:
		return &hfError{kind: "ratelimit", msg: fmt.Sprintf("hf rate limited: %s", body)}
	case status >= 500:
		return &hfError{kind: "transient", msg: fmt.Sprintf("hf server error %d: %s", status, body)}
	case status == 401 || status == 403:
		return &hfError{kind: "fatal", msg: fmt.Sprintf("hf auth error %d: %s", status, body)}
	default:
		return &hfError{kind: "fatal", msg: fmt.Sprintf("hf error %d: %s", status, body)}
	}
}

func classifyNet(err error) error {
	if err == nil {
		return nil
	}
	s := err.Error()
	if strings.Contains(s, "connection reset") || strings.Contains(s, "EOF") {
		return &hfError{kind: "connreset", msg: err.Error()}
	}
	return &hfError{kind: "transient", msg: err.Error()}
}

func (c *HFClient) do(req *http.Request) (*http.Response, []byte, error) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "tomo-labs")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, classifyNet(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, body, nil
}

// createRepoPayload builds the /api/repos/create body for repo. The API rejects
// a slash in name, so a "namespace/name" repo is split into organization and
// bare name; a plain name (no namespace) is sent as is and lands under the
// token's own account.
func createRepoPayload(repo string, private bool) map[string]any {
	payload := map[string]any{
		"type":    "dataset",
		"name":    repo,
		"private": private,
	}
	if ns, name, ok := strings.Cut(repo, "/"); ok {
		payload["organization"] = ns
		payload["name"] = name
	}
	return payload
}

// CreateDatasetRepo creates the dataset repo, treating an "already exists"
// response as success so the call is idempotent.
func (c *HFClient) CreateDatasetRepo(ctx context.Context, private bool) error {
	b, _ := json.Marshal(createRepoPayload(c.Repo, private))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		hfEndpoint+"/api/repos/create", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, body, err := c.do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 == 2 {
		return nil
	}
	if resp.StatusCode == 409 || strings.Contains(strings.ToLower(string(body)), "already") {
		return nil
	}
	return classifyHTTP(resp.StatusCode, string(body))
}

// commitByteCap bounds how many bytes of file content one commit carries before
// UploadFiles flushes it and starts another. It exists so a single publish is a
// single commit at normal scale (a run's trace plus the regenerated front matter
// is a handful of small files, well under the cap, so it commits once), while a
// pathologically large backfill still gets split rather than posting one huge
// NDJSON body. The cap is on content bytes, not the larger base64 the commit
// actually sends, which is a deliberate slack so the real body stays a few times
// under any hub request limit.
const commitByteCap = 32 * 1024 * 1024

// UploadFiles commits ops to the dataset's main branch. It packs as many ops as
// fit under commitByteCap into each commit so a normal publish lands as one
// atomic commit rather than one commit per fixed-size batch, which used to spam
// the dataset history with a run of identical-summary commits. Each commit is
// retried three times. Files above lfsThreshold go through LFS; smaller files
// are inlined in the commit.
func (c *HFClient) UploadFiles(ctx context.Context, ops []HFOp) error {
	for i := 0; i < len(ops); {
		// Grow the batch until the next op would push it past the byte cap, always
		// taking at least one op so a single file larger than the cap still commits.
		end, size := i, 0
		for end < len(ops) {
			n := len(ops[end].Content)
			if end > i && size+n > commitByteCap {
				break
			}
			size += n
			end++
		}
		batch := ops[i:end]
		i = end
		var lastErr error
		for attempt := 0; attempt < 3; attempt++ {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = c.commitBatch(ctx, batch)
			if lastErr == nil {
				break
			}
			if IsRateLimit(lastErr) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(30<<attempt) * time.Second):
				}
				continue
			}
			if IsFatal(lastErr) {
				return lastErr
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(5<<attempt) * time.Second):
			}
		}
		if lastErr != nil {
			return lastErr
		}
	}
	return nil
}

// preparedFile holds the data needed to put one file in a commit.
type preparedFile struct {
	op     HFOp
	data   []byte // nil when delete or when the file went through LFS
	oid    string // sha256 hex, for LFS files
	size   int64
	useLFS bool
}

// fileBody returns an op's bytes, preferring the in-memory Content and falling
// back to reading LocalPath.
func fileBody(op HFOp) ([]byte, error) {
	if op.Content != nil {
		return op.Content, nil
	}
	return os.ReadFile(op.LocalPath)
}

func (c *HFClient) commitBatch(ctx context.Context, ops []HFOp) error {
	files := make([]preparedFile, 0, len(ops))
	var lfsFiles []preparedFile
	for _, op := range ops {
		if op.Delete {
			files = append(files, preparedFile{op: op})
			continue
		}
		data, err := fileBody(op)
		if err != nil {
			return &hfError{kind: "fatal", msg: fmt.Sprintf("read %s: %v", op.PathInRepo, err)}
		}
		sum := sha256.Sum256(data)
		pf := preparedFile{
			op:     op,
			data:   data,
			oid:    hex.EncodeToString(sum[:]),
			size:   int64(len(data)),
			useLFS: len(data) >= lfsThreshold,
		}
		files = append(files, pf)
		if pf.useLFS {
			lfsFiles = append(lfsFiles, pf)
		}
	}

	// Upload LFS objects first, then reference them in the commit.
	if len(lfsFiles) > 0 {
		if err := c.uploadLFS(ctx, lfsFiles); err != nil {
			return err
		}
	}

	return c.postCommit(ctx, files)
}

// uploadLFS runs the git-lfs batch protocol against the hub: ask for upload
// actions, then PUT any object the server does not already have.
func (c *HFClient) uploadLFS(ctx context.Context, files []preparedFile) error {
	type lfsObj struct {
		OID  string `json:"oid"`
		Size int64  `json:"size"`
	}
	reqObjs := make([]lfsObj, len(files))
	for i, f := range files {
		reqObjs[i] = lfsObj{OID: f.oid, Size: f.size}
	}
	batchReq := map[string]any{
		"operation": "upload",
		"transfers": []string{"basic"},
		"objects":   reqObjs,
		"hash_algo": "sha256",
	}
	b, _ := json.Marshal(batchReq)
	url := fmt.Sprintf("%s/datasets/%s.git/info/lfs/objects/batch", hfEndpoint, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	req.Header.Set("Accept", "application/vnd.git-lfs+json")
	resp, body, err := c.do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return classifyHTTP(resp.StatusCode, string(body))
	}

	var batchResp struct {
		Objects []struct {
			OID     string `json:"oid"`
			Actions struct {
				Upload *struct {
					Href   string            `json:"href"`
					Header map[string]string `json:"header"`
				} `json:"upload"`
			} `json:"actions"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(body, &batchResp); err != nil {
		return &hfError{kind: "transient", msg: fmt.Sprintf("lfs batch decode: %v", err)}
	}

	byOID := make(map[string]preparedFile, len(files))
	for _, f := range files {
		byOID[f.oid] = f
	}
	for _, o := range batchResp.Objects {
		if o.Error != nil {
			return &hfError{kind: "fatal", msg: fmt.Sprintf("lfs object %s: %s", o.OID, o.Error.Message)}
		}
		if o.Actions.Upload == nil {
			continue // the server already has this object
		}
		f := byOID[o.OID]
		put, err := http.NewRequestWithContext(ctx, http.MethodPut,
			o.Actions.Upload.Href, bytes.NewReader(f.data))
		if err != nil {
			return err
		}
		for k, v := range o.Actions.Upload.Header {
			put.Header.Set(k, v)
		}
		put.ContentLength = f.size
		// The upload action hits storage directly, so this request carries its
		// own auth headers and must not get our hub bearer token.
		resp, body, derr := doRaw(c.HTTP, put)
		if derr != nil {
			return classifyNet(derr)
		}
		if resp.StatusCode/100 != 2 {
			return classifyHTTP(resp.StatusCode, string(body))
		}
	}
	return nil
}

func doRaw(client *http.Client, req *http.Request) (*http.Response, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, body, nil
}

// postCommit sends the NDJSON commit: a header line, then one line per file
// (LFS reference, inline base64, or delete).
func (c *HFClient) postCommit(ctx context.Context, files []preparedFile) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	summary, description := commitMessage(files)
	if c.Message != "" {
		summary = c.Message
	}
	header := map[string]any{
		"key": "header",
		"value": map[string]any{
			"summary":     summary,
			"description": description,
		},
	}
	if err := enc.Encode(header); err != nil {
		return fmt.Errorf("encode commit header: %w", err)
	}

	for _, f := range files {
		if f.op.Delete {
			if err := enc.Encode(map[string]any{
				"key":   "deletedFile",
				"value": map[string]any{"path": f.op.PathInRepo},
			}); err != nil {
				return fmt.Errorf("encode deleted file: %w", err)
			}
			continue
		}
		if f.useLFS {
			if err := enc.Encode(map[string]any{
				"key": "lfsFile",
				"value": map[string]any{
					"path": f.op.PathInRepo,
					"oid":  f.oid,
					"size": f.size,
				},
			}); err != nil {
				return fmt.Errorf("encode lfs file: %w", err)
			}
			continue
		}
		if err := enc.Encode(map[string]any{
			"key": "file",
			"value": map[string]any{
				"path":     f.op.PathInRepo,
				"content":  base64.StdEncoding.EncodeToString(f.data),
				"encoding": "base64",
			},
		}); err != nil {
			return fmt.Errorf("encode file: %w", err)
		}
	}

	url := fmt.Sprintf("%s/api/datasets/%s/commit/main", hfEndpoint, c.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	resp, body, err := c.do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return classifyHTTP(resp.StatusCode, string(body))
	}
	return nil
}
