// 周期账单计划状态管理
import { defineStore } from 'pinia'
import {
  createPlanApi,
  listPlansApi,
  pausePlanApi,
  removePlanApi,
  resumePlanApi,
  runPlanApi,
  updatePlanApi,
  type RecurringPlanInfo,
  type RecurringPlanPayload,
  type RecurringPlanQuery,
} from '@/api/recurringPlan'

export const useRecurringPlanStore = defineStore('recurringPlan', {
  state: () => ({
    plans: [] as RecurringPlanInfo[],
    total: 0,
    loading: false,
  }),
  actions: {
    async fetchPlans(groupId: number, query: RecurringPlanQuery) {
      this.loading = true
      try {
        const data = await listPlansApi(groupId, { page_size: 50, ...query })
        this.plans = data.list
        this.total = data.total
      } finally {
        this.loading = false
      }
    },
    async createPlan(payload: RecurringPlanPayload) {
      return await createPlanApi(payload)
    },
    async updatePlan(id: number, payload: RecurringPlanPayload) {
      await updatePlanApi(id, payload)
    },
    async pausePlan(id: number) {
      await pausePlanApi(id)
    },
    async resumePlan(id: number) {
      await resumePlanApi(id)
    },
    async removePlan(id: number) {
      await removePlanApi(id)
    },
    async runPlan(id: number) {
      return await runPlanApi(id)
    },
  },
})
