import { describe, expect, it, vi } from 'vitest'
import {
  ANY_KEYWORDS,
  companyDetail,
  formatWindowStart,
  sumAge,
  sumCollection,
  sumDate,
  sumEntities,
  sumGenres,
  sumLanguage,
  sumOrder,
  sumRatings,
  sumWatch,
  summaryClass,
  withShuffle,
} from './summary'

// The catalog form's one value import from the API barrel, which would
// otherwise load the auth session and its `window` listener.
vi.mock('@/api', () => ({ CATALOG_PROVIDER: 'tmdb' }))

const countries = new Map([
  ['US', 'United States of America'],
  ['CA', 'Canada'],
])

describe('sumOrder', () => {
  it('words the direction for the kind of field', () => {
    expect(sumOrder('movie', 'popularity', 'desc')).toBe('Popularity, highest first')
    expect(sumOrder('movie', 'title', 'asc')).toBe('Title, a to z')
    expect(sumOrder('series', 'first_air_date', 'asc')).toBe('First aired, oldest first')
  })

  it('reads an unset or unknown field as the first one', () => {
    expect(sumOrder('movie', '', 'desc')).toBe('Popularity, highest first')
    expect(sumOrder('series', 'revenue', 'desc')).toBe('Popularity, highest first')
  })
})

describe('sumGenres', () => {
  const genres = [
    { id: 28, name: 'Action' },
    { id: 35, name: 'Comedy' },
    { id: 18, name: 'Drama' },
  ]

  it('joins included genres the way the join reads', () => {
    expect(sumGenres(genres, [], 'and', [])).toBe('Any genre')
    expect(sumGenres(genres, [28, 35], 'and', [])).toBe('Action and Comedy')
    expect(sumGenres(genres, [28, 35, 18], 'or', [])).toBe('Action, Comedy or Drama')
  })

  it('lists left-out genres after, and falls back to the id before names load', () => {
    expect(sumGenres(genres, [28], 'and', [35, 99])).toBe('Action · not Comedy or 99')
    expect(sumGenres([], [], 'and', [18])).toBe('not 18')
  })
})

describe('sumRatings', () => {
  it('reads every unset bound as any', () => {
    expect(sumRatings(undefined, undefined, undefined, undefined, undefined, undefined)).toBe(
      'Any rating · any number of ratings · any length',
    )
  })

  it('reads one-sided and two-sided bounds', () => {
    expect(sumRatings(undefined, 6.5, 100, 2500, 90, undefined)).toBe(
      'Rated up to 6.5 · 100 to 2,500 ratings · 90 min or longer',
    )
  })

  it('reads a bound at or past the slider’s end as the number set', () => {
    expect(sumRatings(7, 10, 5000, undefined, undefined, 300)).toBe(
      'Rated 7.0 to 10 · 5,000 or more ratings · up to 300 min',
    )
    expect(sumRatings(undefined, undefined, 6000, undefined, 400, undefined)).toBe(
      'Any rating · 6,000 or more ratings · 400 min or longer',
    )
  })
})

describe('sumLanguage', () => {
  it('names the language, falling back to its code', () => {
    const languages = [{ iso_639_1: 'ko', english_name: 'Korean', name: '한국어/조선말' }]
    expect(sumLanguage(undefined, languages)).toBe('Any language')
    expect(sumLanguage('ko', languages)).toBe('In Korean')
    expect(sumLanguage('xx', languages)).toBe('In xx')
  })
})

describe('sumDate', () => {
  it('reads fixed dates, or says none are chosen', () => {
    expect(sumDate('movie', 'any', undefined, undefined, undefined)).toBe('Any time')
    expect(sumDate('movie', 'fixed', '2020-06-15', '2021-06-15', undefined)).toBe(
      'Released 15 Jun 2020 to 15 Jun 2021',
    )
    expect(sumDate('series', 'fixed', '2020-06-15', undefined, undefined)).toBe('First aired from 15 Jun 2020')
    expect(sumDate('movie', 'fixed', undefined, undefined, undefined)).toBe('Released — no dates chosen yet')
  })

  it('keeps a picked date on its day west of UTC', () => {
    vi.stubEnv('TZ', 'America/Los_Angeles')
    try {
      // Proves the zone took hold: this instant is still the 14th in Los Angeles.
      expect(new Date('2020-06-15').getDate()).toBe(14)
      expect(sumDate('movie', 'fixed', '2020-06-15', '2021-06-15', undefined)).toBe(
        'Released 15 Jun 2020 to 15 Jun 2021',
      )
    } finally {
      vi.unstubAllEnvs()
    }
  })

  it('names a rolling window by its preset, in years, or in days', () => {
    const since = (days: number) => `released since ${formatWindowStart(days)}, updated daily`
    expect(sumDate('movie', 'rolling', undefined, undefined, 30)).toBe(`Last 30 days · ${since(30)}`)
    expect(sumDate('movie', 'rolling', undefined, undefined, 730)).toBe(`Last 2 years · ${since(730)}`)
    expect(sumDate('movie', 'rolling', undefined, undefined, 45)).toBe(`Last 45 days · ${since(45)}`)
  })

  it('says a series window matches any episode aired, not the first', () => {
    expect(sumDate('series', 'rolling', undefined, undefined, 30)).toBe(
      `Last 30 days · aired since ${formatWindowStart(30)}, updated daily`,
    )
    expect(sumDate('series', 'rolling', undefined, undefined, undefined)).toBe(
      'Aired recently — no window chosen yet',
    )
  })

  it('reads the one-day window as upcoming, from the date the server asks for', () => {
    expect(sumDate('series', 'rolling', undefined, undefined, 1)).toBe(
      `Upcoming · airing from ${formatWindowStart(1)} on, updated daily`,
    )
  })

  it('counts a window back from the UTC date, as the server does', () => {
    vi.useFakeTimers({ now: new Date('2026-10-08T23:30:00Z') })
    vi.stubEnv('TZ', 'Asia/Singapore')
    try {
      // 07:30 on the 9th in Singapore, the 8th in UTC, so a day back is the 7th.
      const seventh = new Date('2026-10-07T12:00:00Z').toLocaleDateString(undefined, {
        day: 'numeric',
        month: 'short',
        year: 'numeric',
        timeZone: 'UTC',
      })
      expect(formatWindowStart(1)).toBe(seventh)
    } finally {
      vi.unstubAllEnvs()
      vi.useRealTimers()
    }
  })

  it('says when no window is chosen', () => {
    expect(sumDate('movie', 'rolling', undefined, undefined, undefined)).toBe(
      'Released recently — no window chosen yet',
    )
  })
})

