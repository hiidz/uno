import type { DeleteConsequences } from './deleteConsequences'
import { scopedCatalogsLine } from './deleteConsequences'

/** A delete confirm's body: the row named, then only the consequences that
 *  apply, then the warning. */
export function DeleteMessage({
  name,
  consequences,
}: {
  name: string
  consequences: DeleteConsequences
}) {
  const { removedFrom, addersKeep, scopedCatalogs } = consequences
  return (
    <div className="flex flex-col gap-2">
      <p className="m-0">
        <strong className="text-ink">{name}</strong> is deleted permanently.
      </p>
      {removedFrom && <p className="m-0">{removedFrom}</p>}
      {addersKeep && <p className="m-0">People who added it keep it.</p>}
      {scopedCatalogs > 0 && <p className="m-0">{scopedCatalogsLine(scopedCatalogs)}</p>}
      <p className="m-0">This can't be undone.</p>
    </div>
  )
}
