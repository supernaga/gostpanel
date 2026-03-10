// 规则管理模块（Bypass、Admission、HostMapping 等）
package api

import (
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)



// ==================== Bypass 分流规则 ====================

func (s *Server) listBypasses(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	bypasses, err := s.svc.ListBypasses(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, bypasses)
}

func (s *Server) getBypass(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	bypass, err := s.svc.GetBypassByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bypass not found"})
		return
	}
	c.JSON(http.StatusOK, bypass)
}

func (s *Server) createBypass(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var bypass model.Bypass
	if err := c.ShouldBindJSON(&bypass); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bypass.OwnerID = &userID
	if err := s.svc.CreateBypass(&bypass); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "bypass", bypass.ID, bypass.Name)
	c.JSON(http.StatusOK, bypass)
}

func (s *Server) updateBypass(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetBypassByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateBypass(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "bypass", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteBypass(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetBypassByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteBypass(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "bypass", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Admission 准入控制 ====================

func (s *Server) listAdmissions(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	admissions, err := s.svc.ListAdmissions(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, admissions)
}

func (s *Server) getAdmission(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	admission, err := s.svc.GetAdmissionByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "admission not found"})
		return
	}
	c.JSON(http.StatusOK, admission)
}

func (s *Server) createAdmission(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var admission model.Admission
	if err := c.ShouldBindJSON(&admission); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	admission.OwnerID = &userID
	if err := s.svc.CreateAdmission(&admission); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "admission", admission.ID, admission.Name)
	c.JSON(http.StatusOK, admission)
}

func (s *Server) updateAdmission(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetAdmissionByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateAdmission(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "admission", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteAdmission(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetAdmissionByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteAdmission(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "admission", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== HostMapping 主机映射 ====================

func (s *Server) listHostMappings(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	mappings, err := s.svc.ListHostMappings(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, mappings)
}

func (s *Server) getHostMapping(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	mapping, err := s.svc.GetHostMappingByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "host mapping not found"})
		return
	}
	c.JSON(http.StatusOK, mapping)
}

func (s *Server) createHostMapping(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var mapping model.HostMapping
	if err := c.ShouldBindJSON(&mapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mapping.OwnerID = &userID
	if err := s.svc.CreateHostMapping(&mapping); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "host_mapping", mapping.ID, mapping.Name)
	c.JSON(http.StatusOK, mapping)
}

func (s *Server) updateHostMapping(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetHostMappingByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateHostMapping(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "host_mapping", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteHostMapping(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetHostMappingByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteHostMapping(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "host_mapping", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Ingress 反向代理 ====================

func (s *Server) listIngresses(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	ingresses, err := s.svc.ListIngresses(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ingresses)
}

func (s *Server) getIngress(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	ingress, err := s.svc.GetIngressByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ingress not found"})
		return
	}
	c.JSON(http.StatusOK, ingress)
}

func (s *Server) createIngress(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var ingress model.Ingress
	if err := c.ShouldBindJSON(&ingress); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ingress.OwnerID = &userID
	if err := s.svc.CreateIngress(&ingress); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "ingress", ingress.ID, ingress.Name)
	c.JSON(http.StatusOK, ingress)
}

func (s *Server) updateIngress(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetIngressByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateIngress(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "ingress", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteIngress(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetIngressByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteIngress(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "ingress", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Recorder 流量记录 ====================

func (s *Server) listRecorders(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	recorders, err := s.svc.ListRecorders(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, recorders)
}

func (s *Server) getRecorder(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	recorder, err := s.svc.GetRecorderByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recorder not found"})
		return
	}
	c.JSON(http.StatusOK, recorder)
}

func (s *Server) createRecorder(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var recorder model.Recorder
	if err := c.ShouldBindJSON(&recorder); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	recorder.OwnerID = &userID
	if err := s.svc.CreateRecorder(&recorder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "recorder", recorder.ID, recorder.Name)
	c.JSON(http.StatusOK, recorder)
}

func (s *Server) updateRecorder(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetRecorderByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateRecorder(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "recorder", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteRecorder(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetRecorderByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteRecorder(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "recorder", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Router 路由管理 ====================

func (s *Server) listRouters(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	routers, err := s.svc.ListRouters(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, routers)
}

func (s *Server) getRouter(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	router, err := s.svc.GetRouterByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}
	c.JSON(http.StatusOK, router)
}

func (s *Server) createRouter(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var router model.Router
	if err := c.ShouldBindJSON(&router); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	router.OwnerID = &userID
	if err := s.svc.CreateRouter(&router); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "router", router.ID, router.Name)
	c.JSON(http.StatusOK, router)
}

func (s *Server) updateRouter(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetRouterByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateRouter(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "router", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteRouter(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetRouterByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteRouter(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "router", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== SD 服务发现 ====================

func (s *Server) listSDs(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	sds, err := s.svc.ListSDs(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sds)
}

func (s *Server) getSD(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	sd, err := s.svc.GetSDByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sd not found"})
		return
	}
	c.JSON(http.StatusOK, sd)
}

func (s *Server) createSD(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var sd model.SD
	if err := c.ShouldBindJSON(&sd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sd.OwnerID = &userID
	if err := s.svc.CreateSD(&sd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "sd", sd.ID, sd.Name)
	c.JSON(http.StatusOK, sd)
}

func (s *Server) updateSD(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetSDByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateSD(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "sd", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteSD(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetSDByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteSD(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "sd", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

