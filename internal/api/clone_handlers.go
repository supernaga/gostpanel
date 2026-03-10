// 克隆操作模块
package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== Clone Handlers for Rules ====================

func (s *Server) cloneBypass(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	bypass, err := s.svc.GetBypassByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bypass not found"})
		return
	}

	cloned := &model.Bypass{
		Name:      bypass.Name + " (副本)",
		Whitelist: bypass.Whitelist,
		Matchers:  bypass.Matchers,
		NodeID:    bypass.NodeID,
		OwnerID:   &userID,
	}

	if err := s.svc.CreateBypass(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "bypass", cloned.ID, fmt.Sprintf("from #%d", bypass.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneAdmission(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	admission, err := s.svc.GetAdmissionByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "admission not found"})
		return
	}

	cloned := &model.Admission{
		Name:      admission.Name + " (副本)",
		Whitelist: admission.Whitelist,
		Matchers:  admission.Matchers,
		NodeID:    admission.NodeID,
		OwnerID:   &userID,
	}

	if err := s.svc.CreateAdmission(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "admission", cloned.ID, fmt.Sprintf("from #%d", admission.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneHostMapping(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	mapping, err := s.svc.GetHostMappingByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "host mapping not found"})
		return
	}

	cloned := &model.HostMapping{
		Name:     mapping.Name + " (副本)",
		Mappings: mapping.Mappings,
		NodeID:   mapping.NodeID,
		OwnerID:  &userID,
	}

	if err := s.svc.CreateHostMapping(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "host_mapping", cloned.ID, fmt.Sprintf("from #%d", mapping.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneIngress(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	ingress, err := s.svc.GetIngressByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ingress not found"})
		return
	}

	cloned := &model.Ingress{
		Name:    ingress.Name + " (副本)",
		Rules:   ingress.Rules,
		NodeID:  ingress.NodeID,
		OwnerID: &userID,
	}

	if err := s.svc.CreateIngress(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "ingress", cloned.ID, fmt.Sprintf("from #%d", ingress.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneRecorder(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	recorder, err := s.svc.GetRecorderByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recorder not found"})
		return
	}

	cloned := &model.Recorder{
		Name:    recorder.Name + " (副本)",
		Type:    recorder.Type,
		Config:  recorder.Config,
		NodeID:  recorder.NodeID,
		OwnerID: &userID,
	}

	if err := s.svc.CreateRecorder(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "recorder", cloned.ID, fmt.Sprintf("from #%d", recorder.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneRouter(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	router, err := s.svc.GetRouterByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}

	cloned := &model.Router{
		Name:    router.Name + " (副本)",
		Routes:  router.Routes,
		NodeID:  router.NodeID,
		OwnerID: &userID,
	}

	if err := s.svc.CreateRouter(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "router", cloned.ID, fmt.Sprintf("from #%d", router.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneSD(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	sd, err := s.svc.GetSDByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sd not found"})
		return
	}

	cloned := &model.SD{
		Name:    sd.Name + " (副本)",
		Type:    sd.Type,
		Config:  sd.Config,
		NodeID:  sd.NodeID,
		OwnerID: &userID,
	}

	if err := s.svc.CreateSD(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "sd", cloned.ID, fmt.Sprintf("from #%d", sd.ID))
	c.JSON(http.StatusOK, cloned)
}

