# The trace proxy: a single static Go binary on a minimal base. Its only
# non-stdlib dependency is tomo's pkg/wire, which is itself stdlib-only, so the
# build stays trivial and the image small. The build context is the repository
# root (the module), so this copies the module metadata, fetches the pinned
# dependency into the cache, then copies the two packages the binary needs.
FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/proxy ./cmd/proxy
COPY pkg/proxy ./pkg/proxy
RUN CGO_ENABLED=0 go build -trimpath -o /proxy ./cmd/proxy

FROM gcr.io/distroless/static-debian12
COPY --from=build /proxy /proxy
EXPOSE 8080
ENTRYPOINT ["/proxy"]
