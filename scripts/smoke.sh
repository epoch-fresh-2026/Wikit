#!/usr/bin/env bash
set -euo pipefail
export WIKIT_IMAGE=${WIKIT_IMAGE:-wikit:ci}
export POSTGRES_PASSWORD=isolated_ci_only
export WIKIT_DATABASE_PASSWORD=isolated_app_ci_only
export WIKIT_MEILI_KEY=isolated_ci_search_key_2026
export WIKIT_PUBLIC_URL=http://localhost:8080
export WIKIT_BIND=127.0.0.1 WIKIT_PORT=8080
export COMPOSE_PROJECT_NAME="wikit-ci-${GITHUB_RUN_ID:-$$}-${GITHUB_RUN_ATTEMPT:-1}"
source_project=$COMPOSE_PROJECT_NAME
restore_project="${source_project}-restore"
previous_project="${source_project}-previous"
previous_restore_project="${source_project}-previous-restore"
state_project=$source_project
current_image=$WIKIT_IMAGE
acceptance_image=wikit:acceptance
evidence=$(realpath -m .local/acceptance)
mkdir -p "$evidence"
exec > >(tee "$evidence/run.log") 2>&1
cleanup() {
  COMPOSE_PROJECT_NAME=$source_project docker compose down -v --remove-orphans
  COMPOSE_PROJECT_NAME=$restore_project docker compose down -v --remove-orphans
  COMPOSE_PROJECT_NAME=$previous_project docker compose down -v --remove-orphans
  COMPOSE_PROJECT_NAME=$previous_restore_project docker compose down -v --remove-orphans
  docker volume rm "${previous_project}-state" >/dev/null 2>&1 || true
  docker volume rm "${source_project}-state" >/dev/null 2>&1 || true
}
trap cleanup EXIT
driver() {
  WIKIT_IMAGE=$acceptance_image docker compose run --rm -T --no-deps \
    -v "${state_project}-state:/state" \
    -e WIKIT_ACCEPTANCE=isolated -e ACCEPTANCE_PASSWORD=isolated_password_2026 \
    --entrypoint /acceptance wikit "$1"
}
printf 'Commit: %s\n' "$(git rev-parse HEAD)"
uname -a; lscpu; free -h; docker version; docker compose version
docker image inspect "$WIKIT_IMAGE" --format 'Image={{.Id}} User={{.Config.User}}'
test "$(docker image inspect "$WIKIT_IMAGE" --format '{{.Config.User}}')" = '65532:65532'
docker build --target acceptance -t wikit:acceptance .
docker compose config --quiet
docker compose up -d --wait postgres meilisearch
test "$(docker compose exec -T postgres psql -U wikit -d wikit -Atc "SELECT rolsuper OR rolcreatedb OR rolcreaterole FROM pg_roles WHERE rolname=current_user")" = f
docker compose run --rm -T wikit migrate
driver init
docker compose up -d --wait wikit
driver smoke
docker compose stop -t 45 wikit
driver audit
bash scripts/backup.sh "$evidence/backup"

# The old UUID baseline must use its own image AND backup, never the new schema.
baseline=f3e284fbf845d46903da0f33c29938a57ff2b98f
mkdir "$evidence/baseline"
git archive "$baseline" | tar -x -C "$evidence/baseline"
docker build -t wikit:previous "$evidence/baseline"
docker build --target acceptance -t wikit:previous-acceptance "$evidence/baseline"
printf 'Rollback baseline commit=%s\n' "$baseline"
export COMPOSE_PROJECT_NAME=$previous_project WIKIT_IMAGE=wikit:previous
state_project=$previous_project
acceptance_image=wikit:previous-acceptance
docker compose up -d --wait postgres meilisearch
docker compose run --rm -T wikit migrate
driver init
docker compose up -d --wait wikit
driver smoke
docker compose stop -t 45 wikit
bash scripts/backup.sh "$evidence/previous-backup"
if WIKIT_IMAGE=$current_image docker compose run --rm -T wikit migrate; then
  echo 'New baseline unexpectedly accepted the old UUID database'; exit 1
fi
echo 'Old UUID schema rejected without modifying data'
export COMPOSE_PROJECT_NAME=$previous_restore_project
docker compose up -d --wait postgres meilisearch
docker compose create wikit
bash scripts/restore.sh "$evidence/previous-backup"
driver audit
docker compose run --rm -T wikit search-rebuild
docker compose up -d --wait wikit
driver restored
docker compose stop -t 45 wikit
export WIKIT_IMAGE=$current_image
state_project=$source_project
acceptance_image=wikit:acceptance

# New target has new database, media and search volumes. Source remains stopped.
export COMPOSE_PROJECT_NAME=$restore_project
docker compose up -d --wait postgres meilisearch
docker compose create wikit
# Reject corruption before changing any restored data.
cp -a "$evidence/backup" "$evidence/corrupt-backup"
printf 'corruption' >> "$evidence/corrupt-backup/database.dump"
if bash scripts/restore.sh "$evidence/corrupt-backup"; then
  echo 'Corrupt backup unexpectedly restored'; exit 1
fi
echo 'Corrupt backup rejected as expected'
bash scripts/restore.sh "$evidence/backup"
if bash scripts/restore.sh "$evidence/backup"; then
  echo 'Nonempty restore target unexpectedly accepted'; exit 1
fi
echo 'Nonempty restore target rejected as expected'
driver audit
start=$SECONDS
docker compose run --rm -T wikit search-rebuild
printf 'Restore index rebuild seconds=%s\n' "$((SECONDS-start))"
docker compose up -d --wait wikit
driver restored

# Offline scale fixture, followed by a full rebuild and real HTTP workload.
docker compose stop -t 45 wikit
driver seed
start=$SECONDS
docker compose run --rm -T wikit search-rebuild
printf '100k index rebuild seconds=%s\n' "$((SECONDS-start))"
docker compose run --rm -T wikit search-status
docker compose up -d --wait wikit
driver bench
printf 'Container, recovery, old-image/old-backup rollback and 100k/100-concurrency acceptance passed.\n'
