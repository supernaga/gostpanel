<template>
  <div class="rules">
    <n-card>
      <template #header>
        <span>规则管理</span>
      </template>
      <n-tabs type="line" v-model:value="activeTab">
        <!-- Bypass 分流规则 -->
        <n-tab-pane name="bypass" tab="分流规则 (Bypass)">
          <n-alert type="info" style="margin-bottom: 16px;">
            分流规则控制哪些目标地址走代理或直连。黑名单模式：匹配的地址不走代理；白名单模式：仅匹配的地址走代理。
          </n-alert>
          <n-space justify="end" style="margin-bottom: 12px;">
            <n-button type="primary" size="small" @click="openBypassModal()">添加规则</n-button>
          </n-space>
          <TableSkeleton v-if="bypassLoading && bypasses.length === 0" :rows="3" />
          <EmptyState v-else-if="!bypassLoading && bypasses.length === 0" type="rules" action-text="添加规则" @action="openBypassModal()" />
          <n-data-table v-else :columns="bypassColumns" :data="bypasses" :loading="bypassLoading" :row-key="(row: any) => row.id" size="small" />
        </n-tab-pane>

        <!-- Admission 准入控制 -->
        <n-tab-pane name="admission" tab="准入控制 (Admission)">
          <n-alert type="info" style="margin-bottom: 16px;">
            准入控制限制哪些客户端 IP 可以连接代理服务。黑名单模式：拒绝匹配的 IP；白名单模式：仅允许匹配的 IP。
          </n-alert>
          <n-space justify="end" style="margin-bottom: 12px;">
            <n-button type="primary" size="small" @click="openAdmissionModal()">添加规则</n-button>
          </n-space>
          <TableSkeleton v-if="admissionLoading && admissions.length === 0" :rows="3" />
          <EmptyState v-else-if="!admissionLoading && admissions.length === 0" type="rules" action-text="添加规则" @action="openAdmissionModal()" />
          <n-data-table v-else :columns="admissionColumns" :data="admissions" :loading="admissionLoading" :row-key="(row: any) => row.id" size="small" />
        </n-tab-pane>

        <!-- HostMapping 主机映射 -->
        <n-tab-pane name="hosts" tab="主机映射 (Hosts)">
          <n-alert type="info" style="margin-bottom: 16px;">
            自定义 DNS 解析，将域名映射到指定 IP 地址，类似 /etc/hosts 文件。
          </n-alert>
          <n-space justify="end" style="margin-bottom: 12px;">
            <n-button type="primary" size="small" @click="openHostsModal()">添加映射</n-button>
          </n-space>
          <TableSkeleton v-if="hostsLoading && hostMappings.length === 0" :rows="3" />
          <EmptyState v-else-if="!hostsLoading && hostMappings.length === 0" type="rules" action-text="添加映射" @action="openHostsModal()" />
          <n-data-table v-else :columns="hostsColumns" :data="hostMappings" :loading="hostsLoading" :row-key="(row: any) => row.id" size="small" />
        </n-tab-pane>

        <!-- Ingress 反向代理 -->
        <n-tab-pane name="ingress" tab="反向代理 (Ingress)">
          <n-alert type="info" style="margin-bottom: 16px;">
            Ingress 根据域名将请求路由到不同的后端服务（endpoint），实现虚拟主机或反向代理功能。
          </n-alert>
          <n-space justify="end" style="margin-bottom: 12px;">
            <n-button type="primary" size="small" @click="openIngressModal()">添加规则</n-button>
          </n-space>
          <TableSkeleton v-if="ingressLoading && ingresses.length === 0" :rows="3" />
          <EmptyState v-else-if="!ingressLoading && ingresses.length === 0" type="rules" action-text="添加规则" @action="openIngressModal()" />
          <n-data-table v-else :columns="ingressColumns" :data="ingresses" :loading="ingressLoading" :row-key="(row: any) => row.id" size="small" />
        </n-tab-pane>

        <!-- Recorder 流量记录 -->
        <n-tab-pane name="recorder" tab="流量记录 (Recorder)">
          <n-alert type="info" style="margin-bottom: 16px;">
            Recorder 记录代理流量数据，支持输出到文件、Redis 或 HTTP 接口，用于审计和分析。
          </n-alert>
          <n-space justify="end" style="margin-bottom: 12px;">
            <n-button type="primary" size="small" @click="openRecorderModal()">添加记录器</n-button>
          </n-space>
          <TableSkeleton v-if="recorderLoading && recorders.length === 0" :rows="3" />
          <EmptyState v-else-if="!recorderLoading && recorders.length === 0" type="rules" action-text="添加记录器" @action="openRecorderModal()" />
          <n-data-table v-else :columns="recorderColumns" :data="recorders" :loading="recorderLoading" :row-key="(row: any) => row.id" size="small" />
        </n-tab-pane>

        <!-- Router 路由管理 -->
        <n-tab-pane name="router" tab="路由管理 (Router)">
          <n-alert type="info" style="margin-bottom: 16px;">
            Router 根据目标网络地址将流量路由到不同的网关，实现策略路由和分流转发。
          </n-alert>
          <n-space justify="end" style="margin-bottom: 12px;">
            <n-button type="primary" size="small" @click="openRouterModal()">添加路由</n-button>
          </n-space>
          <TableSkeleton v-if="routerLoading && routers.length === 0" :rows="3" />
          <EmptyState v-else-if="!routerLoading && routers.length === 0" type="rules" action-text="添加路由" @action="openRouterModal()" />
          <n-data-table v-else :columns="routerColumns" :data="routers" :loading="routerLoading" :row-key="(row: any) => row.id" size="small" />
        </n-tab-pane>

        <!-- SD 服务发现 -->
        <n-tab-pane name="sd" tab="服务发现 (SD)">
          <n-alert type="info" style="margin-bottom: 16px;">
            SD 从外部注册中心（HTTP/Consul/Etcd/Redis）动态发现服务，用于自动负载均衡和节点管理。
          </n-alert>
          <n-space justify="end" style="margin-bottom: 12px;">
            <n-button type="primary" size="small" @click="openSDModal()">添加服务发现</n-button>
          </n-space>
          <TableSkeleton v-if="sdLoading && sds.length === 0" :rows="3" />
          <EmptyState v-else-if="!sdLoading && sds.length === 0" type="rules" action-text="添加服务发现" @action="openSDModal()" />
          <n-data-table v-else :columns="sdColumns" :data="sds" :loading="sdLoading" :row-key="(row: any) => row.id" size="small" />
        </n-tab-pane>
      </n-tabs>
    </n-card>

    <!-- Bypass Modal -->
    <n-modal v-model:show="showBypassModal" preset="dialog" :title="editingBypass ? '编辑分流规则' : '添加分流规则'" style="width: 600px;">
      <n-form :model="bypassForm" label-placement="left" label-width="100">
        <n-form-item label="名称" required>
          <n-input v-model:value="bypassForm.name" placeholder="例如: 国内直连" />
        </n-form-item>
        <n-form-item label="模式">
          <n-radio-group v-model:value="bypassForm.whitelist">
            <n-radio :value="false">黑名单 (匹配的不走代理)</n-radio>
            <n-radio :value="true">白名单 (仅匹配的走代理)</n-radio>
          </n-radio-group>
        </n-form-item>
        <n-form-item label="关联节点">
          <n-select v-model:value="bypassForm.node_id" :options="nodeOptions" clearable filterable placeholder="全局 (不关联节点)" />
        </n-form-item>
        <n-form-item label="规则列表">
          <n-input v-model:value="bypassForm.matchersText" type="textarea" :rows="8" placeholder="每行一条规则，支持：&#10;*.google.com (域名通配)&#10;.github.com (子域名匹配)&#10;10.0.0.0/8 (IP/CIDR)&#10;192.168.1.1 (精确 IP)" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showBypassModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSaveBypass">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Admission Modal -->
    <n-modal v-model:show="showAdmissionModal" preset="dialog" :title="editingAdmission ? '编辑准入规则' : '添加准入规则'" style="width: 600px;">
      <n-form :model="admissionForm" label-placement="left" label-width="100">
        <n-form-item label="名称" required>
          <n-input v-model:value="admissionForm.name" placeholder="例如: 仅允许内网" />
        </n-form-item>
        <n-form-item label="模式">
          <n-radio-group v-model:value="admissionForm.whitelist">
            <n-radio :value="false">黑名单 (拒绝匹配的 IP)</n-radio>
            <n-radio :value="true">白名单 (仅允许匹配的 IP)</n-radio>
          </n-radio-group>
        </n-form-item>
        <n-form-item label="关联节点">
          <n-select v-model:value="admissionForm.node_id" :options="nodeOptions" clearable filterable placeholder="全局 (不关联节点)" />
        </n-form-item>
        <n-form-item label="IP 列表">
          <n-input v-model:value="admissionForm.matchersText" type="textarea" :rows="8" placeholder="每行一条规则，支持：&#10;192.168.0.0/16 (CIDR)&#10;10.0.0.1 (精确 IP)&#10;172.16.0.0/12 (CIDR)" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showAdmissionModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSaveAdmission">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- HostMapping Modal -->
    <n-modal v-model:show="showHostsModal" preset="dialog" :title="editingHosts ? '编辑主机映射' : '添加主机映射'" style="width: 650px;">
      <n-form :model="hostsForm" label-placement="left" label-width="100">
        <n-form-item label="名称" required>
          <n-input v-model:value="hostsForm.name" placeholder="例如: 自定义DNS" />
        </n-form-item>
        <n-form-item label="关联节点">
          <n-select v-model:value="hostsForm.node_id" :options="nodeOptions" clearable filterable placeholder="全局 (不关联节点)" />
        </n-form-item>
        <n-form-item label="映射规则">
          <n-input v-model:value="hostsForm.mappingsText" type="textarea" :rows="8" placeholder="每行一条: IP 域名 [prefer]&#10;例如:&#10;127.0.0.1 example.com&#10;1.2.3.4 api.example.com ipv4&#10;::1 v6.example.com ipv6" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showHostsModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSaveHosts">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Ingress Modal -->
    <n-modal v-model:show="showIngressModal" preset="dialog" :title="editingIngress ? '编辑反向代理规则' : '添加反向代理规则'" style="width: 650px;">
      <n-form :model="ingressForm" label-placement="left" label-width="100">
        <n-form-item label="名称" required>
          <n-input v-model:value="ingressForm.name" placeholder="例如: Web服务路由" />
        </n-form-item>
        <n-form-item label="关联节点">
          <n-select v-model:value="ingressForm.node_id" :options="nodeOptions" clearable filterable placeholder="全局 (不关联节点)" />
        </n-form-item>
        <n-form-item label="路由规则">
          <n-input v-model:value="ingressForm.rulesText" type="textarea" :rows="8" placeholder="每行一条: 域名 后端地址&#10;例如:&#10;example.com 192.168.1.1:8080&#10;api.example.com 192.168.1.2:3000&#10;*.test.com 10.0.0.1:80" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showIngressModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSaveIngress">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Recorder Modal -->
    <n-modal v-model:show="showRecorderModal" preset="dialog" :title="editingRecorder ? '编辑记录器' : '添加记录器'" style="width: 600px;">
      <n-form :model="recorderForm" label-placement="left" label-width="100">
        <n-form-item label="名称" required>
          <n-input v-model:value="recorderForm.name" placeholder="例如: 流量审计" />
        </n-form-item>
        <n-form-item label="类型">
          <n-select v-model:value="recorderForm.type" :options="recorderTypeOptions" />
        </n-form-item>
        <n-form-item label="关联节点">
          <n-select v-model:value="recorderForm.node_id" :options="nodeOptions" clearable filterable placeholder="全局 (不关联节点)" />
        </n-form-item>
        <n-form-item label="配置">
          <n-input v-model:value="recorderForm.configText" type="textarea" :rows="6" :placeholder="recorderConfigPlaceholder" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showRecorderModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSaveRecorder">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Router Modal -->
    <n-modal v-model:show="showRouterModal" preset="dialog" :title="editingRouter ? '编辑路由' : '添加路由'" style="width: 650px;">
      <n-form :model="routerForm" label-placement="left" label-width="100">
        <n-form-item label="名称" required>
          <n-input v-model:value="routerForm.name" placeholder="例如: 内网路由" />
        </n-form-item>
        <n-form-item label="关联节点">
          <n-select v-model:value="routerForm.node_id" :options="nodeOptions" clearable filterable placeholder="全局 (不关联节点)" />
        </n-form-item>
        <n-form-item label="路由规则">
          <n-input v-model:value="routerForm.routesText" type="textarea" :rows="8" placeholder="每行一条: 目标网段 网关地址&#10;例如:&#10;192.168.0.0/16 192.168.0.1&#10;10.0.0.0/8 10.0.0.1&#10;0.0.0.0/0 172.16.0.1" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showRouterModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSaveRouter">保存</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- SD Modal -->
    <n-modal v-model:show="showSDModal" preset="dialog" :title="editingSD ? '编辑服务发现' : '添加服务发现'" style="width: 600px;">
      <n-form :model="sdForm" label-placement="left" label-width="100">
        <n-form-item label="名称" required>
          <n-input v-model:value="sdForm.name" placeholder="例如: Consul 发现" />
        </n-form-item>
        <n-form-item label="类型">
          <n-select v-model:value="sdForm.type" :options="sdTypeOptions" />
        </n-form-item>
        <n-form-item label="关联节点">
          <n-select v-model:value="sdForm.node_id" :options="nodeOptions" clearable filterable placeholder="全局 (不关联节点)" />
        </n-form-item>
        <n-form-item label="配置">
          <n-input v-model:value="sdForm.configText" type="textarea" :rows="6" :placeholder="sdConfigPlaceholder" />
        </n-form-item>
      </n-form>
      <template #action>
        <n-space>
          <n-button @click="showSDModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="handleSaveSD">保存</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted, computed } from 'vue'
