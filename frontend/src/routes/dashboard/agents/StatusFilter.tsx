import { Filter } from 'lucide-react'
import { Combobox } from '@/components/ui/combobox'

/** Which bucket the registry table is narrowed to. `ready` mirrors
 * `has_provider_key`; `archived` is its own bucket, never mixed into the other
 * two — a retired agent is not "ready to take a task" whatever its credentials
 * say. */
export type StatusFilterValue = 'all' | 'ready' | 'needsKey' | 'archived'

/**
 * The toolbar's status filter.
 *
 * A thin wrapper over `Combobox`, not a second dropdown. It used to be its own
 * button + listbox — which is why the toolbar looked right while the form next
 * to it, built from native `<select>`s, looked like a different product. There
 * is one control now and this is the four-item configuration of it: no search
 * (the list is shorter than the threshold) and the filter glyph in front.
 */
export function StatusFilter({
  value,
  onChange,
  label,
  options,
}: {
  value: StatusFilterValue
  onChange: (next: StatusFilterValue) => void
  label: string
  options: { value: StatusFilterValue; label: string }[]
}) {
  return (
    <Combobox
      value={value}
      onChange={(next) => onChange(next as StatusFilterValue)}
      label={label}
      options={options}
      icon={<Filter size={13} />}
      className="w-auto min-w-[120px]"
    />
  )
}
