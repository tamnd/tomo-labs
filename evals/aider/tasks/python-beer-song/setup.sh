#!/usr/bin/env bash
# Lay the exercise stub and its tests into the work tree.
set -e
W="$1"
cp -R "$(dirname "$0")/files/." "$W/"
