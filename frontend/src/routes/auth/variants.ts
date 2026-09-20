/**
 * Per-screen values lifted from the four auth designs.
 *
 * Data, not components — which is why it lives in its own file. The four auth
 * screens share a composition but NOT a set of numbers. Read straight off the
 * design files:
 *
 *   01-login          card 10px, title 17px/600, fields 13px, button 36px/500
 *   02-register       card 14px, title 18px/700, fields 13px, button 36px/600
 *   03-reset-request  card 14px, title 18px/700, fields 12px, button 36px/600
 *   04-reset-confirm  card 14px, title 18px/700, fields 13px, button 38px/600
 *
 * Register also spaces its fields with margins (`form-group { gap: 6px;
 * margin-bottom: 14px }`) where the other three use `space-y-4`, and its brand
 * mark is a 24px sans glyph while login's is a 14px mono one. Averaging those
 * into a single shell is exactly the drift this table prevents, so every
 * divergent value is named per variant instead of being normalised.
 */
export type AuthVariant = 'login' | 'register' | 'resetRequest' | 'resetConfirm'

/**
 * Per-screen values lifted from the four designs.
 *
 * The four auth screens share a composition but NOT a set of numbers. Read
 * straight off the design files:
 *
 *   01-login          card 10px, title 17px/600, fields 13px, button 36px/500
 *   02-register       card 14px, title 18px/700, fields 13px, button 36px/600
 *   03-reset-request  card 14px, title 18px/700, fields 12px, button 36px/600
 *   04-reset-confirm  card 14px, title 18px/700, fields 13px, button 38px/600
 *
 * Register also spaces its fields with margins (`form-group { gap: 6px;
 * margin-bottom: 14px }`) where the other three use `space-y-4`, and its
 * brand mark is a 24px sans glyph while login's is a 14px mono one. Averaging
 * those into a single shell is exactly the drift this table prevents, so every
 * divergent value is named per variant instead of being normalised.
 */
export const V: Record<
  AuthVariant,
  {
    main: string
    header: string
    logoRow: string
    mark: string
    wordmark: string
    chipRow: string
    chip: string
    tagline: string
    chipBullet: boolean
    card: string
    titleBlock: string
    title: string
    subtitle: string
    fieldWrap: string
    fieldRow: string
    label: string
    input: string
    inputBorder: string
    errorWrap: string
    button: string
    buttonWrap: string
    cardFooter: string
    systemFooter: string
    /** A footer row that sits below the system footer, outside the card. */
    outsideFooter?: string
  }
