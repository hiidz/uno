# Frontend

`web/` — Vite + React 19 + TypeScript, CSR only, no SSR. Everything sits behind per-user Nuvio
auth, nothing is public or crawlable (the addon server already owns the one public surface), and
there is no server-side session to render against.

Monorepo: `web/` at the repo root beside the Go module. One repo, one pipeline. `vite build` →
`web/dist` → `//go:embed all:dist` (`web/embed.go`) → served same-origin on the same port as the
API and addon server. No CORS in production; the Vite proxy is a dev-only convenience. `web/dist`
is gitignored except for a `.gitkeep` exception, so the embed target only exists after a build
has run in that working tree.

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
from one place. `web/src/features/preview/` holds the tile rendering shared by Home's Preview
view, the folder page, and the catalog editor's Run button. Nothing under `features/` is a
separate URL; `Builder` composes all of it.

`BuilderMode = 'edit' | 'duplicate'` is declared twice — once in
`web/src/features/catalogs/catalogForm.ts`, once in
`web/src/features/collections/collectionForm.ts`. Feature-local and duplicated on purpose, so
neither feature depends on the other for a two-value alias.

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

## `/configure` — two regions on one page

**Selecting is the common path; authoring is the rare one.** A generic user browses community
catalogs and collections, picks some, previews the result, pushes, and never opens an editor.
That is why this is two regions rather than four co-equal tabs.

| Region | Holds |
| --- | --- |
| **Library** — left rail | Every catalog and collection, yours and community, in two labelled groups. One filter chip group (Mine/Community/All) and one search box at the top of the rail narrow **both** groups at once. Compact rows: name plus a one-line recipe summary |
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

All four list endpoints fetched via `useQueries`, merged and deduped by `mergeOwned`
(`web/src/features/library/merge.ts`), every row tagged `owned`. Filter and search are **pure
client-side derivations** of that one dataset, so a chip click costs no network call — justified
by both lists being unpaginated and small. One dataset means one loading/empty/error state, not
one per filter mode.

All list endpoints are unpaginated; assume small N. That is what makes fetch-both-and-merge
viable. And because the community endpoints return your own public rows too, **ownership is a
per-row fact derived from membership in the owned set, never from which endpoint a row arrived
on** — deriving it from the source list renders your own public catalogs as someone else's. The
owner-vs-community distinction is applied consistently across the rail, both editors, the Home
pane, and the folder catalog-ref picker.

Two things established here that everything downstream depends on: **genre lookups are kept
per-kind, never merged** (the movie and tv genre id spaces are separate), and **genre queries are
excluded from the rail's loading/error state** — a failed genre fetch degrades the summary line
to raw ids rather than failing the list, so the rail never blocks on TMDB being reachable.

## Catalog authoring

