# Frontend

The web app in `web/` is React 19 and TypeScript, built with Vite. It uses React Router for pages, TanStack Query for server data, Tailwind CSS for styling, and Radix UI for dialogs and menus.

`npm run build` writes it to `web/dist`, which the Go binary embeds and serves. In development, `npm run dev` serves it and proxies `/api`, `/u` and `/config.js` to the Go server on port 8123.

## Pages

| Path | Page |
|---|---|
| `/login` | Sign in with your Nuvio email and password. |
| `/profiles` | Pick a Nuvio profile. |
| `/configure` | The builder: library, editors, home screen, Community. |

## Layout

```text
web/src/
  routes/       pages and the router
  auth/         Nuvio sign-in and session
  api/          every call to the server, wire types, query keys
  features/     the builder, one folder per area (catalogs, collections, home, push, community, ...)
  components/   shared UI pieces
  lib/          small helpers
  test/         test setup and fixtures
```

## Conventions

- Every server call goes through a function in `src/api`. Don't call `fetch` directly.
- The Nuvio access token stays in memory, and the refresh token in `localStorage`. On a `401`, `apiFetch` refreshes once and retries.
- Home edits stay in the browser until you push.
- Tests sit next to the code as `*.test.ts(x)` and run with `npm test`. A test that renders a component starts with `// @vitest-environment jsdom`.
