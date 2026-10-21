import { Navigate, createBrowserRouter } from 'react-router-dom'
import { RequireAuth } from '@/auth'
import { ErrorPage } from './ErrorPage'
import { Login } from './Login'
import { ProfilePicker } from './ProfilePicker'

export const router = createBrowserRouter([
  {
    // Pathless, so every page below shares one error page for whatever throws
    // while it renders or loads.
    errorElement: <ErrorPage kind="crashed" />,
    children: [
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
        // The builder carries dnd-kit, Radix and every feature, so it is its own
        // chunk, fetched on the first visit to /configure. A page load that lands
        // here renders nothing until it arrives.
        HydrateFallback: () => null,
        lazy: async () => {
          const { Builder } = await import('./Builder')
          return {
            element: (
              <RequireAuth>
                <Builder />
              </RequireAuth>
            ),
          }
        },
      },
      { path: '*', element: <ErrorPage kind="missing" /> },
    ],
  },
])
