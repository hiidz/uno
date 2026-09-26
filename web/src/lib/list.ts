// en-GB, whatever the viewer's locale: every other word on screen is English,
// and British lists take no comma before the last item — "a, b and c".
const AND = new Intl.ListFormat('en-GB', { type: 'conjunction' })
const OR = new Intl.ListFormat('en-GB', { type: 'disjunction' })

/** "a", "a and b", "a, b and c". */
export function andList(items: string[]): string {
  return AND.format(items)
}

/** "a", "a or b", "a, b or c". */
export function orList(items: string[]): string {
  return OR.format(items)
}
