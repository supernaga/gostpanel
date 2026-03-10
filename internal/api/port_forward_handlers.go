// 端口转发模块
package api

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== 端口转发 ====================

// portForwardToResponse 转换 PortForward 为前端兼容的响应格式
func portForwardToResponse(pf model.PortForward, nodeName string) gin.H {
	listenHost := "0.0.0.0"
	listenPort := 0
	if pf.LocalAddr != "" {
		if h, p, err := net.SplitHostPort(pf.LocalAddr); err == nil {
			if h != "" {
				listenHost = h
			}
			listenPort, _ = strconv.Atoi(p)
		}
	}

	targetHost := ""
	targetPort := 0
	if pf.RemoteAddr != "" {
		if h, p, err := net.SplitHostPort(pf.RemoteAddr); err == nil {
			targetHost = h
			targetPort, _ = strconv.Atoi(p)
		}
	}

	return gin.H{
		"id":          pf.ID,
		"node_id":     pf.NodeID,
		"name":        pf.Name,
		"type":        pf.Type,
		"protocol":    pf.Type,
		"local_addr":  pf.LocalAddr,
		"remote_addr": pf.RemoteAddr,
		"listen_host": listenHost,
		"listen_port": listenPort,
		"target_host": targetHost,
		"target_port": targetPort,
		"chain_id":    pf.ChainID,
		"enabled":     pf.Enabled,
		"owner_id":    pf.OwnerID,
		"node_name":   nodeName,
		"created_at":  pf.CreatedAt,
		"updated_at":  pf.UpdatedAt,
	}
}

func (s *Server) listPortForwards(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	forwards, err := s.svc.ListPortForwards(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 构建节点名称映射
	nodeIDs := make(map[uint]bool)
	for _, f := range forwards {
		if f.NodeID > 0 {
			nodeIDs[f.NodeID] = true
		}
	}
	nodeNames := make(map[uint]string)
	if len(nodeIDs) > 0 {
		nodes, _ := s.svc.ListNodesByOwner(userID, isAdmin)
		for _, n := range nodes {
			if nodeIDs[n.ID] {
				nodeNames[n.ID] = n.Name
			}
		}
	}

	result := make([]gin.H, len(forwards))
	for i, f := range forwards {
		result[i] = portForwardToResponse(f, nodeNames[f.NodeID])
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) getPortForward(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	forward, err := s.svc.GetPortForwardByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "port forward not found"})
		return
	}

	nodeName := ""
	if forward.NodeID > 0 {
		if node, err := s.svc.GetNodeByOwner(forward.NodeID, userID, isAdmin); err == nil {
			nodeName = node.Name
		}
	}
	c.JSON(http.StatusOK, portForwardToResponse(*forward, nodeName))
}

type CreatePortForwardRequest struct {
	NodeID     uint   `json:"node_id"`
	Name       string `json:"name" binding:"required"`
	Type       string `json:"type"`        // tcp/udp/rtcp/rudp (后端字段)
	Protocol   string `json:"protocol"`    // 前端字段 (兼容)
	LocalAddr  string `json:"local_addr"`  // 后端字段
	RemoteAddr string `json:"remote_addr"` // 后端字段
	// 前端兼容字段
	ListenHost  string `json:"listen_host"`
	ListenPort  int    `json:"listen_port"`
	TargetHost  string `json:"target_host"`
	TargetPort  int    `json:"target_port"`
	Description string `json:"description"` // 前端发送但后端忽略
	ChainID     *uint  `json:"chain_id"`
	Enabled     bool   `json:"enabled"`
}

func (s *Server) createPortForward(c *gin.Context) {
	var req CreatePortForwardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 兼容前端字段
	forwardType := req.Type
	if forwardType == "" {
		forwardType = req.Protocol
	}
	if forwardType == "" {
		forwardType = "tcp"
	}

	localAddr := req.LocalAddr
	if localAddr == "" && req.ListenPort > 0 {
		host := req.ListenHost
		if host == "" {
			host = "0.0.0.0"
		}
		localAddr = fmt.Sprintf("%s:%d", host, req.ListenPort)
	}

	remoteAddr := req.RemoteAddr
	if remoteAddr == "" && req.TargetPort > 0 {
		remoteAddr = fmt.Sprintf("%s:%d", req.TargetHost, req.TargetPort)
	}

	if localAddr == "" || remoteAddr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local_addr and remote_addr are required"})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "port_forward")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}

		// 检查节点访问权限
		if req.NodeID > 0 {
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
		if req.ChainID != nil {
			if _, err := s.svc.GetProxyChainByOwner(*req.ChainID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
	}

	forward := &model.PortForward{
		NodeID:     req.NodeID,
		Name:       req.Name,
		Type:       forwardType,
		LocalAddr:  localAddr,
		RemoteAddr: remoteAddr,
		ChainID:    req.ChainID,
		Enabled:    req.Enabled,
		OwnerID:    &userID,
	}

	if err := s.svc.CreatePortForward(forward); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, forward)
}

func (s *Server) updatePortForward(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetPortForwardByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此转发规则"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	delete(updates, "id")
	delete(updates, "owner_id")
	delete(updates, "created_at")
	delete(updates, "updated_at")
	delete(updates, "description") // 前端发送但后端不支持
	delete(updates, "node_name")   // 响应专用字段

	// 兼容前端字段: protocol -> type
	if protocol, ok := updates["protocol"]; ok {
		updates["type"] = protocol
		delete(updates, "protocol")
	}

	// 兼容前端字段: listen_host + listen_port -> local_addr
	if _, hasListenPort := updates["listen_port"]; hasListenPort {
		host := "0.0.0.0"
		if h, ok := updates["listen_host"].(string); ok && h != "" {
			host = h
		}
		port := int(updates["listen_port"].(float64))
		updates["local_addr"] = fmt.Sprintf("%s:%d", host, port)
		delete(updates, "listen_host")
		delete(updates, "listen_port")
	}

	// 兼容前端字段: target_host + target_port -> remote_addr
	if _, hasTargetPort := updates["target_port"]; hasTargetPort {
		host := ""
		if h, ok := updates["target_host"].(string); ok {
			host = h
		}
		port := int(updates["target_port"].(float64))
		updates["remote_addr"] = fmt.Sprintf("%s:%d", host, port)
		delete(updates, "target_host")
		delete(updates, "target_port")
	}

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
		if rawChainID, exists := updates["chain_id"]; exists && rawChainID != nil {
			chainID, err := parseUintField(rawChainID, "chain_id")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if _, err := s.svc.GetProxyChainByOwner(chainID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
	}

	if err := s.svc.UpdatePortForward(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deletePortForward(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetPortForwardByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此转发规则"})
		return
	}

	if err := s.svc.DeletePortForward(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

