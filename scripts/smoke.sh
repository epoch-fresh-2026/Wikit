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
evidence=$(realpath -m .local/acceptance)
mkdir -p "$evidence"
exec > >(tee "$evidence/run.log") 2>&1
cleanup() {
  COMPOSE_PROJECT_NAME=$source_project docker compose down -v --remove-orphans
  COMPOSE_PROJECT_NAME=$restore_project docker compose down -v --remove-orphans
  docker volume rm "${source_project}-state" >/dev/null 2>&1 || true
}
trap cleanup EXIT
driver() {
  WIKIT_IMAGE=wikit:acceptance docker compose run --rm -T --no-deps \
    -v "${source_project}-state:/state" \
    -e WIKIT_ACCEPTANCE=isolated -e ACCEPTANCE_PASSWORD=isolated_password_2026 \
    --entrypoint /acceptance wikit "$1"
}
printf 'Commit: %s\n' "$(git rev-parse HEAD)"
uname -a; lscpu; free -h; docker version
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

# First release: build the actual PR #25 backend as the previous version.
baseline=e6f50d3318fe238758cf92c91451cff0acc3283d
mkdir "$evidence/baseline"
git archive "$baseline" | tar -x -C "$evidence/baseline"
cp scripts/Dockerfile.baseline "$evidence/baseline/Dockerfile"
docker build -t wikit:previous "$evidence/baseline"
printf 'Rollback baseline commit=%s\n' "$baseline"
WIKIT_IMAGE=wikit:previous docker compose up -d --wait wikit
driver verify-api
docker compose stop -t 45 wikit
docker compose up -d --wait wikit
driver verify
docker compose stop -t 45 wikit

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
printf 'Container, recovery, same-schema rollback and 100k/100-concurrency acceptance passed.\n'
