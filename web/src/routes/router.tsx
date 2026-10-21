import { lazy, Suspense } from 'react'
import { Navigate, createBrowserRouter } from 'react-router-dom'
import { RequireAuth } from '@/auth'
import { ErrorPage } from './ErrorPage'
import { BuilderShellSkeleton } from './LoadingShells'
import { Login } from './Login'
import { ProfilePicker } from './ProfilePicker'

/** The builder carries dnd-kit, Radix and every feature, so it is its own
 *  chunk, fetched once the sign-in check has passed: a signed-out visit to a
 *  builder address goes to sign-in without it. */
const Builder = lazy(() => import('./Builder').then(({ Builder }) => ({ default: Builder })))

export const router = createBrowserRouter([
  {
    // Pathless, so every page below shares one error page for whatever throws
    // while it renders or loads.
    errorElement: <ErrorPage kind="crashed" />,
    children: [
      { path: '/login', element: <Login /> },
      {
        // The builder on the profile picked last, which goes through the
        // picker first when none is.
        path: '/',
        element: (
          <RequireAuth>
            <Navigate to="/configure" replace />
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
        // The builder, on the profile picked last (`activeProfile.ts`): its
        // screens are children (`features/builder/paths.ts`) that `Builder`
        // reads from the path itself, so moving between them, Workspace and
        // Community included, keeps it mounted with Home's unpushed edits.
        // Its shell stands in while its chunk arrives.
        element: (
          <RequireAuth>
            <Suspense fallback={<BuilderShellSkeleton />}>
              <Builder />
            </Suspense>
          </RequireAuth>
        ),
        children: [
          { path: '/configure' },
          { path: '/configure/catalog/:id' },
          { path: '/configure/collection/:id' },
          { path: '/community' },
          { path: '/community/catalog/:id' },
          { path: '/community/collection/:id' },
        ],
      },
      { path: '*', element: <ErrorPage kind="missing" /> },
    ],
  },
])
