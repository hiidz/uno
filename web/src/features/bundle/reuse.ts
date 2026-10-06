import type { ImportCatalog, ImportCheck } from '@/api'

/**
 * What the import dialog does with one matched bundle catalog: import a copy,
 * or point its refs at one of the existing catalogs the check matched.
 *
 * `existingID` is kept while `useExisting` is off, so switching a row to
 * "copy" and back returns to the catalog it had picked.
 */
export interface ReuseChoice {
  useExisting: boolean
  existingID: string
}

/** Keyed by bundle catalog key. */
export type ReuseChoices = Record<string, ReuseChoice>

/**
 * Every match set the same way, each picking its first existing catalog.
 * `existing` arrives sorted by name, so the first is the one listed first.
 * With `useExisting` off this is the dialog's starting state: every catalog
 * imported as a copy.
 */
export function choicesForAll(matches: ImportCatalog[], useExisting: boolean): ReuseChoices {
  const choices: ReuseChoices = {}
  for (const match of matches) {
    choices[match.key] = { useExisting, existingID: match.existing[0]?.id ?? '' }
  }
  return choices
}

/**
 * The `reuse` body of `POST .../import`: bundle key → existing catalog id,
 * for every match set to use an existing catalog. An id that isn't among the
 * match's own `existing` is left out, so the key imports as a copy.
 */
export function reuseMap(matches: ImportCatalog[], choices: ReuseChoices): Record<string, string> {
  const reuse: Record<string, string> = {}
  for (const match of matches) {
    const choice = choices[match.key]
    if (choice?.useExisting && match.existing.some((e) => e.id === choice.existingID)) {
      reuse[match.key] = choice.existingID
    }
  }
  return reuse
}

/** `skip` with the collection at `index` skipped or not. */
export function withSkip(skip: number[], index: number, skipped: boolean): number[] {
  const rest = skip.filter((i) => i !== index)
  if (skipped) rest.push(index)
  return rest
}

/** Every catalog still in the import: the top-level ones, and the own
 *  catalogs of every collection not skipped. */
export function liveCatalogs(check: ImportCheck, skip: number[]): ImportCatalog[] {
  const kept = check.collections.filter((_, index) => !skip.includes(index))
  return [...check.catalogs, ...kept.flatMap((collection) => collection.catalogs)]
}

/** The catalogs still in the import that match one of the library's. */
export function liveMatches(check: ImportCheck, skip: number[]): ImportCatalog[] {
  return liveCatalogs(check, skip).filter((catalog) => catalog.existing.length > 0)
}

/**
 * What an import with these choices adds: every catalog still in it except
 * the ones it reuses, and every collection not skipped.
 */
export function importTally(
  check: ImportCheck,
  choices: ReuseChoices,
  skip: number[],
): { catalogs: number; collections: number } {
  const live = liveCatalogs(check, skip)
  const reused = Object.keys(reuseMap(live, choices)).length
  return { catalogs: live.length - reused, collections: check.collections.length - skip.length }
}
