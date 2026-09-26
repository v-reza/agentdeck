import { baseApi } from './base'
import type { Column, Task, TaskStatus, TaskLink, TaskEvent, Project, Board } from '@/lib/domain'
import type { AssignableAgent } from './agents'

/**
 * Board-domain server state (ARCHITECTURE 18.2: `store/api/boards.ts`, tags
 * Board, Task, Run, Event, Approval).
 *
 * Every endpoint here mirrors an existing route in ARCHITECTURE 6.2 verbatim —
 * path, method, body shape, and response shape. Nothing is invented: when the
 * backend cannot answer yet, the endpoint is simply absent and the UI renders
 * the empty state, never a fabricated number.
 *
 * Mutations invalidate by tag rather than by manual refetch, so a move on one
 * screen updates the board list, the drawer, and the cost rail in one pass.
 */

export interface CreateProjectArgs {
  name: string
  slug: string
}

export interface CreateBoardArgs {
  projectID: string
  name: string
  slug: string
  budgetDailyMicros?: number
}

export interface UpdateBoardArgs {
  id: string
  name?: string
  budgetDailyMicros?: number
}

/**
 * US-AD10. `columns` used to live on UpdateBoardArgs, which made the layout
 * writable through the Member-gated `PATCH /boards/{id}`. It is its own arg type
 * now so the only way to send a layout is the Admin-gated route.
 */
export interface UpdateBoardColumnsArgs {
  boardID: string
  columns: Column[]
}

/** The board listing's query. Filters mirror §6.2.16; all are optional. */
export interface ListTasksArgs {
  boardID: string
  /** Repeated as `?status=a&status=b`; the API takes the whole set. */
  statuses?: string[]
  assignee?: string
  search?: string
}

export interface CreateTaskArgs {
  boardID: string
  title: string
  body?: string
  priority?: number
  status?: TaskStatus
  /** US-AD11 AC1. Omitted or empty = unassigned, which is valid (AC5). */
  assignee_agent_id?: string
}

export interface UpdateTaskArgs {
  id: string
  title: string
  body: string
  priority: number
}

export interface MoveTaskArgs {
  id: string
  from: TaskStatus
  to: TaskStatus
}

