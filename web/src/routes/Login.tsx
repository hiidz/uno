import { useState } from 'react'
import type { FormEvent } from 'react'
import type { Location } from 'react-router-dom'
import { useLocation, useNavigate } from 'react-router-dom'
import { NuvioAuthError, useAuth } from '@/auth'

export function Login() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const navigate = useNavigate()
  const location = useLocation()
  const { login } = useAuth()

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await login(email, password)
      const from = (location.state as { from?: Location } | null)?.from
      navigate(from ? `${from.pathname}${from.search}${from.hash}` : '/', { replace: true })
    } catch (err) {
      setError(
        err instanceof NuvioAuthError
          ? err.message
          : "Couldn't reach Nuvio. Check your connection and try again.",
      )
      setSubmitting(false)
    }
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
      </form>
    </div>
  )
}
