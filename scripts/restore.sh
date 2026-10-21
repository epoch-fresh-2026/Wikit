#!/usr/bin/env bash
# Restore into an initialized but empty target project, leaving its app stopped.
set -euo pipefail
: "${COMPOSE_PROJECT_NAME:?Set the empty target Compose project name}"
src=$(realpath "${1:?Usage: restore.sh BACKUP_DIRECTORY}")
(cd "$src"; sha256sum -c SHA256SUMS)
test -z "$(docker compose ps --status running -q wikit)"
test "$(docker compose exec -T postgres psql -U wikit -d wikit -Atc "SELECT count(*) FROM pg_namespace WHERE nspname='wikit'")" = 0
volume="${COMPOSE_PROJECT_NAME}_media"
test "$(docker volume inspect -f '{{index .Labels "com.docker.compose.project"}}' "$volume")" = "$COMPOSE_PROJECT_NAME"
docker run --rm --network none --user 0 -v "$volume:/media" -v "$src:/backup:ro" postgres:18.6 \
  sh -ec 'test -z "$(ls -A /media)"; cd /media; tar --numeric-owner -xpf /backup/media.tar; sha256sum -c /backup/media.sha256'
docker compose exec -T postgres pg_restore -U wikit -d wikit --single-transaction --no-owner --no-privileges < "$src/database.dump"
# Restoring a dump can resurrect revoked secrets and old idempotent responses.
docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U wikit -d wikit <<'SQL'
BEGIN;
DELETE FROM wikit.sessions;
UPDATE wikit.user_tokens SET digest=NULL,generation=generation+1;
UPDATE wikit.users SET auth_version=auth_version+1;
DELETE FROM wikit.idempotency_records;
DELETE FROM wikit.list_cursors;
INSERT INTO wikit.audit_events(method,credential_generation,action,request_id)
VALUES('local',0,'system.restore.credentials_revoked',gen_random_uuid()::text);
COMMIT;
SQL
printf 'Restored; verify reachable originals and rebuild search before starting the application.\n'