> = {
  login: {
    main: 'w-[400px]',
    header: 'mb-6',
    logoRow: 'mb-2 gap-2',
    mark: 'h-7 w-7 rounded-[6px] font-mono text-[14px] tracking-tighter',
    wordmark: 'text-[18px]',
    chipRow: 'gap-2',
    chip: 'rounded-[6px] bg-[var(--color-surface-sunken)] px-2 py-0.5 font-mono text-[11px]',
    tagline: 'text-[12px] text-[var(--color-secondary)]',
    chipBullet: false,
    card: 'rounded-[10px] border-[var(--color-border-standard)] p-6 shadow-sm',
    titleBlock: 'mb-6',
    title: 'mb-1 text-[17px] font-semibold',
    subtitle: 'text-[13px]',
    fieldWrap: '',
    fieldRow: 'mb-1.5',
    label: 'font-medium',
    input: 'px-3 text-[13px]',
    inputBorder: 'border-[var(--color-border-standard)]',
    errorWrap: '',
    button: 'h-9 text-[13px] font-medium',
    buttonWrap: 'pt-2',
    // 01-login's card ends at the button — the design has no footer row there.
    // The link to /register is still rendered, because without it /register is
    // unreachable from this screen; it sits outside the card, under the footer
    // row, so the card itself stays a class-for-class clone of the design.
    cardFooter: '',
    systemFooter: 'mt-6 gap-3',
    outsideFooter: 'mt-3 text-[12px]',
  },
  register: {
    main: 'w-[400px]',
    header: 'mb-6',
    logoRow: 'mb-[10px] gap-2',
    mark: 'h-6 w-6 rounded-[6px] text-[11px] tracking-[-0.02em]',
    wordmark: 'text-[18px] tracking-[-0.02em]',
    chipRow: 'gap-2 text-[11px]',
    chip: 'rounded-[6px] bg-[#edefed] px-1.5 py-0.5 font-mono text-[11px]',
    tagline: 'text-[11px]',
    chipBullet: false,
    card: 'rounded-[14px] border-[var(--color-border-standard)] p-6 shadow-[0_4px_20px_rgba(0,0,0,0.04)]',
    // card-title carries `margin-bottom: 4px` and card-description 18px, so the
    // block itself adds nothing.
    titleBlock: '',
    title: 'mb-1 text-[18px] font-bold tracking-[-0.02em]',
    subtitle: 'mb-[18px] text-[12px] leading-[1.5]',
    // form-group { gap: 6px; margin-bottom: 14px } — margins, not a flex gap.
    fieldWrap: 'mb-3.5',
    fieldRow: '',
    label: 'font-semibold',
    input: 'px-[10px] text-[13px]',
    inputBorder: 'border-[var(--color-border-standard)]',
    errorWrap: 'mt-3',
    button: 'h-9 text-[13px] font-semibold',
    buttonWrap: '',
    cardFooter: 'mt-4 border-t border-[var(--color-border-subtle)] pt-3.5',
    // `.system-meta-footer { margin-top: 20px }` — the one design that is not 24.
    systemFooter: 'mt-5 gap-3',
  },
  resetRequest: {
    main: 'w-[400px]',
    header: 'mb-6',
    logoRow: 'mb-2.5 gap-2',
    mark: 'h-7 w-7 rounded-[4px] text-[12px] tracking-wider shadow-sm',
    wordmark: 'text-[20px]',
    chipRow: 'gap-2',
    chip: 'rounded-[6px] border border-[rgb(12_26_22_/_0.08)] bg-[var(--color-surface-sunken)] px-2 py-0.5 font-mono text-[12px]',
    tagline: 'text-[12px]',
    chipBullet: false,
    card: 'rounded-[14px] border-[rgb(12_26_22_/_0.10)] p-6 shadow-card',
    titleBlock: 'mb-5',
    title: 'text-[18px] font-bold',
    subtitle: 'mt-1 text-[12px] leading-relaxed',
    fieldWrap: '',
    fieldRow: 'mb-1.5',
    label: 'font-semibold',
    input: 'px-3 text-[12px]',
    inputBorder: 'border-[rgb(12_26_22_/_0.15)]',
    errorWrap: '',
    button: 'h-9 text-[12px] font-semibold shadow-sm',
    buttonWrap: 'pt-1',
    // 03-reset-request keeps its "Ingat password Anda?" row inside the form, so
    // it picks up the form's 16px `space-y-4` on top of its own `pt-1`.
    cardFooter: 'mt-4 pt-1',
    systemFooter: 'mt-6 gap-3',
  },
  resetConfirm: {
    main: 'max-w-[400px]',
    header: 'mb-6',
    logoRow: 'mb-2.5 gap-2.5',
    mark: 'h-7 w-7 rounded-[6px] text-[12px] tracking-wider',
    wordmark: 'text-[18px]',
    chipRow: 'gap-2 font-mono text-[11px]',
    chip: 'rounded-[4px] border border-[var(--color-border-subtle)] bg-[#ecefe9] px-1.5 py-0.5',
    tagline: '',
    chipBullet: true,
    card: 'rounded-[14px] border-[var(--color-border-standard)] p-[24px] shadow-card',
    titleBlock: 'mb-5',
    title: 'mb-1 text-[18px] font-bold',
    subtitle: 'text-[12px] leading-relaxed',
    fieldWrap: '',
    fieldRow: 'mb-1.5',
    label: 'font-semibold',
    input: 'px-3 text-[13px]',
    inputBorder: 'border-[var(--color-border-standard)]',
    errorWrap: '',
    button: 'h-[38px] text-[13px] font-semibold shadow-sm',
    buttonWrap: 'pt-1',
    cardFooter: 'mt-4 border-t border-[var(--color-border-subtle)] pt-3',
    systemFooter: 'mt-6 gap-2',
  },
}

/** The brand row the header of all four designs starts with. */
