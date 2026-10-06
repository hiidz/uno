import { useEffect, useRef, type ReactNode } from 'react'
import { tmdbKind } from '@/api'
import type { ImportCatalog } from '@/api'
import { Segmented, Select } from '@/components/fields'
import { describeFolders } from '@/features/library/collection'
import { recipeLine } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'
import { COLLECTION_KIND, kindSticker, type SharingSticker } from '@/features/sharing/sharingState'
import { SharingStickers } from '@/features/sharing/SharingStickers'
import { pluralCount } from '@/lib/plural'
import { choicesForAll, liveCatalogs, liveMatches, withSkip, type ReuseChoice, type ReuseChoices } from './reuse'
import type { Review } from './useImportFlow'

interface ImportReviewProps {
  review: Review
  genres: GenreLookups
  busy: boolean
  onEdit(): void
  onChoices(choices: ReuseChoices): void
  onSkip(skip: number[]): void
}

/**
 * What the check found, before anything is written: everything the bundle
 * holds as the Library lists it, catalogs with their kind and filters, then
 * collections with their folders. A row that matches something the library
 * already has carries its choice; a matched catalog of a collection's own sits
 * under its collection, and leaves with it when the collection is skipped.
 * Focus lands on the summary when the review appears.
 */
export function ImportReview({ review, genres, busy, onEdit, onChoices, onSkip }: ImportReviewProps) {
  const { check, choices, skip } = review
  const live = liveMatches(check, skip)
  const folders = check.collections.reduce((sum, collection) => sum + collection.folders.length, 0)
  const summary = useRef<HTMLDivElement>(null)
  useEffect(() => summary.current?.focus(), [])

  function catalogRow(catalog: ImportCatalog, scoped: boolean): ReactNode {
    return (
      <ContentRow
        key={catalog.key}
        name={catalog.name}
        line={recipeLine(catalog, genres[tmdbKind(catalog.type)])}
        sticker={kindSticker(catalog.type)}
      >
        {catalog.existing.length > 0 && (
          <CatalogChoice
            catalog={catalog}
            scoped={scoped}
            choice={choices[catalog.key]}
            onChange={(choice) => onChoices({ ...choices, [catalog.key]: choice })}
          />
        )}
      </ContentRow>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start gap-x-3 gap-y-2">
        <div ref={summary} tabIndex={-1} className="flex flex-1 basis-[240px] flex-col gap-1 outline-none">
          <p className="text-ink m-0 text-[13.5px] leading-relaxed break-words">
            This JSON holds {pluralCount(liveCatalogs(check, []).length, 'catalog')},{' '}
            {pluralCount(check.collections.length, 'collection')} and {pluralCount(folders, 'folder')}.
          </p>
          <Question
            collections={check.collections.filter((collection) => collection.matched).length}
            catalogs={live.length}
          />
        </div>
        <button type="button" onClick={onEdit} disabled={busy} className="btn-secondary shrink-0">
          Edit JSON
        </button>
      </div>
      {live.length > 0 && (
        <div className="-ml-3 flex flex-wrap">
          <button type="button" className="btn-ghost" onClick={() => onChoices(choicesForAll(live, false))}>
            Import all
          </button>
          <button type="button" className="btn-ghost" onClick={() => onChoices(choicesForAll(live, true))}>
            Use mine for all
          </button>
        </div>
      )}
      <ContentGroup label="Catalogs" count={check.catalogs.length}>
        {check.catalogs.map((catalog) => catalogRow(catalog, false))}
      </ContentGroup>
      <ContentGroup label="Collections" count={check.collections.length}>
        {check.collections.map((collection, index) => (
          <CollectionRow
            key={`collection-${index}`}
            title={collection.title}
            folders={collection.folders}
            matched={collection.matched}
            ownCatalogs={collection.catalogs.length}
            skipped={skip.includes(index)}
            onSkip={(skipped) => onSkip(withSkip(skip, index, skipped))}
          >
            {collection.catalogs
              .filter((catalog) => catalog.existing.length > 0)
              .map((catalog) => catalogRow(catalog, true))}
          </CollectionRow>
        ))}
      </ContentGroup>
    </div>
  )
}

/** What matched, collections by title and catalogs by filters, then what to
 *  do; nothing when nothing matched. */
function Question({ collections, catalogs }: { collections: number; catalogs: number }) {
  const found = [
    sameAs(collections, 'title', 'collection'),
    sameAs(catalogs, 'filters', 'catalog'),
  ].filter((s) => s !== '')
  if (found.length === 0) return null
  return (
    <p className="text-dim m-0 text-[13px] leading-relaxed">
      {found.join(' ')} Choose what Import does with each.
    </p>
  )
}

/** That `n` of the rows have the same `what` as a `noun` you already have. */
function sameAs(n: number, what: string, noun: string): string {
  if (n === 0) return ''
  if (n === 1) return `One of these has the same ${what} as a ${noun} you already have.`
  return `${n} of these have the same ${what} as ${noun}s you already have.`
}

interface ContentGroupProps {
  label: string
  count: number
  children: ReactNode
}

/** One kind of row under its label; nothing when the bundle holds none. */
function ContentGroup({ label, count, children }: ContentGroupProps) {
  if (count === 0) return null
  return (
    <section className="flex flex-col gap-1">
      <h3 className="type-label m-0">
        {label} · {count}
      </h3>
      <ul className="m-0 flex list-none flex-col p-0">{children}</ul>
    </section>
  )
}

interface ContentRowProps {
  name: string
  line: string
  sticker: SharingSticker
  children: ReactNode
}

/** One thing the bundle holds, as a Library row draws it: name, summary line
 *  and kind, then whatever the row asks. */
function ContentRow({ name, line, sticker, children }: ContentRowProps) {
  return (
    <li className="border-line flex min-w-0 flex-col gap-2 border-t py-3">
      <div className="flex min-w-0 flex-col gap-1">
        <span className="text-ink text-[14px] font-semibold break-words">{name}</span>
        {line && <span className="text-dim text-[12.5px] break-words">{line}</span>}
        <span className="mt-0.5 flex flex-wrap gap-1.5">
          <SharingStickers stickers={[sticker]} />
        </span>
      </div>
      {children}
    </li>
  )
}

interface CollectionRowProps {
  title: string
  folders: string[]
  matched: boolean
  ownCatalogs: number
  skipped: boolean
  onSkip(skipped: boolean): void
  children: ReactNode
}

/** A bundle collection, with its choice when its title is one the library
 *  already has, and its own matched catalogs under it while it is imported. */
function CollectionRow({ title, folders, matched, ownCatalogs, skipped, onSkip, children }: CollectionRowProps) {
  return (
    <ContentRow name={title} line={describeFolders(folders)} sticker={COLLECTION_KIND}>
      {matched && <CollectionChoice title={title} own={ownCatalogs} skipped={skipped} onSkip={onSkip} />}
      {!skipped && (
        <ul className="border-line m-0 flex list-none flex-col border-l p-0 pl-4 empty:hidden">{children}</ul>
      )}
    </ContentRow>
  )
}

interface CollectionChoiceProps {
  title: string
  own: number
  skipped: boolean
  onSkip(skipped: boolean): void
}

/** Import a matched collection, or leave it out with its own catalogs. */
function CollectionChoice({ title, own, skipped, onSkip }: CollectionChoiceProps) {
  return (
    <>
      <Segmented
        ariaLabel={`What to do with ${title}`}
        value={skipped ? 'skip' : 'import'}
        onChange={(value) => onSkip(value === 'skip')}
        options={[
          { value: 'import', label: 'Import it' },
          { value: 'skip', label: 'Skip, I already have it' },
        ]}
      />
      {skipped && own > 0 && (
        <span className="type-data text-dimmer text-[12.5px] leading-[1.45]">
          Its catalogs are left out too.
        </span>
      )}
    </>
  )
}

interface CatalogChoiceProps {
  catalog: ImportCatalog
  /** Whether the catalog is one of a collection's own rather than top level. */
  scoped: boolean
  choice: ReuseChoice
  onChange(choice: ReuseChoice): void
}

/**
 * A matched catalog's choice. The wording follows where the catalog sits in
 * the bundle: a top-level catalog reused is simply not imported, while one of
 * a collection's own reused makes that collection reference your library
 * catalog in its place.
 */
function CatalogChoice({ catalog, scoped, choice, onChange }: CatalogChoiceProps) {
  return (
    <>
      <Segmented
        ariaLabel={`What to do with ${catalog.name}`}
        value={choice.useExisting ? 'existing' : 'copy'}
        onChange={(value) => onChange({ ...choice, useExisting: value === 'existing' })}
        options={[
          { value: 'copy', label: 'Import it' },
          { value: 'existing', label: scoped ? 'Use my existing one' : 'Skip, I already have it' },
        ]}
      />
      {choice.useExisting && <Reuse catalog={catalog} scoped={scoped} choice={choice} onChange={onChange} />}
    </>
  )
}

/** A reused catalog: which of yours it becomes, and for one of a
 *  collection's own, that the collection then shares it. */
function Reuse({ catalog, scoped, choice, onChange }: CatalogChoiceProps) {
  return (
    <>
      <ExistingPick catalog={catalog} choice={choice} onChange={onChange} />
      {scoped && (
        <span className="type-data text-dimmer text-[12.5px] leading-[1.45]">
          The collection will share your library catalog, so later edits to it show up there too.
        </span>
      )}
    </>
  )
}

/** Which of your catalogs a reused one becomes: a Select when the check
 *  matched several, else the one named. */
function ExistingPick({ catalog, choice, onChange }: Omit<CatalogChoiceProps, 'scoped'>) {
  if (catalog.existing.length > 1) {
    return (
      <Select
        ariaLabel={`Which of your catalogs to use for ${catalog.name}`}
        value={choice.existingID}
        onChange={(existingID) => onChange({ ...choice, existingID })}
        options={catalog.existing.map((e) => ({ value: e.id, label: e.name }))}
      />
    )
  }
  return (
    <span className="type-data text-dim text-[12.5px] break-words">
      Uses {catalog.existing[0].name} from your library
    </span>
  )
}
