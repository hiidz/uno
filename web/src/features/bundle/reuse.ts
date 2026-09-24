import type { ImportMatch } from '@/api'

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
export function choicesForAll(matches: ImportMatch[], useExisting: boolean): ReuseChoices {
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
export function reuseMap(matches: ImportMatch[], choices: ReuseChoices): Record<string, string> {
  const reuse: Record<string, string> = {}
  for (const match of matches) {
    const choice = choices[match.key]
    if (choice?.useExisting && match.existing.some((e) => e.id === choice.existingID)) {
      reuse[match.key] = choice.existingID
    }
  }
  return reuse
}
