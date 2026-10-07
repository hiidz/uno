/** What Show more needs from Community's paged query. */
export interface MorePages {
  hasNextPage: boolean
  isFetchingNextPage: boolean
  isFetchNextPageError: boolean
  fetchNextPage(): unknown
}

/** Under Community's rows while another page follows: reads it, says so while
 *  it loads, and offers it again when it fails. */
export function ShowMore({ pages }: { pages: MorePages }) {
  if (!pages.hasNextPage) return null
  return (
    <div className="flex flex-col items-start gap-2 pt-2">
      {pages.isFetchNextPageError && <p className="type-data text-danger m-0 text-[12.5px]">Couldn’t load more.</p>}
      <button
        type="button"
        onClick={() => void pages.fetchNextPage()}
        disabled={pages.isFetchingNextPage}
        className="btn-secondary"
      >
        {moreLabel(pages)}
      </button>
    </div>
  )
}

function moreLabel(pages: MorePages): string {
  if (pages.isFetchingNextPage) return 'Loading…'
  if (pages.isFetchNextPageError) return 'Try again'
  return 'Show more'
}
