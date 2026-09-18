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
| Drag-and-drop | dnd-kit — `PointerSensor` (covers touch and mouse) + `KeyboardSensor`, so every reorderable list is operable with no pointer at all |
| Components | Hand-written, on Radix UI primitives (`Popover`, `Slider`, `DropdownMenu`) for the three controls that need real accessible behaviour. Everything else — `Modal`, `ConfirmDialog`, `TypeBar`, `ListState`, `fields.tsx`, `GlyphButton` — is local. Styling is Tailwind v4 against Uno's own `--uno-*` tokens in `web/src/index.css`; `shadcn` remains a devDependency (its CLI), and a small base set (`--background`, `--foreground`, `--border`, `--ring`, `--sidebar`) is aliased onto the Uno palette |
| Types | **Hand-written per endpoint, no codegen.** For a one-person team on both ends, drift surfaces immediately in the browser rather than silently in production. If drift pain ever shows up, a lightweight generator reading the Go structs (`tygo`-style) is the first upgrade to reach for, not a full OpenAPI pipeline |

Because there is no codegen, **`"strict": true` must stay on in `web/tsconfig.app.json`.** The
`| null` unions and `?? []` guards in `web/src/api/types.ts` are the only thing enforcing the
wire contract; without `strictNullChecks` they are documentation the compiler never reads. The
code is strict-clean, so turning it off would cost nothing immediately and remove the only drift
check that exists.

Load-bearing shared modules, rather than a directory listing: `web/src/components/fields.tsx`
holds `Field`, `TextInput`, `Select`, `Segmented`, `Checkbox` — shared between both editors —
while `web/src/features/catalogs/fields.tsx` re-exports them and keeps `NumberInput`,
`RangeField`, `GenrePicker`, which are catalog-shaped. `EditorShell` + `EditorFooter` +
`useEditorForm` under `web/src/features/builder/` are the scaffolding both editors sit in, so a
catalog and a collection get the same header, dirty state, footer, and save/discard behaviour
from one place. `web/src/features/preview/` holds the Jost-styled tile rendering shared by the
collection editor's own preview panel and the catalog editor's Run button. The Home pane's
Preview on TV tab is a separate tree, `web/src/features/home/tv.tsx` — Roboto inside a real
framed bezel, not a restyle of `features/preview/` — because `cqw` sizing only means anything
against that frame's own `container-type`, and the two panels carry different content (the
collection editor's panel states layout only; the TV picture never pins a note onto itself — see
"Home pane — Preview view" below). Nothing under `features/` is a separate URL; `Builder`
composes all of it.

