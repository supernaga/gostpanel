// 标签管理模块
package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== 节点标签管理 ====================

// listTags 获取所有标签
func (s *Server) listTags(c *gin.Context) {
	tags, err := s.svc.ListTags()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tags)
}

// getTag 获取单个标签
func (s *Server) getTag(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	tag, err := s.svc.GetTag(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}
	c.JSON(http.StatusOK, tag)
}

// CreateTagRequest 创建标签请求
type CreateTagRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color"`
}

// createTag 创建标签
func (s *Server) createTag(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可管理标签"})
		return
	}
	var req CreateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tag := &model.Tag{
		Name:  req.Name,
		Color: req.Color,
	}
	if tag.Color == "" {
		tag.Color = "#3b82f6" // 默认蓝色
	}

	if err := s.svc.CreateTag(tag); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, tag)
}

// updateTag 更新标签
func (s *Server) updateTag(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可管理标签"})
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.UpdateTag(uint(id), updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "标签已更新"})
}

// deleteTag 删除标签
func (s *Server) deleteTag(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可管理标签"})
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if err := s.svc.DeleteTag(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "标签已删除"})
}

// getNodeTags 获取节点的标签
func (s *Server) getNodeTags(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(nodeID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}
	tags, err := s.svc.GetNodeTags(uint(nodeID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tags)
}

// NodeTagRequest 节点标签请求
type NodeTagRequest struct {
	TagID uint `json:"tag_id" binding:"required"`
}

// addNodeTag 给节点添加标签
func (s *Server) addNodeTag(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(nodeID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}

	var req NodeTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.AddNodeTag(uint(nodeID), req.TagID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "标签已添加"})
}

// removeNodeTag 从节点移除标签
func (s *Server) removeNodeTag(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(nodeID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}
	tagID, _ := strconv.ParseUint(c.Param("tagId"), 10, 32)

	if err := s.svc.RemoveNodeTag(uint(nodeID), uint(tagID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "标签已移除"})
}

// SetNodeTagsRequest 设置节点标签请求
type SetNodeTagsRequest struct {
	TagIDs []uint `json:"tag_ids"`
}

// setNodeTags 设置节点的所有标签
func (s *Server) setNodeTags(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(nodeID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}

	var req SetNodeTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.SetNodeTags(uint(nodeID), req.TagIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "节点标签已更新"})
}

// getNodesByTag 获取具有指定标签的节点
func (s *Server) getNodesByTag(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	tagID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	nodes, err := s.svc.GetNodesByTag(uint(tagID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Filter by ownership for non-admin users
	if !isAdmin {
		filtered := make([]model.Node, 0)
		for _, n := range nodes {
			if n.OwnerID != nil && *n.OwnerID == userID {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}
	c.JSON(http.StatusOK, nodes)
}

// ==================== 全局搜索 ====================

// SearchResult 全局搜索结果
type SearchResult struct {
	Type string      `json:"type"` // node, client, user
	ID   uint        `json:"id"`
	Name string      `json:"name"`
	Desc string      `json:"desc"`
	Data interface{} `json:"data,omitempty"`
}

// globalSearch 全局搜索
func (s *Server) globalSearch(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusOK, []SearchResult{})
		return
	}

	userID, isAdmin := getUserInfo(c)
	var results []SearchResult

	// 搜索节点
	nodes, _ := s.svc.ListNodesByOwner(userID, isAdmin)
	for _, node := range nodes {
		if containsIgnoreCase(node.Name, query) || containsIgnoreCase(node.Host, query) {
			results = append(results, SearchResult{
				Type: "node",
				ID:   node.ID,
				Name: node.Name,
				Desc: fmt.Sprintf("%s:%d - %s", node.Host, node.Port, node.Status),
			})
		}
		if len(results) >= 20 {
			break
		}
	}

	// 搜索客户端
	clients, _ := s.svc.ListClientsByOwner(userID, isAdmin)
	for _, client := range clients {
		if containsIgnoreCase(client.Name, query) || containsIgnoreCase(client.Token, query) {
			tokenPreview := client.Token
			if len(tokenPreview) > 8 {
				tokenPreview = tokenPreview[:8] + "..."
			}
			results = append(results, SearchResult{
				Type: "client",
				ID:   client.ID,
				Name: client.Name,
				Desc: fmt.Sprintf("Token: %s - %s", tokenPreview, client.Status),
			})
		}
		if len(results) >= 30 {
			break
		}
	}

	// 搜索用户 (仅管理员)
	if isAdmin {
		users, _ := s.svc.ListUsers()
		for _, user := range users {
			userEmail := ""
			if user.Email != nil {
				userEmail = *user.Email
			}
			if containsIgnoreCase(user.Username, query) || containsIgnoreCase(userEmail, query) {
				results = append(results, SearchResult{
					Type: "user",
					ID:   user.ID,
					Name: user.Username,
					Desc: fmt.Sprintf("%s - %s", userEmail, user.Role),
				})
			}
			if len(results) >= 40 {
				break
			}
		}
	}

	c.JSON(http.StatusOK, results)
}

// containsIgnoreCase 忽略大小写的字符串包含检查
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

