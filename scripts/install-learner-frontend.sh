#!/usr/bin/env bash
set -euo pipefail

: "${APP_DIR:?APP_DIR is required}"
: "${RELEASE_ID:?RELEASE_ID is required}"
[[ "$RELEASE_ID" =~ ^[a-f0-9]{40}$ ]] || { echo 'Invalid release ID' >&2; exit 1; }
stage_path=$(cd "${1:?Staged frontend directory is required}" && pwd -P)
release_path=$(cd "$APP_DIR/current" && pwd -P)
dist_path="$release_path/front/dist"
test -s "$stage_path/index.html"
test -d "$stage_path/assets"
test -f "$dist_path/index.html"
test -d "$dist_path/assets"

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$stage_path" && sha256sum -c manifest.sha256)
else
  (cd "$stage_path" && shasum -a 256 -c manifest.sha256)
fi

backup_path="$APP_DIR/frontend-backups/$RELEASE_ID"
mkdir -p "$backup_path"
backup_file=$(mktemp "$backup_path/index.XXXXXX")
cp -p "$dist_path/index.html" "$backup_file"
# Retain prior bundles for open pages, and retain dictionaries and public files.
cp -R "$stage_path/assets/." "$dist_path/assets/"
cp "$stage_path/index.html" "$dist_path/.index.$RELEASE_ID.tmp"
mv -f "$dist_path/.index.$RELEASE_ID.tmp" "$dist_path/index.html"

if ! curl --fail --silent --show-error --max-time 15 \
  "${FRONTEND_CHECK_URL:-http://127.0.0.1:35173/}" -o "$stage_path/served.html" \
  || ! cmp -s "$stage_path/index.html" "$stage_path/served.html"; then
  cp "$backup_file" "$dist_path/.index.$RELEASE_ID.rollback"
  mv -f "$dist_path/.index.$RELEASE_ID.rollback" "$dist_path/index.html"
  echo "Frontend health check failed; restored index from $backup_file" >&2
  exit 1
fi
printf 'Learner frontend updated: %s; index backup: %s\n' "$RELEASE_ID" "$backup_file"
