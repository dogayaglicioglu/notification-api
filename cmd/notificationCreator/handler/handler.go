package handler

import (
	"notif-api/internal/handlers"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func SetupRoutes(router *gin.Engine, notificationHandler *handlers.NotificationHandler) {
	router.POST("/notifications/batch", notificationHandler.CreateBatch)
	router.GET("/notifications/batch/:batchId", notificationHandler.GetByBatchID)
	router.GET("/notifications/:id", notificationHandler.GetByID)
	router.PATCH("/notifications/cancel", notificationHandler.Cancel)
	router.GET("/notifications", notificationHandler.List)

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "ok",
		})
	})

	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
