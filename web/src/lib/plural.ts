/** Regular English pluralization: `word` unchanged at `n === 1`, `word` + 's'
 *  otherwise. For a word that doesn't inflect this way, spell out both forms
 *  at the call site instead of reaching for this. */
export function plural(n: number, word: string): string {
  return n === 1 ? word : `${word}s`
}

/** `plural`, joined with the count it's counting: "1 folder", "3 folders". */
export function pluralCount(n: number, word: string): string {
  return `${n} ${plural(n, word)}`
}
