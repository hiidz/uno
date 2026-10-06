import { useState, type Dispatch, type ReactNode } from 'react'
import type { Catalog, Collection } from '@/api'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import type { SignStep } from '@/components/PaneSign'
import type { ToastMessage } from '@/components/useToast'
import type { GenreLookups } from '@/features/library/useLibrary'
import { SinceLastPublished } from './Changes'
import { PublishDialog, type PublishSubject } from './PublishDialog'
import { SharingStickers } from './SharingStickers'
import { UnpublishedNotice } from './UnpublishedNotice'
import { errorText, ownSharing, publishGroups, rowStickers, sharingStep, type OwnSharing } from './sharingState'
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

/** What an open editor shows of its own row's sharing: its next step as the
 *  sign's button, its stickers, and the line its body leads with once its
 *  publisher unpublished it. */
export interface EditorSharing {
  sharingStep: SignStep
  sharingBadges: ReactNode
  sharingNotice: ReactNode
}

/** The two things the sign's button can do. */
interface StepActions {
  publish: () => void
  unpublish: () => void
}

/** What the sign's button does: a live row's next step is Unpublish…, which
 *  asks first; every other is a publish, through its dialog. */
function stepAction(state: OwnSharing, actions: StepActions) {
  return state === 'live' ? actions.unpublish : actions.publish
}

/** A library row as the editor's sharing reads it. */
type SharedRow = Named & Pick<Catalog, 'publication' | 'subscription' | 'publisher_unpublished'>

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
 * sticker and To push) and its next step with Community as the sign's button,
 * with the dialogs that step opens: publish and unpublish. A row added from
 * Community has no editor, so none of this applies to it
 * (`FromCommunityView`). Every sharing call refreshes the library and
 * Community (`useSharingMutations`), and says what it did through `onToast`.
 * Community holds the saved row, so the step waits while `dirty`.
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

  /** The publish dialog's own Unpublish: the dialog gives way to the
   *  Unpublish confirmation. */
  function unpublishInstead(row: Named) {
    setPublishing(null)
    askToUnpublish(row)
  }

  function editorSharing(row: SharedRow, subject: PublishSubject): EditorSharing {
    const state = ownSharing(row.publication)
    const onPublish = () => publish({ kind: row.kind, id: row.id, name: row.name, subject, update: state === 'changed' })
    const onUnpublish = () => askToUnpublish(row)
    return {
      sharingStep: { ...sharingStep(state, dirty), onClick: stepAction(state, { publish: onPublish, unpublish: onUnpublish }) },
      sharingBadges: <SharingStickers stickers={rowStickers(row, waitingForPush.has(row.id))} />,
      sharingNotice: <UnpublishedNotice row={row} />,
    }
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
          onUnpublish={() => unpublishInstead(publishing)}
        />
      )}
      {unpublishing && (
        <ConfirmDialog
          open
          title={`Unpublish “${unpublishing.name}”?`}
          body="Community stops listing it. People who added it keep it as their own and won’t get your updates, even if you publish it again."
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
