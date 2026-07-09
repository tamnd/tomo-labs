# The tomo tool image: the shared base plus a tomo binary and the adapter that
# drives it. tomo is built from its public module, so this image is independent
# of any tomo checkout on the host.
FROM golang:1.26-bookworm AS build
# Pinned to a tomo commit that carries the workspace rooting (file and shell
# tools at /work), `tomo -p` (the one-shot headless mode the adapter drives), and
# the build-info version fallback so `tomo version` reports the real build rather
# than the dev placeholder. Bump to a released tag once github.com/tamnd/tomo
# tags a release that includes all three.
ARG TOMO_VERSION=c1a34b365454e1877e633184ffc47f521d216e0d
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
