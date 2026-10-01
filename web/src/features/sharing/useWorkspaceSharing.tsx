import { useState, type ComponentProps, type ReactNode } from 'react'
import type { Catalog, Collection, SubscriptionState } from '@/api'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import type { ToastMessage } from '@/components/useToast'
import { catalogTarget, collectionTarget, type EditorTarget } from '@/features/builder/target'
import type { OpenPublication } from '@/features/community/communityQuery'
import type { GenreLookups } from '@/features/library/useLibrary'
import { FromCommunityRow } from './FromCommunityRow'
import { PublishDialog, type PublishSubject } from './PublishDialog'
import { SharingRow } from './SharingRow'
import { SharingStickers } from './SharingStickers'
import { errorText, ownSharing, publishGroups, rowStickers } from './sharingState'
import { useSharingMutations, type SharingTarget } from './useSharingMutations'

/** A row the workspace shares or detaches: which one, and its name. */
interface Named extends SharingTarget {
  name: string
}

/** A publish waiting on its dialog: the row, what it shares, and whether it
 *  publishes changes to something already shared. */
interface Publishing extends Named {
  subject: PublishSubject
  update: boolean
}

/** A question about a row, asked before stopping sharing it or detaching it.
 *  `withdrawn` is true for a copy whose owner has stopped sharing it. */
type Asking = { kind: 'withdraw' | 'detach'; row: Named; withdrawn: boolean }

/** A save of a copy waiting on its question: the copy's name, whether its
 *  owner has stopped sharing it, and the save. */
interface SavingCopy {
  name: string
  withdrawn: boolean
  save: Action
}

/** Something a button does. */
type Action = () => void

/** Something done to a row, or with a value. */
type RowAction<Row> = (row: Row) => void

/** What an open editor shows of its own row's sharing. */
export interface EditorSharing {
  sharingRow: ReactNode
  sharingBadges: ReactNode
}

/** A library row as the editor's sharing reads it. */
type SharedRow = Named & Pick<Catalog, 'publication' | 'subscription'>

/**
 * The workspace's sharing: the stickers every row's editor shows, and its
 * sharing setting — the Sharing row of an own row, or the From Community row
 * of a copy taken from Community — with the dialogs they open: publish, stop
 * sharing, detach, and the question a copy's save asks first
 * (`confirmCopySave`). Every sharing call refreshes the library and Community
 * (`useSharingMutations`), and says what it did through `onToast`. Sharing
 * publishes the saved row, so it waits while `dirty`, and so does Detach,
 * since saving the changes detaches the copy too. A copy's Update… opens its
 * publication's page in Community through `onOpenPublication`, which shows
 * the new version and applies it. A detached copy reopens through
 * `onReopen`, its editor seeded from the row as the detach left it.
 */
