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

## UI rules

What the app does today, written down so new screens match. Each rule says when to break it.

1. **Feedback.** A toast for a finished action ("Added to your catalogs", "Imported 2 catalogs"), through `useToasts`: a success leaves after 5 seconds, a failure stays until closed, and a notice explains a move the app made on its own. A lasting banner for something that needs a decision or a fix, such as a failed push. A line beside the field for a problem with that field. Break it when the outcome is worth reading in full rather than noticing: a push's outcome stays on screen until dismissed.
2. **Confirm or undo.** A server write that can't be taken back asks first (Delete, Unpublish, Discard). A draft needs neither, since nothing is saved until Save. An editor undoes in place, like a removed folder's Undo. Don't add a confirmation to anything that can be undone.
3. **Choice controls.** `Segmented` for one of a few, checkboxes for several, tabs only to switch whole panels. A choice from a long list is a select or a picker.
4. **Headers.** ← for a page reached from a list (a Community publication), × for an editor or a dialog.
5. **Form errors.** `FieldError` directly beneath the field, from the first Save on. `EditorFooter` reads the errors out when Save shows them. A server refusal that belongs to no field sits above the save bar's buttons.
6. **Loading, empty, error.** Through `StateView`, `size="compact"` in a picker or popover. An error says what happened and offers the next step, with no apology. An empty view invites the next action rather than saying "No items". A list with a skeleton draws it while loading.
7. **Components.** Buttons are `Button` or `IconButton`, text sizes are the text roles (`text-meta`, `text-title`, …), and colours are the colour tokens. The drift check in `npm test` fails a screen file that writes these raw. A deliberate one-off carries `drift-ok: <reason>` on its line or the line above.
8. **Overlays.** Menus, pickers and tips stay anchored to what opened them, on every screen. A form opens full screen on a phone, as a dialog or pane above. A confirmation is a centred alert. Only a view that works like a page (an editor, a publication, a folder page) adds a history step, so Back closes it.
9. **⋯ menus.** A ⋯ holds three or more actions; a row with one or two shows them as buttons or moves them to the page the row opens. Keep a ⋯ for one or two when the line has no room for them (a Home row beside its arrows) or when they are rare actions over the whole list (the Library rail's Import and Export beside its filter).
10. **Long text.** A name someone typed (a catalog, collection, folder or publication) wraps and is never cut short. A summary may be cut to at most two lines when its full form is a tap away, in the editor or on the page. So may a caption sized to a chip (a folder's name on a Home chip), whose full name heads what it opens. A `title` tooltip is never the only way to read something.
11. **Reordering.** Visible arrows where ordering is the screen's job (Home's rows, a collection's folders), entries in a ⋯ where it is occasional (a catalog inside a folder), and drag only as an extra for pointer users beside them.
12. **Focus.** When a dialog, menu or editor closes, focus goes back to what opened it (`components/returnFocus.ts`). When an action takes that away, focus goes to the nearest thing left: the next row's ⋯ (or the one above) for a removed row, the rail for a deleted one, and the arrow beside it when a moved row's arrow turns disabled. A row just created or duplicated takes focus on its editor's title.
