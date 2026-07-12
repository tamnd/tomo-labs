# tomolab-base is the shared runtime every tool image builds on, so each agent
# runs against the same toolchain and the comparison is about the agent, not
# about what happened to be installed. A scenario may ask the agent to build a
# Go program, run a Node test, or drive make, so those live here. GNU time is
# here too: the harness wraps the agent with it to read peak memory back.
FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      bash coreutils findutils grep sed gawk \
      ca-certificates curl gnupg jq git make time \
      python3 python3-pip \
      golang-go \
    && rm -rf /var/lib/apt/lists/*

# Node 22 from NodeSource. Debian's Node 18 is too old for some tools (its
# postinstall uses Array.prototype.toSorted, which needs Node 20+), so every
# image gets the same modern Node rather than a per-tool patch.
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

# uv builds the per-task Python environment the harness prepares before an agent
# runs, the way canonical SWE-bench ships a task with its env already installed.
# It comes from the published uv image rather than a curl-to-shell, so the binary
# is pinned and verifiable.
COPY --from=ghcr.io/astral-sh/uv:0.11 /uv /uvx /usr/local/bin/

# Where the prepared environment lives, split so a venv's interpreter stays valid
# once the agent container mounts the same paths the prep container wrote:
#   /opt/uv    shared across tasks: uv's managed Pythons and wheel cache
#   /opt/venv  the per-task virtualenv, first on PATH so `python` and `pytest`
#              resolve into it without any tool knowing the prep happened
# With no prepared env mounted (every non-SWE-bench scenario) these paths are
# simply absent and the system python3 above serves as before.
ENV UV_PYTHON_INSTALL_DIR=/opt/uv/py \
    UV_CACHE_DIR=/opt/uv/cache \
    VIRTUAL_ENV=/opt/venv \
    PATH=/opt/venv/bin:$PATH

# A predictable, writable home for whatever the agent scatters around.
ENV HOME=/root
WORKDIR /work
