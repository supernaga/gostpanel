<template>
  <div class="tunnels">
    <n-card>
      <template #header>
        <n-space justify="space-between" align="center">
          <span>隧道转发</span>
          <n-space>
            <n-input
              v-model:value="searchText"
              placeholder="搜索隧道名称、描述、目标地址..."
              clearable
              style="width: 280px;"
            >
              <template #prefix>
                <span>🔍</span>
              </template>
            </n-input>
            <n-button type="primary" @click="openCreateModal" v-if="userStore.canWrite">
              添加隧道
            </n-button>
          </n-space>
        </n-space>
      </template>

      <n-alert type="info" style="margin-bottom: 16px;">
        隧道转发：用户 → 入口节点(A) → 出口节点(B) → 目标网站。入口节点监听端口，流量通过出口节点转发。
      </n-alert>

      <!-- 骨架屏加载 -->
      <TableSkeleton v-if="loading && tunnels.length === 0" :rows="3" />

      <!-- 空状态 -->
      <EmptyState
        v-else-if="!loading && tunnels.length === 0"
        type="tunnels"
        :action-text="userStore.canWrite ? '添加隧道' : undefined"
        @action="openCreateModal"
      />

      <!-- 搜索无结果 -->
      <EmptyState
        v-else-if="searchText && filteredTunnels.length === 0"
        type="search"
        :description="`未找到包含 '${searchText}' 的隧道`"
      />

      <!-- 数据表格 -->
      <n-data-table
        v-else
        :columns="columns"
        :data="filteredTunnels"
        :loading="loading"
        :row-key="(row: any) => row.id"
      />
    </n-card>

    <!-- Create/Edit Modal -->
    <n-modal v-model:show="showCreateModal" preset="dialog" :title="editingTunnel ? '编辑隧道' : '添加隧道'" style="width: 650px;">
      <n-form :model="form" label-placement="left" label-width="100">
        <n-form-item label="名称" required>
          <n-input v-model:value="form.name" placeholder="例如: HK-US隧道" />
        </n-form-item>
        <n-form-item label="描述">
          <n-input v-model:value="form.description" placeholder="隧道用途说明" />
        </n-form-item>

        <n-divider>入口端配置</n-divider>

        <n-form-item label="入口节点" required>
          <n-select
            v-model:value="form.entry_node_id"
            :options="nodeOptions"
            placeholder="选择入口节点 (用户连接的节点)"
            filterable
          />
        </n-form-item>
        <n-form-item label="监听端口" required>
          <n-input-number v-model:value="form.entry_port" :min="1" :max="65535" style="width: 200px">
            <template #suffix>端口</template>
          </n-input-number>
        </n-form-item>
        <n-form-item label="协议">
          <n-select v-model:value="form.protocol" :options="protocolOptions" style="width: 200px" />
        </n-form-item>

        <n-divider>出口端配置</n-divider>

        <n-form-item label="出口节点" required>
          <n-select
            v-model:value="form.exit_node_id"
            :options="exitNodeOptions"
            placeholder="选择出口节点 (落地节点)"
            filterable
          />
        </n-form-item>
        <n-form-item label="目标地址">
          <n-input v-model:value="form.target_addr" placeholder="留空则使用代理模式，填写则为端口转发 (如 8.8.8.8:53)">
            <template #prefix>可选</template>
          </n-input>
        </n-form-item>

        <n-divider>限制配置</n-divider>

        <n-grid :cols="2" :x-gap="12">
          <n-grid-item>
            <n-form-item label="流量配额">
              <n-input-number v-model:value="form.traffic_quota_gb" :min="0" :precision="2" style="width: 100%">
                <template #suffix>GB</template>
              </n-input-number>
            </n-form-item>
          </n-grid-item>
          <n-grid-item>
            <n-form-item label="限速">
              <n-input-number v-model:value="form.speed_limit_mbps" :min="0" :precision="2" style="width: 100%">
                <template #suffix>Mbps</template>
              </n-input-number>
            </n-form-item>
          </n-grid-item>
        </n-grid>

        <n-form-item label="启用">
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

    <!-- Config Modal -->
    <n-modal v-model:show="showConfigModal" preset="dialog" title="GOST 配置" style="width: 800px;">
      <n-tabs type="line">
        <n-tab-pane name="entry" tab="入口端配置">
          <n-alert type="info" style="margin-bottom: 12px;">
            将此配置部署到入口节点 ({{ currentTunnel?.entry_node?.name || '入口节点' }})
          </n-alert>
          <n-scrollbar style="max-height: 350px;">
            <n-code :code="entryConfig" language="yaml" word-wrap />
          </n-scrollbar>
          <n-button style="margin-top: 12px;" @click="copyConfig(entryConfig)">复制入口配置</n-button>
        </n-tab-pane>
        <n-tab-pane name="exit" tab="出口端配置">
          <n-alert type="info" style="margin-bottom: 12px;">
            将此配置部署到出口节点 ({{ currentTunnel?.exit_node?.name || '出口节点' }})
          </n-alert>
          <n-scrollbar style="max-height: 350px;">
            <n-code :code="exitConfig" language="yaml" word-wrap />
          </n-scrollbar>
          <n-button style="margin-top: 12px;" @click="copyConfig(exitConfig)">复制出口配置</n-button>
        </n-tab-pane>
      </n-tabs>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted, computed } from 'vue'