import { NButton, NSpace, NTag, useMessage, useDialog } from 'naive-ui'
import {
  getBypasses, createBypass, updateBypass, deleteBypass, cloneBypass,
  getAdmissions, createAdmission, updateAdmission, deleteAdmission, cloneAdmission,
  getHostMappings, createHostMapping, updateHostMapping, deleteHostMapping, cloneHostMapping,
  getIngresses, createIngress, updateIngress, deleteIngress, cloneIngress,
  getRecorders, createRecorder, updateRecorder, deleteRecorder, cloneRecorder,
  getRouters, createRouter, updateRouter, deleteRouter, cloneRouter,
  getSDs, createSD, updateSD, deleteSD, cloneSD,
  getNodes,
} from '../api'
import EmptyState from '../components/EmptyState.vue'
import TableSkeleton from '../components/TableSkeleton.vue'

const message = useMessage()
const dialog = useDialog()

const activeTab = ref('bypass')
const saving = ref(false)

// Data
const bypasses = ref<any[]>([])
const admissions = ref<any[]>([])
const hostMappings = ref<any[]>([])
const ingresses = ref<any[]>([])
const recorders = ref<any[]>([])
const routers = ref<any[]>([])
const sds = ref<any[]>([])
const allNodes = ref<any[]>([])

