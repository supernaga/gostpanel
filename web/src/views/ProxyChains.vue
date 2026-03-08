<template>
  <div class="proxy-chains">
    <n-card>
      <template #header>
        <n-space justify="space-between" align="center">
          <span>隧道转发 / 代理链</span>
          <n-space>
            <n-input
              v-model:value="searchText"
              placeholder="搜索隧道名称、描述、监听地址..."
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
        隧道转发支持多跳代理链：用户 → 国内VPS(中转) → 国外VPS(落地) → 目标网站
      </n-alert>

      <!-- 骨架屏加载 -->
      <TableSkeleton v-if="loading && proxyChains.length === 0" :rows="3" />

      <!-- 空状态 -->
      <EmptyState
        v-else-if="!loading && proxyChains.length === 0"
        type="tunnels"
        :action-text="userStore.canWrite ? '添加隧道' : undefined"
        @action="openCreateModal"
      />

      <!-- 搜索无结果 -->
      <EmptyState
        v-else-if="searchText && filteredProxyChains.length === 0"
        type="search"
        :description="`未找到包含 '${searchText}' 的隧道`"
      />

      <n-data-table
        v-else
        :columns="columns"
        :data="filteredProxyChains"
        :loading="loading"
        :row-key="(row: any) => row.id"
      />
    </n-card>

    <!-- Create/Edit Modal -->
    <n-modal v-model:show="showCreateModal" preset="dialog" :title="editingChain ? '编辑隧道' : '添加隧道'" style="width: 600px;">
      <n-form :model="form" label-placement="left" label-width="100">
        <n-form-item label="名称" required>
          <n-input v-model:value="form.name" placeholder="例如: HK-US隧道" />
        </n-form-item>
        <n-form-item label="描述">
          <n-input v-model:value="form.description" placeholder="隧道用途说明" />
        </n-form-item>
        <n-form-item label="监听地址" required>
          <n-input v-model:value="form.listen_addr" placeholder="例如: :1080">
            <template #prefix>本地端口</template>
          </n-input>
        </n-form-item>
        <n-form-item label="监听类型">
          <n-select v-model:value="form.listen_type" :options="listenTypeOptions" />
        </n-form-item>
        <n-form-item label="目标地址">
          <n-input v-model:value="form.target_addr" placeholder="留空则使用代理模式，填写则为端口转发">
            <template #prefix>可选</template>
          </n-input>
        </n-form-item>
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

    <!-- Hops Modal -->
    <n-modal v-model:show="showHopsModal" preset="dialog" :title="`管理跳点: ${currentChain?.name}`" style="width: 850px;">
      <n-space vertical size="large">
        <n-alert type="info">
          按顺序添加代理节点，流量将依次经过每个节点。例如：用户 → 节点1(国内中转) → 节点2(国外落地) → 目标
        </n-alert>
        <n-space justify="space-between" align="center">
          <span>跳点列表 (按顺序转发)</span>
          <n-button type="primary" size="small" @click="openAddHopModal" v-if="userStore.canWrite">
            添加跳点
          </n-button>
        </n-space>
        <n-data-table
          :columns="hopColumns"
          :data="hops"
          :loading="hopsLoading"
          :row-key="(row: any) => row.id"
          size="small"
          max-height="400"
        />
      </n-space>
      <template #action>
        <n-button @click="showHopsModal = false">关闭</n-button>
      </template>
    </n-modal>

    <!-- Add Hop Modal -->
    <n-modal v-model:show="showAddHopModal" preset="dialog" title="添加跳点" style="width: 500px;">
      <n-form :model="hopForm" label-placement="left" label-width="100">
        <n-form-item label="选择节点">
          <n-select v-model:value="hopForm.node_id" :options="availableNodeOptions" filterable placeholder="选择要添加的节点" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showAddHopModal = false">取消</n-button>
          <n-button type="primary" :loading="addingHop" @click="handleAddHop">添加</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Config Modal -->
    <n-modal v-model:show="showConfigModal" preset="dialog" title="GOST 配置" style="width: 750px;">
      <n-alert type="info" style="margin-bottom: 16px;">
        将此配置保存到 GOST 配置文件即可启用多跳隧道转发
      </n-alert>
      <n-scrollbar style="max-height: 400px;">
        <n-code :code="configContent" language="yaml" word-wrap />
      </n-scrollbar>
      <template #action>
        <n-button @click="copyConfig">复制配置</n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted, computed } from 'vue'
