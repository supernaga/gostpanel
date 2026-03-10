// 代理链模块
package api

import (
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/gost"
	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== 代理链/隧道转发 ====================

func (s *Server) listProxyChains(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	var ownerID *uint
	if !isAdmin {
		ownerID = &userID
	}
	chains, err := s.svc.ListProxyChains(ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, chains)
}

func (s *Server) getProxyChain(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	chain, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy chain not found"})
		return
	}
	c.JSON(http.StatusOK, chain)
}

func (s *Server) createProxyChain(c *gin.Context) {
	var chain model.ProxyChain
	if err := c.ShouldBindJSON(&chain); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "proxy_chain")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}
	}

	// 强制设置所有者 (防止用户指定任意 owner_id)
	chain.OwnerID = &userID

	if err := s.svc.CreateProxyChain(&chain); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, chain)
}

func (s *Server) updateProxyChain(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
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

	if err := s.svc.UpdateProxyChainMap(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, _ := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin)
	c.JSON(http.StatusOK, result)
}

func (s *Server) deleteProxyChain(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}

	if err := s.svc.DeleteProxyChain(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) listProxyChainHops(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}
	hops, err := s.svc.GetProxyChainHopsWithNodes(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, hops)
}

func (s *Server) addProxyChainHop(c *gin.Context) {
	chainID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetProxyChainByOwner(uint(chainID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}

	var hop model.ProxyChainHop
	if err := c.ShouldBindJSON(&hop); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isAdmin {
		if _, err := s.svc.GetNodeByOwner(hop.NodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	hop.ChainID = uint(chainID)

	// 获取当前最大顺序号
	hops, _ := s.svc.GetProxyChainHops(uint(chainID))
	hop.HopOrder = len(hops)

	if err := s.svc.AddProxyChainHop(&hop); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, hop)
}

func (s *Server) updateProxyChainHop(c *gin.Context) {
	chainID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetProxyChainByOwner(uint(chainID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}
	hopID, _ := strconv.ParseUint(c.Param("hopId"), 10, 32)
	if err := s.svc.DB().Where("id = ? AND chain_id = ?", uint(hopID), uint(chainID)).First(&model.ProxyChainHop{}).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "hop not found"})
		return
	}

	var hop model.ProxyChainHop
	if err := c.ShouldBindJSON(&hop); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isAdmin {
		if _, err := s.svc.GetNodeByOwner(hop.NodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	hop.ID = uint(hopID)
	hop.ChainID = uint(chainID)
	if err := s.svc.UpdateProxyChainHop(&hop); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, hop)
}

func (s *Server) removeProxyChainHop(c *gin.Context) {
	chainID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetProxyChainByOwner(uint(chainID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}
	hopID, _ := strconv.ParseUint(c.Param("hopId"), 10, 32)
	if err := s.svc.DB().Where("id = ? AND chain_id = ?", uint(hopID), uint(chainID)).First(&model.ProxyChainHop{}).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "hop not found"})
		return
	}
	if err := s.svc.RemoveProxyChainHop(uint(hopID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) getProxyChainConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	chain, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy chain not found"})
		return
	}

	hops, err := s.svc.GetProxyChainHopsWithNodes(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	generator := gost.NewConfigGenerator()
	config := generator.GenerateProxyChainFullConfig(chain, hops)

	c.YAML(http.StatusOK, config)
}

