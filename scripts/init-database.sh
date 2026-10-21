#!/usr/bin/env bash
(
set -euo pipefail
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  --set=app_password="$WIKIT_DATABASE_PASSWORD" <<'SQL'
CREATE ROLE wikit LOGIN PASSWORD :'app_password' NOSUPERUSER NOCREATEDB NOCREATEROLE;
ALTER DATABASE wikit OWNER TO wikit;
SQL
)
