# Shared components

The pieces every screen builds on. The rules for using them are in
[`docs/frontend.md`](../../../docs/frontend.md#ui-rules), "UI rules"; this file is for changing
them.

## What's here

| Component | Use it for |
|---|---|
| `Button` | Every text button. `kind` picks the look, `size="sm"` the smaller height. |
| `IconButton` | A button named by an icon: a circle (`xs`, `sm`, `md`) printed on a sign or plain on a pane, or the icon-plus-word pill. |
| `StateView` | The loading, empty and error states of anything fetched; `size="compact"` in pickers. |
| `fields.tsx` | Form fields, `FieldError`, `FieldNote`, `Segmented`, `InfoTip`. |
| `Modal`, `ConfirmDialog` | A dialog, and the alert that asks before something can't be undone. |
| `MoreMenu` | A row's ⋯ menu; when an entry takes its row away, focus goes to the nearest ⋯ left. |
| `returnFocus.ts` | Where focus goes as something closes: `useReturnFocus` for a dialog with no Radix trigger, `returnFocusWhenFree` for the editor, `keepFocusOnMove` for a row's arrows. |
| `Toasts`, `useToasts`, `useToastLift` | The app's toasts: `show({ tone, text, detail })`, a success, failure or notice. `ToastProvider` sits in `App.tsx`; a bar along the screen's foot calls `useToastLift` so they sit above it. |
| `PaneSign`, `EntryPage` | A pane's sign, and the sign-in and profile-picker page. |
| `Icon`, `Skeleton`, `dnd.tsx`, `Wordmark`, `Fascia` | Smaller parts. |

## How a component is built

- **Looks live in `index.css`.** A component maps its props onto the classes in
  `@layer components` (`btn-primary`, `type-label`, …) and the text-role tokens in
  `@theme static`. A caller's utility class overrides them, because utilities outrank the
  components layer, so there is no class-merging library.
- **A new component follows shadcn's shape**, used as a reference and never installed: a
  thin wrapper around a Radix primitive where one exists (`radix-ui` is already a dependency),
  passing `ref` and every other prop through, with a `data-slot` naming it.
- **Variants are a lookup table** while they don't combine. If they start to, `cva` is the
  option to weigh, and adding it needs the maintainer's yes.
- **A new or changed component gets a test** beside it, rendering under jsdom
  (`Button.test.tsx` shows the pattern).

This folder is where the raw forms live (`<button>`, `text-[…px]`), so the drift check in
`src/drift.test.ts` skips it.
