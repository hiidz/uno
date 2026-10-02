import { useState, type Dispatch, type ReactNode } from 'react'
import type { Catalog, Collection } from '@/api'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import type { ToastMessage } from '@/components/useToast'
import type { GenreLookups } from '@/features/library/useLibrary'
import { SinceLastPublished } from './Changes'
import { PublishDialog, type PublishSubject } from './PublishDialog'
import { SharingRow } from './SharingRow'
import { SharingStickers } from './SharingStickers'
import { errorText, ownSharing, publishGroups, rowStickers } from './sharingState'
import { useSharingMutations, type SharingTarget } from './useSharingMutations'

/** A row the workspace publishes: which one, and its name. */
interface Named extends SharingTarget {
  name: string
}

/** A publish waiting on its dialog: the row, what it publishes, and whether
 *  it publishes changes to something already published. */
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
  /** The ids of the rows a push would change in Nuvio (`usePushWaiting`). */
  waitingForPush: ReadonlySet<string>
  onToast: Dispatch<ToastMessage>
}

/**
 * The workspace's sharing: every flag an own row's editor shows (its Community
 * sticker and Push to Nuvio) and its Community setting, with the dialogs it opens: publish and unpublish. A row
 * added from Community has no editor, so none of this applies to it
 * (`FromCommunityView`). Every sharing call refreshes the library and
 * Community (`useSharingMutations`), and says what it did through `onToast`.
 * Publishing publishes the saved row, so it waits while `dirty`.
 */
export function useWorkspaceSharing({ profileIndex, genres, dirty, waitingForPush, onToast }: WorkspaceSharingOptions) {
  const onDone = (text: string) => onToast({ text, tone: 'success' })
  const mutations = useSharingMutations(profileIndex)
  const [publishing, setPublishing] = useState<Publishing | null>(null)
  const [unpublishing, setUnpublishing] = useState<Named | null>(null)

  function publish(row: Publishing) {
    mutations.publish.reset()
    setPublishing(row)
  }

  function askToUnpublish(row: Named) {
    mutations.unpublish.reset()
    setUnpublishing(row)
  }

  function editorSharing(row: SharedRow, subject: PublishSubject): EditorSharing {
    const update = ownSharing(row.publication) === 'changed'
    const onPublish = () => publish({ kind: row.kind, id: row.id, name: row.name, subject, update })
    const sharingRow = (
      <SharingRow publication={row.publication} dirty={dirty} onPublish={onPublish} onUnpublish={() => askToUnpublish(row)} />
    )
    return { sharingRow, sharingBadges: <SharingStickers stickers={rowStickers(row, waitingForPush.has(row.id))} /> }
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
        onDone(row.update ? `Published your changes to “${row.name}”` : `Published “${row.name}”`)
      },
    })
  }

  function confirmUnpublish(row: Named) {
    mutations.unpublish.mutate(row, {
      onSuccess: () => {
        setUnpublishing(null)
        onDone(`Unpublished “${row.name}”`)
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
          since={
            <SinceLastPublished
              profileIndex={profileIndex}
              kind={publishing.kind}
              id={publishing.id}
              enabled={publishing.update}
              genres={genres}
            />
          }
          genres={genres}
          pending={mutations.publish.isPending}
          error={errorText(mutations.publish.error)}
          onConfirm={() => confirmPublish(publishing)}
          onClose={() => setPublishing(null)}
        />
      )}
      {unpublishing && (
        <ConfirmDialog
          open
          title={`Unpublish “${unpublishing.name}”?`}
          body="Community stops listing it. People who added it keep it, and get no updates until you publish it again."
          confirmLabel={mutations.unpublish.isPending ? 'Unpublishing…' : 'Unpublish'}
          cancelLabel="Cancel"
          pending={mutations.unpublish.isPending}
          error={errorText(mutations.unpublish.error)}
          onConfirm={() => confirmUnpublish(unpublishing)}
          onCancel={() => setUnpublishing(null)}
        />
      )}
    </>
  )

  return { catalogSharing, collectionSharing, dialogs }
}
