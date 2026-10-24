import type { CatalogType } from '@/api'

/**
 * TMDB's own lists a catalog can be made from (`tmdb_list`), per type, in the
 * order the tiles show them: each one's stored value (TMDB's own path word,
 * the server's `listEndpoints` key), its name, and the line under it.
 */
export interface TMDBListOption {
  value: string
  label: string
  line: string
}

const TRENDING: TMDBListOption[] = [
  { value: 'trending/day', label: 'Trending today', line: 'Most talked about today' },
  { value: 'trending/week', label: 'Trending this week', line: 'Most talked about this week' },
]

const TMDB_RANKED: TMDBListOption[] = [
  { value: 'popular', label: 'Popular', line: 'Most popular on TMDB' },
  { value: 'top_rated', label: 'Top rated', line: 'Highest rated on TMDB' },
]

export const TMDB_LISTS: Record<CatalogType, TMDBListOption[]> = {
  movie: [
    ...TRENDING,
    { value: 'now_playing', label: 'Now playing', line: 'In cinemas now' },
    { value: 'upcoming', label: 'Upcoming', line: 'Coming to cinemas soon' },
    ...TMDB_RANKED,
  ],
  series: [
    ...TRENDING,
    { value: 'airing_today', label: 'Airing today', line: 'New episodes today' },
    { value: 'on_the_air', label: 'On the air', line: 'New episodes this week' },
    ...TMDB_RANKED,
  ],
}

/** A stored list's name, or the stored value for one this type doesn't
 *  offer. */
export function tmdbListLabel(type: CatalogType, value: string): string {
  return TMDB_LISTS[type].find((list) => list.value === value)?.label ?? value
}
