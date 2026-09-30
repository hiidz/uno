import { useQuery } from '@tanstack/react-query'
import { TriangleAlert } from 'lucide-react'
import { fetchTMDBKey, queryKeys } from '@/api'
import type { TMDBKeyStatus } from '@/api'
import { Icon } from '@/components/Icon'
import { useKeyProblem } from './keyProblem'

/** The banner's words for an account whose saved key TMDB refuses, for one
 *  that has none, and for when asking which failed. */
const WORDS = {
  rejected: { problem: 'TMDB didn’t accept your key.', fix: 'replace it', button: 'Replace key' },
  missing: { problem: 'This account has no TMDB key.', fix: 'add one', button: 'Add key' },
  unknown: { problem: 'Uno couldn’t use your TMDB key.', fix: 'fix it', button: 'Check key' },
}

/** The words for status, the account's key as last read; none when the read
 *  failed. */
function bannerWords(status: TMDBKeyStatus | undefined) {
  if (!status) return WORDS.unknown
  if (status.set) return WORDS.rejected
  return WORDS.missing
}

/**
 * Under the builder's header once a call has found the account's own TMDB key
 * unusable. The key is what the home screen's rows load with as well, so this
 * stays up until a key is saved, rather than going with the call that found
 * it. The key lives on the profile picker; `onFix` goes there through the
 * header's own guard. Whether the account has a key at all is read afresh, to
 * say replace or add; if that read fails, the banner still shows, in words
 * that fit either.
 */
export function KeyProblemBanner({ onFix }: { onFix: () => void }) {
  const problem = useKeyProblem()
  const key = useQuery({
    queryKey: queryKeys.tmdbKey(),
    queryFn: fetchTMDBKey,
    enabled: problem,
    refetchOnMount: 'always',
  })
  if (!problem || key.isLoading) return null

  const words = bannerWords(key.data)
  return (
    <div
      role="status"
      className="bg-raised border-line flex flex-wrap items-center gap-x-3 gap-y-2 border-b px-4 py-2.5 lg:px-5"
    >
      <Icon icon={TriangleAlert} className="text-danger shrink-0" />
      <p className="m-0 min-w-0 flex-1 text-[14px] font-semibold">
        {words.problem} Your home screen can’t load Uno’s rows until you {words.fix}.
      </p>
      <button type="button" className="btn-secondary btn-sm" onClick={onFix}>
        {words.button}
      </button>
    </div>
  )
}
