// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { catalog, collection } from '@/test/fixtures'
import { LibrarySection } from './LibrarySection'
import type { Library } from './useLibrary'

vi.mock('@/features/home/useHomeSelection', () => ({
  useHomeSelection: () => ({ hasCatalog: () => false, hasCollection: () => false }),
}))

function mount() {
  const library = {
    catalogs: [catalog({ id: 'm', name: 'Heist', type: 'movie' }), catalog({ id: 's', name: 'Heist again', type: 'series' })],
    collections: [collection({ id: 'k', title: 'Weekend' })],
    genres: { movie: new Map(), tv: new Map() },
    isLoading: false,
    error: null,
    refetch: () => {},
  } as unknown as Library
  const noop = () => {}
  render(
    <LibrarySection
      library={library}
      selectedID={null}
      onNewCatalog={noop}
      onNewCollection={noop}
      onSelectCatalog={noop}
      onSelectCollection={noop}
      onDuplicateCatalog={noop}
      onDuplicateCollection={noop}
      onDeleteCatalog={noop}
      onDeleteCollection={noop}
      onImport={noop}
      onExport={noop}
    />,
  )
}

const filter = (text: string) => fireEvent.change(screen.getByLabelText('Filter your library'), { target: { value: text } })
const announced = () => screen.getByText((_, el) => el?.getAttribute('role') === 'status' && el.tagName === 'P')

describe('LibrarySection filter', () => {
  it('matches the Movies and Series stickers, and says what it leaves only while filtering', () => {
    mount()
    expect(announced()).toBeEmptyDOMElement()

    filter('series')
    expect(screen.getByText('Heist again')).toBeInTheDocument()
    expect(screen.queryByText('Heist')).not.toBeInTheDocument()
    expect(announced()).toHaveTextContent('1 catalog matches')

    filter('heist')
    expect(announced()).toHaveTextContent('2 catalogs match')

    filter('nothing like it')
    expect(announced()).toHaveTextContent('Nothing matches')

    filter('')
    expect(announced()).toBeEmptyDOMElement()
  })
})

describe('LibrarySection rows', () => {
  it('give a truncated name its full text as the row’s tooltip', () => {
    mount()
    expect(screen.getByRole('button', { name: /^Heist again/ })).toHaveAttribute('title', 'Heist again')
  })
})
