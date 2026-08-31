import { useMemo } from 'react'
import type { ReactNode } from 'react'
import { tmdbKind } from '@/api'
import type { Collection } from '@/api'
import { ListState } from '@/components/ListState'
import { TypeBar } from '@/components/TypeBar'
import { describeRecipe } from '@/features/library/recipe'
import { HomePreview } from './HomePreview'
import { SortableList, SortableRow } from './SortableList'
import { useHomeSelection } from './useHomeSelection'

export type HomeView = 'list' | 'preview'

/**
 * What is actually on the profile's home screen: selected collections (each a
 * row whose tiles are its folders) and selected catalogs (each a row of
 * content), both ordered. Collections with `pin_to_top` sit above the catalog
 * rows, the rest below — Preview is where that order is visible. This view
 * groups by kind instead, because it is the editor and the two lists are
 * ordered separately.
 *
 * Called "your home screen", never "selection" — that's the schema's word.
 * Nothing here writes to the server; every edit is pending until Push.
 *
 * Two views of one state. List is where every edit happens; Preview draws the
 * same state as the shape of the screen it becomes. Both read from the same
 * client state, so the switch costs no fetch. View mode is in-page state, not
 * routed.
 *
 * **The view is held above this component**, because the pane it lives in is
 * shared with the editors: opening one unmounts this, and a view kept here
 * would silently drop back to List every time someone edited a catalog and came
 * back. Closing an editor should return the screen you left.
 */
export function HomePane({
  view,
  onViewChange,
}: {
  view: HomeView
  onViewChange: (view: HomeView) => void
}) {
  const home = useHomeSelection()

  return (
    <main className="flex flex-col gap-8 p-6 lg:min-h-0 lg:overflow-y-auto">
      <div className="flex flex-wrap items-baseline gap-4">
        <h1 className="type-display m-0 text-[21px]">Your home screen</h1>
        <ViewSwitch view={view} onChange={onViewChange} />
      </div>

      {/* Loading and error are shared — both views need the same state before
          they can render anything. Empty is *not* shared: List's empty state is
          an instruction to go add something, Preview's is the screen a TV shows
          when there's nothing to show. So each branch owns it. */}
      <ListState
        isLoading={home.isLoading || !home.ready}
        error={home.error}
        isEmpty={false}
        loadingLabel="Loading your home screen…"
        errorLabel="Couldn't load your home screen."
        emptyLabel={null}
      >
        {view === 'list' ? <HomeList /> : <HomePreview />}
      </ListState>
    </main>
  )
}

function ViewSwitch({ view, onChange }: { view: HomeView; onChange: (view: HomeView) => void }) {
  return (
    <div className="border-line-hi flex overflow-hidden rounded-[2px] border">
      {(['list', 'preview'] as const).map((option) => (
        <button
          key={option}
          type="button"
          onClick={() => onChange(option)}
          aria-pressed={view === option}
          className={`type-data px-3 py-1.5 text-[10.5px] tracking-[0.08em] uppercase transition-colors ${
            view === option ? 'bg-raised-hi text-ink' : 'text-dim hover:text-ink'
          }`}
        >
          {option}
        </button>
      ))}
    </div>
  )
}

function HomeList() {
  const home = useHomeSelection()

  if (home.catalogs.length === 0 && home.collections.length === 0) {
    return (
      <p className="type-data text-dimmer m-0 py-2 text-[11px]">
        Nothing here yet. Add catalogs and collections from the sidebar.
      </p>
    )
  }

  // Capped, unlike Preview beside it. A row here is a name, a recipe and two
  // controls; stretched to a 1100px pane the controls end up an inch of empty
  // space away from the row they belong to. Preview is a drawing of a
  // television and keeps the whole width.
  return (
    <div className="flex max-w-[var(--w-form)] flex-col gap-8">
      <CollectionsBlock />
      <CatalogsBlock />
    </div>
  )
}

function Block({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-2">
      <div className="border-line-hi flex flex-wrap items-baseline gap-x-3 gap-y-1 border-b pb-2">
        <span className="type-eyebrow">{label}</span>
      </div>
      {children}
    </section>
  )
}

