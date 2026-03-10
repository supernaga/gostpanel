// 流量历史模块
package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)



// ==================== 流量历史 ====================

func (s *Server) getTrafficHistory(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	hoursStr := c.DefaultQuery("hours", "1")
	hours, _ := strconv.Atoi(hoursStr)

	nodeIDStr := c.Query("node_id")
	var nodeID *uint
	if nodeIDStr != "" {
		id, err := strconv.ParseUint(nodeIDStr, 10, 32)
		if err == nil {
			uid := uint(id)
			// Verify ownership of the node
			if _, err := s.svc.GetNodeByOwner(uid, userID, isAdmin); err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
				return
			}
			nodeID = &uid
		}
	} else if !isAdmin {
		// Non-admin without node_id: only show their own nodes' traffic
		// For simplicity, return empty if no node_id specified for non-admin
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	history, err := s.svc.GetTrafficHistory(nodeID, hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, history)
}