// Loading states
const bypassLoading = ref(false)
const admissionLoading = ref(false)
const hostsLoading = ref(false)
const ingressLoading = ref(false)
const recorderLoading = ref(false)
const routerLoading = ref(false)
const sdLoading = ref(false)

// Modal states
const showBypassModal = ref(false)
const showAdmissionModal = ref(false)
const showHostsModal = ref(false)
const showIngressModal = ref(false)
const showRecorderModal = ref(false)
const showRouterModal = ref(false)
const showSDModal = ref(false)
const editingBypass = ref<any>(null)
const editingAdmission = ref<any>(null)
const editingHosts = ref<any>(null)
const editingIngress = ref<any>(null)
const editingRecorder = ref<any>(null)
const editingRouter = ref<any>(null)
const editingSD = ref<any>(null)

// Forms
const bypassForm = ref({ name: '', whitelist: false, node_id: null as number | null, matchersText: '' })
const admissionForm = ref({ name: '', whitelist: false, node_id: null as number | null, matchersText: '' })
const hostsForm = ref({ name: '', node_id: null as number | null, mappingsText: '' })
const ingressForm = ref({ name: '', node_id: null as number | null, rulesText: '' })
const recorderForm = ref({ name: '', type: 'file', node_id: null as number | null, configText: '' })
const routerForm = ref({ name: '', node_id: null as number | null, routesText: '' })
const sdForm = ref({ name: '', type: 'http', node_id: null as number | null, configText: '' })

