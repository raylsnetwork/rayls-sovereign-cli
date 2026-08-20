#!/bin/bash
# Postgres init script run automatically by the postgres:17.3-bookworm image
# on first boot (files under /docker-entrypoint-initdb.d/).
#
# 1. Creates an `admin` superuser for relayer/KOS services.
# 2. Reads DB_NAMES (comma-separated) from the env and creates each database
#    owned by admin if it doesn't already exist.
#
# The governance user/db are already created by POSTGRES_USER/POSTGRES_DB, so
# nothing extra needed for governance-api.

set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-'EOSQL'
    DO $$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'admin') THEN
            CREATE USER admin WITH SUPERUSER LOGIN PASSWORD 'admin';
        END IF;
    END
    $$;
EOSQL

if [ -z "${DB_NAMES:-}" ]; then
    echo "DB_NAMES not set, skipping per-participant database creation"
    exit 0
fi

echo "Creating databases from DB_NAMES: $DB_NAMES"
IFS=',' read -ra DATABASES <<< "$DB_NAMES"
for db in "${DATABASES[@]}"; do
    db=$(echo "$db" | xargs)
    [ -z "$db" ] && continue
    echo "  ensuring database: $db"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<EOSQL
SELECT 'CREATE DATABASE "$db" OWNER admin'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '$db')\gexec
EOSQL
done

echo "Database initialization complete"
