// @vitest-environment jsdom
import type { UseQueryResult } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { SnapshotChange } from '@/api'
import { ChangeList, ChangesBlock } from './ChangeList'

const genres = { movie: new Map<number, string>(), tv: new Map<number, string>() }

function removed(name: string): SnapshotChange {
  return { op: 'removed', kind: 'catalog', name, folder: 'F' }
}

const eight = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'].map(removed)

describe('ChangeList', () => {
  it('shows six lines under their group’s label, then "and N more"', () => {
    render(<ChangeList changes={eight} genres={genres} />)
    expect(screen.getByRole('heading', { name: 'Removed' })).toBeInTheDocument()
    expect(screen.getAllByRole('listitem').map((item) => item.textContent)).toEqual([
      '“a” from “F”',
      '“b” from “F”',
      '“c” from “F”',
      '“d” from “F”',
      '“e” from “F”',
      '“f” from “F”',
    ])
    expect(screen.getByRole('button', { name: 'and 2 more' })).toBeInTheDocument()
  })

  it('opens the rest in place', () => {
    render(<ChangeList changes={eight} genres={genres} />)
    fireEvent.click(screen.getByRole('button', { name: 'and 2 more' }))
    expect(screen.getAllByRole('listitem')).toHaveLength(8)
    expect(screen.getByText('“h” from “F”')).toBeInTheDocument()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('has no button for six lines or fewer', () => {
    render(<ChangeList changes={eight.slice(0, 6)} genres={genres} />)
    expect(screen.getAllByRole('listitem')).toHaveLength(6)
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('puts what changed in a recipe under its catalog’s line', () => {
    const change: SnapshotChange = {
      op: 'changed',
      kind: 'catalog',
      aspect: 'recipe',
      name: 'Seed Ghibli',
      was_catalog: { key: 'k', name: 'Seed Ghibli', type: 'movie', provider: 'tmdb', params: { with_companies: '1,2' } },
      catalog: { key: 'k', name: 'Seed Ghibli', type: 'movie', provider: 'tmdb', params: { with_companies: '1' } },
    }
    render(<ChangeList changes={[change]} genres={genres} />)
    expect(screen.getByRole('heading', { name: 'Changed' })).toBeInTheDocument()
    expect(screen.getByRole('listitem')).toHaveTextContent('“Seed Ghibli”Studios: 2 → 1')
  })
})

/** A query result in the one state a block reads. */
function result(state: Partial<UseQueryResult<SnapshotChange[]>>) {
  return { data: undefined, isPending: false, isError: false, isSuccess: false, ...state } as UseQueryResult<SnapshotChange[]>
}

describe('ChangesBlock', () => {
  it('titles the list', () => {
    render(<ChangesBlock title="Since you last published" changes={result({ isSuccess: true, data: eight.slice(0, 1) })} genres={genres} />)
    expect(screen.getByRole('heading', { name: 'Since you last published' })).toBeInTheDocument()
    expect(screen.getByText('“a” from “F”')).toBeInTheDocument()
    expect(screen.queryByText(/change/)).toBeNull()
  })

  it('counts the lines at the end of a shelf’s heading', () => {
    render(<ChangesBlock shelf title="In this update" changes={result({ isSuccess: true, data: eight.slice(0, 3) })} genres={genres} />)
    expect(screen.getByText('3 changes')).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'In this update' })).toHaveClass('bg-raised')
  })

  it('says it is loading, then says nothing once there is nothing to show', () => {
    const { container, rerender } = render(<ChangesBlock title="T" changes={result({ isPending: true })} genres={genres} />)
    expect(screen.getByText('Loading what changed…')).toBeInTheDocument()
    rerender(<ChangesBlock title="T" changes={result({ isSuccess: true, data: [] })} genres={genres} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('says it could not load, without standing in the way', () => {
    render(<ChangesBlock title="T" changes={result({ isError: true })} genres={genres} />)
    expect(screen.getByText('Couldn’t load what changed.')).toBeInTheDocument()
  })
})