const nodeOptions = computed(() =>
  allNodes.value.map((n: any) => ({
    label: `${n.name} (${n.host}:${n.port})`,
    value: n.id,
  }))
)

const recorderTypeOptions = [
  { label: '文件 (file)', value: 'file' },
  { label: 'Redis', value: 'redis' },
  { label: 'HTTP', value: 'http' },
]

const recorderConfigPlaceholder = computed(() => {
  switch (recorderForm.value.type) {
    case 'file': return '{"path": "/var/log/gost/traffic.log"}'
    case 'redis': return '{"addr": "127.0.0.1:6379", "db": 0, "key": "gost:recorder"}'
    case 'http': return '{"url": "http://localhost:8080/api/record", "timeout": 5}'
    default: return '{}'
  }
})

const sdTypeOptions = [
  { label: 'HTTP', value: 'http' },
  { label: 'Consul', value: 'consul' },
  { label: 'Etcd', value: 'etcd' },
  { label: 'Redis', value: 'redis' },
]

const sdConfigPlaceholder = computed(() => {
  switch (sdForm.value.type) {
    case 'http': return '{"url": "http://localhost:8080/sd", "timeout": 5}'
    case 'consul': return '{"addr": "127.0.0.1:8500", "token": "", "prefix": "gost"}'
    case 'etcd': return '{"addr": "127.0.0.1:2379", "prefix": "/gost/services"}'
    case 'redis': return '{"addr": "127.0.0.1:6379", "db": 0, "key": "gost:sd"}'
    default: return '{}'
  }
})

// Parse matchers text to JSON array
const parseMatchers = (text: string): string => {
  const lines = text.split('\n').map(l => l.trim()).filter(l => l && !l.startsWith('#'))
  return JSON.stringify(lines)
}

// Parse matchers JSON to text
const matchersToText = (json: string): string => {
  try {
    const arr = JSON.parse(json)
    return Array.isArray(arr) ? arr.join('\n') : ''
  } catch { return '' }
}

// Parse hosts mappings text to JSON
const parseMappings = (text: string): string => {
  const mappings: { hostname: string; ip: string; prefer?: string }[] = []
  const lines = text.split('\n').map(l => l.trim()).filter(l => l && !l.startsWith('#'))
  for (const line of lines) {
    const parts = line.split(/\s+/)
    if (parts.length >= 2) {
      const entry: { hostname: string; ip: string; prefer?: string } = { ip: parts[0]!, hostname: parts[1]! }
      if (parts[2]) entry.prefer = parts[2]
      mappings.push(entry)
    }
  }
  return JSON.stringify(mappings)
}

// Parse mappings JSON to text
const mappingsToText = (json: string): string => {
  try {
    const arr = JSON.parse(json)
    if (!Array.isArray(arr)) return ''
    return arr.map((m: any) => {
      let line = `${m.ip} ${m.hostname}`
      if (m.prefer) line += ` ${m.prefer}`
      return line
    }).join('\n')
  } catch { return '' }
}

// Parse ingress rules text to JSON
const parseIngressRules = (text: string): string => {
  const rules: { hostname: string; endpoint: string }[] = []
  const lines = text.split('\n').map(l => l.trim()).filter(l => l && !l.startsWith('#'))
  for (const line of lines) {
    const parts = line.split(/\s+/)
    if (parts.length >= 2) {
      rules.push({ hostname: parts[0]!, endpoint: parts[1]! })
    }
  }
  return JSON.stringify(rules)
}

// Parse ingress rules JSON to text
const ingressRulesToText = (json: string): string => {
  try {
    const arr = JSON.parse(json)
    if (!Array.isArray(arr)) return ''
    return arr.map((r: any) => `${r.hostname} ${r.endpoint}`).join('\n')
  } catch { return '' }
}

// Parse router routes text to JSON
const parseRouterRoutes = (text: string): string => {
  const routes: { net: string; gateway: string }[] = []
  const lines = text.split('\n').map(l => l.trim()).filter(l => l && !l.startsWith('#'))
  for (const line of lines) {
    const parts = line.split(/\s+/)
    if (parts.length >= 2) {
      routes.push({ net: parts[0]!, gateway: parts[1]! })
    }
  }
  return JSON.stringify(routes)
}

