import { Fascia } from '@/components/Fascia'
import { SkeletonBar } from '@/components/Skeleton'
import { Wordmark } from '@/components/Wordmark'
import { builderView } from '@/features/builder/paths'

/** The builder's header row, shared by the header and its stand-in. One row
 *  at every phone width; below 18rem, which a phone reaches only zoomed in,
 *  Push and its status wrap to a second line rather than run off the screen. */
export const HEADER_ROW =
  'bg-ground border-line flex min-h-[55px] items-center gap-3 border-b px-4 py-2 max-[18rem]:flex-wrap lg:h-[60px] lg:gap-4 lg:px-5 lg:py-0'

export const HEADER_WORDMARK = 'text-ink h-[14px] w-auto shrink-0 lg:h-[16px]'

/** The row the section tabs take under the header below `lg`. */
export const TABS_ROW = 'border-line bg-ground border-b px-4 py-2 lg:hidden'

/**
 * The builder before its profile has loaded: the header and, below `lg`, the
 * tab row at their own heights, with bars where the profile chip, the tabs
 * and Push go, over an empty `main`, so nothing moves when the profile fills
 * in.
 */
export function BuilderShellSkeleton() {
  return (
    <div className="flex min-h-svh flex-col lg:h-svh">
      <header className={HEADER_ROW}>
        <Wordmark className={HEADER_WORDMARK} />
        <SkeletonBar className="h-[34px] w-24 rounded-full" />
        <SkeletonBar className="hidden h-[38px] w-[216px] rounded-full lg:block" />
        <SkeletonBar className="ml-auto h-11 w-[130px] rounded-full lg:h-[38px] lg:w-[170px]" />
      </header>
      <div className={TABS_ROW}>
        <SkeletonBar className="h-[46px] w-[216px] rounded-full" />
      </div>
      <main aria-busy="true" className="flex-1">
        <p className="sr-only">Loading…</p>
      </main>
    </div>
  )
}

/** What the page draws while the startup sign-in check runs, before any
 *  route can: the builder's shell on a builder address, else the entry
 *  pages' fascia. */
export function StartupShell() {
  if (builderView(window.location.pathname)) return <BuilderShellSkeleton />
  return (
    <div className="min-h-svh">
      <Fascia className="h-2" />
    </div>
  )
}
