#!/usr/bin/env bash
# Pass when the exercise's own test suite is green.
W="$1"
cd "$W" && python3 -m unittest bottle_song_test