// Parse router routes JSON to text
const routerRoutesToText = (json: string): string => {
  try {
    const arr = JSON.parse(json)
    if (!Array.isArray(arr)) return ''
    return arr.map((r: any) => `${r.net} ${r.gateway}`).join('\n')
  } catch { return '' }
}

// Count display
const countMatchers = (json: string): number => {
  try { return JSON.parse(json)?.length || 0 } catch { return 0 }
}

// ==================== Bypass ====================
const bypassColumns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '名称', key: 'name', width: 150 },
  {
    title: '模式', key: 'whitelist', width: 100,
    render: (row: any) => h(NTag, { type: row.whitelist ? 'success' : 'warning', size: 'small' }, () => row.whitelist ? '白名单' : '黑名单'),
  },
  {
    title: '规则数', key: 'matchers', width: 80,
    render: (row: any) => countMatchers(row.matchers),
  },
  {
    title: '关联节点', key: 'node_id', width: 120,
    render: (row: any) => {
      if (!row.node_id) return h(NTag, { size: 'small' }, () => '全局')
      const node = allNodes.value.find((n: any) => n.id === row.node_id)
      return node ? node.name : `#${row.node_id}`
    },
  },
  {
    title: '操作', key: 'actions', width: 150,
    render: (row: any) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', onClick: () => openBypassModal(row) }, () => '编辑'),
      h(NButton, { size: 'small', onClick: () => handleCloneBypass(row) }, () => '克隆'),
      h(NButton, { size: 'small', type: 'error', onClick: () => handleDeleteBypass(row) }, () => '删除'),
    ]),
  },
]

const loadBypasses = async () => {
  bypassLoading.value = true
  try {
    const data: any = await getBypasses()
    bypasses.value = data || []
  } catch { message.error('加载分流规则失败') }
  finally { bypassLoading.value = false }
}

const openBypassModal = (row?: any) => {
  if (row) {
    editingBypass.value = row
    bypassForm.value = { name: row.name, whitelist: row.whitelist, node_id: row.node_id || null, matchersText: matchersToText(row.matchers) }
  } else {
    editingBypass.value = null
    bypassForm.value = { name: '', whitelist: false, node_id: null, matchersText: '' }
  }
  showBypassModal.value = true
}

const handleSaveBypass = async () => {
  if (!bypassForm.value.name) { message.error('请输入名称'); return }
  saving.value = true
  try {
    const data = {
      name: bypassForm.value.name,
      whitelist: bypassForm.value.whitelist,
      node_id: bypassForm.value.node_id || undefined,
      matchers: parseMatchers(bypassForm.value.matchersText),
    }
    if (editingBypass.value) {
      await updateBypass(editingBypass.value.id, data)
      message.success('规则已更新')
    } else {
      await createBypass(data)
      message.success('规则已创建')
    }
    showBypassModal.value = false
    loadBypasses()
  } catch (e: any) { message.error(e.response?.data?.error || '保存失败') }
  finally { saving.value = false }
}

const handleDeleteBypass = (row: any) => {
  dialog.warning({
    title: '删除分流规则',
    content: `确定要删除 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try { await deleteBypass(row.id); message.success('已删除'); loadBypasses() }
      catch { message.error('删除失败') }
    },
  })
}

const handleCloneBypass = async (row: any) => {
  try {
    await cloneBypass(row.id)
    message.success('克隆成功')
    loadBypasses()
  } catch (e: any) {
    message.error(e.response?.data?.error || '克隆失败')
  }
}

// ==================== Admission ====================
const admissionColumns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '名称', key: 'name', width: 150 },
  {
    title: '模式', key: 'whitelist', width: 100,
    render: (row: any) => h(NTag, { type: row.whitelist ? 'success' : 'warning', size: 'small' }, () => row.whitelist ? '白名单' : '黑名单'),
  },
  {
    title: '规则数', key: 'matchers', width: 80,
    render: (row: any) => countMatchers(row.matchers),
  },
  {
    title: '关联节点', key: 'node_id', width: 120,
    render: (row: any) => {
      if (!row.node_id) return h(NTag, { size: 'small' }, () => '全局')
      const node = allNodes.value.find((n: any) => n.id === row.node_id)
      return node ? node.name : `#${row.node_id}`
    },
  },
  {
    title: '操作', key: 'actions', width: 150,
    render: (row: any) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', onClick: () => openAdmissionModal(row) }, () => '编辑'),
      h(NButton, { size: 'small', onClick: () => handleCloneAdmission(row) }, () => '克隆'),
      h(NButton, { size: 'small', type: 'error', onClick: () => handleDeleteAdmission(row) }, () => '删除'),
    ]),
  },
]

const loadAdmissions = async () => {
  admissionLoading.value = true
  try {
    const data: any = await getAdmissions()
    admissions.value = data || []
  } catch { message.error('加载准入规则失败') }
  finally { admissionLoading.value = false }
}

const openAdmissionModal = (row?: any) => {
  if (row) {
    editingAdmission.value = row
    admissionForm.value = { name: row.name, whitelist: row.whitelist, node_id: row.node_id || null, matchersText: matchersToText(row.matchers) }
  } else {
    editingAdmission.value = null
    admissionForm.value = { name: '', whitelist: false, node_id: null, matchersText: '' }
  }
  showAdmissionModal.value = true
}

