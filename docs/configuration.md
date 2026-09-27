# Configuration and deployment

## Environment variables

Loaded by `internal/config`, which reads `.env` if present (via `godotenv`) and then the process
environment. See `.env.example`.

| Var | Required | Default | Purpose |
| --- | --- | --- | --- |
| `TMDB_API_KEY` | **yes** — startup fails if empty | none | Provider's upstream key. Not public — every TMDB call is server-side for this reason |
| `NUVIO_PUBLISHABLE_KEY` | **yes** — startup fails if empty | none | `apikey` header on Nuvio REST/RPC calls. Public by design; it is printed in Nuvio's own public docs and intended for embedding in client apps. Not a service credential |
| `SITE_BASE_URL` | **yes** — startup fails if empty | none | Base for the absolute manifest URL handed to clients and pushed into Nuvio — see below |
| `VAULT_DB` | no | `vault.db` | Path to the SQLite file |
| `PORT` | no | `8123` | Listen port (plain HTTP, no TLS) |
| `NUVIO_BASE_URL` | no | `https://api.nuvio.tv` | Base for JWKS discovery and all REST/RPC calls. Its origin is also the only cross-origin `connect-src` in the SPA's Content-Security-Policy, so it must match the `VITE_NUVIO_BASE_URL` the frontend was built with (same default). If they differ, the browser blocks login |
| `DEV_AUTH_BYPASS_TOKEN` | no | empty (bypass off) | **Local development only** — see below |

`config.Load` collects *all* missing required vars before failing, so a fresh setup gets one
error naming both rather than two runs.

### The `SITE_BASE_URL` hazard

`selectProfile` and `pushAddons` both build the absolute manifest URL pushed into Nuvio from
`SiteBaseURL`. `config.Load` (`internal/config/config.go`) has no default for `SITE_BASE_URL`
and includes it in the required-var check alongside `TMDB_API_KEY` and `NUVIO_PUBLISHABLE_KEY`,
so a deploy that leaves it unset fails startup immediately rather than pushing an unreachable
URL into the user's real Nuvio profile with no warning. `.env.example` ships
`SITE_BASE_URL=http://localhost:8123` as the correct local dev value — that's fine for local dev,
since the value is explicit there, not defaulted.

**This does not catch a value that's wrong but non-empty** — e.g. `.env.example`'s
`http://localhost:8123` placeholder copied verbatim into a production `.env`. Startup still
succeeds, and the wrong URL still gets pushed to Nuvio with no warning. `SITE_BASE_URL` must be
set correctly, not merely set, at deploy time.

**Correcting a profile already pushed with the wrong URL takes more than re-pushing.**
`pushAddons` (`internal/api/push.go`) reads the profile's current addon list, upserts Uno's entry
**by URL match**, and pushes the complete merged list back — every entry that doesn't match is
carried forward verbatim, because `sync_push_addons` is full-replace and omitting an addon would
delete someone else's. So after fixing the variable, the next push finds no entry matching the
*new* URL, appends it, and re-pushes a list that still contains the stale localhost entry as an
unmatched existing addon. The result is two Uno entries, one of them dead, and every subsequent
push preserves it. The stale entry has to be removed in Nuvio's own UI, or by a one-off push of a
list that omits it.

## The dev auth bypass

`DEV_AUTH_BYPASS_TOKEN` (`internal/api/devauth.go`, wired in `cmd/server/main.go`) makes one
fixed bearer token authenticate as `sub = "dev-user"` with no JWKS fetch and no Nuvio account.
When the variable is set, `main.go` wraps both the `TokenVerifier` and the `NuvioClient` in the
decorators from `devauth.go` before building `api.Deps` — `internal/api` itself is unchanged and
still sees one verifier and one client. Every Nuvio call carrying the bypass token is served from
an in-memory fake account: two profiles, "Dev" at slot 1 and "Dev 2" at slot 2, each with its own
in-memory addon and collections store that starts empty and holds whatever pushes write into it —
enough to exercise switching profiles in the builder without a real Nuvio account. A request for
any other slot — `requireProfile` accepts 1–6 — fails instead of falling back to slot 1, so a
hand-made call can't read or overwrite the wrong profile's store. Any other token verifies and
routes normally.

Driving the SPA with it takes a second entry: Vite reads env files from `web/` only and exposes
only `VITE_`-prefixed vars, so the root `.env` is invisible to the frontend. Set
`VITE_DEV_AUTH_BYPASS_TOKEN` in `web/.env` (see `web/.env.example`) to the **same value** as the
root `.env`'s `DEV_AUTH_BYPASS_TOKEN`, and `/login` grows a "Dev bypass login" button that signs
in as the fake account — the whole authenticated UI is then drivable with no Nuvio credentials.
The button only exists in a dev build (`import.meta.env.DEV`). Nothing checks that the two values
match: a mismatch is rejected as an ordinary 401, which surfaces only as a redirect back to
`/login` with no hint that the token is the cause.

What the bypass reaches, and what it does not:

- **Reaches**: every path that ends in Uno's own vault or in TMDB — catalog and collection CRUD,
  selection, preview, the pickers.
- **Reaches partially**: push. The handler's ordering, the access validation, the
  owned-collection-ids-only merge and its addon-id heuristic for forgotten collections, and the
  compensating revert all run; what they run *against* is the in-memory fake, not Nuvio.
