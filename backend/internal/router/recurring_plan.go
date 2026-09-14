package router

import (
	"github.com/aasplit/aasplit/internal/handler"
	"github.com/aasplit/aasplit/internal/middleware"
	"github.com/aasplit/aasplit/internal/util"
	"github.com/gin-gonic/gin"
)

// RegisterRecurringPlanRoutes 注册周期账单计划路由。
func RegisterRecurringPlanRoutes(r *gin.RouterGroup, h *handler.RecurringPlanHandler, jwt *util.JWTManager) {
	groups := r.Group("/groups/:id/plans")
	groups.Use(middleware.Auth(jwt))
	{
		groups.GET("", h.List)
		groups.POST("", h.Create)
	}
	plans := r.Group("/plans")
	plans.Use(middleware.Auth(jwt))
	{
		plans.GET("/:id", h.Get)
		plans.PUT("/:id", h.Update)
		plans.POST("/:id/pause", h.Pause)
		plans.POST("/:id/resume", h.Resume)
		plans.POST("/:id/run", h.Run)
		plans.DELETE("/:id", h.Remove)
	}
}
