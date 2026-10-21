import { Link } from 'react-router-dom'
import { EntryPage } from '@/components/EntryPage'
import { Button } from '@/components/Button'

/**
 * The page for an address the router has no route for (`missing`), and the
 * router's `errorElement` for a render or loader that threw (`crashed`). Both
 * lead home, which sends a signed-in user on to their profiles.
 */
export function ErrorPage({ kind }: { kind: 'missing' | 'crashed' }) {
  const missing = kind === 'missing'
  return (
    <EntryPage
      title={missing ? "This page doesn't exist" : 'Something went wrong'}
      intro={
        missing
          ? 'The address may be mistyped, or the page has moved.'
          : 'Uno hit a problem showing this page. Reloading usually fixes it.'
      }
    >
      <div className="flex flex-wrap gap-3 max-sm:flex-col">
        {!missing && (
          <Button kind="primary" onClick={() => window.location.reload()}>
            Reload
          </Button>
        )}
        <Link to="/" className={missing ? 'btn-primary' : 'btn-secondary'}>
          Go to Uno&rsquo;s home
        </Link>
      </div>
    </EntryPage>
  )
}
