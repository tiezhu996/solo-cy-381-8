<template>
  <div class="plan-list">
    <el-page-header :content="'周期账单计划 - ' + (groupStore.current?.name || '')" @back="router.push(`/groups/${groupId}`)" />
    <el-card shadow="never" style="margin-top: 16px">
      <div class="filter-bar">
        <el-select v-model="filters.status" placeholder="启停状态" clearable style="width: 140px" @change="handleFilter">
          <el-option v-for="opt in RecurringPlanStatusOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
        </el-select>
        <el-button v-if="isGroupActive" type="primary" @click="openCreate">新建周期计划</el-button>
        <el-tag v-else type="info" effect="plain">群组已归档，无法新建或修改计划</el-tag>
      </div>
      <DataTable :data="planStore.plans" :loading="planStore.loading" :total="planStore.total" show-pagination @page-change="handlePage">
        <el-table-column prop="name" label="计划名称" min-width="130" show-overflow-tooltip />
        <el-table-column label="金额" width="110">
          <template #default="{ row }"><MoneyText :value="row.amount" /></template>
        </el-table-column>
        <el-table-column label="类别" width="90">
          <template #default="{ row }"><StatusBadge :status="row.category" kind="category" /></template>
        </el-table-column>
        <el-table-column prop="payer_name" label="付款人" width="100" />
        <el-table-column label="分摊方式" width="100">
          <template #default="{ row }"><SplitTypeTag :type="row.split_type" /></template>
        </el-table-column>
        <el-table-column label="每月执行日" width="100">
          <template #default="{ row }">每月 {{ row.day_of_month }} 日</template>
        </el-table-column>
        <el-table-column label="下次执行日" width="110">
          <template #default="{ row }">
            <span v-if="row.status === 'active'">{{ row.next_run_date }}</span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="上次生成结果" min-width="180">
          <template #default="{ row }">
            <template v-if="row.last_run">
              <div>{{ row.last_run.expense_title || row.last_run.period + ' 期' }}</div>
              <div class="last-run__time">{{ row.last_run.generated_at }}</div>
            </template>
            <span v-else class="last-run__empty">尚未生成</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }"><StatusBadge :status="row.status" kind="plan" /></template>
        </el-table-column>
        <el-table-column v-if="isGroupActive" label="操作" width="230" fixed="right">
          <template #default="{ row }">
            <el-button size="small" text type="primary" @click="openEdit(row)">编辑</el-button>
            <el-button v-if="row.status === 'active'" size="small" text type="warning" @click="handlePause(row)">暂停</el-button>
            <el-button v-else size="small" text type="success" @click="handleResume(row)">恢复</el-button>
            <el-button v-if="row.status === 'active'" size="small" text type="primary" @click="handleRun(row)">立即执行</el-button>
            <el-button size="small" text type="danger" @click="openRemoveConfirm(row)">移除</el-button>
          </template>
        </el-table-column>
      </DataTable>
      <EmptyState v-if="!planStore.loading && planStore.plans.length === 0" description="还没有周期账单计划，点击「新建周期计划」让固定账单自动入账" />
    </el-card>

    <RecurringPlanFormDialog ref="formDialog" :members="groupStore.members" :plan="editingPlan" @saved="handleSaved" />
    <ConfirmDialog
      ref="confirmRemove"
      title="移除周期计划"
      message="移除后计划不再生成消费，已生成的消费记录保持不变。确定移除该计划吗？"
      @confirm="handleRemove"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { RecurringPlanStatusOptions, GroupStatus } from '@/constants'
import { useGroupStore } from '@/stores/group'
import { useRecurringPlanStore } from '@/stores/recurringPlan'
import type { RecurringPlanInfo, RecurringPlanPayload } from '@/api/recurringPlan'
import DataTable from '@/components/DataTable.vue'
import EmptyState from '@/components/EmptyState.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import SplitTypeTag from '@/components/SplitTypeTag.vue'
import MoneyText from '@/components/MoneyText.vue'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import RecurringPlanFormDialog from '@/components/RecurringPlanFormDialog.vue'

const route = useRoute()
const router = useRouter()
const groupStore = useGroupStore()
const planStore = useRecurringPlanStore()
const groupId = Number(route.params.id)

const filters = reactive<{ status?: string }>({})
const page = ref(1)
const editingPlan = ref<RecurringPlanInfo | null>(null)
const removingPlan = ref<RecurringPlanInfo | null>(null)
const formDialog = ref<InstanceType<typeof RecurringPlanFormDialog>>()
const confirmRemove = ref<InstanceType<typeof ConfirmDialog>>()

const isGroupActive = computed(() => groupStore.current?.status === GroupStatus.ACTIVE)

onMounted(async () => {
  if (!groupStore.current) {
    await groupStore.fetchGroup(groupId)
  }
  await groupStore.fetchMembers(groupId)
  await loadPlans()
})

async function loadPlans() {
  await planStore.fetchPlans(groupId, { ...filters, page: page.value })
}

function handleFilter() {
  page.value = 1
  loadPlans()
}

function handlePage(p: number) {
  page.value = p
  loadPlans()
}

function openCreate() {
  editingPlan.value = null
  formDialog.value?.open()
}

function openEdit(plan: RecurringPlanInfo) {
  editingPlan.value = plan
  formDialog.value?.open(plan)
}

async function handleSaved(payload: RecurringPlanPayload) {
  if (editingPlan.value) {
    await planStore.updatePlan(editingPlan.value.id, payload)
    ElMessage.success('周期计划已更新')
  } else {
    await planStore.createPlan({ ...payload, group_id: groupId })
    ElMessage.success('周期计划已创建')
  }
  await loadPlans()
}

async function handlePause(row: RecurringPlanInfo) {
  await planStore.pausePlan(row.id)
  ElMessage.success('计划已暂停')
  await loadPlans()
}

async function handleResume(row: RecurringPlanInfo) {
  await planStore.resumePlan(row.id)
  ElMessage.success('计划已恢复')
  await loadPlans()
}

async function handleRun(row: RecurringPlanInfo) {
  const result = await planStore.runPlan(row.id)
  if (result.generated > 0) {
    ElMessage.success(`已生成 ${result.generated} 个周期的消费：${result.periods.join('、')}`)
  } else {
    ElMessage.info('当前没有到期的周期，未生成新消费')
  }
  await loadPlans()
}

function openRemoveConfirm(row: RecurringPlanInfo) {
  removingPlan.value = row
  confirmRemove.value?.open()
}

async function handleRemove() {
  if (!removingPlan.value) return
  await planStore.removePlan(removingPlan.value.id)
  ElMessage.success('计划已移除，已生成记录保持不变')
  removingPlan.value = null
  await loadPlans()
}
</script>

<style scoped>
.filter-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
  flex-wrap: wrap;
}
.last-run__time {
  color: #909399;
  font-size: 12px;
}
.last-run__empty {
  color: #c0c4cc;
}
</style>
