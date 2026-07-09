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

# A predictable, writable home for whatever the agent scatters around.
ENV HOME=/root
WORKDIR /work
