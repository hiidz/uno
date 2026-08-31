import type { Country } from '@/api'

/**
 * Names a TMDB certification key the way a person reads it. The
 * certification response (`GET /api/certifications/{type}`) is keyed by
 * code, and a dropdown of "AU / BR / DE / GB" asks the user to know the
 * table — so codes are resolved against TMDB's own country table
 * (`GET /api/countries`, `/configuration/countries`) rather than a shipped
 * list: TMDB adds rating boards, and a stale table would silently show a raw
 * code for the new one.
 */

export type CountryLookup = ReadonlyMap<string, string>

export function buildCountryLookup(countries: Country[]): CountryLookup {
  return new Map(countries.map((c) => [c.iso_3166_1, c.english_name]))
}

export function countryName(code: string, lookup: CountryLookup): string {
  // `CA-QC` → "Canada (QC)": the parent country is what someone scans the list
  // for, and the subdivision is what distinguishes it from the entry above.
  const [country, subdivision] = code.split('-')
  if (subdivision) return `${lookup.get(country) ?? country} (${subdivision})`
  return lookup.get(code) ?? code
}
