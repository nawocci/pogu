#!/usr/bin/env bash
# Bring up pogu via compose (detached) and print the one-time setup token.
#
# `docker compose up -d` intentionally shows no container logs, so on first
# boot the setup token would otherwise require a manual
# `docker logs pogu | grep 'setup token'`. This wrapper starts the stack and
# then tails the logs until the token appears, printing it prominently.
#
# Usage:
#   ./scripts/docker-up.sh [--build] [-- <extra compose up args>]
#   make docker-up
set -euo pipefail

cd "$(dirname "$0")/.."

UP_ARGS=(-d)
if [ "$#" -eq 0 ]; then
  UP_ARGS+=(--build)
else
  UP_ARGS+=("$@")
fi

docker compose up "${UP_ARGS[@]}"

# Wait for the setup token to appear in the container logs (first boot
# only). The server logs both a JSON line
# ({"msg":"setup token","token":"..."}) and a plain
# "Pogu initial setup token: ..." line; accept either.
TIMEOUT_SECS="${POGU_SETUP_TOKEN_TIMEOUT:-60}"
token=""
for ((i = 0; i < TIMEOUT_SECS; i++)); do
  logs="$(docker compose logs --no-color --no-log-prefix pogu 2>/dev/null || docker logs pogu 2>&1 || true)"
  if [ -n "$logs" ]; then
    # Prefer the plain human-readable line, fall back to the JSON field.
    token="$(printf '%s\n' "$logs" | grep -oE '[Ss]etup token: *[^[:space:]]+' | tail -n 1 | awk '{print $NF}' || true)"
    if [ -z "$token" ]; then
      token="$(printf '%s\n' "$logs" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p' | tail -n 1 || true)"
    fi
    if [ -n "$token" ]; then
      break
    fi
    # Server started without setup mode => already initialized.
    if printf '%s\n' "$logs" | grep -q 'server starting'; then
      # Give the setup lines a moment to appear right after startup.
      sleep 2
      logs="$(docker compose logs --no-color --no-log-prefix pogu 2>/dev/null || docker logs pogu 2>&1 || true)"
      token="$(printf '%s\n' "$logs" | grep -oE '[Ss]etup token: *[^[:space:]]+' | tail -n 1 | awk '{print $NF}' || true)"
      if [ -z "$token" ]; then
        token="$(printf '%s\n' "$logs" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p' | tail -n 1 || true)"
      fi
      if [ -z "$token" ]; then
        break
      fi
      break
    fi
  fi
  sleep 1
done

if [ -n "$token" ]; then
  printf '\n'
  printf '==============================================================\n'
  printf ' Pogu initial setup token:\n'
  printf '   %s\n' "$token"
  printf ' Open http://127.0.0.1:8099 and enter the token to set your\n'
  printf ' admin password (whoever completes setup first claims the\n'
  printf ' instance, so do this immediately).\n'
  printf '==============================================================\n'
else
  printf '\nNo setup token found in the container logs.\n'
  printf 'The instance is probably already initialized — open http://127.0.0.1:8099 and log in.\n'
  printf 'If you expected a token, inspect the logs with:\n'
  printf '  docker compose logs pogu | grep -i "setup token"\n'
fi
