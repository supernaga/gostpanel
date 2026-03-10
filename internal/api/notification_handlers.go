// 通知和告警模块
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== 通知渠道管理 ====================

func (s *Server) listNotifyChannels(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	channels, err := s.svc.GetAlertService().ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, channels)
}

func (s *Server) getNotifyChannel(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	channel, err := s.svc.GetAlertService().GetChannel(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}
	c.JSON(http.StatusOK, channel)
}

type CreateNotifyChannelRequest struct {
	Name    string                 `json:"name" binding:"required"`
	Type    string                 `json:"type" binding:"required"` // telegram/webhook/smtp
	Config  map[string]interface{} `json:"config" binding:"required"`
	Enabled bool                   `json:"enabled"`
}

func (s *Server) createNotifyChannel(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var req CreateNotifyChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 将 config 对象转为 JSON 字符串
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config format"})
		return
	}

	channel := &model.NotifyChannel{
		Name:    req.Name,
		Type:    req.Type,
		Config:  string(configJSON),
		Enabled: req.Enabled,
	}

	if err := s.svc.GetAlertService().CreateChannel(channel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, channel)
}

func (s *Server) updateNotifyChannel(c *gin.Context) {
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

	delete(updates, "id")
	delete(updates, "created_at")

	// 如果 config 是对象，转为 JSON 字符串
	if config, ok := updates["config"]; ok && config != nil {
		if configMap, isMap := config.(map[string]interface{}); isMap {
			configJSON, err := json.Marshal(configMap)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config format"})
				return
			}
			updates["config"] = string(configJSON)
		}
	}

	if err := s.svc.GetAlertService().UpdateChannel(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteNotifyChannel(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.GetAlertService().DeleteChannel(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) testNotifyChannel(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.GetAlertService().TestChannel(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "测试通知已发送"})
}

// ==================== 告警规则管理 ====================

func (s *Server) listAlertRules(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	rules, err := s.svc.GetAlertService().ListRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 转换为前端期望的格式
	formattedRules := s.formatAlertRules(rules)
	c.JSON(http.StatusOK, formattedRules)
}

func (s *Server) getAlertRule(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	rule, err := s.svc.GetAlertService().GetRule(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	// 转换为前端期望的格式
	formatted := s.formatAlertRule(rule)
	c.JSON(http.StatusOK, formatted)
}

// formatAlertRules 转换告警规则列表为前端格式
func (s *Server) formatAlertRules(rules []model.AlertRule) []gin.H {
	result := make([]gin.H, len(rules))
	for i, rule := range rules {
		result[i] = s.formatAlertRule(&rule)
	}
	return result
}

// formatAlertRule 转换单个告警规则为前端格式
func (s *Server) formatAlertRule(rule *model.AlertRule) gin.H {
	// 解析 channel_ids 字符串为数组 - 始终返回数组，即使为空
	channelIDs := make([]int, 0)
	if rule.ChannelIDs != "" {
		for _, idStr := range strings.Split(rule.ChannelIDs, ",") {
			idStr = strings.TrimSpace(idStr)
			if id, err := strconv.Atoi(idStr); err == nil {
				channelIDs = append(channelIDs, id)
			}
		}
	}

	// 解析 condition JSON 字符串为对象
	var condition interface{}
	if rule.Condition != "" && rule.Condition != "{}" {
		json.Unmarshal([]byte(rule.Condition), &condition)
	} else {
		condition = map[string]interface{}{}
	}

	return gin.H{
		"id":               rule.ID,
		"name":             rule.Name,
		"type":             rule.Type,
		"alert_type":       rule.Type, // 前端兼容字段
		"condition":        condition,
		"channel_ids":      channelIDs,
		"enabled":          rule.Enabled,
		"cooldown_min":     rule.CooldownMin,
		"silence_duration": rule.CooldownMin * 60000, // 转为毫秒给前端
		"last_alert_at":    rule.LastAlertAt,
		"created_at":       rule.CreatedAt,
		"updated_at":       rule.UpdatedAt,
	}
}

type CreateAlertRuleRequest struct {
	Name            string      `json:"name" binding:"required"`
	Type            string      `json:"type"`        // 后端字段名
	AlertType       string      `json:"alert_type"`  // 前端字段名 (兼容)
	Condition       interface{} `json:"condition"`   // 接受对象或字符串
	ChannelIDs      interface{} `json:"channel_ids"` // 接受数组或字符串
	Enabled         bool        `json:"enabled"`
	CooldownMin     int         `json:"cooldown_min"`     // 后端字段名 (分钟)
	SilenceDuration int         `json:"silence_duration"` // 前端字段名 (毫秒，兼容)
}

func (s *Server) createAlertRule(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var req CreateAlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 兼容前端字段名
	ruleType := req.Type
	if ruleType == "" {
		ruleType = req.AlertType
	}
	if ruleType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type or alert_type is required"})
		return
	}

	// 处理 condition (对象转字符串)
	conditionStr := ""
	if req.Condition != nil {
		switch v := req.Condition.(type) {
		case string:
			conditionStr = v
		case map[string]interface{}:
			if data, err := json.Marshal(v); err == nil {
				conditionStr = string(data)
			}
		}
	}

	// 处理 channel_ids (数组转逗号分隔字符串)
	channelIDsStr := ""
	if req.ChannelIDs != nil {
		switch v := req.ChannelIDs.(type) {
		case string:
			channelIDsStr = v
		case []interface{}:
			ids := make([]string, len(v))
			for i, id := range v {
				ids[i] = fmt.Sprintf("%v", id)
			}
			channelIDsStr = strings.Join(ids, ",")
		}
	}

	// 处理 cooldown_min (前端发毫秒，转为分钟)
	cooldownMin := req.CooldownMin
	if cooldownMin == 0 && req.SilenceDuration > 0 {
		cooldownMin = req.SilenceDuration / 60000 // 毫秒转分钟
	}

	rule := &model.AlertRule{
		Name:        req.Name,
		Type:        ruleType,
		Condition:   conditionStr,
		ChannelIDs:  channelIDsStr,
		Enabled:     req.Enabled,
		CooldownMin: cooldownMin,
	}

	if rule.CooldownMin == 0 {
		rule.CooldownMin = 30
	}

	if err := s.svc.GetAlertService().CreateRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rule)
}

