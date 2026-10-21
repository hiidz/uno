# Upgrading

Every release upgrades the same way, whether or not it changes the database.

## Docker

In the directory with `compose.yaml`:

```sh
docker compose pull
docker compose up -d
docker compose logs -f uno
```

`up -d` alone keeps running the image already on the host, so `pull` first. `compose.yaml` runs `latest` unless `UNO_TAG` in `.env` pins a version (`1.2.0` for release `v1.2.0`). Pin it before you upgrade, so you know which version to go back to. Upgrading from any earlier release works, however many you skip.

## Manual

Pull the new source, rebuild as in the [README](../README.md#manual), and restart `uno`.

## When the database changes

A release that changes the database schema says so in its notes. There is nothing extra to do: on its first start the new version

1. copies the database to `vault.db.v<old version>.bak` beside it (on the `uno-data` volume, for Docker),
2. upgrades it in one transaction, through every release you skipped,
3. logs `Upgrading the database from schema version X to Y, backup at …`.

If the upgrade fails, the database is left as it was and the server stops with the error. Docker keeps restarting it, and each attempt fails the same way without changing anything. Read the error in `docker compose logs uno`, then set `UNO_TAG` back to the version you were on and run `docker compose up -d`. Each upgrade leaves its own `vault.db.v<N>.bak`, and nothing deletes them: remove old ones yourself.

## Going back

An older version won't start on a database a newer one has upgraded. To go back after a schema change, restore the backup. With Docker, where `12` is the old version from the log:

```sh
docker compose stop uno
docker run --rm --volumes-from "$(docker compose ps -aq uno)" alpine \
  sh -c 'cp /data/vault.db.v12.bak /data/vault.db && rm -f /data/vault.db-wal /data/vault.db-shm'
# set UNO_TAG in .env to the version you were on
docker compose up -d
```

Anything changed since the upgrade is lost. Without a schema change, setting `UNO_TAG` back and running `pull` and `up -d` is enough.

Never run `docker compose down -v`: it deletes the volume and the database with it.
