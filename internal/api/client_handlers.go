// 客户端管理模块
package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/supernaga/gost-panel/internal/service"
	"github.com/gin-gonic/gin"
)



// ==================== 客户端管理 ====================

func (s *Server) listClients(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	clients, err := s.svc.ListClientsByOwner(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, clients)
}

func (s *Server) listClientsPaginated(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("search")
	sortBy := c.Query("sort_by")
	sortDesc := c.Query("sort_desc") == "true"

	params := service.NewPaginationParams(page, pageSize, search)
	params.SortBy = sortBy
	params.SortDesc = sortDesc

	result, err := s.svc.ListClientsPaginated(userID, isAdmin, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) getClient(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	client, err := s.svc.GetClientByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}
	c.JSON(http.StatusOK, client)
}

type CreateClientRequest struct {
	Name          string `json:"name" binding:"required"`
	NodeID        uint   `json:"node_id" binding:"required"`
	LocalPort     int    `json:"local_port"`
	RemotePort    int    `json:"remote_port" binding:"required"`
	ProxyUser     string `json:"proxy_user"`
	ProxyPass     string `json:"proxy_pass"`
	TrafficQuota  int64  `json:"traffic_quota"`   // 流量配额 (bytes)
	QuotaResetDay int    `json:"quota_reset_day"` // 每月重置日
}

func (s *Server) createClient(c *gin.Context) {
	var req CreateClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "client")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}

		// 检查节点访问权限
		allowed, msg = s.svc.CheckPlanNodeAccess(userID, req.NodeID)
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}

		if _, err := s.svc.GetNodeByOwner(req.NodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	client := &model.Client{
		Name:          req.Name,
		NodeID:        req.NodeID,
		LocalPort:     req.LocalPort,
		RemotePort:    req.RemotePort,
		ProxyUser:     req.ProxyUser,
		ProxyPass:     req.ProxyPass,
		TrafficQuota:  req.TrafficQuota,
		QuotaResetDay: req.QuotaResetDay,
		OwnerID:       &userID,
	}

	if client.LocalPort == 0 {
		client.LocalPort = 38777
	}
	if client.QuotaResetDay == 0 {
		client.QuotaResetDay = 1
	}

	if err := s.svc.CreateClient(client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, client)
}

