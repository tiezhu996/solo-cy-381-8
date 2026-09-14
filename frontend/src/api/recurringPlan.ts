// 周期账单计划模块 API
import { del, get, post, put } from '@/utils/request'
import type { PageData } from '@/utils/request'
import type { ShareInput } from '@/api/expense'

export interface RecurringPlanShare {
  user_id: number
  username: string
  nickname: string
  ratio: number
  amount: number
}

export interface RecurringPlanLastRun {
  period: string
  expense_id: number
  expense_title: string
  amount: number
  generated_at: string
}

export interface RecurringPlanInfo {
  id: number
  group_id: number
  name: string
  amount: number
  category: string
  payer_id: number
  payer_name: string
  split_type: string
  day_of_month: number
  status: string
  status_text: string
  next_run_date: string
  last_run: RecurringPlanLastRun | null
  created_by: number
  created_at: string
  shares: RecurringPlanShare[]
}

export interface RecurringPlanPayload {
  group_id?: number
  name: string
  amount: number
  category: string
  payer_id: number
  split_type: string
  day_of_month: number
  shares: ShareInput[]
}

export interface RecurringPlanQuery {
  page?: number
  page_size?: number
  status?: string
}

export interface RecurringPlanRunResult {
  generated: number
  periods: string[]
}

export function listPlansApi(groupId: number, params: RecurringPlanQuery) {
  return get<PageData<RecurringPlanInfo>>(`/groups/${groupId}/plans`, params)
}

export function createPlanApi(payload: RecurringPlanPayload) {
  return post<RecurringPlanInfo>('/groups/' + payload.group_id + '/plans', payload)
}

export function getPlanApi(id: number) {
  return get<RecurringPlanInfo>(`/plans/${id}`)
}

export function updatePlanApi(id: number, payload: RecurringPlanPayload) {
  return put<{ message: string }>(`/plans/${id}`, payload)
}

export function pausePlanApi(id: number) {
  return post<{ message: string }>(`/plans/${id}/pause`)
}

export function resumePlanApi(id: number) {
  return post<{ message: string }>(`/plans/${id}/resume`)
}

export function runPlanApi(id: number) {
  return post<RecurringPlanRunResult>(`/plans/${id}/run`)
}

export function removePlanApi(id: number) {
  return del<{ message: string }>(`/plans/${id}`)
}
