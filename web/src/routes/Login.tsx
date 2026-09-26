import { useState } from 'react'
import type { FormEvent } from 'react'
import type { Location } from 'react-router-dom'
import { useLocation, useNavigate } from 'react-router-dom'
import { EntryPage } from '@/components/EntryPage'
import { FieldError } from '@/components/fields'
import { NuvioAuthError, loginWithBypassToken, useAuth } from '@/auth'

// Gated on import.meta.env.DEV so the whole bypass branch is statically dead
// in a production build. Must equal the server's DEV_AUTH_BYPASS_TOKEN — see
// web/.env.example.
const devBypassToken: string | undefined = import.meta.env.DEV
  ? import.meta.env.VITE_DEV_AUTH_BYPASS_TOKEN
  : undefined

type FieldErrors = { email?: string; password?: string }

export function Login() {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [networkError, setNetworkError] = useState<string | null>(null)
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
    if (submitting) return

    const errors: FieldErrors = {}
    if (!email) errors.email = 'Enter the email you use for Nuvio.'
    if (!password) errors.password = 'Enter your password to sign in.'
    setFieldErrors(errors)
    if (errors.email || errors.password) return

    setNetworkError(null)
    setSubmitting(true)
    try {
      await login(email, password)
      navigate(destination(), { replace: true })
    } catch (err) {
      if (err instanceof NuvioAuthError) {
        setFieldErrors({ password: err.message })
      } else {
        setNetworkError("Couldn't reach Nuvio. Nothing was sent. Try again in a moment.")
      }
      setSubmitting(false)
    }
  }

  function handleDevBypass() {
    if (!devBypassToken) return
    loginWithBypassToken(devBypassToken)
    navigate(destination(), { replace: true })
  }

  return (
    <EntryPage
      title="Build your Nuvio home screen"
      intro={<>Sign in with the account you use on your TV. There&rsquo;s no separate Uno account.</>}
    >
      <form onSubmit={handleSubmit} noValidate className="bg-raised rounded-2xl px-5 pt-3 pb-5">
        <div className="setting">
          <label htmlFor="login-email" className="setting-label type-label">
            Email
          </label>
          <div className="setting-value">
            <input
              id="login-email"
              type="email"
              autoComplete="username"
              aria-invalid={Boolean(fieldErrors.email) || undefined}
              aria-describedby={fieldErrors.email ? 'login-email-error' : undefined}
              value={email}
              onChange={(event) => {
                setEmail(event.target.value)
                setFieldErrors((prev) => ({ ...prev, email: undefined }))
                setNetworkError(null)
              }}
              className={`field w-full ${fieldErrors.email ? 'border-danger' : ''}`}
            />
            {fieldErrors.email && (
              <FieldError id="login-email-error">{fieldErrors.email}</FieldError>
            )}
          </div>
        </div>

        <div className="setting">
          <label htmlFor="login-password" className="setting-label type-label">
            Password
          </label>
          <div className="setting-value">
            <input
              id="login-password"
              type="password"
              autoComplete="current-password"
              aria-invalid={Boolean(fieldErrors.password) || undefined}
              aria-describedby={fieldErrors.password ? 'login-password-error' : undefined}
              value={password}
              onChange={(event) => {
                setPassword(event.target.value)
                setFieldErrors((prev) => ({ ...prev, password: undefined }))
                setNetworkError(null)
              }}
              className={`field w-full ${fieldErrors.password ? 'border-danger' : ''}`}
            />
            {fieldErrors.password && (
              <FieldError id="login-password-error">{fieldErrors.password}</FieldError>
            )}
          </div>
        </div>

        <div className="grid justify-items-start gap-3 pt-4 max-sm:justify-items-stretch">
          <button
            type="submit"
            disabled={submitting}
            aria-busy={submitting || undefined}
            className="btn-primary max-sm:w-full"
          >
            {submitting ? 'Signing in…' : 'Sign in'}
          </button>
          {networkError && <FieldError role="alert">{networkError}</FieldError>}
        </div>
      </form>

      {devBypassToken && (
        <div className="border-line mt-8 flex flex-col gap-2 border-t pt-4">
          <button type="button" onClick={handleDevBypass} className="btn-ghost -ml-3 self-start">
            Dev bypass login
          </button>
          <p className="type-data text-dimmer m-0 text-[13px]">
            Local dev only — signs in as the server&rsquo;s fake account.
          </p>
        </div>
      )}
    </EntryPage>
  )
}
