# The tomo tool image: the shared base plus a tomo binary and the adapter that
# drives it. tomo is built from its public module, so this image is independent
# of any tomo checkout on the host.
FROM golang:1.26-bookworm AS build
# v0.2.3 plans a multi-step job in one turn with the plan tool rather than a
# fresh context per step, on top of the workspace rooting (file and shell tools
# at /work) and `tomo -p` (the one-shot headless mode the adapter drives) that
# landed in v0.2.2.
ARG TOMO_VERSION=v0.2.3
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
