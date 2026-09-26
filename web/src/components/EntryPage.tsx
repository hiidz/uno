import type { ReactNode } from 'react'
import { Fascia } from './Fascia'
import { Wordmark } from './Wordmark'

/**
 * The way in — the sign-in page and the profile picker: the shop's fascia
 * across the top, then the wordmark, the page's question in sign lettering,
 * and one line of intro above whatever the page asks for. `wide` is the
 * picker's, whose membership cards sit two across.
 */
export function EntryPage({
  title,
  intro,
  wide = false,
  children,
}: {
  title: ReactNode
  intro: ReactNode
  wide?: boolean
  children: ReactNode
}) {
  return (
    <div className="min-h-svh">
      <Fascia className="h-2" />
      <main
        className={`mx-auto w-full px-4 pt-[14vh] pb-16 max-sm:pt-[9vh] max-sm:pb-40 ${
          wide ? 'max-w-[640px]' : 'max-w-[480px]'
        }`}
      >
        <header className={`grid gap-5 ${wide ? 'mb-9' : 'mb-8'}`}>
          <h1 className="m-0 justify-self-start">
            <Wordmark className="text-ink block h-[26px] w-auto max-sm:h-[20px]" />
          </h1>
          <h2
            className={`type-sign m-0 leading-[1.05] ${
              wide ? 'text-[38px] max-sm:text-[27px]' : 'text-[34px] max-sm:text-[25px]'
            }`}
          >
            {title}
          </h2>
          <p className="text-dim m-0 max-w-[44ch] text-[16px] leading-[1.5]">{intro}</p>
        </header>
        {children}
      </main>
    </div>
  )
}