Create is a three-field form — name, `type`, `is_public` — plus the TMDB params sub-form.
`provider` is derived (`"tmdb"`) and never rendered; `is_default` is excluded (and is `json:"-"`
server-side, so it can't even appear in a fetched row). `type` renders read-only on edit; **this
is a UI-only rule** — `PUT` still accepts it — so it holds only as long as every write goes
through this form. `provider`, by contrast, is enforced server-side in both validation places.

**Duplicate is the only path to modify a community catalog** — the fork-to-make-it-mine escape
hatch, and a first-class action, not a hidden overflow item. It reuses the create flow verbatim:
read source → prefill builder → unsaved → `POST`. Nothing appears in the list until the user
commits, and duplicates default to `is_public: false` regardless of source, because publishing is
a deliberate act rather than something inherited from whoever you forked. No extra fetch is
needed to duplicate from community — `GET /api/catalogs` returns the full row including
`params`. Duplicate is also the only way to "change a catalog's type", which is what makes
locking `type` acceptable.

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
- **Writes invalidate both the owned and community catalog lists** (`is_public` can change on
  any save, and a public row appears in both) but deliberately **not** the selection queries,
  which would clobber pending edits.
- **`useCatalogMutations` also invalidates the collection lists.** Not defensive — required.
  `DELETE FROM catalogs` cascades `folder_catalogs`, so a cached collection tree keeps a phantom
  ref: the overlay lists a folder member that no longer exists, and saving that collection
  `400`s.

## Home pane — List view

Add/remove from the rail, drag-reorder, `show_in_home` per catalog row. Hydrates once from both
`GET .../selection` endpoints; client state only, nothing writes until Push.

- **The baseline is snapshotted at hydration, not read live from the query cache.** A background
  refetch must not move the baseline under the user and silently change the diff.
- **Selected rows render from the selection response, not by library lookup** — the selection
  endpoints have no visibility filter, so the response can contain a community catalog whose
  owner has since made it private. Rendering by library lookup would make those rows vanish from
  the page while still being live on the user's TV. Such rows are marked "not in library": they
  work, but removing them is one-way.
- **`show_in_home` is labelled for what it will do, with a tooltip saying it isn't wired up
  yet** — `buildManifest` does not consume the flag, so the control is inert, and shipping it
  silent would be worse than shipping it honest.
- The header pending indicator is information, not an affordance, which is why it can exist
  before Push does. `countPendingChanges` drives both it and the navigation guard's dialog.

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
title text" (singular tile — it hides the title under the folder's own tile on home), and the
schema carries per-folder `focus_gif_url`/`focus_gif_enabled`, which only make sense on a
focusable tile. `pin_to_top` is "pin to top of **home screen**", which is why it partitions home
into bands rather than merely sorting collections to the front.

Decisions that shape the code:

- **Read-only, with exactly one interaction.** No drag, no remove, no toggles — editing lives
  entirely in List. An affordance that looks live and isn't reads as a bug. Opening a folder is
  navigation *within the mock*, not an edit. The open page is held as
  `{collectionId, folderId}`, **never indices**, so a folder deleted in List collapses back to
  home instead of silently rendering a different folder that happens to occupy the same slot.
- **`show_in_home` does not hide anything.** Discover-only rows render as a dimmed, labelled
  group carrying the same "not wired up yet" caveat as the List toggle. Dropping them would be
  the app's first claim that the split is live. When the manifest builder consumes the flag,
  that group becomes a genuine omission and the caveat text in `HomePreview.tsx` and the
  `ShowInHomeToggle` tooltip both come out.
- **A folder's `catalog_ids` are *sources*, never tiles.** Each is a standing query contributing
  an unknown number of items, so no view draws one tile per ref — that would misstate how much
  the folder holds. They are the folder page's spine: one row or one tab each.
- **"Not in library" and "nothing resolves" are two different conditions here**, and conflating
  them is a real bug. `catalogById`/`collectionById` are assembled from the selection response
  *as well as* the library, and the selection endpoint has no visibility filter — so a row whose
  owner made it private after selection still resolves. Testing *resolvability* would mean the
  "not in library" marker essentially never fires in Preview while List shows it for exactly
  that row: two views of one state disagreeing. Preview asks `isDetached` for the marker (same
  predicate, same wording as List) and keeps unresolvability for the genuinely-nothing-known
  fallback.
- **Slack wire values are handled, not cast away.** `tile_shape` can be `''` (falls back to
  `POSTER`, and says so on screen); `view_mode` is a bare `string`, so `FOLLOW_LAYOUT` and
  anything unrecognised land in one branch that admits it's guessing.
- **Selection order is preserved within each band**, so pinning moves a row between bands
  without discarding the order the user just dragged.
- **`ListState` owns loading and error for both views; each view owns its own empty case.**
  List's empty is an instruction to go add something; Preview's is the colour-bars moment from
  the design section below.

**Real tiles.** `web/src/features/home/useCatalogTiles.ts` issues **one query per catalog, never
a batch**. A batched endpoint carrying N recipes would flatten round trips but needs a per-item
error shape, and one slow TMDB call would hold up every row; per-catalog queries fail, retry,
and cache independently, and same-origin HTTP/2 multiplexes them anyway. Placeholder tiles are
the loading state (count 20, so the row doesn't reflow when content arrives); if TMDB is
unreachable, preview degrades to exactly the layout-only behaviour it has without the endpoint.

**Every row fetches at once — there is no IntersectionObserver, deliberately.** Preview rows
don't scroll horizontally and there is no "load more", so one call is the complete answer for a
row rather than a first page of one; at fifteen rows, deferring them isn't worth an observer's
complexity. The catalog editor's Run button is the one place a preview fetch is gated, and it is
gated on a keypress, not on visibility.

Two fidelity limits are named in the UI rather than papered over:

- **Merged catalogs.** Nuvio merges N catalogs into one view and **its merge rule is
  unspecified** — folders aren't an addon concept, so nothing in the addon protocol or Nuvio's
  docs specifies the order, and Uno cannot derive it. This applies to **exactly one view**: the
  `show_all_tab` "All" tab on a `TABBED_GRID` folder page. Every other view is one catalog per
  row or grid. Preview concatenates and labels it rather than implying the order is real.
- **`randomized` catalogs** take a random TMDB page per call on the addon path while preview
  always asks page 1, so preview genuinely won't match the TV. The one case where a placeholder
  is *more* accurate than real content. The flag comes from the server rather than being
  re-derived client-side.

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
  *every* droppable in the context, and a folder card is tall — so dragging a folder past its
  neighbour usually finds a **ref row inside** that neighbour to be the nearest center;
  `onDragEnd` then correctly refuses to guess and drops the folder back where it started. The
  dispatch check is doing its job; the candidate set is what has to be right. Worth remembering
  as a class: **a mis-drop guard turns a wrong reorder into a dead drag, which is not the same
  as making the drag work.** Fingerprint of this bug: `sortableKeyboardCoordinates` filters by
  `containerId` itself, so keyboard reordering works while pointer drag dies.
- **Cross-folder ref dragging is deliberately out** — it needs an insert position and a de-dup
  rule against the destination, to replace two clicks. Remove-then-add is the supported move.
- **"Not in the library" and "the save will 400" are one condition here**, which is the
  *opposite* of Preview's finding and not a contradiction. `useLibrary` is `owned ∪ is_public`;
  the server's `validateAccess` is `owner_id = ? OR is_public = 1` — identical predicates over
  identical rows. What makes Preview's two conditions diverge is the *selection* endpoint
  supplying rows the library doesn't have, and the builder has no selection endpoint in play. So
  the mirror is exact, and an unresolvable ref gets one marker naming the action that clears it
  ("Remove it to save").
- **Edit keeps unresolvable refs and blocks the save; duplicate drops them.** Dropping on edit
  would delete rows the user never touched. On duplicate they *have* to go — cloning a community
  collection that references a since-private catalog would `400` on a uuid the user has never
  seen — so the clone is partial and says which refs it left behind. The check costs nothing:
  `accessibleIDs` is the library, the same `owned ∪ is_public` set the server validates against,
  so no pre-flight request is needed.
- **Clone strips folder IDs.** A folder with an existing `id` must still belong to this
  collection or the save `400`s; stripping in `folderFromWire` when `mode === 'duplicate'` is
  also what turns the clone's folders into inserts.
- **A repeat inside one folder is unrepresentable, not merely validated.** It would be a
  `PRIMARY KEY (folder_id, catalog_id)` violation surfacing as a 500 the plain-text error
  channel can't explain — so the picker omits ids already in the folder, the add handler guards,
  and the validator backstops.
- **Removing a folder is a standing warning, not a confirm.** Omitting a folder from the payload
  deletes it server-side and cascades its refs — but nothing commits until Save, so
  `removedFolders(initial, current)` names exactly which folders the next save would destroy,
  shown continuously above the footer. A confirm would ask the user to approve something that
  hasn't happened. Dropping a folder that was never saved is correctly silent.
- **This editor confirms on discard; the catalog editor doesn't.** A deliberate asymmetry:
  losing a three-field form to a stray Escape is an annoyance, losing a folder tree is an
  evening. `EditorGuard` is what a rail click has to clear before the pane's occupant changes,
  so the confirm has one place to fire from rather than one per exit route.
- **`tile_shape: ''` and an unknown `view_mode` are handled differently, and both are honest.**
  `''` is a value the server accepts and Preview reads as an assumed poster, so it's kept as its
  own "Default" option rather than normalised — a round trip through this form must not silently
  rewrite stored data. An unrecognised `view_mode` can't be shown at all, so it reads as unset;
  there is no third option.
- **`cover_emoji` is a short text input** (`maxLength` 8, since an emoji can be several
  codepoints), not a picker — a bundled emoji picker is a large dependency for a field every
  keyboard already has an input method for.
- **Ordering is array position.** No `sort_order` field in the form; drag order *is* the value.
  Each folder shows `tab n/N` and each ref its index, so the thing being written is legible and
  not only draggable.
- **The catalog-ref picker is inline under the folder**, not a dialog — a scrim would hide the
  folder being filled. It stays open across picks and drops each chosen row out of the list, so
  what remains is always exactly what can still be added. Rows carry the type bar **and** state
  ownership in text, since the bar is `aria-hidden`.

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
- Owner-vs-community distinction is applied consistently across the rail, both editors, the Home
  pane, and the folder catalog-ref picker.

## Visual direction

Dark-only, single-theme, deliberate: every reference client (Stremio, Nuvio, Plex, Jellyfin) is
dark, and this app configures a TV. Nothing here is a stock default.

**The constraint that drives it:** in a *list* there is no artwork to carry the UI, so the
**summary line is the content**, and the list is designed to make standing queries scannable
rather than to frame posters. The rail, both editors, and the List view stay artwork-free by
choice. Preview is the one place posters land, which makes the `List | Preview` switch a payoff
rather than a mode change.

**Palette — derived from SMPTE colour bars**, desaturated into a working set. Television's own
artifact for "nothing to show", which is exactly this page's condition. Cool near-black ground
(`#0D0F13`); two working hues — cyan `#4EA8B8` for movie, amber `#C39A3E` for series — violet
`#8E7BC4` for collections, red `#C0483F` reserved for destructive. Tokens live in
`web/src/index.css` as `--uno-*` custom properties, exposed to Tailwind via `@theme inline`.

**Type — one family, three roles, via variable axes.** Archivo at `wdth 118` for display (a wide
grotesque reads as broadcast titling), Archivo at `wdth 100` for body, IBM Plex Mono for
recipes, ids, and numbers — a summary line is dense, aligned data and should look like it. (Mono
is the *face*, not the vocabulary: the line itself is plain English — see the copy rules below.)
`@fontsource-variable/archivo/wdth.css` ships both axes in one file. Fontsource packages, not
the Google Fonts CDN, because the build is `go:embed`'d and must not carry a runtime font
dependency.

**Signature — the type bar.** Every row carries a 3px full-height colour bar at its left edge,
hue by type. Read down a list, the column of bars is a test pattern. **Ownership rides the same
device rather than adding a second one: solid = yours, 45° hatched = community.** One device,
two facts, no badge. Applies everywhere a catalog or collection is listed. It is `aria-hidden`,
so anywhere ownership matters the row states it in text too.

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

**The one place a caveat survives in full** is the `show_in_home` toggle, because
`buildManifest` does not consume the flag: the control is inert, and its "not active yet"
wording is the only thing stopping the UI from claiming a feature that doesn't work. Delete that
text only together with the control.

**`InfoTip` is not a place to move hints to.** A hint the user needs *before* filling a field in
stays on the page as a `FieldNote`; one that doesn't survive "is this needed at all" is deleted.
The icon is for the narrow middle — a control genuinely worth a sentence, in a grid where that
sentence would push its neighbours down a line. Three fields pass a `tip`: the age-rating
control in `CatalogEditor`, and view mode and backdrop image in `CollectionEditor`. Adding a
tenth means rule 1 is being skipped.
