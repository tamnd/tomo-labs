#!/usr/bin/env bash
set -e
W="$1"
cat > "$W/util.js" <<'JS'
function add(a, b) {
  return a + b;
}

function add(a, b) {
  return a + b;
}

module.exports = { add };
JS
cat > "$W/test.js" <<'JS'
const { add } = require('./util');
console.log(add(2, 3) === 5 ? 'PASS' : 'FAIL');
JS
