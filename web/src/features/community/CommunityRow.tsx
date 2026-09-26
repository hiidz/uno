import { useEffect, useId, useMemo, type ReactNode } from 'react'
import { Check } from 'lucide-react'
import type { CommunityCatalog, CommunityCollection } from '@/api'
import { InfoTip } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { MoreMenu, MoreMenuItem } from '@/components/MoreMenu'
import { RecipePreview } from '@/features/catalogs/RecipePreview'
import { CollectionPreview } from '@/features/collections/CollectionPreview'
import { toPreviewCollection } from '@/features/home/preview'
import { useRecipeTiles } from '@/features/preview/useRecipeTiles'
import type { CommunityAction } from './useCommunityMutations'

const PENDING_LABEL: Record<CommunityAction, string> = {
  take: 'Taking…',
  update: 'Updating…',
  duplicate: 'Duplicating…',
}

/**
 * One community row: name, then either "Movies"/"Series" or a folder count in
 * place of the library's recipe summary — someone here is deciding whether to
 * take a copy, and Preview shows the rest. No author, no handle, no
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
  const label = pending ? (
    PENDING_LABEL[pending]
  ) : updateAvailable ? (
    'Update'
  ) : taken ? (
    <>
      <Icon icon={Check} size={14} />
      Taken
    </>
  ) : (
    'Take'
  )

  return (
    <div className="border-line border-b py-3.5">
      <div className="flex items-center gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <span className="truncate text-[16px] font-bold">{name}</span>
          <span className="text-dim truncate text-[13.5px]">{summary}</span>
        </div>

        <button
          type="button"
          onClick={onTogglePreview}
          aria-expanded={previewOpen}
          aria-controls={previewPanelID}
          className="btn-ghost btn-sm shrink-0"
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
          <MoreMenu label={name}>
            <MoreMenuItem disabled={pending !== undefined} onSelect={onDuplicate}>
              Duplicate
            </MoreMenuItem>
          </MoreMenu>
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
export function CommunityCollectionPreview({ collection }: { collection: CommunityCollection }) {
  const preview = useMemo(
    () =>
      toPreviewCollection(
        collection.id,
        collection,
        new Map((collection.catalogs ?? []).map((catalog) => [catalog.id, catalog])),
      ),
    [collection],
  )
  return <CollectionPreview collection={preview} />
}
