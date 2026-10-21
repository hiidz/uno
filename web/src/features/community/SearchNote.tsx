/** Under the search box: why the search isn't sent, while it isn't. */
export function SearchNote({ problem }: { problem: string }) {
  if (!problem) return null
  return (
    <p role="status" className="type-data text-danger text-meta m-0">
      {problem}
    </p>
  )
}
