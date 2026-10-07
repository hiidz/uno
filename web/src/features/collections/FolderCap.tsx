import { FieldError } from '@/components/fields'
import { MAX_FOLDERS, MAX_REFS_PER_FOLDER } from './collectionForm'

/** Under the Folders heading: the folder-count error once errors show, else a
 *  line saying why Add folder is off at the cap. */
export function FolderCount({ count, error, showError }: { count: number; error: string | undefined; showError: boolean }) {
  if (showError && error) return <FieldError>{error}</FieldError>
  if (count < MAX_FOLDERS) return null
  return <p className="ed-note m-0">A collection holds at most {MAX_FOLDERS} folders.</p>
}

/** In the Add catalogs dropdown: why the unticked catalogs are off once the
 *  folder is full. */
export function RefCapNote({ full }: { full: boolean }) {
  if (!full) return null
  return (
    <p className="ed-note m-0 px-2 py-1">
      This folder is full: it holds at most {MAX_REFS_PER_FOLDER} catalogs, one split by genre counting once a genre.
    </p>
  )
}
