# Frontend

`web/` — Vite + React 19 + TypeScript, CSR only, no SSR. Everything sits behind per-user Nuvio
auth, nothing is public or crawlable (the addon server already owns the one public surface), and
there is no server-side session to render against.

Monorepo: `web/` at the repo root beside the Go module. One repo, one pipeline. `vite build` →
`web/dist` → `//go:embed all:dist` (`web/embed.go`) → served same-origin on the same port as the
API and addon server. No CORS in production; the Vite proxy is a dev-only convenience. `web/dist`
is gitignored except for the committed `.gitkeep`, which keeps the embed target present — and
`go build ./...` working — in a tree where no frontend build has run.

| Concern | Choice |
| --- | --- |
| Router | React Router. Four routes total: `/login`, `/` (redirects to `/profiles`), `/profiles`, `/configure` |
| Server state | TanStack Query, keys scoped by profile under `['p', i, …]` so switching slots invalidates cleanly with no manual cache wipe |
| Drag-and-drop | dnd-kit — `MouseSensor` + `TouchSensor` (hold to drag) + `KeyboardSensor`, so every reorderable list is operable with no pointer at all |
| Components | Hand-written, on Radix UI primitives wherever a control needs real accessible behaviour: `Dialog` under `Modal` and `ConfirmDialog`, `Popover`, `Slider`, `DropdownMenu`, and `Tabs` on the preview's folder page. The rest — `Toast`, `ListState`, `fields.tsx`, `GlyphButton` — is plain local markup. Styling is Tailwind v4 against Uno's own `--uno-*` tokens in `web/src/index.css`; a small base set (`--background`, `--foreground`, `--border`, `--ring`, `--sidebar`) is aliased onto the Uno palette |
| Types | **Hand-written per endpoint, no codegen.** For a one-person team on both ends, drift surfaces immediately in the browser rather than silently in production. If drift pain ever shows up, a lightweight generator reading the Go structs (`tygo`-style) is the first upgrade to reach for, not a full OpenAPI pipeline |

Because there is no codegen, **`"strict": true` must stay on in `web/tsconfig.app.json`.** The
`| null` unions and `?? []` guards in `web/src/api/types.ts` are the only thing enforcing the
wire contract; without `strictNullChecks` they are documentation the compiler never reads. The
code is strict-clean, so turning it off would cost nothing immediately and remove the only drift
check that exists.