import { NButton, NSpace, NTag, NDropdown, useMessage, useDialog } from 'naive-ui'
import { getTunnels, createTunnel, updateTunnel, deleteTunnel, syncTunnel, getTunnelEntryConfig, getTunnelExitConfig, cloneTunnel, getNodes } from '../api'
import EmptyState from '../components/EmptyState.vue'
import TableSkeleton from '../components/TableSkeleton.vue'
import { useUserStore } from '../stores/user'

const userStore = useUserStore()
const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const saving = ref(false)
const tunnels = ref<any[]>([])
const searchText = ref('')
const allNodes = ref<any[]>([])
const showCreateModal = ref(false)
const showConfigModal = ref(false)
const entryConfig = ref('')
const exitConfig = ref('')
const editingTunnel = ref<any>(null)
const currentTunnel = ref<any>(null)

// 搜索过滤
const filteredTunnels = computed(() => {
  if (!searchText.value) return tunnels.value
  const search = searchText.value.toLowerCase()
  return tunnels.value.filter((tunnel: any) =>
    tunnel.name?.toLowerCase().includes(search) ||
    tunnel.description?.toLowerCase().includes(search) ||
    tunnel.target_addr?.toLowerCase().includes(search)
  )
})

const protocolOptions = [
  { label: 'TCP+UDP (端口复用)', value: 'tcp+udp' },
  { label: '仅 TCP', value: 'tcp' },
  { label: '仅 UDP', value: 'udp' },
]

const defaultForm = () => ({
  name: '',
  description: '',
  entry_node_id: null as number | null,
  entry_port: 10000,
  protocol: 'tcp+udp',
  exit_node_id: null as number | null,
  target_addr: '',
  traffic_quota_gb: 0,
  speed_limit_mbps: 0,
  enabled: true,
})

const form = ref(defaultForm())

const nodeOptions = computed(() =>
  allNodes.value.map((n: any) => ({
    label: `${n.name} (${n.host}:${n.port})`,
    value: n.id,
  }))
)

const exitNodeOptions = computed(() =>
  allNodes.value
    .filter((n: any) => n.id !== form.value.entry_node_id)
    .map((n: any) => ({
      label: `${n.name} (${n.host}:${n.port}) - ${n.protocol}`,
      value: n.id,
    }))
)