import { NButton, NSpace, NTag, NDropdown, useMessage, useDialog } from 'naive-ui'
import { getProxyChains, createProxyChain, updateProxyChain, deleteProxyChain, getProxyChainHops, addProxyChainHop, removeProxyChainHop, getProxyChainConfig, cloneProxyChain, getNodes } from '../api'
import EmptyState from '../components/EmptyState.vue'
import TableSkeleton from '../components/TableSkeleton.vue'
import { useUserStore } from '../stores/user'

const userStore = useUserStore()
const message = useMessage()
const dialog = useDialog()

const loading = ref(false)
const saving = ref(false)
const hopsLoading = ref(false)
const addingHop = ref(false)
const proxyChains = ref<any[]>([])
const searchText = ref('')
const hops = ref<any[]>([])
const allNodes = ref<any[]>([])
const showCreateModal = ref(false)
const showHopsModal = ref(false)
const showAddHopModal = ref(false)
const showConfigModal = ref(false)
const configContent = ref('')
const editingChain = ref<any>(null)
const currentChain = ref<any>(null)

// 搜索过滤
const filteredProxyChains = computed(() => {
  if (!searchText.value) return proxyChains.value
  const search = searchText.value.toLowerCase()
  return proxyChains.value.filter((chain: any) =>
    chain.name?.toLowerCase().includes(search) ||
    chain.description?.toLowerCase().includes(search) ||
    chain.listen_addr?.toLowerCase().includes(search)
  )
})

const listenTypeOptions = [
  { label: 'SOCKS5 代理', value: 'socks5' },
  { label: 'HTTP 代理', value: 'http' },
  { label: 'TCP 转发', value: 'tcp' },
  { label: 'UDP 转发', value: 'udp' },
]

const defaultForm = () => ({
  name: '',
  description: '',
  listen_addr: ':1080',
  listen_type: 'socks5',
  target_addr: '',
  enabled: true,
})

const form = ref(defaultForm())

const hopForm = ref({
  node_id: null as number | null,
})

const availableNodeOptions = computed(() => {
  const hopNodeIds = new Set(hops.value.map((h: any) => h.node_id))
  return allNodes.value
    .filter((n: any) => !hopNodeIds.has(n.id))
    .map((n: any) => ({
      label: `${n.name} (${n.host}:${n.port}) - ${n.protocol}`,
      value: n.id,
    }))
})

const columns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '名称', key: 'name', width: 150 },
  { title: '描述', key: 'description', ellipsis: { tooltip: true } },
  { title: '监听', key: 'listen_addr', width: 100 },
  {
    title: '类型',
    key: 'listen_type',
    width: 100,
    render: (row: any) => h(NTag, { type: 'info', size: 'small' }, () => row.listen_type?.toUpperCase() || 'SOCKS5'),
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
        buttons.push(h(NButton, { size: 'small', type: 'primary', onClick: () => handleManageHops(row) }, () => '跳点'))
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

const hopColumns = [
  { title: '顺序', key: 'hop_order', width: 60, render: (row: any) => row.hop_order + 1 },
  { title: '节点名称', key: 'node.name', width: 150, render: (row: any) => row.node?.name || '-' },
  { title: '地址', key: 'node.host', render: (row: any) => row.node ? `${row.node.host}:${row.node.port}` : '-' },
  { title: '协议', key: 'node.protocol', width: 100, render: (row: any) => row.node?.protocol || 'socks5' },
  {
    title: '状态',
    key: 'node.status',
    width: 80,
    render: (row: any) =>
      h(NTag, { type: row.node?.status === 'online' ? 'success' : 'default', size: 'small' }, () => row.node?.status === 'online' ? '在线' : '离线'),
  },
  {
    title: '启用',
    key: 'enabled',
    width: 80,
    render: (row: any) =>
      h(NTag, { type: row.enabled ? 'success' : 'default', size: 'small' }, () => row.enabled ? '是' : '否'),
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row: any) =>
      userStore.canWrite
        ? h(NButton, { size: 'small', type: 'error', onClick: () => handleRemoveHop(row) }, () => '移除')
        : null,
  },
]

