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
    expect(parseBundleText(text, 'x')).toEqual({ ok: true, bundle })
  })
})

describe('parseBundleText', () => {
  it('accepts surrounding whitespace', () => {
    expect(parseBundleText('\n  {"a": 1}  \n', 'x')).toEqual({ ok: true, bundle: { a: 1 } })
  })

  it('names the source in a JSON refusal, empty text included', () => {
    const bad = parseBundleText('{nope', 'Pasted text')
    expect(bad.ok).toBe(false)
    if (!bad.ok) expect(bad.message).toMatch(/^Pasted text isn't valid JSON: /)

    const empty = parseBundleText('   ', 'export.json')
    expect(empty.ok).toBe(false)
    if (!empty.ok) expect(empty.message).toMatch(/^export\.json isn't valid JSON: /)
  })

  it('refuses text over the size limit before parsing it', () => {
    const big = `"${'a'.repeat(MAX_BUNDLE_BYTES)}"`
    expect(parseBundleText(big, 'Pasted text')).toEqual({
      ok: false,
      message: 'Pasted text is over 4 MiB, the most an import can carry.',
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
