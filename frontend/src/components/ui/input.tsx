import type { InputHTMLAttributes, LabelHTMLAttributes, ReactNode, TextareaHTMLAttributes } from 'react'
import { cn } from '@/lib/cn'

/**
 * Input primitives. Height 32px and radius 6px match the shell's control row in
 * the design source, so a form field and a toolbar button sit on the same
 * baseline. The focus ring is the single accent colour — DESIGN.md forbids a
 * second accent.
 */
export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        'h-8 w-full rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] px-2.5 text-[12px] text-[var(--color-primary)]',
        'placeholder:text-[var(--color-tertiary)] focus:border-[var(--color-accent)] focus:outline-none',
        // Driven by the a11y attribute itself, so a field cannot be red without
        // also announcing why, and every caller gets it without a prop.
        'aria-invalid:border-[var(--color-danger)]',
        'disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    />
  )
}

export function Textarea({ className, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      className={cn(
        'w-full rounded-[6px] border border-[var(--color-border-subtle)] bg-[var(--color-surface-panel)] p-2.5 text-[12px] text-[var(--color-primary)]',
        'placeholder:text-[var(--color-tertiary)] focus:border-[var(--color-accent)] focus:outline-none',
        className,
      )}
      {...props}
    />
  )
}

export function Label({ className, ...props }: LabelHTMLAttributes<HTMLLabelElement>) {
  return (
    <label
      className={cn('text-[10px] font-semibold uppercase tracking-[0.06em] text-[var(--color-tertiary)]', className)}
      {...props}
    />
  )
}

/**
 * Field-level error text. Form failures are rendered inline rather than in an
 * alert(): the operator needs the message next to the field that caused it.
 */
export function FieldError({ children }: { children?: string | null }) {
  if (!children) return null
  return <p className="text-[11px] text-[var(--color-danger)]">{children}</p>
}

/**
 * Label + control wrapper. Every form field in the app uses it, so labels keep
 * the DESIGN.md `label` style (11px, 600, tracked) without repeating classes.
 */
export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <label className="flex flex-col gap-1">
      {/* The wrapper <label> is what associates the visible text with the
          control inside it, so `getByLabel('Name')` finds the right input and a
          screen reader announces it. Without the wrapper a bare <span> would
          leave the input unlabelled. */}
      <span className="text-[11px] font-semibold tracking-[0.06em] text-[var(--color-tertiary)] uppercase">
        {label}
      </span>
      {children}
      {hint ? <span className="text-[11px] text-[var(--color-tertiary)]">{hint}</span> : null}
    </label>
  )
}
