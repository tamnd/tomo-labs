# The trace proxy: a single static Go binary on a minimal base. It has no
# dependencies beyond the standard library, so the build stays trivial and the
# image small.
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -trimpath -o /proxy .

FROM gcr.io/distroless/static-debian12
COPY --from=build /proxy /proxy
EXPOSE 8080
ENTRYPOINT ["/proxy"]