func (s *Server) updateAlertRule(c *gin.Context) {
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

	delete(updates, "id")
	delete(updates, "created_at")
	delete(updates, "last_alert_at")

	// 兼容前端字段名: alert_type -> type
	if alertType, ok := updates["alert_type"]; ok {
		updates["type"] = alertType
		delete(updates, "alert_type")
	}

	// 兼容前端字段名: silence_duration (毫秒) -> cooldown_min (分钟)
	if silenceDuration, ok := updates["silence_duration"]; ok {
		if ms, isFloat := silenceDuration.(float64); isFloat {
			updates["cooldown_min"] = int(ms / 60000)
		}
		delete(updates, "silence_duration")
	}

	// 处理 condition (对象转字符串)
	if condition, ok := updates["condition"]; ok && condition != nil {
		if condMap, isMap := condition.(map[string]interface{}); isMap {
			if data, err := json.Marshal(condMap); err == nil {
				updates["condition"] = string(data)
			}
		}
	}

	// 处理 channel_ids (数组转逗号分隔字符串)
	if channelIDs, ok := updates["channel_ids"]; ok && channelIDs != nil {
		if ids, isArray := channelIDs.([]interface{}); isArray {
			strIDs := make([]string, len(ids))
			for i, id := range ids {
				strIDs[i] = fmt.Sprintf("%v", id)
			}
			updates["channel_ids"] = strings.Join(strIDs, ",")
		}
	}

	if err := s.svc.GetAlertService().UpdateRule(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteAlertRule(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.GetAlertService().DeleteRule(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 告警日志 ====================

func (s *Server) getAlertLogs(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	logs, total, err := s.svc.GetAlertService().GetAlertLogs(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 转换为前端期望的格式
	formattedLogs := make([]gin.H, len(logs))
	for i, log := range logs {
		formattedLogs[i] = gin.H{
			"id":          log.ID,
			"rule_id":     log.RuleID,
			"rule_name":   log.RuleName,
			"type":        log.Type,
			"alert_type":  log.Type, // 前端兼容字段
			"message":     log.Message,
			"target_type": log.TargetType,
			"target_id":   log.TargetID,
			"target_name": log.TargetName,
			"status":      log.Status,
			"sent":        log.Status == "sent", // 前端期望的布尔值
			"created_at":  log.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  formattedLogs,
		"total": total,
	})
}

