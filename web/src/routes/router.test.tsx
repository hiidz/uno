// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { RouterProvider, createMemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { router } from './router'

vi.mock('./Login', () => ({
  Login: () => {
    throw new Error('render failed')
  },
}))

function open(path: string) {
  render(<RouterProvider router={createMemoryRouter(router.routes, { initialEntries: [path] })} />)
}

describe('router', () => {
  it('shows an address with no route as a missing page that leads home', async () => {
    open('/no/such/page')
    expect(await screen.findByRole('heading', { name: "This page doesn't exist" })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Go to Uno’s home' })).toHaveAttribute('href', '/')
    expect(screen.queryByRole('button', { name: 'Reload' })).not.toBeInTheDocument()
  })

  it('shows a page that throws as the error page, with Reload and home', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    open('/login')
    expect(await screen.findByRole('heading', { name: 'Something went wrong' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reload' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Go to Uno’s home' })).toHaveAttribute('href', '/')
  })
})