const handleSaveAdmission = async () => {
  if (!admissionForm.value.name) { message.error('请输入名称'); return }
  saving.value = true
  try {
    const data = {
      name: admissionForm.value.name,
      whitelist: admissionForm.value.whitelist,
      node_id: admissionForm.value.node_id || undefined,
      matchers: parseMatchers(admissionForm.value.matchersText),
    }
    if (editingAdmission.value) {
      await updateAdmission(editingAdmission.value.id, data)
      message.success('规则已更新')
    } else {
      await createAdmission(data)
      message.success('规则已创建')
    }
    showAdmissionModal.value = false
    loadAdmissions()
  } catch (e: any) { message.error(e.response?.data?.error || '保存失败') }
  finally { saving.value = false }
}

const handleDeleteAdmission = (row: any) => {
  dialog.warning({
    title: '删除准入规则',
    content: `确定要删除 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try { await deleteAdmission(row.id); message.success('已删除'); loadAdmissions() }
      catch { message.error('删除失败') }
    },
  })
}

const handleCloneAdmission = async (row: any) => {
  try {
    await cloneAdmission(row.id)
    message.success('克隆成功')
    loadAdmissions()
  } catch (e: any) {
    message.error(e.response?.data?.error || '克隆失败')
  }
}

// ==================== HostMapping ====================
const hostsColumns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '名称', key: 'name', width: 150 },
  {
    title: '映射数', key: 'mappings', width: 80,
    render: (row: any) => countMatchers(row.mappings),
  },
  {
    title: '关联节点', key: 'node_id', width: 120,
    render: (row: any) => {
      if (!row.node_id) return h(NTag, { size: 'small' }, () => '全局')
      const node = allNodes.value.find((n: any) => n.id === row.node_id)
      return node ? node.name : `#${row.node_id}`
    },
  },
  {
    title: '操作', key: 'actions', width: 150,
    render: (row: any) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', onClick: () => openHostsModal(row) }, () => '编辑'),
      h(NButton, { size: 'small', onClick: () => handleCloneHostMapping(row) }, () => '克隆'),
      h(NButton, { size: 'small', type: 'error', onClick: () => handleDeleteHosts(row) }, () => '删除'),
    ]),
  },
]

const loadHostMappings = async () => {
  hostsLoading.value = true
  try {
    const data: any = await getHostMappings()
    hostMappings.value = data || []
  } catch { message.error('加载主机映射失败') }
  finally { hostsLoading.value = false }
}

const openHostsModal = (row?: any) => {
  if (row) {
    editingHosts.value = row
    hostsForm.value = { name: row.name, node_id: row.node_id || null, mappingsText: mappingsToText(row.mappings) }
  } else {
    editingHosts.value = null
    hostsForm.value = { name: '', node_id: null, mappingsText: '' }
  }
  showHostsModal.value = true
}

const handleSaveHosts = async () => {
  if (!hostsForm.value.name) { message.error('请输入名称'); return }
  saving.value = true
  try {
    const data = {
      name: hostsForm.value.name,
      node_id: hostsForm.value.node_id || undefined,
      mappings: parseMappings(hostsForm.value.mappingsText),
    }
    if (editingHosts.value) {
      await updateHostMapping(editingHosts.value.id, data)
      message.success('映射已更新')
    } else {
      await createHostMapping(data)
      message.success('映射已创建')
    }
    showHostsModal.value = false
    loadHostMappings()
  } catch (e: any) { message.error(e.response?.data?.error || '保存失败') }
  finally { saving.value = false }
}

const handleDeleteHosts = (row: any) => {
  dialog.warning({
    title: '删除主机映射',
    content: `确定要删除 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try { await deleteHostMapping(row.id); message.success('已删除'); loadHostMappings() }
      catch { message.error('删除失败') }
    },
  })
}

const handleCloneHostMapping = async (row: any) => {
  try {
    await cloneHostMapping(row.id)
    message.success('克隆成功')
    loadHostMappings()
  } catch (e: any) {
    message.error(e.response?.data?.error || '克隆失败')
  }
}

// ==================== Ingress ====================
const ingressColumns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '名称', key: 'name', width: 150 },
  {
    title: '规则数', key: 'rules', width: 80,
    render: (row: any) => countMatchers(row.rules),
  },
  {
    title: '关联节点', key: 'node_id', width: 120,
    render: (row: any) => {
      if (!row.node_id) return h(NTag, { size: 'small' }, () => '全局')
      const node = allNodes.value.find((n: any) => n.id === row.node_id)
      return node ? node.name : `#${row.node_id}`
    },
  },
  {
    title: '操作', key: 'actions', width: 150,
    render: (row: any) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', onClick: () => openIngressModal(row) }, () => '编辑'),
      h(NButton, { size: 'small', onClick: () => handleCloneIngress(row) }, () => '克隆'),
      h(NButton, { size: 'small', type: 'error', onClick: () => handleDeleteIngress(row) }, () => '删除'),
    ]),
  },
]

const loadIngresses = async () => {
  ingressLoading.value = true
  try {
    const data: any = await getIngresses()
    ingresses.value = data || []
  } catch { message.error('加载反向代理规则失败') }
  finally { ingressLoading.value = false }
}

