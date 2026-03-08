<template>
  <div class="node-groups">
    <n-card>
      <template #header>
        <n-space justify="space-between" align="center">
          <span>节点组 / 负载均衡</span>
          <n-space>
            <n-input
              v-model:value="searchText"
              placeholder="搜索节点组名称、策略..."
              clearable
              style="width: 250px;"
            >
              <template #prefix>
                <span>🔍</span>
              </template>
            </n-input>
            <n-button type="primary" @click="openCreateModal" v-if="userStore.canWrite">
              添加节点组
            </n-button>
          </n-space>
        </n-space>
      </template>

      <!-- 骨架屏加载 -->
      <TableSkeleton v-if="loading && nodeGroups.length === 0" :rows="3" />

      <!-- 空状态 -->
      <EmptyState
        v-else-if="!loading && nodeGroups.length === 0"
        type="groups"
        :action-text="userStore.canWrite ? '添加节点组' : undefined"
        @action="openCreateModal"
      />

      <!-- 搜索无结果 -->
      <EmptyState
        v-else-if="searchText && filteredNodeGroups.length === 0"
        type="search"
        :description="`未找到包含 '${searchText}' 的节点组`"
      />

      <!-- 数据表格 -->
      <n-data-table
        v-else
        :columns="columns"
        :data="filteredNodeGroups"
        :loading="loading"
        :row-key="(row: any) => row.id"
      />
    </n-card>

    <!-- Create/Edit Modal -->
    <n-modal v-model:show="showCreateModal" preset="dialog" :title="editingGroup ? '编辑节点组' : '添加节点组'" style="width: 600px;">
      <n-form :model="form" label-placement="left" label-width="120">
        <n-form-item label="组名">
          <n-input v-model:value="form.name" placeholder="例如: HK-Group" />
        </n-form-item>
        <n-form-item label="负载均衡策略">
          <n-select v-model:value="form.strategy" :options="strategyOptions" />
        </n-form-item>
        <n-form-item label="健康检查">
          <n-switch v-model:value="form.health_check_enabled" />
        </n-form-item>
        <template v-if="form.health_check_enabled">
          <n-form-item label="检查间隔">
            <n-space>
              <n-input-number v-model:value="healthCheckIntervalSec" :min="10" style="width: 120px" />
              <span>秒</span>
            </n-space>
          </n-form-item>
          <n-form-item label="检查超时">
            <n-space>
              <n-input-number v-model:value="healthCheckTimeoutSec" :min="1" style="width: 120px" />
              <span>秒</span>
            </n-space>
          </n-form-item>
          <n-form-item label="最大失败次数">
            <n-input-number v-model:value="form.max_fails" :min="1" style="width: 120px" />
          </n-form-item>
        </template>
        <n-form-item label="描述">
          <n-input v-model:value="form.description" type="textarea" placeholder="节点组描述" :autosize="{ minRows: 2 }" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showCreateModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Members Modal -->
    <n-modal v-model:show="showMembersModal" preset="dialog" :title="`管理节点组: ${currentGroup?.name}`" style="width: 800px;">
      <n-space vertical size="large">
        <n-space justify="space-between" align="center">
          <span>节点成员</span>
          <n-button type="primary" size="small" @click="openAddMemberModal" v-if="userStore.canWrite">
            添加节点
          </n-button>
        </n-space>
        <n-data-table
          :columns="memberColumns"
          :data="members"
          :loading="membersLoading"
          :row-key="(row: any) => row.id"
          size="small"
          max-height="400"
        />
      </n-space>
      <template #action>
        <n-button @click="showMembersModal = false">关闭</n-button>
      </template>
    </n-modal>

    <!-- Add Member Modal -->
    <n-modal v-model:show="showAddMemberModal" preset="dialog" title="添加节点成员" style="width: 500px;">
      <n-form :model="memberForm" label-placement="left" label-width="100">
        <n-form-item label="选择节点">
          <n-select v-model:value="memberForm.node_id" :options="availableNodeOptions" filterable placeholder="选择要添加的节点" />
        </n-form-item>
        <n-form-item label="权重">
          <n-input-number v-model:value="memberForm.weight" :min="1" :max="100" style="width: 120px" />
          <n-text depth="3" style="margin-left: 12px; font-size: 12px;">数值越大，流量分配越多</n-text>
        </n-form-item>
        <n-form-item label="优先级">
          <n-input-number v-model:value="memberForm.priority" :min="1" :max="10" style="width: 120px" />
          <n-text depth="3" style="margin-left: 12px; font-size: 12px;">数值越小，优先级越高</n-text>
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showAddMemberModal = false">取消</n-button>
          <n-button type="primary" :loading="addingMember" @click="handleAddMember">添加</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Config Modal -->
    <n-modal v-model:show="showConfigModal" preset="dialog" title="负载均衡配置" style="width: 700px;">
      <n-code :code="configContent" language="yaml" style="max-height: 500px; overflow: auto;" />
      <template #action>
        <n-button @click="copyConfig">复制</n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted, computed } from 'vue'
