import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchWatchProviders, fetchWatchRegions, queryKeys } from '@/api'
import type { CatalogType, TMDBParams } from '@/api'
import { FieldNote, Select } from '@/components/fields'

/**
 * Which streaming services a title has to be on, picked by name.
 *
 * This used to be a free-text box for TMDB's numeric provider ids, with a hint
 * telling the user to go and look them up. `GET /api/watch-providers/{type}`
 * exists precisely so it doesn't have to: services are named, ranked by TMDB's
 * own prominence, and clicked.
 *
 * **Region first, because a service list only exists per market.** TMDB reports
 * what it has data for in one country, so the region chooses the list. Provider
 * ids themselves are global — Netflix is 8 everywhere — so changing region
 * keeps the picks, unlike the certification country, where the codes really do
 * mean different things.
 */

/** Enough of the ranked list to cover the services nearly everyone means,
 *  without a wall of chips. Search reaches the rest. */
const TOP_SERVICES = 15

/** Search can widen the list a long way — 291 services in the US — so matches
 *  are capped at a screenful. Narrowing the query is the way past it. */
const MAX_MATCHES = 40

const DEFAULT_REGION = 'US'

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
  // `watch_region` on the wire, and the list on screen shouldn't jump back to
  // the default region when it does.
  const [region, setRegion] = useState(params.watch_region || DEFAULT_REGION)
  const [query, setQuery] = useState('')

  const regions = useQuery({
    queryKey: queryKeys.watchRegions(),
    queryFn: fetchWatchRegions,
    staleTime: Infinity,
  })

  const providers = useQuery({
    queryKey: queryKeys.watchProviders(type, region),
    queryFn: () => fetchWatchProviders(type, region),
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

  // Selected first, always — a service picked in another region, or one TMDB
  // has since dropped from this one, would otherwise vanish from the chips
  // while still being in the payload.
  const shown = useMemo(() => {
    const selected = selectedIDs.map((id) => ({
      id,
      name: nameOf.get(id) ?? `Service ${id}`,
    }))

    const q = query.trim().toLowerCase()
    const rest = all
      .filter((p) => !selectedIDs.includes(p.provider_id))
      .filter((p) => !q || p.provider_name.toLowerCase().includes(q))
      .slice(0, q ? MAX_MATCHES : TOP_SERVICES)
      .map((p) => ({ id: p.provider_id, name: p.provider_name }))

    return [...selected, ...rest]
  }, [all, selectedIDs, nameOf, query])

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
    if (selectedIDs.length > 0) onParams({ watch_region: next })
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-3">
        <label className="type-eyebrow">Streaming on</label>
        <Select
          value={region}
          onChange={changeRegion}
          options={regionOptions}
          placeholder="Pick a country"
          width="var(--w-pick)"
        />
      </div>

      {providers.isError ? (
        <FieldNote tone="danger">Couldn't load streaming services.</FieldNote>
      ) : (
        <>
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search services…"
            aria-label="Search streaming services"
            className="field type-data w-full max-w-[var(--w-entry)] text-[12.5px]"
          />

          <div className="flex flex-wrap gap-1.5">
            {shown.map((service) => {
              const selected = selectedIDs.includes(service.id)
              return (
                <button
                  key={service.id}
                  type="button"
                  onClick={() => toggle(service.id)}
                  aria-pressed={selected}
                  className={`rounded-[2px] border px-2 py-1 text-[11px] transition-colors ${
                    selected
                      ? 'bg-raised-hi border-dim text-ink'
                      : 'border-line text-dim hover:border-dim hover:text-ink'
                  }`}
                >
                  {service.name}
                </button>
              )
            })}
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
