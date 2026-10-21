// @vitest-environment jsdom
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { TOAST_MS, useToasts, type ToastInput } from './toastQueue'
import { ToastProvider } from './Toasts'

let show: (toast: ToastInput) => void

function Grab() {
  show = useToasts().show
  return null
}

function renderToasts() {
  render(
    <ToastProvider>
      <Grab />
    </ToastProvider>,
  )
}

afterEach(() => vi.useRealTimers())

describe('ToastProvider', () => {
  it('shows a failure’s headline with its reason beneath, and keeps it past the success time', () => {
    vi.useFakeTimers()
    renderToasts()
    act(() => show({ tone: 'danger', text: 'Couldn’t add it', detail: 'TMDB isn’t answering right now.' }))
    expect(screen.getByText('Couldn’t add it')).toBeInTheDocument()
    expect(screen.getByText('TMDB isn’t answering right now.')).toBeInTheDocument()
    act(() => vi.advanceTimersByTime(TOAST_MS * 3))
    expect(screen.getByText('Couldn’t add it')).toBeInTheDocument()
  })

  it('takes a success away after its time', () => {
    vi.useFakeTimers()
    renderToasts()
    act(() => show({ tone: 'success', text: 'Added “Noir” to your catalogs' }))
    expect(screen.getByText('Added “Noir” to your catalogs')).toBeInTheDocument()
    act(() => vi.advanceTimersByTime(TOAST_MS + 500))
    expect(screen.queryByText('Added “Noir” to your catalogs')).not.toBeInTheDocument()
  })

  it('closes a toast from its ×', () => {
    vi.useFakeTimers()
    renderToasts()
    act(() => show({ tone: 'notice', text: 'That catalog isn’t in this profile.' }))
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    act(() => vi.advanceTimersByTime(500))
    expect(screen.queryByText('That catalog isn’t in this profile.')).not.toBeInTheDocument()
  })

  it('shows at most three', () => {
    renderToasts()
    act(() => {
      for (const n of [1, 2, 3, 4]) show({ tone: 'success', text: `Done ${n}` })
    })
    expect(screen.queryByText('Done 1')).not.toBeInTheDocument()
    expect(screen.getByText('Done 4')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Dismiss' })).toHaveLength(3)
  })

  it('refuses to work without its provider', () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    expect(() => render(<Grab />)).toThrow(/ToastProvider/)
  })
})