import { NButton, NSpace, NTag, NDropdown, useMessage, useDialog } from 'naive-ui'
import { getNodeGroups, createNodeGroup, updateNodeGroup, deleteNodeGroup, getNodeGroupMembers, addNodeGroupMember, removeNodeGroupMember, getNodeGroupConfig, cloneNodeGroup, getNodes } from '../api'
import EmptyState from '../components/EmptyState.vue'
import TableSkeleton from '../components/TableSkeleton.vue'
import { useUserStore } from '../stores/user'

const userStore = useUserStore()
const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const saving = ref(false)
const membersLoading = ref(false)
const addingMember = ref(false)
const nodeGroups = ref<any[]>([])
const searchText = ref('')
const members = ref<any[]>([])
const allNodes = ref<any[]>([])
const showCreateModal = ref(false)
const showMembersModal = ref(false)
const showAddMemberModal = ref(false)
const showConfigModal = ref(false)
const configContent = ref('')
const editingGroup = ref<any>(null)
const currentGroup = ref<any>(null)

// 搜索过滤
const filteredNodeGroups = computed(() => {
  if (!searchText.value) return nodeGroups.value
  const search = searchText.value.toLowerCase()
  return nodeGroups.value.filter((group: any) =>
    group.name?.toLowerCase().includes(search) ||
    group.strategy?.toLowerCase().includes(search)
  )
})

const strategyOptions = [
  { label: '轮询 (Round Robin)', value: 'round_robin' },
  { label: '随机 (Random)', value: 'random' },
  { label: '加权轮询 (Weighted)', value: 'weighted' },
  { label: '最少连接 (Least Conn)', value: 'least_conn' },
  { label: '哈希 (Hash)', value: 'hash' },
  { label: 'IP 哈希 (IP Hash)', value: 'ip_hash' },
]

const defaultForm = () => ({
  name: '',
  strategy: 'round_robin',
  health_check_enabled: true,
  health_check_interval: 30000,
  health_check_timeout: 5000,
  max_fails: 3,
  description: '',
})

const form = ref(defaultForm())

const memberForm = ref({
  node_id: null,
  weight: 1,
  priority: 1,
})

const healthCheckIntervalSec = computed({
  get: () => form.value.health_check_interval / 1000,
  set: (val) => { form.value.health_check_interval = val * 1000 }
})

const healthCheckTimeoutSec = computed({
  get: () => form.value.health_check_timeout / 1000,
  set: (val) => { form.value.health_check_timeout = val * 1000 }
})

const availableNodeOptions = computed(() => {
  const memberNodeIds = new Set(members.value.map((m: any) => m.node_id))
  return allNodes.value
    .filter((n: any) => !memberNodeIds.has(n.id))
    .map((n: any) => ({
      label: `${n.name} (${n.host}:${n.port})`,
      value: n.id,
    }))
})

const getStrategyLabel = (strategy: string) => {
  const opt = strategyOptions.find(o => o.value === strategy)
  return opt ? opt.label : strategy
}

const columns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '组名', key: 'name', width: 150 },
  {
    title: '策略',
    key: 'strategy',
    width: 180,
    render: (row: any) => h(NTag, { type: 'info', size: 'small' }, () => getStrategyLabel(row.strategy)),
  },
  {
    title: '健康检查',
    key: 'health_check_enabled',
    width: 100,
    render: (row: any) =>
      h(NTag, { type: row.health_check_enabled ? 'success' : 'default', size: 'small' }, () => row.health_check_enabled ? '启用' : '禁用'),
  },
  { title: '节点数量', key: 'node_count', width: 100 },
  {
    title: '描述',
    key: 'description',
    ellipsis: { tooltip: true },
  },
  {
    title: '操作',
    key: 'actions',
    width: 300,
    render: (row: any) => {
      const allDropdownOptions = [
        { label: '克隆', key: 'clone' },
        { type: 'divider', key: 'd1' },
        { label: '删除', key: 'delete' },
      ]
      const writeOnlyKeys = new Set(['clone', 'delete', 'd1'])
      const dropdownOptions = userStore.canWrite
        ? allDropdownOptions
        : allDropdownOptions.filter(o => !writeOnlyKeys.has(o.key))
      const handleSelect = (key: string) => {
        switch (key) {
          case 'clone': handleClone(row); break
          case 'delete': handleDelete(row); break
        }
      }
      const buttons: any[] = []
      if (userStore.canWrite) {
        buttons.push(h(NButton, { size: 'small', onClick: () => handleEdit(row) }, () => '编辑'))
        buttons.push(h(NButton, { size: 'small', type: 'primary', onClick: () => handleManageMembers(row) }, () => '管理节点'))
      }
      buttons.push(h(NButton, { size: 'small', onClick: () => handleShowConfig(row) }, () => '配置'))
      if (userStore.canWrite && dropdownOptions.length > 0) {
        buttons.push(h(NDropdown, {
          options: dropdownOptions,
          onSelect: handleSelect,
          trigger: 'click'
        }, () => h(NButton, { size: 'small' }, () => '更多')))
      }
      return h(NSpace, { size: 'small' }, () => buttons)
    }
  },
]