Neither editor has a `mode` concept any more. Duplicating a catalog (`useCatalogMutations`'
`create`, called directly with a duplicate payload) and duplicating a collection
(`useCollectionMutations`'s `duplicate`) are both single atomic server calls that hand back a
finished copy, not a pre-filled form the editor saves to create one — see "Catalog authoring" and
"Collection authoring" below. Every row either editor opens is always a real one.

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
- **`apiFetch`** (`web/src/api/client.ts`) attaches the bearer header to every `/api/*` call,
  catches `401`, refreshes once, retries the original request, and only then clears session
  state and navigates to `/login` via the `router` singleton exported from
  `web/src/routes/router.tsx`.
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
`POST /api/profiles/select` → navigate to `/configure` with `{profileIndex, profileName}` as
React Router **navigation state**, not a URL param.

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

## `/configure` — Workspace and Community

`web/src/routes/Builder.tsx` renders one top-level `Segmented` switch, Workspace / Community
(its own row below `lg`, matching DESIGN.md's two-row phone top bar) — everything below is one
of the two. Push, the pending indicator, and the profile chip sit in the header outside both:
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
| **Library** — left rail | Every catalog and collection you own, one labelled "Mine" group. One search box at the top of the rail narrows both the catalog and collection lists at once. Compact rows: name plus a one-line recipe summary. The closed-graph sharing model means this is the whole library — taking someone else's public catalog or collection is the separate Community tab's job, not a second group in this rail |
| **Pane** — right | One thing at a time: your home screen (with a `List \| Preview` switch), or the editor for whichever rail row is selected. The page's centre of gravity |
| **Push** — header | Global action, beside a persistent unpushed-changes indicator. Not a section — it's the commit for Home, so it lives where Home is always visible, whether or not Home is the pane's current occupant |

**Authoring replaces the pane's occupant rather than opening over it.** Picking a rail row to
edit swaps Home out for that row's editor; leaving the editor swaps Home back. `Workspace`
(`web/src/features/builder/Workspace.tsx`) owns that one-occupant rule, and `EditorTarget`
(`web/src/features/builder/target.ts`) is the union describing what the pane currently holds.

**Below `lg` the two regions stack into one scrolling document**, not two screens —
`web/src/features/builder/stacked.ts` owns the threshold (`matchMedia` against
`--breakpoint-lg`), the scroll requests that move focus between rail and pane, and the published
header height the sticky offsets need. The breakpoint value is asserted explicitly as
`--breakpoint-lg` in `web/src/index.css` and mirrored as `LG_BREAKPOINT` in
`web/src/lib/breakpoints.ts`, because `stacked.ts` needs the same threshold as a `matchMedia`
query and Tailwind's CSS output doesn't expose it to JS. Change one and you must change the
other; each side carries a comment pointing at its twin.

**Two kinds of unsaved work, deliberately separate.** An editor's changes are saved to the
server; Home's are pushed to the TV. They are lost in different ways, they warn separately, and
the header shows only Home's — an editor states its own inside the pane. `EditorGuard` is what
stops a rail click from discarding an editor mid-edit.

**Vocabulary:** the UI says **"your home screen"**, never "selection". `selection` is the
schema's word and stays in code, types, and endpoint names; it does not appear on screen.
`features/home` is likewise named for what the user sees, not for the tables it writes.

## Library rail

**The library is exactly this profile's own catalogs and collections — `useLibrary`
(`web/src/features/library/useLibrary.ts`) fetches only `GET /api/p/{i}/catalogs` and
`GET /api/p/{i}/collections`.** There is no merge and no `owned` field on `LibraryCatalog`/
`LibraryCollection`: under the closed-graph sharing model, a folder can
only ever reference your own listed catalogs, so "the library" and "what you own" are one set by
construction, not two sets reconciled in the frontend. Browsing and taking someone
else's public rows is the separate Community tab's job, not this rail's (see "Community tab"
below).

All list endpoints are unpaginated; assume small N. Filter and search are **pure client-side
derivations** of `useLibrary`'s dataset, so typing in the rail's search box costs no network call.

Two things established here that everything downstream depends on: **genre lookups are kept
per-kind, never merged** (the movie and tv genre id spaces are separate), and **genre queries are
excluded from the rail's loading/error state** — a failed genre fetch degrades the summary line
to raw ids rather than failing the list, so the rail never blocks on TMDB being reachable.

## Catalog authoring

Create is a three-field form — name, `type`, `is_public` — plus the TMDB params sub-form.
`provider` is derived (`"tmdb"`) and never rendered; `is_default` is excluded (and is `json:"-"`
server-side, so it can't even appear in a fetched row). `type` renders read-only unconditionally —
there is no path, here or anywhere else, that changes an existing row's type — and that's backed
server-side too: `UpdateUserCatalog` reads the stored `type` and rejects a `PUT` that changes it
with `ErrInvalidInput` — a catalog's type is part of the pushed collections blob, so changing it
would alter what Nuvio should have without bumping any collection's `version`. `provider` is
enforced server-side the same way, in both validation places.

**Duplicate is a first-class action on your own rows, not a hidden overflow item, and it's
atomic** — every row in the library is yours, so opening one always edits it; the Duplicate button
(`LibraryItem`'s row actions, or the editor header below `lg`) is the only way to reach it.
`Workspace.tsx`'s `confirmDuplicateCatalog` `POST`s a straight copy of the source's exact type and
params the instant the confirm dialog is accepted (`catalogForm.ts`'s `duplicatePayload` — no form
to fill in first, unlike a bare "New catalog"), then opens the finished copy in this same editor
like any other real row — the same atomic-then-open shape `confirmDuplicateCollection` already
used for collections. Duplicates default to `is_public: false` regardless of source, because
publishing is a deliberate act rather than something inherited from what was duplicated. There is
no longer any way to change a catalog's type: duplicate used to be that path (picking a different
type before the row existed), traded away when duplicate became atomic — a type choice would have
needed its own step ahead of the create firing, and the tradeoff was to drop the capability rather
than add one. (Taking someone else's public catalog is a different action,
`POST /api/p/{i}/community/catalogs/{id}/take` — a server-side deep copy with fresh ids, not this
duplicate flow; its UI is the Community tab, below.)

**Delete's confirm copy states the real consequence under the closed-graph Take model**
(`Workspace.tsx`). A shared catalog's confirm names the actual effect — a taker holds an
independent copy (`taken_from` nulled on this delete), so nobody else's copy is touched; the row
just disappears from the community list for future takers — rather than claiming deletion
"removes it for everyone using it". The unshared-catalog branch states the folder-ref cascade
within this profile.

**Validation rules are enforced structurally where possible**, and this order of preference is
the point:

1. **Structurally.** The date window is one three-way mode (`Any` / `Range` / `Recent`), so
   "both fixed and rolling set" — the thing `Validate()` rejects — is *unrepresentable* rather
   than merely caught. `applyDateMode` (behind `toPayload`) also strips the other type's date
   fields entirely, so a series catalog can never ship `primary_release_date_*` or
   `released_within_days`. Certification is shared, not stripped — movies use theatrical
   ratings, series use TV content ratings, both scoped by `certification_country`.
2. **Grouped controls** for the two required-together pairs: picking an age rating defaults its
   country, picking a streaming service defaults the region, and clearing one clears the other.
3. **Mirrored checks** (`web/src/features/catalogs/catalogForm.ts`) as the per-field backstop.
4. **The server's plain-text 400** as an unexpected-case banner only.

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
  still in the payload.
- **Age rating is the same shape**, over `GET /api/certifications/{type}` — options come from
  TMDB per type because the scales differ, and picking a rating defaults its country.
- **The editor previews on request, not as you type** (`RecipePreview` → `useRecipeTiles` →
  `POST /api/catalogs/preview`). The recipe changes on every keystroke, so fetching live would
  mean a request per character and tiles that never settle long enough to read; an explicit Run
  button is one call per deliberate act. The block is one TMDB page — the same page the row
  itself is, not a sample of it — except for a shuffling recipe, which is labelled as such.
  Every tile links to its TMDB page, which is where "what *is* that one?" gets answered.
- **Deleting a catalog also drops it from the pending home screen** — the row is gone for
  everyone, so leaving a reference behind would only fail at Push. Deletion confirm copy says
  "removes for everyone using it", not "are you sure".
- **Creating a catalog does not select it.** Auto-adding would silently increment the
  unpushed-changes count as a side effect of a save that already succeeded, blurring the two
  persistence models the page has to keep legible: authoring writes immediately, selection is
  pending until Push. Adding from the rail defaults `show_in_home: true`.
- **Writes invalidate the owned catalog list** (`useCatalogMutations` also invalidates the
  community query keys, which the Community tab now subscribes to — an `is_public` flip changes
  what that tab shows) but deliberately **not** the selection queries, which would clobber
  pending edits.
- **`useCatalogMutations` also invalidates the collection lists.** Not defensive — required.
  `DELETE FROM catalogs` cascades `folder_catalogs`, so a cached collection tree keeps a phantom
  ref: the overlay lists a folder member that no longer exists, and saving that collection
  `400`s.
- **A scoped catalog's "Sharing" row becomes a "Scope" row** (`CatalogFormState.collectionID`):
  it can't be shared while scoped (the schema's own CHECK), so the Switch is replaced by a note
  and a "Move to library" button that clears `collectionID` — promote, always allowed, applied on
  the next Save like any other field. This editor never offers the other direction (demote):
  it's only ever opened on a scoped row from inside `CollectionEditor`, which is where "copy into
  this collection" and "new inside this collection" already cover getting one scoped in the first
  place.

## Home pane — List view

Add/remove from the rail, drag/keyboard/↑↓-reorder, `show_in_home` per catalog row. Hydrates once
from both `GET .../selection` endpoints; client state only, nothing writes until Push.

- **The baseline is snapshotted at hydration, not read live from the query cache.** A background
  refetch must not move the baseline under the user and silently change the diff.
- **Selected rows render from the selection response, not by library lookup** — a selected row
  can be deleted after selection, and the selection response still returns it until the next
  push clears it from the TV. Rendering by library lookup would make those rows vanish from
  the page while still being live on the user's TV. Such rows are marked "not in library": they
  work, but removing them is one-way.
- **List groups in the same three TV bands Preview draws** (`preview.ts`'s `buildHomePreview`, not
  a separate derivation): pinned collections, then home-shown catalogs, then unpinned collections,
  numbered with one ordinal straight through all three — a row's number is its place on the TV. A
  reorder (drag, keyboard, or a row's own ↑/↓) only ever moves a row within its own band;
  `HomeSelectionContext`'s `reorderCollections`/`reorderCatalogs`/`moveCollection`/`moveCatalog`
  reconstruct the *complete* underlying list on every edit (`pending.ts`'s `reorderWithinBand` /
  `moveWithinBand`) rather than replacing just the touched band, so the untouched band can't
  silently relocate to the array's tail and register as a phantom pending change.
- **`show_in_home` toggles whether the catalog gets a home row** — off keeps it in Discover only,
  via a required `genre` extra `buildManifest` adds to that catalog's manifest entry. Off-catalogs
  render outside the numbered bands entirely, in their own "Not on home" tray (no drag, no
  ordinal — they have no place in the TV's order); the flip itself is a row's ⋯ menu
  ("Move to Discover" / the tray's "Move to home"), not a dedicated toggle control.
- **The pending count is the list of changes' length, not a separate tally.** `changes.ts`'s
  `computeHomeChanges` diffs `baseline` against `current` into named, per-row sentences ("Moved
  “X” from 5th to 3rd"), using a longest-increasing-subsequence pass per band so a drag reports
  only the row that actually moved. `HomeSelectionContext.pendingCount` is `changes.length`; the
  header's pending indicator and the navigation guard's dialog both read it, and the indicator
  doubles as the toggle that opens the list itself (`ChangesStrip` in `PushControls.tsx`) — the
  count and the sentences behind it must never disagree, which is why there is only one number.
- **A collection already on the TV can itself be a pending change**, with no selection edit at
  all: `computeHomeChanges` adds a line for any collection present in *both* `baseline.collections`
  and `current.collections` whose `version !== pushed_version` — a save to a collection's folders
  changes the derived manifest immediately, but Nuvio's own folder sources stay stale until the next
  push (the "Save-to-Push window", accepted rather than closed). `version`
  is an integer bumped on every content write and `pushed_version` is the version push actually
  read and sent, so the compare is exact rather than a clock — an earlier timestamp-based version
  missed a Save landing inside the same second as a push, or between push's read and its local write, and
  the integer version has no such gap. Restricted to collections in both sets: a collection taken
  off the TV, pushed, edited, then put back would otherwise show both "Added …" and "changed
  since …" for the same collection; only "Added …" should fire.

**Selection is client state until Push, and the one thing enforcing that is the one-shot
hydration guard** in `web/src/features/home/HomeSelectionContext.tsx`
(`if (current !== null || !selectionLoaded) return`). The mutation hooks' invalidation of
`['p', i, 'catalogs']` is a *prefix* of the selection key `['p', i, 'catalogs', 'selection']`,
and `invalidateQueries` matches by prefix — so a selection refetch does fire on every catalog
write; it just can't move `baseline` or `current`. Removing the guard, or making hydration re-run
on fresh data, silently clobbers the user's pending home-screen edits on the next catalog or
collection write.

**`HomeSelectionContext`'s `genres` passthrough is load-bearing, not a redundant re-export.**
`HomePane` is rendered as `<HomePane />` with no props, so it has no `profileIndex` to call
`useLibrary` with itself.

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
(`useUnloadGuard`) and a navigation confirm both fire with a pending count, which is the
mitigation. Mirroring pending selection into `localStorage` is cheap but adds a "your local
state disagrees with the server" case to handle on next load; not planned.

## Home pane — Preview view

A pure render of state List already holds — no endpoint and no fetch for *layout*; real tiles
come from `POST /api/catalogs/preview`. The derivation lives in
`web/src/features/home/preview.ts`, kept pure and separate from `HomePreview.tsx`.

**The model being previewed.** *Home* is **one page** in three bands:

```
pinned collection rows      pin_to_top hoists above everything
catalog rows                tiles = content
unpinned collection rows    tiles = folders
```

A collection is **one row whose tiles are its folders**, drawn from folder metadata
(`cover_emoji`, `cover_image_url`, `title`) at each folder's own `tile_shape`. No TMDB content is
rendered for a collection on home. Clicking a folder tile opens a **folder page** scoped to
*that one folder*; sibling folders are not on it. `view_mode` is a collection-level setting
applied to every folder in it, and it governs this page only:

| `view_mode` | Folder page |
| --- | --- |
| `TABBED_GRID` | One tab per **catalog in the folder** (plus "All" when `show_all_tab`), over a grid of that catalog's content |
| `ROWS` | One row per catalog in the folder, stacked — the same shape home uses |
| `FOLLOW_LAYOUT` / unknown | Falls back to `ROWS`, labelled on screen as a guess |

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
- **"Not in library" and "nothing resolves" are two different conditions here**, and conflating
  them is a real bug. `catalogById`/`collectionById` are assembled from the selection response
  *as well as* the library, so a row deleted after selection still resolves through the selection
  response even once it's gone from the library. Per DESIGN.md's Clean Preview spec (its ninth
  amendment, `web/_incoming-design/DESIGN.md`), `isDetached` marks nothing *inside* the TV
  frame — the real TV shows a detached row plainly, with no note pinned onto it — but the
  Discover-only list beneath the frame still names it, in List's own wording, because that list is
  Uno's own words about the picture, not the picture itself. Unresolvability still degrades a row
  inside the frame to an empty strip, no explanation, matching how the TV would show it.
- **Slack wire values are handled, not cast away.** `tile_shape` can be `''` (falls back to
  `POSTER`, and says so on screen; Nuvio does the same with a pushed `''` — see
  `docs/data-model.md`); `view_mode` is a bare `string`, so `FOLLOW_LAYOUT` and
  anything unrecognised land in one branch that admits it's guessing.
- **Selection order is preserved within each band**, so pinning moves a row between bands
  without discarding the order the user just dragged.
- **`ListState` owns loading and error for both views; each view owns its own empty case.**
  List's empty is an instruction to go add something; Preview's is the colour-bars moment from
  the design section below.

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
Clean Preview spec and the owner's instruction that Uno pin nothing onto the TV picture anywhere:

- **Merged catalogs.** Nuvio merges N catalogs into one view and **its merge rule is
  unspecified** — folders aren't an addon concept, so nothing in the addon protocol or Nuvio's
  docs specifies the order, and Uno cannot derive it. This applies to **exactly one view**: the
  `show_all_tab` "All" tab on a `TABBED_GRID` folder page (`interleaveTiles` in
  `features/preview/model.ts`, still shared with the collection editor's own preview panel, which
  keeps its own caveat text — Clean Preview's no-pinned-notes rule applies only inside the TV
  frame). The TV's own "All" tab just shows the merged tiles, with no caption saying the order is
  a guess.
- **`randomized` catalogs** take a random TMDB page per call on both the addon path and the
  preview, independently, so the TV preview genuinely won't match the TV. Not flagged inline; the
  placeholder-tone tile behind a poster is the only visual difference, and it isn't specific to
  this case.

**The folder page's back arrow lives inside the TV frame, beside the folder's own title**, per
DESIGN.md's Back Like the Remote spec, not as a button in Uno's own chrome above it. Escape and the
browser's own Back do the same thing. `HomePreview.tsx`'s `useFolderPage` hook owns this: opening
a folder pushes one `history.pushState({unoFolder: true}, '')` entry (a `try`/`catch` — a
sandboxed frame can throw, and Escape/the arrow still work without it, only the browser's own
Back doesn't); a `popstate` listener closes the folder when that entry is popped. Leaving any
other way — the arrow, Escape, the target becoming unresolvable, or this view unmounting entirely
(the List | Preview switch, or an editor opening) — consumes the pushed entry with one more
`history.back()` rather than leaving it to `popstate`, so a later physical Back press never lands
on a dead entry nobody is listening for.

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
  (`web/src/features/collections/FolderCard.tsx`). This is not optional. `closestCenter` ranks
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
  (`useCollectionMutations`'s `duplicate`) reuses `TakeCollection`'s own tree-copy logic
  server-side: every folder ref survives, a listed source catalog stays a reference, and each
  distinct catalog scoped to the source collection becomes a fresh scoped copy in the new one.
  There is no client-side seed-and-drop step any more — a collection's scoped catalogs can't be
  represented on the client without fetching them, so the copy has to happen server-side
  regardless, which is what makes every scoped ref survive a duplicate (the client's library only
  ever holds listed catalogs, so a client-built clone could only ever seed listed ones).
  `Workspace.tsx`'s `confirmDuplicateCollection` calls the mutation, then
  opens the finished copy straight into its own editor for review — `EditorTarget`'s collection
  variant carries an `initialCatalogs` override for this, since the fresh copy's scoped rows may
  not have reached a `library.collections` refetch yet. `CollectionEditor` itself has no
  `'duplicate'` mode any more — every collection it opens is editing a real row.
- **A catalog can be in one folder more than once, never twice under the same genre.** The
  (catalog, genre) pair is `folder_catalogs`' primary key. Form refs are
  `FolderRefState {key, catalogID, genre}`, and every per-ref action, drag id and React key uses
  the session-local `key`, because neither the catalog id nor the pair stays put while the genre
  is being edited. The picker adds an unfiltered ref, so it omits a catalog that already has one
  here (`addRef` guards too). A row's ⋯ "Add another genre" inserts a second ref to the same
  catalog directly below it, under the first genre option not already taken. The row's genre
  select leaves out genres its siblings on the same catalog already use. The validator's
  "same catalog with the same genre twice" backstops all of that, mirroring
  `CollectionForm.Validate`.
- **Removing a folder is a standing warning, not a confirm.** Omitting a folder from the payload
  deletes it server-side and cascades its refs — but nothing commits until Save, so
  `removedFolders(initial, current)` names exactly which folders the next save would destroy,
  shown above the folder list as soon as any exist. A confirm would ask the user to approve
  something that hasn't happened. Dropping a folder that was never saved is correctly silent.
  "Undo" reinserts the removed rows verbatim — they still carry their original form
  `key`, which is what makes putting them straight back into `state.folders` safe. Its body text
  is unconditional ("Copies others have taken keep theirs"),
  matching the closed-graph Take model: a taker holds an independent copy, so removing a folder
  from your own collection never reaches theirs regardless of sharing. `Workspace.tsx`'s delete
  confirms state the same fact.
- **The save bar's quiet button reads "Discard changes" here, "Cancel" in the catalog editor**
  (`EditorFooter`'s `cancelLabel`) — DESIGN.md's own wording for the heavier thing this editor can
  lose. Both route through the same call, this editor's own `onRequestClose`, and from there
  through the one shared `EditorGuard` confirm every exit from a dirty editor already goes
  through — there is no second, folder-aware confirm layered on top of it.
- **`tile_shape: ''` and an unknown `view_mode` are handled differently, and both are honest.**
  `''` is a value the server accepts and Preview reads as an assumed poster, so it's kept as its
  own "Default" option rather than normalised — a round trip through this form must not silently
  rewrite stored data. An unrecognised `view_mode` can't be shown at all, so it reads as unset;
  there is no third option.
- **`cover_emoji` is a short text input** (`maxLength` 8, since an emoji can be several
  codepoints), not a picker — a bundled emoji picker is a large dependency for a field every
  keyboard already has an input method for.
- **Ordering is array position.** No `sort_order` field in the form; drag order *is* the value.
  Folders are reordered by dragging a tile's corner grip (`rectSortingStrategy`, since the strip
  wraps) or with the selected folder's ←/→ (`RowIconButton`, `moveByOne` — the same shared pieces
  in `components/dnd.tsx` the Home pane's rows use). A folder's own catalogs are running-order
  rows (`.run-row`) that number in plain figures ("1", "2") and carry only the grip inline — see
  the "⋯" note below.
- **Folders are a tile strip, the way the TV draws them.** Under the "Folders" `.cr.is-head` row
  (holding "Add folder"), `FolderTiles` draws each folder at its own `tile_shape` with its cover,
  name and catalog count — the editor's list and the "On your TV" row are the same picture. One
  folder is always selected (the one picked, else the first), and `FolderDetail` shows it below
  the strip as one raised panel: a heading with its name, "1st of 2" and an appearance summary,
  ←/→ and Remove; then its title, then its catalogs (a `.cr.is-head` row with "New catalog" and
  "Add catalogs"), then an "Appearance" `.sec-head` that folds away hide-title, tile shape,
  cover, the focus GIF (URL plus an on/off) and the three Modern Home hero URLs (backdrop, video,
  title logo). Preview renders none of the focus or hero fields; they only reach the TV through
  push. Catalogs come before appearance because they're what a folder is opened for. Inside the
  panel the credit grid narrows to a 128px role column, so the folder's settings read as inside
  the folder rather than as more of the collection's; fields drop to `ground` so they don't
  vanish into the `raised` panel. A failed Save selects the first folder with errors, and every
  other folder with errors shows a danger triangle on its tile. Anything without a role of its
  own — the empty state, the picker, errors — takes `.cr-indent`. Below 640px the panel's grid
  collapses the same way `.cr` does; on touch the tile grip is always visible (there's no hover
  to reveal it) and the heading's ←/→/Remove spread apart so their 44px `.tap` boxes don't
  overlap.
- **The catalog-ref picker is inline under the Catalogs head**, not a dialog — a scrim would hide the
  folder being filled. It stays open across picks and drops each chosen row out of the list, so
  what remains is always exactly what can still be added.
- **Three sources for a folder's catalog:** the picker's plus icon **links** a listed catalog — a live pointer,
  edits reach every folder that references it — and, once this collection has been saved at
  least once, a second icon **copies** the same row into a fresh catalog scoped to this
  collection alone, which the original can't drift. A third button, beside "Add catalogs",
  starts a catalog **new inside this collection**: named first (the same two-step the library's
  own "New catalog" uses), then opened in the nested editor below to fill its filters. The last
  two are hidden with a hint to save first when the collection doesn't have a server id yet — a
  scoped catalog needs a real collection row to scope to.
  **Copy and new-inside-this-collection are staged locally, not written until Save.** Both used
  to `POST /api/catalogs` immediately on click, independent of the collection's own Save — which
  meant discarding the edit instead of saving it left the row behind forever (nothing in the
  discard path, or anywhere outside `UpdateUserCollection`'s own next Save, ever cleaned it up).
  Now the click stages a synthetic `Catalog` client-side, keyed by a `draft:` id sentinel
  (`CollectionEditor.tsx`'s `draftCatalog`), in the same `localCatalogs` registry a real scoped
  catalog lives in — nothing downstream of that registry needs to tell a draft apart from a real
  row to render it. `collectionForm.ts`'s `toCollectionPayload` resolves every `draft:` id into an
  inline `FolderCatalogRef.New` spec right before the collection's own Save reaches the wire,
  keyed by that draft id, so every ref to one draft (two genre rows, or two folders) resolves to
  the one catalog Save creates rather than one catalog each;
  `internal/vault/collections.go`'s `resolveFolderCatalogRef` is the one place a `New` entry is
  ever written — inside `CreateUserCollection`/`UpdateUserCollection`'s own transaction, atomic
  with the folder write that references it. So Save creates the catalog and the ref together in
  one commit, and discarding instead of saving never wrote anything in the first place, closing
  the leak structurally rather than by adding a cleanup step. The nested editor for a draft (below)
  edits this local object directly — no network call — until the collection's own Save resolves it.
- **A folder-catalog row's quiet Edit is scoped-catalog only**, DESIGN.md's own spec for the
  affordance. It opens the referenced catalog one level down: for a scoped catalog (real or a
  session's own draft) that's unambiguous, since nothing else can reference it. A *listed* catalog
  has no inline Edit here at all — it's a live pointer, and editing it from inside a collection
  used to silently reach every other folder and the library too, which read as a surprise rather
  than a feature. The row instead says how many places it's used (home screen plus every folder
  across every owned collection, `Workspace`'s `usedInPlaces`) and offers "Copy into this
  collection", which replaces just this ref with a fresh scoped copy (itself now staged, per
  above) in place rather than adding a second reference. Editing a listed catalog directly is the
  library rail's job. "One level down" is a `Modal` layered over this editor, not a second pane —
  the builder's pane holds one occupant (see Library rail, above), so a second real editor has to
  be a modal rather than a stack. `CollectionEditor` stays mounted underneath it, so this editor's
  own unsaved folder edits survive the round trip; the modal resets `--app-h` to `0` locally so the
  nested `CatalogEditor`'s sticky header doesn't try to clear the outer app header's height a
  second time. The modal's own catalog lookup reads `localCatalogs` only — every scoped catalog
  (real or draft) this editor can open here is already in it by construction, so there is no
  library fallback to reach for.
- **A catalog row shows one inline action — Edit for a scoped catalog, Remove for an unavailable
  one, nothing for a listed one — everything else is behind "⋯"**: "Add another genre" (disabled
  until the genre options land, or once every one is taken), "Copy into this collection" (listed
  catalogs only), Move up/down, and "Remove from folder". The grip reorders by pointer,
  touch and keyboard (`useDragSensors`' `KeyboardSensor`), so the menu's moves are the fallback,
  and the name keeps the row's width at phone size.
- **Each catalog row has a genre select under its recipe line** (`RefGenrePicker`), narrowing that
  one reference: "All genres", or "Only Western" and so on. Its options come from
  `POST /api/catalogs/genre-options` for the catalog's own recipe, not the whole TMDB list, so
  every choice actually narrows the row. It is keyed on the recipe, so editing a scoped catalog's
  filters in the nested editor refreshes them. A stored genre that the recipe no longer allows is
  kept, labelled "(no longer applies)", with a danger note saying the TV shows that row
  unfiltered. The genre lives on the ref, so removing a ref takes its genre with it, and
  "Copy into this collection" swaps only that ref's catalog for the copy, keeping its genre.
  The options query lives in `RefRow` (`useGenreOptions`), so the picker and "Add another
  genre" share one list. The "On your TV" panel and Home's folder pages fetch each source's
  tiles with its genre (`queryKeys.catalogPreview` includes it), so they show the filtered row
  the TV will.
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
- **Delete's confirm copy (`Workspace.tsx`) states the real consequence:** no
  claim that removing a shared collection reaches "everyone using it" — a taker's copy is
  independent — and "the catalogs referenced here are kept" is qualified by
  `scopedCatalogCount`: it names how many of the collection's own scoped catalogs (which have no
  life outside it) go with it, distinct from any listed catalog it merely references and which
  survives.
- **Sharing is the shared `Switch` component here too**, same as the catalog editor, its label
  saying what shares along with the collection: "along with every
  catalog inside it" — sharing a collection shares the recipes inside it, so the switch has
  to say so. Show first and the "All" tab are `Segmented`, the latter greyed (DESIGN.md's
  "Greyed" segmented state, `Segmented`'s `disabled` prop) rather than hidden while the view mode
  isn't Tabbed Grids, keeping its value for when it switches back.
- **The "On your TV" panel is a working TV, docked beside the form.** The collection editor has
  its own layout (`.ed.ed-tv`, `EditorShell`'s `docked="tv"`, capped at `--w-editor-tv`): the form keeps
  its `--w-form` column and the TV takes `clamp(380px, 42cqw, 620px)` beside it, undocking under
  the form below 960px of pane. It draws the live draft with the Home Preview's own components
  (`TVCollectionRow`, `TVFolderPage` from `features/home/tv.tsx`), so a folder tile opens the same
  folder page Home does — tabs or rows per `view_mode`, real titles per catalog. `.tv-crop` sizes
  every `cqw` against a stage at least 800px wide, so tiles match the Home Preview's size in any
  column width and the frame shows a crop: rows bleed off its edge and scroll, a folder grid
  reflows to it. Tiles come from each source's recipe (`useRecipesTiles`), not its id, so an
  unsaved draft and a Community collection both preview, and only the open folder's catalogs are
  fetched. The back arrow and Escape return to the row — Escape is stopped inside the screen so it
  never reaches the editor's own Escape-to-close — and there is no history entry.

## Community tab

`web/src/features/community/` — everyone else's public catalogs and collections, browsed and
copied rather than referenced. The closed-graph model's only path across an owner boundary:
a folder can only ever reference your own listed catalogs (see
"Library rail" above), so this tab never lets you *use* another owner's row live, only Take a
private copy of it.

- **`CommunityView.tsx`** owns the Catalogs / Collections `Segmented`, a name-only search field,
  and a Name / Newest `Select` — pure client-side filtering and sorting over
  `useCommunityCatalogs`/`useCommunityCollections` (`useCommunity.ts`), which are thin
  `useQuery` wrappers over the community endpoints, keyed the same way the library's queries are
  (`queryKeys.communityCatalogs`/`communityCollections`, both under the `['p', i, …]` prefix).
  It also calls `useLibrary` for its genre lookups only — the same query key `Workspace` and
  `HomeSelectionContext` already hold open, so this is a third subscriber to cached data, not a
  third network round trip.
- **No author, no handle, no "copied from" line anywhere here** —
  that provenance is retired, not merely hidden. `taken_from` exists in the schema only so a
  profile's own copies can answer "you already took this," which surfaces here as a plain
  "✓ Taken" mark beside the row's Take button, from the server's own `taken` field. Taking again
  is allowed and makes another copy, so the button never disables once a row is taken — mirroring
  the backend's own Take semantics.
- **Preview reuses the editors' own preview components, not a new one.** A catalog row's Preview
  mounts `CommunityCatalogPreview`, which is `RecipePreview` run over the row's own stored
  `type`/`params` via `useRecipeTiles` — the same on-request, one-TMDB-page component the catalog
  editor's results panel uses, never `invalid` since a community row is always a saved catalog the
  server already accepted. A collection row's Preview mounts `CommunityCollectionPreview`, which
  is the collection editor's own "On your TV" panel (`CollectionPreview`) fed from
  `buildRefOptions`/`formFromCollection` over the row's own `catalogs` array rather than the
  library — the community row already carries every catalog its folders reference, listed or
  scoped on the source side, so nothing resolves as unavailable the way a library-sourced ref
  picker's accessible set would for someone else's catalog.
- **Take invalidates both the library and the community lists** (`useCommunityMutations.ts`) —
  the taken copy has to appear in "Mine" and the source row's `taken` flag has to flip, both from
  one mutation. A small auto-dismissing strip (2.5s) reports "Added to your catalogs" /
  "Added to your collections"; unlike the push outcome strip this never needs a decision, so
  nothing about it persists past being read.
- **The tab switch is a `Segmented` in `Builder.tsx`'s header**, not a route — `/configure` stays
  one URL. Above `lg` it sits inline in the header row; below `lg` it drops to its own row
  underneath, because the header row's height is measured to fit exactly what it holds at phone
  width and a third control has no room there (see "`/configure` — Workspace and Community"
  above for the guard this goes through).

## Push UI

Header button beside the pending indicator. **One call**, one indeterminate in-flight state — no
staged progress, because it is one HTTP call from the frontend's perspective and animating
through fake stages ("Saving…", "Installing addon…") would be fabricated.

**Two failure states, not four:**

- **Ordinary failure** (bad input, Nuvio unreachable, either push rejected) — *nothing changed*,
  full stop, catalog selection included. One generic message: "Push failed — nothing changed.
  Your edits are still here; try again." True for every ordinary failure mode because of the
  backend ordering.
- **Rare compound failure** (`undo_failed`) — both Nuvio calls succeeded, the local commit
  failed, *and* the compensating undo also failed. The one case where "nothing changed" isn't
  true. Copy: "Push failed, and we couldn't fully undo it — your collections in Nuvio may be
  temporarily out of sync. Push again to reconcile."

- **The Push button stays enabled with zero pending edits.** It is the only recovery path from
  that compound-failure case, and nothing else marks that state — graying it out on
  `isDirty === false` is an obvious-looking cleanup that quietly removes the recovery path.
- **A second push is blocked while one is in flight**, via a **ref**, not render-captured
  state — the guard has to reject a second call raised before a re-render. Two overlapping
  `PullCollections`→`PushCollections` cycles can clobber each other.
- **`markPushed` takes the pushed state, not `current`.** The user can keep editing while a push
  is in flight; advancing the baseline to "whatever is current now" would silently swallow those
  edits and report them as already live.
- **Selection queries are invalidated on success.** With `staleTime: 30_000`, a successful push
  otherwise leaves them holding pre-push data, so switching profile and returning inside that
  window re-hydrates the baseline from stale data and makes the pushed changes look undone.
- **The owned-collections query is invalidated on success too** (found in a
  dev-loop check). `pushed_version` lives on the `Collection` row from *both* `queryKeys.ownedCollections` and
  `queryKeys.collectionSelection`, and `HomeSelectionContext`'s `collectionById` map is built by
  writing the selection response first and the owned list second — so on an id present in both
  (the ordinary case: a collection that's both owned and currently selected), the owned list's
  copy always wins. Invalidating only the selection query left `collectionById` holding the
  owned list's pre-push `pushed_version` forever, so the "changed since it was last pushed"
  line (`computeHomeChanges`, Home pane section above) never cleared after a successful push —
  it looked like every push silently failed to update anything.
- **`ApiError` carries an optional `body`** (`web/src/api/http.ts`), best-effort JSON-parsed
  from the text it already reads on every non-2xx. Without it, push's structured failure arrives
  as an `ApiError` whose `message` is the raw JSON blob — unusable, and worse, renderable
  straight into an error UI. No other call site reads `.body`; it exists for this one endpoint.
- The manifest URL comes from `POST /api/profiles/select`, so it's shown as a header copy button
  from profile selection onward rather than waiting on a first push.

## Cross-cutting client rules

- `401` from any call → refresh + retry, then bounce to login.
- `404` from any `/api/p/{i}/...` route → profile not selected → send back to the picker
  (`ProfileNotSelectedError`, `web/src/api/http.ts`).
- All list endpoints are unpaginated; assume small N.

## Visual direction

Dark-only, single-theme, deliberate: every reference client (Stremio, Nuvio, Plex, Jellyfin) is
dark, and this app configures a TV. Nothing here is a stock default.

**The constraint that drives it:** in a *list* there is no artwork to carry the UI, so the
**summary line is the content**, and the list is designed to make standing queries scannable
rather than to frame posters. The rail, both editors, and the List view stay artwork-free by
choice. Preview is the one place posters land, which makes the `List | Preview` switch a payoff
rather than a mode change.

**Palette — derived from SMPTE colour bars**, desaturated into a working set. Television's own
artifact for "nothing to show", which is exactly this page's condition. Near-black ground
(`#0B0B0C`); two working hues — cyan `#4EA8B8` for movie, amber `#C39A3E` for series — violet
`#8E7BC4` for collections, red `#E0594E` reserved for destructive. Tokens live in
`web/src/index.css` as `--uno-*` custom properties, exposed to Tailwind via `@theme inline`.

**Type — one family everywhere except the TV screen.** Jost Variable, at two weights (400 body,
500 label/button/heading) via its `wght` axis, carries every word Uno says, including recipes,
ids, and numbers — there is no separate mono face. Roboto is the TV's own face, the Two Faces
Rule, loaded only inside `.tv-screen`, never elsewhere. `@fontsource-variable/jost/wght.css` and
`@fontsource/roboto` ship both. Fontsource packages, not
the Google Fonts CDN, because the build is `go:embed`'d and must not carry a runtime font
dependency.

**Signature — the type bar.** Every row carries a 3px full-height colour bar at its left edge,
hue by type. Read down a list, the column of bars is a test pattern. Applies everywhere a
catalog or collection is listed. It is `aria-hidden`, so anywhere ownership matters the row
states it in text too — every row in the library and its editors is the caller's own, so there
is no ownership distinction left to encode in the bar itself.

**The one loud moment: an empty home screen renders full-saturation colour bars** with a caption
slug across them. It is the only place the `--smpte-*` tokens fire at full amplitude, it is
literally correct (no content, so the screen shows bars), and it is first-run-only in practice.
Everything else stays quiet so it lands. In the Preview view only — List's empty state stays a
plain instruction, because there the user is being told what to do next rather than shown what
their screen looks like.

**Motion — one orchestrated moment.** Type bars scale in staggered on mount, the rundown signing
on. Everything else is 120ms hover/focus. `prefers-reduced-motion` respected.

**Copy — written for someone who has never heard of TMDB, Stremio, or an addon manifest.** The
user signs into Nuvio and builds rows for their TV; that is the whole of what they are assumed
to know. Three rules, in order:

1. **Prefer a control that needs no explanation to a control plus a sentence.** A hint that
   exists to compensate for an unreadable input is a bug report about the input. The
   watch-provider field is a named, searchable picker and carries no hint text at all, rather
   than a box for TMDB's numeric ids beside "find IDs in the TMDB documentation". `vote_count`
   reads "Number of ratings" and needs no hint either.
2. **Never print the provider's vocabulary.** `describeRecipe`
   (`web/src/features/library/recipe.ts`) returns plain English — "Most popular · Action and
   Comedy · rated 7.0+ · in Japanese", not `popularity.desc · Action + Comedy · ★7.0+ · lang
   ja`. Language, country and region codes resolve through `Intl.DisplayNames`. This is the
   app's highest-volume text: it renders on every Home row and every row of the folder picker.
3. **Say what the user sees, not why the code can't do better.** "Shown as rows — the app
   decides the real layout", not "Uno can't read the app's layout setting". Errors name the
   failure and the next action without naming the machinery: "Couldn't load the preview. Your
   filters are fine — try again."

**`InfoTip` is not a place to move hints to.** A hint the user needs *before* filling a field in
stays on the page as a `FieldNote`; one that doesn't survive "is this needed at all" is deleted.
The icon is for the narrow middle — a control genuinely worth a sentence, in a grid where that
sentence would push its neighbours down a line. Three fields pass a `tip`: the age-rating
control in `CatalogEditor`, and view mode and backdrop image in `CollectionEditor`. Adding a
tenth means rule 1 is being skipped.
