#!/bin/sh
set -eu
cd /media
if test -s /backup/media.sha256; then sha256sum -c /backup/media.sha256; fi
while read -r key digest size; do
  printf '%s\n' "$key" | grep -Eq '^objects/[a-f0-9-]{36}$'
  test "$(stat -c%s -- "$key")" = "$size"
  printf '%s  %s\n' "$digest" "$key" | sha256sum -c -
done < /backup/reachable.tsv
