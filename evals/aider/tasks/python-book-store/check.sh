#!/usr/bin/env bash
# Pass when the exercise's own test suite is green.
W="$1"
cd "$W" && python3 -m unittest book_store_test
