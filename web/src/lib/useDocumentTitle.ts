import { useEffect } from 'react'

/** Names the browser tab after what the page shows, as "`title` · Uno", and
 *  gives it back the plain "Uno" of `index.html` when the page goes. */
export function useDocumentTitle(title: string) {
  useEffect(() => {
    document.title = `${title} · Uno`
    return () => {
      document.title = 'Uno'
    }
  }, [title])
}
