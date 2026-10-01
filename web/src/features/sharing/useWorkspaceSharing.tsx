import { useState, type Dispatch, type ReactNode } from 'react'
import type { Catalog, Collection } from '@/api'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import type { ToastMessage } from '@/components/useToast'
import type { GenreLookups } from '@/features/library/useLibrary'
import { PublishDialog, type PublishSubject } from './PublishDialog'
import { SharingRow } from './SharingRow'
import { SharingStickers } from './SharingStickers'
import { errorText, ownSharing, publishGroups, rowStickers } from './sharingState'
import { useSharingMutations, type SharingTarget } from './useSharingMutations'

/** A row the workspace shares: which one, and its name. */
interface Named extends SharingTarget {
  name: string
}

/** A publish waiting on its dialog: the row, what it shares, and whether it
 *  publishes changes to something already shared. */
interface Publishing extends Named {
  subject: PublishSubject
  update: boolean
}

/** What an open editor shows of its own row's sharing. */
export interface EditorSharing {
  sharingRow: ReactNode
  sharingBadges: ReactNode
}

/** A library row as the editor's sharing reads it. */
type SharedRow = Named & Pick<Catalog, 'publication' | 'subscription'>

interface WorkspaceSharingOptions {
  profileIndex: number
  genres: GenreLookups
  dirty: boolean
  onToast: Dispatch<ToastMessage>
}

/**
 * The workspace's sharing: the stickers an own row's editor shows and its
 * Sharing row, with the dialogs it opens: publish and stop sharing. A row
 * taken from Community has no editor, so none of this applies to it
 * (`FromCommunityView`). Every sharing call refreshes the library and
 * Community (`useSharingMutations`), and says what it did through `onToast`.
 * Sharing publishes the saved row, so it waits while `dirty`.
 */
export function useWorkspaceSharing({ profileIndex, genres, dirty, onToast }: WorkspaceSharingOptions) {
  const onDone = (text: string) => onToast({ text, tone: 'success' })
  const mutations = useSharingMutations(profileIndex)
  const [publishing, setPublishing] = useState<Publishing | null>(null)
  const [stopping, setStopping] = useState<Named | null>(null)

  function share(row: Publishing) {
    mutations.publish.reset()
    setPublishing(row)
  }

  function askToStop(row: Named) {
    mutations.withdraw.reset()
    setStopping(row)
  }

  function editorSharing(row: SharedRow, subject: PublishSubject): EditorSharing {
    const update = ownSharing(row.publication) === 'changed'
    const onShare = () => share({ kind: row.kind, id: row.id, name: row.name, subject, update })
    const sharingRow = <SharingRow publication={row.publication} dirty={dirty} onShare={onShare} onStop={() => askToStop(row)} />
    return { sharingRow, sharingBadges: <SharingStickers stickers={rowStickers(row)} /> }
  }

  /** Undefined until the library lists a row that was just created, which
   *  has no sharing to show yet. */
  function catalogSharing(catalog: Catalog | undefined): EditorSharing | undefined {
    if (!catalog) return undefined
    const row = { ...catalog, kind: 'catalog' as const }
    return editorSharing(row, { kind: 'catalog', name: catalog.name, catalog })
  }

  function collectionSharing(collection: Collection | undefined): EditorSharing | undefined {
    if (!collection) return undefined
    const row = { ...collection, kind: 'collection' as const, name: collection.title }
    const subject: PublishSubject = {
      kind: 'collection',
      name: collection.title,
      folderCount: (collection.folders ?? []).length,
      ...publishGroups(collection),
    }
    return editorSharing(row, subject)
  }

  function confirmPublish(row: Publishing) {
    mutations.publish.mutate(row, {
      onSuccess: () => {
        setPublishing(null)
        onDone(row.update ? `Published your changes to “${row.name}”` : `Shared “${row.name}”`)
      },
    })
  }

  function confirmStop(row: Named) {
    mutations.withdraw.mutate(row, {
      onSuccess: () => {
        setStopping(null)
        onDone(`Stopped sharing “${row.name}”`)
      },
    })
  }

  const dialogs = (
    <>
      {publishing && (
        <PublishDialog
          open
          subject={publishing.subject}
          update={publishing.update}
          genres={genres}
          pending={mutations.publish.isPending}
          error={errorText(mutations.publish.error)}
          onConfirm={() => confirmPublish(publishing)}
          onClose={() => setPublishing(null)}
        />
      )}
      {stopping && (
        <ConfirmDialog
          open
          title={`Stop sharing “${stopping.name}”?`}
          body="Community stops listing it. Copies people took stay theirs, and get no updates until you share it again."
          confirmLabel={mutations.withdraw.isPending ? 'Stopping…' : 'Stop sharing'}
          cancelLabel="Cancel"
          pending={mutations.withdraw.isPending}
          error={errorText(mutations.withdraw.error)}
          onConfirm={() => confirmStop(stopping)}
          onCancel={() => setStopping(null)}
        />
      )}
    </>
  )

  return { catalogSharing, collectionSharing, dialogs }
}
