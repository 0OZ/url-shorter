#!/bin/bash

set -e

host="$DB_HOST"
port="$DB_PORT"
user="$DB_USER"
password="$DB_PASSWORD"
dbname="$DB_NAME"

# Set defaults if environment variables are not set
host=${host:-postgres}
port=${port:-5432}
user=${user:-postgres}
password=${password:-postgres}
dbname=${dbname:-urlshortener}

echo "Waiting for PostgreSQL to become available..."
echo "Host: $host, Port: $port, Database: $dbname"

# Wait for PostgreSQL to become available
until PGPASSWORD=$password psql -h "$host" -p "$port" -U "$user" -d "$dbname" -c '\q'; do
  echo "PostgreSQL is unavailable - sleeping"
  sleep 1
done

echo "PostgreSQL is up - executing command"

# Execute the main command passed to this script
exec "$@" 