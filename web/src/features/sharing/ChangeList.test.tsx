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

const seven = ['a', 'b', 'c', 'd', 'e', 'f', 'g'].map(removed)

describe('ChangeList', () => {
  it('shows five lines in the server’s order, then "and N more"', () => {
    render(<ChangeList changes={seven} genres={genres} />)
    expect(screen.getAllByRole('listitem').map((item) => item.textContent)).toEqual([
      'Removed “a” from “F”',
      'Removed “b” from “F”',
      'Removed “c” from “F”',
      'Removed “d” from “F”',
      'Removed “e” from “F”',
    ])
    expect(screen.getByRole('button', { name: 'and 2 more' })).toBeInTheDocument()
  })

  it('opens the rest in place', () => {
    render(<ChangeList changes={seven} genres={genres} />)
    fireEvent.click(screen.getByRole('button', { name: 'and 2 more' }))
    expect(screen.getAllByRole('listitem')).toHaveLength(7)
    expect(screen.getByText('Removed “g” from “F”')).toBeInTheDocument()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('has no button for five lines or fewer', () => {
    render(<ChangeList changes={seven.slice(0, 5)} genres={genres} />)
    expect(screen.getAllByRole('listitem')).toHaveLength(5)
    expect(screen.queryByRole('button')).toBeNull()
  })
})

/** A query result in the one state a block reads. */
function result(state: Partial<UseQueryResult<SnapshotChange[]>>) {
  return { data: undefined, isPending: false, isError: false, isSuccess: false, ...state } as UseQueryResult<SnapshotChange[]>
}

describe('ChangesBlock', () => {
  it('titles the list', () => {
    render(<ChangesBlock title="Since you last published" changes={result({ isSuccess: true, data: seven.slice(0, 1) })} genres={genres} />)
    expect(screen.getByRole('heading', { name: 'Since you last published' })).toBeInTheDocument()
    expect(screen.getByText('Removed “a” from “F”')).toBeInTheDocument()
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
