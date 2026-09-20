import { use, useMemo } from 'react'
import type { InputHTMLAttributes, ReactNode } from 'react'
import { loadDictionary, type Dictionary } from '@/lib/i18n'
import { useAppSelector } from '@/store/hooks'
import { cn } from '@/lib/cn'
import { V, type AuthVariant } from './variants'

/**
 * The copy for the current locale, unwrapped with React 19's `use()`
 * (ARCHITECTURE 18.2 names `use()` for exactly this: the i18n dictionary).
 *
 * Auth screens are the one place the locale is not a nicety: the PRD writes the
 * copy itself (US-AD02 AC5 names the button "Log in" and the link "Lupa
 * password?"), so these screens must read from the dictionary rather than
 * hard-coding an English string.
 */
export function useAuthCopy(): Dictionary {
  const lang = useAppSelector((state) => state.lang.lang)
  return use(useMemo(() => loadDictionary(lang), [lang]))
}

function brandMark(v: (typeof V)[AuthVariant]) {
  return (
    <div
      className={cn(
        'flex items-center justify-center bg-[var(--color-accent)] font-bold text-[var(--color-on-accent)]',
        v.mark,
      )}
    >
      AD
    </div>
  )
}

/**
 * The frame shared by 01-login, 02-register, 03-reset-request, and
 * 04-reset-confirm: brand mark and wordmark, a route chip with the tagline, a
 * 400px card, and the system footer.
 *
 * The composition is written once and the numbers come from `V`, so each screen
 * renders its own design rather than a compromise between the four.
 */
export function AuthShell({
  variant,
  route,
  title,
  subtitle,
  children,
  footer,
  outsideFooter,
}: {
  variant: AuthVariant
  route: string
  title: string
  subtitle: string
  children: ReactNode
  footer?: ReactNode
  outsideFooter?: ReactNode
}) {
  const t = useAuthCopy()
  const v = V[variant]

  return (
    // The designs put the page background on <body>. The outer wrapper carries
    // the padding so the 400px card cannot overflow a narrow viewport, which is
    // invisible at the reference width.
    <div className="flex min-h-screen w-full flex-col items-center justify-center bg-[var(--color-surface-page)] p-4 select-none">
      <main className={cn('flex flex-col items-center', v.main)}>
        <header className={cn('flex flex-col items-center text-center', v.header)}>
          <div className={cn('flex items-center', v.logoRow)}>
            {brandMark(v)}
            <span className={cn('font-bold tracking-tight text-[var(--color-primary)]', v.wordmark)}>AgentDeck</span>
          </div>
          <div className={cn('flex items-center', v.chipRow)}>
            <span className={v.chip}>{route}</span>
            {v.chipBullet ? <span>•</span> : null}
            <span className={cn('text-[var(--color-tertiary)]', v.tagline)}>{t['auth.tagline']}</span>
          </div>
        </header>

        <section className={cn('w-full border bg-[var(--color-surface-panel)]', v.card)}>
          <div className={v.titleBlock}>
            <h1 className={cn('tracking-tight text-[var(--color-primary)]', v.title)}>{title}</h1>
            <p className={cn('text-[var(--color-tertiary)]', v.subtitle)}>{subtitle}</p>
          </div>

          {children}

          {footer ? (
            <div className={cn('text-center text-[12px] text-[var(--color-secondary)]', v.cardFooter)}>{footer}</div>
          ) : null}
        </section>

        <footer
          className={cn(
            'flex items-center justify-center font-mono text-[11px] text-[var(--color-tertiary)]',
            v.systemFooter,
          )}
        >
          <span>agentdeck v2.0</span>
          <span>•</span>
          <span>self-hosted</span>
          <span>•</span>
          <span>isolated</span>
        </footer>

        {v.outsideFooter && outsideFooter ? (
          <div className={cn('text-center text-[var(--color-secondary)]', v.outsideFooter)}>{outsideFooter}</div>
        ) : null}
      </main>
    </div>
  )
}

/**
 * A labelled auth field: a label row (optionally carrying a hint on the right)
 * above the control. Spacing between the label and the control, and between
 * consecutive fields, comes from the variant because the register design spaces
 * its fields with margins while the rest use a flex gap.
 *
 * Deliberately not the dashboard's `Field`: that one renders a 10px uppercase
 * tracked label for dense toolbars, while every auth design asks for a 12px
 * sentence-case label.
 */
