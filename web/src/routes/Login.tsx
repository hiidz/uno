import { useState } from 'react'
import type { FormEvent } from 'react'
import type { Location } from 'react-router-dom'
import { useLocation, useNavigate } from 'react-router-dom'
import { NuvioAuthError, loginWithBypassToken, useAuth } from '@/auth'

// Gated on import.meta.env.DEV so the whole bypass branch is statically dead
// in a production build. Must equal the server's DEV_AUTH_BYPASS_TOKEN — see
// web/.env.example.
const devBypassToken: string | undefined = import.meta.env.DEV
  ? import.meta.env.VITE_DEV_AUTH_BYPASS_TOKEN
  : undefined

export function Login() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { login } = useAuth()

  // Where RequireAuth bounced from, if it did — shared by both sign-in paths.
  function destination(): string {
    const from = (location.state as { from?: Location } | null)?.from
    return from ? `${from.pathname}${from.search}${from.hash}` : '/'
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await login(email, password)
      navigate(destination(), { replace: true })
    } catch (err) {
      setError(
        err instanceof NuvioAuthError
          ? err.message
          : "Couldn't reach Nuvio. Check your connection and try again.",
      )
      setSubmitting(false)
    }
  }

  function handleDevBypass() {
    if (!devBypassToken) return
    loginWithBypassToken(devBypassToken)
    navigate(destination(), { replace: true })
  }

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <form onSubmit={handleSubmit} className="flex w-full max-w-[320px] flex-col gap-6">
        <div className="flex flex-col gap-2">
          <h1 className="type-wordmark m-0 text-[15px]">Uno</h1>
          <p className="type-data text-dim text-[11.5px]">
            Sign in with your Nuvio account.
          </p>
        </div>

        <div className="flex flex-col gap-2.5">
          <input
            type="email"
            placeholder="Email"
            autoComplete="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            className="field"
          />
          <input
            type="password"
            placeholder="Password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            className="field"
          />
        </div>

        {error && <p className="type-data text-danger m-0 text-[11.5px]">{error}</p>}

        <button type="submit" disabled={submitting} className="btn-primary">
          {submitting ? 'Signing in…' : 'Sign in'}
        </button>

        {devBypassToken && (
          <div className="flex flex-col gap-2 border-line border-t pt-4">
            <button type="button" onClick={handleDevBypass} className="btn-ghost">
              Dev bypass login
            </button>
            <p className="type-data text-dim m-0 text-[11.5px]">
              Local dev only — signs in as the server's fake account.
            </p>
          </div>
        )}
      </form>
    </div>
  )
}
