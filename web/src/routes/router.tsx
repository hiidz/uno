import { Navigate, createBrowserRouter } from 'react-router-dom'
import { RequireAuth } from '@/auth'
import { Builder } from './Builder'
import { Login } from './Login'
import { ProfilePicker } from './ProfilePicker'

export const router = createBrowserRouter([
  { path: '/login', element: <Login /> },
  {
    path: '/',
    element: (
      <RequireAuth>
        <Navigate to="/profiles" replace />
      </RequireAuth>
    ),
  },
  {
    path: '/profiles',
    element: (
      <RequireAuth>
        <ProfilePicker />
      </RequireAuth>
    ),
  },
  {
    path: '/configure',
    element: (
      <RequireAuth>
        <Builder />
      </RequireAuth>
    ),
  },
])