function CollectionsBlock() {
  const home = useHomeSelection()

  return (
    <Block label="Collections">
      {home.collections.length === 0 ? (
        <EmptyBlock>No collections yet — add one from the sidebar.</EmptyBlock>
      ) : (
        <SortableList ids={home.collections} onReorder={home.reorderCollections}>
          {home.collections.map((id) => {
            const collection = home.collectionById.get(id)
            const name = collection?.title ?? 'Unavailable collection'
            return (
              <SortableRow key={id} id={id} label={`Reorder ${name}`}>
                <TypeBar kind="collection" owned={home.isOwned(id)} />
                <RowMeta
                  name={name}
                  detail={describeCollection(collection)}
                  detached={home.isDetached(id)}
                />
                <span />
                <RemoveButton
                  label={`Remove ${name} from your home screen`}
                  onClick={() => home.removeCollection(id)}
                />
              </SortableRow>
            )
          })}
        </SortableList>
      )}
    </Block>
  )
}

function CatalogsBlock() {
  const home = useHomeSelection()
  const ids = useMemo(() => home.catalogs.map((c) => c.id), [home.catalogs])

  return (
    <Block label="Catalogs">
      {home.catalogs.length === 0 ? (
        <EmptyBlock>No catalogs yet — add one from the sidebar.</EmptyBlock>
      ) : (
        <SortableList ids={ids} onReorder={home.reorderCatalogs}>
          {home.catalogs.map((entry) => {
            const catalog = home.catalogById.get(entry.id)
            const name = catalog?.name ?? 'Unavailable catalog'
            const recipe = catalog
              ? describeRecipe(catalog, home.genres[tmdbKind(catalog.type)]).join(' · ')
              : ''
            return (
              <SortableRow key={entry.id} id={entry.id} label={`Reorder ${name}`}>
                <TypeBar kind={catalog?.type ?? 'movie'} owned={home.isOwned(entry.id)} />
                <RowMeta
                  name={name}
                  detail={recipe || 'no filters'}
                  detached={home.isDetached(entry.id)}
                />
                <ShowInHomeToggle
                  name={name}
                  showInHome={entry.showInHome}
                  onToggle={() => home.toggleShowInHome(entry.id)}
                />
                <RemoveButton
                  label={`Remove ${name} from your home screen`}
                  onClick={() => home.removeCatalog(entry.id)}
                />
              </SortableRow>
            )
          })}
        </SortableList>
      )}
    </Block>
  )
}

function RowMeta({ name, detail, detached }: { name: string; detail: string; detached: boolean }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <span className="truncate text-[13px] font-medium">{name}</span>
      <span className="type-data text-dimmer flex min-w-0 items-baseline gap-1.5 text-[10.5px]">
        <span className="truncate">{detail}</span>
        {detached && (
          <span
            className="text-series shrink-0"
            title="Its owner deleted it or made it private. It still works here, but you can't edit it."
          >
            · unavailable
          </span>
        )}
      </span>
    </div>
  )
}

/**
 * `show_in_home = false` is meant to hide a catalog's row from home while
 * keeping it in Discover, by marking its genre filter `isRequired` in the
 * manifest. **`buildManifest` (`internal/addon/addon.go`) does not consume the
 * flag**, so the tooltip says the toggle isn't live rather than letting it
 * appear to work.
 */
function ShowInHomeToggle({
  name,
  showInHome,
  onToggle,
}: {
  name: string
  showInHome: boolean
  onToggle: () => void
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-pressed={showInHome}
      title={
        showInHome
          ? `${name} shows on your home screen`
          : `${name} is meant to be hidden from your home screen — not active yet, so it still shows`
      }
      // A fixed width, because the two labels are different lengths and the
      // toggles sit in a column: sized to their content, "on home" and
      // "discover only" gave the column a ragged left edge.
      className={`type-data border-line-hi hover:border-dim flex w-[116px] items-center justify-center gap-1.5 rounded-[2px] border px-2 py-1.5 text-[10.5px] tracking-[0.05em] transition-colors ${
        showInHome ? 'text-ink' : 'text-dim'
      }`}
    >
      <span
        aria-hidden="true"
        className={`h-[5px] w-[5px] rounded-full ${showInHome ? 'bg-movie' : 'bg-dimmer'}`}
      />
      {showInHome ? 'on home' : 'discover only'}
    </button>
  )
}

function RemoveButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      title={label}
      className="text-dimmer hover:text-danger hover:border-danger grid h-6 w-6 place-items-center rounded-[2px] border border-transparent leading-none transition-colors"
    >
      ×
    </button>
  )
}

function EmptyBlock({ children }: { children: ReactNode }) {
  return <p className="type-data text-dimmer m-0 py-3 text-[11px]">{children}</p>
}

function describeCollection(collection: Collection | undefined): string {
  if (!collection) return 'no longer available'
  const folders = collection.folders ?? []
  const count = `${folders.length} ${folders.length === 1 ? 'folder' : 'folders'}`
  if (folders.length === 0) return count
  return `${count} · ${folders.map((f) => f.title).join(', ')}`
}
