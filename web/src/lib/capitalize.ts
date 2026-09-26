/** `s` with its first character upper-cased, to start a sentence or a line. */
export function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}
