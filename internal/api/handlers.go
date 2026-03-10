// 通用辅助函数
// 所有具体的 handler 函数已拆分到各个模块文件中
package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)

// ==================== 辅助函数 ====================

// getUserInfo 从 JWT context 获取用户信息
func getUserInfo(c *gin.Context) (userID uint, isAdmin bool) {
	userIDFloat, _ := c.Get("user_id")
	role, _ := c.Get("role")

	if userIDFloat != nil {
		if id, ok := userIDFloat.(float64); ok {
			userID = uint(id)
		}
	}
	if role != nil {
		if r, ok := role.(string); ok {
			isAdmin = r == "admin"
		}
	}
	return
}

// parseID 从 URL 参数中解析 ID，失败时返回 400 错误
func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID 参数"})
		return 0, false
	}
	return uint(id), true
}

// getPanelURL 获取 Panel 的完整 URL（支持反向代理）
func (s *Server) getPanelURL(c *gin.Context) string {
	// 优先使用系统设置中的站点 URL
	if siteURL := s.svc.GetSiteConfig(model.ConfigSiteURL); siteURL != "" {
		return strings.TrimSuffix(siteURL, "/")
	}

	// 回退: 从请求头自动检测
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}

	scheme := "http"
	if c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	} else if strings.EqualFold(c.GetHeader("X-Forwarded-Ssl"), "on") {
		scheme = "https"
	} else if c.GetHeader("X-Forwarded-Port") == "443" {
		scheme = "https"
	} else if c.Request.TLS != nil {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}
