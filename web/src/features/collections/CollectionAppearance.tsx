import { useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { InfoTip, Segmented, TextInput } from '@/components/fields'
import { Icon } from '@/components/Icon'
import { VIEW_MODES, VIEW_MODE_LABELS, appearanceSummary, type CollectionFormState } from './collectionForm'

/** A change to the collection's form. */
type CollectionPatch = Partial<CollectionFormState>

const OFF_ON: { value: 'off' | 'on'; label: string }[] = [
  { value: 'off', label: 'Off' },
  { value: 'on', label: 'On' },
]

interface CollectionAppearanceProps {
  state: CollectionFormState
  onChange: (update: CollectionPatch) => void
}

function onOff(on: boolean): 'off' | 'on' {
  return on ? 'on' : 'off'
}

interface ViewModeChoicesProps {
  value: CollectionFormState['viewMode']
  onChange: CollectionAppearanceProps['onChange']
}

/** How folders open, one choice chip per view mode. */
function ViewModeChoices({ value, onChange }: ViewModeChoicesProps) {
  return (
    <div className="choices" role="group" aria-label="How folders open">
      {VIEW_MODES.map((viewMode) => (
        <button key={viewMode} type="button" className="choice" aria-pressed={value === viewMode} onClick={() => onChange({ viewMode })}>
          {VIEW_MODE_LABELS[viewMode]}
        </button>
      ))}
    </div>
  )
}

/**
 * How the collection looks in Nuvio, folded after its folders into one shelf
 * the way a folder's own Appearance is: how folders open, the "All" tab, the
 * background image and the focus glow, summarised on the closed head.
 */
export function CollectionAppearance({ state, onChange }: CollectionAppearanceProps) {
  const [open, setOpen] = useState(false)
  const toggle = () => setOpen(!open)
  const setAllTab = (value: 'off' | 'on') => onChange({ showAllTab: value === 'on' })
  const setBackdrop = (backdropImageURL: string) => onChange({ backdropImageURL })
  const setGlow = (value: 'off' | 'on') => onChange({ focusGlowEnabled: value === 'on' })

  return (
    <div className="mt-6">
      <button type="button" id="col-appearance-head" className="sec-head" aria-expanded={open} aria-controls="col-appearance-body" onClick={toggle}>
        <span className="setting-label type-label">Appearance</span>
        <span className="sec-sum">{appearanceSummary(state)}</span>
        <Icon icon={ChevronDown} size={16} className="ico" />
      </button>
      <div id="col-appearance-body" className="sec-body" hidden={!open}>
        <div className="setting">
          <span className="setting-label type-label">How folders open</span>
          <div className="setting-value">
            <ViewModeChoices value={state.viewMode} onChange={onChange} />
          </div>
        </div>

        <div className="setting">
          <span className="setting-label type-label">“All” tab</span>
          <div className="setting-value ed-line">
            {/* Only Tabbed Grids has tabs to add one to. The value is left
                alone while it's greyed — it's still what this collection is
                set to, and applies again the moment the view mode goes back
                to Tabbed Grids. */}
            <Segmented ariaLabel='"All" tab' value={onOff(state.showAllTab)} onChange={setAllTab} disabled={state.viewMode !== 'TABBED_GRID'} options={OFF_ON} />
          </div>
        </div>

        <div className="setting">
          <label htmlFor="col-backdrop" className="setting-label type-label">
            Background image
          </label>
          <div className="setting-value">
            <TextInput id="col-backdrop" value={state.backdropImageURL} onChange={setBackdrop} placeholder="https://…" />
          </div>
        </div>

        <div className="setting">
          <span className="setting-label type-label">Focus glow</span>
          <div className="setting-value ed-line">
            <Segmented ariaLabel="Focus glow" value={onOff(state.focusGlowEnabled)} onChange={setGlow} options={OFF_ON} />
            <InfoTip label="Focus glow" text="Glow around a folder tile while it's selected." />
          </div>
        </div>
      </div>
    </div>
  )
}

