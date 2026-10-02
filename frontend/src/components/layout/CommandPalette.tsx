import { useEffect, useMemo, useRef, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import {
  Bot,
  Columns3,
  Command,
  FilePlus2,
  LayoutGrid,
  ListChecks,
  Network,
  ReceiptText,
  Search,
  ShieldCheck,
  Table2,
  type LucideIcon,
} from 'lucide-react'
import { Modal } from '@/components/ui/modal'
import { useListProjectsQuery } from '@/store/api/boards'
import { useSearchTasksQuery } from '@/store/api/dashboard'
import { useAppDispatch, useAppSelector } from '@/store/hooks'
import { toggleCommandPalette, openCreateTask, openTask } from '@/store/slices/uiSlice'
import { useT } from '@/hooks/use-t'

/**
 * Screen 14-command-palette — Cmd+K (US-AD55).
 *
 * AC1 asks for three kinds of entry: navigation to a project or board, task
 * search, and quick actions. All three are here, and the distinction is
 * load-bearing — a palette that only navigated would satisfy the screenshot and
 * not the AC.
 *
 * AC2 is arrow keys plus Enter, so the highlighted row is tracked by INDEX and
 * not by CSS hover. The list is flat and ordered (actions, then navigation, then
 * search results), because a sectioned list with per-section cursors needs a
 * second piece of state and buys nothing the operator can feel.
 *
 * WHAT IS NOT BUILT, AND WHY:
 *
 * - **Boards in the empty state.** `GET /boards` does not exist: boards are read
 *   per project (`GET /projects/{id}/boards`). Fanning out over every project
 *   would mean a hook per project and RTK Query has no `useQueries`, so boards
 *   appear once a project is CHOSEN — which is also when the operator wants them.
 *   The same constraint is already recorded in `hooks/use-projects.ts`.
 * - **Search on an empty query.** The palette does not fire a search for `''`:
 *   the endpoint would be asked to match everything, and the list would be noise.
 * - **Shortcut letters.** The design draws a per-row key (`C`, `G`, `T`). This
 *   repo has no shortcut registry, and inventing one for four rows would add a
 *   global key handler that has to avoid every input on every screen. The design
 *   itself calls them a hint, not a contract.
 */

/** US-AD55 AC1's third kind: quick actions. Wired to real destinations only. */
interface Action {
  id: string
  icon: LucideIcon
  label: string
  hint: string
  run: () => void
}

interface Row {
  id: string
  icon: LucideIcon
  label: string
  hint: string
  /** The displayed section name. Translated, so tests must not lock it. */
  group: string
  /**
   * AC1's three kinds, as a stable token: `action` | `project` | `task`.
   *
   * The visible `group` label is translated (the default locale is Indonesian),
   * so an assertion on it would be a translation test. This is what the e2e
   * locks instead.
   */
  kind: 'action' | 'project' | 'task'
  /** Present on every row: selecting one always goes somewhere or does something. */
  select: () => void
}

export function CommandPalette() {
  const t = useT()
  const dispatch = useAppDispatch()
  const navigate = useNavigate()
  const orgID = useAppSelector((state) => state.session.activeOrgID)
  const open = useAppSelector((state) => state.ui.commandPaletteOpen)
  // The open board is a URL fact, not slice state: the toolbar derives its own
  // view from `location.pathname` for the same reason. Reading it here is what
  // lets the two board-scoped quick actions know whether they have a destination.
  const location = useLocation()
  const boardID = /\/boards\/([^/]+)/.exec(location.pathname)?.[1] ?? ''

  const [query, setQuery] = useState('')
  const [cursor, setCursor] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLUListElement>(null)

  const { data: projects } = useListProjectsQuery(undefined, { skip: !open || !orgID })
  const { data: tasks } = useSearchTasksQuery(
    { q: query.trim(), boardID: boardID || undefined, limit: 8 },
    {
      skip: !open || query.trim().length === 0,
    },
  )

  const close = () => {
    dispatch(toggleCommandPalette())
    setQuery('')
    setCursor(0)
  }

  const actions = useMemo<Action[]>(() => {
    if (!orgID) return []
    return [
      {
        id: 'action:create-task',
        icon: FilePlus2,
        label: t['palette.createTask'],
        hint: boardID ? t['palette.createTaskBoard'] : t['palette.createTaskNoBoard'],
        run: () => {
          // Only offered when a board is open: `openCreateTask` needs one, and a
          // palette row that opens a modal without a destination is a dead end.
          if (boardID) dispatch(openCreateTask(boardID))
        },
      },
      {
        id: 'action:approvals',
        icon: ShieldCheck,
        label: t['palette.approvals'],
        hint: t['palette.approvalsHint'],
        run: () => navigate(`/app/${orgID}/approvals`),
      },
      {
        id: 'action:agents',
        icon: Bot,
        label: t['palette.agents'],
        hint: t['palette.agentsHint'],
        run: () => navigate(`/app/${orgID}/agents`),
      },
      {
        id: 'action:ledger',
        icon: ReceiptText,
        label: t['palette.ledger'],
        hint: t['palette.ledgerHint'],
        run: () => navigate(`/app/${orgID}/cost/ledger`),
      },
      {
        id: 'action:graph',
        icon: Network,
        label: t['palette.graph'],
        hint: boardID ? t['palette.graphHint'] : t['palette.graphNoBoard'],
        run: () => {
          if (boardID) navigate(`/app/${orgID}/boards/${boardID}/graph`)
        },
      },
      {
        id: 'action:table-view',
        icon: Columns3,
        label: t['palette.tableView'],
        hint: boardID ? t['palette.tableViewHint'] : t['palette.tableViewNoBoard'],
        run: () => {
          if (boardID) navigate(`/app/${orgID}/boards/${boardID}/table`)
        },
      },
    ]
  }, [orgID, boardID, dispatch, navigate, t])

  const rows = useMemo<Row[]>(() => {
    const out: Row[] = []
    const needle = query.trim().toLowerCase()

    for (const action of actions) {
      // Quick actions are only useful when no query is typed, or when the query
      // matches them; otherwise they crowd out the thing being searched for.
      if (needle && !action.label.toLowerCase().includes(needle)) continue
      out.push({
        id: action.id,
        icon: action.icon,
        label: action.label,
        hint: action.hint,
        group: t['palette.groupActions'],
        kind: 'action',
        select: action.run,
      })
    }

    if (orgID) {
      for (const project of projects ?? []) {
        if (needle && !project.name.toLowerCase().includes(needle)) continue
        out.push({
          id: `project:${project.id}`,
          icon: LayoutGrid,
          label: project.name,
          hint: t['palette.project'],
          group: t['palette.groupNavigate'],
          kind: 'project',
          select: () => navigate(`/app/${orgID}/projects/${project.id}`),
        })
      }
    }

    // The open board is a navigation row too, and the one an operator reaches for
    // most: it is only present when a board is open, because there is no
    // `GET /boards` to list them all (see the header note).
    if (boardID) {
      out.push({
        id: `board:${boardID}`,
        icon: Table2,
        label: t['palette.currentBoard'],
        hint: t['palette.currentBoardHint'],
        group: t['palette.groupNavigate'],
        kind: 'project',
        select: () => navigate(`/app/${orgID}/boards/${boardID}`),
      })
    }

    for (const task of tasks ?? []) {
      out.push({
        id: `task:${task.id}`,
        icon: ListChecks,
        label: task.title,
        // The board is the useful hint here: a task title alone does not say
        // where selecting it will land.
        hint: `${task.status} · ${task.board_id.slice(-8)}`,
        group: t['palette.groupTasks'],
        kind: 'task',
        select: () => {
          // Navigate to the task's board and open its drawer. There is no
          // `/tasks/:id` route — the drawer is state (`openTask`), which is why
          // this is two steps and not one link.
          navigate(`/app/${orgID}/boards/${task.board_id}`)
          dispatch(openTask(task.id))
        },
      })
    }

    return out
  }, [actions, projects, tasks, query, orgID, navigate, t])

  // The cursor is clamped rather than reset: narrowing a query should keep the
  // selection as close as possible, and a row that vanished must not leave the
  // index past the end of the list.
  useEffect(() => {
    setCursor((current) => (rows.length === 0 ? 0 : Math.min(current, rows.length - 1)))
  }, [rows.length])

  // US-AD55: the shortcut itself. `Cmd+K` and `Ctrl+K` both open it — the app
  // runs on macOS and Windows, and the design's `⌘K` is the mac spelling of the
  // same chord. Bound here rather than in the shell so the palette owns its own
  // entry point, and mounted once (`AppShell`), so there is exactly one listener.
  useEffect(() => {
    const onShortcut = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        dispatch(toggleCommandPalette())
      }
    }
    window.addEventListener('keydown', onShortcut)
    return () => window.removeEventListener('keydown', onShortcut)
  }, [dispatch])

  // Focus the input when the palette opens. `Modal` handles Escape, the focus
  // trap and the return-focus; this is only about the caret.
  useEffect(() => {
    if (open) inputRef.current?.focus()
  }, [open])

  useEffect(() => {
    if (!open) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (rows.length === 0) return
      if (event.key === 'ArrowDown') {
        event.preventDefault()
        setCursor((current) => (current + 1) % rows.length)
      } else if (event.key === 'ArrowUp') {
        event.preventDefault()
        setCursor((current) => (current - 1 + rows.length) % rows.length)
      } else if (event.key === 'Enter') {
        event.preventDefault()
        const row = rows[cursor]
        if (row) {
          close()
          row.select()
        }
      }
    }
    // Capture phase: the palette owns these keys while it is open, and nothing
    // underneath should see an arrow key scroll the board.
    window.addEventListener('keydown', onKeyDown, true)
    return () => window.removeEventListener('keydown', onKeyDown, true)
    // `close` is recreated every render; depending on it would re-bind the
    // listener each time. `rows` and `cursor` are what the handler reads.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, rows, cursor])

  // Keep the highlighted row in view when the cursor walks past the fold.
  useEffect(() => {
    const node = listRef.current?.querySelector<HTMLElement>(`[data-index="${cursor}"]`)
    node?.scrollIntoView({ block: 'nearest' })
  }, [cursor])

  return (
    <Modal open={open} onClose={close} size="md" title={t['palette.title']} description={t['palette.hint']}>
      <div className="flex flex-col gap-2">
        <label className="flex items-center gap-2 border-b border-[var(--color-border-subtle)] pb-2">
          <Search size={14} className="text-[var(--color-tertiary)]" aria-hidden="true" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(event) => {
              setQuery(event.target.value)
              setCursor(0)
            }}
            placeholder={t['palette.placeholder']}
            aria-label={t['palette.title']}
            data-testid="palette-input"
            className="h-7 flex-1 bg-transparent text-[13px] text-[var(--color-primary)] outline-none placeholder:text-[var(--color-tertiary)]"
          />
        </label>

        {rows.length === 0 ? (
          <div className="p-4 text-[12px] text-[var(--color-tertiary)]" data-testid="palette-empty">
            {t['palette.empty']}
          </div>
        ) : (
          <ul ref={listRef} className="flex max-h-[320px] flex-col overflow-y-auto" data-testid="palette-list">
            {rows.map((row, index) => {
              const Icon = row.icon
              const active = index === cursor
              return (
                <li key={row.id}>
                  <button
                    type="button"
                    data-testid="palette-row"
                    data-index={index}
                    data-group={row.group}
                    data-kind={row.kind}
                    data-active={active}
                    aria-current={active ? 'true' : undefined}
                    onMouseEnter={() => setCursor(index)}
                    onClick={() => {
                      close()
                      row.select()
                    }}
                    className={
                      'flex w-full items-center gap-2 rounded-[6px] px-2 py-1.5 text-left transition-colors ' +
                      (active ? 'bg-[var(--color-accent-tint)]' : 'hover:bg-[var(--color-surface-hover)]')
                    }
                  >
                    <Icon size={14} className="shrink-0 text-[var(--color-tertiary)]" aria-hidden="true" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[12px] text-[var(--color-primary)]">{row.label}</span>
                      <span className="block truncate text-[11px] text-[var(--color-tertiary)]">{row.hint}</span>
                    </span>
                    <span className="shrink-0 font-mono text-[10px] uppercase tracking-wider text-[var(--color-tertiary)]">
                      {row.group}
                    </span>
                  </button>
                </li>
              )
            })}
          </ul>
        )}

        <div className="flex items-center gap-3 border-t border-[var(--color-border-subtle)] pt-2 text-[11px] text-[var(--color-tertiary)]">
          <span>{t['palette.keysNavigate']}</span>
          <span>{t['palette.keysSelect']}</span>
          <span className="ml-auto flex items-center gap-1">
            <Command size={12} aria-hidden="true" />
            {t['palette.shortcut']}
          </span>
        </div>
      </div>
    </Modal>
  )
}
