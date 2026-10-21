// @vitest-environment jsdom
import { render, screen, waitFor } from '@testing-library/react'
import { RouterProvider, createMemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { router } from './router'

const builder = vi.hoisted(() => ({ loaded: false }))

// Signed out, as `RequireAuth` sees it.
vi.mock('@/auth/useAuth', () => ({ useAuth: () => ({ status: 'unauthenticated' }) }))
vi.mock('./Builder', () => {
  builder.loaded = true
  return { Builder: () => null }
})
vi.mock('./Login', () => ({
  Login: () => {
    throw new Error('render failed')
  },
}))

function open(path: string) {
  const memory = createMemoryRouter(router.routes, { initialEntries: [path] })
  render(<RouterProvider router={memory} />)
  return memory
}

describe('router', () => {
  it('shows an address with no route as a missing page that leads home', async () => {
    open('/no/such/page')
    expect(await screen.findByRole('heading', { name: "This page doesn't exist" })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Go to Uno’s home' })).toHaveAttribute('href', '/')
    expect(screen.queryByRole('button', { name: 'Reload' })).not.toBeInTheDocument()
  })

  it('shows an old /p/ address as the missing page', async () => {
    open('/p/1/community')
    expect(await screen.findByRole('heading', { name: "This page doesn't exist" })).toBeInTheDocument()
  })

  it('shows a page that throws as the error page, with Reload and home', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    open('/login')
    expect(await screen.findByRole('heading', { name: 'Something went wrong' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reload' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Go to Uno’s home' })).toHaveAttribute('href', '/')
  })

  it('sends a signed-out visit to a builder address to sign-in without loading the builder', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const memory = open('/configure')
    await waitFor(() => expect(memory.state.location.pathname).toBe('/login'))
    expect(builder.loaded).toBe(false)
  })
})
