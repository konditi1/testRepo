#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

# Configuration
SQLITE_DB="eval.db"
SQLITE_DUMP="eval_backup.sql"
PG_DUMP="eval_pg.sql"
PG_DB="evalhub"
PG_USER="evalhub"

# Step 1: Dump SQLite database
echo "📦 Dumping SQLite database..."
sqlite3 "$SQLITE_DB" .dump > "$SQLITE_DUMP"
echo "✅ SQLite dump saved to $SQLITE_DUMP"

# Optional: View the dump
# less "$SQLITE_DUMP"

# Step 2: Convert SQLite dump to PostgreSQL-compatible SQL
echo "🔄 Converting to PostgreSQL-compatible SQL..."
if ! command -v sqlite3-to-postgres &> /dev/null; then
    echo "📦 'sqlite3-to-postgres' not found. Installing it..."
    pip install --user sqlite3-to-postgres
    export PATH="$HOME/.local/bin:$PATH"
fi
sqlite3-to-postgres "$SQLITE_DB" > "$PG_DUMP"
echo "✅ Converted dump saved to $PG_DUMP"

# Step 3: Import into PostgreSQL
echo "📥 Importing into PostgreSQL..."
psql -U "$PG_USER" -d "$PG_DB" -f "$PG_DUMP"
echo "✅ Import complete."

# Done
echo "🎉 Migration completed successfully."
