#!/usr/bin/env bash
# Check out the repository at the instance's base commit into the work tree.
set -e
W="$1"
D="$(cd "$(dirname "$0")" && pwd)"
SUITE="$(cd "$D/../.." && pwd)"
NAME="$(basename "$D")"
ORACLE="$SUITE/oracle/$NAME"
REPO="$(cat "$ORACLE/repo")"
SHA="$(cat "$ORACLE/base_commit")"
CACHE="$SUITE/.cache/$(echo "$REPO" | tr '/' '_').git"
if [ ! -d "$CACHE" ]; then
  git clone --bare --quiet "https://github.com/$REPO.git" "$CACHE"
fi
git clone --quiet --no-hardlinks "$CACHE" "$W"
git -C "$W" checkout --quiet "$SHA"
