// @vitest-environment jsdom
import type { ReactNode } from 'react'
import { renderHook } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { useRedirectNotice } from './redirects'

describe('useRedirectNotice', () => {
  it('says a notice once on a screen that mounts fresh, which StrictMode runs twice', () => {
    const show = vi.fn()
    const notice = 'Community is off on this server.'
    const wrapper = ({ children }: { children: ReactNode }) => (
      <MemoryRouter initialEntries={[{ pathname: '/configure', state: { notice } }]}>{children}</MemoryRouter>
    )
    renderHook(() => useRedirectNotice(show), { wrapper, reactStrictMode: true })
    expect(show).toHaveBeenCalledTimes(1)
    expect(show).toHaveBeenCalledWith({ text: notice, tone: 'notice' })
  })
})
