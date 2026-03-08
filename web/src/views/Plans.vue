<template>
  <div class="plans">
    <n-card>
      <template #header>
        <n-space justify="space-between" align="center">
          <span>套餐管理</span>
          <n-space>
            <n-input
              v-model:value="searchText"
              placeholder="搜索套餐名称、描述..."
              clearable
              style="width: 250px;"
            >
              <template #prefix>
                <span>🔍</span>
              </template>
            </n-input>
            <n-button type="primary" @click="openCreateModal">
              添加套餐
            </n-button>
          </n-space>
        </n-space>
      </template>

      <!-- 骨架屏加载 -->
      <TableSkeleton v-if="loading && plans.length === 0" :rows="3" :columns="[1, 2, 1, 1, 1]" />

      <!-- 空状态 -->
      <EmptyState
        v-else-if="!loading && plans.length === 0"
        type="plans"
        action-text="添加套餐"
        @action="openCreateModal"
      />

      <!-- 搜索无结果 -->
      <EmptyState
        v-else-if="searchText && filteredPlans.length === 0"
        type="search"
        :description="`未找到包含 '${searchText}' 的套餐`"
      />

      <!-- 数据表格 -->
      <n-data-table
        v-else
        :columns="columns"
        :data="filteredPlans"
        :loading="loading"
        :row-key="(row: any) => row.id"
      />
    </n-card>

    <!-- Create/Edit Modal -->
    <n-modal v-model:show="showCreateModal" preset="dialog" :title="editingPlan ? '编辑套餐' : '添加套餐'" style="width: 600px;">
      <n-form :model="form" label-placement="left" label-width="100">
        <n-form-item label="套餐名称">
          <n-input v-model:value="form.name" placeholder="如: 基础版、专业版" />
        </n-form-item>
        <n-form-item label="套餐描述">
          <n-input v-model:value="form.description" type="textarea" placeholder="套餐功能说明" :rows="2" />
        </n-form-item>
        <n-divider title-placement="left">配额限制</n-divider>
        <n-form-item label="流量配额">
          <n-space>
            <n-input-number
              v-model:value="trafficQuotaGB"
              :min="0"
              :max="102400"
              :step="10"
              style="width: 150px;"
              placeholder="0"
            />
            <span>GB (0 = 无限制)</span>
          </n-space>
        </n-form-item>
        <n-form-item label="速度限制">
          <n-space>
            <n-input-number
              v-model:value="speedLimitMbps"
              :min="0"
              :max="10000"
              :step="10"
              style="width: 150px;"
              placeholder="0"
            />
            <span>Mbps (0 = 不限速)</span>
          </n-space>
        </n-form-item>
        <n-form-item label="有效期">
          <n-space>
            <n-input-number
              v-model:value="form.duration"
              :min="0"
              :max="3650"
              :step="30"
              style="width: 150px;"
              placeholder="30"
            />
            <span>天 (0 = 永久)</span>
          </n-space>
        </n-form-item>
        <n-divider title-placement="left">资源限制</n-divider>
        <n-form-item label="最大节点数">
          <n-space>
            <n-input-number
              v-model:value="form.max_nodes"
              :min="0"
              :max="1000"
              style="width: 150px;"
              placeholder="0"
            />
            <span>(0 = 无限制)</span>
          </n-space>
        </n-form-item>
        <n-form-item label="最大客户端">
          <n-space>
            <n-input-number
              v-model:value="form.max_clients"
              :min="0"
              :max="1000"
              style="width: 150px;"
              placeholder="0"
            />
            <span>(0 = 无限制)</span>
          </n-space>
        </n-form-item>
        <n-form-item label="最大隧道数">
          <n-space>
            <n-input-number
              v-model:value="form.max_tunnels"
              :min="0"
              :max="1000"
              style="width: 150px;"
              placeholder="0"
            />
            <span>(0 = 无限制)</span>
          </n-space>
        </n-form-item>
        <n-form-item label="最大端口转发">
          <n-space>
            <n-input-number
              v-model:value="form.max_port_forwards"
              :min="0"
              :max="1000"
              style="width: 150px;"
              placeholder="0"
            />
            <span>(0 = 无限制)</span>
          </n-space>
        </n-form-item>
        <n-form-item label="最大代理链">
          <n-space>
            <n-input-number
              v-model:value="form.max_proxy_chains"
              :min="0"
              :max="1000"
              style="width: 150px;"
              placeholder="0"
            />
            <span>(0 = 无限制)</span>
          </n-space>
        </n-form-item>
        <n-form-item label="最大节点组">
          <n-space>
            <n-input-number
              v-model:value="form.max_node_groups"
              :min="0"
              :max="1000"
              style="width: 150px;"
              placeholder="0"
            />
            <span>(0 = 无限制)</span>
          </n-space>
        </n-form-item>
        <n-collapse>
          <n-collapse-item title="资源范围配置" name="resources">
            <n-alert type="info" style="margin-bottom: 12px;">
              选择套餐可访问的具体资源。不选择则表示不限制该类型资源。
            </n-alert>
            <n-form-item label="可用节点">
              <n-select
                v-model:value="planResources.node"
                multiple
                :options="allNodes.map(n => ({label: n.name, value: n.id}))"
                placeholder="不选择则不限制"
                clearable
                filterable
                style="width: 100%;"
              />
            </n-form-item>
            <n-form-item label="可用隧道">
              <n-select
                v-model:value="planResources.tunnel"
                multiple
                :options="allTunnels.map(t => ({label: t.name, value: t.id}))"
                placeholder="不选择则不限制"
                clearable
                filterable
                style="width: 100%;"
              />
            </n-form-item>
            <n-form-item label="可用端口转发">
              <n-select
                v-model:value="planResources.port_forward"
                multiple
                :options="allPortForwards.map(p => ({label: p.name, value: p.id}))"
                placeholder="不选择则不限制"
                clearable
                filterable
                style="width: 100%;"
              />
            </n-form-item>
            <n-form-item label="可用代理链">
              <n-select
                v-model:value="planResources.proxy_chain"
                multiple
                :options="allProxyChains.map(c => ({label: c.name, value: c.id}))"
                placeholder="不选择则不限制"
                clearable
                filterable
                style="width: 100%;"
              />
            </n-form-item>
            <n-form-item label="可用节点组">
              <n-select
                v-model:value="planResources.node_group"
                multiple
                :options="allNodeGroups.map(g => ({label: g.name, value: g.id}))"
                placeholder="不选择则不限制"
                clearable
                filterable
                style="width: 100%;"
              />
            </n-form-item>
          </n-collapse-item>
        </n-collapse>
        <n-divider title-placement="left">其他设置</n-divider>
        <n-form-item label="排序顺序">
          <n-input-number
            v-model:value="form.sort_order"
            :min="0"
            :max="999"
            style="width: 150px;"
          />
        </n-form-item>
        <n-form-item label="启用套餐">
          <n-switch v-model:value="form.enabled" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showCreateModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted, computed } from 'vue'
