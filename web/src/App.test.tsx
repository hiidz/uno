import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import App from './App'

// A smoke test, not a behavior test: it exists to catch "the app doesn't
// even mount" regressions (a provider wired wrong, a route module that
// throws on import, ...). It intentionally starts with no stored Nuvio
// refresh token, so bootstrap resolves synchronously to "unauthenticated"
// and the router lands on /login.
describe('App', () => {
  it('renders the login screen when signed out', async () => {
    render(<App />)

    expect(await screen.findByText(/sign in with your nuvio account/i)).toBeInTheDocument()
  })
})
