package main

// =============================================================================
//  cmd/main.go — 入口
// =============================================================================

import (
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"warehouse/internal/config"
	"warehouse/internal/handler"
	"warehouse/internal/repository"
	"warehouse/internal/router"
	"warehouse/internal/service"
	"warehouse/internal/ws"
)

func main() {
	// 1. 加载配置
	cfg := config.Load()

	// 2. 设置 Gin 模式
	gin.SetMode(cfg.Server.Mode)

	// 3. 初始化数据库 (ent)
	repo, err := repository.New(cfg.Database.DSN)
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}
	defer repo.Close()
	log.Println("✅ 数据库连接成功")

	// 4. 初始化业务服务
	svc := service.NewWarehouseService(repo)

	// 5. 初始化设备命令 WebSocket 通道 + HTTP 处理器
	commandHub := ws.NewHub()
	h := handler.NewHandler(svc, commandHub)

	// 6. 注册路由
	r := router.Setup(h)

	// 7. 启动服务
	addr := ":" + cfg.Server.Port
	fmt.Printf("🚀 Warehouse API listening on %s\n", addr)
	fmt.Printf("   GET  /ping, /health              → health check\n")
	fmt.Printf("   GET/POST /api/stores             → stores\n")
	fmt.Printf("   GET/POST /api/stores/:store_id/warehouses → store warehouses\n")
	fmt.Printf("   GET/POST /api/warehouses         → warehouses\n")
	fmt.Printf("   GET  /api/warehouses/:warehouse_id → warehouse detail\n")
	fmt.Printf("   GET/POST /api/warehouses/:warehouse_id/shelves → shelves\n")
	fmt.Printf("   GET  /api/warehouses/:warehouse_id/shelves/:number → shelf detail\n")
	fmt.Printf("   POST /api/warehouses/:warehouse_id/shelves/init → init shelves\n")
	fmt.Printf("   PUT  /api/units/:id/capacity     → update unit capacity\n")
	fmt.Printf("   PUT  /api/units/:id/product      → bind/unbind product to unit\n")
	fmt.Printf("   GET/POST /api/products           → products (ean13/ean8/qr_code)\n")
	fmt.Printf("   GET/PUT/DELETE /api/products/:id → product detail/update/delete\n")
	fmt.Printf("   GET/POST /api/warehouses/:warehouse_id/packages → packages\n")
	fmt.Printf("   POST /api/warehouses/:warehouse_id/packages/from-barcode → create package by product barcode\n")
	fmt.Printf("   GET/POST /api/warehouses/:warehouse_id/landmarks → landmarks (entrance/exit/person/robot)\n")
	fmt.Printf("   GET/PATCH/DELETE /api/warehouses/:warehouse_id/landmarks/:code → landmark detail\n")
	fmt.Printf("   POST /api/warehouses/:warehouse_id/path/plan → path planning (start/end 支持位置编码)\n")
	fmt.Printf("   POST /api/route/plan             → path planning (app 约定格式)\n")
	fmt.Printf("   GET  /ws                          → device command WebSocket channel\n")
	fmt.Printf("   POST /api/commands/push           → push command to connected apps (e.g. open_camera)\n")
	fmt.Printf("   POST /api/demo/init              → init demo data\n")
	fmt.Printf("   (旧接口 /api/shelves、/api/packages、/api/path/plan 仍兼容，映射默认仓库)\n")

	if err := r.Run(addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
		os.Exit(1)
	}
}
