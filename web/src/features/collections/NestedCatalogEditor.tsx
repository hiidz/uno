import type { CSSProperties } from 'react'
import type { Catalog, CertificationsByCountry, Genre, Language } from '@/api'
import { Modal } from '@/components/Modal'
import { DiscardPrompt, EditorGuardProvider, useEditorGuard } from '@/features/builder/EditorGuard'
import { CatalogEditor } from '@/features/catalogs/CatalogEditor'
import type { CatalogFormState } from '@/features/catalogs/catalogForm'
import type { CountryLookup } from '@/features/catalogs/countries'
import { isDraftCatalogID } from './collectionForm'

/** What the nested editor's Done hands back to the collection's form. */
type StageCatalog = (state: CatalogFormState) => void

/**
 * A scoped catalog opened one level down from its collection, in a modal over
 * the collection's editor. Its Done stages the edit in the collection's form
 * and writes nothing. Its own unsaved edits aren't the pane's, so it guards
 * its exits — ×, Close, Escape, the scrim — with a provider of its own, asking
 * the same "Discard unsaved changes?" the pane does.
 */
export function NestedCatalogEditor(props: NestedCatalogProps) {
  return (
    <EditorGuardProvider>
      <NestedCatalogModal {...props} />
    </EditorGuardProvider>
  )
}

interface NestedCatalogProps {
  catalog: Catalog
  initial: CatalogFormState
  genres: { movie: Genre[]; tv: Genre[] }
  certifications: { movie: CertificationsByCountry; tv: CertificationsByCountry }
  countryNames: CountryLookup
  languages: Language[]
  onSave: StageCatalog
  onClose: () => void
}

function NestedCatalogModal({
  catalog,
  initial,
  genres,
  certifications,
  countryNames,
  languages,
  onSave,
  onClose,
}: NestedCatalogProps) {
  const { guard, setDirty } = useEditorGuard()
  const close = () => guard(onClose)

  return (
    <Modal open onClose={close} labelledBy="nested-catalog-title" width="min(860px, 100%)">
      {/* Resets the sticky offset `EditorShell` computes for the outer app
          header — inside this modal there is no such header to clear, and
          inheriting the real one would leave a stray gap once the form
          scrolls on a narrow screen. `flex` plus `overflow-hidden` gives
          `EditorShell`'s own `lg:h-full` a bounded parent, the same shape the
          real app shell gives it, so its internal header/footer stay put and
          only the form between them scrolls. */}
      <div style={{ '--app-h': '0px' } as CSSProperties} className="flex max-h-[85vh] flex-col overflow-hidden">
        <h2 id="nested-catalog-title" className="sr-only">
          Edit {catalog.name}
        </h2>
        <CatalogEditor
          key={catalog.id}
          initial={initial}
          genres={genres}
          certifications={certifications}
          countryNames={countryNames}
          languages={languages}
          saving={false}
          serverError={null}
          onSave={onSave}
          onRequestClose={close}
          onDirtyChange={setDirty}
          canMoveToLibrary={!isDraftCatalogID(catalog.id)}
          saveLabel="Done"
        />
      </div>
      <DiscardPrompt subject={catalog.name} />
    </Modal>
  )
}
