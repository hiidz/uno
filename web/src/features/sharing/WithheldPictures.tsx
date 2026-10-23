import { useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ImageOff } from 'lucide-react'
import { fetchWithheldMedia, queryKeys, type WithheldMedia, type WithheldPicture } from '@/api'
import { Icon } from '@/components/Icon'
import { communitySetting } from '@/lib/pageConfig'
import { plural } from '@/lib/plural'
import { asksWithheldMedia } from './sharingState'

/** Each field's name in the collection editor. */
const FIELD_WORDS: Record<WithheldPicture['field'], string> = {
  backdrop_image_url: 'Background image',
  cover_image_url: 'Cover image',
  focus_gif_url: 'Focus GIF',
  hero_backdrop_url: 'Hero backdrop',
  title_logo_url: 'Title logo',
}

/** The pictures Community would leave out of an own collection as saved,
 *  fetched while `enabled`. Never kept, like `useChangesSincePublish`: a save
 *  of the collection changes it. */
function useWithheldMedia(profileIndex: number, collectionID: string, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.withheldMedia(profileIndex, collectionID),
    queryFn: () => fetchWithheldMedia(profileIndex, collectionID),
    enabled,
    staleTime: 0,
    gcTime: 0,
  })
}

/** The Publish dialog's pictures card for the row `kind` and `id` name, asked
 *  for when the dialog opens (`asksWithheldMedia`) and shown once the answer
 *  arrives; nothing until then, on any other server, or if it fails. */
export function WithheldPictures({
  profileIndex,
  kind,
  id,
}: {
  profileIndex: number
  kind: 'catalog' | 'collection'
  id: string
}) {
  const asks = asksWithheldMedia(kind, communitySetting())
  const media = useWithheldMedia(profileIndex, id, asks)
  if (!asks || !media.data) return null
  return <WithheldCard media={media.data} />
}

/** A picture's thumbnail: the picture itself. */
function Thumbnail({ picture }: { picture: WithheldPicture }) {
  return (
    <span className="bg-raised flex h-9 w-16 shrink-0 items-center justify-center overflow-hidden rounded-md">
      <img src={picture.url} alt="" loading="lazy" className="h-full w-full object-cover" />
    </span>
  )
}

function PictureRow({ picture }: { picture: WithheldPicture }) {
  const field = FIELD_WORDS[picture.field]
  return (
    <li className="flex items-center gap-3">
      <Thumbnail picture={picture} />
      <span className="flex min-w-0 flex-col">
        <span className="text-ink font-semibold break-words">{picture.title}</span>
        <span className="text-dim break-words">{picture.site ? `${field} · ${picture.site}` : field}</span>
      </span>
    </li>
  )
}

/**
 * "N pictures won't show in Community": a row per picture Community leaves
 * out (its folder, or the collection for its background image, then its
 * field and site), then the line saying why, whose "Which sites?" opens the
 * server's sites in the card. Nothing when no picture is left out.
 */
export function WithheldCard({ media }: { media: WithheldMedia }) {
  const [sitesOpen, setSitesOpen] = useState(false)
  const sitesID = useId()
  const count = media.pictures.length
  if (count === 0) return null
  return (
    <section className="bg-raised-hi flex flex-col gap-3 rounded-xl p-4" aria-label="Pictures Community leaves out">
      <h3 className="text-ink m-0 flex items-center gap-2 font-semibold">
        <Icon icon={ImageOff} className="text-dim shrink-0" />
        {count} {plural(count, 'picture')} won’t show in Community
      </h3>
      <ul className="m-0 flex list-none flex-col gap-3 p-0">
        {media.pictures.map((picture, i) => (
          <PictureRow key={i} picture={picture} />
        ))}
      </ul>
      <p className="m-0">
        This server only shares pictures from sites it trusts.{' '}
        {/* drift-ok: an underlined link inside the card's sentence */}
        <button
          type="button"
          aria-expanded={sitesOpen}
          aria-controls={sitesID}
          onClick={() => setSitesOpen((open) => !open)}
          className="text-ink cursor-pointer font-semibold underline decoration-[var(--uno-community)] decoration-[1.5px] underline-offset-[3px]"
        >
          Which sites?
        </button>
      </p>
      {sitesOpen && (
        <p id={sitesID} className="border-line text-dim m-0 border-t pt-3 break-words">
          {media.sites.join(' · ')}
        </p>
      )}
    </section>
  )
}
