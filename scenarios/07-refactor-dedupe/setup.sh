#!/usr/bin/env bash
# A small utility module where a bad merge left clamp() defined twice. The tests
# exercise every function, so the fix has to keep behavior identical.
set -e
W="$1"
cat > "$W/util.js" <<'JS'
function add(a, b) {
  return a + b;
}

function sub(a, b) {
  return a - b;
}

function clamp(x, lo, hi) {
  return Math.max(lo, Math.min(hi, x));
}

function mul(a, b) {
  return a * b;
}

// Duplicate introduced by a bad merge.
function clamp(x, lo, hi) {
  return Math.max(lo, Math.min(hi, x));
}

module.exports = { add, sub, mul, clamp };
JS
cat > "$W/test.js" <<'JS'
const { add, sub, mul, clamp } = require('./util');
const ok =
  add(2, 3) === 5 &&
  sub(9, 4) === 5 &&
  mul(3, 4) === 12 &&
  clamp(10, 0, 5) === 5 &&
  clamp(-2, 0, 5) === 0;
console.log(ok ? 'PASS' : 'FAIL');
JS
