import { useQueries, useQuery } from '@tanstack/react-query'
import { fetchCompany, fetchKeyword, fetchNetwork, fetchWatchProviders, queryKeys } from '@/api'
import type { Catalog } from '@/api'
import { parseIdList, parseParams } from '@/features/catalogs/params'
import type { RecipeNames } from './recipe'

interface Named {
  id: number
  name: string
}

/** The ids of every list, once each. */
function idsOf(...lists: Array<string | undefined>): number[] {
  return [...new Set(lists.flatMap((raw) => parseIdList(raw).ids))]
}

/** What a lookup has answered so far, and whether an answer is still on its way. */
interface Answered {
  names: ReadonlyMap<number, string>
  loading: boolean
}

/** The names of `ids`, as far as the by-id lookups have answered. They are the
 *  catalog editor's own (`TMDBEntityPicker`), under the same query keys, so a
 *  name the editor loaded is here already. */
function useNames(
  ids: number[],
  key: (id: number) => readonly unknown[],
  fetchOne: (id: number) => Promise<Named>,
  enabled: boolean,
): Answered {
  const answers = useQueries({
    queries: ids.map((id) => ({ queryKey: key(id), queryFn: () => fetchOne(id), staleTime: Infinity, enabled })),
  })
  return {
    names: new Map(answers.flatMap((answer, index) => (answer.data ? [[ids[index], answer.data.name] as const] : []))),
    loading: answers.some((answer) => answer.isLoading),
  }
}

/** The streaming services' names, from the service list of the recipe's region
 *  (all of TMDB's when it sets none), which is the picker's own list. */
function useProviderNames(catalog: Pick<Catalog, 'type'>, region: string, ids: number[], enabled: boolean): Answered {
  const providers = useQuery({
    queryKey: queryKeys.watchProviders(catalog.type, region),
    queryFn: () => fetchWatchProviders(catalog.type, region),
    enabled: enabled && ids.length > 0,
    staleTime: Infinity,
  })
  return {
    names: new Map((providers.data ?? []).map((provider) => [provider.provider_id, provider.provider_name])),
    loading: providers.isLoading,
  }
}

/**
 * The names of a recipe's studios, keywords, networks and streaming services,
 * for `recipeFacts`. They load from the lookups the catalog editor's pickers
 * use, once `wanted` says they are shown: a folded catalog in a collection's
 * folder asks for none until it opens. Until a list has all its names,
 * `recipeFacts` counts it; `loading` says a lookup is still answering, so a
 * count shown meanwhile is not final.
 */
export function useRecipeNames(
  catalog: Pick<Catalog, 'type' | 'params'>,
  display: { foldable: boolean; open: boolean },
): RecipeNames & { loading: boolean } {
  const wanted = !display.foldable || display.open
  const p = parseParams(catalog.params)
  const providers = idsOf(p.with_watch_providers)
  const company = useNames(idsOf(p.with_companies, p.without_companies), queryKeys.company, fetchCompany, wanted)
  const keyword = useNames(idsOf(p.with_keywords, p.without_keywords), queryKeys.keyword, fetchKeyword, wanted)
  const network = useNames(idsOf(catalog.type === 'series' ? p.with_networks : undefined), queryKeys.network, fetchNetwork, wanted)
  const provider = useProviderNames(catalog, p.watch_region ?? '', providers, wanted)
  return {
    company: company.names,
    keyword: keyword.names,
    network: network.names,
    provider: provider.names,
    loading: [company, keyword, network, provider].some((answered) => answered.loading),
  }
}
