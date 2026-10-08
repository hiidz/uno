# Configuration

Uno reads environment variables, and a `.env` file in its working directory if there is one. The server refuses to start on a missing or invalid value, and names it in the error.

| Variable | Required | Default | Description |
|---|---|---|---|
| `SITE_BASE_URL` | Yes | — | Public URL Nuvio's apps reach this server at, with no trailing slash. Addon manifest URLs are built from it. |
| `NUVIO_PUBLISHABLE_KEY` | Yes | — | Nuvio's publishable API key. It's public; the browser uses it to sign in. |
| `TMDB_API_KEY` | In `shared` mode | — | The TMDB key every account uses. |
| `TMDB_KEY_MODE` | No | `shared` | `shared`, or `per-account`: each account saves its own TMDB key in the app. |
| `UNO_SECRET` | In `per-account` mode | — | 32 random bytes, base64 (`openssl rand -base64 32`). Encrypts the saved TMDB keys. |
| `UNO_ACCESS` | No | `open` | `open` lets any Nuvio account sign in. `allowlist` lets in only `UNO_ALLOWED_EMAILS`. |
| `UNO_ALLOWED_EMAILS` | With `allowlist` | — | Comma-separated email addresses. |
| `PORT` | No | `8123` | Listen port. Plain HTTP. |
| `VAULT_DB` | No | `vault.db` | Path to the SQLite file. |
| `NUVIO_BASE_URL` | No | `https://api.nuvio.tv` | Nuvio's API. |
| `DEV_AUTH_BYPASS_TOKEN` | No | — | Local development only. At least 32 characters. Sending it as the bearer token signs in as a fake account, with an in-memory stand-in for Nuvio. Never set it on a server others can reach. |
| `UNO_TAG` | No | `latest` | Read by `compose.yaml` only: the image tag to run. |

The browser gets `NUVIO_BASE_URL` and `NUVIO_PUBLISHABLE_KEY` from `/config.js`, which the server generates at runtime.