import { NButton, NSpace, NTag, useMessage, useDialog, NTooltip, NCollapse, NCollapseItem, NAlert, NSelect } from 'naive-ui'
import { getPlans, createPlan, updatePlan, deletePlan, getPlanResources, setPlanResources, getNodes, getTunnels, getPortForwards, getProxyChains, getNodeGroups } from '../api'
import EmptyState from '../components/EmptyState.vue'
import TableSkeleton from '../components/TableSkeleton.vue'
import { useKeyboard } from '../composables/useKeyboard'

const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const saving = ref(false)
const plans = ref<any[]>([])
const searchText = ref('')
const showCreateModal = ref(false)
const editingPlan = ref<any>(null)

// 资源关联
const allNodes = ref<any[]>([])
const allTunnels = ref<any[]>([])
const allPortForwards = ref<any[]>([])
const allProxyChains = ref<any[]>([])
const allNodeGroups = ref<any[]>([])
const planResources = ref<Record<string, number[]>>({
  node: [],
  tunnel: [],
  port_forward: [],
  proxy_chain: [],
  node_group: [],
})

// 搜索过滤
const filteredPlans = computed(() => {
  if (!searchText.value) return plans.value
  const search = searchText.value.toLowerCase()
  return plans.value.filter((plan: any) =>
    plan.name?.toLowerCase().includes(search) ||
    plan.description?.toLowerCase().includes(search)
  )
})

