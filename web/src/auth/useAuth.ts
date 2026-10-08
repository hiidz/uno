import { use } from 'react'
import { AuthContext } from './AuthContext'

/** The auth state (`status`, `user`) and its actions, such as `login`, from the
 *  nearest `AuthProvider`. Throws outside one. */
export function useAuth() {
  const ctx = use(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within an AuthProvider')
  return ctx
}
