#!/usr/bin/env bash
# Pass when the exercise's own test suite is green.
W="$1"
cd "$W" && go test ./...
