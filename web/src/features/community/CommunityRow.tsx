import { useEffect, useId, useMemo, type ReactNode } from 'react'
import type { CommunityCatalog, CommunityCollection } from '@/api'
import { tmdbKind } from '@/api'
import { InfoTip } from '@/components/fields'
import { RecipePreview } from '@/features/catalogs/RecipePreview'
import { CollectionPreview } from '@/features/collections/CollectionPreview'
import { formFromCollection } from '@/features/collections/collectionForm'
import { buildRefOptions, indexRefOptions } from '@/features/collections/refs'
import { describeCollection } from '@/features/library/collection'
import type { GenreLookups } from '@/features/library/useLibrary'
import { useRecipeTiles } from '@/features/preview/useRecipeTiles'

/**
 * One community row: name, then either "Movies"/"Series" or a folder count in
 * place of the library's recipe summary — nobody is filtering these by name
 * yet, they're deciding whether to take a copy. No author, no handle, no
 * provenance: the closed graph means nothing here can be attributed without
 * becoming a live pointer.
 *
 * **Taking again is allowed.** `taken` only marks that a copy exists, so the
 * button never disables — a second press makes a second, independent copy.
 */
export function CommunityRow({
  name,
  summary,
  taken,
  taking,
  previewOpen,
  onTogglePreview,
  onTake,
  preview,
}: {
  name: string
  summary: string
  taken: boolean
  taking: boolean
  previewOpen: boolean
  onTogglePreview: () => void
  onTake: () => void
  preview: ReactNode
}) {
  const previewPanelID = useId()

  return (
    <div className="border-line border-b py-2.5">
      <div className="flex items-center gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-[3px]">
          <span className="truncate text-[13px] font-medium">{name}</span>
          <span className="type-data text-dimmer truncate text-[10.5px]">{summary}</span>
        </div>

        <button
          type="button"
          onClick={onTogglePreview}
          aria-expanded={previewOpen}
          aria-controls={previewPanelID}
          className="btn-ghost shrink-0"
        >
          {previewOpen ? 'Hide preview' : 'Preview'}
        </button>

        {taken && (
          <span className="type-data text-dimmer shrink-0 text-[10.5px]">✓ Taken</span>
        )}

        <div className="flex shrink-0 items-center gap-1.5">
          <button
            type="button"
            onClick={onTake}
            disabled={taking}
            className="btn-secondary btn-sm"
          >
            {taking ? 'Taking…' : 'Take'}
          </button>
          <InfoTip
            label="Take"
            text="Taking makes an independent copy. Nothing you do to it reaches this original, and you can take it again anytime."
          />
        </div>
      </div>

      {previewOpen && (
        <div id={previewPanelID} className="mt-3">
          {preview}
        </div>
      )}
    </div>
  )
}

/** `RecipePreview` over a community catalog's own stored recipe — the same
 *  component the catalog editor's results panel uses. Unlike the editor, the
 *  recipe here is a saved row, not something being typed, so there's no
 *  keystroke-per-request concern — it runs itself as soon as the panel
 *  opens, and "Run again" still works for a shuffled recipe's next page.
 *  Never `invalid`: a community row is a saved catalog the server already
 *  accepted. */
export function CommunityCatalogPreview({ catalog }: { catalog: CommunityCatalog }) {
  const preview = useRecipeTiles(catalog.type, catalog.params)
  // Empty deps deliberately: this component mounts fresh each time the
  // preview panel opens (see `CommunityRow`), so running once on mount is
  // "run once, on open" — `preview.run` itself isn't stable across renders.
  // oxlint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => preview.run(), [])
  return <RecipePreview preview={preview} type={catalog.type} invalid={false} onRun={preview.run} />
}

/** "On your TV" over a community collection's own tree — the same component
 *  the collection editor's docked panel uses, fed from the row's own
 *  `catalogs` rather than the library, so every folder resolves regardless of
 *  what this profile owns. */
export function CommunityCollectionPreview({
  collection,
  genres,
}: {
  collection: CommunityCollection
  genres: GenreLookups
}) {
  const optionByID = useMemo(
    () => indexRefOptions(buildRefOptions(collection.catalogs ?? [], genres)),
    [collection, genres],
  )
  // Memoised on the row: `formFromCollection` mints fresh folder keys on every
  // call, and the preview holds its open folder page by key, so a fresh form on
  // each render of the list would close that page.
  const state = useMemo(() => formFromCollection(collection), [collection])
  return <CollectionPreview state={state} optionByID={optionByID} />
}

export function catalogSummary(catalog: CommunityCatalog): string {
  return tmdbKind(catalog.type) === 'tv' ? 'Series' : 'Movies'
}

export function collectionSummary(collection: CommunityCollection): string {
  return describeCollection(collection)
}
