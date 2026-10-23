# Configuration

Uno reads environment variables, and a `.env` file in its working directory if there is one. The server refuses to start on a missing or invalid value, and names it in the error.

| Variable | Required | Default | Description |
|---|---|---|---|
| `SITE_BASE_URL` | Yes | — | Public URL Nuvio's apps reach this server at, with no trailing slash. Addon manifest URLs are built from it. |
| `NUVIO_PUBLISHABLE_KEY` | Yes | — | Nuvio's publishable API key. It's public; the browser uses it to sign in. |
| `TMDB_API_KEY` | In `shared` mode | — | The TMDB key every account uses. Must be unset in `per-account` mode. |
| `TMDB_KEY_MODE` | No | `shared` | `shared`, or `per-account`: each account saves its own TMDB key in the app. |
| `UNO_SECRET` | In `per-account` mode | — | 32 random bytes, base64 (`openssl rand -base64 32`). Encrypts the saved TMDB keys. |
| `UNO_ACCESS` | No | `open` | `open` lets any Nuvio account sign in. `allowlist` lets in only `UNO_ALLOWED_EMAILS`. |
| `UNO_ALLOWED_EMAILS` | With `allowlist` | — | Comma-separated email addresses. Must be unset while access is `open`. |
| `UNO_COMMUNITY` | No | `on` | `on`, `trusted-images`, `no-images` or `off`. `trusted-images` is `on` with every image and video address from a site not on `UNO_TRUSTED_HOSTS` left out, the way `no-images` leaves out all of them; use it on a server open to strangers. `no-images` leaves every image and video address out of Community and out of every copy added, duplicated or updated from then on; a copy added before keeps its pictures until an update from its publisher replaces them. Under both, a collection's Community page shows each address left out as text to copy and never loads it. `off` turns Community and publishing off; what was already shared is kept for when it is on again. |
| `UNO_TRUSTED_HOSTS` | No | Uno's list | The sites `trusted-images` shares pictures from, comma-separated: a site name, or `*.` and a name for the names under it. Unset: Uno's built-in list (TMDB, fanart.tv, TheTVDB, flagcdn.com, placehold.co, GitHub); `default` in it stands for that list. Must be unset unless Community is `trusted-images`. A site that sends viewers on wherever it's told (like `*.github.io`), or shows any other site's picture (like weserv), lets every picture through. |
| `PORT` | No | `8123` | Listen port. Plain HTTP. |
| `VAULT_DB` | No | `vault.db` | Path to the SQLite file. |
| `NUVIO_BASE_URL` | No | `https://api.nuvio.tv` | Nuvio's API. |
| `DEV_AUTH_BYPASS_TOKEN` | No | — | Local development only. At least 32 characters. Sending it as the bearer token signs in as a fake account, with an in-memory stand-in for Nuvio. Never set it on a server others can reach. |
| `UNO_TAG` | No | `latest` | Read by `compose.yaml` only: the image tag to run. |

The browser gets `NUVIO_BASE_URL`, `NUVIO_PUBLISHABLE_KEY`, the TMDB key mode and the Community setting at runtime: the server puts them in the page it serves, as a `<script type="application/json" id="uno-config">` block.
