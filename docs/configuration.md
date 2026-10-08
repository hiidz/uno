# Configuration and deployment

## Environment variables

Loaded by `internal/config`, which reads `.env` if present (via `godotenv`) and then the process
environment. See `.env.example`.

| Var | Required | Default | Purpose |
| --- | --- | --- | --- |
| `TMDB_KEY_MODE` | no | `shared` | How the server reaches TMDB: `shared` (one key for every account) or `per-account` (each Nuvio account enters its own) — see *TMDB key modes* |
| `TMDB_API_KEY` | in `shared` mode — startup fails if empty; must be unset in `per-account` | none | The key every account shares. Not public — every TMDB call is server-side for this reason |
| `UNO_SECRET` | in `per-account` mode | none | 32 random bytes, base64 (`openssl rand -base64 32`): the AES-256 key each account's TMDB key is sealed under. Ignored in `shared` mode |
| `NUVIO_PUBLISHABLE_KEY` | **yes** — startup fails if empty | none | `apikey` header on Nuvio REST/RPC calls, the server's and the SPA's (it reaches the SPA through `/config.js`). Public by design; it is printed in Nuvio's own public docs (https://nuvio.tv/docs#publishable-key) and intended for embedding in client apps. Not a service credential |
| `SITE_BASE_URL` | **yes** — startup fails if empty | none | Base for the absolute manifest URL handed to clients and pushed into Nuvio — see below |
| `VAULT_DB` | no | `vault.db` | Path to the SQLite file |
| `PORT` | no | `8123` | Listen port (plain HTTP, no TLS) |
| `NUVIO_BASE_URL` | no | `https://api.nuvio.tv` | Base for JWKS discovery and all REST/RPC calls, the server's and the SPA's login and refresh (through `/config.js`). Its origin is also the only cross-origin `connect-src` in the SPA's Content-Security-Policy |
| `DEV_AUTH_BYPASS_TOKEN` | no | empty (bypass off) | **Local development only**, at least 32 characters — see below |
| `UNO_ACCESS` | no | `open` | Who may sign in: `open` (any Nuvio account) or `allowlist` — see *Access* |
| `UNO_ALLOWED_EMAILS` | with `allowlist` | empty | Comma-separated email addresses of the Nuvio accounts admitted under `allowlist` |

`config.Load` collects every problem before failing: each missing required variable and each
value it can't use, in one error, rather than one per run.

## TMDB key modes

**`shared`** (the default, and what dev uses): every TMDB call — the builder's and the addon's —
goes out with `TMDB_API_KEY`.

