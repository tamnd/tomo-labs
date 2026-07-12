# The tomo tool image: the shared base plus a tomo binary and the adapter that
# drives it. tomo is built from its public module, so this image is independent
# of any tomo checkout on the host.
FROM golang:1.26-bookworm AS build
# This build teaches the agent to run the project's tests or build and iterate
# until they pass before ending the turn, so a wrong first draft gets caught
# rather than declared done, and it bounds the agent loop so a run that stops
# making progress ends instead of spinning. It sits on top of the plan tool
# (plan a multi-step job in one turn rather than a fresh context per step), the
# workspace rooting (file and shell tools at /work), and `tomo -p` (the one-shot
# headless mode the adapter drives).
#
# tomo has no tag past v0.2.4 yet, so the image pins the exact main commit under
# test rather than the stale tag. This is a pseudo-version build: `tomo version`
# reports v0.2.5-0.<date>-<commit>. Bump this to the tag once v0.2.5 ships.
ARG TOMO_VERSION=60584e7ffb4d8e543a47a97ce894d19d85b0eff8
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
