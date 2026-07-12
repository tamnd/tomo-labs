# The tomo tool image: the shared base plus a tomo binary and the adapter that
# drives it. tomo is built from its public module, so this image is independent
# of any tomo checkout on the host.
FROM golang:1.26-bookworm AS build
# This build teaches the agent to run the project's tests or build and iterate
# until they pass before ending the turn, so a wrong first draft gets caught
# rather than declared done. It sits on top of the plan tool (plan a multi-step
# job in one turn rather than a fresh context per step), the workspace rooting
# (file and shell tools at /work), and `tomo -p` (the one-shot headless mode the
# adapter drives).
ARG TOMO_VERSION=v0.2.5-0.20260711144441-0442c7ea8a03
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
