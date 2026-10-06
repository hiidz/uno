import { ApiError } from '@/api/http'

/** A failure as the import dialog shows it: what happened in plain words, and
 *  the technical text beneath it, empty when there is none. */
export interface ImportProblem {
  headline: string
  detail: string
}

/** The headline of a failed `/import/check`, by status; any other status
 *  gets `CHECK_FAILED`. */
const CHECK_HEADLINES: Record<number, string> = {
  400: "This JSON isn't a bundle Uno can import.",
  502: "TMDB couldn't be reached to check the filters. Try again in a moment.",
}

const CHECK_FAILED = "Couldn't check this JSON. Try again."

/** A failed `/import/check`, with the server's own words beneath. */
export function checkProblem(err: Error): ImportProblem {
  const headline = err instanceof ApiError ? CHECK_HEADLINES[err.status] : undefined
  return { headline: headline ?? CHECK_FAILED, detail: err.message }
}

/** A failed import. It wrote nothing: the import is one transaction. */
export function writeProblem(err: Error): ImportProblem {
  return { headline: "Couldn't import. Nothing was added.", detail: err.message }
}
