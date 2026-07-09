# The tomo tool image: the shared base plus a tomo binary and the adapter that
# drives it. tomo is built from its public module, so this image is independent
# of any tomo checkout on the host.
FROM golang:1.26-bookworm AS build
# Pinned to the oneshot-prompt-mode branch, which carries both the workspace
# rooting (file and shell tools at /work) and `tomo -p`, the one-shot headless
# mode the adapter drives. Revert to a released tag (or latest) once
# github.com/tamnd/tomo lands both in a release.
ARG TOMO_VERSION=oneshot-prompt-mode
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