const defaultForm = () => ({
  name: '',
  description: '',
  traffic_quota: 0,
  speed_limit: 0,
  duration: 30,
  max_nodes: 0,
  max_clients: 0,
  max_tunnels: 0,
  max_port_forwards: 0,
  max_proxy_chains: 0,
  max_node_groups: 0,
  enabled: true,
  sort_order: 0,
})

const form = ref(defaultForm())

// GB 单位转换
const trafficQuotaGB = computed({
  get: () => Math.round((form.value.traffic_quota || 0) / (1024 * 1024 * 1024)),
  set: (val: number) => { form.value.traffic_quota = val * 1024 * 1024 * 1024 }
})

// Mbps 单位转换 (1 Mbps = 125000 bytes/s)
const speedLimitMbps = computed({
  get: () => Math.round((form.value.speed_limit || 0) / 125000),
  set: (val: number) => { form.value.speed_limit = val * 125000 }
})

// 格式化流量
const formatTraffic = (bytes: number) => {
  if (!bytes || bytes === 0) return '无限制'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let size = bytes
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024
    i++
  }
  return `${size.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

// 格式化速度
const formatSpeed = (bytesPerSec: number) => {
  if (!bytesPerSec || bytesPerSec === 0) return '不限速'
  const mbps = bytesPerSec / 125000
  if (mbps >= 1000) {
    return `${(mbps / 1000).toFixed(1)} Gbps`
  }
  return `${mbps.toFixed(0)} Mbps`
}

// 格式化有效期
const formatDuration = (days: number) => {
  if (!days || days === 0) return '永久'
  if (days >= 365) {
    const years = Math.floor(days / 365)
    const remainDays = days % 365
    if (remainDays === 0) return `${years} 年`
    return `${years} 年 ${remainDays} 天`
  }
  if (days >= 30) {
    const months = Math.floor(days / 30)
    const remainDays = days % 30
    if (remainDays === 0) return `${months} 个月`
    return `${months} 个月 ${remainDays} 天`
  }
  return `${days} 天`
}

const columns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '套餐名称', key: 'name', width: 150 },
  {
    title: '流量配额',
    key: 'traffic_quota',
    width: 120,
    render: (row: any) => formatTraffic(row.traffic_quota)
  },
  {
    title: '速度限制',
    key: 'speed_limit',
    width: 100,
    render: (row: any) => formatSpeed(row.speed_limit)
  },
  {
    title: '有效期',
    key: 'duration',
    width: 100,
    render: (row: any) => formatDuration(row.duration)
  },
  {
    title: '资源限制',
    key: 'limits',
    width: 200,
    render: (row: any) => {
      const nodes = row.max_nodes || '无限'
      const clients = row.max_clients || '无限'
      const tunnels = row.max_tunnels || '无限'
      const pfs = row.max_port_forwards || '无限'
      const chains = row.max_proxy_chains || '无限'
      const groups = row.max_node_groups || '无限'
      return h(NTooltip, {}, {
        trigger: () => `${nodes} 节点 / ${clients} 客户端`,
        default: () => h('div', {}, [
          h('div', `节点: ${nodes}`),
          h('div', `客户端: ${clients}`),
          h('div', `隧道: ${tunnels}`),
          h('div', `端口转发: ${pfs}`),
          h('div', `代理链: ${chains}`),
          h('div', `节点组: ${groups}`)
        ])
      })
    }
  },
  {
    title: '用户数',
    key: 'user_count',
    width: 80,
    render: (row: any) => h(NTag, { type: 'info', size: 'small' }, () => row.user_count || 0)
  },
  {
    title: '状态',
    key: 'enabled',
    width: 80,
    render: (row: any) =>
      h(NTag, { type: row.enabled ? 'success' : 'default', size: 'small' }, () => row.enabled ? '启用' : '禁用'),
  },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    render: (row: any) =>
      h(NSpace, { size: 'small' }, () => [
        h(NButton, { size: 'small', onClick: () => handleEdit(row) }, () => '编辑'),
        h(NButton, {
          size: 'small',
          type: 'error',
          onClick: () => handleDelete(row),
          disabled: row.user_count > 0
        }, () => '删除'),
      ]),
  },
]

const loadPlans = async () => {
  loading.value = true
  try {
    const data: any = await getPlans()
    plans.value = data || []
  } catch (e) {
    message.error('加载套餐失败')
  } finally {
    loading.value = false
  }
}

// 加载资源选项
const loadResourceOptions = async () => {
  try {
    const [nodes, tunnels, pfs, chains, groups] = await Promise.all([
      getNodes(),
      getTunnels(),
      getPortForwards(),
      getProxyChains(),
      getNodeGroups()
    ])
    allNodes.value = (nodes as any) || []
    allTunnels.value = (tunnels as any) || []
    allPortForwards.value = (pfs as any) || []
    allProxyChains.value = (chains as any) || []
    allNodeGroups.value = (groups as any) || []
  } catch (e) {
    message.error('加载资源列表失败')
  }
}

const openCreateModal = async () => {
  form.value = defaultForm()
  editingPlan.value = null
  planResources.value = {
    node: [],
    tunnel: [],
    port_forward: [],
    proxy_chain: [],
    node_group: [],
  }
  await loadResourceOptions()
  showCreateModal.value = true
}

const handleEdit = async (row: any) => {
  editingPlan.value = row
  form.value = {
    ...defaultForm(),
    ...row,
  }
  await loadResourceOptions()
  try {
    const resources = await getPlanResources(row.id)
    planResources.value = (resources as any) || {
      node: [],
      tunnel: [],
      port_forward: [],
      proxy_chain: [],
      node_group: [],
    }
  } catch (e) {
    planResources.value = {
      node: [],
      tunnel: [],
      port_forward: [],
      proxy_chain: [],
      node_group: [],
    }
  }
  showCreateModal.value = true
}

const handleSave = async () => {
  if (!form.value.name) {
    message.error('请输入套餐名称')
    return
  }

  saving.value = true
  try {
    let planId: number
    if (editingPlan.value) {
      await updatePlan(editingPlan.value.id, form.value)
      planId = editingPlan.value.id
      message.success('套餐已更新')
    } else {
      const result: any = await createPlan(form.value)
      planId = result.id
      message.success('套餐已创建')
    }

    // 保存资源关联
    try {
      await setPlanResources(planId, planResources.value)
    } catch (e: any) {
      console.error('保存资源关联失败:', e)
      message.warning('套餐已保存，但资源关联配置失败')
    }

    showCreateModal.value = false
    loadPlans()
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存套餐失败')
  } finally {
    saving.value = false
  }
}

const handleDelete = (row: any) => {
  if (row.user_count > 0) {
    message.error('该套餐正在被使用，无法删除')
    return
  }

  dialog.warning({
    title: '删除套餐',
    content: `确定要删除套餐 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deletePlan(row.id)
        message.success('套餐已删除')
        loadPlans()
      } catch (e: any) {
        message.error(e.response?.data?.error || '删除套餐失败')
      }
    },
  })
}

onMounted(() => {
  loadPlans()
})

// Keyboard shortcuts
useKeyboard({
  onNew: openCreateModal,
  modalVisible: showCreateModal,
  onSave: handleSave,
})
</script>

<style scoped>
</style>
