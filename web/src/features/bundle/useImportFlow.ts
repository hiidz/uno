import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { checkImport } from '@/api'
import type { ImportCheck, ImportResult } from '@/api'
import { choicesForAll, reuseMap, type ReuseChoices } from './reuse'
import { parseBundleText } from './text'
import { useImport } from './useImport'

/** The matches the check found, held until Import writes with the choices made. */
export interface Review {
  bundle: unknown
  check: ImportCheck
  choices: ReuseChoices
}

/**
 * The state of one opening of the import dialog. The pasted JSON is the
 * source; Validate only parses it here. Import checks it with the server and
 * writes it at once, unless the check finds catalogs the library already has:
 * then the review holds the choices and the next Import writes with them. Any
 * edit drops the review, so a choice never travels with a bundle it wasn't
 * made for.
 */
export function useImportFlow(profileIndex: number, onImported: (result: ImportResult) => void) {
  const [pasted, setPasted] = useState('')
  const [valid, setValid] = useState(false)
  const [inputError, setInputError] = useState<string | null>(null)
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

  function write(bundle: unknown, reuse: Record<string, string>) {
    importing.mutate({ bundle, reuse }, { onSuccess: onImported })
  }

  function runImport() {
    if (review) {
      write(review.bundle, reuseMap(review.check.matches, review.choices))
      return
    }
    if (!hasText) return
    clear()
    const parsed = parseBundleText(pasted)
    if (!parsed.ok) {
      setInputError(parsed.message)
      return
    }
    const { bundle } = parsed
    checking.mutate(bundle, {
      onSuccess: (check) =>
        check.matches.length === 0
          ? write(bundle, {})
          : setReview({ bundle, check, choices: choicesForAll(check.matches, false) }),
    })
  }

  return {
    pasted,
    hasText,
    valid,
    review,
    inputError,
    checkError: checking.error?.message ?? null,
    importError: importing.error?.message ?? null,
    checking: checking.isPending,
    importing: importing.isPending,
    edit(text: string) {
      clear()
      setPasted(text)
    },
    validate() {
      const parsed = parseBundleText(pasted)
      if (parsed.ok) setValid(true)
      else setInputError(parsed.message)
    },
    setChoices: (choices: ReuseChoices) => setReview((held) => held && { ...held, choices }),
    runImport,
  }
}
