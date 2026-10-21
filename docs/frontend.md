# Frontend

The web app in `web/` is React 19 and TypeScript, built with Vite. It uses React Router for pages, TanStack Query for server data, Tailwind CSS for styling, and Radix UI for dialogs and menus.

`npm run build` writes it to `web/dist`, which the Go binary embeds and serves. In development, `npm run dev` serves it, proxies `/api` and `/u` to the Go server on port 8123, and copies the settings block from the Go server's page into its own, so the Go server must be running first.

## Pages

| Path | Page |
|---|---|
| `/` | The builder on the profile picked last, or the picker when none is. |
| `/login` | Sign in with your Nuvio email and password. |
| `/profiles` | Pick a Nuvio profile. |
| `/configure` | The builder: library and home screen, on the profile picked last. |
| `/configure/catalog/{id}`, `/configure/collection/{id}` | The builder with that row's editor open. |
| `/community`, `/community/catalog/{id}`, `/community/collection/{id}` | The builder's Community section, and one publication's page. |
| anything else | A "page doesn't exist" page with a link home. A page that throws shows a "Something went wrong" page with Reload. |

The profile is not in the address. The browser remembers the last profile picked for each Nuvio account, and an open tab keeps its own. A builder address with no profile picked yet goes to `/profiles` first, then on to the address. An address naming a row this profile doesn't have goes to its section with a short note.

Opening an editor or a publication's page adds a browser history step, so Back closes it. Moving from one editor to another doesn't add one. Back, or any move that would discard unsaved editor changes or unpushed home-screen changes, asks first.

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
