<!-- RecurringPlanFormDialog：周期账单计划新建/编辑弹窗（RecurringPlanList 使用） -->
<template>
  <el-dialog v-model="visible" :title="isEdit ? '编辑周期计划' : '新建周期计划'" width="640px" append-to-body destroy-on-close>
    <el-form ref="formRef" :model="form" :rules="rules" label-width="90px">
      <el-form-item label="计划名称" prop="name">
        <el-input v-model="form.name" placeholder="例如：每月房租" maxlength="128" />
      </el-form-item>
      <el-form-item label="金额" prop="amount">
        <el-input-number v-model="form.amount" :min="0.01" :precision="2" :step="100" style="width: 200px" />
      </el-form-item>
      <el-form-item label="类别" prop="category">
        <el-select v-model="form.category" style="width: 200px">
          <el-option v-for="opt in CategoryOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
        </el-select>
      </el-form-item>
      <el-form-item label="付款人" prop="payer_id">
        <el-select v-model="form.payer_id" style="width: 200px">
          <el-option v-for="m in members" :key="m.user_id" :label="m.nickname + ' (' + m.username + ')'" :value="m.user_id" />
        </el-select>
      </el-form-item>
      <el-form-item label="每月执行日" prop="day_of_month">
        <el-select v-model="form.day_of_month" style="width: 200px" placeholder="选择每月执行日">
          <el-option v-for="d in 31" :key="d" :label="'每月 ' + d + ' 日'" :value="d" />
        </el-select>
        <div class="form-tip">月末没有对应日期时，自动落到当月最后一天执行</div>
      </el-form-item>
      <el-form-item label="分摊方式" prop="split_type">
        <el-radio-group v-model="form.split_type">
          <el-radio v-for="opt in SplitTypeOptions" :key="opt.value" :value="opt.value">{{ opt.label }}</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="参与人" prop="shares">
        <div class="share-list">
          <div v-for="(share, index) in form.shares" :key="share.user_id" class="share-row">
            <span class="share-row__name">{{ share.nickname }}</span>
            <el-input-number
              v-if="form.split_type === 'ratio'"
              v-model="share.ratio"
              :min="0"
              :precision="2"
              :step="1"
              size="small"
              style="width: 140px"
            />
            <el-input-number
              v-else-if="form.split_type === 'amount'"
              v-model="share.amount"
              :min="0"
              :precision="2"
              :step="10"
              size="small"
              style="width: 140px"
            />
            <el-button v-if="share.user_id !== form.payer_id" size="small" text type="danger" @click="removeShare(index)">
              移除
            </el-button>
          </div>
          <div class="share-add">
            <el-select v-model="addCandidateId" placeholder="选择要添加的成员" size="small" clearable style="width: 200px">
              <el-option v-for="m in addableMembers" :key="m.user_id" :label="m.nickname + ' (' + m.username + ')'" :value="m.user_id" />
            </el-select>
            <el-button size="small" type="primary" plain :disabled="!addCandidateId" @click="addShare">添加参与人</el-button>
          </div>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="handleSubmit">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import type { FormInstance, FormRules } from 'element-plus'
import { ElMessage } from 'element-plus'
import { CategoryOptions, SplitTypeOptions, SplitType } from '@/constants'
import type { RecurringPlanInfo, RecurringPlanPayload } from '@/api/recurringPlan'
import type { MemberInfo } from '@/api/group'

interface ShareForm {
  user_id: number
  nickname: string
  ratio: number
  amount: number
}

const props = defineProps<{ members: MemberInfo[]; plan?: RecurringPlanInfo | null }>()
const emit = defineEmits<{ (e: 'saved', payload: RecurringPlanPayload, done: (ok: boolean) => void): void }>()

const visible = ref(false)
const submitting = ref(false)
const formRef = ref<FormInstance>()
const isEdit = ref(false)
const addCandidateId = ref<number | undefined>()

// 可补回的成员：群组中尚未列入参与人的成员（编辑时可重新选择并补回被移除的参与人）
const addableMembers = computed(() => props.members.filter((m) => !form.shares.some((s) => s.user_id === m.user_id)))

