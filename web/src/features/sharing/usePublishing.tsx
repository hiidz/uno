import { useState, type Dispatch } from 'react'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import type { ToastInput } from '@/components/toastQueue'
import type { GenreLookups } from '@/features/library/useLibrary'
import { SinceLastPublished } from './Changes'
import { PublishDialog, type PublishSubject } from './PublishDialog'
import { errorText, ownName, ownSharing, publishGroups, type OwnRow } from './sharingState'
import { useSharingMutations, type SharingTarget } from './useSharingMutations'

/** A row Community publishes: which one, and its name. */
interface Named extends SharingTarget {
  name: string
}

/** A publish waiting on its dialog: the row, what it publishes, and whether
 *  it publishes changes to something already published. */
interface Publishing extends Named {
  subject: PublishSubject
  update: boolean
}

function subjectOf(own: OwnRow): PublishSubject {
  if (own.kind === 'catalog') return { kind: 'catalog', name: own.row.name, catalog: own.row }
  return {
    kind: 'collection',
    name: own.row.title,
    folderCount: (own.row.folders ?? []).length,
    ...publishGroups(own.row),
  }
}

interface PublishingOptions {
  profileIndex: number
  genres: GenreLookups
  onToast: Dispatch<ToastInput>
  /** After an unpublish lands: the row's publication is gone. */
  onUnpublished?: () => void
}

/**
 * Publishing a profile's own rows, which happens in Community: `publish`
 * opens the publish dialog (first publish, or the changes since the last
 * one), `unpublish` asks first. Every call refreshes the library and
 * Community (`useSharingMutations`) and says what it did through `onToast`;
 * `dialogs` is what the caller renders.
 */
export function usePublishing({ profileIndex, genres, onToast, onUnpublished }: PublishingOptions) {
  const onDone = (text: string) => onToast({ text, tone: 'success' })
  const mutations = useSharingMutations(profileIndex)
  const [publishing, setPublishing] = useState<Publishing | null>(null)
  const [unpublishing, setUnpublishing] = useState<Named | null>(null)

  function publish(own: OwnRow) {
    mutations.publish.reset()
    setPublishing({
      kind: own.kind,
      id: own.row.id,
      name: ownName(own),
      subject: subjectOf(own),
      update: ownSharing(own.row.publication) === 'changed',
    })
  }

  function unpublish(own: OwnRow) {
    mutations.unpublish.reset()
    setUnpublishing({ kind: own.kind, id: own.row.id, name: ownName(own) })
  }

  /** The publish dialog's own Unpublish: the dialog gives way to the
   *  Unpublish confirmation. */
  function unpublishInstead(row: Named) {
    setPublishing(null)
    mutations.unpublish.reset()
    setUnpublishing(row)
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
        onUnpublished?.()
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

  return { publish, unpublish, dialogs }
}
