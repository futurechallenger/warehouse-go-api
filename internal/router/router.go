package router

// =============================================================================
//  internal/router — 路由注册
//  领域层级：门店(Store) → 仓库(Warehouse) → 货架(Shelf) → 单元(ShelfUnit)
// =============================================================================

import (
	"bytes"
	"io"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"warehouse/internal/handler"
)

// Setup 注册所有路由
func Setup(h *handler.Handler) *gin.Engine {
	r := gin.New()

	// 全局中间件
	r.Use(gin.Recovery())
	r.Use(RequestLogger())

	// ========== 健康检查 ==========
	r.GET("/ping", h.Ping) // Gin quick start 兼容
	r.GET("/health", h.Health)

	// ========== 设备命令 WebSocket 通道（app 端约定 ws://host:8080/ws）==========
	r.GET("/ws", h.HandleWS)

	// ========== API v1 ==========
	v1 := r.Group("/api")
	{
		// ---- 门店 ----
		v1.GET("/stores", h.ListStores)
		v1.POST("/stores", h.CreateStore)

		// ---- 仓库 ----
		v1.GET("/stores/:store_id/warehouses", h.ListStoreWarehouses)
		v1.POST("/stores/:store_id/warehouses", h.CreateWarehouse)
		v1.GET("/warehouses", h.ListWarehouses)
		v1.POST("/warehouses", h.CreateWarehouse)
		v1.GET("/warehouses/:warehouse_id", h.GetWarehouse)

		// ---- 货架（仓库维度）----
		v1.GET("/warehouses/:warehouse_id/shelves", h.ListShelves)
		v1.GET("/warehouses/:warehouse_id/shelves/:number", h.GetShelf)
		v1.POST("/warehouses/:warehouse_id/shelves", h.CreateShelf)
		v1.POST("/warehouses/:warehouse_id/shelves/init", h.InitShelves)

		// ---- 单元 ----
		v1.PUT("/units/:id/capacity", h.UpdateUnitCapacity)
		v1.PUT("/units/:id/product", h.AssignUnitProduct)

		// ---- 商品（货物主数据，EAN-13 / EAN-8 / QR code）----
		v1.GET("/products", h.ListProducts)
		v1.POST("/products", h.CreateProduct)
		// 按商品码查询必须注册在 /products/:id 之前，避免被 :id 捕获
		v1.GET("/products/by-barcode/:barcode", h.GetProductByBarcode)
		v1.GET("/products/:id", h.GetProduct)
		v1.PUT("/products/:id", h.UpdateProduct)
		v1.DELETE("/products/:id", h.DeleteProduct)

		// ---- 包裹（仓库维度）----
		v1.GET("/warehouses/:warehouse_id/packages/pending", h.ListPendingPackages)
		v1.POST("/warehouses/:warehouse_id/packages", h.CreatePackage)
		// 扫码建包：商品码 → 商品 → 该商品存放的货架单元
		v1.POST("/warehouses/:warehouse_id/packages/from-barcode", h.CreatePackageFromBarcode)
		v1.GET("/packages/:id", h.GetPackage)
		v1.PUT("/packages/:id/status", h.UpdatePackageStatus)

		// ---- 位置点（仓库维度：入口/出口/人员/机器位置等）----
		v1.GET("/warehouses/:warehouse_id/landmarks", h.ListLandmarks)
		v1.POST("/warehouses/:warehouse_id/landmarks", h.CreateLandmark)
		v1.GET("/warehouses/:warehouse_id/landmarks/:code", h.GetLandmark)
		v1.PATCH("/warehouses/:warehouse_id/landmarks/:code", h.UpdateLandmark)
		v1.DELETE("/warehouses/:warehouse_id/landmarks/:code", h.DeleteLandmark)

		// ---- 执行设备位置（仓库维度，复用位置点存储）----
		v1.PUT("/warehouses/:warehouse_id/devices/:device_id/position", h.UpsertDevicePosition)

		// ---- 路径规划（仓库维度）----
		v1.POST("/warehouses/:warehouse_id/path/plan", h.PlanPath)
		// app 端约定接口：POST /api/route/plan
		v1.POST("/route/plan", h.RoutePlan)

		// ---- 设备命令推送 ----
		v1.POST("/commands/push", h.PushCommand)

		// ---- 演示数据 ----
		v1.POST("/demo/init", h.InitDemo)
	}

	// ========== 旧接口兼容（映射到默认仓库）==========
	legacy := r.Group("/api")
	{
		legacy.GET("/shelves", h.ListShelves)
		legacy.GET("/shelves/:number", h.GetShelf)
		legacy.POST("/shelves", h.CreateShelf)
		legacy.POST("/shelves/init", h.InitShelves)
		legacy.GET("/packages/pending", h.ListPendingPackages)
		legacy.POST("/packages", h.CreatePackage)
		legacy.POST("/path/plan", h.PlanPath)
	}

	return r
}

// RequestLogger 请求访问日志中间件：打印访问的 API 与参数，异常（>=400）时附上错误信息
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 不打印 /ping 和 /health 的日志，避免刷屏
		path := c.Request.URL.Path
		if path == "/ping" || path == "/health" {
			c.Next()
			return
		}

		start := time.Now()
		method := c.Request.Method

		// 读取请求体（限 1MB），读完后恢复，不影响 handler 二次读取
		body := ""
		if c.Request.Body != nil {
			if raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20)); err == nil {
				c.Request.Body = io.NopCloser(bytes.NewReader(raw))
				body = string(raw)
			}
		}
		if len(body) > 500 { // 过长截断，避免刷屏
			body = body[:500] + "...(truncated)"
		}

		slog.Info("req",
			"method", method,
			"path", path,
			"query", c.Request.URL.RawQuery,
			"body", body,
		)

		c.Next()

		// 响应日志：状态码 + 耗时；异常时附带错误详情
		status := c.Writer.Status()
		if status >= 400 {
			attrs := []any{
				"method", method,
				"path", path,
				"status", status,
				"dur", time.Since(start).String(),
			}
			if len(c.Errors) > 0 {
				attrs = append(attrs, "errors", c.Errors.String())
			}
			slog.Warn("resp", attrs...)
		} else {
			slog.Info("resp",
				"method", method,
				"path", path,
				"status", status,
				"dur", time.Since(start).String(),
			)
		}
	}
}
