import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// jsdom has no ResizeObserver, and Radix measures with one on mount. No test
// depends on a real measurement.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
}

// Testing Library unmounts after each test on its own only when the runner
// exposes `afterEach` as a global, which this config doesn't.
afterEach(() => {
  cleanup()
})
