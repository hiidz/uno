import { useState } from 'react'
import type { FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { EntryPage } from '@/components/EntryPage'
import { FieldError } from '@/components/fields'
import { NuvioAuthError, loginWithBypassToken, useAuth } from '@/auth'
import { returnPath } from '@/features/builder/redirects'
import { Button } from '@/components/Button'

// Gated on import.meta.env.DEV so the whole bypass branch is statically dead
// in a production build. The Vite dev server sets it from
// DEV_AUTH_BYPASS_TOKEN in the repo-root .env (web/vite.config.ts).
const devBypassToken: string | undefined = import.meta.env.DEV
  ? import.meta.env.VITE_DEV_AUTH_BYPASS_TOKEN
  : undefined

// Nuvio's account page, where an account is created and a password reset.
const NUVIO_ACCOUNT = 'https://nuvio.tv/account'

type FieldErrors = { email?: string; password?: string }

// Something, an @, and a domain with a dot: enough to catch a typo before
// Nuvio answers it as wrong credentials, without second-guessing real addresses.
const EMAIL_SHAPE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

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
    return returnPath(location.state, '/')
  }

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    if (submitting) return

    const errors: FieldErrors = {}
    if (!email) errors.email = 'Enter the email you use for Nuvio.'
    else if (!EMAIL_SHAPE.test(email)) errors.email = 'Enter a full email address, like name@example.com.'
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
        setNetworkError("Couldn't reach Nuvio. Try again in a moment.")
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
      intro={
        <>
          Sign in with your Nuvio account. New to Nuvio, or forgot your password? Go to{' '}
          <a href={NUVIO_ACCOUNT} target="_blank" rel="noreferrer" className="text-ink underline">
            nuvio.tv/account
          </a>
          .
        </>
      }
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
              enterKeyHint="next"
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
              enterKeyHint="go"
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
          <Button
            kind="primary"
            type="submit"
            disabled={submitting}
            aria-busy={submitting || undefined}
            className="max-sm:w-full"
          >
            {submitting ? 'Signing in…' : 'Sign in'}
          </Button>
          {networkError && <FieldError role="alert">{networkError}</FieldError>}
        </div>
      </form>

      {devBypassToken && (
        <div className="border-line mt-8 flex flex-col gap-2 border-t pt-4">
          <Button kind="ghost" onClick={handleDevBypass} className="-ml-3 self-start">
            Dev bypass login
          </Button>
          <p className="type-data text-dimmer text-meta m-0">
            Local dev only — signs in as the server&rsquo;s fake account.
          </p>
        </div>
      )}
    </EntryPage>
  )
}
