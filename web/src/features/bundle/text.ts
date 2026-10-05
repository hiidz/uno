/** The largest bundle the import dialog takes, as a file or as pasted text. The
 *  twin of `maxBundleBodyBytes` in `internal/api/bundle.go`, the most either
 *  import route accepts; change one and you must change the other. */
export const MAX_BUNDLE_BYTES = 4 << 20 // 4 MiB

/** The refusal for a bundle over `MAX_BUNDLE_BYTES`, named for where it came from. */
export function tooLargeMessage(label: string): string {
  return `${label} is over 4 MiB, the most an import can carry.`
}

/** A bundle as text, pretty-printed. The downloaded file and the copied text
 *  are this one string. */
export function bundleText(value: unknown): string {
  return JSON.stringify(value, null, 2)
}

/** What a bundle that came from the paste box is called in a message. */
export const PASTED_JSON = 'Pasted JSON'

/**
 * Parses bundle text. `label` names where it came from ("export.json",
 * `PASTED_JSON`) and leads the message of a refusal. Returns the parsed value
 * on success and the message on failure, told apart by `ok`.
 */
export function parseBundleText(
  text: string,
  label: string,
): { ok: true; bundle: unknown } | { ok: false; message: string } {
  const trimmed = text.trim()
  if (new Blob([trimmed]).size > MAX_BUNDLE_BYTES) {
    return { ok: false, message: tooLargeMessage(label) }
  }
  try {
    return { ok: true, bundle: JSON.parse(trimmed) }
  } catch (err) {
    return { ok: false, message: invalidMessage(label, (err as Error).message) }
  }
}

/** The refusal for text that isn't JSON. Pasted JSON already says JSON. */
function invalidMessage(label: string, detail: string): string {
  const noun = label === PASTED_JSON ? '' : ' JSON'
  return `${label} isn't valid${noun}: ${detail}`
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
