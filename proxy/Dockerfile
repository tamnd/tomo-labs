# The trace proxy: a single static Go binary on a minimal base. It has no
# dependencies beyond the standard library, so the build stays trivial and the
# image small. The build context is the repository root (the module), so this
# copies just the module metadata and the two packages the binary needs.
FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/proxy ./cmd/proxy
COPY pkg/proxy ./pkg/proxy
RUN CGO_ENABLED=0 go build -trimpath -o /proxy ./cmd/proxy

FROM gcr.io/distroless/static-debian12
COPY --from=build /proxy /proxy
EXPOSE 8080
ENTRYPOINT ["/proxy"]
