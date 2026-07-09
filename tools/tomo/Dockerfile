# The tomo tool image: the shared base plus a tomo binary and the adapter that
# drives it. tomo is built from its public module, so this image is independent
# of any tomo checkout on the host.
FROM golang:1.26-bookworm AS build
ARG TOMO_VERSION=latest
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
