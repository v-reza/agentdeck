import { describe, expect, it } from 'vitest'
import {
  budgetOver,
  budgetPercent,
  formatDuration,
  formatMicroUSD,
  formatMicroUSDPadded,
  formatTokens,
  plural,
  shortID,
} from './formatters'

/**
 * DECISIONS 6 is the contract under test: money is an integer count of micro-USD
 * (1 USD = 1_000_000) and must never be a float. These assertions pin the
 * boundary where the integer becomes a string, because that is where a float
 * would sneak in.
 */
/**
 * US-AD32 AC1 pins the cost badge format (`$0.04`, `$1.23`) and US-AD27 AC2 pins
 * micro-USD as an integer with no float. These assertions are where those two
 * contract rules become a rendered string, so they carry the citation.
 */
describe('formatMicroUSD — US-AD32 AC1', () => {
  it('renders dollars and cents without a trailing third decimal', () => {
    expect(formatMicroUSD(5_000_000)).toBe('$5.00')
    expect(formatMicroUSD(4_820_000)).toBe('$4.82')
  })

  it('handles zero', () => {
    expect(formatMicroUSD(0)).toBe('$0.00')
  })

  it('keeps sub-cent spend visible instead of rounding it to a clean zero', () => {
    // A run that cost $0.000318 is not free. Collapsing it to $0.00 — or to
    // $0.000 — would hide real spend, which is the one thing a cost ledger must
    // never do. Asserting `!== '$0.00'` was not enough: `$0.000` passed that
    // check while being exactly the clean zero it was meant to catch. So the
    // assertion is the real property: a non-zero amount must show a digit.
    expect(formatMicroUSD(9_400)).toBe('$0.0094')
    expect(formatMicroUSD(318)).toBe('$0.000318')
    expect(formatMicroUSD(1)).toBe('$0.000001')

    for (const micros of [1, 318, 9_400, 40_000]) {
      expect(formatMicroUSD(micros)).not.toMatch(/^\$0\.0+$/)
    }
  })

  it('matches the two literals the PRD pins for a task cost badge — US-AD32 AC1', () => {
    // US-AD32 AC1: "badge biaya (format: $0.04 atau $1.23)".
    expect(formatMicroUSD(40_000)).toBe('$0.04')
    expect(formatMicroUSD(1_230_000)).toBe('$1.23')
  })

  it('stays at two decimals when two decimals are enough', () => {
    expect(formatMicroUSD(910_000)).toBe('$0.91')
    expect(formatMicroUSD(20_000_000)).toBe('$20.00')
  })
})

describe('formatMicroUSDPadded', () => {
  it('keeps a fixed width so a column of costs stays aligned', () => {
    expect(formatMicroUSDPadded(4_820_000)).toHaveLength(formatMicroUSDPadded(318).length)
  })
})

describe('budgetPercent — US-AD30 AC1', () => {
  it('reports the used fraction of the cap', () => {
    expect(budgetPercent(5_000_000, 20_000_000)).toBe(25)
  })

  it('clamps at 100 because the result is a CSS width', () => {
    expect(budgetPercent(25_000_000, 20_000_000)).toBe(100)
  })

  it('returns 0 for a zero cap instead of dividing by zero', () => {
    expect(budgetPercent(1_000_000, 0)).toBe(0)
  })
})

describe('budgetOver — US-AD30 AC1', () => {
  it('reports an overspend that the clamped percentage cannot express', () => {
    expect(budgetOver(25_000_000, 20_000_000)).toBe(true)
    expect(budgetOver(19_000_000, 20_000_000)).toBe(false)
    expect(budgetOver(1_000_000, 0)).toBe(false)
  })

  it('treats spending exactly at the cap as over, because the AC says ">="', () => {
    // US-AD30 AC1: "Jika >= N16, claim ditolak, task tetap ready." A strict `>`
    // lets a board sit at exactly 100% of its cap while the rail still reads as
    // healthy — the one state an operator most needs to see as reached.
    expect(budgetOver(20_000_000, 20_000_000)).toBe(true)
    expect(budgetPercent(20_000_000, 20_000_000)).toBe(100)
  })
})

describe('formatDuration', () => {
  it('formats sub-minute durations in seconds', () => {
    expect(formatDuration(45_000)).toBe('45s')
  })

  it('formats minutes with the remaining seconds', () => {
    expect(formatDuration(90_000)).toBe('1m 30s')
  })

  it('drops seconds once hours are in play', () => {
    expect(formatDuration(3_725_000)).toBe('1h 2m')
  })
})

describe('formatTokens', () => {
  it('abbreviates thousands the way the design source does', () => {
    expect(formatTokens(1500)).toBe('1.5k')
    expect(formatTokens(128_000)).toBe('128.0k')
  })

  it('abbreviates millions', () => {
    expect(formatTokens(3_400_000)).toBe('3.4M')
  })

  it('leaves small counts alone', () => {
    expect(formatTokens(999)).toBe('999')
  })
})

describe('plural', () => {
  it('does not render "1 boards"', () => {
    expect(plural(1, 'board')).toBe('1 board')
    expect(plural(3, 'board')).toBe('3 boards')
    expect(plural(0, 'board')).toBe('0 boards')
  })

  it('takes an irregular plural when the language needs one', () => {
    expect(plural(2, 'agent', 'agents')).toBe('2 agents')
  })
})

describe('shortID', () => {
  it('keeps the tail, because a ULID prefix is shared by ids from the same ms', () => {
    const first = '01J8ZQ7F5K3M9N2P4R6T8V0XAB'
    const second = '01J8ZQ7F5K3M9N2P4R6T8V0XCD'
    expect(shortID(first)).not.toBe(shortID(second))
    expect(shortID(first)).toContain(first.slice(-8))
  })

  it('leaves an id that is already short untouched', () => {
    expect(shortID('abc123')).toBe('abc123')
  })
})
