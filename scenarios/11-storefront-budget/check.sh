#!/usr/bin/env bash
# Budget is $40.00. Of the ten products on the page, six cost 40 or less:
# USB-C Cable 3.49, Spiral Notebook 6.20, HDMI Adapter 12.75, Desk Lamp 19.95,
# Wireless Mouse 24.99, Laptop Stand 34.50. Cheapest first, that is the order
# below. The four over budget must not appear.
W="$1"
f="$W/affordable.txt"
[ -f "$f" ] || { echo "affordable.txt missing"; exit 1; }

# Non-empty lines, lowercased, so a stray blank line or capitalization does not
# fail an otherwise correct answer.
lines=()
while IFS= read -r line; do
  lines+=("$line")
done < <(tr 'A-Z' 'a-z' < "$f" | sed '/^[[:space:]]*$/d')

want=(usb-c spiral hdmi "desk lamp" "wireless mouse" "laptop stand")
if [ "${#lines[@]}" -ne "${#want[@]}" ]; then
  echo "expected ${#want[@]} products, got ${#lines[@]}:"; cat "$f"; exit 1
fi
for i in "${!want[@]}"; do
  case "${lines[$i]}" in
    *"${want[$i]}"*) ;;
    *) echo "line $((i + 1)) is '${lines[$i]}', expected the product named ${want[$i]}"; exit 1 ;;
  esac
done
for bad in keyboard headphones monitor webcam; do
  if printf '%s\n' "${lines[@]}" | grep -qi "$bad"; then
    echo "over-budget product matching '$bad' should not be listed"; exit 1
  fi
done
echo "affordable list is correct and in ascending price order"; exit 0
