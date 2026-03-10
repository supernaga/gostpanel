# GOST Panel 代码重构计划

## 重构目标
将 5547 行的 handlers.go 拆分为多个可维护的文件

## 文件拆分方案

### 1. 核心辅助文件
- ✅ `helpers.go` - 已创建，包含通用辅助函数
- ✅ `errors.go` - 已优化，统一错误处理

### 2. 业务处理文件（按功能模块拆分）

#### 节点相关 (~800 行)
- `node_handlers.go` - 节点 CRUD 操作
  - listNodes, getNode, createNode, updateNode, deleteNode
  - listNodesPaginated
  - applyNodeConfig, syncNodeConfig
  - getNodeGostConfig, getNodeInstallScript, getNodeProxyURI
  - pingNode, pingAllNodes, getNodeHealthLogs

- `node_batch_handlers.go` - 节点批量操作
  - batchDeleteNodes, batchSyncNodes
  - batchEnableNodes, batchDisableNodes

- `node_tag_handlers.go` - 节点标签管理
  - getNodeTags, addNodeTag, setNodeTags, removeNodeTag

#### 客户端相关 (~400 行)
- `client_handlers.go` - 客户端管理
  - listClients, getClient, createClient, updateClient, deleteClient
  - listClientsPaginated
  - getClientInstallScript, getClientGostConfig, getClientProxyURI

- `client_batch_handlers.go` - 客户端批量操作
  - batchDeleteClients, batchEnableClients, batchDisableClients, batchSyncClients

#### Agent 相关 (~200 行)
- `agent_handlers.go` - Agent 接口
  - agentRegister, agentHeartbeat
  - agentGetConfig, agentGetVersion
  - agentCheckUpdate, agentDownload

#### 用户相关 (~600 行)
- `user_handlers.go` - 用户管理
  - listUsers, getUser, createUser, updateUser, deleteUser
  - changePassword

- `profile_handlers.go` - 个人资料
  - getProfile, updateProfile

- `auth_handlers.go` - 认证相关（已存在 2fa_handlers.go）
  - enable2FA, verify2FA, disable2FA

#### 隧道和转发 (~600 行)
- `tunnel_handlers.go` - 隧道管理
  - listTunnels, getTunnel, createTunnel, updateTunnel, deleteTunnel
  - syncTunnel, getTunnelEntryConfig, getTunnelExitConfig

- `port_forward_handlers.go` - 端口转发
  - listPortForwards, getPortForward, createPortForward
  - updatePortForward, deletePortForward

- `proxy_chain_handlers.go` - 代理链
  - listProxyChains, getProxyChain, createProxyChain
  - updateProxyChain, deleteProxyChain
  - listProxyChainHops, addProxyChainHop, updateProxyChainHop

#### 高级功能 (~800 行)
- `node_group_handlers.go` - 节点组（负载均衡）
  - listNodeGroups, getNodeGroup, createNodeGroup
  - updateNodeGroup, deleteNodeGroup
  - listNodeGroupMembers, addNodeGroupMember, removeNodeGroupMember

- `bypass_handlers.go` - 分流规则
  - listBypasses, getBypass, createBypass, updateBypass, deleteBypass

- `admission_handlers.go` - 准入控制
  - listAdmissions, getAdmission, createAdmission, updateAdmission, deleteAdmission

- `host_mapping_handlers.go` - 主机映射
  - listHostMappings, getHostMapping, createHostMapping
  - updateHostMapping, deleteHostMapping

- `ingress_handlers.go` - 反向代理
  - listIngresses, getIngress, createIngress, updateIngress, deleteIngress

- `recorder_handlers.go` - 流量记录
  - listRecorders, getRecorder, createRecorder, updateRecorder, deleteRecorder

- `router_handlers.go` - 路由管理
  - listRouters, getRouter, createRouter, updateRouter, deleteRouter

- `sd_handlers.go` - 服务发现
  - listSDs, getSD, createSD, updateSD, deleteSD

#### 系统管理 (~500 行)
- `system_handlers.go` - 系统配置和管理
  - getSiteConfigs, updateSiteConfigs
  - getStats, healthCheck
  - backupDatabase, restoreDatabase
  - exportData, importData

- `notification_handlers.go` - 通知管理
  - listNotifyChannels, getNotifyChannel, createNotifyChannel
  - updateNotifyChannel, deleteNotifyChannel, testNotifyChannel

- `alert_handlers.go` - 告警管理
  - listAlertRules, getAlertRule, createAlertRule
  - updateAlertRule, deleteAlertRule
  - getAlertLogs

- `operation_log_handlers.go` - 操作日志
  - getOperationLogs

- `traffic_handlers.go` - 流量历史
  - getTrafficHistory

#### 套餐管理 (~300 行)
- `plan_handlers.go` - 套餐管理
  - listPlans, getPlan, createPlan, updatePlan, deletePlan
  - getPlanResources, setPlanResources
  - assignUserPlan, removeUserPlan, renewUserPlan

#### 其他 (~200 行)
- `template_handlers.go` - 模板管理
  - listTemplates, getTemplate, getTemplateCategories
  - listClientTemplates, getClientTemplate, getClientTemplateCategories

- `tag_handlers.go` - 标签管理
  - listTags, getTag, createTag, updateTag, deleteTag
  - getNodesByTag

- `config_version_handlers.go` - 配置版本
  - getConfigVersions, createConfigVersion, getConfigVersion
  - restoreConfigVersion, deleteConfigVersion

- `clone_handlers.go` - 克隆操作（通用）
  - cloneNode, cloneClient, clonePortForward, cloneTunnel
  - cloneProxyChain, cloneNodeGroup, cloneBypass, etc.

- `search_handlers.go` - 全局搜索
  - globalSearch

## 重构步骤

### 阶段 1: 准备工作 ✅
1. ✅ 创建 helpers.go - 提取通用辅助函数
2. ✅ 优化 errors.go - 统一错误处理

### 阶段 2: 核心模块拆分（优先级高）
3. 创建 node_handlers.go - 节点管理（最常用）
4. 创建 client_handlers.go - 客户端管理
5. 创建 user_handlers.go - 用户管理
6. 创建 tunnel_handlers.go - 隧道管理

### 阶段 3: 扩展模块拆分
7. 创建 agent_handlers.go - Agent 接口
8. 创建 port_forward_handlers.go - 端口转发
9. 创建 proxy_chain_handlers.go - 代理链
10. 创建 system_handlers.go - 系统管理

### 阶段 4: 高级功能拆分
11. 创建各种高级功能的 handlers 文件
12. 创建批量操作和克隆操作的独立文件

### 阶段 5: 清理和测试
13. 删除原 handlers.go 文件
14. 运行测试确保功能正常
15. 更新文档

## 注意事项

1. **保持向后兼容**: 所有函数签名保持不变
2. **统一错误处理**: 使用新的错误处理函数
3. **添加注释**: 为每个文件添加清晰的包级注释
4. **测试**: 每拆分一个模块就测试一次

## 预期效果

- 原文件: 5547 行
- 拆分后: 约 25-30 个文件，每个文件 100-300 行
- 可维护性: 大幅提升
- 代码审查: 更容易定位问题
- 团队协作: 减少合并冲突

## 下一步行动

建议按以下顺序执行：
1. 先拆分最常用的模块（节点、客户端、用户）
2. 测试基本功能是否正常
3. 继续拆分其他模块
4. 最后清理和优化
