import { createContext, useEffect, useSyncExternalStore } from 'react'
import type { ReactNode } from 'react'
import { bootstrap, getAuthState, login, logout, subscribeAuth } from './session'
import type { AuthState } from './session'

export interface AuthContextValue extends AuthState {
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

export const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const state = useSyncExternalStore(subscribeAuth, getAuthState)

  useEffect(() => {
    void bootstrap()
  }, [])

  // Nothing gated renders until the load-time refresh-token exchange (if
  // any) resolves. A visitor with no stored refresh token resolves this
  // synchronously with no network round trip, so /login isn't delayed.
  if (state.status === 'loading') return null

  return <AuthContext.Provider value={{ ...state, login, logout }}>{children}</AuthContext.Provider>
}
