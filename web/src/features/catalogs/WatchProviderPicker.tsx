import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchWatchProviders, fetchWatchRegions, queryKeys } from '@/api'
import type { CatalogType, TMDBParams } from '@/api'
import { FieldNote, Select } from '@/components/fields'

/**
 * Which streaming services a title has to be on, picked by name.
 *
 * `GET /api/watch-providers/{type}` names every service and ranks them by
 * TMDB's own prominence, so this is a click rather than a lookup of TMDB's
 * numeric provider ids.
 *
 * **Region first, because a service list only exists per market.** TMDB reports
 * what it has data for in one country, so the region chooses the list, and
 * there is nothing to show until one is picked — a guessed default would put a
 * US service list in front of someone who never said US. Provider ids
 * themselves are global — Netflix is 8 everywhere — so changing region keeps
 * the picks, unlike the certification country, where the codes really do mean
 * different things.
 */


export function WatchProviderPicker({
  type,
  params,
  error,
  onParams,
}: {
  type: CatalogType
  params: TMDBParams
  error?: string
  onParams: (update: Partial<TMDBParams>) => void
}) {
  // Local, not derived from params: clearing every service clears
  // `watch_region` on the wire, and the list on screen shouldn't empty itself
  // when it does — the country you were browsing is still the country you
  // were browsing.
  const [region, setRegion] = useState(params.watch_region ?? '')
  const [query, setQuery] = useState('')

  const regions = useQuery({
    queryKey: queryKeys.watchRegions(),
    queryFn: fetchWatchRegions,
    staleTime: Infinity,
  })

  const providers = useQuery({
    queryKey: queryKeys.watchProviders(type, region),
    queryFn: () => fetchWatchProviders(type, region),
    enabled: Boolean(region),
    staleTime: Infinity,
  })

  const all = useMemo(() => providers.data ?? [], [providers.data])
  const selectedIDs = useMemo(() => parseProviderIDs(params.with_watch_providers), [
    params.with_watch_providers,
  ])

  const regionOptions = useMemo(
    () =>
      [...(regions.data ?? [])]
        .sort((a, b) => a.english_name.localeCompare(b.english_name))
        .map(({ iso_3166_1, english_name }) => ({ value: iso_3166_1, label: english_name })),
    [regions.data],
  )

  const nameOf = useMemo(
    () => new Map(all.map((p) => [p.provider_id, p.provider_name])),
    [all],
  )

  const matches = useMemo(() => {
    const q = query.trim().toLowerCase()
    return all
      .filter((p) => !selectedIDs.includes(p.provider_id))
      .filter((p) => !q || p.provider_name.toLowerCase().includes(q))
  }, [all, selectedIDs, query])

  // Selected first, always — a service picked in another region, or one TMDB
  // has since dropped from this one, would otherwise vanish from the chips
  // while still being in the payload. First also means visible without
  // scrolling, which is where your own picks belong.
  const shown = useMemo(() => {
    const selected = selectedIDs.map((id) => ({
      id,
      name: nameOf.get(id) ?? `Service ${id}`,
    }))
    const rest = matches.map((p) => ({ id: p.provider_id, name: p.provider_name }))

    return [...selected, ...rest]
  }, [matches, selectedIDs, nameOf])

  function toggle(id: number) {
    const next = selectedIDs.includes(id)
      ? selectedIDs.filter((existing) => existing !== id)
      : [...selectedIDs, id]

    onParams({
      with_watch_providers: next.length ? next.join(',') : undefined,
      // The pair is required together server-side, so the region rides along
      // with the ids rather than being a field of its own to forget.
      watch_region: next.length ? region : undefined,
    })
  }

  function changeRegion(next: string) {
    setRegion(next)
    setQuery('')
    if (!next) {
      // The pair is required together server-side, so clearing the country
      // takes the services with it. Keeping them would leave ids without the
      // market they were picked in — the one state this control is supposed to
      // make unreachable rather than merely catch.
      onParams({ with_watch_providers: undefined, watch_region: undefined })
      return
    }
    if (selectedIDs.length > 0) onParams({ watch_region: next })
  }

  return (
    // No label of its own: the section head above already says "Where to watch".
    <div className="flex w-full flex-col gap-2">
      <Select
        ariaLabel="Streaming country"
        value={region}
        onChange={changeRegion}
        options={regionOptions}
        placeholder="Select a country"
        clearable
        clearLabel="streaming country"
      />

      {!region ? (
        <FieldNote>Pick a country to see the services it carries.</FieldNote>
      ) : providers.isError ? (
        <FieldNote tone="danger">Couldn't load streaming services.</FieldNote>
      ) : (
        <>
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search services…"
            aria-label="Search streaming services"
            className="field type-data w-full max-w-[var(--w-entry)] text-[12.5px] pointer-coarse:text-[16px]"
          />

          {/* Scrolls rather than grows: the list runs to a few hundred chips in
              a large region — up to ~291 services, ranked by TMDB's own
              prominence, so the ones nearly everyone means are the first few
              rows — and a field that pushes everything under it off the page is
              worse than one you scroll.

              Two heights, because the page around it differs. Above `lg` the
              pane scrolls on its own and 13rem is a block within it. Below,
              the page is one document and a short fixed box inside it is a box
              a thumb gets caught in; half the viewport is still one scroll
              region, sized to the screen it is actually on. */}
          <div className="border-line flex max-h-[50svh] flex-wrap gap-1.5 overflow-y-auto overscroll-contain rounded-[2px] border p-2 lg:max-h-[13rem]">
            {shown.map((service) => {
              const selected = selectedIDs.includes(service.id)
              return (
                <button
                  key={service.id}
                  type="button"
                  onClick={() => toggle(service.id)}
                  aria-pressed={selected}
                  className={`rounded-[2px] border px-2 py-1 text-[11px] transition-colors pointer-coarse:py-3 ${
                    selected
                      ? 'bg-raised-hi border-dim text-ink'
                      : 'border-line text-dim hover:border-dim hover:text-ink'
                  }`}
                >
                  {service.name}
                </button>
              )
            })}
            {providers.isLoading && <FieldNote>Loading services…</FieldNote>}
            {shown.length === 0 && !providers.isLoading && (
              <FieldNote>No service matches that name here.</FieldNote>
            )}
          </div>
        </>
      )}

      {error && <FieldNote tone="danger">{error}</FieldNote>}
    </div>
  )
}

/** Stored TMDB-side as a comma- or pipe-separated id list. Anything that isn't
 *  a number is dropped rather than carried through as `NaN`, which would
 *  render as a chip nobody can deselect. */
function parseProviderIDs(raw: string | undefined): number[] {
  if (!raw) return []
  return raw
    .split(/[,|]/)
    .map((part) => Number(part.trim()))
    .filter((id) => Number.isInteger(id) && id > 0)
}
