---
title: "Upgrading the tools"
description: "Keep the wired agents current: how each version is pinned, how to pull newer releases, and how to refresh the captured versions, results, and system prompts after a bump."
weight: 35
---

The agents under test ship new versions often, some of them daily.
A comparison is only honest if it says which version of each tool it measured, and stays worth re-reading as those tools move.
This guide covers how the versions are pinned, how to move them, and what to refresh after a bump.

## How a version is pinned

Every tool image takes its version as a Docker build argument, so the pin lives in one line of the tool's `Dockerfile`.

```dockerfile
ARG CODEX_VERSION=latest
RUN npm install -g @openai/codex@${CODEX_VERSION}
```

Most tools install from npm and default the argument to `latest`, so a fresh build pulls the newest published release.
tomo is the exception: it installs from its Go module and pins an exact pseudo-version, because it is the reference point the lab was built around and its behaviour needs to be reproducible run to run.

Because the argument defaults to `latest`, the version a build actually captured is not obvious from the `Dockerfile` alone.
That is what `lab meta` is for.

## Recording what you ran

```bash
go run ./cmd/lab meta
```

`meta` inspects each built tool image and records the version it resolved to and the date that version was published.
Run it after a build so the report and the docs can name the exact versions the sweep measured, rather than a moving `latest`.

## Pulling newer releases

A plain rebuild does not always pull a newer release, because the container runtime caches the install layer.
There are two clean ways to move a pin.

Pull the newest release by rebuilding without the cached layer:

```bash
go run ./cmd/lab build codex   # rebuilds the codex image
```

If the runtime served the install from cache and you want to force the newest release, rebuild the image with the cache disabled for that tool, or bump the version argument to an exact release so the pin is explicit:

```dockerfile
ARG CODEX_VERSION=0.45.0
```

An exact pin is the better choice when you are about to publish numbers, because it makes the sweep reproducible: anyone who rebuilds gets the same version you measured.
Leave `latest` in place for day-to-day exploration, pin an exact version when you record a result.

## Refreshing after a bump

A new version can change the tool's prompt, its tool schema, its token appetite, or its footprint, so a bump is only finished once the captured artifacts catch up.

- Rebuild the tool image and run `lab meta` to record the new version and date.
- Re-run the tool over the suite so its results reflect the new version.

  ```bash
  go run ./cmd/lab run codex
  ```
- Rebuild the report so the comparison table is over the current versions.

  ```bash
  go run ./cmd/lab report
  ```
- Regenerate the tool's system-prompt page so any prompt change shows up in the diff.

  ```bash
  go run ./cmd/lab prompts codex --json > docs/content/prompts/codex.json
  ```

  The [prompts](/prompts/) pages are checked into the repo for exactly this reason: when a tool changes its system prompt between versions, the change lands in a reviewable diff instead of going unnoticed.

## A note on the free tier

Every tool talks to the same upstream model through the proxy, so upgrading a tool never changes the model it runs against.
A version bump moves the agent, its prompt, and its scaffolding, and leaves the one variable the lab holds fixed exactly where it was.
That is the point: a newer agent is measured against the same model as the older one, so the difference you read is the agent.
