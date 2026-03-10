// 网站配置模块
package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)



// ==================== 网站配置 ====================

func (s *Server) getSiteConfigs(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	configs := s.svc.GetSiteConfigs()
	c.JSON(http.StatusOK, configs)
}

func (s *Server) updateSiteConfigs(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var configs map[string]string
	if err := c.ShouldBindJSON(&configs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.SetSiteConfigs(configs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "配置已保存"})
}

// getPublicSiteConfig 公开接口，无需认证
func (s *Server) getPublicSiteConfig(c *gin.Context) {
	configs := s.svc.GetSiteConfigs()
	// 只返回前端需要的公开配置
	public := map[string]string{
		"site_name":        configs["site_name"],
		"site_description": configs["site_description"],
		"favicon_url":      configs["favicon_url"],
		"logo_url":         configs["logo_url"],
		"footer_text":      configs["footer_text"],
		"custom_css":       configs["custom_css"],
	}
	c.JSON(http.StatusOK, public)
}

