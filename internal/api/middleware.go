package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// APIRateLimiter 全局 API 限流器 (Token Bucket 算法)
type APIRateLimiter struct {
	mu       sync.RWMutex
	visitors map[string]*visitor
	limit    int
	window   time.Duration
	stopCh   chan struct{}
}

type visitor struct {
	tokens    int
	lastReset time.Time
}

// NewAPIRateLimiter 创建 API 限流器
func NewAPIRateLimiter(limit int, window time.Duration) *APIRateLimiter {
	rl := &APIRateLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		window:   window,
		stopCh:   make(chan struct{}),
	}
	// 定期清理
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rl.cleanup()
			case <-rl.stopCh:
				return
			}
		}
	}()
	return rl
}

// Stop 停止清理 goroutine
func (rl *APIRateLimiter) Stop() {
	close(rl.stopCh)
}

// Allow 检查是否允许请求，返回 (是否允许, 剩余配额, 重置时间)
func (rl *APIRateLimiter) Allow(key string) (bool, int, time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[key]
	now := time.Now()

	if !exists || now.Sub(v.lastReset) > rl.window {
		rl.visitors[key] = &visitor{tokens: rl.limit - 1, lastReset: now}
		return true, rl.limit - 1, now.Add(rl.window)
	}

	if v.tokens <= 0 {
		return false, 0, v.lastReset.Add(rl.window)
	}

	v.tokens--
	return true, v.tokens, v.lastReset.Add(rl.window)
}

// cleanup 定期清理过期记录
func (rl *APIRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for key, v := range rl.visitors {
		if time.Since(v.lastReset) > rl.window*2 {
			delete(rl.visitors, key)
		}
	}
}

// APIRateLimitMiddleware 全局 API 限流中间件
func APIRateLimitMiddleware(limiter *APIRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 按用户ID限流（已认证），否则按IP限流
		key := c.ClientIP()
		if userID, exists := c.Get("user_id"); exists {
			key = fmt.Sprintf("user:%v", userID)
		}

		allowed, remaining, reset := limiter.Allow(key)

		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", limiter.limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", reset.Unix()))

		if !allowed {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "请求过于频繁，请稍后再试",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
