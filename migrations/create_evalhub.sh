#!/bin/bash

# Must be run with sudo
if [ "$EUID" -ne 0 ]; then
  echo "❌ Please run as root or use sudo."
  exit 1
fi

# Configuration
SQLITE_DB="eval.db"
PG_DB="evalhub"
PG_USER="evalhub"
PG_PASSWORD="2222"
PG_HOST="localhost"
PG_PORT="5432"
SQLITE_BACKUP="eval_backup.sql"

# Step 1: Create PostgreSQL user and database
echo "🚀 Switching to postgres user and creating database..."
sudo -i -u postgres psql <<EOF
-- Create user
DO \$\$
BEGIN
   IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '$PG_USER') THEN
      CREATE USER $PG_USER WITH PASSWORD '$PG_PASSWORD';
   END IF;
END
\$\$;

-- Create database if not exists
DO \$\$
BEGIN
   IF NOT EXISTS (SELECT FROM pg_database WHERE datname = '$PG_DB') THEN
      CREATE DATABASE $PG_DB OWNER $PG_USER;
   END IF;
END
\$\$;

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE $PG_DB TO $PG_USER;
EOF
echo "✅ Database '$PG_DB' and user '$PG_USER' ready."

# Step 2: Install pgloader if not installed
if ! command -v pgloader &> /dev/null; then
  echo "📦 Installing pgloader..."
  apt update && apt install -y pgloader
fi

# Step 3: Backup SQLite database
echo "📦 Backing up SQLite database to $SQLITE_BACKUP..."
sqlite3 "$SQLITE_DB" .dump > "$SQLITE_BACKUP"
echo "✅ Backup complete."

# Step 4: Run pgloader migration
echo "🔄 Migrating data from SQLite to PostgreSQL..."
export PGPASSWORD="$PG_PASSWORD"
pgloader "$SQLITE_DB" "postgresql://$PG_USER:$PG_PASSWORD@$PG_HOST:$PG_PORT/$PG_DB"
echo "✅ Migration completed successfully."

# Done
echo "🎉 All done! Your SQLite data is now in PostgreSQL."
