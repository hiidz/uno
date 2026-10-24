import { useMemo } from 'react'
import type { TMDBParams } from '@/api'
import { FieldError, FieldNote, Segmented, TextInput } from '@/components/fields'
import type { CatalogFormState } from './catalogForm'
import { parseIdList } from './params'
import { sumShuffle } from './summary'
import { TMDBEntityPicker } from './TMDBEntityPicker'

/** The setters these settings are handed. */
type FormPatch = Partial<CatalogFormState>
type PatchForm = (update: FormPatch) => void
type ParamsPatch = Partial<TMDBParams>
type PatchParams = (update: ParamsPatch) => void

interface EntityListsProps {
  kind: 'company' | 'keyword'
  type: CatalogFormState['type']
  withValue: string | undefined
  withoutValue: string | undefined
  withError: string | undefined
  withoutError: string | undefined
  onWith: (value: string | undefined) => void
  onWithout: (value: string | undefined) => void
}

/**
 * A company or keyword section's two lists: the ids a title must match, and
 * the ids that drop a title. Each list's search hides the other's picks, so
 * one id can't be in both.
 */
export function EntityLists({
  kind,
  type,
  withValue,
  withoutValue,
  withError,
  withoutError,
  onWith,
  onWithout,
}: EntityListsProps) {
  const withIds = useMemo(() => parseIdList(withValue).ids, [withValue])
  const withoutIds = useMemo(() => parseIdList(withoutValue).ids, [withoutValue])
  return (
    <>
      <div className="flex w-full flex-col gap-2">
        <label className="setting-label type-label" htmlFor={`cat-${kind}-with`}>
          Include
        </label>
        <TMDBEntityPicker
          kind={kind}
          type={type}
          inputId={`cat-${kind}-with`}
          value={withValue}
          hiddenIds={withoutIds}
          invalid={Boolean(withError)}
          onChange={onWith}
        />
        {withError && <FieldNote tone="danger">{withError}</FieldNote>}
      </div>
      <div className="flex w-full flex-col gap-2">
        <label className="setting-label type-label" htmlFor={`cat-${kind}-without`}>
          Leave out
        </label>
        <TMDBEntityPicker
          kind={kind}
          type={type}
          inputId={`cat-${kind}-without`}
          exclude
          value={withoutValue}
          hiddenIds={withIds}
          invalid={Boolean(withoutError)}
          onChange={onWithout}
        />
        {withoutError && <FieldNote tone="danger">{withoutError}</FieldNote>}
      </div>
    </>
  )
}

const OFF_ON: { value: 'off' | 'on'; label: string }[] = [
  { value: 'off', label: 'Off' },
  { value: 'on', label: 'On' },
]

interface ShuffleControlProps {
  randomized: boolean | undefined
  filmSeries: boolean
  onParams: PatchParams
}

/** Shuffle, the last control of the Order section (or the TMDB collection
 *  section), with what it does beside it once on. */
export function ShuffleControl({
  randomized,
  filmSeries,
  onParams,
}: ShuffleControlProps) {
  return (
    <div className="flex flex-col gap-2">
      <span className="setting-label type-label">Shuffle</span>
      <div className="ed-line">
        <Segmented<'off' | 'on'>
          ariaLabel="Shuffle"
          value={randomized ? 'on' : 'off'}
          onChange={(value) => onParams({ randomized: value === 'on' || undefined })}
          options={OFF_ON}
        />
        {randomized && <span className="ed-note">{sumShuffle(filmSeries)}</span>}
      </div>
    </div>
  )
}


interface NameSettingProps {
  value: string
  error: string | undefined
  onChange: PatchForm
}

export function NameSetting({ value, error, onChange }: NameSettingProps) {
  return (
    <div className="setting">
      <label htmlFor="cat-name" className="setting-label type-label">
        Name
      </label>
      <div className="setting-value">
        <TextInput
          id="cat-name"
          value={value}
          onChange={(name) => onChange({ name })}
          placeholder="Trending Sci-Fi"
          invalid={Boolean(error)}
          width="100%"
        />
        <NameError error={error} />
      </div>
    </div>
  )
}

function NameError({ error }: { error: string | undefined }) {
  if (!error) return null
  return <FieldError>{error}</FieldError>
}
