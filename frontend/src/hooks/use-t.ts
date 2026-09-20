import { use } from 'react'
import { loadDictionary, type Dictionary } from '@/lib/i18n'
import { useAppSelector } from '@/store/hooks'

/**
 * The copy for the current locale, unwrapped with React 19's `use()`
 * (ARCHITECTURE 18.2 names `use()` for exactly this: the i18n dictionary).
 *
 * The dictionary is keyed `keyof Dictionary` and both locales are complete by
 * type, so `t['members.title']` either exists in both languages or does not
 * compile. That is the point: a hardcoded English string on a dashboard screen
 * is the drift this hook exists to prevent.
 *
 * The auth screens keep their own copy of this pattern because AuthShell resolves
 * the dictionary before the router is mounted; every screen *inside* the shell
 * uses this hook. `loadDictionary` already caches per language, so the promise
 * identity is stable and no `useMemo` is needed.
 */
export function useT(): Dictionary {
  const lang = useAppSelector((state) => state.lang.lang)
  return use(loadDictionary(lang))
}