export function AuthField({
  variant,
  id,
  label,
  hint,
  children,
}: {
  variant: AuthVariant
  id: string
  label: string
  hint?: ReactNode
  children: ReactNode
}) {
  const v = V[variant]
  return (
    <div className={v.fieldWrap}>
      <div className={cn('flex items-center justify-between', v.fieldRow)}>
        <label htmlFor={id} className={cn('text-[12px] text-[var(--color-primary)]', v.label)}>
          {label}
        </label>
        {hint}
      </div>
      {children}
    </div>
  )
}

/**
 * The auth input control.
 *
 * A bare <input> rather than the shared `Input`: the shell's control is 32px
 * tall with 12px text, while every auth design asks for 36px with 12-13px text
 * and its own border weight. Two different controls, so two components — the
 * alternative is a pile of overrides that only look equivalent.
 */
export function AuthInput({
  variant,
  className,
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { variant: AuthVariant }) {
  const v = V[variant]
  return (
    <input
      className={cn(
        'h-9 w-full rounded-[6px] border bg-[var(--color-surface-panel)] text-[var(--color-primary)]',
        'placeholder:text-[var(--color-tertiary)] focus:border-[var(--color-accent)] focus:outline-none',
        'disabled:cursor-not-allowed disabled:opacity-50',
        v.inputBorder,
        v.input,
        className,
      )}
      {...props}
    />
  )
}

/** A `<code>` chip inside a notice body (the designs' mono token highlight). */
function CodeChip({ children }: { children: ReactNode }) {
  return (
    <code className="rounded-[4px] border border-[var(--color-border-subtle)] bg-white px-1 py-0.5 font-mono text-[10px] text-[var(--color-primary)]">
      {children}
    </code>
  )
}

/**
 * Renders `{token}` markers in a notice body as the design's mono code chips.
 *
 * The two reset designs print `sessions` and `users.password_hash` as inline
 * chips mid-sentence, so the sentence stays one translatable string with the
 * token names in place instead of being sliced into three dictionary keys.
 */
function withCodeChips(text: string) {
  return text
    .split(/(\{[\w.]+\})/g)
    .map((part, index) =>
      part.startsWith('{') && part.endsWith('}') ? <CodeChip key={index}>{part.slice(1, -1)}</CodeChip> : part,
    )
}

/**
 * The accent notice box. The three designs that use one draw it three different
 * ways, so each shape is cloned rather than merged:
 *
 *   register  dot and title on one row, body underneath, 10px padding
 *   03-reset  dot at the top-left, title and body in a column beside it, 12px
 *   04-confirm title row, then the body as its own paragraph, 12px
 */
export function AuthNotice({ variant, title, children }: { variant: AuthVariant; title: string; children: ReactNode }) {
  if (variant === 'register') {
    return (
      <div className="mb-[18px] flex flex-col gap-1 rounded-[6px] border border-[rgb(13_122_112_/_0.18)] bg-[var(--color-accent-tint)] p-2.5">
        <div className="flex items-center gap-1.5">
          <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--color-accent)]" />
          <span className="text-[11px] font-semibold tracking-[0.01em] text-[var(--color-accent)]">{title}</span>
        </div>
        <p className="text-[11px] leading-[1.45] text-[var(--color-secondary)]">{children}</p>
      </div>
    )
  }

  if (variant === 'resetRequest') {
    return (
      <div className="rounded-[6px] border border-[rgb(13_122_112_/_0.2)] bg-[rgb(238_247_245_/_0.7)] p-3">
        <div className="flex items-start gap-2">
          <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-[var(--color-accent)]" />
          <div className="text-[11px] leading-relaxed text-[var(--color-secondary)]">
            <span className="mb-0.5 block font-semibold text-[var(--color-primary)]">{title}</span>
            {typeof children === 'string' ? withCodeChips(children) : children}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-[6px] border border-[rgb(194_222_218_/_0.7)] bg-[rgb(234_243_241_/_0.5)] p-3 text-[11px] leading-normal text-[var(--color-secondary)]">
      <div className="mb-1 flex items-center gap-1.5 text-[11px] font-semibold text-[var(--color-accent)]">
        <span className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--color-accent)]" />
        <span>{title}</span>
      </div>
      <p className="mb-1 text-[11px]">{typeof children === 'string' ? withCodeChips(children) : children}</p>
    </div>
  )
}
