// 用户管理模块（包含 2FA）
package api

import (
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== 用户管理 ====================

func (s *Server) listUsers(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	users, err := s.svc.GetUsersWithTrafficSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (s *Server) getUser(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	user, err := s.svc.GetUser(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

type CreateUserRequest struct {
	Username      string `json:"username" binding:"required"`
	Email         string `json:"email"`
	Password      string `json:"password" binding:"required"`
	Role          string `json:"role"`
	Enabled       *bool  `json:"enabled"` // 使用指针以区分未设置和 false
	EmailVerified *bool  `json:"email_verified"`
}

func (s *Server) createUser(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 默认启用账户
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// 管理员创建的账户默认验证邮箱
	emailVerified := true
	if req.EmailVerified != nil {
		emailVerified = *req.EmailVerified
	}

	user, err := s.svc.CreateUserFull(req.Username, req.Email, req.Password, req.Role, enabled, emailVerified)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

func (s *Server) updateUser(c *gin.Context) {
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

	// 防止篡改敏感字段
	delete(updates, "id")
	delete(updates, "created_at")

	if err := s.svc.UpdateUser(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteUser(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.DeleteUser(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func (s *Server) changePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 从 JWT 获取当前用户 ID
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	id := uint(userIDFloat)
	if err := s.svc.ChangePassword(id, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if currentJTI, ok := c.Get("jti"); ok {
		if jti, ok := currentJTI.(string); ok && jti != "" {
			_, _ = s.svc.DeleteOtherSessions(id, jti)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 个人账户设置 ====================

// getProfile 获取当前用户个人资料
func (s *Server) getProfile(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	user, err := s.svc.GetUser(uint(userIDFloat))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateProfileRequest 更新个人资料请求
type UpdateProfileRequest struct {
	Email *string `json:"email"`
}

// updateProfile 更新当前用户个人资料
func (s *Server) updateProfile(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Email != nil {
		updates["email"] = *req.Email
		// 如果邮箱变更，需要重新验证
		updates["email_verified"] = false
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no updates provided"})
		return
	}

	if err := s.svc.UpdateUser(uint(userIDFloat), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回更新后的用户信息
	user, _ := s.svc.GetUser(uint(userIDFloat))
	c.JSON(http.StatusOK, user)
}

// ==================== 2FA 双因素认证 ====================

// enable2FA 开始启用 2FA
func (s *Server) enable2FA(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	userID := uint(userIDFloat)

	var user model.User
	if err := s.svc.DB().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	if user.TwoFactorEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "2FA 已启用"})
		return
	}

	secret, qrCode, err := s.svc.GenerateTOTPSecret(user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成密钥失败"})
		return
	}

	// 临时保存 secret（未激活状态）
	s.svc.DB().Model(&user).Update("two_factor_secret", secret)

	c.JSON(http.StatusOK, gin.H{
		"secret": secret,
		"qrcode": qrCode,
	})
}

// verify2FA 验证并正式启用 2FA
func (s *Server) verify2FA(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	userID := uint(userIDFloat)

	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入验证码"})
		return
	}

	var user model.User
	if err := s.svc.DB().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	if !s.svc.ValidateTOTP(user.TwoFactorSecret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证码错误"})
		return
	}

	// 生成备份码
	codes, hashJSON, err := s.svc.GenerateBackupCodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成备份码失败"})
		return
	}

	s.svc.DB().Model(&user).Updates(map[string]interface{}{
		"two_factor_enabled": true,
		"backup_codes":       hashJSON,
	})

	// 记录操作日志
	username, _ := c.Get("username")
	s.svc.LogOperation(userID, username.(string), "enable", "2fa", userID, "启用 2FA", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	c.JSON(http.StatusOK, gin.H{
		"backup_codes": codes,
		"message":      "2FA 已启用",
	})
}

// disable2FA 禁用 2FA
func (s *Server) disable2FA(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	userID := uint(userIDFloat)

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入密码"})
		return
	}

	var user model.User
	if err := s.svc.DB().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	// 验证密码
	if !model.CheckPassword(user.Password, req.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码错误"})
		return
	}

	s.svc.DB().Model(&user).Updates(map[string]interface{}{
		"two_factor_enabled": false,
		"two_factor_secret":  "",
		"backup_codes":       "",
	})

	// 记录操作日志
	username, _ := c.Get("username")
	s.svc.LogOperation(userID, username.(string), "disable", "2fa", userID, "禁用 2FA", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	c.JSON(http.StatusOK, gin.H{"message": "2FA 已禁用"})
}

