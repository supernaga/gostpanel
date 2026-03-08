package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/AliceNetworks/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Login2FARequest 2FA 登录请求
type Login2FARequest struct {
	TempToken string `json:"temp_token" binding:"required"`
	Code      string `json:"code" binding:"required"`
}

func (s *Server) login2FA(c *gin.Context) {
	var req Login2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证临时令牌
	token, err := jwt.Parse(req.TempToken, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired temp token"})
		return
	}

	claims := token.Claims.(jwt.MapClaims)

	// 检查是否为临时令牌
	temp2FA, ok := claims["temp_2fa"].(bool)
	if !ok || !temp2FA {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid temp token"})
		return
	}

	userIDFloat, _ := claims["user_id"].(float64)
	userID := uint(userIDFloat)

	// 获取用户信息
	var user model.User
	if err := s.svc.DB().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// 验证 TOTP 代码
	validTOTP := s.svc.ValidateTOTP(user.TwoFactorSecret, req.Code)
	validBackup := false
	var newBackupCodes string

	if !validTOTP {
		// 尝试验证备份码
		validBackup, newBackupCodes = s.svc.ValidateBackupCode(user.BackupCodes, req.Code)
	}

	if !validTOTP && !validBackup {
		// 记录失败尝试
		s.svc.LogOperation(userID, user.Username, "login", "2fa", userID, "2FA verification failed", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
		RecordLoginAttempt(false)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid 2FA code"})
		return
	}

	// 如果使用了备份码，更新数据库
	if validBackup {
		s.svc.DB().Model(&user).Update("backup_codes", newBackupCodes)
	}

	// 登录成功，重置限流计数
	s.loginLimiter.Reset(c.ClientIP())
	RecordLoginAttempt(true)

	// 更新登录信息
	s.svc.UpdateUserLoginInfo(user.ID, c.ClientIP())

	// 记录登录成功
	s.svc.LogOperation(user.ID, user.Username, "login", "2fa", user.ID, "2FA login success", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	// 生成正式 JWT（带会话管理）
	jti := uuid.NewString()
	expiresAt := time.Now().Add(24 * time.Hour)

	finalToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
		"jti":      jti,
		"exp":      expiresAt.Unix(),
	})

	tokenString, err := finalToken.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// 创建会话记录
	if err := s.svc.CreateUserSession(user.ID, jti, c.ClientIP(), c.GetHeader("User-Agent"), expiresAt); err != nil {
		// 会话创建失败不影响登录，只记录错误
		s.svc.LogOperation(user.ID, user.Username, "session_create", "user_session", 0, fmt.Sprintf("failed to create session: %v", err), c.ClientIP(), c.GetHeader("User-Agent"), "failed")
	}

	c.JSON(http.StatusOK, gin.H{
		"token": tokenString,
		"user": gin.H{
			"id":               user.ID,
			"username":         user.Username,
			"email":            user.Email,
			"role":             user.Role,
			"email_verified":   user.EmailVerified,
			"password_changed": user.PasswordChanged,
		},
	})
}
