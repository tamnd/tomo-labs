---
title: "Quick start"
description: "From an empty checkout to a scored comparison table: build the images, run a tool through every scenario, and read the report."
weight: 30
---

This walks the first run end to end.
By the last step you have a scored table comparing however many agents you have built.

## 1. Build the images

```bash
go run ./cmd/lab build
```

This builds the shared base image, the trace proxy, and every wired tool image. It only needs to run again after a `Dockerfile` changes.

## 2. Run a tool through every scenario

```bash
go run ./cmd/lab run tomo
```

Each scenario gets up to `LAB_ATTEMPTS` tries (default 3) and stops at the first pass. Every run writes its full trace under `$HOME/data/tomo/<scenario>/<timestamp>/`, so nothing is summarized away before you can look at it.

Run just one scenario instead of the full sweep:

```bash
go run ./cmd/lab run tomo 03-bugfix-fizzbuzz
```

## 3. Read the report

```bash
go run ./cmd/lab report
```

`report` reads every run ever captured for every tool and prints a comparison table: pass rate, tokens, latency, memory, and install footprint. Add `--json` for the same summary as JSON.

## A few more useful shapes

```bash
go run ./cmd/lab -p "explain this repo in one line"   # one ad-hoc prompt, every tool, in parallel
go run ./cmd/lab meta                                  # capture each tool's version and release date
go run ./cmd/lab tools                                 # list wired tools
go run ./cmd/lab scenarios                              # list scenarios
```

Next: see [results](/guides/results/) for what a full sweep across all eight tools looks like, or [adding a tool](/guides/adding-a-tool/) to wire in another agent.
