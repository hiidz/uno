import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { checkImport } from '@/api'
import type { ImportCheck, ImportResult } from '@/api'
import { choicesForAll, reuseMap, type ReuseChoices } from './reuse'
import { MAX_BUNDLE_BYTES, PASTED_JSON, parseBundleText, tooLargeMessage } from './text'
import { useImport } from './useImport'

export type ImportMode = 'file' | 'paste'

/** Bundle text and what to call it: the file's name, or `PASTED_JSON`. */
export interface Source {
  label: string
  text: string
}

/** The matches the check found, held until Import writes with the choices made. */
export interface Review {
  bundle: unknown
  check: ImportCheck
  choices: ReuseChoices
}

/**
 * The state of one opening of the import dialog. A file or pasted JSON is the
 * source; Validate (and picking a file) only parses it here. Import checks it
 * with the server and writes it at once, unless the check finds catalogs the
 * library already has: then the review holds the choices and the next Import
 * writes with them. Any edit, new file or mode switch drops the review, so a
 * choice never travels with a bundle it wasn't made for.
 */
export function useImportFlow(profileIndex: number, onImported: (result: ImportResult) => void) {
  const [mode, setMode] = useState<ImportMode>('file')
  const [pasted, setPasted] = useState('')
  const [file, setFile] = useState<Source | null>(null)
  const [valid, setValid] = useState(false)
  const [inputError, setInputError] = useState<string | null>(null)
  const [review, setReview] = useState<Review | null>(null)
  const checking = useMutation({
    mutationFn: (bundle: unknown) => checkImport(profileIndex, bundle),
  })
  const importing = useImport(profileIndex)

  const source: Source | null =
    mode === 'file' ? file : pasted.trim() === '' ? null : { label: PASTED_JSON, text: pasted }

  function clear() {
    setValid(false)
    setInputError(null)
    setReview(null)
    checking.reset()
    importing.reset()
  }

  /** Parses `from` alone, no request; the answer is shown under the field. */
  function validate(from: Source) {
    const parsed = parseBundleText(from.text, from.label)
    if (parsed.ok) setValid(true)
    else setInputError(parsed.message)
  }

  function write(bundle: unknown, reuse: Record<string, string>) {
    importing.mutate({ bundle, reuse }, { onSuccess: onImported })
  }

  function runImport() {
    if (review) {
      write(review.bundle, reuseMap(review.check.matches, review.choices))
      return
    }
    if (!source) return
    clear()
    const parsed = parseBundleText(source.text, source.label)
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

  async function pickFile(picked: File) {
    clear()
    if (picked.size > MAX_BUNDLE_BYTES) {
      setFile(null)
      setInputError(tooLargeMessage(picked.name))
      return
    }
    const next = { label: picked.name, text: await picked.text() }
    setFile(next)
    validate(next)
  }

  return {
    mode,
    pasted,
    file,
    source,
    valid,
    review,
    inputError,
    checkError: checking.error?.message ?? null,
    importError: importing.error?.message ?? null,
    checking: checking.isPending,
    importing: importing.isPending,
    changeMode(next: ImportMode) {
      clear()
      setMode(next)
    },
    edit(text: string) {
      clear()
      setPasted(text)
    },
    validate() {
      if (source) validate(source)
    },
    pickFile,
    setChoices: (choices: ReuseChoices) => setReview((held) => held && { ...held, choices }),
    runImport,
  }
}