const openIngressModal = (row?: any) => {
  if (row) {
    editingIngress.value = row
    ingressForm.value = { name: row.name, node_id: row.node_id || null, rulesText: ingressRulesToText(row.rules) }
  } else {
    editingIngress.value = null
    ingressForm.value = { name: '', node_id: null, rulesText: '' }
  }
  showIngressModal.value = true
}

const handleSaveIngress = async () => {
  if (!ingressForm.value.name) { message.error('请输入名称'); return }
  saving.value = true
  try {
    const data = {
      name: ingressForm.value.name,
      node_id: ingressForm.value.node_id || undefined,
      rules: parseIngressRules(ingressForm.value.rulesText),
    }
    if (editingIngress.value) {
      await updateIngress(editingIngress.value.id, data)
      message.success('规则已更新')
    } else {
      await createIngress(data)
      message.success('规则已创建')
    }
    showIngressModal.value = false
    loadIngresses()
  } catch (e: any) { message.error(e.response?.data?.error || '保存失败') }
  finally { saving.value = false }
}

const handleDeleteIngress = (row: any) => {
  dialog.warning({
    title: '删除反向代理规则',
    content: `确定要删除 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try { await deleteIngress(row.id); message.success('已删除'); loadIngresses() }
      catch { message.error('删除失败') }
    },
  })
}

const handleCloneIngress = async (row: any) => {
  try {
    await cloneIngress(row.id)
    message.success('克隆成功')
    loadIngresses()
  } catch (e: any) {
    message.error(e.response?.data?.error || '克隆失败')
  }
}

// ==================== Recorder ====================
const recorderColumns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '名称', key: 'name', width: 150 },
  {
    title: '类型', key: 'type', width: 100,
    render: (row: any) => {
      const typeMap: Record<string, string> = { file: '文件', redis: 'Redis', http: 'HTTP' }
      return h(NTag, { size: 'small' }, () => typeMap[row.type] || row.type)
    },
  },
  {
    title: '关联节点', key: 'node_id', width: 120,
    render: (row: any) => {
      if (!row.node_id) return h(NTag, { size: 'small' }, () => '全局')
      const node = allNodes.value.find((n: any) => n.id === row.node_id)
      return node ? node.name : `#${row.node_id}`
    },
  },
  {
    title: '操作', key: 'actions', width: 150,
    render: (row: any) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', onClick: () => openRecorderModal(row) }, () => '编辑'),
      h(NButton, { size: 'small', onClick: () => handleCloneRecorder(row) }, () => '克隆'),
      h(NButton, { size: 'small', type: 'error', onClick: () => handleDeleteRecorder(row) }, () => '删除'),
    ]),
  },
]

const loadRecorders = async () => {
  recorderLoading.value = true
  try {
    const data: any = await getRecorders()
    recorders.value = data || []
  } catch { message.error('加载记录器失败') }
  finally { recorderLoading.value = false }
}

const openRecorderModal = (row?: any) => {
  if (row) {
    editingRecorder.value = row
    recorderForm.value = { name: row.name, type: row.type || 'file', node_id: row.node_id || null, configText: row.config || '' }
  } else {
    editingRecorder.value = null
    recorderForm.value = { name: '', type: 'file', node_id: null, configText: '' }
  }
  showRecorderModal.value = true
}

const handleSaveRecorder = async () => {
  if (!recorderForm.value.name) { message.error('请输入名称'); return }
  saving.value = true
  try {
    const data = {
      name: recorderForm.value.name,
      type: recorderForm.value.type,
      node_id: recorderForm.value.node_id || undefined,
      config: recorderForm.value.configText,
    }
    if (editingRecorder.value) {
      await updateRecorder(editingRecorder.value.id, data)
      message.success('记录器已更新')
    } else {
      await createRecorder(data)
      message.success('记录器已创建')
    }
    showRecorderModal.value = false
    loadRecorders()
  } catch (e: any) { message.error(e.response?.data?.error || '保存失败') }
  finally { saving.value = false }
}

const handleDeleteRecorder = (row: any) => {
  dialog.warning({
    title: '删除记录器',
    content: `确定要删除 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try { await deleteRecorder(row.id); message.success('已删除'); loadRecorders() }
      catch { message.error('删除失败') }
    },
  })
}

const handleCloneRecorder = async (row: any) => {
  try {
    await cloneRecorder(row.id)
    message.success('克隆成功')
    loadRecorders()
  } catch (e: any) {
    message.error(e.response?.data?.error || '克隆失败')
  }
}

// ==================== Router ====================
const routerColumns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '名称', key: 'name', width: 150 },
  {
    title: '路由数', key: 'routes', width: 80,
    render: (row: any) => countMatchers(row.routes),
  },
  {
    title: '关联节点', key: 'node_id', width: 120,
    render: (row: any) => {
      if (!row.node_id) return h(NTag, { size: 'small' }, () => '全局')
      const node = allNodes.value.find((n: any) => n.id === row.node_id)
      return node ? node.name : `#${row.node_id}`
    },
  },
  {
    title: '操作', key: 'actions', width: 150,
    render: (row: any) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', onClick: () => openRouterModal(row) }, () => '编辑'),
      h(NButton, { size: 'small', onClick: () => handleCloneRouter(row) }, () => '克隆'),
      h(NButton, { size: 'small', type: 'error', onClick: () => handleDeleteRouter(row) }, () => '删除'),
    ]),
  },
]