export function useWorkspaceSharing({
  profileIndex,
  genres,
  dirty,
  onToast,
  onReopen,
  onOpenPublication,
}: {
  profileIndex: number
  genres: GenreLookups
  dirty: boolean
  onToast: (toast: ToastMessage) => void
  onReopen: (target: EditorTarget) => void
  onOpenPublication: (publication: OpenPublication) => void
}) {
  const onDone = (text: string) => onToast({ text, tone: 'success' })
  const mutations = useSharingMutations(profileIndex)
  const [publishing, setPublishing] = useState<Publishing | null>(null)
  const [asking, setAsking] = useState<Asking | null>(null)
  const [savingCopy, setSavingCopy] = useState<SavingCopy | null>(null)

  function share(row: Publishing) {
    mutations.publish.reset()
    setPublishing(row)
  }

  function ask(kind: Asking['kind'], row: Named, withdrawn: boolean) {
    mutations[kind].reset()
    setAsking({ kind, row, withdrawn })
  }

  function editorSharing(row: SharedRow, subject: PublishSubject, duplicate: Action): EditorSharing {
    return {
      sharingRow: sharingSetting(row, subject, duplicate),
      sharingBadges: <SharingStickers stickers={rowStickers(row)} />,
    }
  }

  /** A copy's From Community row, or an own row's Sharing row. */
  function sharingSetting(row: SharedRow, subject: PublishSubject, duplicate: Action): ReactNode {
    if (row.subscription) return copySetting(row, row.subscription, duplicate)
    const update = ownSharing(row.publication) === 'changed'
    return (
      <SharingRow
        publication={row.publication}
        dirty={dirty}
        onShare={() => share({ kind: row.kind, id: row.id, name: row.name, subject, update })}
        onStop={() => ask('withdraw', row, false)}
      />
    )
  }

  /** Undefined until the library lists a row that was just created, which
   *  has no sharing to show yet. `onDuplicate` is the row's Duplicate, which
   *  a copy's From Community row offers. */
  function catalogSharing<C extends Catalog>(catalog: C | undefined, onDuplicate: RowAction<C>): EditorSharing | undefined {
    if (!catalog) return undefined
    const row = { ...catalog, kind: 'catalog' as const }
    return editorSharing(row, { kind: 'catalog', name: catalog.name, catalog }, () => onDuplicate(catalog))
  }

  function collectionSharing<C extends Collection>(collection: C | undefined, onDuplicate: RowAction<C>): EditorSharing | undefined {
    if (!collection) return undefined
    const row = { ...collection, kind: 'collection' as const, name: collection.title }
    const subject: PublishSubject = {
      kind: 'collection',
      name: collection.title,
      folderCount: (collection.folders ?? []).length,
      ...publishGroups(collection),
    }
    return editorSharing(row, subject, () => onDuplicate(collection))
  }

  function copySetting(row: Named, subscription: SubscriptionState, duplicate: Action): ReactNode {
    return (
      <FromCommunityRow
        subscription={subscription}
        dirty={dirty}
        onUpdate={() => onOpenPublication({ id: subscription.publication_id, kind: row.kind })}
        onDetach={() => ask('detach', row, subscription.withdrawn)}
        onDuplicate={duplicate}
      />
    )
  }

  /**
   * Saves `value` through `save`, a save of `row`'s editor: at once for an
   * own row, and after asking for a copy taken from Community, which the save
   * makes the profile's own. `row` is undefined until the library lists a row
   * that was just created, which is always the profile's own.
   */
  function confirmCopySave<T>(row: Catalog | Collection | undefined, save: RowAction<T>, value: T) {
    if (!row?.subscription) {
      save(value)
      return
    }
    setSavingCopy({
      name: 'title' in row ? row.title : row.name,
      withdrawn: row.subscription.withdrawn,
      save: () => save(value),
    })
  }

  function confirmSave(copy: SavingCopy) {
    setSavingCopy(null)
    copy.save()
  }

  function confirmPublish(row: Publishing) {
    mutations.publish.mutate(row, {
      onSuccess: () => {
        setPublishing(null)
        onDone(row.update ? `Published your changes to “${row.name}”` : `Shared “${row.name}”`)
      },
    })
  }

  function answer({ kind, row }: Asking) {
    mutations[kind].mutate(row, {
      onSuccess: (result) => {
        setAsking(null)
        if (kind === 'detach') onReopen(targetOf(row.kind, result))
        onDone(kind === 'withdraw' ? `Stopped sharing “${row.name}”` : `Detached “${row.name}”`)
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
      {asking && (
        <ConfirmDialog
          open
          {...askProps(asking, mutations[asking.kind])}
          onConfirm={() => answer(asking)}
          onCancel={() => setAsking(null)}
        />
      )}
      {savingCopy && (
        <ConfirmDialog
          open
          title="Save and make it yours?"
          body={
            <>
              Saving your changes to <strong className="text-ink">{savingCopy.name}</strong> makes it yours, and it{' '}
              {updatesLost(savingCopy.withdrawn)}
            </>
          }
          confirmLabel="Save"
          cancelLabel="Keep editing"
          onConfirm={() => confirmSave(savingCopy)}
          onCancel={() => setSavingCopy(null)}
        />
      )}
    </>
  )

  return { catalogSharing, collectionSharing, confirmCopySave, dialogs }
}

/** A detached row as the pane's target, by its kind. */
function targetOf(kind: SharingTarget['kind'], row: Catalog | Collection): EditorTarget {
  return kind === 'catalog' ? catalogTarget(row as Catalog) : collectionTarget(row as Collection)
}

type ConfirmProps = Omit<ComponentProps<typeof ConfirmDialog>, 'open' | 'onConfirm' | 'onCancel'>

/** The question asked before stopping sharing a row, or detaching a copy. */
function askProps(
  { kind, row, withdrawn }: Asking,
  mutation: { isPending: boolean; error: Error | null },
): ConfirmProps {
  const common = { pending: mutation.isPending, error: errorText(mutation.error), cancelLabel: 'Cancel' }
  if (kind === 'withdraw') {
    return {
      ...common,
      title: `Stop sharing “${row.name}”?`,
      body: 'Community stops listing it. Copies people took stay theirs, and get no updates until you share it again.',
      confirmLabel: mutation.isPending ? 'Stopping…' : 'Stop sharing',
    }
  }
  return {
    ...common,
    title: `Detach “${row.name}”?`,
    body: `It becomes yours to share, and ${updatesLost(withdrawn)}`,
    confirmLabel: mutation.isPending ? 'Detaching…' : 'Detach',
  }
}

/** What a copy gives up in becoming the profile's own. A copy whose owner has
 *  stopped sharing it can't be taken again, but would follow the owner's
 *  updates if they shared it again. */
function updatesLost(withdrawn: boolean): string {
  return withdrawn
    ? 'won’t get its owner’s updates if they share it again.'
    : 'stops getting its owner’s updates. To follow them again, take it from Community as a new copy.'
}
