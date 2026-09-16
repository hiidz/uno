import { useState } from 'react'
import type { FormEvent } from 'react'
import type { Location } from 'react-router-dom'
import { useLocation, useNavigate } from 'react-router-dom'
import { TriangleAlert } from 'lucide-react'
import { Icon } from '@/components/Icon'
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
    <div className="min-h-svh px-4">
      <main className="mx-auto w-full max-w-[560px] pt-[18vh] pb-16 max-sm:pt-[12vh] max-sm:pb-40">
        <header className="border-line mb-8 grid gap-4 border-b pb-8">
          <h1 className="type-wordmark m-0 text-[40px] leading-none max-sm:text-[28px]">Uno</h1>
          <h2 className="type-display m-0 text-[20px] leading-[28px]">
            Build your Nuvio home screen.
          </h2>
          <p className="text-dim m-0 max-w-[44ch] text-[15px] leading-[22px]">
            Sign in with the account you use on your TV. There&rsquo;s no separate Uno account.
          </p>
        </header>

        <form onSubmit={handleSubmit} noValidate>
          <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-x-8 gap-y-2 py-2 max-sm:grid-cols-[minmax(0,1fr)] max-sm:items-start">
            <label htmlFor="login-email" className="type-eyebrow pt-3.5 text-right max-sm:pt-0 max-sm:text-left">
              Email
            </label>
            <div>
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
                <p id="login-email-error" className="mt-2 grid grid-cols-[16px_minmax(0,1fr)] gap-x-2 text-[13px] leading-[18px]">
                  <Icon icon={TriangleAlert} className="text-danger mt-0.5" />
                  <span>{fieldErrors.email}</span>
                </p>
              )}
            </div>
          </div>

          <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-x-8 gap-y-2 py-2 max-sm:grid-cols-[minmax(0,1fr)] max-sm:items-start">
            <label htmlFor="login-password" className="type-eyebrow pt-3.5 text-right max-sm:pt-0 max-sm:text-left">
              Password
            </label>
            <div>
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
                <p id="login-password-error" className="mt-2 grid grid-cols-[16px_minmax(0,1fr)] gap-x-2 text-[13px] leading-[18px]">
                  <Icon icon={TriangleAlert} className="text-danger mt-0.5" />
                  <span>{fieldErrors.password}</span>
                </p>
              )}
            </div>
          </div>

          <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-x-8 pt-4 max-sm:grid-cols-[minmax(0,1fr)]">
            <div className="col-start-2 grid justify-items-start gap-3 max-sm:col-start-1 max-sm:justify-items-stretch">
              <button
                type="submit"
                disabled={submitting}
                aria-busy={submitting || undefined}
                className="btn-primary h-11 px-8 max-sm:h-12 max-sm:w-full"
              >
                {submitting ? 'Signing in…' : 'Sign in'}
              </button>
              {networkError && (
                <p role="alert" className="grid grid-cols-[16px_minmax(0,1fr)] gap-x-2 text-[14px] leading-[20px]">
                  <Icon icon={TriangleAlert} className="text-danger mt-0.5" />
                  <span>{networkError}</span>
                </p>
              )}
            </div>
          </div>
        </form>

        {devBypassToken && (
          <div className="border-line mt-8 flex flex-col gap-2 border-t pt-4">
            <button type="button" onClick={handleDevBypass} className="btn-quiet self-start px-0">
              Dev bypass login
            </button>
            <p className="type-data text-dimmer m-0 text-[11.5px]">
              Local dev only — signs in as the server&rsquo;s fake account.
            </p>
          </div>
        )}
      </main>
    </div>
  )
}
