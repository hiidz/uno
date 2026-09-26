/**
 * Tailwind's `lg` breakpoint, as one value both sides of the app read from.
 *
 * Asserted explicitly as `--breakpoint-lg` in `index.css` rather than left to
 * Tailwind's own default, and mirrored here as a plain string because
 * `stacked.ts` needs the same threshold as a `matchMedia` query, which
 * Tailwind's CSS output doesn't expose to JS. If this value ever changes,
 * change it in both places — `index.css` carries a comment pointing back
 * here for exactly that reason.
 */
const LG_BREAKPOINT = '64rem'

/** The media query Tailwind's `lg:` variant compiles to. */
export const LG_MEDIA_QUERY = `(min-width: ${LG_BREAKPOINT})`
