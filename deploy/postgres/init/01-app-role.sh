#!/bin/sh
# Creates the least-privilege role used by the API. The owner role (POSTGRES_USER)
# runs migrations; lamma_app can only read/write rows, never alter schema.
set -eu
psql -v ON_ERROR_STOP=1 -v app_pw="$POSTGRES_APP_PASSWORD" \
  --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<'SQL'
CREATE ROLE lamma_app LOGIN PASSWORD :'app_pw';
GRANT CONNECT ON DATABASE lamma TO lamma_app;
GRANT USAGE ON SCHEMA public TO lamma_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO lamma_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO lamma_app;
SQL