**`per-account`**: there is no shared key. Each Nuvio account enters its own TMDB API key on the
profile picker, which holds the profiles until one is saved. The server checks a key with one TMDB
call before storing it, and stores it AES-GCM sealed under `UNO_SECRET`, bound to its provider and
account (`uno-account-key/1\0{provider}\0{account}`), beside its last four characters
(`docs/data-model.md` → `account_keys`). A key is never returned,
never logged, and only ever sent to TMDB. The builder uses the signed-in account's key; the addon
routes use the key of the account that owns the token's profile. Startup fails on an unknown mode,
on `shared` without `TMDB_API_KEY`, and on `per-account` with `TMDB_API_KEY` set (which would
read as a fallback that isn't there) or without a usable `UNO_SECRET`.

Operator notes:

- **Switching to `per-account` breaks the home screens of accounts without a key** until they add
  one: their rows fail to load (a page another account loaded in the last 30 minutes still comes
  from the shared cache). Switching back to `shared` needs `TMDB_API_KEY` again; stored keys stay,
  unused, and `UNO_SECRET` may stay set.
- **Losing or changing `UNO_SECRET` makes every stored key unreadable.** Each account's builder
  then says TMDB didn't accept its key, and its owner enters it again on the picker.
- The key routes (`/api/account/tmdb-key`) answer 404 in `shared` mode.

## Access

**`UNO_ACCESS`.** With `open`, the default, any Nuvio account that signs in can use the server.
With `allowlist`, only the accounts whose email address is in `UNO_ALLOWED_EMAILS` can; every
other one gets a 403 on every builder route, and the profile picker says the account can't use
this server. The public addon routes (`/u/{token}/…`) are not affected: a TV keeps loading
catalogs its profile already pushed. Startup fails on an unknown mode, on `allowlist` with no one
listed, and on emails listed while the mode is `open` (which would read as a restriction that
isn't there).

**`UNO_ALLOWED_EMAILS`** is a comma-separated list of the email addresses the accounts sign in to
Nuvio with, for example `me@example.com, partner@example.com`. Each entry is trimmed and
lower-cased, so case doesn't matter. An entry must be one address: exactly one `@`, text on both
sides, and no spaces. A Nuvio account id (a UUID) stops the start with a message saying the list
takes email addresses, and so does anything else that isn't an address, rather than matching
nobody. The server compares each entry with the `email` claim of the account's token, which
Supabase writes into every token. After an account changes its email, the old address keeps
matching until the token refreshes, within the hour. The dev bypass account has no email, and is
admitted by its id when the bypass is on.

> **Add an address only once its owner has a Nuvio account.** Nuvio doesn't verify the address
> an account signs up with: api.nuvio.tv confirms every new sign-up on the spot
> (`mailer_autoconfirm` in its `/auth/v1/settings`, read 2026-10-08), and so does the Nuvio
> self-host build by default (`ENABLE_EMAIL_AUTOCONFIRM=true`). An address can belong to only one
> Nuvio account, so a listed address that already has one is safe. A listed address that has
> none yet, a friend who hasn't signed up or a typo, is admitted for whoever registers it first.

**An open server has no moderation.** Under `open` any account can publish to Community, and
every account browsing it sees the titles, catalog names and cover images. Only a publication's
publisher can unpublish it (`vault.UnpublishCatalog`, `UnpublishCollection`). The operator's one
way to take one down is the same delete by hand, with the server stopped:
`PRAGMA foreign_keys = ON; DELETE FROM publications WHERE id = '<publication id>';`. The pragma
matters: the `sqlite3` shell starts with foreign keys off, and without it the publication's
subscriptions outlive it. With it, the schema does what an unpublish does: subscribers keep their
copies as their own.

### The `SITE_BASE_URL` hazard

`selectProfile` and `pushAddons` both build the absolute manifest URL pushed into Nuvio from
`SiteBaseURL`. `config.Load` (`internal/config/config.go`) has no default for `SITE_BASE_URL`
and includes it in the required-var check alongside `NUVIO_PUBLISHABLE_KEY` (and
`TMDB_API_KEY` in shared mode), so a deploy that leaves it unset fails startup immediately
rather than pushing an unreachable URL into the user's real Nuvio profile with no warning. `.env.example` ships
`SITE_BASE_URL=http://localhost:8123` as the correct local dev value — that's fine for local dev,
since the value is explicit there, not defaulted.

**This does not catch a value that's wrong but non-empty** — e.g. `.env.example`'s
`http://localhost:8123` placeholder copied verbatim into a production `.env`. Startup still
succeeds, and the wrong URL still gets pushed to Nuvio with no warning. `SITE_BASE_URL` must be
set correctly, not merely set, at deploy time.

**Correcting a profile already pushed with the wrong URL takes more than re-pushing.**
`pushAddons` (`internal/api/push.go`) reads the profile's current addon list, merges Uno's entry
in (`mergeAddon`), and pushes the complete merged list back — every other entry is carried
forward verbatim, because `sync_push_addons` is full-replace and omitting an addon would delete
someone else's. The merge drops another token's Uno entry only under the **current**
`SITE_BASE_URL`. So after fixing the variable, the next push finds no entry for the *new* URL,
appends it, and re-pushes a list that still contains the stale localhost entry as someone else's
addon. The result is two Uno entries, one of them dead, and every subsequent push preserves it.
The stale entry has to be removed in Nuvio's own UI, or by a one-off push of a list that omits
it.

## The dev auth bypass

`DEV_AUTH_BYPASS_TOKEN` (`internal/api/devauth.go`, wired in `cmd/uno/main.go`) makes one
fixed bearer token authenticate as `sub = "dev-user"` with no JWKS fetch and no Nuvio account.
When the variable is set, `apiDeps` in `main.go` wraps both the `TokenVerifier` and the `NuvioClient` in the
decorators from `devauth.go` before building `api.Deps` — `internal/api` itself is unchanged and
still sees one verifier and one client. Every Nuvio call carrying the bypass token is served from
an in-memory fake account: two profiles, "Dev" at slot 1 and "Dev 2" at slot 2, each with its own
in-memory addon and collections store that starts empty and holds whatever pushes write into it —
enough to exercise switching profiles in the builder without a real Nuvio account. A request for
any other slot — `requireProfile` accepts 1–6 — fails instead of falling back to slot 1, so a
hand-made call can't read or overwrite the wrong profile's store. Any other token verifies and
routes normally.

The Vite dev server reads the same `DEV_AUTH_BYPASS_TOKEN` from the root `.env`
(`web/vite.config.ts`) and hands it to the SPA, so `/login` grows a "Dev bypass login" button that
signs in as the fake account — the whole authenticated UI is then drivable with no Nuvio
credentials. Only `vite dev` reads it: `vite build`, in any mode, never inlines the token, and the
button only exists in a dev build (`import.meta.env.DEV`). A Vite dev server started before the
token changed still holds the old one; its login is rejected as an ordinary 401, which surfaces
only as a redirect back to `/login`.

What the bypass reaches, and what it does not:

- **Reaches**: every path that ends in Uno's own vault or in TMDB — catalog and collection CRUD,
  selection, preview, the pickers.
- **Reaches partially**: push. The handler's ordering, the access validation, the live profile
  check, the addon merge, the owned-or-last-pushed collections merge, and the compensating
  revert all run; what they run *against* is the in-memory fake, not Nuvio. The fake's profiles
  never use profile 1's addons.
- **Does not reach**: anything depending on Nuvio's own behaviour — the addon URL round-trip
  through a real account, and every push failure branch whose failure originates upstream.
- The fake's fixed `sub = "dev-user"` alongside a real account is two subjects over one vault,
  which is what exercising cross-account isolation requires.

> **Never set `DEV_AUTH_BYPASS_TOKEN` on a deployed server.** It is a full auth bypass: anyone
> who knows the token holds the fake account, and it is the one env var whose presence changes
> who a request is, and the allowlist admits its account. It is off by default (empty),
> `api.LogDevBypassEnabled` prints a loud banner at startup so it can never be active unnoticed,
> and a token shorter than 32 characters stops the start (`devBypassProblem`,
> `internal/config/config.go`), so a guessable one like `dev` can't be set at all. Make one with
> `openssl rand -hex 16`.

## Deployment

Each `v*` tag publishes an image to `ghcr.io/hiidz/uno` (`.github/workflows/release.yml`) for
`linux/amd64` and `linux/arm64`, tagged with its version, its `major.minor`, and `latest`. The
image holds no configuration: the server reads everything from its environment at startup, and
hands the SPA the Nuvio base URL and publishable key through `/config.js`, so one image serves
any Nuvio backend and survives a key rotation with a restart.

Nuvio fetches the addon from `SITE_BASE_URL`, so a deployment needs a public HTTPS URL; Uno
itself speaks plain HTTP and expects a reverse proxy in front.

### Docker Compose

```
cp .env.example .env   # fill in SITE_BASE_URL, NUVIO_PUBLISHABLE_KEY, TMDB_API_KEY
docker compose up -d
```

`compose.yaml` sets:

- `image: ghcr.io/hiidz/uno:${UNO_TAG:-latest}`. Set `UNO_TAG` in `.env` to pin a release, without its `v` (`1.2.0` for `v1.2.0`). A
  database at another schema version stops the start (see *Database lifecycle*), so upgrade one
  release at a time and read release notes for schema changes. `docker compose up -d` alone never
  re-pulls a tag already on the host; upgrade with `docker compose pull && docker compose up -d`.
- `127.0.0.1:8123:8123`: loopback only, for a reverse proxy on the same host. Docker-published
  ports bypass host firewalls such as UFW, so publishing on every interface would expose plain
  HTTP. The mapping assumes `PORT=8123`.
- `env_file: .env`, with `VAULT_DB=/data/vault.db` set in `compose.yaml` itself so the vault
  lives on the named volume `uno-data` at `/data` whatever `.env` says.
- `stop_grace_period: 70s`, longer than the server's 60s shutdown drain (`shutdownGracePeriod`,
  `cmd/uno/main.go`), so a push in flight finishes instead of being killed by Docker's default
  10s SIGKILL.

The volume is named `<project>_uno-data`, and the project name is the directory's. Moving the
compose files to another directory, or adding a top-level `name:`, points at a new empty volume.

### Behind a reverse proxy on a Docker network

A proxy running in Docker reaches Uno by service name over a shared network, with no host port.
Compose merges a `compose.override.yaml` beside `compose.yaml` automatically; it is gitignored
for exactly this:

```yaml
services:
  uno:
    ports: !reset []   # Docker Compose 2.24 or later
    networks: [edge]   # the proxy's network
networks:
  edge:
    external: true
```

`docker compose config` shows the merged result: check it has no `ports` before starting.

### Building the image yourself

```
docker build -t ghcr.io/hiidz/uno:local .
```

then `UNO_TAG=local` in `.env`. The `Dockerfile` is multi-stage: `node:22-alpine`
(`npm ci && npm run build` → `web/dist`) → `golang:1.26-alpine` (`CGO_ENABLED=0 go build`,
cross-compiled for the target platform) → `distroless/static-debian12`, running as `nonroot`
(uid 65532).

### Without Docker

Build as in the README (`npm run build` in `web/`, then `go build ./cmd/uno`), set the variables
above in the environment or a `.env` beside the binary, and point `VAULT_DB` at persistent
storage.

> **Volume ownership trap.** Docker creates a fresh named volume owned by `root`, but the final
> image runs as `nonroot` (uid 65532), so `vault.InitDB` fails with
> `unable to open database file (14)` on first boot if `/data` is root-owned. The Dockerfile
> handles this by `chown`-ing an empty `/data` directory in the Go build stage and
> `COPY --chown`-ing it into the distroless stage, so Docker's copy-on-first-populate for a new
> empty volume picks up the right owner. **This only helps a brand-new volume** — a volume
> created without that ownership has to be dropped with `docker compose down -v`, not merely
> rebuilt.

## Releasing

A push to `main` runs CI only. An image is built only for a version tag, so a release is a tag:

1. Pick the next version: the last number for fixes (`v0.1.0` → `v0.1.1`), the middle one for
   features (`v0.2.0`). A `-rc.N` suffix (`v0.2.0-rc.1`) makes a prerelease, for trying a build
   before it becomes a release.
2. Tag the commit and push the tag: `git tag v0.1.1 && git push origin v0.1.1`, or on GitHub,
   **Releases → Draft a new release** with a new tag. The release notes say when a release
   changes the database schema.
3. **Actions → Release** builds and publishes `ghcr.io/hiidz/uno:0.1.1` (the tag without its
   `v`), `:0.1`, and `:latest`.
4. On a server: `docker compose pull && docker compose up -d`, with `UNO_TAG` set to the new
   version or unset to follow `latest`.

## Database lifecycle

The schema is `internal/vault/schema.sql`, embedded in the binary. On every start `vault.InitDB`
reads `PRAGMA user_version` in one `BEGIN IMMEDIATE` transaction:

- at `0`, an empty or new file, it creates the schema and sets `user_version` to
  `schemaVersion` (`internal/vault/db.go`) in that same transaction;
- at `schemaVersion` it does nothing;
- at any other version the start fails, naming both versions, and the server exits without
  serving.

A schema change edits `schema.sql` and bumps `schemaVersion`. When a database at the previous
version has to keep its rows, the change ships with a one-off `uno migrate --db <path>` subcommand
(`cmd/uno/migrate.go`) that is deleted once prod has run it; none exists now.

**Local dev:** deleting `vault.db` is fine; the next start creates the schema.

**The deployed `uno-data` volume** holds real data, so never `docker compose down -v` it.