const formatTraffic = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const columns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '名称', key: 'name', width: 150 },
  {
    title: '入口节点',
    key: 'entry_node',
    width: 150,
    render: (row: any) => row.entry_node?.name || '-',
  },
  {
    title: '监听',
    key: 'entry_port',
    width: 130,
    render: (row: any) => `:${row.entry_port} (${row.protocol || 'tcp+udp'})`,
  },
  {
    title: '出口节点',
    key: 'exit_node',
    width: 150,
    render: (row: any) => row.exit_node?.name || '-',
  },
  {
    title: '目标',
    key: 'target_addr',
    width: 150,
    render: (row: any) => row.target_addr || '代理模式',
  },
  {
    title: '流量',
    key: 'traffic',
    width: 120,
    render: (row: any) => `↑${formatTraffic(row.traffic_out)} ↓${formatTraffic(row.traffic_in)}`,
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
    width: 320,
    render: (row: any) => {
      const allDropdownOptions = [
        { label: '克隆隧道', key: 'clone' },
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
        buttons.push(h(NButton, { size: 'small', type: 'primary', onClick: () => handleSync(row) }, () => '同步'))
      }
      buttons.push(h(NButton, { size: 'small', type: 'info', onClick: () => handleShowConfig(row) }, () => '配置'))
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

const syncing = ref<number | null>(null)

const handleSync = async (row: any) => {
  syncing.value = row.id
  try {
    const result: any = await syncTunnel(row.id)
    message.success(result.message || '同步成功')
  } catch (e: any) {
    message.error(e.response?.data?.message || e.response?.data?.error || '同步失败')
  } finally {
    syncing.value = null
  }
}

const loadTunnels = async () => {
  loading.value = true
  try {
    const data: any = await getTunnels()
    tunnels.value = data || []
  } catch (e) {
    message.error('加载隧道列表失败')
  } finally {
    loading.value = false
  }
}

const loadNodes = async () => {
  try {
    const data: any = await getNodes()
    allNodes.value = data || []
  } catch (e) {
    console.error('Failed to load nodes', e)
  }
}

const openCreateModal = () => {
  form.value = defaultForm()
  editingTunnel.value = null
  showCreateModal.value = true
}

const handleEdit = (row: any) => {
  editingTunnel.value = row
  form.value = {
    name: row.name,
    description: row.description || '',
    entry_node_id: row.entry_node_id,
    entry_port: row.entry_port,
    protocol: row.protocol || 'tcp',
    exit_node_id: row.exit_node_id,
    target_addr: row.target_addr || '',
    traffic_quota_gb: row.traffic_quota ? row.traffic_quota / (1024 * 1024 * 1024) : 0,
    speed_limit_mbps: row.speed_limit ? row.speed_limit / (1024 * 1024 / 8) : 0,
    enabled: row.enabled,
  }
  showCreateModal.value = true
}

const handleSave = async () => {
  if (!form.value.name) {
    message.error('请输入名称')
    return
  }
  if (!form.value.entry_node_id) {
    message.error('请选择入口节点')
    return
  }
  if (!form.value.exit_node_id) {
    message.error('请选择出口节点')
    return
  }

  saving.value = true
  try {
    const payload = {
      ...form.value,
      traffic_quota: Math.round(form.value.traffic_quota_gb * 1024 * 1024 * 1024),
      speed_limit: Math.round(form.value.speed_limit_mbps * 1024 * 1024 / 8),
    }
    delete (payload as any).traffic_quota_gb
    delete (payload as any).speed_limit_mbps

    if (editingTunnel.value) {
      await updateTunnel(editingTunnel.value.id, payload)
      message.success('隧道已更新')
    } else {
      await createTunnel(payload)
      message.success('隧道已创建')
    }
    showCreateModal.value = false
    loadTunnels()
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const handleDelete = (row: any) => {
  dialog.warning({
    title: '删除隧道',
    content: `确定要删除隧道 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteTunnel(row.id)
        message.success('隧道已删除')
        loadTunnels()
      } catch (e) {
        message.error('删除失败')
      }
    },
  })
}

const handleClone = async (row: any) => {
  try {
    await cloneTunnel(row.id)
    message.success(`隧道 "${row.name}" 已克隆`)
    loadTunnels()
  } catch {
    message.error('克隆隧道失败')
  }
}

const handleShowConfig = async (row: any) => {
  currentTunnel.value = row
  try {
    const [entryData, exitData] = await Promise.all([
      getTunnelEntryConfig(row.id),
      getTunnelExitConfig(row.id),
    ])
    entryConfig.value = typeof entryData === 'string' ? entryData : JSON.stringify(entryData, null, 2)
    exitConfig.value = typeof exitData === 'string' ? exitData : JSON.stringify(exitData, null, 2)
    showConfigModal.value = true
  } catch (e) {
    message.error('获取配置失败')
  }
}

const copyConfig = (config: string) => {
  navigator.clipboard.writeText(config)
  message.success('已复制到剪贴板')
}

onMounted(() => {
  loadTunnels()
  loadNodes()
})
</script>

<style scoped>
</style>
