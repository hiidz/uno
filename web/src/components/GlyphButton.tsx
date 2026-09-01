/**
 * A small button naming itself by a single glyph (⧉ duplicate, 🗑 delete),
 * in the two shapes this app draws one at: icon-only, or icon plus the
 * first word of its label.
 */
export function GlyphButton({
  label,
  glyph,
  onClick,
  destructive,
  variant,
}: {
  label: string
  glyph: string
  onClick: () => void
  destructive?: boolean
  /** `'icon'`: a glyph-only square — the editor header's own duplicate and
   *  delete, below `lg`, where the row is one compact line. `'labeled'`: the
   *  glyph plus the label's first word — the library rail's own row actions,
   *  above `lg`, where a selected row has room to spend. */
  variant: 'icon' | 'labeled'
}) {
  return (
    <button
      type="button"
      onClick={(event) => {
        // Stops the click from also reaching whatever this button sits
        // inside. A no-op where nothing's listening (the editor header);
        // load-bearing where it is (a library row that selects itself).
        event.stopPropagation()
        onClick()
      }}
      aria-label={label}
      title={label}
      className={
        variant === 'icon'
          ? `tap border-line-hi text-dim grid h-7 w-7 shrink-0 place-items-center rounded-[2px] border text-[12px] leading-none transition-colors ${
              destructive
                ? 'hover:border-danger hover:text-danger'
                : 'hover:border-dim hover:text-ink'
            }`
          : `type-data flex h-6 items-center gap-1 rounded-[2px] border px-2 text-[10px] tracking-[0.06em] uppercase transition-colors pointer-coarse:h-9 ${
              destructive
                ? 'border-line-hi text-dim hover:border-danger hover:text-danger'
                : 'border-line-hi text-dim hover:border-dim hover:text-ink'
            }`
      }
    >
      {variant === 'icon' ? (
        <span aria-hidden="true">{glyph}</span>
      ) : (
        <>
          <span aria-hidden="true" className="text-[11px] leading-none">
            {glyph}
          </span>
          {label.split(' ')[0]}
        </>
      )}
    </button>
  )
}
