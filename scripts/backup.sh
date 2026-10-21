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
running=$(docker compose ps --status running -q wikit)
test -z "$running"
container=$(docker compose ps -aq wikit)
test -n "$container"
test "$(docker inspect -f '{{.State.ExitCode}}' "$container")" = 0
docker compose exec -T postgres pg_dump -U wikit -d wikit -Fc > "$dest/database.dump"
docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U wikit -d wikit -Atc \
  'COPY (SELECT DISTINCT storage_key,digest,size FROM wikit.blobs WHERE id IN (SELECT blob_id FROM wikit.revisions UNION SELECT blob_id FROM wikit.proposal_revisions)) TO STDOUT' > "$dest/reachable.tsv"
docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U wikit -d wikit -Atc \
  'SELECT version,name,checksum FROM public.wikit_schema_migrations ORDER BY version' > "$dest/schema.txt"
docker run --rm --network none --user 0 -v "$volume:/media:ro" -v "$dest:/backup" -v "$PWD/scripts/verify-media.sh:/verify-media.sh:ro" postgres:18.6 \
  sh -ec 'cd /media; find . -type f -exec sha256sum {} \; | sort > /backup/media.sha256; sh /verify-media.sh; tar --numeric-owner -cpf /backup/media.tar .'
docker compose images --format json > "$dest/images.json"
(cd "$dest"; sha256sum database.dump media.tar media.sha256 reachable.tsv schema.txt images.json > SHA256SUMS)
printf 'Backup complete; application remains stopped: %s\n' "$dest"
