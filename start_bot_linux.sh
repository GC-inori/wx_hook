#!/usr/bin/env sh
# Compatibility entry only. All startup logic and settings live in wxbot/config.yaml.
ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
exec "$ROOT_DIR/wxbot" -root "$ROOT_DIR" "$@"
