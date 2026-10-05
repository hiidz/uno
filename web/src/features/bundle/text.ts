/** The largest bundle the import dialog takes, as pasted text. The
 *  twin of `maxBundleBodyBytes` in `internal/api/bundle.go`, the most either
 *  import route accepts; change one and you must change the other. */
export const MAX_BUNDLE_BYTES = 4 << 20 // 4 MiB

/** A bundle as text, pretty-printed. */
export function bundleText(value: unknown): string {
  return JSON.stringify(value, null, 2)
}

/** What a bundle is called in a message. */
export const PASTED_JSON = 'Pasted JSON'

/**
 * Parses bundle text. Returns the parsed value on success and the message of a
 * refusal on failure, told apart by `ok`.
 */
export function parseBundleText(
  text: string,
): { ok: true; bundle: unknown } | { ok: false; message: string } {
  const trimmed = text.trim()
  if (new Blob([trimmed]).size > MAX_BUNDLE_BYTES) {
    return { ok: false, message: `${PASTED_JSON} is over 4 MiB, the most an import can carry.` }
  }
  try {
    return { ok: true, bundle: JSON.parse(trimmed) }
  } catch (err) {
    return { ok: false, message: `${PASTED_JSON} isn't valid: ${(err as Error).message}` }
  }
}

/** Writes through a `ClipboardItem` that holds the text as a promise, so the
 *  user gesture that started a copy survives an `await` before the text is known. */
function writeItem(clipboard: Clipboard, text: Promise<string>): Promise<void> {
  const blob = text.then((value) => new Blob([value], { type: 'text/plain' }))
  return clipboard.write([new ClipboardItem({ 'text/plain': blob })])
}

/**
 * Copies text to the clipboard and says whether it landed. Never throws:
 * clipboard access can be missing (insecure origin) or refused (permission),
 * and the caller falls back to showing the text.
 *
 * A promise of text is written as a `ClipboardItem` where the browser has
 * one, so a copy that waits on a request still counts as the click's own.
 */
export async function copyText(text: string | Promise<string>): Promise<boolean> {
  const clipboard = typeof navigator === 'undefined' ? undefined : navigator.clipboard
  if (!clipboard) return false
  try {
    if (typeof text !== 'string' && typeof ClipboardItem !== 'undefined') {
      await writeItem(clipboard, text)
    } else {
      await clipboard.writeText(await text)
    }
    return true
  } catch {
    return false
  }
}
