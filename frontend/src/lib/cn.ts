import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * shadcn/ui's class merger. Tailwind classes conflict by specificity, not by
 * order in the file, so a component that spreads `className` last must merge
 * rather than append — otherwise `p-3` from a caller loses to `p-5` in the base
 * string and the override silently does nothing.
 */
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
