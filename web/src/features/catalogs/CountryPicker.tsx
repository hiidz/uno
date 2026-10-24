import { useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'
import { FieldNote } from '@/components/fields'
import { IconButton } from '@/components/IconButton'
import { MAX_ENTITY_IDS } from './catalogForm'
import { countryName, type CountryLookup } from './countries'
import { parseCodeList } from './params'

/**
 * The countries a title was made in (`with_origin_country`), picked by name
 * from TMDB's country table and kept as chips.
 *
 * The whole table is already loaded, so search runs as you type with no
 * request: names that start with the query come first, then names that hold
 * it. Several picks match any of them, so the stored value is pipe-joined and
 * there is no All/Any switch: a title has one country of origin far more often
 * than two. A stored comma-joined list keeps its join. At most
 * `MAX_ENTITY_IDS` countries.
 */

const MAX_MATCHES = 8

/** Countries whose name holds `query`, the ones starting with it first. */
function matchCountries(query: string, countryNames: CountryLookup, picked: string[]): [string, string][] {
  const needle = query.toLowerCase()
  const starts: [string, string][] = []
  const holds: [string, string][] = []
  for (const [code, name] of countryNames) {
    if (picked.includes(code)) continue
    const at = name.toLowerCase().indexOf(needle)
    if (at === 0) starts.push([code, name])
    else if (at > 0) holds.push([code, name])
  }
  const byName = (a: [string, string], b: [string, string]) => a[1].localeCompare(b[1])
  return [...starts.sort(byName), ...holds.sort(byName)].slice(0, MAX_MATCHES)
}

export function CountryPicker({
  value,
  countryNames,
  onChange,
  inputId,
  invalid,
}: {
  value: string | undefined
  countryNames: CountryLookup
  onChange: (value: string | undefined) => void
  inputId?: string
  invalid?: boolean
}) {
  const inputRef = useRef<HTMLInputElement>(null)
  const resultsRef = useRef<HTMLUListElement>(null)
  const { codes, join } = useMemo(() => parseCodeList(value), [value])
  const separator = join === 'and' && codes.length > 1 ? ',' : '|'
  const full = codes.length >= MAX_ENTITY_IDS

  const [query, setQuery] = useState('')
  const trimmed = query.trim()
  const matches = useMemo(
    () => (trimmed && !full ? matchCountries(trimmed, countryNames, codes) : []),
    [trimmed, full, countryNames, codes],
  )

  function write(next: string[]) {
    onChange(next.length ? next.join(separator) : undefined)
  }

  function add(code: string) {
    write([...codes, code])
    setQuery('')
    inputRef.current?.focus()
  }

  function resultButtons(): HTMLButtonElement[] {
    return Array.from(resultsRef.current?.querySelectorAll('button') ?? [])
  }

  function onInputKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      const first = resultButtons()[0]
      if (first) {
        event.preventDefault()
        first.focus()
      }
    } else if (event.key === 'Enter' && matches.length > 0) {
      event.preventDefault()
      add(matches[0][0])
    } else if (event.key === 'Escape' && query) {
      // Clears the search rather than reaching the editor's own
      // Escape-to-close.
      event.preventDefault()
      event.stopPropagation()
      setQuery('')
    }
  }

  function onResultsKeyDown(event: KeyboardEvent<HTMLUListElement>) {
    const buttons = resultButtons()
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
    if (index === -1) return
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      buttons[Math.min(index + 1, buttons.length - 1)]?.focus()
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      if (index === 0) inputRef.current?.focus()
      else buttons[index - 1]?.focus()
    } else if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      inputRef.current?.focus()
    }
  }

  let status = ''
  if (full) status = `A row can use up to ${MAX_ENTITY_IDS} countries. Remove one to add another.`
  else if (countryNames.size === 0) status = 'Couldn’t load countries.'
  else if (trimmed && matches.length === 0) status = 'No country matches that name.'

  return (
    <div className="flex w-full flex-col gap-2">
      {codes.length > 0 && (
        <ul aria-label="Chosen countries" className="m-0 flex list-none flex-wrap gap-1.5 p-0">
          {codes.map((code) => {
            const label = countryName(code, countryNames)
            return (
              <li key={code} className="text-meta bg-raised-hi border-dim text-ink inline-flex items-center rounded-full border pl-2">
                <span className="py-1 pointer-coarse:py-3">{label}</span>
                <IconButton
                  label={`Remove ${label}`}
                  icon={X}
                  onClick={() => write(codes.filter((existing) => existing !== code))}
                  variant="plain"
                  size="xs"
                />
              </li>
            )
          })}
        </ul>
      )}

      <input
        ref={inputRef}
        id={inputId}
        type="search"
        enterKeyHint="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        onKeyDown={onInputKeyDown}
        placeholder="Search by name…"
        aria-label="Search countries"
        aria-invalid={invalid}
        autoComplete="off"
        disabled={full}
        className="field type-data text-meta w-full max-w-[var(--w-entry)] pointer-coarse:text-field"
      />

      <div role="status">{status && <FieldNote tone="dim">{status}</FieldNote>}</div>

      {matches.length > 0 && (
        <ul
          ref={resultsRef}
          aria-label="Matching countries"
          onKeyDown={onResultsKeyDown}
          className="border-line m-0 flex max-h-[50svh] w-full max-w-[var(--w-entry)] list-none flex-col overflow-y-auto overscroll-contain rounded-xl border p-1 lg:max-h-[13rem]"
        >
          {matches.map(([code, name]) => (
            <li key={code}>
              {/* drift-ok: a search result's row, the whole row its target */}
              <button
                type="button"
                onClick={() => add(code)}
                aria-label={`Add ${name}`}
                className="text-dim hover:bg-raised-hi hover:text-ink focus-visible:bg-raised-hi focus-visible:text-ink text-meta w-full rounded-lg px-2 py-1.5 text-left transition-colors pointer-coarse:py-3"
              >
                {name}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
