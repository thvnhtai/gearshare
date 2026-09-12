#!/usr/bin/env bash
# Bootstraps real MySQL primary/replica binlog replication between
# docker-compose.yml's mysql-primary and mysql-replica containers — the
# "database replication" scaling-pattern requirement, demonstrated with an
# actual replicating second instance rather than just a doc claiming one
# exists. Run once after `docker compose up -d mysql-primary mysql-replica`
# and after migrations have been applied to the primary.
set -euo pipefail

ROOT_PASSWORD="root_password"
PRIMARY_CONTAINER="gearshare-mysql-primary"
REPLICA_CONTAINER="gearshare-mysql-replica"

echo "Creating replication user on primary..."
docker exec "$PRIMARY_CONTAINER" mysql -uroot -p"$ROOT_PASSWORD" -e "
  CREATE USER IF NOT EXISTS 'repl'@'%' IDENTIFIED BY 'repl_password';
  GRANT REPLICATION SLAVE ON *.* TO 'repl'@'%';
  FLUSH PRIVILEGES;
"

echo "Snapshotting primary and reading its binlog position..."
docker exec "$PRIMARY_CONTAINER" mysqldump -uroot -p"$ROOT_PASSWORD" \
  --all-databases --source-data=2 --single-transaction --set-gtid-purged=ON \
  > /tmp/gearshare-primary-snapshot.sql

echo "Loading snapshot into replica..."
docker exec -i "$REPLICA_CONTAINER" mysql -uroot -p"$ROOT_PASSWORD" < /tmp/gearshare-primary-snapshot.sql

echo "Pointing replica at primary (GTID-based, so no manual log-position bookkeeping)..."
docker exec "$REPLICA_CONTAINER" mysql -uroot -p"$ROOT_PASSWORD" -e "
  CHANGE REPLICATION SOURCE TO
    SOURCE_HOST='mysql-primary',
    SOURCE_USER='repl',
    SOURCE_PASSWORD='repl_password',
    SOURCE_AUTO_POSITION=1,
    GET_SOURCE_PUBLIC_KEY=1;
  START REPLICA;
"

sleep 2
echo "Replica status:"
docker exec "$REPLICA_CONTAINER" mysql -uroot -p"$ROOT_PASSWORD" -e "SHOW REPLICA STATUS\G" | grep -E "Replica_IO_Running|Replica_SQL_Running|Seconds_Behind_Source"

rm -f /tmp/gearshare-primary-snapshot.sql
echo "Done. Point MYSQL_REPLICA_DSN at mysql-replica:3306 to route reads there (internal/db/mysql.go)."