const form = reactive<{
  name: string
  amount: number
  category: string
  payer_id: number
  split_type: string
  day_of_month: number
  shares: ShareForm[]
}>({
  name: '',
  amount: 1000,
  category: 'other',
  payer_id: 0,
  split_type: SplitType.EQUAL,
  day_of_month: 1,
  shares: [],
})

const rules: FormRules = {
  name: [{ required: true, min: 1, max: 128, message: '请输入计划名称', trigger: 'blur' }],
  amount: [{ required: true, message: '请输入金额', trigger: 'blur' }],
  category: [{ required: true, message: '请选择类别', trigger: 'change' }],
  payer_id: [{ required: true, message: '请选择付款人', trigger: 'change' }],
  day_of_month: [{ required: true, message: '请选择每月执行日', trigger: 'change' }],
  split_type: [{ required: true, message: '请选择分摊方式', trigger: 'change' }],
  shares: [{ required: true, type: 'array', min: 1, message: '至少选择一位参与人', trigger: 'change' }],
}

function buildShares() {
  form.shares = props.members
    .filter((m) => m.user_id !== form.payer_id)
    .map((m) => ({ user_id: m.user_id, nickname: m.nickname || m.username, ratio: 1, amount: 0 }))
  // 付款人默认也参与
  const payer = props.members.find((m) => m.user_id === form.payer_id)
  if (payer && !form.shares.some((s) => s.user_id === payer.user_id)) {
    form.shares.unshift({ user_id: payer.user_id, nickname: payer.nickname || payer.username, ratio: 1, amount: 0 })
  }
}

function open(plan?: RecurringPlanInfo | null) {
  isEdit.value = !!plan
  addCandidateId.value = undefined
  const firstMember = props.members[0]
  form.name = plan?.name || ''
  form.amount = plan?.amount || 1000
  form.category = plan?.category || 'other'
  form.payer_id = plan?.payer_id || firstMember?.user_id || 0
  form.split_type = plan?.split_type || SplitType.EQUAL
  form.day_of_month = plan?.day_of_month || 1
  form.shares = []
  if (plan && plan.shares.length > 0) {
    form.shares = plan.shares.map((s) => ({
      user_id: s.user_id,
      nickname: s.nickname || s.username,
      ratio: s.ratio || 1,
      amount: s.amount || 0,
    }))
  } else {
    buildShares()
  }
  visible.value = true
}

function removeShare(index: number) {
  form.shares.splice(index, 1)
}

// addShare 把选中的群组成员补回参与人列表（编辑时可恢复被移除的参与人）。
function addShare() {
  const member = props.members.find((m) => m.user_id === addCandidateId.value)
  if (!member) return
  form.shares.push({ user_id: member.user_id, nickname: member.nickname || member.username, ratio: 1, amount: 0 })
  addCandidateId.value = undefined
}

async function handleSubmit() {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  if (form.shares.length === 0) {
    ElMessage.warning('请至少选择一位参与人')
    return
  }
  const payload: RecurringPlanPayload = {
    name: form.name,
    amount: form.amount,
    category: form.category,
    payer_id: form.payer_id,
    split_type: form.split_type,
    day_of_month: form.day_of_month,
    shares: form.shares.map((s) => ({
      user_id: s.user_id,
      ratio: form.split_type === SplitType.RATIO ? s.ratio : undefined,
      amount: form.split_type === SplitType.AMOUNT ? s.amount : undefined,
    })),
  }
  submitting.value = true
  // 提交期间保留弹窗与当前输入；由父组件保存完成后回调，仅成功时才关闭
  emit('saved', payload, (ok: boolean) => {
    submitting.value = false
    if (ok) {
      visible.value = false
    }
  })
}

defineExpose({ open })
</script>

<style scoped>
.share-list {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.share-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.share-row__name {
  width: 120px;
}
.share-add {
  display: flex;
  align-items: center;
  gap: 8px;
}
.form-tip {
  width: 100%;
  color: #909399;
  font-size: 12px;
  line-height: 1.5;
}
</style>
