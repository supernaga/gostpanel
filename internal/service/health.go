package service

import (
	"log"
	"sync"
	"time"

	"github.com/AliceNetworks/gost-panel/internal/gost"
	"github.com/AliceNetworks/gost-panel/internal/model"
	"gorm.io/gorm"
)

// HealthChecker 节点健康检查器
type HealthChecker struct {
	db           *gorm.DB
	alertService interface {
		TriggerAlert(alertType, targetType string, targetID uint, targetName, message string)
	}
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(db *gorm.DB, alertService interface {
	TriggerAlert(alertType, targetType string, targetID uint, targetName, message string)
}, interval time.Duration) *HealthChecker {
	return &HealthChecker{
		db:           db,
		alertService: alertService,
		interval:     interval,
		stopCh:       make(chan struct{}),
	}
}

// Start 启动健康检查
func (h *HealthChecker) Start() {
	h.wg.Add(1)
	go h.run()
	log.Printf("Health checker started (interval: %v)", h.interval)
}

// Stop 停止健康检查
func (h *HealthChecker) Stop() {
	close(h.stopCh)
	h.wg.Wait()
	log.Println("Health checker stopped")
}

func (h *HealthChecker) run() {
	defer h.wg.Done()

	// 延迟 30 秒再执行首次检测，给 Agent 时间发送心跳
	log.Println("Health checker: waiting 30s before first check...")
	select {
	case <-time.After(30 * time.Second):
		h.checkAll()
	case <-h.stopCh:
		return
	}

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkAll()
		case <-h.stopCh:
			return
		}
	}
}

func (h *HealthChecker) checkAll() {
	var nodes []model.Node
	if err := h.db.Find(&nodes).Error; err != nil {
		log.Printf("Health check: failed to get nodes: %v", err)
		return
	}

	log.Printf("Health check: checking %d nodes...", len(nodes))

	for _, node := range nodes {
		go h.checkNode(node)
	}

	// 检查节点超时 (2分钟无心跳则标记离线)
	h.checkNodeTimeout()

	// 检查客户端超时 (2分钟无心跳则标记离线)
	h.checkClientTimeout()
}

func (h *HealthChecker) checkNodeTimeout() {
	timeout := time.Now().Add(-2 * time.Minute)
	h.db.Model(&model.Node{}).
		Where("status = ? AND last_seen < ?", "online", timeout).
		Update("status", "offline")
}

func (h *HealthChecker) checkClientTimeout() {
	timeout := time.Now().Add(-2 * time.Minute)
	h.db.Model(&model.Client{}).
		Where("status = ? AND last_seen < ?", "online", timeout).
		Update("status", "offline")
}

func (h *HealthChecker) checkNode(node model.Node) {
	// 如果节点在 90 秒内收到过 Agent 心跳，以心跳为准，跳过直接 API 检测
	// 避免面板无法直接访问节点 GOST API（防火墙/NAT/仅监听本地）时的误判
	if !node.LastSeen.IsZero() && time.Since(node.LastSeen) < 90*time.Second {
		h.db.Create(&model.HealthCheckLog{
			NodeID:    node.ID,
			Status:    "healthy",
			Latency:   0,
			ErrorMsg:  "",
			CheckedAt: time.Now(),
		})
		// 清理旧日志（保留7天）
		h.db.Where("node_id = ? AND checked_at < ?", node.ID, time.Now().AddDate(0, 0, -7)).Delete(&model.HealthCheckLog{})
		return
	}

	// Agent 心跳超时或无 Agent，使用直接 GOST API 检测
	start := time.Now()
	client := gost.NewClient(node.Host, node.APIPort, node.APIUser, node.APIPass)

	err := client.Ping()
	latency := int(time.Since(start).Milliseconds())

	status := "healthy"
	errMsg := ""
	newNodeStatus := "online"

	if err != nil {
		status = "unhealthy"
		errMsg = err.Error()
		newNodeStatus = "offline"
	}

	// 记录健康检查日志
	h.db.Create(&model.HealthCheckLog{
		NodeID:    node.ID,
		Status:    status,
		Latency:   latency,
		ErrorMsg:  errMsg,
		CheckedAt: time.Now(),
	})

	// 状态变更
	if node.Status != newNodeStatus {
		log.Printf("Health check: node %s status changed: %s -> %s", node.Name, node.Status, newNodeStatus)

		// 更新数据库状态
		h.db.Model(&model.Node{}).Where("id = ?", node.ID).Updates(map[string]interface{}{
			"status":    newNodeStatus,
			"last_seen": time.Now(),
		})

		// 触发告警
		if newNodeStatus == "offline" && h.alertService != nil {
			h.alertService.TriggerAlert("node_offline", "node", node.ID, node.Name, "Node is offline")
		}
	} else if newNodeStatus == "online" {
		// 在线时更新 last_seen
		h.db.Model(&model.Node{}).Where("id = ?", node.ID).Update("last_seen", time.Now())
	}

	// 清理旧日志（保留7天）
	h.db.Where("node_id = ? AND checked_at < ?", node.ID, time.Now().AddDate(0, 0, -7)).Delete(&model.HealthCheckLog{})
}
