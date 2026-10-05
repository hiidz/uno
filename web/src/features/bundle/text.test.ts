import { afterEach, describe, expect, it, vi } from 'vitest'
import { MAX_BUNDLE_BYTES, bundleText, copyText, parseBundleText } from './text'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('bundleText', () => {
  it('pretty-prints with two spaces and round-trips through parseBundleText', () => {
    const bundle = { format: 'uno-bundle', catalogs: [{ name: 'A' }] }
    const text = bundleText(bundle)
    expect(text).toBe('{\n  "format": "uno-bundle",\n  "catalogs": [\n    {\n      "name": "A"\n    }\n  ]\n}')
    expect(parseBundleText(text)).toEqual({ ok: true, bundle })
  })
})

describe('parseBundleText', () => {
  it('accepts surrounding whitespace', () => {
    expect(parseBundleText('\n  {"a": 1}  \n')).toEqual({ ok: true, bundle: { a: 1 } })
  })

  it('refuses text that is not JSON, empty text included', () => {
    for (const text of ['{nope', '   ']) {
      const bad = parseBundleText(text)
      expect(bad.ok).toBe(false)
      if (!bad.ok) expect(bad.message).toMatch(/^Pasted JSON isn't valid: /)
    }
  })

  it('refuses text over the size limit before parsing it', () => {
    const big = `"${'a'.repeat(MAX_BUNDLE_BYTES)}"`
    expect(parseBundleText(big)).toEqual({
      ok: false,
      message: 'Pasted JSON is over 4 MiB, the most an import can carry.',
    })
  })
})

describe('copyText', () => {
  it('writes a string through writeText', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    await expect(copyText('hello')).resolves.toBe(true)
    expect(writeText).toHaveBeenCalledWith('hello')
  })

  it('awaits a promise of text for writeText when there is no ClipboardItem', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    await expect(copyText(Promise.resolve('later'))).resolves.toBe(true)
    expect(writeText).toHaveBeenCalledWith('later')
  })

  it('hands a promise of text to the clipboard as a ClipboardItem', async () => {
    const write = vi.fn().mockResolvedValue(undefined)
    const items: Record<string, Promise<Blob>>[] = []
    vi.stubGlobal('navigator', { clipboard: { write, writeText: vi.fn() } })
    vi.stubGlobal(
      'ClipboardItem',
      class {
        constructor(data: Record<string, Promise<Blob>>) {
          items.push(data)
        }
      },
    )
    await expect(copyText(Promise.resolve('later'))).resolves.toBe(true)
    expect(write).toHaveBeenCalledTimes(1)
    expect(await (await items[0]['text/plain']).text()).toBe('later')
  })

  it('says false when the write is refused, or the text never arrives', async () => {
    vi.stubGlobal('navigator', {
      clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    })
    await expect(copyText('x')).resolves.toBe(false)
    await expect(copyText(Promise.reject(new Error('offline')))).resolves.toBe(false)
  })

  it('says false when the browser has no clipboard', async () => {
    vi.stubGlobal('navigator', {})
    await expect(copyText('x')).resolves.toBe(false)
  })
})
