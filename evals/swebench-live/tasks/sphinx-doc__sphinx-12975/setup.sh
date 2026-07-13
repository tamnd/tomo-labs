#!/usr/bin/env bash
# Check out the repository at the instance's base commit into the work tree, then
# strip every commit, branch, and tag that came after it, so the instance's own
# gold fix is not sitting in the work tree's git history for a tool to find and
# cherry-pick instead of solving the bug.
set -e
W="$1"
D="$(cd "$(dirname "$0")" && pwd)"
SUITE="$(cd "$D/../.." && pwd)"
NAME="$(basename "$D")"
ORACLE="$SUITE/oracle/$NAME"
REPO="$(cat "$ORACLE/repo")"
SHA="$(cat "$ORACLE/base_commit")"
DATA="${LAB_DATA:-$HOME/data/tomo-labs}"
CACHE="$DATA/cache/$(basename "$SUITE")/$(echo "$REPO" | tr '/' '_').git"
if [ ! -d "$CACHE" ]; then
  mkdir -p "$(dirname "$CACHE")"
  git clone --bare --quiet "https://github.com/$REPO.git" "$CACHE"
fi
git clone --quiet --no-hardlinks "$CACHE" "$W"
git -C "$W" checkout --quiet -B __base "$SHA"
git -C "$W" remote remove origin 2>/dev/null || true
for ref in $(git -C "$W" for-each-ref --format='%(refname)' refs/heads); do
  [ "$ref" = "refs/heads/__base" ] || git -C "$W" update-ref -d "$ref"
done
for tag in $(git -C "$W" tag); do
  git -C "$W" merge-base --is-ancestor "refs/tags/$tag" __base 2>/dev/null || git -C "$W" tag -d "$tag" >/dev/null
done
git -C "$W" reflog expire --expire=now --all >/dev/null 2>&1 || true
git -C "$W" gc --prune=now --quiet 2>/dev/null || true