func (s *Server) updateClient(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetClientByOwner(id, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	delete(updates, "id")
	delete(updates, "token")
	delete(updates, "created_at")
	delete(updates, "owner_id")
	if !isAdmin {
		if rawNodeID, exists := updates["node_id"]; exists {
			if rawNodeID == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
				return
			}
			nodeID, err := parseUintField(rawNodeID, "node_id")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			allowed, msg := s.svc.CheckPlanNodeAccess(userID, nodeID)
			if !allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": msg})
				return
			}
			if _, err := s.svc.GetNodeByOwner(nodeID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
	}

	// 密码字段为空时不更新，防止编辑时误覆盖已有密码
	if v, ok := updates["proxy_pass"]; ok && v == "" {
		delete(updates, "proxy_pass")
	}

	if err := s.svc.UpdateClient(id, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteClient(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetClientByOwner(id, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := s.svc.DeleteClient(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) getClientInstallScript(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	osType := c.DefaultQuery("os", "linux")
	userID, isAdmin := getUserInfo(c)

	client, err := s.svc.GetClientByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	// 获取正确的 Panel URL（支持反向代理）
	panelURL := s.getPanelURL(c)
	githubRaw := s.cfg.GitHubRawURL

	var script, oneLineCommand string

	if osType == "windows" {
		// PowerShell 安装命令
		oneLineCommand = fmt.Sprintf(`irm "%s/install-client.ps1" -OutFile "$env:TEMP\install-client.ps1"; & "$env:TEMP\install-client.ps1" -PanelUrl "%s" -Token "%s"`, githubRaw, panelURL, client.Token)
		script = fmt.Sprintf(`# GOST Panel Client Installation for Windows
# Run in PowerShell as Administrator

# Client: %s
# Local SOCKS5 Port: %d (credentials configured in panel)

# One-line install (copy and run this command):
%s
`, client.Name, client.LocalPort, oneLineCommand)
	} else {
		// Bash 安装命令
		oneLineCommand = fmt.Sprintf(`(curl -fsSL "%s/install-client.sh" 2>/dev/null || wget -qO- "%s/install-client.sh") | bash -s -- -p "%s" -t "%s"`, githubRaw, githubRaw, panelURL, client.Token)
		script = fmt.Sprintf(`#!/bin/bash
# GOST Panel Client Installation (Agent Mode)
# Supported: Linux (amd64, arm64, armv7, armv6, mips, mipsle)

# Client: %s
# Local SOCKS5 Port: %d (credentials configured in panel)

# One-line install (copy and run this command):
%s

# Or with forced architecture:
# (curl -fsSL "%s/install-client.sh" 2>/dev/null || wget -qO- "%s/install-client.sh") | bash -s -- -p "%s" -t "%s" -a armv6
`, client.Name, client.LocalPort, oneLineCommand, githubRaw, githubRaw, panelURL, client.Token)
	}

	c.JSON(http.StatusOK, gin.H{
		"script":           script,
		"one_line_command": oneLineCommand,
		"token":            client.Token,
	})
}

func (s *Server) getClientGostConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	client, err := s.svc.GetClientByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	config := s.generateClientConfig(client)
	c.YAML(http.StatusOK, config)
}

func (s *Server) getClientProxyURI(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	client, err := s.svc.GetClientByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	// 生成通过节点访问的代理 URI
	// 格式: socks5://user:pass@节点地址:远程端口
	if client.Node == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "客户端未关联节点"})
		return
	}
	node := client.Node
	uri := fmt.Sprintf("socks5://%s:%s@%s:%d",
		client.ProxyUser, client.ProxyPass, node.Host, client.RemotePort)

	c.JSON(http.StatusOK, gin.H{"uri": uri})
}

func (s *Server) generateClientConfig(client *model.Client) map[string]interface{} {
	node := client.Node
	chainName := "forward-chain"

	config := map[string]interface{}{
		"services": []map[string]interface{}{
			// 本地 SOCKS5
			{
				"name": "local-socks5",
				"addr": fmt.Sprintf(":%d", client.LocalPort),
				"handler": map[string]interface{}{
					"type":   "socks5",
					"auther": "local-auth",
					"metadata": map[string]interface{}{
						"udp":         true,
						"udpAddr":     fmt.Sprintf(":%d", client.LocalPort),
						"ignoreChain": true,
					},
				},
				"listener": map[string]interface{}{
					"type": "tcp",
				},
			},
			// RTCP 反向隧道
			{
				"name": "rtcp-tunnel",
				"addr": fmt.Sprintf(":%d", client.RemotePort),
				"handler": map[string]interface{}{
					"type": "rtcp",
				},
				"listener": map[string]interface{}{
					"type":  "rtcp",
					"chain": chainName,
					"metadata": map[string]interface{}{
						"keepalive": true,
					},
				},
				"forwarder": map[string]interface{}{
					"nodes": []map[string]interface{}{
						{"name": "target", "addr": fmt.Sprintf("127.0.0.1:%d", client.LocalPort)},
					},
				},
			},
			// RUDP 反向隧道
			{
				"name": "rudp-tunnel",
				"addr": fmt.Sprintf(":%d", client.RemotePort),
				"handler": map[string]interface{}{
					"type": "rudp",
				},
				"listener": map[string]interface{}{
					"type":  "rudp",
					"chain": chainName,
					"metadata": map[string]interface{}{
						"keepalive": true,
					},
				},
				"forwarder": map[string]interface{}{
					"nodes": []map[string]interface{}{
						{"name": "target", "addr": fmt.Sprintf("127.0.0.1:%d", client.LocalPort)},
					},
				},
			},
		},
		"chains": []map[string]interface{}{
			{
				"name": chainName,
				"hops": []map[string]interface{}{
					{
						"name": "hop-0",
						"nodes": []map[string]interface{}{
							{
								"name": "node-0",
								// 连接到节点的 SOCKS5 服务 (使用 bind 功能建立反向隧道)
								"addr": fmt.Sprintf("%s:%d", node.Host, node.Port),
								"connector": func() map[string]interface{} {
									c := map[string]interface{}{
										"type": "socks5",
									}
									// 如果节点有代理认证，添加认证信息
									if node.ProxyUser != "" {
										c["auth"] = map[string]string{
											"username": node.ProxyUser,
											"password": node.ProxyPass,
										}
									}
									return c
								}(),
								"dialer": map[string]interface{}{
									"type": "tcp",
									"metadata": map[string]interface{}{
										"keepAlive":       true,
										"keepAlivePeriod": "15s",
									},
								},
							},
						},
					},
				},
			},
		},
		"authers": []map[string]interface{}{
			{
				"name": "local-auth",
				"auths": []map[string]string{
					{"username": client.ProxyUser, "password": client.ProxyPass},
				},
			},
		},
	}

	return config
}