const memberColumns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '节点名称', key: 'node_name', width: 150 },
  { title: '地址', key: 'node_host' },
  { title: '权重', key: 'weight', width: 80 },
  { title: '优先级', key: 'priority', width: 80 },
  {
    title: '状态',
    key: 'node_status',
    width: 80,
    render: (row: any) =>
      h(NTag, { type: row.node_status === 'online' ? 'success' : 'default', size: 'small' }, () => row.node_status === 'online' ? '在线' : '离线'),
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row: any) =>
      userStore.canWrite
        ? h(NButton, { size: 'small', type: 'error', onClick: () => handleRemoveMember(row) }, () => '移除')
        : null,
  },
]

const loadNodeGroups = async () => {
  loading.value = true
  try {
    const data: any = await getNodeGroups()
    nodeGroups.value = data || []
  } catch (e) {
    message.error('加载节点组失败')
  } finally {
    loading.value = false
  }
}

const loadAllNodes = async () => {
  try {
    const data: any = await getNodes()
    allNodes.value = data || []
  } catch (e) {
    console.error('Failed to load nodes', e)
  }
}

const openCreateModal = () => {
  form.value = defaultForm()
  editingGroup.value = null
  showCreateModal.value = true
}

const handleEdit = (row: any) => {
  editingGroup.value = row
  form.value = { ...defaultForm(), ...row }
  showCreateModal.value = true
}

const handleSave = async () => {
  if (!form.value.name) {
    message.error('请输入组名')
    return
  }

  saving.value = true
  try {
    if (editingGroup.value) {
      await updateNodeGroup(editingGroup.value.id, form.value)
      message.success('节点组已更新')
    } else {
      await createNodeGroup(form.value)
      message.success('节点组已创建')
    }
    showCreateModal.value = false
    loadNodeGroups()
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存节点组失败')
  } finally {
    saving.value = false
  }
}

const handleDelete = (row: any) => {
  dialog.warning({
    title: '删除节点组',
    content: `确定要删除节点组 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteNodeGroup(row.id)
        message.success('节点组已删除')
        loadNodeGroups()
      } catch (e) {
        message.error('删除节点组失败')
      }
    },
  })
}

const handleClone = async (row: any) => {
  try {
    await cloneNodeGroup(row.id)
    message.success(`节点组 "${row.name}" 已克隆`)
    loadNodeGroups()
  } catch {
    message.error('克隆失败')
  }
}

const handleManageMembers = async (row: any) => {
  currentGroup.value = row
  showMembersModal.value = true
  await loadMembers(row.id)
}

const loadMembers = async (groupId: number) => {
  membersLoading.value = true
  try {
    const data: any = await getNodeGroupMembers(groupId)
    members.value = data || []
  } catch (e) {
    message.error('加载节点成员失败')
  } finally {
    membersLoading.value = false
  }
}

const openAddMemberModal = () => {
  memberForm.value = {
    node_id: null,
    weight: 1,
    priority: 1,
  }
  showAddMemberModal.value = true
}

const handleAddMember = async () => {
  if (!memberForm.value.node_id) {
    message.error('请选择节点')
    return
  }

  addingMember.value = true
  try {
    await addNodeGroupMember(currentGroup.value.id, memberForm.value)
    message.success('节点已添加到组')
    showAddMemberModal.value = false
    await loadMembers(currentGroup.value.id)
    await loadNodeGroups()
  } catch (e: any) {
    message.error(e.response?.data?.error || '添加节点失败')
  } finally {
    addingMember.value = false
  }
}

const handleRemoveMember = (row: any) => {
  dialog.warning({
    title: '移除节点',
    content: `确定要从组中移除节点 "${row.node_name}" 吗？`,
    positiveText: '移除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await removeNodeGroupMember(currentGroup.value.id, row.id)
        message.success('节点已移除')
        await loadMembers(currentGroup.value.id)
        await loadNodeGroups()
      } catch (e) {
        message.error('移除节点失败')
      }
    },
  })
}

const handleShowConfig = async (row: any) => {
  try {
    const config: any = await getNodeGroupConfig(row.id)
    configContent.value = typeof config === 'string' ? config : JSON.stringify(config, null, 2)
    showConfigModal.value = true
  } catch (e) {
    message.error('获取配置失败')
  }
}

const copyConfig = () => {
  navigator.clipboard.writeText(configContent.value)
  message.success('已复制到剪贴板')
}

onMounted(() => {
  loadNodeGroups()
  loadAllNodes()
})
</script>

<style scoped>
</style>
