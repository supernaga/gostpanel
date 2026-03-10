// 节点组（负载均衡）模块
package api

import (
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/gost"
	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== 节点组 (负载均衡) ====================

func (s *Server) listNodeGroups(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	groups, err := s.svc.ListNodeGroups(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

func (s *Server) getNodeGroup(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	group, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node group not found"})
		return
	}
	c.JSON(http.StatusOK, group)
}

type CreateNodeGroupRequest struct {
	Name          string `json:"name" binding:"required"`
	Strategy      string `json:"strategy"`       // round/random/fifo/hash (后端)
	Selector      string `json:"selector"`       // 选择器配置 JSON
	FailTimeout   int    `json:"fail_timeout"`   // 故障超时时间(秒)
	MaxFails      int    `json:"max_fails"`      // 最大失败次数
	HealthCheck   bool   `json:"health_check"`   // 后端字段
	CheckInterval int    `json:"check_interval"` // 健康检查间隔(秒)
	// 前端兼容字段
	HealthCheckEnabled  bool   `json:"health_check_enabled"`  // 前端字段
	HealthCheckInterval int    `json:"health_check_interval"` // 前端字段 (毫秒)
	HealthCheckTimeout  int    `json:"health_check_timeout"`  // 前端发送但忽略
	Description         string `json:"description"`           // 前端发送但忽略
}

func (s *Server) createNodeGroup(c *gin.Context) {
	var req CreateNodeGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "node_group")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}
	}

	// 兼容前端策略值
	strategy := req.Strategy
	if strategy == "round_robin" {
		strategy = "round"
	}
	if strategy == "" {
		strategy = "round"
	}

	// 兼容前端健康检查字段
	healthCheck := req.HealthCheck || req.HealthCheckEnabled
	checkInterval := req.CheckInterval
	if checkInterval == 0 && req.HealthCheckInterval > 0 {
		checkInterval = req.HealthCheckInterval / 1000 // 毫秒转秒
	}
	if checkInterval == 0 {
		checkInterval = 30
	}

	group := &model.NodeGroup{
		Name:          req.Name,
		Strategy:      strategy,
		Selector:      req.Selector,
		FailTimeout:   req.FailTimeout,
		MaxFails:      req.MaxFails,
		HealthCheck:   healthCheck,
		CheckInterval: checkInterval,
		OwnerID:       &userID,
	}

	// 默认值
	if group.FailTimeout == 0 {
		group.FailTimeout = 30
	}
	if group.MaxFails == 0 {
		group.MaxFails = 3
	}

	if err := s.svc.CreateNodeGroup(group); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, group)
}

func (s *Server) updateNodeGroup(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
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
	delete(updates, "description")          // 前端发送但不支持
	delete(updates, "health_check_timeout") // 前端发送但不支持

	// 兼容前端策略值: round_robin -> round
	if strategy, ok := updates["strategy"].(string); ok && strategy == "round_robin" {
		updates["strategy"] = "round"
	}

	// 兼容前端字段: health_check_enabled -> health_check
	if healthCheckEnabled, ok := updates["health_check_enabled"]; ok {
		updates["health_check"] = healthCheckEnabled
		delete(updates, "health_check_enabled")
	}

	// 兼容前端字段: health_check_interval (毫秒) -> check_interval (秒)
	if interval, ok := updates["health_check_interval"].(float64); ok {
		updates["check_interval"] = int(interval / 1000)
		delete(updates, "health_check_interval")
	}

	if err := s.svc.UpdateNodeGroup(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteNodeGroup(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
		return
	}

	if err := s.svc.DeleteNodeGroup(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// 节点组成员

func (s *Server) listNodeGroupMembers(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
		return
	}
	members, err := s.svc.ListNodeGroupMembers(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, members)
}

type AddNodeGroupMemberRequest struct {
	NodeID   uint `json:"node_id" binding:"required"`
	Weight   int  `json:"weight"`
	Priority int  `json:"priority"`
}

func (s *Server) addNodeGroupMember(c *gin.Context) {
	groupID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeGroupByOwner(uint(groupID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
		return
	}

	var req AddNodeGroupMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isAdmin {
		if _, err := s.svc.GetNodeByOwner(req.NodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	member := &model.NodeGroupMember{
		GroupID:  uint(groupID),
		NodeID:   req.NodeID,
		Weight:   req.Weight,
		Priority: req.Priority,
		Enabled:  true,
	}

	if member.Weight == 0 {
		member.Weight = 1
	}

	if err := s.svc.AddNodeGroupMember(member); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, member)
}

func (s *Server) removeNodeGroupMember(c *gin.Context) {
	groupID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeGroupByOwner(uint(groupID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
		return
	}
	memberID, _ := strconv.ParseUint(c.Param("memberId"), 10, 32)
	if err := s.svc.DB().Where("id = ? AND group_id = ?", uint(memberID), uint(groupID)).First(&model.NodeGroupMember{}).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	if err := s.svc.RemoveNodeGroupMember(uint(memberID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) getNodeGroupConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	group, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node group not found"})
		return
	}

	members, err := s.svc.GetNodeGroupMembersWithNodes(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	generator := gost.NewConfigGenerator()
	config := generator.GenerateChainConfig(group, members)

	c.YAML(http.StatusOK, config)
}