Load-bearing shared modules, rather than a directory listing: `web/src/components/fields.tsx`
holds `Field`, `TextInput`, `Select`, `Segmented`, `Checkbox` — shared between both editors —
while `web/src/features/catalogs/fields.tsx` re-exports them and keeps `NumberInput`,
`RangeField`, `GenreCycler` and `CertificationPicker`, which are catalog-shaped. `EditorShell` +
`EditorFooter` + `useEditorForm` under `web/src/features/builder/` are the scaffolding both
editors sit in, so a catalog and a collection get the same header, dirty state, footer, and
save/discard behaviour from one place. `web/src/features/preview/` holds the preview model
(`model.ts`), the tile queries, and the Figtree-styled tiles the catalog editor's results panel
draws. The preview rows — the Home pane's Preview tab, the collection
editor's Preview panel and Community's collection preview — are `web/src/features/home/previewScreen.tsx`:
Nuvio's layout (scrolling rows, captions, folder tiles that open folder pages) in the same Figtree
and tonal steps as the rest of the app, with no note pinned between the rows (see "Home pane —
Preview view" below). Nothing under `features/` is a separate URL; `Builder`
composes all of it.

Every row either editor opens is a saved one, and `EditorTarget`
(`web/src/features/builder/target.ts`) always carries its `id`. A catalog or collection is named
into existence before its editor opens, and duplicating a catalog (`useCatalogMutations`'
`duplicate`) or a collection (`useCollectionMutations`'s
`duplicate`) is a single server call that hands back a finished copy — see "Catalog authoring"
and "Collection authoring" below. An open editor reads only its `update` mutation's pending state
and error: `create` belongs to the naming dialogs and `duplicate` to Duplicate, which can run while an editor
is open, and their failures stay in their own dialogs.

## Auth / session

Entirely frontend code. Uno's Go side never mints, refreshes, or stores a Nuvio credential.

- **Login and refresh talk directly to Nuvio** — `POST /auth/v1/token?grant_type=password` and
  `?grant_type=refresh_token` (`web/src/auth/client.ts`), against the same base URL and
  publishable key the server uses.
- **Access token in memory only** (never persisted, so a reload can't leak it from disk);
  **refresh token in `localStorage`**, since it must survive a reload and there is no backend
  session to hold it. On app load, `bootstrap()` proactively exchanges the refresh token before
  rendering anything that needs an access token, and is **memoized** so React StrictMode's
  double effect-invoke in dev cannot present an already-burned refresh token.
- **Single-flight refresh** — concurrent 401s in one tab share one in-flight call.
- **Cross-tab sync** — `BroadcastChannel` with a `storage`-event fallback, both idempotent
  against each other. Necessary because refresh tokens rotate and burn on use, so two tabs
  racing would strand one. A refresh attempt that loses the race *adopts the winning tab's
  session* rather than signing out a still-valid one.
- **Only a refused refresh token ends the session** (`isTokenRefused`): a `NuvioAuthError` with
  a 4xx other than 408 or 429. A network error, a 5xx, a 408 or a 429 keeps the session and the
  stored refresh token, so waking a laptop before its Wi-Fi is up fails a request rather than
  signing out every tab and dropping unpushed Home edits. During `bootstrap()` such a failure
  settles signed out, and the next load redeems the stored token again.
- **`apiFetch`** (`web/src/api/client.ts`) attaches the bearer header to every `/api/*` call,
  catches `401`, refreshes once, retries the original request, and only then clears session
  state and navigates to `/login` via the `router` singleton exported from
  `web/src/routes/router.tsx`. A refresh that fails for any reason but a refused token rejects
  the request instead.
- **The query cache is per account.** No query key carries the user, so
  `web/src/lib/query-client.ts` subscribes to auth state and calls `queryClient.clear()`
  whenever the signed-in user id changes — sign-out, "Switch account", or another tab's
  session for a different user.
- **The profile's cache is dropped each time the builder opens on a profile.** The router's
  location state is where the active profile is set, so `watchActiveProfile`
  (`web/src/routes/activeProfile.ts`, wired in `App.tsx`) watches it and calls
  `removeQueries({ queryKey: ['p'] })` whenever `/configure` opens on a profile — from the picker,
  from a history entry, or from one profile's entry to another's. The account-wide queries (genres,
  lookups, recipe previews, the TMDB key) stay. `Builder` keys `HomeSelectionProvider` on the
  profile index, so a profile change under the mounted builder starts a new pending and baseline
  state rather than keeping the last profile's. Queries refetch on window focus once stale, so a
  change made in another tab or on another device shows without a reload; the account-wide
  lookups kept with `staleTime: Infinity` never go stale and are not refetched.
- **Dev bypass** — in a dev build only, `/login` renders a "Dev bypass login" button when
  `VITE_DEV_AUTH_BYPASS_TOKEN` is set in `web/.env`. `loginWithBypassToken` (`session.ts`) builds
  a synthetic session holding that token as its access token and applies it directly, with no
  Nuvio round trip; the server accepts it via `DEV_AUTH_BYPASS_TOKEN` (see
  `docs/configuration.md`). The session carries no refresh token, so it is neither persisted nor
  broadcast: a reload returns to `/login`, and a real session in another tab is untouched. The
  whole branch is gated on `import.meta.env.DEV` and cannot exist in a production build.
- **`RequireAuth`** wraps `/`, `/profiles`, and `/configure`. `AuthProvider` withholds rendering
  entirely while the load-time exchange is in flight, so routes only ever see a settled status,
  never `'loading'`.

## Profile selection

`/profiles` renders `ProfilePicker`: `GET /api/profiles` (Nuvio's live list) → pick →
`POST /api/profiles/select` → navigate to `/configure` with `{profileIndex, profileName,
manifestURL, sharesAddons}` as React Router **navigation state**, not a URL param.

**Each card draws the profile as Nuvio does** (`ProfileAvatar`, `PinBadge`): its picture
(`avatar_image_url`, an upload or one of Nuvio's built-in avatars) in a circle beside its name,
else that circle in the profile's Nuvio colour with its initial, and "PIN in Nuvio" beside the slot
when the profile has a PIN. A picture that fails to load leaves the coloured circle. The circle is
decorative: the card's name already says whose it is, and the PIN joins that name.

**A profile that uses profile 1's addons in Nuvio** (`uses_primary_addons`, *Nuvio integration* in
`docs/architecture.md`) opens like any other, but its card says "Uses profile 1's addons in Nuvio
· Push is off" (`SharesAddonsNote`), and `sharesAddons` keeps Push off in the builder (*Push UI*).

`profileIndex` deliberately does not live in the URL. A bookmarkable `/p/:profileIndex` would let
a user land on `/configure` for an arbitrary index without going through selection. Router state
lives on the history entry, so it survives client-side back/forward but is lost on a hard reload
or a shared link — `Builder` reads `useLocation().state` and renders
`<Navigate to="/profiles" replace />` when it's `null`. That is intentional. Passing the name
along also avoids a second `GET /api/profiles` round-trip just to resolve a name for the header.

The profile chip in the header *is* the switcher, and it navigates back to `/profiles` rather
than offering an in-place dropdown — a second selection path would mean two ways to do the same
thing, and `POST /api/profiles/select` needs calling either way. It confirms through a dialog
naming the pending count when edits are pending; `useUnloadGuard` covers reload and tab-close.
Both count only the edits that exist in the tab alone (`unsavedCount`, behind `isDirty`). A
"changed since it was last pushed" line names a collection already saved in the vault, so it
stays in the header count and the list of changes but arms neither guard.

**A refused account.** When the server's access policy doesn't admit the signed-in Nuvio account
(`UNO_ACCESS=allowlist`, `docs/configuration.md`), the profile calls answer 403, and the picker
shows a plain card in place of the profiles ("This Nuvio account can't use this Uno.") rather
than an alert (`pickerFailure`, `RefusedAccount` in `ProfilePicker.tsx`). "Sign in with a
different account" below it is the way on. Any other failure still shows as the alert with the
server's words.

**The TMDB key** (`web/src/features/account/`). On a server in per-account key mode
(`GET /api/config`, `docs/configuration.md` → *TMDB key modes*), the picker asks for the
account's own TMDB key; in shared mode, or for a refused account, there is no key UI anywhere.
`useKeyStep` reads the mode and `GET /api/account/tmdb-key` and puts the picker in one of these
steps:

- **Checking.** Until both answers are in, the profiles are held (`holdsProfiles`) with no card
  shown, so a slow key status can't let a keyless account into the builder. A failed answer asks
  nothing and holds nothing; the builder's banner covers that account.
- **Needed.** `TMDBKeyGate` sits above the profiles: "Add your TMDB key to start", one sentence
  on what TMDB is and where its API Key is (a link to TMDB's API settings page), the key field and
  Save. It holds the profiles (`cardOff`, greyed and not pressable, each pointing at the gate's
  heading through `cardNote`): without a key, neither the builder nor the account's home screen
  rows can reach TMDB. This is the one screen that names TMDB, since the user has to go and get a
  key there by name.
- **Set.** `TMDBKeyShelf` sits under the profiles: TMDB key, a Set sticker (the dim outline;
  pink would say Published), "ends in" and the last four characters, Replace (the same form in place,
  with Cancel) and Remove. Remove confirms first, saying the home screen can't load Uno's rows and
  the builder can't preview or save catalogs until a key is back; it isn't red, since adding the
  key again undoes it.
- **Saving** asks TMDB, so Save reads "Checking…" while it runs. The server's refusal shows under
  the field in its own words: a key of the wrong shape, TMDB's Read Access Token pasted instead
  of the API Key, a key TMDB refuses, or TMDB unreachable. A save writes the new status straight
  into the key query and clears the builder's key problem.

**A key problem in the builder.** Any call that reaches TMDB can come back `422`: the account
has no key, or TMDB no longer accepts it (revoked, or the server's `UNO_SECRET` changed). The
query client offers every failed query and mutation to `keyProblem.ts`, which notes a `422` and
holds it until a key is saved or the account changes. `KeyProblemBanner`, under the builder's
header with the push banner, then says the key was refused (or is missing) and that the home
screen can't load Uno's rows until it is replaced (or added), since the same key serves those
rows. Its button, Replace key or Add key, goes back to the picker through the header's own
guard, as switching profile does. If reading the key's status fails, the banner still shows, in
words that fit either case ("Uno couldn't use your TMDB key", Check key). The call that failed
shows its own error as usual.

## `/configure` — Workspace and Community

`web/src/routes/Builder.tsx` renders one top-level `Segmented` switch, Workspace / Community
(its own row below `lg`, matching DESIGN.md's two-row phone top bar) — everything below is one
of the two. Push and the profile chip sit in the header outside both, and so does the pending
indicator (beside Push from `lg` up, joined to Push as one pill below it):
Home's state and its commit don't belong to either tab. Switching tabs goes through the same
`EditorGuard` an in-app exit already does, since it unmounts `Workspace` (and any editor mid-edit
inside it) the same way leaving `/configure` would; unlike leaving the page, it also resets
`EditorGuard`'s `dirty` flag itself, because that provider spans both tabs and a stale `true`
from the editor just discarded would wrongly guard the next tab switch too.

### Workspace tab — two regions on one page

**Selecting is the common path; authoring is the rare one.** A generic user picks from their own
catalogs and collections, previews the result, pushes, and never opens an editor. That is why
this is two regions rather than several co-equal panes.

| Region | Holds |
| --- | --- |
| **Library** — left rail | Every catalog and collection you own, no heading above them. One search box at the top of the rail, with a ⋯ menu beside it for Import JSON and Export JSON, narrows both the catalog and collection lists at once. Compact rows: name plus a one-line recipe summary. The closed-graph sharing model means this is the whole library — adding what someone else publishes is the separate Community tab's job, not a second group in this rail. A row added from there is one of your rows, marked From Community |
| **Pane** — right | One thing at a time: your home screen (with a `List \| Preview` switch), or the editor for whichever rail row is selected. The page's centre of gravity |
| **Push** — header | Global action, beside a persistent unpushed-changes indicator. Not a section — it's the commit for Home, so it lives where Home is always visible, whether or not Home is the pane's current occupant |

**From `lg`, authoring replaces the pane's occupant rather than opening over it.** Picking a rail
row to edit swaps Home out for that row's editor; leaving the editor swaps Home back. Picking the
row that is already open is a way out too — the same guarded close as × (`selectionAction`,
`builder/selection.ts`). The row says which is open with `aria-expanded`. `Workspace`
(`web/src/features/builder/Workspace.tsx`) owns that one-occupant rule, and `EditorTarget`
(`web/src/features/builder/target.ts`) is the union describing what the pane currently holds.

**Below `lg` the rail and Home stack into one scrolling document, and an open editor covers it
as a layer.** `EditorLayer` (`web/src/features/builder/EditorLayer.tsx`) is the pane's slot for
an editor or From Community view at every width — the same element, so crossing `lg` keeps the
editor and its unsaved edits. Below `lg` it is `fixed inset-0` at `z-[35]` (over the app header's
`z-30`, under menus' `z-40` and dialogs' `z-50`), with `role="dialog"`, `aria-modal` and
`data-editor-layer`; Home stays mounted underneath it, so closing leaves the page where it was.
While it covers the page, every sibling of the layer and of its ancestors up to `<body>` (the
rail, Home, the header and tab row) is `inert`, as a modal dialog shuts out the page behind it;
`<html>` stops scrolling; focus
lands on the editor's title and returns to what opened it, or to the rail when that is gone (a
deleted row). Opening fades it in over 160ms, off under reduced motion (`.layer-in`,
`index.css`). The browser's Back closes it through the same guard as × (`useLayerHistory.ts`, a
`unoEditor` history entry from `lib/historyEntry.ts`): Keep editing puts the entry back, so the
next Back asks again, and every other way out consumes the entry. The entry drops `unoFolder`
from the state it copies, so a Home folder page open underneath keeps its own entry and closes
only on the next Back. `web/src/features/builder/stacked.ts` owns the threshold (`matchMedia`
against `--breakpoint-lg`), the two scroll shortcuts between rail and Home (the rail's "Your home
screen ↓" and Home's Library button), and the published header height the sticky offsets need. The breakpoint value is asserted explicitly as
`--breakpoint-lg` in `web/src/index.css` and mirrored as `LG_BREAKPOINT` in
`web/src/lib/breakpoints.ts`, because `stacked.ts` needs the same threshold as a `matchMedia`
query and Tailwind's CSS output doesn't expose it to JS. Change one and you must change the
other; each side carries a comment pointing at its twin.

**Two kinds of unsaved work, deliberately separate.** An editor's changes are saved to the
server; Home's are pushed to Nuvio. They are lost in different ways, they warn separately, and
the header shows only Home's — an editor states its own inside the pane. `EditorGuard` is what
stops a rail click from discarding an editor mid-edit.

**Vocabulary:** the UI says **"your home screen"**, never "selection". `selection` is the
schema's word and stays in code, types, and endpoint names; it does not appear on screen.
`features/home` is likewise named for what the user sees, not for the tables it writes.

## Library rail

**The library is exactly this profile's own catalogs and collections — `useLibrary`
(`web/src/features/library/useLibrary.ts`) reads only `GET /api/p/{i}/library`, one query
(`queryKeys.library`) that also carries what waits for a push.** There is no merge and no `owned` field on `LibraryCatalog`/
`LibraryCollection`: under the closed-graph sharing model, a folder can
only ever reference your own listed catalogs, so "the library" and "what you own" are one set by
construction, not two sets reconciled in the frontend. Browsing and adding what someone else
publishes is the separate Community tab's job, not this rail's (see "Community tab" below). A row
added from Community is a library row like any other, marked by its stickers (see "Sharing"
below).

The library's list endpoints are unpaginated; assume small N. Filter and search are **pure client-side
derivations** of `useLibrary`'s dataset, so typing in the rail's search box costs no network call.

Two things established here that everything downstream depends on: **genre lookups are kept
per-kind, never merged** (the movie and tv genre id spaces are separate), and **genre queries are
excluded from the rail's loading/error state** — a failed genre fetch degrades the summary line
to raw ids rather than failing the list, so the rail never blocks on TMDB being reachable.

### Import and export

**Import JSON** and **Export JSON** are the two items of the ⋯ `MoreMenu` ("More for your
library") beside the rail's search field (`LibrarySection`'s `onImport`/`onExport`). `Workspace` owns both dialogs' open state, as it does
the naming dialogs and the confirms. Neither dialog goes through `EditorGuard`: both open over the
pane without replacing what it holds. Both are `Modal`s, so below `lg` they fill the viewport width
less its padding. The dialogs live in `web/src/features/bundle/`, and the three calls in
`web/src/api/bundle.ts`. The bundle stays `unknown` in TypeScript because the format is defined
by the Go types alone (`docs/data-model.md`, "Bundle format"). Only the `/import/check` answer
(`ImportCheck`, `ImportMatch`) and the import answer (`ImportResult`) are typed.

- **`ExportDialog`** has two checkbox groups, Catalogs and Collections, filled from `useLibrary`'s
  lists, each with **All** and **None**. The row open in the pane starts ticked. A note says
  collections include the catalogs they use, and the server exports those whether or not they are
  ticked. The footer is Cancel and **Copy JSON** (the one primary);
  it is disabled while nothing is ticked or a request is in flight. There is no file download:
  the export is text only.
  - **Copy JSON** calls `exportBundle`, writes the response to the clipboard and leaves the dialog
    open, the button reading "Copied" for 1.6s. `copyText` (`text.ts`) hands the clipboard the
    request's own promise as a `ClipboardItem` where the browser has one, so Safari keeps the
    click's gesture across the fetch. It never throws: with no clipboard or a refused write, the
    dialog shows "Couldn't copy. Copy the JSON below." over a read-only textarea holding the text,
    focused and selected. Changing the ticks clears the textarea.
  - The text is `bundleText` (`JSON.stringify(value, null, 2)`).
- **`ImportDialog`** is one screen, 760px wide (`Modal`'s `width`), with one input, an **Import**
  button in the footer, and the state in `useImportFlow`. There is no file picker: the bundle comes
  in as pasted JSON only.
  - **The input.** A `JsonField` shows the text with a line number beside each line. It grows with
    its text from 8 lines up to 60% of the viewport height, then scrolls; lines don't wrap, so a
    number always sits on its line, and the gutter scrolls with the text. The gutter holds every
    number. At a 4 MiB paste a keystroke takes over a second, almost all of it in the browser's
    own textarea; the gutter's share is a few tenths of a second of the paste itself. It ends
    in `parseBundleText` (`text.ts`); text over 4 MiB is refused before it is sent
    (`MAX_BUNDLE_BYTES`, the twin of the server's `maxBundleBodyBytes`). A paste whose length
    alone is past the limit (`surelyOversize`) never enters the field: it keeps what it held and
    shows the same refusal (`OVERSIZE`). Text under that length but over 4 MiB in bytes is
    caught by `parseBundleText` on the next press.
  - **Validate JSON** (the field's button, disabled while it is blank) is `parseBundleText` alone:
    instant, no request. The result shows under the field, "Valid JSON." or "This isn't valid
    JSON." over the parser's own words. The field stays editable, and any edit clears the
    result. Validate says nothing about whether the JSON is a bundle: that is checked by the
    server, when Import is pressed.
  - **Import** is disabled until there is something to import and while a request is in flight,
    since a second import writes a second set. It parses the source again and sends
    `/import/check`; nothing is written until the next press, so every import is two presses and
    what a bundle holds is always seen before it lands.
  - **The review** (`ImportReview.tsx`) takes the field's place, so nothing it asks is below the
    fold, and focus moves to its summary. "This JSON holds 3 catalogs, 1 collection and 2
    folders." sits beside **Edit JSON**, which brings the field back with its text and focus and
    drops the review; under it, when something matches, the question, which says "filters", the
    word the rest of the UI uses for a recipe. A match is a bundle collection with the title of
    one the library already has, trimmed and in any case, or a bundle catalog with the same
    recipe as one; catalog names are not compared.
    - **Everything the bundle holds** is listed from the check, as the Library lists
      it: under "Catalogs · N", each top-level catalog's name, its filters line (`recipeLine`,
      genre names from the Library's lookups, which `Workspace` passes in) and its Movies or
      Series sticker; under "Collections · N", each collection's title, its folders
      (`describeFolders`) and the Collection sticker. The dialog never reads the bundle's own
      format for these.
    - **The footer button** names what its press adds with the choices as they stand,
      `importTally` (`reuse.ts`): every catalog still in the import (`liveCatalogs`: the top-level
      ones and those of each collection not skipped) less the reused ones, and every collection
      not skipped ("Import 2 catalogs and 1
      collection"). With nothing left to add it reads **Nothing to import** and is disabled.
    - **A matched collection's row** carries **Import it** (the default) or **Skip, I already
      have it**. Skipping one leaves it out with its own catalogs, says so under the row, and
      drops those catalogs' choices (`liveMatches`); the request's `skip_collections` carries the
      skipped positions (`withSkip`). Import all and Use mine for all never touch a collection,
      and show only while a catalog choice does.
    - **A matched catalog's row** carries its choice; a collection's own matched catalogs are
      listed under the collection, and its unmatched ones only through its folders. Each choice is
      **Import it** (the default) or the reuse choice; nothing here says "copy", the word
      Duplicate keeps. A
      top-level catalog reads **Skip, I already have it**. A collection's own catalog reads **Use
      my existing one**, with a hint that the collection will then share the library catalog.
      Several existing matches get a `Select`, which starts on the first by name. **Import all**
      and **Use mine for all** set every row; the second picks each row's first match. The choices
      are `reuse.ts`'s `ReuseChoices`, and `reuseMap` turns them into the request's `reuse` map.
      Editing the text drops the review, so a choice never goes with a bundle it wasn't made for.
  - **Errors.** Each is a plain headline with the technical text in dimmer beneath
    (`problem.ts`'s `ImportProblem`). A failed `/import/check` reads by status (`checkProblem`):
    a 400 "This JSON isn't a bundle Uno can import.", a 502 "TMDB couldn't be reached to check
    the filters. Try again in a moment.", anything else "Couldn't check this JSON. Try again.",
    each over the server's own words, so a mistyped key still comes back named
    (`invalid request body: unknown field "tile_shap"`). A failed write reads "Couldn't import.
    Nothing was added." (`writeProblem`), true because the import is one transaction, and keeps
    the review and its choices. Every error line takes focus when it appears, which scrolls it
    into view and has it read out.
  - **After a write.** `useImport` invalidates the library and settles only once it
    has refetched. The dialog then closes, and a toast under the rail's search row names what the
    rail gained, for example "Imported 2 catalogs and 1 collection". It counts only new listed
    catalogs and new collections, which is what the import response lists. An import opens
    nothing and adds nothing to home.

The owned-list keys prefix the selection keys, so an import marks those stale too. An import
doesn't change them, so the refetch returns what they already held. The Community keys sit beside
the owned-list keys rather than under them (`['p', i, 'community', …]`), and an import leaves
them alone: nothing it writes is shared.

The toast is `components/Toast.tsx` with `components/useToast.ts`, the same auto-dismissing
message the Community tab shows its outcomes in.

## Catalog authoring

Create is a two-field form — name and `type` — plus the TMDB params sub-form.
`provider` is derived (`"tmdb"`) and never rendered. `type` shows only as the kind sticker
(Movies, Series) on the editor's sign, never as a setting — there is no path, here or anywhere
else, that changes an existing
row's type — and that's backed server-side too: `UpdateUserCatalog` reads the stored `type` and
rejects a `PUT` that changes it with `ErrInvalidInput` — a catalog's type is part of the pushed
collections blob, so changing it would alter what Nuvio should have with no save of any
collection. `provider` is enforced server-side the same way, in both validation
places.

**Duplicate is a first-class action on your own rows, not a hidden overflow item, and it's
atomic** — every row in the library is yours, so opening one always edits it; the Duplicate button
(`LibraryItem`'s row actions, or the editor header below `lg`) is the only way to reach it.
`Workspace.tsx`'s `confirmDuplicateCatalog` calls `POST .../catalogs/{id}/duplicate` the instant
the confirm dialog is accepted — no form to fill in first, unlike a bare "New catalog". The server
writes a straight copy of the source's exact type and params, named "<name> (copy)" and cut short
to fit 200 characters, with no TMDB call. The SPA then opens the finished copy in this same editor
like any other real row — the same atomic-then-open shape `confirmDuplicateCollection` uses for
collections. A duplicate is never published, whatever its source, because publishing is a
deliberate act rather than something inherited from what was duplicated. Nothing changes a
catalog's type, Duplicate included: it copies the source's type, and offering another would need
a step of its own before the create fires. (Adding what someone else publishes is a different action,
`POST /api/p/{i}/community/{id}/subscribe`, whose UI is the Community tab, below.)

**Delete's confirm says only what applies** (`Workspace.tsx`, `DeleteMessage.tsx`). The pure helpers
in `features/builder/deleteConsequences.ts` decide the lines: "X is deleted permanently.", then
"Also removes it from: Community · 2 collections · Nuvio (next push)" when anything applies,
then "People who added it keep it." when the row is published, then "This can't be undone."
Community appears while the row is published. A catalog's collections are counted from the
library's collections whose folders reference it. Nuvio appears when the row holds a stored
`home_position` (for a catalog, also when a collection using it does), because Nuvio keeps what it
was last pushed until the next push. A bare row gets no "Also removes it from" line.

**Delete is never disabled for a row on Home.** The server allows it any time
(`docs/architecture.md`, *Deletes are allowed any time*): Nuvio keeps the row until the next
push, and the Home pane's list says so (*Home pane*, below).

**Validation rules are enforced structurally where possible**, and this order of preference is
the point:

1. **Structurally.** The date window is one three-way mode (`DateMode`: `any` / `fixed` /
   `rolling`), shown as one `Segmented` of four in `DateWindow.tsx` (Any time / Recent / Upcoming / Dates — Upcoming
   is the one-day rolling window, `UPCOMING_DAYS`), so
   "both fixed and rolling set" — the thing `Validate()` rejects — is *unrepresentable* rather
   than merely caught. Recent opens on `RECENT_DAYS` (90) unless a window is already set, so
   choosing it raises no error; its chips are `DATE_PRESETS` (30 days to 10 years), and a stored
   window none of them is keeps a chip of its own ("3 years", "45 days"). `applyDateMode` (behind `toPayload`) also strips the other type's date
   fields entirely, so a series catalog can never ship `primary_release_date_*` or
   `released_within_days`. Certification is shared, not stripped — movies use theatrical
   ratings, series use TV content ratings, both scoped by `certification_country`.
2. **Grouped controls** for the two required-together pairs: picking an age rating defaults its
   country, picking a streaming service defaults the region, and clearing one clears the other.
3. **Checks for the rules a form can reach** (`web/src/features/catalogs/catalogForm.ts`: name
   length, rating 0–10, no negative counts or runtimes, inverted ranges, fixed dates as real
   dates in order) as the per-field backstop.
4. **The server's 400** as an unexpected-case banner only.

Other decisions worth keeping:

- **Genre picker is a chip grid, not a combobox** — TMDB's per-type genre list is short enough
  to fit on screen at one click each, and the AND/OR join lives inside the same control because
  ids and join are one value on the wire (`28,878` vs `28|878`).
- **There is no control for the genre filter clients show in Discover.** The manifest derives
  it server-side from TMDB's genre list and the recipe's own genre filters. See the addon-server
  section of `docs/architecture.md`.
- **Watch providers are a named, region-scoped picker** (`WatchProviderPicker`) over
  `GET /api/watch-providers/{type}`, not a box for TMDB's numeric ids. Region and services are
  one grouped control because they are required together on the wire: `watch_region` is written
  only alongside a non-empty id list and cleared with the last chip. A selected service the
  newly chosen region doesn't list renders as a `Service {id}` chip rather than vanishing while
  still in the payload. Services are stored pipe-joined (`8|337`), which TMDB reads as "on any of
  these"; a comma would mean "on every one of these at once", which is almost never what picking
  several services means.
- **Production companies and keywords are one server-search picker** (`TMDBEntityPicker`,
  `kind="company"` / `kind="keyword"`), the only picker in the editor that searches the server
  rather than filtering a list it already holds: TMDB has no "list them all" endpoint for either.
  The query settles for 300ms (`lib/useDebounce.ts`) and runs against
  `GET /api/{companies,keywords}/search` only once it is two or more characters, since the route
  rejects a blank one. Picks are chips; ids and join are one value on the wire, like genres
  (`420,2` all, `420|2` any, through `parseIdList`/`serializeIdList` in
  `features/catalogs/params.ts`, the one reader of every stored id list), and the all/any
  `Segmented` appears once two are picked. Companies default to "any" — a title rarely has two
  named studios. Each holds at most 20 ids (`MAX_ENTITY_IDS`, mirroring the server's cap): at 20
  the picker stops offering results and its status line says to remove one first, and
  `validateForm` flags a stored value over the cap. Company search takes the catalog's type
  (`?type=movie|series`, part of the query key) and the server returns at most 10 companies with
  5 or more titles of that type, most titles first; each row reads "A24 · US · 176 films" (or
  "series"), the country omitted when TMDB has none. The per-kind table's optional `rowDetail`
  carries that suffix (`companyDetail` in `summary.ts`); keyword and collection rows are the
  name alone. A saved recipe's chips are named through `GET /api/{companies,keywords}/{id}`
  (`staleTime: Infinity`, seeded on pick so a fresh chip needs no lookup); an id TMDB answers 404
  for stays a chip marked "not found", so a stale recipe can be cleaned up rather than silently
  keeping a filter nobody can see. Results are focusable buttons (Down from the search box, then
  Up/Down; Escape clears the search without closing the editor), and the searching / no-match /
  count line is a `role="status"` live region. Section heads and the library summary count these
  rather than name them, for the same reason as streaming services.
- **Each of those sections holds two pickers, Include and Leave out** (`EntityLists` in
  `CatalogSettings.tsx`), writing `with_*` and `without_*`. The Leave out picker is the same
  component with `exclude`: no all/any `Segmented`, and always comma-joined, because TMDB drops a
  title carrying any of the listed ids whichever separator is used. Each list has its own 20-id
  cap. Each picker takes the other's ids as `hiddenIds` and never offers them in search, so one
  id can't be both included and left out. There is no server rule for that overlap either, the
  same as genres. The closed head reads "1 production company · not 2 production companies" (`sumEntities`), and the
  library summary reads "not from 2 production companies" / "not tagged with 1 keyword".
- **A movie catalog shows either Filters or a TMDB collection**, a `Segmented` that is the first
  control under the "What the row shows" heading. On screen a TMDB collection is always called
  "TMDB collection", never bare "collection", since "collection" is Uno's word for a group of
  catalogs. The mode is form state (`sourceMode` in `catalogForm.ts`, `'filters' | 'collection'`),
  not a stored field: `formFromCatalog` reads a saved `with_collection` as TMDB collection,
  anything else as Filters. Filters shows every filter section and no TMDB collection section;
  TMDB collection shows only that section, its picker with Shuffle under it, every other section
  — sort order included — hidden rather than disabled. A TMDB collection row lists that
  collection's films in release order, and the server rejects any other filter beside
  `with_collection` (`randomized` excepted), so what a save or Preview sends follows the mode
  (both go through `paramsString` → `recipeParams`): TMDB collection keeps only the
  `COLLECTION_KEYS` allow-list, so a field added later is dropped by default, and Filters drops
  `with_collection`. Form state keeps both sides' values, so switching mode back and forth loses
  nothing until Save. `validateForm` checks the sent params, so a dropped field raises no error,
  and TMDB collection mode with nothing picked is an error ("Pick a TMDB collection."), which
  also stops Preview from running. Series catalogs have no switch and no TMDB collection section,
  since TMDB has no collections for series.
- **Shuffle is the last control of the Order section** (of the TMDB collection section in that
  mode), with "New set each time" (or "New order each time" for a TMDB collection, which always
  holds the same films) beside it once on, and the section's summary ends "· shuffled"
  (`withShuffle`).
- **The results panel names what to fix.** While the recipe has errors, the line standing in for
  results reads "Fix Order first." (or "Fix Order and Production companies first."), the sections named
  through `roleLabelFor` (`recipeSections` in `CatalogEditor.tsx`); `name` is left out, being no
  part of a recipe.
- **The collection picker is the same server-search picker, single-pick** (`kind="collection"`,
  over `GET /api/collections/{search,{id}}`, stored as `with_collection`). TMDB takes one
  collection id, so the kind's table entry marks it `single`: a pick replaces the chip rather
  than adding one, and there is no all/any toggle. Its section head names the pick rather than
  counting it: the editor reads the same `staleTime: Infinity` by-id key the chip does, so the
  name costs no extra request. "Collection" is also Uno's word for a group of catalogs, so the
  TMDB kind is `TMDBCollection` in code, the picker says "TMDB collection", and the library
  summary describes a TMDB collection row as "from a TMDB collection" (plus "shuffled"), with none
  of the other filters.
- **A series catalog's Networks section is the same server-search picker again**
  (`kind="network"`, over `GET /api/networks/{search,{id}}`, stored as `with_networks`). It is
  series only, since `/discover/movie` has no network filter. A movie catalog has no Networks
  section, and `applyDateMode` drops `with_networks` from anything a movie catalog sends. The
  section holds one picker, with no Leave out list, because TMDB ignores `without_networks`.
  Networks default to "any", like companies, because a show rarely airs on two networks. Rows
  read "HBO · US · 376 series" through the same `companyDetail`. The search takes no type, and
  the server always counts series. The cap is 20, the same as the other id lists. The closed
  head reads "2 networks, any of them" (`sumEntities` with no left-out list), and the library
  summary reads "on 2 networks".
- **Age rating is the same shape**, over `GET /api/certifications/{type}` — options come from
  TMDB per type because the scales differ, and picking a rating defaults its country.
- **The editor previews on request, not as you type** (`RecipePreview` → `useRecipeTiles` →
  `POST /api/catalogs/preview`). The recipe changes on every keystroke, so fetching live would
  mean a request per character and tiles that never settle long enough to read; an explicit Run
  button is one call per deliberate act. The block is one TMDB page — the same page the row
  itself is, not a sample of it — except for a shuffling recipe, which is labelled as such.
  Every tile links to its TMDB page, which is where "what *is* that one?" gets answered.
- **Deleting a catalog also drops it from the pending home screen** — a catalog added there since
  the last push can still be deleted, and leaving its reference behind would only fail at Push.
  The confirm says what goes with it, "Any references to this catalog from a collection will also
  be removed", not "are you sure".
- **Creating a catalog does not select it.** Auto-adding would silently increment the
  unpushed-changes count as a side effect of a save that already succeeded, blurring the two
  persistence models the page has to keep legible: authoring writes immediately, selection is
  pending until Push. Adding from the rail defaults `show_in_home: true`.
- **Writes invalidate the owned catalog list** (`useCatalogMutations` also invalidates every
  Community key, through `invalidateProfileLists` — a save can make a published row changed since
  publishing, and a delete unpublishes it). The Home pane reads its rows from these lists;
  `HomeSelectionContext`'s one-shot hydration keeps a refetch from clobbering pending edits.
- **A collection save never invalidates the catalog list** (`useCollectionMutations`): catalog
  edits inside a collection change scoped rows, which the list never holds.
- **`useCatalogMutations` also invalidates the collection lists.** Not defensive — required.
  `DELETE FROM catalogs` cascades `folder_catalogs`, so a cached collection tree keeps a phantom
  ref: the overlay lists a folder member that no longer exists, and saving that collection
  `400`s.
- **A scoped catalog has no publish button**: it is published only by publishing its
  collection, so its nested editor gets no `sharingStep`. A catalog is scoped only inside
  `CollectionEditor`, where New or a folder row's Unlink from library makes one, and its scope
  never changes: a library copy of it is a new row (its folder row's Copy into library), and
  nothing moves a listed catalog into a collection.
  `collectionID` is form state only: `toPayload` leaves it out, since a catalog's `PUT` never
  changes its scope, and `isSameCatalog` compares the payload alone. The nested editor's Save
  reads Done, since it only stages the edit.
- **A save carries the revision the editor opened at.** `formFromCatalog` copies the row's
  `revision` into the form, so `target.initial` holds it from the moment the editor opens, and
  `Workspace` sends it with the payload (`CatalogSave`); a create sends none. It is never read
  from the live library row a refetch moves: a save from a catalog another tab saved since is
  refused with a `409` "Error saving.", which the editor shows where it shows any save error.
  Nothing refetches or touches the cache, and nothing words it as a change elsewhere; the user
  refreshes the page. Reopening a row right after this tab's own save, before the library has
  refetched, seeds the old revision and is refused too, correctly: the form then holds the
  pre-save content.

## Home pane — List view

Add/remove from the rail, drag/keyboard/↑↓-reorder, `show_in_home` per catalog row. Hydrates once
from the library, the rows with a `home_position` in its order, once it has loaded: the library
is one read, so Home never hydrates from one list without the other, and Push replaces what Nuvio
holds.
Client state only, nothing writes until Push.

- **The baseline is snapshotted at hydration, not read live from the query cache.** A background
  refetch must not move the baseline under the user and silently change the diff.
- **Home's revision is its own state beside the baseline** (`homeRevision`). Every edit rebuilds
  `HomeState` as `{rows}`, so the revision isn't part of it: hydration sets it from the
  library's `home_revision`, and only `markPushed(sent, newRevision)` moves it after, or the
  tab's own next push would be refused. A refetch never does.
- **Every Home row is a library row.** The rows come from the library, and a row the library
  drops is pruned from the pending state and the baseline at once (`withoutDeleted`), so no row
  is ever drawn without its library row.
- **A row is its name, its kind, To push while one waits, and a detail line.** A catalog's detail
  is its recipe line. A collection's is its folders (`FolderChips`): up to six small tiles, each
  its cover image or emoji and its title (a folder with neither is its title alone), then "+N more"; a collection with no folders, or one
  nothing describes, shows `describeCollection`'s line instead.
- **List groups in the same two bands Preview draws** (`preview.ts`'s `buildHomePreview`, not
  a separate derivation), under the headings Pinned and Rows: pinned collections, then the home
  rows, catalogs and unpinned collections mixed in one order (`HomeState.rows`, one ordered list of
  `HomeEntry`, hydrated by each selection row's `home_position`), numbered with one ordinal straight
  through both — a row's number is its place in Nuvio. A reorder (drag, keyboard, or a row's own
  ↑/↓) only ever moves a row within its own band; `HomeSelectionContext`'s `reorderBand`/`moveRow`
  reconstruct the *complete* underlying list on every edit (`pending.ts`'s `reorderWithinBand` /
  `moveWithinBand`) rather than replacing just the touched band, so the untouched band can't
  silently relocate to the array's tail and register as a phantom pending change.
- **`show_in_home` toggles whether the catalog gets a home row** — off keeps it in Discover only,
  via a required `genre` extra `buildManifest` adds to that catalog's manifest entry. Off-catalogs
  render outside the numbered bands entirely, in their own "Only in Discover" tray (no drag, no
  ordinal — they have no place in Nuvio's order); the flip itself is a row's ⋯ menu
  ("Move to Discover" / the tray's "Move to home"), not a dedicated toggle control.
- **Pin (`pin_to_top`) is a pending edit here, like Home or Discover**, and nowhere else:
  a collection row's ⋯ menu offers "Pin" / "Unpin" (`showFirstAction`), which
  flips the entry's `pinToTop` in `HomeState.rows` and moves the row to the other band at its
  place in Home order. The baseline takes each collection's stored pin, the one it was last pushed
  with; a collection added to the home screen starts from its stored pin too. Push sends the pins
  in its selection and is the only thing that writes them — the collection editor has no Pin.
  The band-aware edits (`reorderBand`, `moveInBand`, `togglePinToTop` in `pending.ts`) read each
  row's band from the state they edit.
- **The pending count is the list of changes' length, not a separate tally.** `changes.ts`'s
  `computeHomeChanges` diffs `baseline` against `current` into named, per-row sentences ("Moved
  “X” from 5th to 3rd"), using a longest-increasing-subsequence pass per band so a drag reports
  only the row that actually moved. Each state carries its own pins, so a collection whose pin
  changed is reported once, as "Pinned “X”" or "Unpinned “X”"
  (`pinFlips`), and left out of the moves: it changed band, which a per-band pass would
  otherwise read as moving it and every row it passed. `HomeSelectionContext.pendingCount` is `changes.length`; the
  header's pending indicator and the navigation guard's dialog both read it, and the indicator
  doubles as the toggle that opens the list itself (`ChangesStrip` in `PushControls.tsx`) — the
  count and the sentences behind it must never disagree, which is why there is only one number.
- **What the server says waits for a push is a source of lines too**, with no selection edit at
  all: `computeHomeChanges` takes the `pending` list `GET .../library` answers
  (`useLibrary`'s `pending`, `docs/architecture.md`, *HTTP surface*) and adds
  a saved line for each row in it — a catalog or collection edited since its last push, one on
  Home that Nuvio holds nothing for ("isn’t in Nuvio yet"), or one **deleted** since ("Removed …
  from home screen", by the name Nuvio still holds it under). A catalog's name or recipe edit is
  one, and a collection using an edited catalog is listed as changed, since the addon serves a
  catalog from what the last push left and a collection's save changes what Nuvio shows only at
  the next push. A rename and back, which the server finds no difference in, adds no line. A
  changed or added row counts only while it is in *both* `baseline` and `current`: one taken off
  the home screen in this tab is the "Removed …" edit instead. A removal says once what the tab
  already said (the lines share a key). A row deleted here or in another tab leaves `baseline`
  and `current` both once the library has refetched without it (`withoutDeleted`), so Push is
  never refused for naming it.

**Selection is client state until Push, and the one thing enforcing that is the one-shot
hydration guard** in `web/src/features/home/HomeSelectionContext.tsx`
(`if (current === null && library.loaded)`). The mutation hooks invalidate the library
the Home hydrates from, so they refetch on every catalog or collection write; the guard keeps that
from moving `baseline` or `current`. Removing the guard, or making hydration re-run
on fresh data, silently clobbers the user's pending home-screen edits on the next catalog or
collection write.

**The provider publishes two contexts.** `useHomeSelection` returns everything, and its value
changes on every edit; `useHomeEdits` returns only the edit functions, which change only with
the collections an added one reads its stored pin from (`storedPin`). A component that only
makes edits (`PinItem` in `HomePane.tsx`) reads `useHomeEdits` and doesn't re-render on every
change. `Workspace` reads neither hook, so an edit to the home screen re-renders the
components that show the selection — the rail, the Home pane, the header, a collection editor's
"used in N places" rows — and not the workspace and the open editor under it.

**`HomeSelectionContext`'s `genres` passthrough is load-bearing, not a redundant re-export.**
The provider builds the genre lookups once and every Home row reads that one value; a row
calling `useGenreLookups` itself would add a query observer and rebuild the lookup per row.

`useLibrary` runs at two simultaneously-live call sites —
`web/src/features/home/HomeSelectionContext.tsx` and
`web/src/features/builder/Workspace.tsx`. React Query dedupes the fetches, so the cost is two
copies of the derived arrays and the observer subscriptions behind them, not network traffic.
The defensible refactor is a separate `LibraryProvider` wrapping `HomeSelectionProvider` — *not*
folding the library into `HomeSelection`, which would conflate "what is on your home screen"
with "what exists to choose from". Declined while `Builder` is the only surface: it costs a new
context and couples `useLibrary` to a provider. Revisit when a second page needs the library.

**Pending selection is lost on a hard reload, and that is accepted.** There is no save button
for the Home pane; selection lives in browser memory until Push. A `beforeunload` guard
(`useUnloadGuard`) and a navigation confirm both fire with a count of those unsaved edits, which is the
mitigation. Mirroring pending selection into `localStorage` is cheap but adds a "your local
state disagrees with the server" case to handle on next load; not planned.

## Home pane — Preview view

A pure render of state List already holds — no endpoint and no fetch for *layout*; real tiles
come from `POST /api/catalogs/preview`. The derivation lives in
`web/src/features/home/preview.ts`, kept pure and separate from `HomePreview.tsx`.

**The model being previewed.** *Home* is **one page** in two bands:

```
pinned collection rows      Pin (the pending pin) hoists above everything
home rows                   catalogs (tiles = content) and collections (tiles = folders), mixed
```

The bands follow the pending pins in `HomeState.rows`, not the pins last pushed, so a
pin flipped in the List view moves the row in Preview at once.

A collection is **one row whose tiles are its folders**, drawn from folder metadata
(`cover_emoji`, `cover_image_url`, `title`) at each folder's own `tile_shape`. No TMDB content is
rendered for a collection on home. Clicking a folder tile opens a **folder page** scoped to
*that one folder*; sibling folders are not on it. `view_mode` is a collection-level setting
applied to every folder in it, and it governs this page only:

| `view_mode` | Folder page |
| --- | --- |
| `TABBED_GRID` | One tab per **catalog in the folder** (plus "All" when `show_all_tab`), over a grid of that catalog's content |
| `ROWS` | One row per catalog in the folder, stacked — the same shape home uses |

Corroborating details from Nuvio's own field descriptions: `hideTitle` is "Hide the **tile**
title text" (singular tile — it hides the title under the folder's own tile on home). Nuvio's
per-folder focus GIF (`focusGifUrl`/`focusGifEnabled`, seen in pulled collections and in NuvioTV's
own source rather than the public doc) plays over the folder's tile while it's focused, which
only makes sense on a focusable tile. `pin_to_top` is "pin to top of **home screen**", which is why it partitions home
into bands rather than merely sorting collections to the front.

Decisions that shape the code:

- **Read-only, with exactly one interaction.** No drag, no remove, no toggles — editing lives
  entirely in List. An affordance that looks live and isn't reads as a bug. Opening a folder is
  navigation *within the mock*, not an edit. The open page is held as
  `{collectionId, folderId}`, **never indices**, so a folder deleted in List collapses back to
  home instead of silently rendering a different folder that happens to occupy the same slot.
- **Discover-only rows render as a dimmed, labelled group**, separate from the home rows above —
  a genuine omission from home, since `buildManifest` marks their genre filter `isRequired`.
- **A folder's `refs` are *sources*, never tiles.** Each is a standing query contributing
  an unknown number of items, so no view draws one tile per ref — that would misstate how much
  the folder holds. They are the folder page's spine: one row or one tab each.
- **A row nothing resolves degrades to an empty strip**, no explanation, matching how Nuvio
  would show it. `catalogById`/`collectionById` hold the library's rows and the scoped catalogs
  its collections' folders use.
- **The wire's view mode and tile shape are drawn as they are.** The server requires both
  (`view_mode` `TABBED_GRID` or `ROWS`, `tile_shape` `POSTER`, `LANDSCAPE` or `SQUARE`; see
  `docs/data-model.md`), so `ViewMode` and `TileShape` in `api/types.ts` are those unions and
  nothing reads an empty or unknown value.
- **Selection order is preserved within each band**, so pinning moves a row between bands
  without discarding the order the user just dragged.
- **`ListState` owns loading and error for both views; each view owns its own empty case.**
  List's empty is an instruction to go add something; Preview's is the colour-bars moment from
  the design section below. The error is only one that leaves nothing to draw — a library
  that never loaded, so Home never hydrated — and carries a Retry. The rail reports the same
  failure itself: one error and one Retry, leaving out both groups. A background refetch that
  fails after the page has loaded keeps what it had rather than replacing the pane, since the only
  other way out of a replaced pane is a reload, which discards pending edits.

**Real tiles.** `web/src/features/home/useCatalogTiles.ts` issues **one query per catalog, never
a batch** for the catalog rows; a folder page goes straight to `useRecipesTiles` with
`folderRecipes(folder)`, because a folder reference's genre isn't something a catalog id can
look up. A batched endpoint carrying N recipes would flatten round trips but needs a per-item
error shape, and one slow TMDB call would hold up every row; per-catalog queries fail, retry,
and cache independently, and same-origin HTTP/2 multiplexes them anyway. Placeholder tiles are
the loading state (count 20, so the row doesn't reflow when content arrives); if TMDB is
unreachable, preview degrades to exactly the layout-only behaviour it has without the endpoint.

**Every row fetches at once — there is no IntersectionObserver, deliberately.** Preview rows
don't scroll horizontally and there is no "load more", so one call is the complete answer for a
row rather than a first page of one; at fifteen rows, deferring them isn't worth an observer's
complexity. The catalog editor's Run button is the one place a preview fetch is gated, and it is
gated on a keypress, not on visibility.

Two fidelity limits exist and are silently absorbed rather than named in the UI, per DESIGN.md's
Clean Preview spec and the owner's instruction that Uno pin nothing between the preview rows anywhere:

- **Merged catalogs.** Nuvio merges N catalogs into one view and **its merge rule is
  unspecified** — folders aren't an addon concept, so nothing in the addon protocol or Nuvio's
  docs specifies the order, and Uno cannot derive it. This applies to **exactly one view**: the
  `show_all_tab` "All" tab on a `TABBED_GRID` folder page (`interleaveTiles` in
  `features/preview/model.ts`, which the collection editor's Preview panel draws through the
  same preview components). Nuvio's own "All" tab just shows the merged tiles, with no caption saying
  the order is a guess.
- **`randomized` catalogs** take a random TMDB page per call on both the addon path and the
  preview, independently, so the preview genuinely won't match Nuvio. A collection row shuffles
  its film list instead, with the same independent-per-call mismatch. Not flagged inline; the
  blank tile face behind a poster is the only visual difference, and it isn't specific to
  this case.

**The folder page's back arrow lives in the preview panel, beside the folder's own title**, per
DESIGN.md's One Way Back rule, not as a button in Uno's own chrome above it. Escape and the
browser's own Back do the same thing. `HomePreview.tsx`'s `useFolderPage` hook owns this: opening
a folder pushes one `unoFolder` history entry (`pushEntry`, `lib/historyEntry.ts`; a sandboxed
frame can throw, and Escape/the arrow still work without it, only the browser's own Back
doesn't); a `popstate` listener closes the folder once the current entry no longer carries the
flag, so stepping back off an editor layer's entry above it leaves the folder open. Its Escape
listens on `window`, after `EditorShell`'s on `document`, so an Escape that closed an editor's
layer over the folder page leaves the folder alone. Leaving any
other way — the arrow, Escape, the target becoming unresolvable, or this view unmounting entirely
(the List | Preview switch, or an editor taking the pane from `lg`) — consumes the pushed entry with one more
`history.back()` rather than leaving it to `popstate`, so a later physical Back press never lands
on a dead entry nobody is listening for. It does so only while that entry is still the current
one: an in-app navigation away (Switch profile) has already pushed past it, and stepping back
from there would land on it again and re-render the builder that was just left.

## Collection authoring

Collection → ordered folders → ordered catalog refs, the whole tree in one `POST`/`PUT`. That
shape is what makes a per-folder save impossible, so there is one dirty state and one Save
button.

- **One `DndContext`, several `SortableContext`s** — folders among themselves, each folder's
  refs among themselves. Nesting a second `DndContext` is the obvious shape and the wrong one:
  the outer context still sees the inner drags. Every sortable declares its list via dnd-kit's
  `data`, and `onDragEnd` reorders only when the dragged item and the drop target agree on it,
  so a ref dragged out of its folder is a no-op rather than a mis-drop.
- **`collisionDetection` filters the droppables to the dragged item's own list before ranking**
  (`withinContainer` in `web/src/features/collections/folderDnd.ts`). This is not optional. `closestCenter` ranks
  *every* droppable in the context, and the selected folder's catalogs sit right under the tile
  strip — so a folder tile dragged downward finds a **ref row** to be the nearest center;
  `onDragEnd` then correctly refuses to guess and drops the folder back where it started. The
  dispatch check is doing its job; the candidate set is what has to be right. Worth remembering
  as a class: **a mis-drop guard turns a wrong reorder into a dead drag, which is not the same
  as making the drag work.** Fingerprint of this bug: `sortableKeyboardCoordinates` filters by
  `containerId` itself, so keyboard reordering works while pointer drag dies.
- **Cross-folder ref dragging is deliberately out** — it needs an insert position and a de-dup
  rule against the destination, to replace two clicks. Remove-then-add is the supported move.
- **"Not accessible" and "the save will 400" are one condition here**, which is the *opposite*
  of Preview's finding and not a contradiction. Accessible is the library plus every scoped
  catalog `CollectionEditor` already knows about (`localCatalogs`, seeded from the wire's own
  `catalogs` array and grown by every scoped create/edit made this session) — exactly
  `validateFolderRefs`'s closed-graph rule (owned, and listed or scoped to this collection). What
  makes Preview's two conditions diverge is the *selection* endpoint supplying rows the library
  doesn't have, and the builder has no selection endpoint in play. So the mirror is exact, and an
  unresolvable ref gets one marker naming the action that clears it ("Remove it to save").
- **Edit keeps unresolvable refs and blocks the save** — dropping them silently would delete rows
  the user never touched, so `validateCollectionForm` flags them instead ("Remove it to save").
- **Duplicating a collection is one atomic server call, not a client-built clone.**
  `POST /api/p/{i}/collections/{id}/duplicate`
  (`useCollectionMutations`'s `duplicate`) copies the tree server-side (`copyCollection`):
  every folder ref survives, a listed source catalog stays a reference, and each
  distinct catalog scoped to the source collection becomes a fresh scoped copy in the new one.
  A collection's scoped catalogs can't be represented on the client without fetching them, so the
  copy happens server-side, which is what makes every scoped ref survive a duplicate (the
  client's library only ever holds listed catalogs, so a client-built clone could only ever seed
  listed ones). `Workspace.tsx`'s `confirmDuplicateCollection` calls the mutation, then
  opens the finished copy straight into its own editor for review — `EditorTarget`'s collection
  variant carries an `initialCatalogs` override for this, since the fresh copy's scoped rows may
  not have reached a `library.collections` refetch yet.
- **A catalog can be in one folder more than once, never twice under the same genre.** The
  (catalog, genre) pair is `folder_catalogs`' primary key. Form refs are
  `FolderRefState {key, catalogID, genre}`, and every per-ref action, drag id and React key uses
  the session-local `key`, because the catalog id is shared by every genre of one catalog and
  changes on Unlink. **A catalog's refs always sit side by side.** `folderFromWire` gathers them
  where the catalog first appears (`groupedByCatalog`), so a folder saved interleaved is stored
  grouped by the next Save, and every edit keeps them so (`folderEdits.ts`: `withCatalogOrder`,
  `withGenreOrder`, `withGenreAdded` after the catalog's last). The editor draws one line per
  catalog (`refGroups`) and each ref as a genre chip under it. The picker adds an unfiltered ref
  only to a catalog the folder doesn't hold (`addRef` guards too), the genre dropdown adds only a
  genre not already ticked, and the validator's "same catalog with the same genre twice"
  backstops both, as `CollectionForm.Validate` does. The same validator holds the title and
  folder-title lengths (200 characters), the 10-folder and 20-ref caps, and the media addresses
  (http or https, 2048 characters), the last shown under the folder's or the collection's
  Appearance, which opens to show it.
- **Removing a folder is a standing warning, not a confirm.** Omitting a folder from the payload
  deletes it server-side and cascades its refs — but nothing commits until Save, so
  `removedFolders(initial, current)` names exactly which folders the next save would destroy,
  shown above the folder list as soon as any exist. A confirm would ask the user to approve
  something that hasn't happened. Dropping a folder that was never saved is correctly silent.
  "Undo" reinserts the removed rows verbatim — they still carry their original form
  `key`, which is what makes putting them straight back into `state.folders` safe. Its body text
  is unconditional ("People who added it keep theirs"): a row someone added changes only when
  they apply an Update of a publication, so removing a folder from your own collection never
  reaches it. `Workspace.tsx`'s delete confirms state the same fact.
- **The save bar reads Close and Save in every editor** (`EditorFooter`), the nested catalog
  editor's Save reading Done. Close routes through the editor's own `onRequestClose`, and from
  there through the one shared `EditorGuard` confirm every exit from a dirty editor goes through
  (×, Escape, Close, the browser's Back below `lg`, and from `lg` pressing the open library row) — there
  is no second, folder-aware confirm layered on top of it. The nested catalog editor
  (`NestedCatalogEditor.tsx`) has its own `EditorGuardProvider`, since its
  unsaved edits aren't the pane's: its ×, Close, Escape and scrim pass that guard, and
  `DiscardPrompt` (`EditorGuard.tsx`) asks the same "Discard unsaved changes?" with Keep editing
  and Discard.
- **`view_mode` and `tile_shape` are the server's enums, with no "unset" option.** The server
  refuses an empty or unknown value (`docs/data-model.md`), so a new collection starts as Tabbed
  Grids (`emptyCollectionForm`), a new folder as Poster (`newFolder`), and a saved row loads as
  it is. How folders open offers Tabbed Grids and Rows (`VIEW_MODES`).
- **A collection holds at most 10 folders and a folder at most 20 refs** (`MAX_FOLDERS`,
  `MAX_REFS_PER_FOLDER` in `collectionForm.ts`, the server's caps). "Add folder" is off at 10,
  with a line under the Folders heading saying why (`FolderCount`, `FolderCap.tsx`). A folder
  holding 20 refs, a catalog split by genre counting once a genre, is full: the Add catalogs
  dropdown turns off New catalog and every unticked catalog and says so (`RefCapNote`), while
  unticking still works. A collection loaded past either cap, stored before the caps existed,
  opens as it is, and `validateCollectionForm` names it (`folderCount`, and the folder's
  `catalogIDs`) so Save is refused until it is trimmed.
- **`cover_emoji` is a short text input** (`maxLength` 8, since an emoji can be several
  codepoints), not a picker — a bundled emoji picker is a large dependency for a field every
  keyboard already has an input method for.
- **Ordering is array position.** No `sort_order` field in the form; drag order *is* the value.
  Folders are reordered by dragging a tile's corner grip (`rectSortingStrategy`, since the strip
  wraps) or with the selected folder's ←/→ (`RowIconButton` from `components/dnd.tsx` and
  `moveByOne` from `lib/order.ts` — the same shared pieces the Home pane's rows use). A folder's own catalogs are running-order
  rows (`.run-row`) that number in plain figures ("1", "2") and carry only the grip inline — see
  the "⋯" note below.
- **Folders are a tile strip, the way Nuvio draws them.** Under the "Folders" `.setting.is-head`
  heading
  (holding "Add folder"), `FolderTiles` draws each folder at its own `tile_shape` with its cover,
  name and catalog count — the editor's list and the Preview panel's row are the same picture. One
  folder is always selected (the one picked, else the first), and `FolderDetail` shows it below
  the strip as one raised panel: a heading reading the folder's title, or "Untitled folder" while it has none (the
  appearance summary is the folded row's)
  and Remove; the selected tile carries ← and → under it (`FolderMoveArrows`, `onMove`), which
  move the folder one place; then its title, then its catalogs (a `.setting.is-head` heading with
  an "Add catalogs" dropdown, which holds New), then a "Folder Appearance" `.sec-head` that folds away hide-title, tile shape,
  cover, the focus GIF (an on/off above its URL) and the three Modern Home hero URLs (backdrop, video,
  title logo). Preview renders none of the focus or hero fields; they only reach Nuvio through
  push. A setting not every Nuvio app reads carries an `OnlyIn` tag beside its label: "Nuvio TV,
  Modern layout" on the hero URLs and the collection's background image, "Nuvio TV" on its focus
  glow. The background image's InfoTip says it fills the tile of a folder without a cover image in
  place of its emoji, which Preview doesn't show. The focus GIF's InfoTip says Nuvio's phone and
  desktop apps ignore its on/off and show the GIF as the tile itself unless the device's own
  setting turns it off. Catalogs come before appearance because they're what a folder is opened for. The panel is
  a shelf of its own, so the folder's settings read as inside the folder rather than as more of
  the collection's; its fields and folding sections step to `ground` and `raised-hi` so they
  don't vanish into it. A failed Save selects the first folder with errors, and every other
  folder with errors shows a danger triangle on its tile. On touch the tile grip is always
  visible (there's no hover to reveal it).
- **The catalog-ref picker is a dropdown under "Add catalogs"** (`CatalogRefPicker.tsx`, a Radix
  `Popover`), not a dialog — a scrim would hide the folder being filled. It holds a search field,
  a **+ New catalog** row, a rule, then a checkbox row (name, kind at the right) for every
  library catalog the search matches (`.checkbox`, rows 44px under `pointer: coarse`). A tick is
  the folder itself, not a pending choice: a row is ticked while the folder holds that catalog
  under any genre, ticking one appends an unfiltered ref at once, unticking removes every ref to
  it, and the dropdown stays open for the next. Nothing reaches the server
  before the collection's Save, so there is no Add, Cancel or toast; Escape or an outside click
  only closes it. New catalog opens
  the naming dialog. On touch the popover skips its own focus on the search (`onOpenAutoFocus`,
  `hasFinePointer` in `lib/pointer.ts`), so a phone opens on the list, not under its keyboard. A
  search that matches nothing turns the row into "New catalog “query”", which opens the naming
  dialog (`NewItemDialog`'s `initialValue`) with the query as the name.
- **Three sources for a folder's catalog:** a tick in the picker **links** a listed catalog — a live pointer,
  edits reach every folder that references it — and the dropdown's + New starts a catalog
  **new inside this collection**, scoped to it alone so nothing else can
  drift it: named first (the same two-step the library's own "New catalog" uses), then opened in
  the nested editor below to fill its filters. A linked row's ⋯ **Unlink from library** is the
  third source: the library catalog staged as a draft of its own with the same name, type and
  recipe (`CollectionEditor`'s `unlinkRef`), and that ref pointed at it in place, genre kept — a
  catalog only this collection has, written by its Save like a New one. A scoped row offers Copy
  into library instead; an unavailable one offers neither.
  **New-inside-this-collection is staged locally, not written until Save.** A catalog
  written on click, independent of the collection's own Save, would outlive a discarded edit:
  nothing in the discard path, or anywhere outside `UpdateUserCollection`'s own next Save, cleans
  it up. So the click stages a synthetic `Catalog` client-side, keyed by a `draft:` id sentinel
  (`CollectionEditor.tsx`'s `draftCatalog`), in the same `localCatalogs` registry a real scoped
  catalog lives in — nothing downstream of that registry needs to tell a draft apart from a real
  row to render it. `collectionForm.ts`'s `toCollectionPayload` resolves every `draft:` id into an
  inline `FolderCatalogRef.New` spec right before the collection's own Save reaches the wire,
  keyed by that draft id, so every ref to one draft (two genre rows, or two folders) resolves to
  the one catalog Save creates rather than one catalog each;
  `internal/vault/folders.go`'s `resolveFolderCatalogRef` is the one place a `New` entry is
  ever written — inside `CreateUserCollection`/`UpdateUserCollection`'s own transaction, atomic
  with the folder write that references it. So Save creates the catalog and the ref together in
  one commit, and discarding instead of saving never wrote anything in the first place, closing
  the leak structurally rather than by adding a cleanup step. The nested editor for a draft (below)
  edits this local object directly — no network call — until the collection's own Save resolves it.
  **Edits to a real scoped catalog are staged the same way.** The nested editor's Save replaces
  the catalog in `localCatalogs`, so every folder shows the change, and records a pending edit in
  the collection form itself (`CollectionFormState.catalogEdits`, via `withCatalogEdit`), which
  `toCollectionPayload` sends as `catalog_edits` with the collection's Save. Because the edit
  lives in the form, `isSameCollection` sees it: the form is dirty, and `EditorGuard` asks before
  discarding it. An edit that would leave the catalog as the editor opened it is dropped rather
  than kept, compared form-to-form (`isSameCatalog`) rather than on the stored `params` string,
  which the form re-serializes. The server refuses `PUT`/`DELETE` on a scoped catalog, so there
  is no other door; a bad recipe comes back as a 400 on the collection's Save.
- **A folder-catalog row's quiet Edit is scoped-catalog only**, DESIGN.md's own spec for the
  affordance. It opens the referenced catalog one level down: for a scoped catalog (real or a
  session's own draft) that's unambiguous, since nothing else can reference it. A *listed* catalog
  has no inline Edit here at all — it's a live pointer, so an edit made from inside a collection
  would silently reach every other folder and the library too, which reads as a surprise rather
  than a feature. The row instead says how many places it's used (home screen plus every folder
  across every owned collection — the folders from `Workspace`'s `usedInFolders`, the home screen
  read by the row itself). Editing a listed catalog directly is the library rail's job. "One level down" is a `Modal` layered over this editor, not a second pane —
  the builder's pane holds one occupant (see Library rail, above), so a second real editor has to
  be a modal rather than a stack. `CollectionEditor` stays mounted underneath it, so this editor's
  own unsaved folder edits survive the round trip; the modal resets `--app-h` to `0` locally so the
  nested `CatalogEditor`'s sticky header doesn't try to clear the outer app header's height a
  second time. It is `Modal`'s `sheet` shape: below `lg` it fills the screen over the outer
  editor's layer, and from `lg` it is the usual dialog. The modal's own catalog lookup reads `localCatalogs` only — every scoped catalog
  (real or draft) this editor can open here is already in it by construction, so there is no
  library fallback to reach for.
- **A catalog row shows one inline action — Edit for a scoped catalog, Remove for an unavailable
  one, nothing for a listed one — everything else is behind "⋯"**: Move up/down, **Copy into
  library** for a scoped catalog alone, **Unlink from library** for a listed one, and "Remove from
  folder". Each acts on the whole catalog, every genre of it: Unlink points every ref to the one
  draft it stages. Copy into library (`useCopyToLibrary`,
  `Workspace`'s `copyToLibrary`) creates a listed catalog at once, through the catalog create
  mutation, from the catalog as this editor holds it — a staged edit or a draft included — and
  says "Copied into your library", or why it couldn't, in a `Toast` beside the row's ⋯. It doesn't
  wait for the collection's Save and changes nothing in the collection. The grip
  reorders by pointer,
  touch and keyboard (`useDragSensors`' `KeyboardSensor`), so the menu's moves are the fallback,
  and the name keeps the row's width at phone size.
- **Each catalog row names its catalog with its kind sticker (Movies, Series) after it, then
  splits it by genre under its recipe line** (`GenreSplit.tsx`). One catalog makes one Nuvio tab
  per ref, each filtered by its genre, with no second catalog — what other addons do with a copy
  per genre. Unsplit (one ref, no genre filter) the row shows only a "Split by genre" link. Split,
  it shows "N tabs" after the name, a "Split by genre · one tab per genre" line, a chip per ref in
  tab order ("No genre filter" for the unfiltered one), draggable by pointer, touch and keyboard
  in their own sortable list (`genreContainer`), each with × to remove it, then a dashed "Genre"
  chip. The link and "Genre" open one dropdown (a Radix `Popover`) of ticks: "No genre filter",
  then the recipe's genre options. A tick is a ref, added after the catalog's last or removed at
  once; the last tick is disabled, since a catalog leaves the folder whole (⋯ or the picker). The
  dropdown's open state lives in `GenreSplit`, so the first tick, which turns the link into
  chips, leaves it open. The word is the collection's view mode (`folderUnit`): "row" for
  `ROWS`, "tab" otherwise — not "All", which a tabbed folder's `show_all_tab` already means.
  Options come from `POST /api/catalogs/genre-options` for the catalog's own recipe, not the
  whole TMDB list, so every tick actually narrows the tab; the query is keyed on the recipe, so
  editing a scoped catalog's filters in the nested editor refreshes them. A stored genre that the
  recipe no longer allows is kept, its chip and tick in danger red ("no longer applies"), with a
  note saying Nuvio shows that tab unfiltered. The Preview panel and Home's folder pages fetch each source's
  tiles with its genre (`queryKeys.catalogPreview` includes it), so they show the filtered row
  Nuvio will.
- **A folder page names each source the way Nuvio does**, as a tab and as a row title alike:
  `<Catalog name> (<Kind>)`, plus ` • <Genre>` when the ref is narrowed, e.g.
  "Popular (Movie) • Western" (`sourceLabel`, `features/preview/model.ts`). The genre suffix is
  also what tells two refs to one catalog apart. Tabs and tiles are keyed by
  `PreviewSource.key`, not the catalog id: the form's ref key in the collection editor, and
  `<catalog id>::<genre>` on Home, which the primary key makes unique within a saved folder.
- **The save bar's "N catalogs will be deleted"** joins "N folders will be deleted" when a
  scoped catalog this editor knows about would lose its last folder reference on Save — the exact
  condition `UpdateUserCollection`'s GC delete checks server-side, mirrored client-side the
  same way the folder-delete warning already was.
- **Delete's confirm** follows the rule under *Catalog authoring*. A collection's adds one line
  when N > 0: "N catalogs made only for this collection go with it." (its own scoped catalogs,
  which have no life outside it). Listed catalogs it merely references survive and go unmentioned.
- **The form runs Title, Folders, then one folding Appearance shelf** (`CollectionAppearance.tsx`, a
  `.sec-head` like a folder's, headed "Collection Appearance" to tell it from the folder's "Folder
  Appearance"): How folders open, the "All" tab, Background image and Focus glow,
  summarised on the closed head ("Tabbed Grids · All tab · glow on", `appearanceSummary`). The
  publish button is in the save bar (see "Sharing" below).
  The "All" tab is a `Segmented`, greyed (DESIGN.md's "Greyed" segmented state,
  `Segmented`'s `disabled` prop) rather than hidden while the view mode isn't Tabbed Grids,
  keeping its value for when it switches back. There is no Pin here: it is the Home pane's
  pending edit, which push writes (see "Home pane — List view").
- **The Preview panel is a working client screen, docked beside the form.** The collection editor has
  its own layout (`.ed.ed-preview`): the form keeps
  its `--w-form` column and the panel takes the rest of the pane, at least 380px, beside it, undocking under
  the form below 960px of pane. It draws the live draft with the Home Preview's own components
  (`PreviewCollectionRow`, `PreviewFolderPage` from `features/home/previewScreen.tsx`), so a folder tile opens the same
  folder page Home does — tabs or rows per `view_mode`, real titles per catalog. The rows sit in
  `.pv-rows` inside the raised `.ed-pv` panel the catalog editor's results use: tiles are the
  same fixed pixel sizes as Home's, rows run off the panel's edge and scroll, and a folder grid
  reflows to the column. Docked, the panel is sticky, so the rows scroll inside it at a height
  that fits beside the form; undocked (and in Community) they flow with the page. Tiles come from each source's recipe (`useRecipesTiles`), not its id, so an
  unsaved draft and a Community collection both preview, and only the open folder's catalogs are
  fetched. The back arrow and Escape return to the row — Escape is stopped inside the panel so it
  never reaches the editor's own Escape-to-close — and there is no history entry.

- **A save carries the collection's revision the editor opened at**, as a catalog save does
  (`formFromCollection` copies it into `target.initial`; `Workspace` sends it as
  `CollectionSave`). `catalog_edits` carry none: the collection's revision guards the catalogs
  scoped to it. A refused save shows "Error saving." where any save error shows.

## Sharing

What an owner publishes is a **publication**: a snapshot of the row as it was saved when it was
published, which Community lists and others add. Live edits stay private until the publisher
publishes an update. A row added from Community is a **subscription**: it is read-only, it follows
its publisher's updates, applied when its subscriber chooses, and it opens as a view, never an
editor. Duplicate makes a separate copy that is the subscriber's to edit. The words are
`docs/architecture.md`'s *Sharing vocabulary*: the UI says Publish, Add, Unpublish, Duplicate and
publisher, and calls only what Duplicate makes a copy. The model and the routes are
`docs/data-model.md` ("Publications and subscriptions") and `docs/architecture.md`; the pieces here
live in `web/src/features/sharing/`, and `Workspace.tsx` reaches them through one hook,
`useWorkspaceSharing`.

- **The publish button** is a listed catalog's or a collection's one step with Community, on its
  editor's save bar, between the status and Close
  (`SignStepButton` in `components/PaneSign.tsx`,
  drawn by `EditorFooter` from the editor's `sharingStep`). It names the next step from where the row stands (`sharingStep` over
  `ownSharing`): Publish… while private, Publish update… once the saved row differs from what was
  published (`changed_since_publish`, which the sign's To publish sticker also says), and
  Unpublish… while live and unchanged; after Unpublish the row is private again and says
  Publish…. The publishes open the publish dialog; Unpublish… asks first ("Community stops
  listing it. People who added it keep it as their own and won’t get your updates, even if you
  publish it again."), since unpublishing is one-way. Community holds the saved row, so while the form has
  unsaved changes the button is greyed (`aria-disabled`) and pressing it shows "Save first." as a
  one-line toast above it.
- **A collection that uses a catalog added from Community can be published** like any other. A
  row added from Community can't be: it opens as a view with no publish button (below).
- **The publish dialog** (`PublishDialog.tsx`) lists everything the publication will hold, each
  catalog by name over its recipe line: for a collection, its own catalogs under "N folders, N
  catalogs of its own", then the library catalogs it uses under "From your library, public as part
  of this collection" (`publishGroups`), with a line saying anyone who opens the collection can read
  them as they are now and that Community doesn't list them on their own: publishing a collection
  makes those catalogs readable as part of it, which is the point to consent to, and nothing more.
  When it publishes changes to an already published row — the row's
  publish button says Publish update… — a **Since you last published** list comes first, under
  its line: what every follower will be offered (`SinceLastPublished`, from
  `GET .../changes-since-publish`, fetched as the dialog opens). It is also what explains a
  collection flagged To publish because a library catalog it uses was edited. Publishing a row
  not published, never or not since Unpublish, shows no list, and neither does the editor. A catalog added from Community carries the From Community sticker
  (`FROM_COMMUNITY`, the one `sharingState.ts` draws it from), so it reads as someone else's catalog being
  published as it stands. Its heading is "Publish “X”?" (or "Publish your changes to “X”?"), its
  button Publish (or Publish update), and its line "Anyone on Uno can find it in Community and add
  it. Your later edits stay private until you publish an update." (or "People who added it are
  offered this version. Until then they keep the one they have."). The server's refusal (a 400, or a 502 when TMDB can't check a recipe)
  shows in the dialog. The dialog of a published row (Publish update…) also has a red text
  Unpublish at its footer's start, which closes it and asks the Unpublish question.
- **Stickers and flags** (`sharingState.ts` is the one place a row turns into them; `SharingStickers`
  draws them). Flags are plain stickers, never buttons, and each names a state, never a verb, where
  a row is behind: **To push** (a push would change what Nuvio holds for it), **To publish** (an
  own published row's saved version differs from what it published) and **Update available** (the
  publisher of a row added from Community published a newer version). Only the incoming one says
  "update". A row carries **one Community sticker**, changing with its state: an own row reads
  Published, then To publish; a row added from Community reads From Community, then Update
  available; a row whose publisher unpublished it is the profile's own and carries what any own
  row does. A
  sticker's `tone` is `{hue, fill}` (DESIGN.md's *Sticker Rule*), drawn by
  `stickerClass` as `.stk` with one hue class and `.stk-fill`: a pill is filled only while it
  waits on you, until one action clears it — To publish and Update available (pink), To push
  (yellow); Published and From Community are pink outlines.
- **A row its publisher unpublished becomes an own row like any other, silently.** Nothing
  tells its owner: the From Community sticker goes and its editor opens for edits.
  - **Where they show:** the library rail shows a catalog's kind and the Community sticker
    only (`railStickers`) — never To push, which the pending count already covers, and no
    Collection sticker, which the rail's Collections sign already says. An editor's sign and the
    From Community view's sign show every flag (`rowStickers`, and `viewStickers` for the view).
    The Home pane's rows, the "Only in Discover" tray's included, show the kind and To push alone
    (`homeStickers`). Below `sm` the sign hides its
    stickers, so `EditorShell` heads the body with them.
  - **The kind wears its region's hue** wherever it shows: Movies and Series tangerine
    (`kindSticker`, `kindStickers`), Collection green (`COLLECTION_KIND`), on the rail, Home,
    a folder's catalog rows, Community rows and pages, and the publish dialog. On a sign it is
    printed in sign ink like every sticker there.
  - **To push comes from the waiting list** (`pending` in `GET .../library`, `waitingIDs`: the
    `added` and `changed` rows, never `removed`), for catalogs and collections alike.
    `usePushWaiting` reads it for the Workspace, `HomeSelection.waitingForPush` for the Home pane,
    both from the one library query the list of changes reads. A catalog scoped to a collection has no
    flag of its own: editing one makes the list report its collection `changed`, which flags the
    collection.
- **A row added from Community opens as a view** (`FromCommunityView.tsx`), with no form: `Workspace`
  routes a library row that has a `subscription` to `CatalogFromCommunity` or
  `CollectionFromCommunity` instead of an editor, from the live library row, so an applied Update
  shows at once. Only the row's publisher changes it, so nothing in the view can be unsaved and
  closing never asks. Its place on the home screen, its pin included, is the Home pane's, as
  for any row.
  - **Frame:** `EditorShell`, as for any row: the region's sign (tangerine catalog, green
    collection) with the From Community sticker (and To push while a push would change what
    Nuvio holds for it), ×, Escape,
    and below `lg` Duplicate and Delete on the sign and the editor layer. The sign says From
    Community in place of Update available, since the Update… button says it. Below `sm` the sign
    hides stickers, so `EditorShell` heads the body with them.
  - **No explanatory text:** no sentence, no ⓘ, and no "Added by N · updated …" line — that
    stays on the Community page. The sticker says where it came from. While an update waits, the
    body leads with a community-pink **Update…** (`ViewLead`, `update_available`), which opens
    the publication's page (below), with a
    dim **"7 changes"** beside it (`UpdateCount`, `changeCount`: the lines the page's shelf
    shows, from the same call). The count shows nothing while that loads, if it fails or if it is
    empty.
  - **The catalog block** (`CatalogBlock.tsx`, shared by both views and the Community
    publication page): open, the recipe as spec tiles — flat `raised` tiles on the ground in an
    auto-fill grid (140px minimum), the one-value facts first so they share rows, then the list facts
    (genres, production companies, keywords, networks, streaming services) a row each, a dim 12px label over a bold 15px value, for the facts the recipe sets (`recipeFacts`
    in `features/library/recipe.ts`: Type, Genres, Released, Rating, Votes, Age rating, Order, …; production companies,
    keywords, networks and streaming services are **named** — "Production companies: Studio Ghibli or Pixar",
    the label singular for one id, the names joined with "and"/"or" as the stored list is;
    `recipeLine` and the rail's summaries keep counts); folded, a chevron, the catalog's name and
    under it its order and next two filters, a genre list of more than two counted, then "+9" for
    the rest (`foldedLine`, drawn by `FoldedSummary`), as a button with `aria-expanded`. Folded
    blocks' tiles flow at 140px a tile with the lists on a row each, like the open block's. The names load through the
    lookups the catalog editor's pickers use (`useRecipeNames`: `fetchCompany`, `fetchKeyword`
    and `fetchNetwork` under the pickers' own query keys, and the watch-provider list of the
    recipe's region). A list shows "…" while its lookups are answering (`useRecipeNames`' `loading`), then
    its names once every one has arrived, and settles on its count for one a lookup can't name; a
    folded block asks for none until it opens, and its tiles step down to `ground` wells in two
    columns inside the folder card. After the recipe's own tiles, open or folded, come the
    filters it leaves open (`openFacts`): Genres, Released (Aired for series), Rating, Votes,
    Runtime, Language, Age rating, Streaming service, Production company, Keywords, Network
    (series only) and Order, each reading "Any" ("Any time", "Any length") and Order as TMDB's
    own "Most popular", outlined in `line` with no fill and set in dimmer type — so someone who
    only ever adds rows from Community sees every filter a catalog could set beside the ones it
    does. A TMDB collection row has none: the collection is its whole recipe.
  - **A catalog:** the open block, without the name (the sign carries it), beside the live
    results (`SavedCatalogPreview`). **A collection:** a card for each folder, its catalogs as
    folded blocks that open in place, the folder's name at 16.5px with its catalog count dim at the
    end, ruled off from the blocks by a `line-hi` rule — a folder's narrowing genre is a "Narrowed to" tile and
    follows the recipe line when folded — beside the Preview panel (`SavedCollectionPreview`). A
    catalog inside it never opens an editor. `CatalogBody` and `CollectionBody`
    (`PublicationBodies.tsx`) draw both, and `PublicationPage` too.
  - **Footer:** Close, and **Duplicate to edit** in the region's colour, which is the row's own
    Duplicate (it asks first, as the rail's does). While Update… shows, Duplicate to edit is
    outlined, so one filled button is on screen. "Duplicate to edit" is this view's label only;
    the rail, the "⋯" menus and Community say "Duplicate".
- **Update… opens the publication's page**, which shows the new version before it is applied:
  the page's Update does it (Community tab, below). A view's own Update… leaves the Workspace
  through the editor guard, like every other way out, and opens Community on that page:
  `Workspace`'s `openPublication` reaches `Builder`, which switches the tab and hands
  `CommunityView` the publication (`initialOpen`) and its kind, so going back lands on that kind's
  list. Switching tabs in the header opens Community on its list.
- **A "what changed" list** (`features/sharing/`) draws one server comparison (`GET .../changes`,
  `GET .../changes-since-publish`; `docs/data-model.md`, *Publications and subscriptions*) in
  words, only beside the button that applies its changes — never as standing status, so not in the
  editor and not in the Unpushed changes strip. `groupChanges` (`changeWords.ts`) sets the items
  under dim **Removed**, **Added** and **Changed** labels, one ink line each with no verb repeated:
  `Folder “Kids” · 1 catalog`, `“Gore classics” from “80s”`, `“Predator picks” to “Streaming”`,
  `Folder name: “Kids” → “Family”`, `Folder order`. A catalog removed from or added to a folder
  that is itself removed or added is not a line of its own: the folder's line counts it. A changed
  catalog is its name with sub-lines for what differs, from `recipeFacts` of the recipe it was
  (`was_catalog`, `docs/data-model.md`) against the one it is — `Production companies: 2 → 1`, `Keyword added`,
  `Order: Most popular → Highest rated`, `Rating: any → 7.0 or more` — with a singular and plural
  label read as one fact, lists counted as the lookups are not asked, and "Filters changed" when
  nothing shows. `ChangeList` shows the first six lines in the server's order — removals, then
  additions, then changes — then "and N more", which opens the rest in place (`takeLines`). Plain
  ink, no colour: pink means Community and red means destructive, so neither marks a removal. That
  list is the Publish dialog's **Since you last published**, with no card.
- **The In this update shelf** (`UpdateChanges` on the Community page, `ChangesBlock` with `shelf`):
  a `raised-hi` card with 16px corners, a step above the folder cards under it, the heading in
  sign lettering in the region's accent and a dim "7 changes" (`changeCount`) at its ends. Its
  content is one line (`UpdateSummary` in `MarkViews.tsx`, over `updateSummary(updateMarks(list),
  kind)` in `updateMarks.ts`): what the update does, counted — "1 new folder · 2 folders renamed
  · 1 folder removed · 1 catalog changed · folder order changed", a catalog publication's "renamed,
  was … · filters changed" — each part with a target a button underlined in the accent that
  scrolls to its mark and focuses it (smoothly, unless reduced motion is asked for).
  `updateMarks` places each change by the `key` and `folder_key` the server gives it on what the
  new version shows; `useUpdateMarks` (`Changes.tsx`) reads the shelf's own query, and
  `PublicationPage` hands the marks to `CollectionBody`/`CatalogBody` and draws a renamed
  publication's old name under the meta line (`RenamedFrom`, id `update-renamed`). A folder card
  (id `update-folder-<key>`) says under its name what the update does to it
  (`folderMarkWords`), lists at its foot the catalogs it loses (`RemovedCatalogs`), and the
  folders it removes follow the cards (`RemovedFolders`, id `update-removed`). A catalog block
  takes a `BlockMark` (`folderEntryMark`, `pageBlockMark`): its words under its line; where the
  new version first uses a changed catalog (`firstUses`), its id (`update-catalog-<key>`) and,
  for a changed recipe, an open start and marked tiles; elsewhere "filters changed, see above".
  The tiles (`MarkedTiles.tsx`: `SetTile`, `OpenTile`, `GoneTiles`) read `useMarkedChanges`:
  `recipeChanges` over both recipes as `recipeFacts` plus `openFacts`, named through
  `useRecipeNames` as the tiles are, so a filter that comes or goes reads against "Any". A list
  fact carries its items, join and region (`FactList`), and `factChangeNote` says a list's change
  as what it gained and lost, its join and its region, and anything else as "was …". No quote
  marks and no arrows. The
  queries (`useChanges`) are fetched when shown and never kept (`gcTime: 0`, under
  `['p', i, 'changes', …]`, which no write waits on); a list that is loading or can't load says so
  quietly and never gets in the way of the button, and an empty list shows nothing.
- **Every sharing call refreshes the library and Community** (`invalidateProfileLists`) and
  settles once they have refetched, so a row's stickers and state are current when its toast
  ("Published “X”", "Published your changes to “X”", "Unpublished “X”") shows under the rail's
  search row.

## Community tab

`web/src/features/community/` — what other profiles publish, a page at a time
(`GET /api/p/{i}/community`), searched, filtered and sorted by the server. No publisher, no
handle, no "copied from" line appears anywhere here: Community never names who published a row.

- **`CommunityView.tsx`** holds Catalogs / Collections, the search box, and Sort: Name / Newest.
  `useCommunityList` (`useCommunity.ts`) is an infinite query keyed on kind, sort and search
  (`queryKeys.communityList`), and the server answers the rows of the chosen kind where every
  word of the search is in the title or one of the catalog names, whatever the ASCII case and not
  necessarily all in one name, sorted by name or newest first. The search reaches the server
  trimmed, once typing has held still for 300ms (`useDebounce`); the box takes at most the
  server's 200 characters, and a search of over 8 distinct words is not sent: `SearchNote` says
  "Search with 8 words at most." under the box (`searchProblem`). A new query keeps the rows on
  show until its first page arrives
  (`keepPreviousData`). `rowsOf` (`communityQuery.ts`) joins the pages read; **Show more**
  (`ShowMore.tsx`) under them reads the next while one follows, says Loading… meanwhile, and on a
  failure says "Couldn’t load more." and offers Try again, the rows read so far staying
  (`listError` keeps a failed next page out of the list's own error). An empty list says whether
  nothing of that kind is published ("Nobody has published any catalogs yet. Publish one of your
  own from its editor.") or nothing matches.
- **The open page's row** is `openRow`: the listed row while one is read, else the publication's
  detail (`useOpenPublication`, the same query the page reads), so a page opened from an added
  row's Update… shows though its row is on a page not read yet. A detail that failed to refresh
  counts for nothing, so a publication unpublished meanwhile drops back to the list.
- **A row** (`CommunityRow.tsx`) is a Library-row-style target: 12px corners, a raised fill on
  hover and raised-hi pressed, no dividers, and a pointer cursor, with the name button covering the
  row (`after:absolute after:inset-0`) and the actions above it (`z-10`) keeping the default cursor
  (DESIGN.md's Navigates Rule). Its content is the name with a kind sticker (Movies or Series; the
  Collections list leaves Collection off); a summary (a catalog's recipe line, a collection's folders worded
  as the Library rail and Home word them, "3 folders · Action, Drama, Comedy", from the titles of
  the row's `folders` through `describeFolders` — `itemSummary`); and "Added by 3 · Published 3 weeks
  ago · Updated 2 days ago" (`itemMeta`'s facts, each kept whole by `MetaParts` so a narrow
  column breaks between them), without Added by while nobody has added it and without
  Updated while it was never updated. A row previews what it holds (`RowPreview.tsx`): a catalog
  fans its first five posters ahead of the text (`PosterStack`), fetched under
  `queryKeys.catalogPreview` once the row first scrolls into view (an `IntersectionObserver`), so
  the publication page's results draw from the same answer with no second call; a collection
  lines up its first six folder tiles under the summary at their own shapes, then "+N more"
  (`FolderStrip`), from the row's `folders` with no fetch; from `sm` the tiles shrink together,
  keeping their shapes, where the row is too narrow for them. The poster stack, and from `sm` the
  strip, take no pointer events, so a click on them reaches the name button and opens the
  page. Below `sm` a catalog row stacks
  beside its posters (name with a two-line clamp, stickers, summary, meta, actions on
  their own line), the folder strip scrolls sideways above the row's name button, and opens the
  page on a tap itself, and a touch screen adds a chevron to the name line. Its main button is Add (the
  `subscribe` action), a disabled ✓ Added while the profile has added it, or Update…, outlined in the Community accent (`btn-accent-outline`), while an
  update waits, which opens the publication's page; Duplicate, a copy that is the profile's own
  (`POST .../duplicate`), waits behind "⋯" with "yours to edit" beside it, or "the latest version"
  while an update waits, since it copies the publication and not the older added row. No ⓘ. While an action is
  in flight the button says so (Adding…, Updating…, Duplicating…). The search box says "Search".
- **A row opens its publication's page in place of the list** (`PublicationPage.tsx`, DESIGN.md's
  One Occupant Rule). `CommunitySign` turns Community's sign into the page's: a round back arrow
  outlined in sign ink, the name in Sign Title, then the kind sticker from `sm` up (below `sm` it
  heads the body); no sticker says an update waits, since the filled Update and the shelf do. The arrow and Escape both leave (the One Way Back rule)
  and focus lands on the title. The body leads with the dim meta line; then the actions
  (`PageActions`) — one primary in the Community accent, Add, or Update while an update waits, or
  a disabled outlined ✓ Added, beside an outlined Duplicate (no "⋯", no ⓘ); then, while an update
  waits for this profile's added row, the **In this update** shelf (`UpdateChanges`, `GET
  .../community/{id}/changes`); then what the page holds, from the detail call (`GET
  .../community/{id}`): a collection's folders and their catalogs beside its Preview panel
  (`snapshotAsCollection`, the snapshot read as a `Collection` so `SavedCollectionPreview` draws
  it), or a catalog's spec tiles beside one page of its results (`SavedCatalogPreview`, which runs
  as it mounts) — the same catalog block and bodies a row added from Community opens as
  (`features/sharing/PublicationBodies.tsx`, which take the lead and draw it alone while the detail
  loads). The list sits in an 1100px column and a page in a 1320px one (`.community-body`,
  `index.css`); once the page is wide enough to dock, its details take 45% of it and the results or
  Preview the other 55%, with posters at least 130px
  (`--tile-min`), and below that it stacks as the editors do. Update applies the new version (`POST .../update`) and leaves the page on ✓ Added. A
  saved recipe with no results says "Nothing matches these filters." (`RecipePreview`'s
  `readOnly`). A page that won't load says "Couldn't load this. Its publisher may have
  unpublished it." Going back restores the list's scroll and puts focus on the row's open button
  (`scroll.ts`).
- **Add, Update and Duplicate refresh the library and Community** (`useCommunityMutations.ts`);
  each settles only once they have refetched, so a row never offers Add again for a row it
  already added. Community reads only the genre lookups
  of the library (`useGenreLookups`), for its recipe lines. The toast says what happened: "Added to your catalogs"/"collections",
  "Duplicated to your …", "Updated “X”". A 404 or a 409 means the row was behind the server
  (`isStale`), and the lists refresh before the toast: a 409 from Add is "Already added", a 404
  is "Its publisher unpublished it." Any other failure is "Couldn't add it: …" (or update,
  duplicate).
- **A 404 inside Community is not "profile not selected".** `http.ts` raises
  `ProfileNotSelectedError` only for the profile check's own 404, and the list sends the user back
  to the picker on one; a publication's page answers its own 404 once its publisher unpublishes
  it, an ordinary `ApiError`, so it reads that as unpublished and stays put.
- **The tab switch is a `Segmented` in `Builder.tsx`'s header**, not a route — `/configure` stays
  one URL. Above `lg` it sits inline in the header row; below `lg` it drops to its own row
  underneath, because the header row's height is measured to fit exactly what it holds at phone
  width and a third control has no room there (see "`/configure` — Workspace and Community"
  above for the guard this goes through).

## Push UI

Header button, beside the pending indicator from `lg` up. **One call**, one indeterminate in-flight state — no
staged progress, because it is one HTTP call from the frontend's perspective and animating
through fake stages ("Saving…", "Installing addon…") would be fabricated.

**Two failure states, not four:**

- **Ordinary failure** (bad input, Nuvio unreachable, either push rejected, or a Home another tab
  has pushed since) — *nothing changed*,
  full stop, catalog selection included. One generic message: "Push failed — nothing changed.
  Your edits are still here; try again." True for every ordinary failure mode because of the
  backend ordering.
- **Rare compound failure** (`undo_failed`) — Nuvio took part or all of the push, a later step
  failed, *and* the compensating undo also failed. The one case where "nothing changed" isn't
  true. Copy: "Push failed, and we couldn't fully undo it — your collections in Nuvio may be
  temporarily out of sync. Push again to reconcile."
- **A refusal the server names is an ordinary failure in its own words** (`failedOutcome`):
  `refused: empty_collection` is the outcome `empty-collection` ("A collection on Home has no
  folders — nothing changed."), and `refused: shares_addons` is `shares-addons` ("This profile
  uses profile 1's addons in Nuvio — nothing changed."). The builder blocks both before they're
  sent (below); these words are for a tab that missed one. `refused: profile_changed` is
  `profile-changed` ("This profile changed in Nuvio — nothing changed."): the profile was
  deleted or replaced in Nuvio since it was picked. The builder stays put with every pending edit,
  and the words send the user back to pick the profile again, rather than the builder navigating
  away and losing them. `refused: home_order_unreadable` is `home-order-unreadable` ("Couldn't
  read this profile's home order in Nuvio — nothing changed."): push stopped before writing
  anything to Nuvio, and the detail line says a reorder in a Nuvio app saves a fresh list.
  `refused: too_many_catalogs` is `too-many-catalogs` ("Home has too many catalogs for one push —
  nothing changed."): the detail line says a push can give Nuvio up to 1,000 catalogs, counting
  every one inside a collection. Nothing blocks it before sending, since no real Home comes near.
  A refusal value the builder has no words for reads as the generic failure.
- **A 429 is an ordinary failure in its own words** (`RateLimitedError`, outcome `rate-limited`):
  Uno's server answers none, but a proxy in front of it could, and one would have turned the
  push away before running it, so nothing changed. Copy: "Too many pushes
  — nothing changed. Your edits are still here. Wait a few seconds, then try again." Without it
  a 429 would read as "Couldn't confirm what happened", the outcome for no usable answer at all.

- **The Push button stays enabled with zero pending edits.** It is the only recovery path from
  that compound-failure case, and nothing else marks that state — graying it out on
  `isDirty === false` is an obvious-looking cleanup that quietly removes the recovery path.
- **Push is blocked until the home selection has loaded** (`home.ready`), both on the button
  and inside `push()`. Before then `snapshot()` returns `EMPTY_HOME`, and a full-replace push
  of it would remove every Uno catalog row and collection from the profile.
- **Push is off while something blocks it** (`usePushBlock` → `pushBlock`,
  `BlockablePushButton`): the profile uses profile 1's addons in Nuvio, or a collection on Home
  has no folders. `PushBlockNote`, a strip under the header beside the push outcome, says which
  and what to do: turn sharing off in Nuvio and pick the profile again, or add a folder to the
  named collection or take it off Home. The server refuses both too (*Push* in
  `docs/architecture.md`).
- **A second push is blocked while one is in flight**, via a **ref**, not render-captured
  state — the guard has to reject a second call raised before a re-render. Two overlapping
  `PullCollections`→`PushCollections` cycles can clobber each other.
- **`markPushed` takes the pushed state, not `current`.** The user can keep editing while a push
  is in flight; advancing the baseline to "whatever is current now" would silently swallow those
  edits and report them as already live. It takes the `home_revision` the push answered too,
  which the tab's next push sends (`toPushPayload(sent, homeRevision)`). A push from a tab whose
  Home another tab has pushed since is a `409` with a plain `{success: false}`, the ordinary
  failure: nothing refetches, and the user refreshes the page.
- **The library is invalidated on success.** Push rewrites every owned row's
  `home_position` and `show_in_home` and a collection's `pin_to_top`, which Home hydrates from,
  an added collection starts its pin from, and the delete dialog reads. With `staleTime: 30_000`,
  a stale library would show pre-push data for up to that window while the builder stays open, and
  the pushed changes would look undone.
- **The pending list rides in the library, so that invalidation covers it.** The list of what
  waits for a push (`pending` in `GET .../library`) is read from the server, so without
  refreshing it the "changed since it was last pushed" lines would never clear after a
  successful push, and every push would look as if it had failed to update anything. The writes
  that change what Nuvio holds (`invalidateProfileLists`, and the collection mutations) refresh
  the library too.
- **`ApiError` carries an optional `body`** (`web/src/api/http.ts`), best-effort JSON-parsed
  from the text it already reads on every non-2xx. Without it, push's structured failure arrives
  as an `ApiError` whose `message` is the raw JSON blob — unusable, and worse, renderable
  straight into an error UI. No other call site reads `.body`; it exists for this one endpoint.
- The manifest URL comes from `POST /api/profiles/select` only, so it's shown as a header copy button
  from profile selection onward rather than waiting on a first push, and `PushBanner`'s success names
  the same URL, passed down by `Builder`. Push's answer doesn't carry it.

## Cross-cutting client rules

- `401` from any call → refresh + retry, then bounce to login; a refresh that fails without
  Nuvio refusing the token fails the call and keeps the session.
- `404` with the JSON `code` `profile_not_found` from any `/api/p/{i}/...` route, `requireProfile`'s
  → profile not selected → send back to the picker (`ProfileNotSelectedError`,
  `web/src/api/http.ts`, which reads the code and never the words). A route's own 404 (a catalog
  or a publication not found) has no code, an ordinary `ApiError`. A JSON error
  body is worded by its `error` field and kept as `ApiError.body`.
- `429` from any call → `RateLimitedError` (`web/src/api/http.ts`), worded from `Retry-After`. Uno's
  own server answers none, so only something in front of it can:
  "Too many requests. Try again in 10 seconds.", or "in a moment" without a usable header. Nothing
  retries it: `apiFetch` refreshes and retries only a 401, and `query-client.ts` never retries a
  4xx. Every place that shows an error message shows this one as it is.
- `403` is the server's access policy refusing the account, in its own words, shown like any
  other error. The profile picker alone words it itself (below).
- `422` from any call → the account's own TMDB key can't be used (per-account key mode only);
  the builder's key banner (*Profile selection* → *A key problem in the builder*).
- The library's list endpoints and Community's are unpaginated; assume small N.

## Visual direction

Dark-only, single theme: the builder is used at a desk and on the couch in the
evening, and a light theme is ruled out. `DESIGN.md` holds the full system —
tokens, components, rules; this section says what the frontend implements and where.

**Video Store, after hours.** The builder reads like a neighbourhood video shop after closing:
each region is announced by a sign in its own colour, every catalog carries a plain-words line
saying what it returns, and the home screen is the front shelf, in Nuvio's order.

**Colour holds one meaning per hue.** A night-navy ground and its shelves (`--uno-ground`,
`--uno-raised`, `--uno-raised-hi`) carry the layout. Three hues name the regions — catalog
tangerine, collection green, community pink — and Nuvio yellow means "bound for Nuvio" and nothing
else: Push, what is on the home screen, what is not pushed yet. Danger red marks what can't be
undone, always beside words. A region's colour reaches its primary button and headings through
`--accent`, set by the `.tone-*` classes: `EditorShell` sets `tone-catalog` or `tone-collection`,
the Home pane `tone-home`, Community `tone-community`. Tokens live in `web/src/index.css` as
`--uno-*` custom properties, exposed to Tailwind via `@theme inline`.

**Signs.** `.sign` is a flat band in its region's colour across the full width of what it names —
the rail's Catalogs and Collections, the Home pane, each editor's header, Community — lettered in
`.type-sign`.

**Type — two faces.** Figtree carries every word Uno says, previews included; Archivo at its
widest and heaviest is sign lettering only. Both ship as Fontsource packages
(`@fontsource-variable/figtree/wght.css`, and `@fontsource-variable/archivo/wdth.css` — the file
that carries Archivo's width axis), not the Google Fonts CDN: the build is `go:embed`'d, and the CSP allows
fonts only from `'self'` and `data:`.

**Stickers.** Small printed pills state a row's states in words, following DESIGN.md's *Sticker
Rule*: every pill is `.stk` plus one hue — `.stk-catalog` (the kind Movies or Series, in
tangerine), `.stk-collection` (the kind Collection, in green), `.stk-community` (Published, From Community, To publish, Update available, because
Community is where those rows turn up) and `.stk-nuvio` (To push, bound for Nuvio) — and `.stk-fill` while it waits on you, until one action clears it (To publish, Update
available, To push); every other pill is an outline. On a sign an outline pill turns sign ink and
a filled one becomes a sign-ink pill lettered in its hue. A library row's home-screen toggle is `.home-sticker`: a dashed empty
circle while it's off the home screen, a yellow ON NUVIO price sticker (two lines, ON over NUVIO)
once it's on. A home row's position is a yellow `.pos-sticker`, read out as "3rd on your home
screen"; the pending count and a profile's slot
number are `.count-sticker`s.

**Settings — label above control.** Every editor setting is a `.setting`: its label
(`.setting-label`) above the control, on the form's one left edge. Folding sections (`.sec-head`
and `.sec-body`) are shelves that open in place; a heading inside a form (`.setting.is-head`) is
sign lettering in the region's colour over a rule of the same colour. The rail, the Home rows
and a folder's catalog picker show a catalog's recipe as one line (`recipeLine`, or
`catalogListing` where the list is also searched).

**Shapes and depth.** Buttons, stickers and segmented controls are pills; fields are
10px-rounded wells; shelves, panels and dialogs round at 14–20px. Depth is tonal — ground, then
raised, then raised-hi — and nothing casts a drop shadow: menus, popovers and dialogs separate by
a lighter fill and a `line-hi` edge, over the scrim where they block.

**Motion — the sticker moment.** Putting a row on the home screen from the rail slaps the ON NUVIO sticker on
(`uno-slap` in `index.css`), and a successful push counts the pending number down to zero
(`useCountDown`) before the clean state shows. Everything else is a 160ms colour or background
transition on the one easing curve, `--uno-ease`. `prefers-reduced-motion` turns both off.

**Previews wear the shop's look.** The preview rows follow Nuvio's layout but sit on the app's
own surfaces: Home's in a raised `.pv-panel`, the editors' and Community's in the docked `.ed-pv`,
with tiles on raised-hi and Figtree captions. With nothing on the home screen, Home's Preview
panel holds a test card in the palette's own colours (`.pv-nosignal`): seven bars bright to dark
over a castellated strip, with a sticker-white NO SIGNAL sticker. It is the one place the region
hues stand side by side inside the builder (DESIGN.md's One Meaning Rule names the exception).
Uno's words about the rows sit above and below the panel.

**The way in.** The sign-in page and the profile picker carry the shop's fascia — the four hues
side by side (`web/src/components/Fascia.tsx`) — and each profile is a membership card. The
fascia appears nowhere inside the builder, where each hue stands for its own region.

**Copy — written for someone who has never heard of TMDB, Stremio, or an addon manifest.** The
user signs into Nuvio and builds rows for their home screen, on whatever device they watch on —
TV, phone or desktop. That is the whole of what they are assumed to know. Four rules, in order:

1. **No sentence the control already says.** A greyed control needs no "only applies to…", a
   value shown as plain text needs no "can't be changed", a labelled group needs no caption
   restating its label, and a preview needs no paragraph on how to scroll it or go back. Words
   earn a place on the page only in empty states, errors, and confirmations that say what an
   action will cost.
2. **Prefer a control that needs no explanation to a control plus a sentence.** A hint that
   exists to compensate for an unreadable input is a bug report about the input. The
   watch-provider field is a named, searchable picker and carries no hint text at all, rather
   than a box for TMDB's numeric ids beside "find IDs in the TMDB documentation". `vote_count`
   reads "Number of ratings" and needs no hint either.
3. **Never print the provider's vocabulary.** `describeRecipe`
   (`web/src/features/library/recipe.ts`) returns plain English — "Most popular · Action and
   Comedy · rated 7.0+ · in Japanese", not `popularity.desc · Action + Comedy · ★7.0+ · lang
   ja`. Language, country and region codes resolve through `Intl.DisplayNames`. This is the
   app's highest-volume text: it renders on every Home row and every row of the folder picker.
4. **Say what the user sees, not why the code can't do better.** "Shown as rows — the app
   decides the real layout", not "Uno can't read the app's layout setting". Errors name the
   failure and the next action without naming the machinery: "Couldn't load the preview. Your
   filters are fine — try again."

**`InfoTip` is for the narrow middle.** A sentence that doesn't survive "is this needed at all"
is deleted, not moved. The icon holds one a control genuinely needs but that would crowd the page:
Community's Add button (Add versus Duplicate), the collection's Focus glow and Background image, a folder's Focus GIF and
Modern Home fields, the catalog picker's link-versus-copy, and the genre chips' three-state cycle.
It opens on click rather than hover, so it works on touch.
