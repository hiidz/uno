import { Navigate, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import { useAuth } from './useAuth'

// AuthProvider withholds rendering entirely while status is 'loading', so by
// the time a route renders, status is settled one way or the other — this
// only ever has to handle the unauthenticated case.
export function RequireAuth({ children }: { children: ReactNode }) {
  const { status } = useAuth()
  const location = useLocation()

  if (status === 'unauthenticated') {
    return <Navigate to="/login" replace state={{ from: location }} />
  }

  return children
}
