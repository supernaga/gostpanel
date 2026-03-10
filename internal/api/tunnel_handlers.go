// 隧道转发模块
package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/gost"
	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== 隧道转发 (入口-出口模式) ====================

func (s *Server) listTunnels(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	var ownerID *uint
	if !isAdmin {
		ownerID = &userID
	}
	tunnels, err := s.svc.ListTunnels(ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tunnels)
}

func (s *Server) getTunnel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	tunnel, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}
	c.JSON(http.StatusOK, tunnel)
}

func (s *Server) createTunnel(c *gin.Context) {
	var tunnel model.Tunnel
	if err := c.ShouldBindJSON(&tunnel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "tunnel")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}

		// 检查入口节点访问权限
		allowed, msg = s.svc.CheckPlanNodeAccess(userID, tunnel.EntryNodeID)
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "入口" + msg})
			return
		}

		// 检查出口节点访问权限
		allowed, msg = s.svc.CheckPlanNodeAccess(userID, tunnel.ExitNodeID)
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "出口" + msg})
			return
		}
	}

	// 强制设置所有者 (防止用户指定任意 owner_id)
	if !isAdmin {
		if _, err := s.svc.GetNodeByOwner(tunnel.EntryNodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
		if _, err := s.svc.GetNodeByOwner(tunnel.ExitNodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}
	tunnel.OwnerID = &userID

	if err := s.svc.CreateTunnel(&tunnel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 重新加载以获取关联数据
	result, _ := s.svc.GetTunnel(tunnel.ID)
	c.JSON(http.StatusOK, result)
}

func (s *Server) updateTunnel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此隧道"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 防止篡改受保护字段
	delete(updates, "id")
	delete(updates, "owner_id")
	delete(updates, "created_at")

	if !isAdmin {
		if rawEntryNodeID, exists := updates["entry_node_id"]; exists {
			if rawEntryNodeID == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entry_node_id"})
				return
			}
			entryNodeID, err := parseUintField(rawEntryNodeID, "entry_node_id")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			allowed, msg := s.svc.CheckPlanNodeAccess(userID, entryNodeID)
			if !allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": "鍏ュ彛" + msg})
				return
			}
			if _, err := s.svc.GetNodeByOwner(entryNodeID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
		if rawExitNodeID, exists := updates["exit_node_id"]; exists {
			if rawExitNodeID == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid exit_node_id"})
				return
			}
			exitNodeID, err := parseUintField(rawExitNodeID, "exit_node_id")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			allowed, msg := s.svc.CheckPlanNodeAccess(userID, exitNodeID)
			if !allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": "鍑哄彛" + msg})
				return
			}
			if _, err := s.svc.GetNodeByOwner(exitNodeID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
	}

	if err := s.svc.UpdateTunnelMap(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, _ := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	c.JSON(http.StatusOK, result)
}

func (s *Server) deleteTunnel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此隧道"})
		return
	}

	if err := s.svc.DeleteTunnel(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) syncTunnel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	tunnel, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	// 检查入口节点是否存在
	if tunnel.EntryNode == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "entry node not found"})
		return
	}

	// 检查入口节点是否在线
	if tunnel.EntryNode.Status != "online" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "entry node is offline",
			"message": "入口节点离线，无法同步配置",
		})
		return
	}

	// 生成隧道配置
	generator := gost.NewConfigGenerator()
	config := generator.GenerateTunnelEntryConfig(tunnel)
	if config == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to generate tunnel config"})
		return
	}

	// 连接入口节点的 GOST API
	client := gost.NewClient(
		tunnel.EntryNode.Host,
		tunnel.EntryNode.APIPort,
		tunnel.EntryNode.APIUser,
		tunnel.EntryNode.APIPass,
	)

	// 同步配置
	if err := client.SyncTunnelConfig(config, tunnel.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "sync failed",
			"message": fmt.Sprintf("同步到入口节点失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("隧道配置已同步到入口节点 %s", tunnel.EntryNode.Name),
	})
}

func (s *Server) getTunnelEntryConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	tunnel, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	generator := gost.NewConfigGenerator()
	config := generator.GenerateTunnelEntryConfig(tunnel)

	c.YAML(http.StatusOK, config)
}

func (s *Server) getTunnelExitConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	tunnel, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	generator := gost.NewConfigGenerator()
	config := generator.GenerateTunnelExitConfig(tunnel)

	c.YAML(http.StatusOK, config)
}