export const boardsApi = baseApi.injectEndpoints({
  endpoints: (build) => ({
    listProjects: build.query<Project[], void>({
      query: () => 'projects',
      providesTags: (result) =>
        result
          ? [...result.map((p) => ({ type: 'Project' as const, id: p.id })), { type: 'Project' as const, id: 'LIST' }]
          : [{ type: 'Project' as const, id: 'LIST' }],
    }),

    getProject: build.query<Project, string>({
      query: (id) => `projects/${id}`,
      providesTags: (_r, _e, id) => [{ type: 'Project', id }],
    }),

    createProject: build.mutation<Project, CreateProjectArgs>({
      query: (body) => ({ url: 'projects', method: 'POST', body }),
      invalidatesTags: [{ type: 'Project', id: 'LIST' }],
    }),

    updateProject: build.mutation<Project, { id: string; name: string }>({
      query: ({ id, ...body }) => ({ url: `projects/${id}`, method: 'PATCH', body }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'Project', id },
        { type: 'Project', id: 'LIST' },
      ],
    }),

    deleteProject: build.mutation<void, string>({
      query: (id) => ({ url: `projects/${id}`, method: 'DELETE' }),
      invalidatesTags: [{ type: 'Project', id: 'LIST' }],
    }),

    listBoards: build.query<Board[], string>({
      query: (projectID) => `projects/${projectID}/boards`,
      providesTags: (result, _e, projectID) =>
        result
          ? [
              ...result.map((b) => ({ type: 'Board' as const, id: b.id })),
              { type: 'Board' as const, id: `PROJECT-${projectID}` },
            ]
          : [{ type: 'Board' as const, id: `PROJECT-${projectID}` }],
    }),

    getBoard: build.query<Board, string>({
      query: (id) => `boards/${id}`,
      providesTags: (_r, _e, id) => [{ type: 'Board', id }],
    }),

    createBoard: build.mutation<Board, CreateBoardArgs>({
      query: ({ projectID, ...body }) => ({
        url: `projects/${projectID}/boards`,
        method: 'POST',
        body,
      }),
      invalidatesTags: (_r, _e, { projectID }) => [{ type: 'Board', id: `PROJECT-${projectID}` }],
    }),

    updateBoard: build.mutation<Board, UpdateBoardArgs>({
      query: ({ id, ...body }) => ({ url: `boards/${id}`, method: 'PATCH', body }),
      invalidatesTags: (_r, _e, { id }) => [{ type: 'Board', id }],
    }),

    deleteBoard: build.mutation<void, string>({
      query: (id) => ({ url: `boards/${id}`, method: 'DELETE' }),
      invalidatesTags: [{ type: 'Board', id: 'LIST' }],
    }),

    /**
     * US-AD10 — the board layout. A separate endpoint from `updateBoard` because
     * it is a separate route with a different role gate: `PATCH /boards/{id}` is
     * Member (name, budget) while `PATCH /boards/{id}/columns` is Admin. Folding
     * the layout into `updateBoard` would send it through the Member-gated route
     * and quietly defeat US-AD10 AC4.
     *
     * The whole layout is sent, never a diff: the layout is an ordered list, so
     * "the column at index 2" is only meaningful against the list the caller was
     * looking at.
     */
    updateBoardColumns: build.mutation<Column[], UpdateBoardColumnsArgs>({
      query: ({ boardID, columns }) => ({
        url: `boards/${boardID}/columns`,
        method: 'PATCH',
        body: { columns },
      }),
      invalidatesTags: (_r, _e, { boardID }) => [{ type: 'Board', id: boardID }],
    }),

    /**
     * One board's tasks. The three filters travel as query parameters because
     * §6.2.16 advertises them and the server applies them in SQL — filtering the
     * response here would make the app and `curl` disagree about the same board.
     *
     * The cache key is `boardID + filter`, so switching a filter is a new cache
     * entry rather than a refetch that could race the previous one.
     */
    listTasks: build.query<Task[], ListTasksArgs>({
      query: ({ boardID, statuses, assignee, search }) => {
        const params = new URLSearchParams()
        for (const status of statuses ?? []) params.append('status', status)
        if (assignee) params.set('assignee', assignee)
        if (search) params.set('search', search)
        const qs = params.toString()
        return `boards/${boardID}/tasks${qs ? `?${qs}` : ''}`
      },
      providesTags: (result, _e, { boardID }) =>
        result
          ? [
              ...result.map((t) => ({ type: 'Task' as const, id: t.id })),
              { type: 'Task' as const, id: `BOARD-${boardID}` },
            ]
          : [{ type: 'Task' as const, id: `BOARD-${boardID}` }],
    }),

    getTask: build.query<Task, string>({
      query: (id) => `tasks/${id}`,
      providesTags: (_r, _e, id) => [{ type: 'Task', id }],
    }),

    /**
     * The create-task modal's picker (US-AD11 AC1). Same shape and same server
     * query as the `picker` inside `getAgentTasks`, so the two screens cannot
     * disagree about who is assignable.
     */
    listAssignableAgents: build.query<AssignableAgent[], string>({
      query: (boardID) => `boards/${boardID}/assignable-agents`,
      providesTags: (_r, _e, boardID) => [{ type: 'Agent' as const, id: `ASSIGNABLE-${boardID}` }],
    }),

    createTask: build.mutation<Task, CreateTaskArgs>({
      query: ({ boardID, assignee_agent_id, ...body }) => ({
        url: `boards/${boardID}/tasks`,
        method: 'POST',
        // The picker's "no agent" row yields an empty string, which is dropped
        // rather than sent: US-AD11 AC5's case is the *absent* field, and the
        // server stores an unassigned task as NULL, never ''.
        body: assignee_agent_id ? { ...body, assignee_agent_id } : body,
      }),
      invalidatesTags: (_r, _e, { boardID }) => [
        { type: 'Task', id: `BOARD-${boardID}` },
        { type: 'Event', id: `BOARD-${boardID}` },
      ],
    }),

    updateTask: build.mutation<Task, UpdateTaskArgs>({
      query: ({ id, ...body }) => ({ url: `tasks/${id}`, method: 'PATCH', body }),
      invalidatesTags: (_r, _e, { id }) => [{ type: 'Task', id }],
    }),

    deleteTask: build.mutation<void, string>({
      query: (id) => ({ url: `tasks/${id}`, method: 'DELETE' }),
      invalidatesTags: [{ type: 'Task', id: 'LIST' }],
    }),

    /**
     * POST /tasks/{id}/move. The body carries both ends of the transition
     * because the repository guards the write optimistically: a stale `from`
     * is rejected rather than applied, so two operators dragging the same card
     * cannot silently overwrite each other.
     */
    moveTask: build.mutation<Task, MoveTaskArgs>({
      query: ({ id, ...body }) => ({ url: `tasks/${id}/move`, method: 'POST', body }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'Task', id },
        { type: 'Task', id: 'LIST' },
        { type: 'Event', id: 'LIST' },
      ],
    }),

    assignTask: build.mutation<Task, { id: string; agentID: string }>({
      query: ({ id, agentID }) => ({
        url: `tasks/${id}/assign`,
        method: 'POST',
        body: { agent_id: agentID },
      }),
      invalidatesTags: (_r, _e, { id }) => [{ type: 'Task', id }],
    }),

    listTaskLinks: build.query<{ parents: TaskLink[]; children: TaskLink[] }, string>({
      query: (id) => `tasks/${id}/links`,
      providesTags: (_r, _e, id) => [{ type: 'TaskLink', id }],
    }),

    createTaskLink: build.mutation<
      { parent_id: string; child_id: string; ready: boolean },
      { id: string; parentID: string }
    >({
      query: ({ id, parentID }) => ({
        url: `tasks/${id}/links`,
        method: 'POST',
        body: { parent_id: parentID },
      }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'TaskLink', id },
        { type: 'Task', id },
      ],
    }),

    deleteTaskLink: build.mutation<
      { parent_id: string; child_id: string; ready: boolean },
      { id: string; parentID: string }
    >({
      query: ({ id, parentID }) => ({
        url: `tasks/${id}/links/${parentID}`,
        method: 'DELETE',
      }),
      invalidatesTags: (_r, _e, { id }) => [
        { type: 'TaskLink', id },
        { type: 'Task', id },
      ],
    }),

    taskDag: build.query<{ task: Task; parents: TaskLink[] }, string>({
      query: (id) => `tasks/${id}/dag`,
      providesTags: (_r, _e, id) => [{ type: 'TaskLink', id: `DAG-${id}` }],
    }),
  }),
})

export const {
  useListProjectsQuery,
  useGetProjectQuery,
  useCreateProjectMutation,
  useUpdateProjectMutation,
  useDeleteProjectMutation,
  useListBoardsQuery,
  useGetBoardQuery,
  useCreateBoardMutation,
  useUpdateBoardMutation,
  useUpdateBoardColumnsMutation,
  useDeleteBoardMutation,
  useListTasksQuery,
  useListAssignableAgentsQuery,
  useGetTaskQuery,
  useCreateTaskMutation,
  useUpdateTaskMutation,
  useDeleteTaskMutation,
  useMoveTaskMutation,
  useAssignTaskMutation,
  useListTaskLinksQuery,
  useCreateTaskLinkMutation,
  useDeleteTaskLinkMutation,
  useTaskDagQuery,
} = boardsApi

/** Re-exported for callers that only need the event payload shape. */
export type { TaskEvent }
