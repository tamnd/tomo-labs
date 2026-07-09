#!/usr/bin/env bash
# A realistic slice of an HTTP access log in common log format: a mix of 200s,
# redirects, 404s, and a scatter of 500s among other 5xx codes, so counting
# exactly 500 (not "any 5xx") is the actual work.
set -e
W="$1"
cat > "$W/access.log" <<'LOG'
10.0.0.14 - - [09/Jul/2026:10:00:01 +0000] "GET / HTTP/1.1" 200 1043
10.0.0.14 - - [09/Jul/2026:10:00:02 +0000] "GET /assets/app.css HTTP/1.1" 200 8123
203.0.113.9 - - [09/Jul/2026:10:00:03 +0000] "GET /login HTTP/1.1" 200 512
203.0.113.9 - - [09/Jul/2026:10:00:04 +0000] "POST /login HTTP/1.1" 302 0
198.51.100.7 - - [09/Jul/2026:10:00:05 +0000] "GET /dashboard HTTP/1.1" 200 4211
198.51.100.7 - - [09/Jul/2026:10:00:06 +0000] "GET /api/orders HTTP/1.1" 500 91
10.0.0.22 - - [09/Jul/2026:10:00:07 +0000] "GET /favicon.ico HTTP/1.1" 404 209
10.0.0.22 - - [09/Jul/2026:10:00:08 +0000] "GET /api/orders HTTP/1.1" 500 91
172.16.5.3 - - [09/Jul/2026:10:00:09 +0000] "GET /health HTTP/1.1" 200 2
172.16.5.3 - - [09/Jul/2026:10:00:10 +0000] "GET /api/report HTTP/1.1" 503 88
203.0.113.9 - - [09/Jul/2026:10:00:11 +0000] "GET /profile HTTP/1.1" 200 1888
203.0.113.9 - - [09/Jul/2026:10:00:12 +0000] "POST /api/upload HTTP/1.1" 500 74
198.51.100.7 - - [09/Jul/2026:10:00:13 +0000] "GET /old-page HTTP/1.1" 301 0
10.0.0.14 - - [09/Jul/2026:10:00:14 +0000] "GET /search?q=test HTTP/1.1" 200 3312
10.0.0.14 - - [09/Jul/2026:10:00:15 +0000] "GET /api/orders HTTP/1.1" 502 55
172.16.5.3 - - [09/Jul/2026:10:00:16 +0000] "GET /api/orders HTTP/1.1" 500 91
172.16.5.3 - - [09/Jul/2026:10:00:17 +0000] "GET /assets/logo.png HTTP/1.1" 200 15002
203.0.113.9 - - [09/Jul/2026:10:00:18 +0000] "DELETE /api/session HTTP/1.1" 204 0
198.51.100.7 - - [09/Jul/2026:10:00:19 +0000] "GET /api/metrics HTTP/1.1" 500 63
10.0.0.22 - - [09/Jul/2026:10:00:20 +0000] "GET /docs HTTP/1.1" 200 7781
LOG
