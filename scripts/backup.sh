#!/usr/bin/env bash
# Maintenance-window backup. Invoke from the repository with the deployment .env.
set -euo pipefail
umask 077
: "${COMPOSE_PROJECT_NAME:?Set the exact Compose project name}"
dest=$(realpath -m "${1:?Usage: backup.sh NEW_BACKUP_DIRECTORY}")
test ! -e "$dest"
mkdir -p "$dest"
volume="${COMPOSE_PROJECT_NAME}_media"
test "$(docker volume inspect -f '{{index .Labels "com.docker.compose.project"}}' "$volume")" = "$COMPOSE_PROJECT_NAME"
# Stop includes the in-process worker. No other writers/GC/admin commands may run.
docker compose stop -t 45 wikit
test -z "$(docker compose ps --status running -q wikit)"
docker compose exec -T postgres pg_dump -U wikit -d wikit -Fc > "$dest/database.dump"
docker run --rm --network none --user 0 -v "$volume:/media:ro" -v "$dest:/backup" postgres:18.6 \
  sh -ec 'cd /media; find . -type f -exec sha256sum {} \; | sort > /backup/media.sha256; tar --numeric-owner -cpf /backup/media.tar .'
docker compose images --format json > "$dest/images.json"
(cd "$dest"; sha256sum database.dump media.tar media.sha256 images.json > SHA256SUMS)
printf 'Backup complete; application remains stopped: %s\n' "$dest"
