/** `uno-export-YYYY-MM-DD.json`, dated in the viewer's own time zone. */
export function exportFilename(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `uno-export-${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}.json`
}

/**
 * Saves `value` as a pretty-printed JSON file through a temporary object URL.
 * The URL is revoked on the next task rather than straight after `click()`,
 * which some browsers answer by starting the download asynchronously.
 */
export function downloadJSON(value: unknown, filename: string): void {
  const blob = new Blob([JSON.stringify(value, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  link.click()
  window.setTimeout(() => URL.revokeObjectURL(url), 0)
}
