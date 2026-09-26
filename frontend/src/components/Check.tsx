// Landing check glyph. It is a stroked 16px check, not a filled one: the design
// (05-landing, plan features + roadmap items) draws it as
//
//   <svg viewBox="0 0 16 16" fill="none" stroke="currentColor"
//        stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
//     <polyline points="3.5 8.5 6.5 11.5 12.5 4.5" />
//
// sized `w-4 h-4` (16px) in the plan list and 13-14px in the roadmap footer via
// `.check-icon` / `.roadmap-card .check-icon`.
//
// This used to be a filled 20px glyph (`fill="currentColor"` plus the 20-unit
// evenodd path) — a different shape, not a different size. The two are only
// distinguishable side by side: a filled check reads as a heavier bullet and its
// corners have no stroke caps, which is exactly the difference the mockup draws.
//
// The same design file does use a filled check inside `portal-badge`, but that is
// a different element on a different screen; folding both into one component
// would make one of them wrong.
const Check = () => (
  <svg
    className="check-icon"
    viewBox="0 0 16 16"
    fill="none"
    stroke="currentColor"
    strokeWidth={2}
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
  >
    <polyline points="3.5 8.5 6.5 11.5 12.5 4.5" />
  </svg>
)

export { Check }
