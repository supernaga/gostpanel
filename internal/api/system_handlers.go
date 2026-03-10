// 系统管理模块（日志、导出、备份）
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/supernaga/gost-panel/internal/gost"
	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
)



// ==================== 操作日志 ====================

func (s *Server) getOperationLogs(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	action := c.Query("action")
	resource := c.Query("resource")

	if limit > 100 {
		limit = 100
	}

	logs, total, err := s.svc.GetOperationLogs(limit, offset, action, resource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": total,
	})
}

func (s *Server) getNodeProxyURI(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	uri := gost.GenerateProxyURI(node)
	c.JSON(http.StatusOK, gin.H{"uri": uri})
}

// pingNode 测试节点延迟
func (s *Server) pingNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 测试 TCP 连接延迟到节点的代理端口
	addr := fmt.Sprintf("%s:%d", node.Host, node.Port)
	start := time.Now()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"latency": -1,
			"error":   "connection failed",
		})
		return
	}
	conn.Close()

	latency := time.Since(start).Milliseconds()
	c.JSON(http.StatusOK, gin.H{
		"latency": latency,
	})
}

// pingAllNodes 批量测试所有节点延迟（并发执行）
func (s *Server) pingAllNodes(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	nodes, err := s.svc.ListNodesByOwner(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	results := make(map[uint]int64)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 限制并发数，避免过多连接
	semaphore := make(chan struct{}, 20)

	for _, node := range nodes {
		wg.Add(1)
		go func(n model.Node) {
			defer wg.Done()
			semaphore <- struct{}{}        // 获取信号量
			defer func() { <-semaphore }() // 释放信号量

			addr := fmt.Sprintf("%s:%d", n.Host, n.Port)
			start := time.Now()

			conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
			var latency int64 = -1
			if err == nil {
				conn.Close()
				latency = time.Since(start).Milliseconds()
			}

			mu.Lock()
			results[n.ID] = latency
			mu.Unlock()
		}(node)
	}

	wg.Wait()
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// getNodeHealthLogs 获取节点健康检查日志
func (s *Server) getNodeHealthLogs(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	userID, isAdmin := getUserInfo(c)

	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if _, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	var logs []model.HealthCheckLog
	if err := s.svc.DB().Where("node_id = ?", uint(id)).
		Order("checked_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// getHealthSummary 获取健康检查概览
func (s *Server) getHealthSummary(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	nodes, err := s.svc.ListNodesByOwner(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type NodeHealthSummary struct {
		NodeID      uint      `json:"node_id"`
		NodeName    string    `json:"node_name"`
		Status      string    `json:"status"`
		LastCheck   time.Time `json:"last_check"`
		AvgLatency  int       `json:"avg_latency"`
		FailureRate float64   `json:"failure_rate"`
	}

	summaries := make([]NodeHealthSummary, 0, len(nodes))

	for _, node := range nodes {
		var lastLog model.HealthCheckLog
		var totalChecks int64
		var failedChecks int64
		var avgLatency float64

		// 获取最近一次健康检查
		s.svc.DB().Where("node_id = ?", node.ID).
			Order("checked_at DESC").
			First(&lastLog)

		// 统计最近24小时的健康检查
		since := time.Now().Add(-24 * time.Hour)
		s.svc.DB().Model(&model.HealthCheckLog{}).
			Where("node_id = ? AND checked_at >= ?", node.ID, since).
			Count(&totalChecks)

		s.svc.DB().Model(&model.HealthCheckLog{}).
			Where("node_id = ? AND checked_at >= ? AND status = ?", node.ID, since, "unhealthy").
			Count(&failedChecks)

		// 计算平均延迟（仅健康检查）
		var result struct {
			AvgLatency float64
		}
		s.svc.DB().Model(&model.HealthCheckLog{}).
			Select("AVG(latency) as avg_latency").
			Where("node_id = ? AND checked_at >= ? AND status = ?", node.ID, since, "healthy").
			Scan(&result)
		avgLatency = result.AvgLatency

		failureRate := 0.0
		if totalChecks > 0 {
			failureRate = float64(failedChecks) / float64(totalChecks) * 100
		}

		summaries = append(summaries, NodeHealthSummary{
			NodeID:      node.ID,
			NodeName:    node.Name,
			Status:      node.Status,
			LastCheck:   lastLog.CheckedAt,
			AvgLatency:  int(avgLatency),
			FailureRate: failureRate,
		})
	}

	c.JSON(http.StatusOK, gin.H{"summaries": summaries})
}

// ==================== 数据导出 ====================

// ExportNode 节点导出结构
type ExportNode struct {
	Name       string `json:"name" yaml:"name"`
	Host       string `json:"host" yaml:"host"`
	Port       int    `json:"port" yaml:"port"`
	Protocol   string `json:"protocol" yaml:"protocol"`
	Transport  string `json:"transport" yaml:"transport"`
	ProxyUser  string `json:"proxy_user,omitempty" yaml:"proxy_user,omitempty"`
	ProxyPass  string `json:"proxy_pass,omitempty" yaml:"proxy_pass,omitempty"`
	SSMethod   string `json:"ss_method,omitempty" yaml:"ss_method,omitempty"`
	SSPassword string `json:"ss_password,omitempty" yaml:"ss_password,omitempty"`
	TLSEnabled bool   `json:"tls_enabled" yaml:"tls_enabled"`
	TLSSNI     string `json:"tls_sni,omitempty" yaml:"tls_sni,omitempty"`
	WSPath     string `json:"ws_path,omitempty" yaml:"ws_path,omitempty"`
	WSHost     string `json:"ws_host,omitempty" yaml:"ws_host,omitempty"`
}

// ExportClient 客户端导出结构
type ExportClient struct {
	Name       string `json:"name" yaml:"name"`
	NodeName   string `json:"node_name" yaml:"node_name"`
	LocalPort  int    `json:"local_port" yaml:"local_port"`
	RemotePort int    `json:"remote_port" yaml:"remote_port"`
	ProxyUser  string `json:"proxy_user,omitempty" yaml:"proxy_user,omitempty"`
	ProxyPass  string `json:"proxy_pass,omitempty" yaml:"proxy_pass,omitempty"`
}

// ExportData 导出数据结构
type ExportData struct {
	Version  string         `json:"version" yaml:"version"`
	ExportAt string         `json:"export_at" yaml:"export_at"`
	Nodes    []ExportNode   `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Clients  []ExportClient `json:"clients,omitempty" yaml:"clients,omitempty"`
}

// exportData 导出数据
func (s *Server) exportData(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	format := c.DefaultQuery("format", "json")
	dataType := c.DefaultQuery("type", "all") // all, nodes, clients

	exportData := ExportData{
		Version:  "1.0",
		ExportAt: time.Now().Format(time.RFC3339),
	}

	// 导出节点
	if dataType == "all" || dataType == "nodes" {
		nodes, err := s.svc.ListNodes()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for _, n := range nodes {
			exportData.Nodes = append(exportData.Nodes, ExportNode{
				Name:       n.Name,
				Host:       n.Host,
				Port:       n.Port,
				Protocol:   n.Protocol,
				Transport:  n.Transport,
				ProxyUser:  n.ProxyUser,
				ProxyPass:  n.ProxyPass,
				SSMethod:   n.SSMethod,
				SSPassword: n.SSPassword,
				TLSEnabled: n.TLSEnabled,
				TLSSNI:     n.TLSSNI,
				WSPath:     n.WSPath,
				WSHost:     n.WSHost,
			})
		}
	}

	// 导出客户端
	if dataType == "all" || dataType == "clients" {
		clients, err := s.svc.ListClients()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for _, cl := range clients {
			nodeName := ""
			if cl.Node != nil {
				nodeName = cl.Node.Name
			}
			exportData.Clients = append(exportData.Clients, ExportClient{
				Name:       cl.Name,
				NodeName:   nodeName,
				LocalPort:  cl.LocalPort,
				RemotePort: cl.RemotePort,
				ProxyUser:  cl.ProxyUser,
				ProxyPass:  cl.ProxyPass,
			})
		}
	}

	// 输出格式
	if format == "yaml" {
		data, err := yaml.Marshal(exportData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", "attachment; filename=gost-panel-export.yaml")
		c.Data(http.StatusOK, "application/x-yaml", data)
	} else {
		data, err := json.MarshalIndent(exportData, "", "  ")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", "attachment; filename=gost-panel-export.json")
		c.Data(http.StatusOK, "application/json", data)
	}
}

// importData 导入数据
func (s *Server) importData(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}

	// 读取文件内容
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open file"})
		return
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	// 解析数据
	var importData ExportData

	// 尝试 JSON 解析
	if err := json.Unmarshal(content, &importData); err != nil {
		// 尝试 YAML 解析
		if err := yaml.Unmarshal(content, &importData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file format (expected JSON or YAML)"})
			return
		}
	}

	// 导入统计
	var nodesCreated, nodesSkipped, clientsCreated, clientsSkipped int

	// 预先查询所有节点和客户端，避免 N+1 查询
	existingNodes, _ := s.svc.ListNodes()
	existingClients, _ := s.svc.ListClients()

	// 构建名称查找 map
	nodeNameSet := make(map[string]bool)
	nodeNameToID := make(map[string]uint)
	for _, n := range existingNodes {
		nodeNameSet[n.Name] = true
		nodeNameToID[n.Name] = n.ID
	}
	clientNameSet := make(map[string]bool)
	for _, c := range existingClients {
		clientNameSet[c.Name] = true
	}

	// 导入节点
	for _, n := range importData.Nodes {
		// 检查是否已存在同名节点
		if nodeNameSet[n.Name] {
			nodesSkipped++
			continue
		}

		node := &model.Node{
			Name:       n.Name,
			Host:       n.Host,
			Port:       n.Port,
			Protocol:   n.Protocol,
			Transport:  n.Transport,
			ProxyUser:  n.ProxyUser,
			ProxyPass:  n.ProxyPass,
			SSMethod:   n.SSMethod,
			SSPassword: n.SSPassword,
			TLSEnabled: n.TLSEnabled,
			TLSSNI:     n.TLSSNI,
			WSPath:     n.WSPath,
			WSHost:     n.WSHost,
		}
		if err := s.svc.CreateNode(node); err == nil {
			nodesCreated++
			// 更新 map 以便客户端导入时可以找到新创建的节点
			nodeNameSet[n.Name] = true
			nodeNameToID[n.Name] = node.ID
		}
	}

	// 导入客户端
	for _, cl := range importData.Clients {
		// 检查是否已存在同名客户端
		if clientNameSet[cl.Name] {
			clientsSkipped++
			continue
		}

		// 查找节点 ID
		var nodeID uint
		if cl.NodeName != "" {
			nodeID = nodeNameToID[cl.NodeName]
		}

		client := &model.Client{
			Name:       cl.Name,
			NodeID:     nodeID,
			LocalPort:  cl.LocalPort,
			RemotePort: cl.RemotePort,
			ProxyUser:  cl.ProxyUser,
			ProxyPass:  cl.ProxyPass,
		}
		if err := s.svc.CreateClient(client); err == nil {
			clientsCreated++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Import completed",
		"nodes_created":   nodesCreated,
		"nodes_skipped":   nodesSkipped,
		"clients_created": clientsCreated,
		"clients_skipped": clientsSkipped,
	})
}

// ==================== 数据库备份/恢复 ====================

// backupDatabase 下载数据库备份
func (s *Server) backupDatabase(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	dbPath := s.cfg.DBPath

	// 检查文件是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "database file not found"})
		return
	}

	// 创建临时备份文件（避免读写冲突）
	backupPath := dbPath + ".backup"

	// 复制数据库文件
	src, err := os.Open(dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open database"})
		return
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create backup"})
		return
	}
	defer dst.Close()
	defer os.Remove(backupPath) // 清理临时文件

	if _, err := io.Copy(dst, src); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to copy database"})
		return
	}
	dst.Close()

	// 发送备份文件
	filename := fmt.Sprintf("gost-panel-backup-%s.db", time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.File(backupPath)
}

// restoreDatabase 恢复数据库
func (s *Server) restoreDatabase(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	file, err := c.FormFile("backup")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no backup file provided"})
		return
	}

	// 检查文件大小 (最大 100MB)
	if file.Size > 100*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 100MB)"})
		return
	}

	// 保存上传的文件到临时位置
	tempPath := s.cfg.DBPath + ".restore"
	if err := c.SaveUploadedFile(file, tempPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save uploaded file"})
		return
	}
	defer os.Remove(tempPath)

	// 验证是 SQLite 数据库
	testDB, err := model.InitDB(tempPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid database file"})
		return
	}
	sqlDB, _ := testDB.DB()
	sqlDB.Close()

	// 备份当前数据库
	currentBackup := s.cfg.DBPath + ".bak"
	if err := copyFile(s.cfg.DBPath, currentBackup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to backup current database"})
		return
	}

	// 替换数据库文件
	if err := copyFile(tempPath, s.cfg.DBPath); err != nil {
		// 恢复失败，还原备份
		copyFile(currentBackup, s.cfg.DBPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restore database"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Database restored successfully. Please restart the service.",
	})
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

