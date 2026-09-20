import { use, useMemo } from 'react'
import { loadDictionary } from '@/lib/i18n'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { setLang } from '@/store/slices/langSlice'
import { cn } from '@/lib/cn'
import type { Lang } from '@/lib/domain'

const LANGS: Lang[] = ['en', 'id']

/**
 * EN/ID switch (ARCHITECTURE 18.2 `components/layout/LangToggle`). The selected
 * language is client state in `langSlice`; the dictionary itself is a promise
 * resource unwrapped with React 19's `use()`, which is the pattern the contract
 * names for i18n ("Unwrapping Promise resource (kamus i18n EN/ID)").
 *
 * The dictionary is loaded so a missing key surfaces at the toggle rather than
 * mid-page, but the label here is fixed: a language switcher that renders its own
 * label in the language you have not chosen yet is unreadable.
 */
export function LangToggle() {
  const lang = useAppSelector((state) => state.lang.lang)
  const dispatch = useAppDispatch()

  // React 19 `use()`: suspend on the dictionary, so the control only renders once
  // translations are actually available.
  use(useMemo(() => loadDictionary(lang), [lang]))

  return (
    <div
      role="group"
      aria-label="Language"
      className="flex items-center rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-page)] p-px"
    >
      {LANGS.map((option) => (
        <button
          key={option}
          type="button"
          aria-pressed={lang === option}
          onClick={() => dispatch(setLang(option))}
          className={cn(
            'rounded-[4px] px-1.5 py-0.5 font-mono text-[10px] font-semibold uppercase transition-colors',
            lang === option
              ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
              : 'text-[var(--color-tertiary)] hover:text-[var(--color-primary)]',
          )}
        >
          {option}
        </button>
      ))}
    </div>
  )
}
