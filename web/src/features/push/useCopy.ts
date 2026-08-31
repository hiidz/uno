import { useState } from 'react'

/**
 * Copy-to-clipboard, returning the label to show. Clipboard access can fail
 * outright (insecure origin, denied permission), so the "copied" confirmation
 * is only shown once the write actually resolves.
 *
 * Its own module because two features reach for it: the push controls, and
 * `ProfileMenu`, which owns the addon URL below `lg`.
 */
export function useCopy(url: string): { copied: boolean; copy: () => void } {
  const [copied, setCopied] = useState(false)

  function copy() {
    navigator.clipboard
      .writeText(url)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 1600)
      })
      .catch(() => {
        // Nothing to recover from — just don't claim it copied.
      })
  }

  return { copied, copy }
}
