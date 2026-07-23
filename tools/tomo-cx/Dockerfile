# The tomo-cx tool image: the same tomo binary as the tomo image, driven on its
# cx engine instead of the default loop. cx is a second, independent engine
# inside tomo, shaped after the way codex works a bug: one wide grounding search
# over every symbol the task names, a diagnosis from the real failure, a single
# root-cause fix, then a verify run before finishing. It reuses tomo's own tools
# and provider, and only the system prompt, tool descriptions, and turn loop
# differ from the default engine, so this is the same tomo measured with a
# different loop, not a different tool.
FROM golang:1.26-bookworm AS build
# Pinned to the engine-cx branch commit that adds the cx engine (pkg/engine/cx)
# and the --engine flag. The default engine is byte-identical to the tomo image's
# baseline (this commit does not touch pkg/agent), so the tomo image stays the
# fair rival baseline and this image only adds the cx loop on top. Bump to the
# tag once cx lands on a release.
ARG TOMO_VERSION=v0.2.5-0.20260723031314-2c575dd811aa
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
