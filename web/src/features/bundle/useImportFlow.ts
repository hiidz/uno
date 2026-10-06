import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { checkImport } from '@/api'
import type { ImportCheck, ImportResult } from '@/api'
import { choicesForAll, liveMatches, reuseMap, type ReuseChoices } from './reuse'
import { checkProblem, writeProblem, type ImportProblem } from './problem'
import { OVERSIZE, parseBundleText, surelyOversize } from './text'
import { useImport } from './useImport'

/** What the check found, held until Import writes with the choices made:
 *  `choices` for the matched catalogs, `skip` the positions of the matched
 *  collections left out. */
export interface Review {
  bundle: unknown
  check: ImportCheck
  choices: ReuseChoices
  skip: number[]
}

/**
 * The state of one opening of the import dialog. The pasted JSON is the
 * source; Validate only parses it here. Import checks it with the server,
 * and the review then lists what it holds with a choice on each match; the
 * next Import writes with the choices made, so nothing is written before it
 * has been shown. While the review is up the field is put away; going back to it drops the
 * review, so a choice never travels with a bundle it wasn't made for.
 */
export function useImportFlow(profileIndex: number, onImported: (result: ImportResult) => void) {
  const [pasted, setPasted] = useState('')
  const [valid, setValid] = useState(false)
  const [inputError, setInputError] = useState<ImportProblem | null>(null)
  const [review, setReview] = useState<Review | null>(null)
  const checking = useMutation({
    mutationFn: (bundle: unknown) => checkImport(profileIndex, bundle),
  })
  const importing = useImport(profileIndex)

  const hasText = pasted.trim() !== ''

  function clear() {
    setValid(false)
    setInputError(null)
    setReview(null)
    checking.reset()
    importing.reset()
  }

  /** An edit of the field. Text plainly over the limit never enters it: the
   *  field keeps what it held and says why, so a huge paste costs nothing
   *  past the refusal. */
  function accept(text: string) {
    clear()
    if (surelyOversize(text)) {
      setInputError(OVERSIZE)
      return
    }
    setPasted(text)
  }

  function write(bundle: unknown, reuse: Record<string, string>, skip: number[]) {
    importing.mutate({ bundle, reuse, skip }, { onSuccess: onImported })
  }

  function runImport() {
    if (review) {
      const live = liveMatches(review.check, review.skip)
      write(review.bundle, reuseMap(live, review.choices), review.skip)
      return
    }
    if (!hasText) return
    clear()
    const parsed = parseBundleText(pasted)
    if (!parsed.ok) {
      setInputError(parsed.problem)
      return
    }
    const { bundle } = parsed
    checking.mutate(bundle, {
      onSuccess: (check) =>
        setReview({ bundle, check, choices: choicesForAll(liveMatches(check, []), false), skip: [] }),
    })
  }

  return {
    pasted,
    hasText,
    valid,
    review,
    inputError,
    checkError: checking.error ? checkProblem(checking.error) : null,
    importError: importing.error ? writeProblem(importing.error) : null,
    checking: checking.isPending,
    importing: importing.isPending,
    edit: accept,
    editAgain: clear,
    validate() {
      const parsed = parseBundleText(pasted)
      if (parsed.ok) setValid(true)
      else setInputError(parsed.problem)
    },
    setChoices: (choices: ReuseChoices) => setReview((held) => held && { ...held, choices }),
    setSkip: (skip: number[]) => setReview((held) => held && { ...held, skip }),
    runImport,
  }
}
