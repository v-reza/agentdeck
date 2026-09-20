import { describe, expect, it } from 'vitest'
import { COLUMN_ORDER, TASK_STATUSES, columnForStatus, isTaskStatus } from './domain'

/**
 * DECISIONS 3 is the contract under test: the task lifecycle has exactly ten
 * values, and a column is a *view* of status rather than a status of its own.
 * If a status is added upstream without this file changing, these tests fail —
 * which is the point.
 */
/**
 * The ten statuses are the wire contract the board renders (US-AD12 AC1 drag
 * targets, US-AD15 AC1 filter values). A status the API cannot return must not
 * be renderable, so these assertions pin the union itself.
 */
describe('task status contract — US-AD15 AC1', () => {
  it('has exactly the ten statuses DECISIONS 3 defines', () => {
    expect(TASK_STATUSES).toHaveLength(10)
    expect([...TASK_STATUSES]).toEqual([
      'backlog',
      'ready',
      'running',
      'awaiting_approval',
      'blocked',
      'review',
      'done',
      'failed',
      'cancelled',
      'archived',
    ])
  })

  it('maps every status onto a real board column', () => {
    for (const status of TASK_STATUSES) {
      expect(COLUMN_ORDER).toContain(columnForStatus(status))
    }
  })

  it('groups awaiting_approval with running, not as its own column', () => {
    expect(columnForStatus('awaiting_approval')).toBe('running')
  })

  it('hides archived and blocked in backlog', () => {
    expect(columnForStatus('archived')).toBe('backlog')
    expect(columnForStatus('blocked')).toBe('backlog')
  })
})

describe('isTaskStatus', () => {
  it('accepts a real status', () => {
    expect(isTaskStatus('running')).toBe(true)
  })

  it('rejects a value the API cannot return', () => {
    expect(isTaskStatus('in_progress')).toBe(false)
    expect(isTaskStatus(null)).toBe(false)
  })
})
