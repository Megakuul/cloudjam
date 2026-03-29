#!/bin/sh

if [[ "$1" ]]; then
  DATABASE_PATH="$(realpath "$1")"
else
  DATABASE_PATH="$(dirname $(realpath $0))/.database"
  SOCKETS_PATH="$(dirname $(realpath $0))/.sockets"
  mkdir -p "$DATABASE_PATH"
  mkdir -p "$SOCKETS_PATH"
fi

podman run -it --privileged --rm -v "$DATABASE_PATH:/var/lib/postgresql/data:U,Z" -v "$SOCKETS_PATH:/var/run/postgresql/" \
  -e POSTGRES_INITDB_ARGS="--auth-local=trust --auth-host=reject" ghcr.io/ferretdb/postgres-documentdb:17-0.107.0-ferretdb-2.7.0

exit 0
