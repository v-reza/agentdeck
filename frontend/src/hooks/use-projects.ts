import { useMemo } from 'react'
import { useListProjectsQuery, useListBoardsQuery } from '@/store/api/boards'
import { useAppSelector } from '@/store/hooks'
import type { Project } from '@/lib/domain'

/**
 * The sidebar's project tree, from GET /projects.
 *
 * Board counts are not assembled here. The contract exposes boards per project
 * (GET /projects/{id}/boards) and RTK Query has no `useQueries` equivalent, so a
 * single hook cannot fan out over an unknown number of projects without breaking
 * the rules of hooks. Each row calls `useProjectBoardCount` itself, which keeps
 * one stable cache key per project.
 */
export function useProjects(): {
  projects: Project[]
  isLoading: boolean
  error: unknown
} {
  const activeOrgID = useAppSelector((state) => state.session.activeOrgID)
  const { data, isLoading, error } = useListProjectsQuery(undefined, { skip: !activeOrgID })

  const projects = useMemo(() => data ?? [], [data])

  return { projects, isLoading, error }
}

/**
 * One project's board count. Reads the project-scoped board list, so the number
 * is the server's answer and not a client-side tally. A project whose boards
 * have not been read yet counts 0, which is why the sidebar shows a dash until
 * the query resolves rather than a confident zero — see `ProjectTreeRow`.
 */
export function useProjectBoardCount(projectID: string): { count: number; isUnresolved: boolean } {
  const { data, isUninitialized, isLoading } = useListBoardsQuery(projectID)
  return { count: data?.length ?? 0, isUnresolved: isUninitialized || isLoading }
}
