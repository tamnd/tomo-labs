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
# 4c13896 adds --yolo, the fully autonomous mode the adapter runs so tomo acts
# unattended like every rival instead of stalling on approval prompts. 8a7383a
# bounds a turn that investigates without ever editing, so a run that mines git
# history or searches without committing to a change is nudged, then stopped,
# instead of burning millions of tokens to the wall. 77fe8ad adds the mirror
# bound, a turn that keeps editing without converging (scratch scripts or the
# same file over and over), and lifts the no-edit bound clear of a deep but
# productive read-before-edit investigation. 92b5e82 slims the live-loop toolset
# to the coding core (drops the empty-index readers, time, and the memory writer,
# and gates the network tool on the sandbox), cutting the per-request tool schema
# from 6,229 to 4,805 bytes on every turn.
ARG TOMO_VERSION=92b5e82e7199c778bb959e67c55e423121120cbe
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
