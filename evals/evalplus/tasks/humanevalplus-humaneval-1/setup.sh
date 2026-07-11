#!/usr/bin/env bash
# Lay the function stub into the work tree.
set -e
W="$1"
cp -R "$(dirname "$0")/files/." "$W/"
