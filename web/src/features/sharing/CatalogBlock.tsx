import { useId, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import type { Catalog } from '@/api'
import { Icon } from '@/components/Icon'
import { foldedLine, openFacts, recipeFacts, type FoldedLine, type RecipeFact } from '@/features/library/recipe'
import type { GenreLookups } from '@/features/library/useLibrary'
import { useRecipeNames } from '@/features/library/useRecipeNames'
import { factKey } from './changeWords'
import { MarkLine } from './MarkViews'
import {
  markFocus,
  markID,
  markWords,
  NAMES_PENDING,
  OpenTile,
  GoneTiles,
  SetTile,
  startsOpen,
  TILE,
  useMarkedChanges,
} from './MarkedTiles'
import type { BlockMark, FactChange } from './updateMarks'

/** The genre lookup for a catalog's own kind: movie and tv ids differ. */
function lookupFor(catalog: Catalog, genres: GenreLookups) {
  return catalog.type === 'movie' ? genres.movie : genres.tv
}


/** The facts that list things — genres and named entities — and so run long:
 *  each takes a row of its own. */
const LIST_LABELS = [
  'Genres',
  'Without genres',
  'Left-out',
  'Production company',
  'Production companies',
  'Keyword',
  'Network',
  'Streaming service',
]

function isListFact(label: string): boolean {
  return LIST_LABELS.some((prefix) => label.startsWith(prefix))
}

/** Whether `recipeFacts` has only counted a list: a number, then maybe the
 *  region ("2", "3 in United States"). */
function isCounted(value: string): boolean {
  const [head] = value.split(' ')
  const number = String(Number(head)) === head
  return number && (value === head || value.startsWith(`${head} in `))
}

/** `facts` with each counted list shown as pending while its lookups are
 *  still answering, so a tile never reads "1" and then a name. */
function awaitingNames(facts: RecipeFact[], loading: boolean): RecipeFact[] {
  if (!loading) return facts
  return facts.map((fact) => (isListFact(fact.label) && isCounted(fact.value) ? { ...fact, value: NAMES_PENDING } : fact))
}

/** A catalog's facts for its tiles, with the genre a folder narrows it to
 *  last. The recipe's names load as `display` says. */
function useFacts(
  catalog: Catalog,
  genres: GenreLookups,
  narrowedTo: string,
  display: { foldable: boolean; open: boolean },
): RecipeFact[] {
  const names = useRecipeNames(catalog, display)
  const facts = awaitingNames(recipeFacts(catalog, lookupFor(catalog, genres), names), names.loading)
  return narrowedTo ? [...facts, { label: 'Narrowed to', value: narrowedTo }] : facts
}


/** One tile's classes: flat on the ground, or in a folder card a ground well,
 *  and for a list a row of its own. */
function tileClass(label: string, inFolder: boolean): string {
  const fill = inFolder ? 'bg-ground' : 'bg-raised'
  return `${TILE} ${fill} ${isListFact(label) ? 'col-span-full' : ''}`
}

/** A catalog's facts as spec tiles: a dim label over a bold value, flowing as
 *  far as 140px a tile allows, the short ones first and then the lists on a
 *  row each. Open on its own they sit flat on the ground; a folded block's,
 *  inside a folder card, step down to ground wells. After them come the
 *  filters the recipe leaves `open`, outlined and unfilled, in dimmer type. */
function FactTiles({ facts, open, inFolder, changes }: FactTilesProps) {
  return (
    <dl className="m-0 grid grid-cols-[repeat(auto-fill,minmax(140px,1fr))] gap-2">
      {facts.map((fact) => (
        <SetTile key={fact.label} fact={fact} className={tileClass(fact.label, inFolder)} change={changes.get(factKey(fact.label))} />
      ))}
      {open.map((fact) => (
        <OpenTile key={fact.label} fact={fact} change={changes.get(factKey(fact.label))} />
      ))}
      <GoneTiles changes={changes} shown={[...facts, ...open]} />
    </dl>
  )
}

interface FactTilesProps {
  facts: RecipeFact[]
  open: RecipeFact[]
  inFolder: boolean
  /** The filters an update changes, by `factKey`: their tiles are edged in
   *  the accent with what each was under its value. */
  changes: ReadonlyMap<string, FactChange>
}

/**
 * A catalog block, the one way a catalog added from Community is read: in its
 * view, in a collection's folders, and on a publication's page.
 *
 * Open, it is the recipe's spec tiles and nothing else: what the recipe sets,
 * then every filter it leaves open ("Genres: Any"), so a reader who never
 * opened the editor sees what a catalog could filter on; the name is the
 * surrounding page's to show. Production companies, keywords, networks and
 * streaming services are named, from the lookups the catalog editor uses, and
 * show "…" until the names arrive (a count, if a lookup cannot name them). With
 * `foldable` it is a folder's entry instead: collapsed it is a chevron, the
 * catalog's name and its recipe line under it, and the header opens it in
 * place to the same tiles; a folded block loads no names.
 */
export function CatalogBlock({ catalog, genres, narrowedTo = '', foldable = false, mark }: CatalogBlockProps) {
  return foldable ? (
    <FoldedBlock catalog={catalog} genres={genres} narrowedTo={narrowedTo} mark={mark} />
  ) : (
    <OpenBlock catalog={catalog} genres={genres} mark={mark} />
  )
}

interface CatalogBlockProps {
  catalog: Catalog
  genres: GenreLookups
  /** The genre a folder narrows this catalog to, when it does. */
  narrowedTo?: string
  foldable?: boolean
  /** What an update waiting on a publication's page does to this catalog. */
  mark?: BlockMark
}

/** `facts` with the one-value facts ahead of the lists, so the short tiles
 *  share rows above the lists' full-width ones instead of leaving gaps
 *  between them. */
function shortFactsFirst(facts: RecipeFact[]): RecipeFact[] {
  return [...facts.filter((fact) => !isListFact(fact.label)), ...facts.filter((fact) => isListFact(fact.label))]
}

function OpenBlock({ catalog, genres, mark }: { catalog: Catalog; genres: GenreLookups; mark: BlockMark | undefined }) {
  const display = { foldable: false, open: true }
  const facts = useFacts(catalog, genres, '', display)
  const changes = useMarkedChanges(catalog, mark, genres, display)
  return (
    <div id={markID(mark)} tabIndex={markFocus(mark)} className="outline-none">
      <FactTiles facts={shortFactsFirst(facts)} open={openFacts(catalog, facts)} inFolder={false} changes={changes} />
    </div>
  )
}

function FoldedBlock({ catalog, genres, narrowedTo, mark }: FoldedBlockProps) {
  const [open, setOpen] = useState(startsOpen(mark))
  const panelID = useId()
  const facts = useFacts(catalog, genres, narrowedTo, { foldable: true, open })
  const changes = useMarkedChanges(catalog, mark, genres, { foldable: true, open })
  const toggle = () => setOpen(!open)
  return (
    <div id={markID(mark)} tabIndex={markFocus(mark)} className="border-line border-t py-2 outline-none first:border-t-0">
      <FoldHeader
        name={catalog.name}
        line={foldedLine(catalog, lookupFor(catalog, genres))}
        narrowedTo={narrowedTo}
        open={open}
        panelID={panelID}
        onToggle={toggle}
      />
      <div className="pl-[26px]">
        <MarkLine words={markWords(mark)} />
      </div>
      <div id={panelID} hidden={!open} className="pt-2 pl-[26px]">
        {open && <FactTiles facts={shortFactsFirst(facts)} open={openFacts(catalog, facts)} inFolder changes={changes} />}
      </div>
    </div>
  )
}

interface FoldHeaderProps {
  name: string
  line: FoldedLine
  narrowedTo: string
  open: boolean
  panelID: string
  onToggle: () => void
}

/** A folded block's header, which is the button that opens it: a chevron, the
 *  catalog's name, and while closed its first filters and how many more
 *  (`FoldedSummary`) under that. */
function FoldHeader({ name, line, narrowedTo, open, panelID, onToggle }: FoldHeaderProps) {
  const turn = open ? 'rotate-90' : ''
  return (
    <button
      type="button"
      aria-expanded={open}
      aria-controls={panelID}
      onClick={onToggle}
      className="tap flex w-full items-start gap-2.5 py-1 text-left"
    >
      <Icon icon={ChevronRight} size={16} className={`text-dimmer mt-0.5 shrink-0 transition-transform ${turn}`} />
      <span className="flex min-w-0 flex-col gap-0.5">
        <span className="text-[14px] font-semibold [overflow-wrap:anywhere]">{name}</span>
        {!open && <FoldedSummary line={line} narrowedTo={narrowedTo} />}
      </span>
    </button>
  )
}

/** A folded block's second line: its first phrases with the genre a folder
 *  narrows it to after them, then how many more phrases the tiles hold, in
 *  dimmer, "+10". One line, cut to fit. */
function FoldedSummary({ line, narrowedTo }: { line: FoldedLine; narrowedTo: string }) {
  const text = narrowedTo ? `${line.text} • ${narrowedTo}` : line.text
  return (
    <span className="flex min-w-0 gap-1.5 text-[12.5px]">
      <span className="text-dim truncate">{text}</span>
      {line.more > 0 && <span className="type-data text-dimmer shrink-0">+{line.more}</span>}
    </span>
  )
}

interface FoldedBlockProps {
  catalog: Catalog
  genres: GenreLookups
  narrowedTo: string
  mark: BlockMark | undefined
}
