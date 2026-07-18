# The tomo-kata tool image: the same tomo binary as the tomo image, driven on
# its kata engine. kata is a code-as-action loop built on the lessons the oi
# campaign proved: it shares oi's action surface and fence parsing (pkg/fence),
# and differs only in loop policy, adding a reproduce-first finish guard (a
# coding turn on a bug-report task that never watched anything fail is nudged
# once to run the reported case) and a whole-turn round budget (nudge at 24
# model calls, hard stop at 48). A tomo-kata result therefore compares to
# tomo-oi like for like: same binary, same tools, same model, only the loop
# policy and its prompt differ.
FROM golang:1.26-bookworm AS build
# Pinned to the merged commit that adds the kata engine (pkg/engine/kata) and
# the kata value on the --engine flag. Bump to the tag once kata lands on a
# release.
ARG TOMO_VERSION=v0.2.5-0.20260717122502-73d08e93b4ae
RUN CGO_ENABLED=0 go install github.com/tamnd/tomo/cmd/tomo@${TOMO_VERSION}

FROM tomolab-base
COPY --from=build /go/bin/tomo /usr/local/bin/tomo
COPY adapter.sh /usr/local/bin/adapter
RUN chmod +x /usr/local/bin/adapter
ENTRYPOINT ["/usr/local/bin/adapter"]
