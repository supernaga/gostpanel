package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// SessionResponse 会话响应（包含当前会话标记）
type SessionResponse struct {
	ID         uint      `json:"id"`
	UserID     uint      `json:"user_id"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastActive time.Time `json:"last_active"`
	IsCurrent  bool      `json:"is_current"`
}

// getSessions 获取当前用户的活跃会话列表
func (s *Server) getSessions(c *gin.Context) {
	userID, _ := getUserInfo(c)
	currentJTI, _ := c.Get("jti")

	sessions, err := s.svc.GetUserSessions(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 构建响应，标记当前会话
	var response []SessionResponse
	for _, sess := range sessions {
		isCurrent := false
		if jti, ok := currentJTI.(string); ok && sess.TokenJTI == jti {
			isCurrent = true
		}
		response = append(response, SessionResponse{
			ID:         sess.ID,
			UserID:     sess.UserID,
			IP:         sess.IP,
			UserAgent:  sess.UserAgent,
			CreatedAt:  sess.CreatedAt,
			ExpiresAt:  sess.ExpiresAt,
			LastActive: sess.LastActive,
			IsCurrent:  isCurrent,
		})
	}

	c.JSON(http.StatusOK, response)
}

// deleteSession 撤销指定会话（强制下线）
func (s *Server) deleteSession(c *gin.Context) {
	userID, _ := getUserInfo(c)
	currentJTI, _ := c.Get("jti")
	sessionID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	// 检查会话是否属于当前用户
	session, err := s.svc.GetSessionByID(uint(sessionID))
	if err != nil || session.UserID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// 不允许删除当前会话
	if jti, ok := currentJTI.(string); ok && session.TokenJTI == jti {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete current session"})
		return
	}

	if err := s.svc.DeleteSession(uint(sessionID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 记录操作
	username, _ := c.Get("username")
	s.svc.LogOperation(userID, username.(string), "delete", "user_session", uint(sessionID),
		"session revoked", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// deleteOtherSessions 撤销除当前外所有会话
func (s *Server) deleteOtherSessions(c *gin.Context) {
	userID, _ := getUserInfo(c)
	currentJTI, _ := c.Get("jti")

	jti, ok := currentJTI.(string)
	if !ok || jti == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session"})
		return
	}

	count, err := s.svc.DeleteOtherSessions(userID, jti)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 记录操作
	username, _ := c.Get("username")
	s.svc.LogOperation(userID, username.(string), "delete", "user_session", 0,
		"all other sessions revoked", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	c.JSON(http.StatusOK, gin.H{"success": true, "count": count})
}
