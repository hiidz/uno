import { useEffect, useId, useMemo, type ReactNode } from 'react'
import { MoreHorizontal } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type { CommunityCatalog, CommunityCollection } from '@/api'
import { tmdbKind } from '@/api'
import { InfoTip } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { RecipePreview } from '@/features/catalogs/RecipePreview'
import { CollectionPreview } from '@/features/collections/CollectionPreview'
import { formFromCollection } from '@/features/collections/collectionForm'
import { buildRefOptions, indexRefOptions } from '@/features/collections/refs'
import { describeCollection } from '@/features/library/collection'
import type { GenreLookups } from '@/features/library/useLibrary'
import { useRecipeTiles } from '@/features/preview/useRecipeTiles'
import type { CommunityAction } from './useCommunityMutations'

const PENDING_LABEL: Record<CommunityAction, string> = {
  take: 'Taking…',
  update: 'Updating…',
  duplicate: 'Duplicating…',
}

/**
 * One community row: name, then either "Movies"/"Series" or a folder count in
 * place of the library's recipe summary — nobody is filtering these by name
 * yet, they're deciding whether to take a copy. No author, no handle, no
 * provenance: the closed graph means nothing here can be attributed without
 * becoming a live pointer.
 *
 * **One main button, three states**, from the server's own flags: Take; a
 * disabled "✓ Taken" while this profile holds a linked copy (the server
 * answers a second Take with a 409); and Update while that copy is behind the
 * original. Duplicate, an unlinked copy that is always allowed, waits behind
 * "⋯", following the collection editor's `RefMenu`.
 */
export function CommunityRow({
  name,
  summary,
  taken,
  updateAvailable,
  pending,
  previewOpen,
  onTogglePreview,
  onTake,
  onUpdate,
  onDuplicate,
  preview,
}: {
  name: string
  summary: string
  taken: boolean
  updateAvailable: boolean
  /** The action in flight on this row, if any. Every action waits for it. */
  pending: CommunityAction | undefined
  previewOpen: boolean
  onTogglePreview: () => void
  onTake: () => void
  onUpdate: () => void
  onDuplicate: () => void
  preview: ReactNode
}) {
  const previewPanelID = useId()
  const label = pending
    ? PENDING_LABEL[pending]
    : updateAvailable
      ? 'Update'
      : taken
        ? '✓ Taken'
        : 'Take'

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

        <div className="flex shrink-0 items-center gap-1.5">
          <button
            type="button"
            onClick={updateAvailable ? onUpdate : onTake}
            disabled={pending !== undefined || (taken && !updateAvailable)}
            className="btn-secondary btn-sm"
          >
            {label}
          </button>
          <InfoTip
            label="Take"
            text="Taking makes a copy that stays linked to this one: when its owner changes it, Update brings your copy in line. Editing your copy unlinks it. Nothing you do to your copy reaches this one. Duplicate, under ⋯, makes a copy that is never linked."
          />
          <DropdownMenu.Root modal={false}>
            <DropdownMenu.Trigger
              aria-label={`More for ${name}`}
              className="tap text-dimmer hover:bg-line hover:text-ink grid h-8 w-8 shrink-0 place-items-center rounded-[2px] transition-colors"
            >
              <Icon icon={MoreHorizontal} size={16} />
            </DropdownMenu.Trigger>
            <DropdownMenu.Portal>
              <DropdownMenu.Content
                align="end"
                sideOffset={4}
                className="bg-raised-hi border-line-hi z-40 flex w-56 flex-col gap-0.5 rounded-[2px] border p-1.5"
              >
                <DropdownMenu.Item
                  disabled={pending !== undefined}
                  onSelect={onDuplicate}
                  className="hover:bg-line focus-visible:bg-line data-[disabled]:text-dimmer data-[disabled]:hover:bg-transparent text-ink flex items-center rounded-[2px] px-2 py-2 text-left text-[12px] transition-colors"
                >
                  Duplicate
                </DropdownMenu.Item>
              </DropdownMenu.Content>
            </DropdownMenu.Portal>
          </DropdownMenu.Root>
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
