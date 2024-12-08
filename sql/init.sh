#!/usr/bin/env bash

set -eux
cd $(dirname $0)

if [ "${ENV:-}" == "local-dev" ]; then
	exit 0
fi

if test -f /home/isucon/env.sh; then
	. /home/isucon/env.sh
fi

ISUCON_DB_HOST1=${ISUCON_DB_HOST1:-127.0.0.1}
ISUCON_DB_PORT=${ISUCON_DB_PORT:-3306}
ISUCON_DB_USER=${ISUCON_DB_USER:-isucon}
ISUCON_DB_PASSWORD=${ISUCON_DB_PASSWORD:-isucon}
ISUCON_DB_NAME=${ISUCON_DB_NAME:-isuride}

# MySQLを初期化
mysql -u"$ISUCON_DB_USER" \
	-p"$ISUCON_DB_PASSWORD" \
	--host "$ISUCON_DB_HOST1" \
	--port "$ISUCON_DB_PORT" \
	"$ISUCON_DB_NAME" <1-schema.sql

mysql -u"$ISUCON_DB_USER" \
	-p"$ISUCON_DB_PASSWORD" \
	--host "$ISUCON_DB_HOST1" \
	--port "$ISUCON_DB_PORT" \
	"$ISUCON_DB_NAME" <2-master-data.sql

gzip -dkc 3-initial-data.sql.gz | mysql -u"$ISUCON_DB_USER" \
	-p"$ISUCON_DB_PASSWORD" \
	--host "$ISUCON_DB_HOST1" \
	--port "$ISUCON_DB_PORT" \
	"$ISUCON_DB_NAME"

mysql -u"$ISUCON_DB_USER" \
	-p"$ISUCON_DB_PASSWORD" \
	--host "$ISUCON_DB_HOST1" \
	--port "$ISUCON_DB_PORT" \
	"$ISUCON_DB_NAME" <4-adddistance.sql

# HOST2を初期化
ISUCON_DB_HOST2=${ISUCON_DB_HOST2:-127.0.0.1}

# MySQLを初期化
mysql -u"$ISUCON_DB_USER" \
	-p"$ISUCON_DB_PASSWORD" \
	--host "$ISUCON_DB_HOST2" \
	--port "$ISUCON_DB_PORT" \
	"$ISUCON_DB_NAME" <1-schema.sql

mysql -u"$ISUCON_DB_USER" \
	-p"$ISUCON_DB_PASSWORD" \
	--host "$ISUCON_DB_HOST2" \
	--port "$ISUCON_DB_PORT" \
	"$ISUCON_DB_NAME" <2-master-data.sql

gzip -dkc 3-initial-data.sql.gz | mysql -u"$ISUCON_DB_USER" \
	-p"$ISUCON_DB_PASSWORD" \
	--host "$ISUCON_DB_HOST2" \
	--port "$ISUCON_DB_PORT" \
	"$ISUCON_DB_NAME"

mysql -u"$ISUCON_DB_USER" \
	-p"$ISUCON_DB_PASSWORD" \
	--host "$ISUCON_DB_HOST2" \
	--port "$ISUCON_DB_PORT" \
	"$ISUCON_DB_NAME" <4-adddistance.sql