- **Does not reach**: anything depending on Nuvio's own behaviour — the addon URL round-trip
  through a real account, and every push failure branch whose failure originates upstream.
- The fake's fixed `sub = "dev-user"` alongside a real account is two subjects over one vault,
  which is what exercising cross-account isolation requires.

> **Never set `DEV_AUTH_BYPASS_TOKEN` on a deployed server.** It is a full auth bypass: anyone
> who knows the token holds the fake account, and it is the one env var whose presence changes
> who a request is. It is off by default (empty), and `api.LogDevBypassEnabled` prints a loud
> banner at startup so it can never be active unnoticed.

## Deployment

Docker, joining an existing external `edge` network on the target VPS — matching every other
service already running there rather than being the one bare-`systemd` outlier.

- **`Dockerfile`** — multi-stage: `node:22-alpine` (`npm ci && npm run build` → `web/dist`) →
  `golang:1.26-alpine` (`COPY --from=frontend-build`, `CGO_ENABLED=0 go build`) →
  `distroless/static-debian12`, running as `nonroot` (uid 65532).
- **`compose.yaml`** — external `edge` network, **no `ports:` mapping** (nothing binds to the
  host or needs a firewall rule; ingress reaches the container over `edge` by compose service
  name, resolved via Docker's embedded DNS), `env_file: .env`, named volume `uno-data` at
  `/data`, with `VAULT_DB=/data/vault.db` set directly in `compose.yaml` so it survives a
  container recreate regardless of what `.env` says.

> **Volume ownership trap.** Docker creates a fresh named volume owned by `root`, but the final
> image runs as `nonroot` (uid 65532), so `vault.InitDB` fails with
> `unable to open database file (14)` on first boot if `/data` is root-owned. The Dockerfile
> handles this by `chown`-ing an empty `/data` directory in the Go build stage and
> `COPY --chown`-ing it into the distroless stage, so Docker's copy-on-first-populate for a new
> empty volume picks up the right owner. **This only helps a brand-new volume** — a volume
> created without that ownership has to be dropped with `docker compose down -v`, not merely
> rebuilt.

## Database lifecycle

The schema is versioned. `PRAGMA user_version` records how far a database has been migrated, and
the migrations live one file each in `internal/vault/migrations/`. On every start, before it
opens the connection pool, `vault.InitDB` runs the runner in `internal/vault/migrate.go`:

1. It opens a connection of its own with foreign keys off and reads `user_version`. A version
   this build doesn't know (newer than its last migration, or negative) stops the start.
2. With nothing pending it does nothing more. Otherwise it writes a backup beside the database
   with `VACUUM INTO`, named `<db>.pre-v<N>-<UTC time>.bak`, where `N` is the version it is
   about to migrate to (`/data/vault.db.pre-v1-20260928T101500.123Z.bak` on the volume). The
   backup is taken even of an empty new file.
   - `VACUUM INTO` writes the same bytes for the same content. So when a restart fails the same
     way as the start before it, the new copy matches the newest earlier backup for `N`. It is
     dropped, and a restart loop keeps one copy.
   - Backups are otherwise never pruned.
3. It runs each pending migration in its own `BEGIN IMMEDIATE` transaction.
   - The transaction first re-reads `user_version` under the write lock. If another process has
     migrated the database in the meantime, the start fails instead of applying the migration a
     second time.
   - The transaction also sets `user_version`, and commits only if `PRAGMA foreign_key_check`
     finds nothing.
   - A failing migration rolls back, and the ones before it stay applied. `InitDB` then returns
     the error, and the server exits without serving.

The log names the backup and every migration applied, with its notes.

**Local dev:** deleting `vault.db` is still fine; the next start migrates an empty file.

**The rehearsal.** `migrate --dry-run --db <path>` is a subcommand of the same binary. Locally,
run `go run ./cmd/server migrate --dry-run --db <path>`; in the image, pass `migrate --dry-run
--db …` to `docker run`, since the binary is the entrypoint. It needs no `.env`. It:

- opens the file read-only and never writes to it. A read-only open of a WAL database creates
  `-wal` and `-shm` files, and it removes any it created;
- copies the database into a temporary directory with `VACUUM INTO`;
- runs every pending migration on the copy;
- prints the versions, each migration's notes, and every table's row count before and after.
  When a migration fails, it prints the report up to that point, then the error.

**Upgrading the deployed `uno-data` volume.** The volume holds real data, so never
`docker compose down -v` it.

1. Stop the app with `docker compose stop uno`. Copy the volume with
   `docker run --rm -v <vol>:/data -v "$PWD":/backup alpine cp -a /data/. /backup/uno-<date>/`.
   `docker volume ls | grep uno-data` gives `<vol>`.
2. Rehearse on the copy:
   `docker run --rm -v "$PWD/uno-<date>":/data <new-image> migrate --dry-run --db /data/vault.db`.
   The row counts should reconcile and every note should be one you expect. If you also run
   the new build locally against the copy, never push from it: that would add a localhost addon
   to the real Nuvio profile.
3. Deploy. The server writes its backup, migrates, and only then serves.
4. Roll back by restoring the backup and redeploying the previous image. Always restore first.
   A build with migrations refuses a database newer than it knows. The build from before
   migrations existed never reads `user_version`, and would start on the migrated file without
   complaint.
