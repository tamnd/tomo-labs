#!/usr/bin/env bash
# The cheapest product on the page is the USB-C Cable at $3.49. It is the only
# product whose name mentions USB, so a case-insensitive match is enough.
W="$1"
[ -f "$W/cheapest.txt" ] || { echo "cheapest.txt missing"; exit 1; }
if grep -qi 'usb' "$W/cheapest.txt"; then echo "cheapest product identified"; exit 0; fi
echo "cheapest.txt did not name the cheapest product:"; cat "$W/cheapest.txt"; exit 1
