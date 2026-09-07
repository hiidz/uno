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
| `NUVIO_BASE_URL` | no | `https://api.nuvio.tv` | Base for JWKS discovery and all REST/RPC calls |
| `DEV_AUTH_BYPASS_TOKEN` | no | empty (bypass off) | **Local development only** — see below |

`config.Load` collects *all* missing required vars before failing, so a fresh setup gets one
error naming both rather than two runs.

### The `SITE_BASE_URL` hazard

`selectProfile` and `pushAddons` both build the absolute manifest URL pushed into Nuvio from
`SiteBaseURL`, so a deploy that left it unset — or, previously, that inherited a silent
`http://localhost:8123` default — would push an unreachable URL into the user's real Nuvio
profile with no warning at any point. `config.Load` (`internal/config/config.go`) now has no
default for `SITE_BASE_URL` and includes it in the required-var check alongside `TMDB_API_KEY`
and `NUVIO_PUBLISHABLE_KEY`, so an unset value fails startup immediately instead of silently
defaulting. `.env.example` still ships `SITE_BASE_URL=http://localhost:8123` as the correct local
dev value — that's fine for local dev, since the value is explicit there, not defaulted.

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
an in-memory fake account: one profile at slot 1, plus in-memory addon and collections stores
that start empty and hold whatever pushes write into them. Any other token verifies and routes
normally. Set it and the whole authenticated UI is drivable with no Nuvio credentials.

What the bypass reaches, and what it does not:

- **Reaches**: every path that ends in Uno's own vault or in TMDB — catalog and collection CRUD,
  selection, preview, the pickers.
- **Reaches partially**: push. The handler's ordering, the access validation, the
  `owned ∪ old ∪ new` merge union, and the compensating revert all run; what they run *against*
  is the in-memory fake, not Nuvio.
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

There are no migrations. `vault.InitDB` runs `CREATE TABLE IF NOT EXISTS`, which never alters an
existing table, so any schema change means deleting and recreating the database — the local dev
`vault.db` and the `uno-data` compose volume (`docker compose down -v`) both. A database whose
schema predates the current one fails hard on the affected writes rather than degrading silently.
