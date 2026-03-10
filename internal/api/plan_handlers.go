// 套餐管理模块
package api

import (
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== 套餐管理 ====================

func (s *Server) listPlans(c *gin.Context) {
	plans, err := s.svc.ListPlans()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 附加每个套餐的用户数量
	result := make([]gin.H, len(plans))
	for i, plan := range plans {
		userCount, _ := s.svc.GetPlanUserCount(plan.ID)
		result[i] = gin.H{
			"id":                plan.ID,
			"name":              plan.Name,
			"description":       plan.Description,
			"traffic_quota":     plan.TrafficQuota,
			"speed_limit":       plan.SpeedLimit,
			"duration":          plan.Duration,
			"max_nodes":         plan.MaxNodes,
			"max_clients":       plan.MaxClients,
			"max_tunnels":       plan.MaxTunnels,
			"max_port_forwards": plan.MaxPortForwards,
			"max_proxy_chains":  plan.MaxProxyChains,
			"max_node_groups":   plan.MaxNodeGroups,
			"enabled":           plan.Enabled,
			"sort_order":        plan.SortOrder,
			"user_count":        userCount,
			"created_at":        plan.CreatedAt,
			"updated_at":        plan.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) getPlan(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	plan, err := s.svc.GetPlan(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "plan not found"})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (s *Server) createPlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var plan model.Plan
	if err := c.ShouldBindJSON(&plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if plan.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "套餐名称不能为空"})
		return
	}

	if err := s.svc.CreatePlan(&plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plan)
}

func (s *Server) updatePlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.UpdatePlan(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deletePlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if err := s.svc.DeletePlan(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 用户套餐操作 ====================

func (s *Server) assignUserPlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	userID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var req struct {
		PlanID uint `json:"plan_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.AssignUserPlan(uint(userID), req.PlanID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) removeUserPlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	userID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.RemoveUserPlan(uint(userID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) renewUserPlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	userID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var req struct {
		Days int `json:"days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Days <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "续期天数必须大于0"})
		return
	}

	if err := s.svc.RenewUserPlan(uint(userID), req.Days); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 套餐资源关联 ====================

// getPlanResources 获取套餐关联的资源
func (s *Server) getPlanResources(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	resources, err := s.svc.GetPlanResources(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 组织成 map[resourceType][]resourceID 的格式
	result := make(map[string][]uint)
	result["node"] = []uint{}
	result["tunnel"] = []uint{}
	result["port_forward"] = []uint{}
	result["proxy_chain"] = []uint{}
	result["node_group"] = []uint{}

	for _, r := range resources {
		result[r.ResourceType] = append(result[r.ResourceType], r.ResourceID)
	}

	c.JSON(http.StatusOK, result)
}

// setPlanResources 设置套餐关联的资源
func (s *Server) setPlanResources(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var req map[string][]uint
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 对每个 resourceType 调用 SetPlanResources
	validTypes := []string{"node", "tunnel", "port_forward", "proxy_chain", "node_group"}
	for _, resourceType := range validTypes {
		resourceIDs, ok := req[resourceType]
		if !ok {
			resourceIDs = []uint{}
		}
		if err := s.svc.SetPlanResources(uint(id), resourceType, resourceIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

