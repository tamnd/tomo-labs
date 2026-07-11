# Changelog

All notable changes to tomo-labs are recorded here.

## v0.1.2

Adds the LiveCodeBench eval tier, captures upstream rate limits so a throttled
run reads as throttled rather than failed, and lets a second harness run beside
the first without colliding.

### Added

- A `livecodebench` eval tier that rebuilds LiveCodeBench, the contamination-free
  competition benchmark, rendering both the stdin and the functional problem
  shapes and grading each with LiveCodeBench's own test runner in a suite-local
  Python venv on the host. `gen --suite livecodebench --difficulty easy|medium|hard`
  pins the tier so a run can showcase or stress a tool.
- Rate-limit capture: when the upstream throttles a run, the result records the
  number of 429s and the longest Retry-After, and the summary marks the run
  rate-limited instead of counting it as a plain failure.
- `lab build --no-cache` forces a fresh install of every tool image, so a daily
  rebuild picks up a tool's newest release even when the layer cache would hide it.
- A daily workflow that bumps every wired tool to its latest version.
- `LAB_NAME_PREFIX` gives a run its own container-name prefix, so a second harness
  can run on the same machine under its own names, network, and proxy port.

### Changed

- Every tool page is now a deep reference: what the tool is, its command surface,
  how the lab drives it, its architecture, and the system prompt it actually sent.
- The docs give evals their own section and order the guides by use case.
- Every wired tool is bumped to its current release.

### Fixed

- `lab clean` now removes every worker slot, not just worker zero. A concurrent
  sweep killed mid-flight left its higher slots behind as orphan containers, each
  pinning a writable layer and a published port, and they piled up run after run
  until the container machine filled its disk.

## v0.1.1

Recovers each tool's real system prompt from its traces, and adds a research
documentation tier that studies every wired agent in depth.

### Added

- `lab prompts <tool> [scenario]` recovers a tool's system prompt from its
  captured traces. It reads the request tap across every run, unions the distinct
  prompts, groups the per-run renderings that differ only in volatile spans like
  the date or a session id, and ranks the agent's working prompt first. `--json`
  emits the structured form and `--brief` keeps the headers without the text.
- A research page per wired agent under the docs, covering what it is, its command
  surface, how the lab drives it, its architecture, the system prompt it actually
  sent, and a `00-hello` run traced end to end.
- A versioned page per tool holding the verbatim system prompt it sent, generated
  from `lab prompts`, so a prompt change between tool versions shows up in a diff.
- An overview page for the whole feature set, an evals guide for the eval tiers,
  and a guide for upgrading the wired tools as they release new versions.

### Changed

- The installation guide now offers the signed release archives alongside the
  from-source build, with checksum and cosign verification.
- The CLI reference documents the `prompts`, `gen`, and `reparse` commands and the
  `--suite` flag.

## v0.1.0

First release. tomo-labs runs coding agents through the same tasks on the same
model and measures what actually happened, graded from the files each agent left
on disk.

### Added

- The `lab` harness: `build` the shared base, proxy, and per-tool images, `run` a
  tool through every scenario, and `report` the captured runs as a comparison
  table. `meta`, `scenarios`, `tools`, `reparse`, and `clean` round out the
  command surface.
- A trace proxy that sits in front of every agent, forces greedy decoding onto
  every completion, records each request and response verbatim, and translates
  whatever wire the tool's SDK speaks into one chat-completions call upstream, so
  the model is held fixed across tools that speak different dialects.
- Fourteen scenarios plus a `00-hello` baseline, each graded by a deterministic
  checker that reads the work left in the container, not the model's own account
  of it.
- Eight wired tools: tomo, codex, opencode, claude-code, openclaw, hermes,
  gemini-cli, and pi. Each is a `Dockerfile` on the shared base and a small
  adapter script; the harness never reads a tool's own code.
- A results snapshot in the docs and README: tomo does all fourteen tasks in 187k
  tokens against 732k for codex and 1.79M for claude-code, thirteen of them on the
  first try.

### Changed

- CI runs build, vet, gofmt, race tests, golangci-lint, govulncheck, and a
  go.mod tidy check on Linux and macOS for every push and pull request.
- A version tag ships a GoReleaser release: the `lab` and `proxy` binaries as
  archives for Linux, macOS, and Windows, a checksum file, a CycloneDX SBOM per
  archive, and a keyless cosign signature over the checksums.
