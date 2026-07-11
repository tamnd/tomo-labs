---
title: "Installation"
description: "Get the lab and proxy binaries, from a signed release or from source, plus what the harness needs to run: a repo checkout, a container runtime, and a key for an OpenAI-compatible endpoint."
weight: 20
---

tomo-labs ships two binaries, `lab` and `proxy`, in every release archive.
You can download those or build from source.
Either way you also need a checkout of the repo, because the harness reads its scenarios, tool Dockerfiles, and eval tiers from the working tree.

## From a release

Each tagged release publishes a signed archive per platform on the [releases page](https://github.com/tamnd/tomo-labs/releases).
Every archive carries both binaries plus the README and LICENSE.

```bash
# pick the archive for your platform, for example macOS on Apple silicon
curl -LO https://github.com/tamnd/tomo-labs/releases/download/v0.1.0/tomo-labs_0.1.0_darwin_arm64.tar.gz
tar xzf tomo-labs_0.1.0_darwin_arm64.tar.gz
```

The archive names follow `tomo-labs_<version>_<os>_<arch>`, a `.tar.gz` for darwin and linux and a `.zip` for windows, across `amd64` and `arm64`.

Verify the download before you trust it.
Every release ships a `checksums.txt`, and that file is signed with cosign so you can confirm it came from the release pipeline.

```bash
# checksum
curl -LO https://github.com/tamnd/tomo-labs/releases/download/v0.1.0/checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing

# cosign signature over the checksum file
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp 'https://github.com/tamnd/tomo-labs' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

Each archive also carries a `.sbom.json` next to it, so you can see exactly what went into the build.

Put `lab` on your `PATH` and the docs' `go run ./cmd/lab ...` examples become plain `lab ...`.

## From source

If you have a Go toolchain, building is one command and gives you the same binaries.

```bash
git clone https://github.com/tamnd/tomo-labs
cd tomo-labs
go build -o bin/lab ./cmd/lab
```

Or skip the build and run it directly, which every example in these docs uses:

```bash
go run ./cmd/lab ...
```

## You still need the repo tree

The binaries are only the driver.
`lab` finds the harness by walking up from the working directory until it sees a `scenarios/` dir, so it expects to run from inside a checkout, where it can read the scenarios, the tool Dockerfiles under `tools/`, and the eval tiers under `evals/`.

```bash
git clone https://github.com/tamnd/tomo-labs
cd tomo-labs
lab tools        # a release binary on PATH, run from the checkout
```

Run `lab` from somewhere else and point it at a checkout with `LAB_ROOT`:

```bash
LAB_ROOT=/path/to/tomo-labs lab tools
```

## What it needs to run

- **A repo checkout.** The scenarios, tool Dockerfiles, and eval tiers live in the tree, as above.
- **podman or docker.** The harness detects whichever is present; set `LAB_RUNTIME` to force one.
- **Go 1.26.5 or newer**, only if you build from source or use the `go run` form.
- **A key for an OpenAI-compatible endpoint.** The default targets the OpenCode Zen free tier, whose deepseek model does tool calling:

  ```bash
  export OPENCODE_API_KEY=...
  ```

Nothing else is required. tomo-labs never talks to the real model directly; every agent under test points at the trace proxy, and the proxy is the only thing that reaches the upstream API.

Next: [the quick start](/getting-started/quick-start/).
