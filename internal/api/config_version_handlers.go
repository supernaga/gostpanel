// 配置版本历史模块
package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/gost"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
)



// ==================== ConfigVersion 配置版本历史 ====================

func (s *Server) getConfigVersions(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}

	versions, err := s.svc.GetConfigVersions(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, versions)
}

func (s *Server) createConfigVersion(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	var req struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取节点配置
	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 生成 YAML 配置
	generator := gost.NewConfigGenerator()
	bypasses, _ := s.svc.GetBypassesByNode(node.ID)
	admissions, _ := s.svc.GetAdmissionsByNode(node.ID)
	hostMappings, _ := s.svc.GetHostMappingsByNode(node.ID)
	ingresses, _ := s.svc.GetIngressesByNode(node.ID)
	config := generator.GenerateNodeConfigWithRules(node, bypasses, admissions, hostMappings, ingresses)

	// 将配置序列化为 YAML 字符串
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize config"})
		return
	}

	// 保存版本
	if err := s.svc.SaveConfigVersion(uint(id), string(configYAML), req.Comment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 清理旧版本（保留最新 20 个）
	s.svc.CleanupOldVersions(uint(id), 20)

	s.audit.LogSuccess(c, "create", "config_version", uint(id), req.Comment)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) getConfigVersion(c *gin.Context) {
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 32)

	version, err := s.svc.GetConfigVersion(uint(versionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(version.NodeID, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此配置版本"})
		return
	}

	c.JSON(http.StatusOK, version)
}

func (s *Server) restoreConfigVersion(c *gin.Context) {
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 32)

	version, err := s.svc.GetConfigVersion(uint(versionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}

	// 获取节点信息
	userID, isAdmin := getUserInfo(c)
	node, err := s.svc.GetNodeByOwner(version.NodeID, userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 检查节点是否在线
	if node.Status != "online" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "node is offline",
			"message": "节点离线，无法恢复配置",
		})
		return
	}

	// 解析 YAML 配置
	var config interface{}
	if err := yaml.Unmarshal([]byte(version.Config), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid config format"})
		return
	}

	// 获取 GOST API 客户端
	client, err := s.svc.GetGostClient(version.NodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to GOST"})
		return
	}

	// 应用配置到 GOST（使用 GOST API 重新加载配置）
	if err := client.ReloadConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to restore config: %v", err)})
		return
	}

	s.audit.LogSuccess(c, "restore", "config_version", uint(versionID), fmt.Sprintf("restored to node #%d", version.NodeID))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置已恢复",
	})
}

func (s *Server) deleteConfigVersion(c *gin.Context) {
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 32)

	version, err := s.svc.GetConfigVersion(uint(versionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(version.NodeID, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此配置版本"})
		return
	}

	if err := s.svc.DeleteConfigVersion(uint(versionID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "delete", "config_version", uint(versionID), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}
