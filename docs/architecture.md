# Architecture

Uno is one Go binary. It serves three things over plain HTTP on one port, and keeps its data in one SQLite file:

- **Builder API** (`/api/...`): JSON for the web app. Needs a Nuvio sign-in.
- **Addon** (`/u/{token}/...`): the Stremio addon protocol, which Nuvio's apps read. Public.
- **Web app** (everything else): the React app, embedded in the binary.

```mermaid
flowchart LR
    Browser["Web app<br/>(React)"]
    App["Nuvio apps"]
    subgraph Uno["uno binary"]
        API["api"]
        Addon["addon"]
        Vault["vault"]
        Provider["provider<br/>(TMDB)"]
        NuvioC["nuvio"]
    end
    DB[("SQLite")]
    TMDB["TMDB"]
    Nuvio["Nuvio API"]

    Browser -- "sign in" --> Nuvio
    Browser -- "/api with Nuvio token" --> API
    App -- "/u/{token}" --> Addon
    App --> Nuvio
    API --> Vault
    API --> Provider
    API --> NuvioC
    Addon --> Vault
    Addon --> Provider
    Vault --> DB
    Provider --> TMDB
    NuvioC --> Nuvio
```

## Packages

| Package | Does |
|---|---|
| `cmd/uno` | Entry point: config, database, server, graceful shutdown. |
| `internal/config` | Reads and checks environment variables. |
| `internal/api` | The builder API, sign-in checks, and push. All routes are in `server.go`. |
| `internal/addon` | The public addon: manifest and catalog pages. |
| `internal/vault` | The SQLite database. Schema in `schema.sql`. |
| `internal/provider` | TMDB: recipe checks, lookups, previews, catalog pages, caching and rate limits. |
| `internal/nuvio` | Nuvio: token verification and the profile, addon, collection and home-order calls. |
| `internal/tmdbkey` | Encrypts per-account TMDB keys. |
| `internal/static` | Serves the web app, its page carrying the runtime settings. |
| `web` | The React app. `web/embed.go` embeds the build. |

## Sign-in

Uno has no accounts. The web app signs in with Nuvio directly and sends Nuvio's access token on every `/api` call. The server verifies the token against Nuvio's public keys, checks `UNO_ACCESS`, and for `/api/p/{profileIndex}/...` routes looks up the Uno profile for that Nuvio profile slot (1 to 6).

The check is local: the token's signature, issuer, audience and expiry, with no call to Nuvio. So a token stays valid until the expiry Nuvio gave it (about a week), even after the user signs out or changes their password. Uno doesn't set that lifetime.

## Push

A push sends the whole home screen to `POST /api/p/{profileIndex}/push`. Uno then writes three lists into the Nuvio profile:

1. Addons: makes sure this profile's Uno addon URL is installed.
2. Collections.
3. Home order.

Nuvio replaces each list whole, so Uno pulls each one first and keeps every entry that isn't its own. If a step fails, Uno puts back what it changed. Only after all three succeed does Uno save the new home screen, and a record of what it pushed.

Nuvio's rate limit is shared across its clients, so the three routes that call Nuvio (push, the profile list and profile select) allow each account 10 requests at once, then one more every 3 seconds, and answer a `429` with `Retry-After` beyond that. A `429` from Nuvio itself is passed on the same way.

## Addon

Nuvio's apps load `{SITE_BASE_URL}/u/{token}/manifest.json`. Each profile has its own random token. The addon serves the catalogs from the profile's **last push**, not the current edits, so a change shows up in Nuvio after the next push. Catalog pages come from TMDB, 20 titles at a time.

## Sharing

Profiles on the same server can **publish** catalogs and collections to Community. Others can **add** one (a linked copy that can take updates) or **duplicate** it (an independent copy).

Publishers type in image and video addresses, and every viewer's browser and Nuvio fetches them from the host they name. Uno takes only `https` ones, and `UNO_COMMUNITY` (`docs/configuration.md`) can leave them out of Community (`no-images`) or turn Community off.

Whoever runs the server can take a post down, from a shell on it: `uno takedown <publication id>`, the id being the end of the post's Community address. It works as Unpublish does: the publisher keeps the source, subscribers keep their copies as their own, art included, and nothing in anyone's Nuvio changes. `--block` also takes down every post of the publisher's Nuvio account, across its profiles, and stops the account publishing; it can still build and push to its own Nuvio. `uno blocked` lists blocked accounts and `uno unblock <account id>` lets one publish again. Each command shows what it does, and `takedown` asks first. They run beside the server: in Docker, `docker compose exec uno /app/uno takedown <publication id>`.

## API

[`api/openapi.yaml`](api/openapi.yaml) lists every route. A Go test (`TestOpenAPISpecMatchesRoutes`) fails if the spec and the routes in `server.go` disagree.
