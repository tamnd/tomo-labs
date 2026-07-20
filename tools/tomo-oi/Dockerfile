# The tomo-oi tool image: the same tomo binary as the tomo image, driven on its
# oi engine instead of the default loop. oi is the Open Interpreter code-as-action
# loop ported into tomo: the model acts by writing one Markdown code block, the
# engine runs it in the sandbox, feeds the output back, and the turn ends when the
# model's reply carries no code block. It carries no structured tool registry, so
# it removes the function-calling surface that cheap models mis-use. It reuses
# tomo's own provider, gate, and sandbox, and only the system prompt, the action
# surface, and the turn loop differ from the default engine, so this is the same
# tomo measured with a different loop, not a different tool.
FROM golang:1.26-bookworm AS build
# Pinned to the merged commit that adds the oi engine (pkg/engine/oi) and the oi
# value on the --engine flag. The default engine is untouched (it does not modify
# pkg/agent), so the tomo image stays the fair rival baseline and this image only
# adds the oi loop on top. Bump to the tag once oi lands on a release.
ARG TOMO_VERSION=v0.2.5-0.20260720041632-cf1d1af451c0
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
