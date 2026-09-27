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
and the Home list's strips draw. The preview rows — the Home pane's Preview tab, the collection
editor's Preview panel and Community's collection preview — are `web/src/features/home/tv.tsx`:
the TV's layout (scrolling rows, captions, folder tiles that open folder pages) in the same Figtree
and tonal steps as the rest of the app, with no note pinned between the rows (see "Home pane —
Preview view" below). Nothing under `features/` is a separate URL; `Builder`
composes all of it.

Every row either editor opens is a saved one, and `EditorTarget`
(`web/src/features/builder/target.ts`) always carries its `id`. A catalog or collection is named
into existence before its editor opens, and duplicating a catalog (`useCatalogMutations`'
`create`, called directly with a duplicate payload) or a collection (`useCollectionMutations`'s
`duplicate`) is a single server call that hands back a finished copy — see "Catalog authoring"
and "Collection authoring" below. An open editor reads only its `update` mutation's pending state
and error: `create` belongs to the naming dialogs and to Duplicate, which can run while an editor
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
- **`apiFetch`** (`web/src/api/client.ts`) attaches the bearer header to every `/api/*` call,
  catches `401`, refreshes once, retries the original request, and only then clears session
  state and navigates to `/login` via the `router` singleton exported from
  `web/src/routes/router.tsx`.
- **The query cache is per account.** No query key carries the user, so
  `web/src/lib/query-client.ts` subscribes to auth state and calls `queryClient.clear()`
  whenever the signed-in user id changes — sign-out, "Switch account", or another tab's
  session for a different user.
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

### Import and export

**Import** and **Export** are `btn-ghost` buttons in the rail's "Mine" header
(`LibrarySection`'s `onImport`/`onExport`). `Workspace` owns both dialogs' open state, as it does
the naming dialogs and the confirms. Neither dialog goes through `EditorGuard`: both open over the
pane without replacing what it holds. Both are `Modal`s, so below `lg` they fill the viewport width
less its padding. The dialogs live in `web/src/features/bundle/`, and the three calls in
`web/src/api/bundle.ts`. The bundle stays `unknown` in TypeScript because the format is defined
by the Go types alone (`docs/data-model.md`, "Bundle format"). Only the `/import/check` answer
(`ImportCheck`, `ImportMatch`) and the import answer (`ImportResult`) are typed.

- **`ExportDialog`** has two checkbox groups, Catalogs and Collections, filled from `useLibrary`'s
  lists, each with **All** and **None**. The row open in the pane starts ticked. A note says
  collections include the catalogs they use, and the server exports those whether or not they are
  ticked. Export is disabled while nothing is ticked. The response is saved as
  `uno-export-YYYY-MM-DD.json` in local time, pretty-printed, through an object URL (`download.ts`).
- **`ImportDialog`** has three steps.
  1. **Pick a file.** A file over 4 MiB is refused before it is read (`MAX_BUNDLE_BYTES`, the twin
     of the server's `maxBundleBodyBytes`), and a `JSON.parse` failure is shown in place. Neither
     sends a request. A 400 or 502 from `/import/check` is shown in place too.
  2. **Review.** The dialog shows the file's counts, then one row per match. Each row is
     **Import a copy** (the default) or the reuse choice. A top-level catalog reads **Skip, I
     already have it**. A collection's own catalog reads **Use my existing one**, with a hint that
     the collection will then share the library catalog. Several existing matches get a `Select`,
     which starts on the first by name. **Copy all** and **Use existing for all** set every row;
     the second picks each row's first match. The choices are `reuse.ts`'s `ReuseChoices`, and
     `reuseMap` turns them into the request's `reuse` map. With no matches, the step shows only
     the counts.
  3. **Import.** The button is disabled while the request is in flight, because a second import
     writes a second set. `useImport` invalidates the two owned lists and settles only once they
     have refetched. The dialog then closes, and a toast under the "Mine" header names what the
     rail gained, for example "Imported 2 catalogs and 1 collection". It counts only new listed
     catalogs and new collections, which is what the import response lists. An import opens
     nothing and adds nothing to home.

The owned-list keys prefix the selection keys, so an import marks those stale too. An import
doesn't change them, so the refetch returns what they already held. The Community keys sit beside
the owned-list keys rather than under them (`['p', i, 'community', …]`), and an import leaves
them alone: nothing it writes is public.

The toast is `components/Toast.tsx` with `components/useToast.ts`, the same auto-dismissing
message the Community tab shows its outcomes in.

## Catalog authoring

Create is a three-field form — name, `type`, `is_public` — plus the TMDB params sub-form.
`provider` is derived (`"tmdb"`) and never rendered. `type` renders
read-only unconditionally — there is no path, here or anywhere else, that changes an existing
row's type — and that's backed server-side too: `UpdateUserCatalog` reads the stored `type` and
rejects a `PUT` that changes it with `ErrInvalidInput` — a catalog's type is part of the pushed
collections blob, so changing it would alter what Nuvio should have without bumping any
collection's `version`. `provider` is enforced server-side the same way, in both validation
places.

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
  `CatalogEditor.tsx`), writing `with_*` and `without_*`. The Leave out picker is the same
  component with `exclude`: no all/any `Segmented`, and always comma-joined, because TMDB drops a
  title carrying any of the listed ids whichever separator is used. Each list has its own 20-id
  cap. Each picker takes the other's ids as `hiddenIds` and never offers them in search, so one
  id can't be both included and left out. There is no server rule for that overlap either, the
  same as genres. The closed head reads "1 studio · not 2 studios" (`sumEntities`), and the
  library summary reads "not from 2 studios" / "not tagged with 1 keyword".
- **A movie catalog's Mode is either Filters or Collection**, a `Segmented` right under the
  Movie/Series row. The mode is form state (`sourceMode` in `catalogForm.ts`), not a stored field:
  `formFromCatalog` reads a saved `with_collection` as Collection, anything else as Filters.
  Filters shows every filter section and no collection picker; Collection shows only the
  collection picker and Shuffle, every other section — sort order included — hidden rather than
  disabled. A collection row lists one TMDB collection's films in release order, and the server
  rejects any other filter beside `with_collection` (`randomized` excepted), so what a save or
  Preview sends follows the mode (both go through `paramsString` → `recipeParams`): Collection
  keeps only the `COLLECTION_KEYS` allow-list, so a field added later is dropped by default, and
  Filters drops `with_collection`. Form state keeps both sides' values, so switching mode back
  and forth loses nothing until Save. `validateForm` checks the sent params, so a dropped field
  raises no error, and Collection mode with nothing picked is an error ("Pick a collection."),
  which also stops Preview from running. Series catalogs have no switch and no collection
  picker, since TMDB has no collections for series.
- **The collection picker is the same server-search picker, single-pick** (`kind="collection"`,
  over `GET /api/collections/{search,{id}}`, stored as `with_collection`). TMDB takes one
  collection id, so the kind's table entry marks it `single`: a pick replaces the chip rather
  than adding one, and there is no all/any toggle. Its section head names the pick rather than
  counting it: the editor reads the same `staleTime: Infinity` by-id key the chip does, so the
  name costs no extra request. "Collection" is also Uno's word for a group of catalogs, so the
  TMDB kind is `TMDBCollection` in code, and the library summary describes a collection row as
  "from a movie collection" (plus "shuffled"), with none of the other filters.
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
- **A collection save that moves a catalog to the library invalidates the catalog list**
  (`useCollectionMutations`' `update`, when any `catalog_edits` entry has `move_to_library`), so
  the moved catalog appears in the Library rail. It is the one collection write that adds a
  listed row; other catalog edits inside a collection change scoped rows the list never holds.
- **`useCatalogMutations` also invalidates the collection lists.** Not defensive — required.
  `DELETE FROM catalogs` cascades `folder_catalogs`, so a cached collection tree keeps a phantom
  ref: the overlay lists a folder member that no longer exists, and saving that collection
  `400`s.
- **A scoped catalog's "Sharing" row becomes a "Scope" row** (`CatalogFormState.collectionID`):
  it can't be shared while scoped (the schema's own CHECK), so the Switch is replaced by a note
  and a "Move to library" button that clears `collectionID` — promote, always allowed, and like
  every edit made in this nested editor it takes effect when the collection is saved (a
  `catalog_edits` entry with `move_to_library`). Once staged, the catalog reads as listed in every
  folder, whose rows offer no Edit to reopen it, so `CollectionEditor` states each staged move as a
  standing note above the folders ("Saving moves … into your library", a neutral `StagedNote`)
  with its own Undo. Undo puts the catalog back in `localCatalogs` with its `collection_id` and re-reads it
  through `withCatalogEdit`, so a move that was the only change leaves the form clean and a rename
  made alongside it survives. This editor never offers the other direction (demote):
  it's only ever opened on a scoped row from inside `CollectionEditor`, which is where "copy into
  this collection" and "new inside this collection" already cover getting one scoped in the first
  place. A draft — a catalog staged inside a collection that hasn't been saved yet — has no row to
  promote, so its nested editor leaves the button out and says to save the collection first.

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

**The provider publishes two contexts.** `useHomeSelection` returns everything, and its value
changes on every edit; `useHomeEdits` returns only the edit functions, which change only with
`isPinned`. `Workspace` reads `useHomeEdits`, so an edit to the home screen re-renders the
components that show the selection — the rail, the Home pane, the header, a collection editor's
"used in N places" rows — and not the workspace and the open editor under it.

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
| `FOLLOW_LAYOUT` | Drawn as tabs with the "All" tab first, the app's own default (`TVFolderPage` in `features/home/tv.tsx`); the caption above the panel reads "Follows the app's layout" |
| `''` / unknown | Drawn as `TABBED_GRID`, which is how every Nuvio client reads it |

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
  response even once it's gone from the library. Per DESIGN.md's Clean Preview Rule,
  `isDetached` marks nothing *inside* the preview
  panel — the real TV shows a detached row plainly, with no note pinned onto it — but the
  Discover-only list beneath the panel still names it, in List's own wording, because that list is
  Uno's own words about the rows, not the rows themselves. Unresolvability still degrades a row
  inside the panel to an empty strip, no explanation, matching how the TV would show it.
- **Slack wire values are handled, not cast away.** An older row's `tile_shape` can be `''`
  (falls back to `POSTER`, and says so on screen; Nuvio does the same with a pushed `''` — see
  `docs/data-model.md`). `view_mode` is a bare `string`: `FOLLOW_LAYOUT` lands in a branch that
  admits it's guessing, and `''` or anything unrecognised is drawn as `TABBED_GRID`, which is
  what Nuvio does with it.
- **Selection order is preserved within each band**, so pinning moves a row between bands
  without discarding the order the user just dragged.
- **`ListState` owns loading and error for both views; each view owns its own empty case.**
  List's empty is an instruction to go add something; Preview's is the colour-bars moment from
  the design section below. The error is only one that leaves nothing to draw — a selection that
  never hydrated — and carries a Retry. A library list that failed is the rail's to report, not
  Home's: the rail shows one error and one Retry for both lists, and leaves out the group whose list
  failed while the one that loaded keeps its rows. While it has, `isDetached` marks nothing, since every row would
  otherwise read as missing from a library that simply hasn't arrived. A background refetch that
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
  same TV components). The TV's own "All" tab just shows the merged tiles, with no caption saying
  the order is a guess.
- **`randomized` catalogs** take a random TMDB page per call on both the addon path and the
  preview, independently, so the preview genuinely won't match the TV. A collection row shuffles
  its film list instead, with the same independent-per-call mismatch. Not flagged inline; the
  blank tile face behind a poster is the only visual difference, and it isn't specific to
  this case.

**The folder page's back arrow lives in the preview panel, beside the folder's own title**, per
DESIGN.md's One Way Back rule, not as a button in Uno's own chrome above it. Escape and the
browser's own Back do the same thing. `HomePreview.tsx`'s `useFolderPage` hook owns this: opening
a folder pushes one `history.pushState({unoFolder: true}, '')` entry (a `try`/`catch` — a
sandboxed frame can throw, and Escape/the arrow still work without it, only the browser's own
Back doesn't); a `popstate` listener closes the folder when that entry is popped. Leaving any
other way — the arrow, Escape, the target becoming unresolvable, or this view unmounting entirely
(the List | Preview switch, or an editor opening) — consumes the pushed entry with one more
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
  (`useCollectionMutations`'s `duplicate`) reuses `TakeCollection`'s own tree-copy logic
  server-side: every folder ref survives, a listed source catalog stays a reference, and each
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
- **`view_mode` and `tile_shape` are the server's enums, with no "unset" option.** The server
  stores an empty value as `TABBED_GRID` or `POSTER`, what every Nuvio client shows for one
  (`docs/data-model.md`), so a new folder starts as Poster and an empty or unrecognised value in
  an older row loads as Tabbed Grids or Poster.
- **`cover_emoji` is a short text input** (`maxLength` 8, since an emoji can be several
  codepoints), not a picker — a bundled emoji picker is a large dependency for a field every
  keyboard already has an input method for.
- **Ordering is array position.** No `sort_order` field in the form; drag order *is* the value.
  Folders are reordered by dragging a tile's corner grip (`rectSortingStrategy`, since the strip
  wraps) or with the selected folder's ←/→ (`RowIconButton` from `components/dnd.tsx` and
  `moveByOne` from `lib/order.ts` — the same shared pieces the Home pane's rows use). A folder's own catalogs are running-order
  rows (`.run-row`) that number in plain figures ("1", "2") and carry only the grip inline — see
  the "⋯" note below.
- **Folders are a tile strip, the way the TV draws them.** Under the "Folders" `.setting.is-head`
  heading
  (holding "Add folder"), `FolderTiles` draws each folder at its own `tile_shape` with its cover,
  name and catalog count — the editor's list and the Preview panel's row are the same picture. One
  folder is always selected (the one picked, else the first), and `FolderDetail` shows it below
  the strip as one raised panel: a heading with its name, "1st of 2" and an appearance summary,
  ←/→ and Remove; then its title, then its catalogs (a `.setting.is-head` heading with "New catalog" and
  "Add catalogs"), then an "Appearance" `.sec-head` that folds away hide-title, tile shape,
  cover, the focus GIF (URL plus an on/off) and the three Modern Home hero URLs (backdrop, video,
  title logo). Preview renders none of the focus or hero fields; they only reach the TV through
  push. Catalogs come before appearance because they're what a folder is opened for. The panel is
  a shelf of its own, so the folder's settings read as inside the folder rather than as more of
  the collection's; its fields and folding sections step to `ground` and `raised-hi` so they
  don't vanish into it. A failed Save selects the first folder with errors, and every other
  folder with errors shows a danger triangle on its tile. On touch the tile grip is always
  visible (there's no hover to reveal it) and the heading's ←/→/Remove spread apart so their
  44px `.tap` boxes don't overlap.
- **The catalog-ref picker is inline under the Catalogs head**, not a dialog — a scrim would hide the
  folder being filled. It stays open across picks and drops each chosen row out of the list, so
  what remains is always exactly what can still be added.
- **Three sources for a folder's catalog:** the picker's plus icon **links** a listed catalog — a live pointer,
  edits reach every folder that references it — and a second icon **copies** the same row into a
  fresh catalog scoped to this collection alone, which the original can't drift. A third button,
  beside "Add catalogs", starts a catalog **new inside this collection**: named first (the same
  two-step the library's own "New catalog" uses), then opened in the nested editor below to fill
  its filters.
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
  has no inline Edit here at all — it's a live pointer, and editing it from inside a collection
  used to silently reach every other folder and the library too, which read as a surprise rather
  than a feature. The row instead says how many places it's used (home screen plus every folder
  across every owned collection — the folders from `Workspace`'s `usedInFolders`, the home screen
  read by the row itself) and offers "Copy into this
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
  genre" share one list. The Preview panel and Home's folder pages fetch each source's
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
- **Sharing is the shared `Switch` component here too**, same as the catalog editor, labelled
  Shared or Private. Sharing a collection shares the recipes inside it; when any of them is
  private on its own, a caution under the switch names them. Show first and the "All" tab are `Segmented`, the latter greyed (DESIGN.md's
  "Greyed" segmented state, `Segmented`'s `disabled` prop) rather than hidden while the view mode
  isn't Tabbed Grids, keeping its value for when it switches back.
- **The Preview panel is a working client screen, docked beside the form.** The collection editor has
  its own layout (`.ed.ed-tv`, `EditorShell`'s `docked="tv"`, capped at `--w-editor-tv`): the form keeps
  its `--w-form` column and the panel takes `clamp(380px, 42cqw, 620px)` beside it, undocking under
  the form below 960px of pane. It draws the live draft with the Home Preview's own components
  (`TVCollectionRow`, `TVFolderPage` from `features/home/tv.tsx`), so a folder tile opens the same
  folder page Home does — tabs or rows per `view_mode`, real titles per catalog. The rows sit in
  `.pv-rows` inside the raised `.ed-pv` panel the catalog editor's results use: tiles are the
  same fixed pixel sizes as Home's, rows run off the panel's edge and scroll, and a folder grid
  reflows to the column. Docked, the panel is sticky, so the rows scroll inside it at a height
  that fits beside the form; undocked (and in Community) they flow with the page. Tiles come from each source's recipe (`useRecipesTiles`), not its id, so an
  unsaved draft and a Community collection both preview, and only the open folder's catalogs are
  fetched. The back arrow and Escape return to the row — Escape is stopped inside the panel so it
  never reaches the editor's own Escape-to-close — and there is no history entry.

## Community tab

`web/src/features/community/` — everyone else's public catalogs and collections, browsed and
copied rather than referenced. The closed-graph model's only path across an owner boundary:
a folder can only ever reference your own listed catalogs (see
"Library rail" above), so this tab never lets you *use* another owner's row live, only Take a
private copy of it. A taken copy stays linked to its original until you edit it, and while it
does, Update brings it in line with the owner's changes on request.

- **`CommunityView.tsx`** owns the Catalogs / Collections `Segmented`, a name-only search field,
  and a Name / Newest `Select` — pure client-side filtering and sorting over
  `useCommunityCatalogs`/`useCommunityCollections` (`useCommunity.ts`), which are thin
  `useQuery` wrappers over the community endpoints, keyed the same way the library's queries are
  (`queryKeys.communityCatalogs`/`communityCollections`, both under the `['p', i, …]` prefix).
- **No author, no handle, no "copied from" line anywhere here** —
  that provenance is retired, not merely hidden. `taken_from` links a profile's copy to its
  original (see "A Take is a linked copy" in `docs/data-model.md`), which surfaces here only
  through the row's main button, driven by the server's own `taken` and `update_available`:
  **Take**; a disabled **✓ Taken** while the profile holds a linked copy (the server refuses a
  second Take with a 409); or **Update** while that copy is behind the original. **Duplicate** —
  the same copy with no link, always allowed — waits behind a "⋯" menu, the same
  `components/MoreMenu.tsx` (Radix `DropdownMenu`) the collection editor's `RefMenu` and the Home
  list's rows are built on. While any of the three is in flight on a row, the
  main button is disabled and says so ("Taking…", "Updating…", "Duplicating…").
- **Preview reuses the editors' own preview components, not a new one.** A catalog row's Preview
  mounts `CommunityCatalogPreview`, which is `RecipePreview` run over the row's own stored
  `type`/`params` via `useRecipeTiles` — the same on-request, one-TMDB-page component the catalog
  editor's results panel uses, never `invalid` since a community row is always a saved catalog the
  server already accepted. A collection row's Preview mounts `CommunityCollectionPreview`, which
  is the collection editor's own Preview panel (`CollectionPreview`) fed a
  `PreviewCollection` built by Home's `toPreviewCollection` (`features/home/preview.ts`) over the
  row's own `catalogs` array rather than the library — the community row already carries every
  catalog its folders reference, listed or scoped on the source side, so nothing resolves as
  unavailable the way a library-sourced ref picker's accessible set would for someone else's
  catalog. The editor feeds the same panel its draft through `previewFromForm`
  (`collectionForm.ts`).
- **Take, Update and Duplicate refresh both the library and the community lists**
  (`useCommunityMutations.ts`) — a copy appears or changes in "Mine" and the original's flags
  flip, both from one mutation — and each mutation settles only once those refetches have landed,
  so a row never offers Take again for a copy that already exists. The owned-list keys prefix the
  selection keys, so the selections refetch too, which a collection Update needs because it bumps
  `version`. A small auto-dismissing strip (2.5s) reports the outcome: "Added to your
  catalogs"/"collections", "Updated your copy", "Duplicated to your catalogs"/"collections";
  unlike the push outcome strip this never needs a decision, so nothing about it persists past
  being read.
- **A 404 or a 409 means the row was stale** (`isStale`): the original went private or was
  deleted, a linked copy already exists, or the copy was unlinked by a save. The mutation
  refreshes the same lists before it rejects, then the strip says what happened: a 409 from Take
  is "Already taken"; a 409 from Update, which unlinked a copy it found edited, is "Your copy was
  edited, so it's no longer linked. Take it again to get the latest"; and a 404 from Update, which
  can't say whether the original went private or was deleted or the copy was already unlinked,
  is "Couldn't update: the original is no longer available, or your copy is no longer linked."
  Any other failure reads "Couldn't …" with the server's message.
- **The editors mark a linked copy and ask before unlinking it.** `Workspace` passes the open
  library row's `linked` to `CatalogEditor` and `CollectionEditor`, which then show a "Linked"
  banner as their first row (`LinkedBanner`, `features/builder/LinkedCopy.tsx`) — the
  collection's also covers the catalogs edited inside it, which have no link of their own. Saving
  a linked copy with anything other than Public changed opens `ConfirmUnlink` ("Save and
  unlink"); for a collection, pending `catalog_edits` count, a staged Move to library included
  (`changesContent` in each form module). The client only decides whether to ask — which saves
  unlink is the server's call. The save refetches the library and the community lists, so the
  row's `linked` is current the next time an editor opens on it.
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
- **Push is blocked until the home selection has loaded** (`home.ready`), both on the button
  and inside `push()`. Before then `snapshot()` returns `EMPTY_HOME`, and a full-replace push
  of it would remove every Uno catalog row and collection from the profile.
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

Dark-only, single theme: the builder is used at a desk and on the couch in the
evening, and a light theme is ruled out. `DESIGN.md` holds the full system —
tokens, components, rules; this section says what the frontend implements and where.

**Video Store, after hours.** The builder reads like a neighbourhood video shop after closing:
each region is announced by a sign in its own colour, every catalog carries a plain-words line
saying what it returns, and the home screen is the front shelf, in Nuvio's order.

**Colour holds one meaning per hue.** A night-navy ground and its shelves (`--uno-ground`,
`--uno-raised`, `--uno-raised-hi`) carry the layout. Three hues name the regions — catalog
tangerine, collection green, community pink — and TV yellow means "bound for Nuvio" and nothing
else: Push, what is on the home screen, what is not pushed yet. Danger red marks what can't be
undone, always beside words. A region's colour reaches its primary button and headings through
`--accent`, set by the `.tone-*` classes: `EditorShell` sets `tone-catalog` or `tone-collection`,
the Home pane `tone-tv`, Community `tone-community`. Tokens live in `web/src/index.css` as
`--uno-*` custom properties, exposed to Tailwind via `@theme inline`.

**Signs.** `.sign` is a flat band in its region's colour across the full width of what it names —
the rail's Catalogs and Collections, the Home pane, each editor's header, Community — lettered in
`.type-sign`.

**Type — two faces.** Figtree carries every word Uno says, previews included; Archivo at its
widest and heaviest is sign lettering only. Both ship as Fontsource packages
(`@fontsource-variable/figtree/wght.css`, and `@fontsource-variable/archivo/wdth.css` — the file
that carries Archivo's width axis), not the Google Fonts CDN: the build is `go:embed`'d, and the CSP allows
fonts only from `'self'` and `data:`.

**Stickers.** Small printed labels state a row's facts in words: its kind (`.stk-kind`), Shared
(`.stk-shared`, community pink, because Community is where a shared row turns up), Linked
(`.stk-linked`, outlined in community pink: a Take still linked to its original), and Unavailable (`.stk-danger`). A library row's home-screen toggle is `.tv-sticker`: a dashed empty
circle while it's off the home screen, a yellow ON NUVIO price sticker (two lines, ON over NUVIO)
once it's on. A home row's position is a yellow `.pos-sticker`, read out as "3rd on your home
screen"; the pending count and a profile's slot
number are `.count-sticker`s.

**Settings — label above control.** Every editor setting is a `.setting`: its label
(`.setting-label`) above the control, on the form's one left edge. Folding sections (`.sec-head`
and `.sec-body`) are shelves that open in place; a heading inside a form (`.setting.is-head`) is
sign lettering in the region's colour over a rule of the same colour. The catalog editor opens
with a `.talker`: the recipe as a sentence (`recipeSentence`, `web/src/features/library/recipe.ts`),
kept current as the settings below it change. The rail, the Home rows and a folder's catalog
picker show the same recipe as one line (`recipeLine`, or `catalogListing` where the list is also
searched).

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
with tiles on raised-hi and Figtree captions. The one TV artifact kept is the full-amplitude SMPTE
colour bars (`--smpte-*`, on the `.pv-nosignal` card), television's own picture for "nothing to
show", in Home's Preview view only. Uno's words about the rows sit above and below the panel.

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
Community's Take button (linked copies), the collection's Focus glow, a folder's Focus GIF and
Modern Home fields, the catalog picker's link-versus-copy, and the genre chips' three-state cycle.
It opens on click rather than hover, so it works on touch.