const loadProxyChains = async () => {
  loading.value = true
  try {
    const data: any = await getProxyChains()
    proxyChains.value = data || []
  } catch (e) {
    message.error('加载隧道列表失败')
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
  editingChain.value = null
  showCreateModal.value = true
}

const handleEdit = (row: any) => {
  editingChain.value = row
  form.value = { ...defaultForm(), ...row }
  showCreateModal.value = true
}

const handleSave = async () => {
  if (!form.value.name) {
    message.error('请输入名称')
    return
  }
  if (!form.value.listen_addr) {
    message.error('请输入监听地址')
    return
  }

  saving.value = true
  try {
    if (editingChain.value) {
      await updateProxyChain(editingChain.value.id, form.value)
      message.success('隧道已更新')
    } else {
      await createProxyChain(form.value)
      message.success('隧道已创建')
    }
    showCreateModal.value = false
    loadProxyChains()
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
        await deleteProxyChain(row.id)
        message.success('隧道已删除')
        loadProxyChains()
      } catch (e) {
        message.error('删除失败')
      }
    },
  })
}

const handleClone = async (row: any) => {
  try {
    await cloneProxyChain(row.id)
    message.success(`隧道 "${row.name}" 已克隆`)
    loadProxyChains()
  } catch {
    message.error('克隆失败')
  }
}

const handleManageHops = async (row: any) => {
  currentChain.value = row
  showHopsModal.value = true
  hopsLoading.value = true
  try {
    const data: any = await getProxyChainHops(row.id)
    hops.value = data || []
  } catch (e) {
    message.error('加载跳点失败')
  } finally {
    hopsLoading.value = false
  }
}

const openAddHopModal = () => {
  hopForm.value = { node_id: null }
  showAddHopModal.value = true
}

const handleAddHop = async () => {
  if (!hopForm.value.node_id) {
    message.error('请选择节点')
    return
  }

  addingHop.value = true
  try {
    await addProxyChainHop(currentChain.value.id, { node_id: hopForm.value.node_id, enabled: true })
    message.success('跳点已添加')
    showAddHopModal.value = false
    // 刷新跳点列表
    const data: any = await getProxyChainHops(currentChain.value.id)
    hops.value = data || []
  } catch (e: any) {
    message.error(e.response?.data?.error || '添加失败')
  } finally {
    addingHop.value = false
  }
}

const handleRemoveHop = async (hop: any) => {
  dialog.warning({
    title: '移除跳点',
    content: `确定要移除节点 "${hop.node?.name}" 吗？`,
    positiveText: '移除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await removeProxyChainHop(currentChain.value.id, hop.id)
        message.success('跳点已移除')
        // 刷新跳点列表
        const data: any = await getProxyChainHops(currentChain.value.id)
        hops.value = data || []
      } catch (e) {
        message.error('移除失败')
      }
    },
  })
}

const handleShowConfig = async (row: any) => {
  try {
    const config: any = await getProxyChainConfig(row.id)
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
  loadProxyChains()
  loadAllNodes()
})
</script>

<style scoped>
</style>
