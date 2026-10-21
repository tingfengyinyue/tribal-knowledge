package web

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/tingfengyinyue/tribal-knowledge/internal/bootstrap"
	"github.com/tingfengyinyue/tribal-knowledge/internal/session"
	"github.com/tingfengyinyue/tribal-knowledge/internal/store"
)

type Handler struct {
	svc   *session.Service
	store *store.Store
}

func NewHandler(svc *session.Service, st *store.Store) *Handler {
	return &Handler{svc: svc, store: st}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api/v1/skill")
	{
		api.POST("/sessions", h.createSession)
		api.POST("/sessions/:id/close", h.closeSession)
		api.PUT("/sessions/:id/steps/:seq", h.logStep)
		api.GET("/sessions/:id", h.getSession)
		api.POST("/sessions/:id/updates", h.updateSession)
		api.GET("/bootstrap", h.getBootstrap)

		// Admin
		api.POST("/admin/sessions/:id/revert", h.revertSession)
	}

	r.GET("/health", func(c *gin.Context) {
		if err := h.store.Ping(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})
}

func (h *Handler) createSession(c *gin.Context) {
	var in session.OpenInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errResp("INVALID_ARGUMENT", err.Error()))
		return
	}
	out, err := h.svc.Open(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL", err.Error()))
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) closeSession(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in session.CloseInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errResp("INVALID_ARGUMENT", err.Error()))
		return
	}
	in.SessionID = id
	out, err := h.svc.Close(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL", err.Error()))
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) logStep(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	seq, _ := strconv.Atoi(c.Param("seq"))
	var in session.LogStepInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errResp("INVALID_ARGUMENT", err.Error()))
		return
	}
	in.SessionID = id
	in.Seq = seq
	out, err := h.svc.LogStep(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL", err.Error()))
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) getSession(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	out, err := h.svc.GetSession(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", err.Error()))
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) updateSession(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in session.UpdateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, errResp("INVALID_ARGUMENT", err.Error()))
		return
	}
	in.SessionID = id
	out, err := h.svc.Update(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errResp("INTERNAL", err.Error()))
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handler) getBootstrap(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": bootstrap.Version,
		"script":  bootstrap.Script,
	})
}

func (h *Handler) revertSession(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)

	history, err := h.store.ListHistoryBySession(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, errResp("NOT_FOUND", err.Error()))
		return
	}

	reverted := 0
	for i := len(history) - 1; i >= 0; i-- {
		rec := history[i]
		item, err := h.store.GetItem(c.Request.Context(), rec.ItemID)
		if err != nil {
			continue
		}
		if rec.Op == "created" {
			item.Status = "invalid"
		} else if rec.BeforeData != nil {
			if title, ok := rec.BeforeData["title"].(string); ok {
				item.Title = title
			}
			if content, ok := rec.BeforeData["content"].(string); ok {
				item.Content = content
			}
			if status, ok := rec.BeforeData["status"].(string); ok {
				item.Status = status
			}
		}
		h.store.UpdateItem(c.Request.Context(), item)
		reverted++
	}

	c.JSON(http.StatusOK, gin.H{"reverted": reverted})
}

func errResp(code, msg string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": msg}}
}
