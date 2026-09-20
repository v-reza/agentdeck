import type { ButtonHTMLAttributes } from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/cn'

/**
 * Button primitive (DESIGN.md `button-primary` / `button-secondary`).
 *
 * Contract numbers, not preferences: padding 8px 14px, radius 6px (`rounded.sm`),
 * height 32px. DESIGN.md is explicit that the radius is 6 and *not* 12 — a
 * rounder button reads as a different design system.
 *
 * Ref is a normal prop (React 19), so there is no forwardRef wrapper.
 */
const buttonVariants = cva(
  'inline-flex items-center justify-center gap-1.5 whitespace-nowrap font-medium transition-colors disabled:pointer-events-none disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)] focus-visible:ring-offset-1',
  {
    variants: {
      variant: {
        primary: 'bg-[var(--color-accent)] text-[var(--color-on-accent)] hover:bg-[var(--color-accent-hover)]',
        secondary:
          'bg-[var(--color-surface-panel)] text-[var(--color-secondary)] border border-[var(--color-border-subtle)] hover:text-[var(--color-primary)] hover:bg-[var(--color-surface-hover)]',
        ghost:
          'bg-transparent text-[var(--color-secondary)] hover:bg-[var(--color-surface-hover)] hover:text-[var(--color-primary)]',
        danger: 'bg-[var(--color-danger)] text-[var(--color-on-danger)] hover:brightness-110',
      },
      size: {
        sm: 'h-7 px-2.5 rounded-[6px] text-[11px]',
        md: 'h-8 px-3.5 rounded-[6px] text-[12px]',
        lg: 'h-9 px-4 rounded-[6px] text-[13px]',
        icon: 'h-8 w-8 rounded-[6px]',
      },
    },
    defaultVariants: { variant: 'secondary', size: 'md' },
  },
)

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> {}

export function Button({ className, variant, size, type = 'button', ...props }: ButtonProps) {
  return <button type={type} className={cn(buttonVariants({ variant, size }), className)} {...props} />
}

export { buttonVariants }
