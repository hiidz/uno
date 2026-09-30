import { useId, useState } from 'react'
import type { FormEvent } from 'react'
import { KeyRound } from 'lucide-react'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { FieldError } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { KEY_GATE_HEADING, useKeyMutations } from './useTMDBKey'
import type { KeyStep } from './useTMDBKey'

/** Where TMDB's page for making an API key is. */
const TMDB_API_SETTINGS = 'https://www.themoviedb.org/settings/api'

type SaveKey = ReturnType<typeof useKeyMutations>['save']

interface KeyInputProps {
  id: string
  value: string
  error?: string
  autoFocus: boolean
  onChange: (value: string) => void
}

/** A refused key marks the field and points it at the words under it. */
function inputState(id: string, error?: string) {
  if (!error) return { className: 'field type-data w-full' }
  return { invalid: true, describedBy: `${id}-error`, className: 'field type-data w-full border-danger' }
}

function KeyInput({ id, value, error, autoFocus, onChange }: KeyInputProps) {
  const state = inputState(id, error)
  return (
    <input
      id={id}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      autoComplete="off"
      spellCheck={false}
      autoFocus={autoFocus}
      aria-invalid={state.invalid}
      aria-describedby={state.describedBy}
      className={state.className}
    />
  )
}

interface KeyActionsProps {
  pending: boolean
  empty: boolean
  onCancel?: () => void
}

function KeyActions({ pending, empty, onCancel }: KeyActionsProps) {
  return (
    <span className="flex shrink-0 gap-1">
      <button type="submit" className="btn-primary" disabled={pending || empty}>
        {pending ? 'Checking…' : 'Save key'}
      </button>
      {onCancel && (
        <button type="button" className="btn-ghost" onClick={onCancel}>
          Cancel
        </button>
      )}
    </span>
  )
}

function KeyError({ id, error }: { id: string; error?: string }) {
  if (!error) return null
  return (
    <FieldError id={`${id}-error`} role="alert">
      {error}
    </FieldError>
  )
}

interface KeyFormProps {
  save: SaveKey
  onDone?: () => void
}

/**
 * The key field and Save. Saving asks TMDB, so it can take a moment; the
 * server's refusal (the wrong shape, the Read Access Token, a key TMDB
 * refuses, TMDB unreachable) shows under the field in its own words. With
 * `onDone` it is a replacement, with a Cancel, and closes once saved.
 */
function KeyForm({ save, onDone }: KeyFormProps) {
  const [key, setKey] = useState('')
  const id = useId()

  function submit(event: FormEvent) {
    event.preventDefault()
    if (save.isPending) return
    save.mutate(key, { onSuccess: () => onDone?.() })
  }

  return (
    <form onSubmit={submit} noValidate className="grid gap-1.5">
      <label htmlFor={id} className="type-label">
        TMDB API Key
      </label>
      <div className="flex gap-2 max-sm:flex-col">
        <KeyInput
          id={id}
          value={key}
          error={save.error?.message}
          autoFocus={Boolean(onDone)}
          onChange={(value) => {
            setKey(value)
            save.reset()
          }}
        />
        <KeyActions pending={save.isPending} empty={key.trim() === ''} onCancel={onDone} />
      </div>
      <KeyError id={id} error={save.error?.message} />
    </form>
  )
}

/** Removing the key says what it costs first. Not red: adding the key again
 *  undoes it. */
function RemoveKeyDialog({
  open,
  remove,
  onClose,
}: {
  open: boolean
  remove: ReturnType<typeof useKeyMutations>['remove']
  onClose: () => void
}) {
  return (
    <ConfirmDialog
      open={open}
      title="Remove your TMDB key?"
      body="Until you add one again, your home screen can’t load Uno’s rows, and the builder can’t preview or save catalogs."
      confirmLabel="Remove key"
      cancelLabel="Keep it"
      pending={remove.isPending}
      error={remove.error?.message}
      onConfirm={() => remove.mutate(undefined, { onSuccess: onClose })}
      onCancel={() => {
        remove.reset()
        onClose()
      }}
    />
  )
}

/** TMDB's page for making an API key, opened in a new tab. */
function TMDBLink() {
  return (
    <a href={TMDB_API_SETTINGS} target="_blank" rel="noreferrer" className="text-ink underline">
      themoviedb.org
    </a>
  )
}

/**
 * Asked above the profiles while the account has no key, which holds them:
 * without a key, Uno can't find a title for its rows or the builder. Where to
 * get one is said once, here, for someone who has never heard of TMDB.
 */
export function TMDBKeyGate({ step }: { step: KeyStep }) {
  const { save } = useKeyMutations()
  if (step.kind !== 'needed') return null
  return (
    <section
      aria-labelledby={KEY_GATE_HEADING}
      className="bg-raised border-line-hi mb-4 grid gap-3 rounded-2xl border p-5"
    >
      <h3 id={KEY_GATE_HEADING} className="m-0 text-[17px] font-bold">
        Add your TMDB key to start
      </h3>
      <p className="text-dim m-0 max-w-[52ch] text-[15px] leading-[1.5]">
        Uno finds titles through TMDB, a free film and TV database, and this Uno asks each account for its own key.
        Make a free account at <TMDBLink />, open Settings → API, and copy the API Key.
      </p>
      <KeyForm save={save} />
    </section>
  )
}

/**
 * The saved key, under the profiles: set, its last four characters, and
 * Replace and Remove. Replace opens the same form in place.
 */
export function TMDBKeyShelf({ step }: { step: KeyStep }) {
  const { save, remove } = useKeyMutations()
  const [replacing, setReplacing] = useState(false)
  const [confirmingRemove, setConfirmingRemove] = useState(false)
  if (step.kind !== 'set') return null

  return (
    <section aria-label="Your TMDB key" className="bg-raised mt-4 grid gap-3 rounded-2xl px-4 py-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <Icon icon={KeyRound} className="text-dim" />
        <span className="text-[15px] font-semibold">TMDB key</span>
        <span className="stk stk-kind">Set</span>
        <span className="type-data text-dim text-[14px]">ends in {step.last4}</span>
        <span hidden={replacing} className="ml-auto flex gap-1">
          <button type="button" className="btn-secondary btn-sm" onClick={() => setReplacing(true)}>
            Replace
          </button>
          <button type="button" className="btn-ghost btn-sm" onClick={() => setConfirmingRemove(true)}>
            Remove
          </button>
        </span>
      </div>
      {replacing && <KeyForm save={save} onDone={() => setReplacing(false)} />}
      <RemoveKeyDialog open={confirmingRemove} remove={remove} onClose={() => setConfirmingRemove(false)} />
    </section>
  )
}
