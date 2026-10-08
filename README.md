# Uno

Build your own TMDB catalogs and collections, and push them onto your Nuvio home screen.

[![CI](https://github.com/hiidz/uno/actions/workflows/ci.yml/badge.svg)](https://github.com/hiidz/uno/actions/workflows/ci.yml)

## What it does

- Make catalogs from TMDB filters: genres, ratings, release dates, streaming services, studios, keywords, networks.
- Group catalogs into collections with folders and artwork.
- Arrange your home screen and push it into a Nuvio profile in one step.
- Serve each profile's catalogs as a Stremio-protocol addon that Nuvio's apps read.
- Share catalogs and collections with other profiles on the same server, and export or import them as JSON.

You sign in with your Nuvio account. Uno has no accounts of its own.

## How it works

Uno is a single Go binary with a React web app built in, and it stores its data in one SQLite file. You build in the web app. A push writes the Uno addon, your collections and your home screen order into your Nuvio profile. Nuvio's apps then fetch the catalogs from Uno, which gets the titles from TMDB.

## Run it

```sh
curl -fsSLO https://raw.githubusercontent.com/hiidz/uno/main/compose.yaml
curl -fsSL -o .env https://raw.githubusercontent.com/hiidz/uno/main/.env.example
# set SITE_BASE_URL, NUVIO_PUBLISHABLE_KEY and TMDB_API_KEY in .env
docker compose up -d
```

Then open <http://localhost:8123>. Every setting is listed in [configuration](docs/configuration.md).

## Develop

```sh
go run ./cmd/uno              # API on :8123
cd web && npm ci && npm run dev   # web app, proxies to :8123
```

## Docs

- [Architecture](docs/architecture.md): the parts and how they fit
- [Data model](docs/data-model.md): what's stored
- [Frontend](docs/frontend.md): how the web app is laid out
- [Configuration](docs/configuration.md): environment variables
- [API](docs/api/openapi.yaml): every HTTP route

## License

MIT.
