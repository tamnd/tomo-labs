#!/usr/bin/env bash
# runtime.sh picks the container command once, so the rest of the lab never
# cares which one is installed. docker wins when its daemon is reachable,
# otherwise podman, and LAB_RUNTIME overrides both. Everything the lab does
# (build, run, network, volumes) is in the shared docker/podman CLI surface, so
# a run is identical whichever backs it.
detect_runtime() {
  if [ -n "${LAB_RUNTIME:-}" ]; then
    echo "$LAB_RUNTIME"; return 0
  fi
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    echo docker; return 0
  fi
  if command -v podman >/dev/null 2>&1; then
    echo podman; return 0
  fi
  echo "no container runtime: install docker or podman, or set LAB_RUNTIME" >&2
  return 1
}

# ready_check curls a published proxy port until it answers with any HTTP
# status, which means the listener is up even if the upstream 404s a bare GET.
ready_check() {
  local url="$1" tries="${2:-40}" i
  for ((i = 0; i < tries; i++)); do
    if curl -s -o /dev/null -m 2 "$url"; then return 0; fi
    sleep 0.5
  done
  return 1
}
