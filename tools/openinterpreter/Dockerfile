# The openinterpreter tool image: the shared base plus the Open Interpreter CLI
# and a headless adapter. Open Interpreter was rewritten: the old Python
# code-as-action loop (package open-interpreter, pinned 0.4.2) ended at v0.4.2,
# and main is now a Rust program, a fork of OpenAI Codex "optimized for low-cost
# models" that emulates the agent harness each model was post-trained on. This
# image tracks that rewrite, pinned to the release tag below, so the
# openinterpreter column measures current OI. It is a self-contained release
# bundle downloaded from GitHub, independent of any checkout on the host.
#
# Note: current OI is a structured-tools Codex fork, not the code-as-action loop
# tomo's oi engine was ported from, so this column no longer measures the same
# loop as tomo-oi. That is intentional: it measures OI as it ships today.
FROM tomolab-base

# The release tag to install. Bump this to move the column to a newer OI.
ARG OI_VERSION=rust-v0.0.25

# Download the platform release bundle and stage it the way OI's own installer
# does: a self-contained directory whose bin/interpreter resolves its sibling
# resources (codex-path/rg, codex-resources/bwrap) by its real path. We extract
# to a fixed system location and symlink the binary onto PATH rather than run the
# openinterpreter.com installer, so the version is pinned and the layout does not
# depend on $HOME (the adapter repoints HOME at the throwaway tree at run time).
# We take the .tar.gz asset, so no xz/zstd is needed.
RUN set -eux; \
    arch="$(dpkg --print-architecture)"; \
    case "$arch" in \
      arm64) target="aarch64-unknown-linux-musl" ;; \
      amd64) target="x86_64-unknown-linux-musl" ;; \
      *) echo "unsupported arch: $arch" >&2; exit 1 ;; \
    esac; \
    url="https://github.com/openinterpreter/openinterpreter/releases/download/${OI_VERSION}/open-interpreter-package-${target}.tar.gz"; \
    mkdir -p /opt/openinterpreter; \
    curl -fsSL "$url" -o /tmp/oi.tar.gz; \
    tar -xzf /tmp/oi.tar.gz -C /opt/openinterpreter; \
    rm -f /tmp/oi.tar.gz; \
    chmod 0755 /opt/openinterpreter/bin/interpreter /opt/openinterpreter/codex-path/rg || true; \
    if [ -f /opt/openinterpreter/codex-resources/bwrap ]; then chmod 0755 /opt/openinterpreter/codex-resources/bwrap; fi; \
    ln -sf /opt/openinterpreter/bin/interpreter /usr/local/bin/interpreter; \
    interpreter --version

COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