describe('sumAge', () => {
  const scale = [
    { certification: 'R', meaning: '', order: 4 },
    { certification: 'G', meaning: '', order: 1 },
    { certification: 'PG', meaning: '', order: 2 },
  ]

  it('names the country, and a subdivision under it', () => {
    expect(sumAge(undefined, countries, undefined, undefined, scale)).toBe('Any age rating')
    expect(sumAge('US', countries, undefined, undefined, scale)).toBe('United States of America · any age rating')
    expect(sumAge('CA-QC', countries, undefined, undefined, [])).toBe('Canada (QC) · any age rating')
  })

  it('fills an open end of the range from the scale, in scale order', () => {
    expect(sumAge('US', countries, 'PG', undefined, scale)).toBe('United States of America · PG to R')
    expect(sumAge('US', countries, undefined, 'PG', scale)).toBe('United States of America · G to PG')
  })
})

describe('sumWatch', () => {
  it('counts picked services in the region', () => {
    expect(sumWatch(undefined, countries, 3)).toBe('Any service')
    expect(sumWatch('US', countries, 0)).toBe('United States of America · any service')
    expect(sumWatch('US', countries, 1)).toBe('United States of America · 1 streaming service')
    expect(sumWatch('XX', countries, 2)).toBe('XX · 2 streaming services')
  })
})

describe('sumEntities', () => {
  it('counts ids and names the join once there are two', () => {
    expect(sumEntities(undefined, undefined, 'production company', 'Any production company')).toBe('Any production company')
    expect(sumEntities('420', undefined, 'production company', 'Any production company')).toBe('1 production company')
    expect(sumEntities('420|2', undefined, 'production company', 'Any production company')).toBe('2 production companies, any of them')
    expect(sumEntities('420,2', '', 'keyword', 'Any keywords')).toBe('2 keywords, all of them')
  })

  it('counts the left-out ids after the included ones', () => {
    expect(sumEntities(undefined, '2', 'production company', 'Any production company')).toBe('not 1 production company')
    expect(sumEntities('420', '2,7', 'production company', 'Any production company')).toBe('1 production company · not 2 production companies')
    expect(sumEntities('420|3', '2', 'keyword', 'Any keywords')).toBe(
      '2 keywords, any of them · not 1 keyword',
    )
  })
})

describe('sumCollection', () => {
  it('names the one pick, falling back to its id until the name loads', () => {
    expect(sumCollection(undefined, undefined)).toBe('No TMDB collection picked')
    expect(sumCollection('', 'Star Wars Collection')).toBe('No TMDB collection picked')
    expect(sumCollection('10', 'Star Wars Collection')).toBe('Star Wars Collection')
    expect(sumCollection('10', undefined)).toBe('TMDB collection 10')
  })
})

describe('companyDetail', () => {
  it('names the country and counts titles of the catalog type', () => {
    expect(companyDetail({ origin_country: 'US', title_count: 176 }, 'movie')).toBe('US · 176 films')
    expect(companyDetail({ origin_country: 'GB', title_count: 1 }, 'movie')).toBe('GB · 1 film')
    expect(companyDetail({ origin_country: 'US', title_count: 11 }, 'series')).toBe('US · 11 series')
  })

  it('leaves out a missing country', () => {
    expect(companyDetail({ origin_country: '', title_count: 9 }, 'movie')).toBe('9 films')
  })
})

describe('summaryClass', () => {
  it('marks a head still reading as nothing set', () => {
    for (const summary of [
      sumGenres([], [], 'or', []),
      sumRatings(undefined, undefined, undefined, undefined, undefined, undefined),
      sumLanguage(undefined, []),
      sumDate('movie', 'any', undefined, undefined, undefined),
      sumAge(undefined, countries, undefined, undefined, []),
      sumWatch(undefined, countries, 0),
      sumEntities(undefined, undefined, 'keyword', ANY_KEYWORDS),
      sumCollection(undefined, undefined),
      sumOrder('series', '', 'desc'),
    ]) {
      expect(summaryClass(summary, 'movie')).toBe('sec-sum is-unset')
    }
  })

  it('marks a head the section narrows the row with', () => {
    expect(summaryClass('Action', 'movie')).toBe('sec-sum is-set')
    expect(summaryClass(sumOrder('movie', 'vote_average', 'desc'), 'movie')).toBe('sec-sum is-set')
    expect(summaryClass(withShuffle(sumOrder('movie', '', 'desc'), true), 'movie')).toBe('sec-sum is-set')
    expect(summaryClass(sumRatings(7, undefined, undefined, undefined, undefined, undefined), 'movie')).toBe(
      'sec-sum is-set',
    )
  })
})