const loadRouters = async () => {
  routerLoading.value = true
  try {
    const data: any = await getRouters()
    routers.value = data || []
  } catch { message.error('加载路由失败') }
  finally { routerLoading.value = false }
}

const openRouterModal = (row?: any) => {
  if (row) {
    editingRouter.value = row
    routerForm.value = { name: row.name, node_id: row.node_id || null, routesText: routerRoutesToText(row.routes) }
  } else {
    editingRouter.value = null
    routerForm.value = { name: '', node_id: null, routesText: '' }
  }
  showRouterModal.value = true
}

const handleSaveRouter = async () => {
  if (!routerForm.value.name) { message.error('请输入名称'); return }
  saving.value = true
  try {
    const data = {
      name: routerForm.value.name,
      node_id: routerForm.value.node_id || undefined,
      routes: parseRouterRoutes(routerForm.value.routesText),
    }
    if (editingRouter.value) {
      await updateRouter(editingRouter.value.id, data)
      message.success('路由已更新')
    } else {
      await createRouter(data)
      message.success('路由已创建')
    }
    showRouterModal.value = false
    loadRouters()
  } catch (e: any) { message.error(e.response?.data?.error || '保存失败') }
  finally { saving.value = false }
}

const handleDeleteRouter = (row: any) => {
  dialog.warning({
    title: '删除路由',
    content: `确定要删除 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try { await deleteRouter(row.id); message.success('已删除'); loadRouters() }
      catch { message.error('删除失败') }
    },
  })
}

const handleCloneRouter = async (row: any) => {
  try {
    await cloneRouter(row.id)
    message.success('克隆成功')
    loadRouters()
  } catch (e: any) {
    message.error(e.response?.data?.error || '克隆失败')
  }
}

// ==================== SD ====================
const sdColumns = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '名称', key: 'name', width: 150 },
  {
    title: '类型', key: 'type', width: 100,
    render: (row: any) => {
      const typeMap: Record<string, string> = { http: 'HTTP', consul: 'Consul', etcd: 'Etcd', redis: 'Redis' }
      return h(NTag, { size: 'small' }, () => typeMap[row.type] || row.type)
    },
  },
  {
    title: '关联节点', key: 'node_id', width: 120,
    render: (row: any) => {
      if (!row.node_id) return h(NTag, { size: 'small' }, () => '全局')
      const node = allNodes.value.find((n: any) => n.id === row.node_id)
      return node ? node.name : `#${row.node_id}`
    },
  },
  {
    title: '操作', key: 'actions', width: 150,
    render: (row: any) => h(NSpace, { size: 'small' }, () => [
      h(NButton, { size: 'small', onClick: () => openSDModal(row) }, () => '编辑'),
      h(NButton, { size: 'small', onClick: () => handleCloneSD(row) }, () => '克隆'),
      h(NButton, { size: 'small', type: 'error', onClick: () => handleDeleteSD(row) }, () => '删除'),
    ]),
  },
]

const loadSDs = async () => {
  sdLoading.value = true
  try {
    const data: any = await getSDs()
    sds.value = data || []
  } catch { message.error('加载服务发现失败') }
  finally { sdLoading.value = false }
}

const openSDModal = (row?: any) => {
  if (row) {
    editingSD.value = row
    sdForm.value = { name: row.name, type: row.type || 'http', node_id: row.node_id || null, configText: row.config || '' }
  } else {
    editingSD.value = null
    sdForm.value = { name: '', type: 'http', node_id: null, configText: '' }
  }
  showSDModal.value = true
}

const handleSaveSD = async () => {
  if (!sdForm.value.name) { message.error('请输入名称'); return }
  saving.value = true
  try {
    const data = {
      name: sdForm.value.name,
      type: sdForm.value.type,
      node_id: sdForm.value.node_id || undefined,
      config: sdForm.value.configText,
    }
    if (editingSD.value) {
      await updateSD(editingSD.value.id, data)
      message.success('服务发现已更新')
    } else {
      await createSD(data)
      message.success('服务发现已创建')
    }
    showSDModal.value = false
    loadSDs()
  } catch (e: any) { message.error(e.response?.data?.error || '保存失败') }
  finally { saving.value = false }
}

const handleDeleteSD = (row: any) => {
  dialog.warning({
    title: '删除服务发现',
    content: `确定要删除 "${row.name}" 吗？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try { await deleteSD(row.id); message.success('已删除'); loadSDs() }
      catch { message.error('删除失败') }
    },
  })
}

const handleCloneSD = async (row: any) => {
  try {
    await cloneSD(row.id)
    message.success('克隆成功')
    loadSDs()
  } catch (e: any) {
    message.error(e.response?.data?.error || '克隆失败')
  }
}

// Load all nodes for selector
const loadNodes = async () => {
  try {
    const data: any = await getNodes()
    allNodes.value = data || []
  } catch { /* silent */ }
}

onMounted(() => {
  loadNodes()
  loadBypasses()
  loadAdmissions()
  loadHostMappings()
  loadIngresses()
  loadRecorders()
  loadRouters()
  loadSDs()
})
</script>

<style scoped>
</style>
