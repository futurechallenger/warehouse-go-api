package handler

// =============================================================================
//  internal/handler — Gin HTTP 处理器
//  领域层级：门店(Store) → 仓库(Warehouse) → 货架(Shelf) → 单元(ShelfUnit)
// =============================================================================

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"warehouse/ent"
	"warehouse/internal/service"
	"warehouse/internal/utils"
	"warehouse/internal/ws"
)

// Handler HTTP 处理器集合
type Handler struct {
	Svc *service.WarehouseService
	// CommandHub 设备命令 WebSocket 通道（app 端连接 /ws 后由服务端下推命令）
	CommandHub *ws.Hub
}

// NewHandler 构造
func NewHandler(svc *service.WarehouseService, hub *ws.Hub) *Handler {
	return &Handler{Svc: svc, CommandHub: hub}
}

// 机器可读错误码：供 app / agent 程序化识别错误类型
const (
	ErrCodeBadRequest       = "BAD_REQUEST"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeInternal         = "INTERNAL"
	ErrCodeProductNotFound  = "PRODUCT_NOT_FOUND"
	ErrCodePackageNotFound  = "PACKAGE_NOT_FOUND"
	ErrCodeShelfNotFound    = "SHELF_NOT_FOUND"
	ErrCodeWarehouseMissing = "WAREHOUSE_NOT_FOUND"
	ErrCodeLandmarkNotFound = "LANDMARK_NOT_FOUND"
)

// codeForStatus 根据 HTTP 状态码给出默认错误码
func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return ErrCodeBadRequest
	case http.StatusNotFound:
		return ErrCodeNotFound
	case http.StatusInternalServerError:
		return ErrCodeInternal
	default:
		return "ERROR"
	}
}

// fail 统一错误响应，并把异常写入 c.Errors（供 RequestLogger 中间件打印）。
// 响应格式：{"error": "<人类可读消息>", "code": "<机器可读错误码>"}，
// 其中 error 保持字符串，向后兼容只读取 error 字段的调用方。
func (h *Handler) fail(c *gin.Context, status int, err error) {
	h.failWithCode(c, status, codeForStatus(status), err)
}

// failWithCode 与 fail 相同，但可指定业务错误码（如 ErrCodeProductNotFound）
func (h *Handler) failWithCode(c *gin.Context, status int, code string, err error) {
	c.Error(err)
	c.JSON(status, gin.H{"error": err.Error(), "code": code})
}

// =============================================================================
//  健康检查
// =============================================================================

// Ping Gin quick start 兼容
func (h *Handler) Ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "pong"})
}

// Health 详细健康检查
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "warehouse-api",
		"version": "1.1.0",
	})
}

// =============================================================================
//  辅助
// =============================================================================

// parseWarehouseID 解析路径中的仓库 ID
func (h *Handler) parseWarehouseID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("warehouse_id"))
	if err != nil || id <= 0 {
		h.fail(c, http.StatusBadRequest, errors.New("仓库ID必须是正整数"))
		return 0, false
	}
	return id, true
}

// defaultWarehouseID 获取默认仓库（用于旧接口兼容）
func (h *Handler) defaultWarehouseID(c *gin.Context) (int, bool) {
	wh, err := h.Svc.Repo.EnsureDefaultStoreWarehouse(c.Request.Context())
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return 0, false
	}
	return wh.ID, true
}

// =============================================================================
//  门店 API
// =============================================================================

// ListStores GET /api/stores
func (h *Handler) ListStores(c *gin.Context) {
	list, err := h.Svc.ListStores()
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "count": len(list)})
}

// CreateStoreRequest 创建门店请求体
type CreateStoreRequest struct {
	Name string `json:"name"`
	Code string `json:"code" binding:"required"`
}

// CreateStore POST /api/stores
func (h *Handler) CreateStore(c *gin.Context) {
	var req CreateStoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	store, err := h.Svc.CreateStore(req.Name, req.Code)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "已存在") {
			status = http.StatusConflict
		}
		h.fail(c, status, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": store})
}

// =============================================================================
//  仓库 API
// =============================================================================

// ListStoreWarehouses GET /api/stores/:store_id/warehouses
func (h *Handler) ListStoreWarehouses(c *gin.Context) {
	storeID, err := strconv.Atoi(c.Param("store_id"))
	if err != nil || storeID <= 0 {
		h.fail(c, http.StatusBadRequest, errors.New("门店ID必须是正整数"))
		return
	}
	list, err := h.Svc.ListStoreWarehouses(storeID)
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "count": len(list)})
}

// ListWarehouses GET /api/warehouses
func (h *Handler) ListWarehouses(c *gin.Context) {
	list, err := h.Svc.ListWarehouses()
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "count": len(list)})
}

// GetWarehouse GET /api/warehouses/:warehouse_id
func (h *Handler) GetWarehouse(c *gin.Context) {
	id, ok := h.parseWarehouseID(c)
	if !ok {
		return
	}
	wh, err := h.Svc.GetWarehouse(id)
	if err != nil {
		h.failWithCode(c, http.StatusNotFound, ErrCodeWarehouseMissing, fmt.Errorf("仓库%d不存在", id))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": wh})
}

// CreateWarehouseRequest 创建仓库请求体
type CreateWarehouseRequest struct {
	StoreID int    `json:"store_id"`
	Name    string `json:"name"`
	Code    string `json:"code" binding:"required"`
}

// CreateWarehouse POST /api/warehouses
// 兼容两种形式：路径 /api/stores/:store_id/warehouses 或 body 携带 store_id
func (h *Handler) CreateWarehouse(c *gin.Context) {
	var req CreateWarehouseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}

	storeID := req.StoreID
	if raw := c.Param("store_id"); raw != "" {
		if id, err := strconv.Atoi(raw); err == nil {
			storeID = id
		}
	}
	if storeID <= 0 {
		h.fail(c, http.StatusBadRequest, errors.New("缺少 store_id"))
		return
	}

	wh, err := h.Svc.CreateWarehouse(storeID, req.Name, req.Code)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "已存在") {
			status = http.StatusConflict
		}
		h.fail(c, status, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": wh})
}

// =============================================================================
//  货架 API（仓库维度）
// =============================================================================

// ListShelves GET /api/warehouses/:warehouse_id/shelves
func (h *Handler) ListShelves(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	list, err := h.Svc.ListShelves(whID)
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "count": len(list), "warehouse_id": whID})
}

// GetShelf GET /api/warehouses/:warehouse_id/shelves/:number
func (h *Handler) GetShelf(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	num, err := strconv.Atoi(c.Param("number"))
	if err != nil {
		h.fail(c, http.StatusBadRequest, errors.New("货架编号必须是整数"))
		return
	}
	shelf, err := h.Svc.GetShelf(whID, num)
	if err != nil {
		h.failWithCode(c, http.StatusNotFound, ErrCodeShelfNotFound, fmt.Errorf("仓库%d中货架%d不存在", whID, num))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": shelf})
}

// CreateShelfRequest 创建货架请求体
type CreateShelfRequest struct {
	ShelfNumber    int               `json:"shelf_number"   binding:"required"`
	Label          string            `json:"label"`
	Bold           bool              `json:"bold"`
	Col            int               `json:"col"            binding:"required"`
	Row            int               `json:"row"            binding:"required"`
	Width          int               `json:"width"          binding:"required,min=1"`
	Height         int               `json:"height"         binding:"required,min=1"`
	IsHorizontal   bool              `json:"is_horizontal"`
	IsVerticalLong bool              `json:"is_vertical_long"`
	UnitCount      int               `json:"unit_count"`
	Units          []CreateUnitInput `json:"units"`
}

// CreateUnitInput 单个单元请求体
type CreateUnitInput struct {
	UnitNumber   int     `json:"unit_number"  binding:"required,min=1"`
	Label        string  `json:"label"`
	PositionFrac float64 `json:"position_frac"`
	AccessCol    int     `json:"access_col"`
	AccessRow    int     `json:"access_row"`
	Capacity     int     `json:"capacity"`
}

// CreateShelf POST /api/warehouses/:warehouse_id/shelves
// 支持两种模式：
//  1. 提供 units 数组：按数组内容一次创建货架及其单元
//  2. 只提供 unit_count：自动计算单元坐标与位置比例
func (h *Handler) CreateShelf(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	var req CreateShelfRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	if len(req.Units) == 0 && req.UnitCount == 0 {
		h.fail(c, http.StatusBadRequest, errors.New("必须提供 units 数组或 unit_count"))
		return
	}

	in := service.CreateShelfInput{
		ShelfNumber:    req.ShelfNumber,
		Label:          req.Label,
		Bold:           req.Bold,
		Col:            req.Col,
		Row:            req.Row,
		Width:          req.Width,
		Height:         req.Height,
		IsHorizontal:   req.IsHorizontal,
		IsVerticalLong: req.IsVerticalLong,
		UnitCount:      req.UnitCount,
	}
	for _, u := range req.Units {
		cap := u.Capacity
		if cap <= 0 {
			cap = 10
		}
		in.Units = append(in.Units, service.CreateUnitInput{
			UnitNumber:   u.UnitNumber,
			Label:        u.Label,
			PositionFrac: u.PositionFrac,
			AccessCol:    u.AccessCol,
			AccessRow:    u.AccessRow,
			Capacity:     cap,
		})
	}

	shelf, err := h.Svc.CreateShelf(whID, in)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "已存在") {
			status = http.StatusConflict
		}
		h.fail(c, status, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": shelf})
}

// InitShelves POST /api/warehouses/:warehouse_id/shelves/init
func (h *Handler) InitShelves(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	if err := h.Svc.InitShelves(whID); err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "货架初始化完成", "warehouse_id": whID})
}

// =============================================================================
//  单元 API
// =============================================================================

// UpdateUnitCapacityRequest 更新单元容量请求体
type UpdateUnitCapacityRequest struct {
	Capacity int `json:"capacity" binding:"required,min=1"`
}

// UpdateUnitCapacity PUT /api/units/:id/capacity
func (h *Handler) UpdateUnitCapacity(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		h.fail(c, http.StatusBadRequest, errors.New("单元ID必须是正整数"))
		return
	}
	var req UpdateUnitCapacityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	unit, err := h.Svc.UpdateUnitCapacity(id, req.Capacity)
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": unit})
}

// AssignUnitProductRequest 单元绑定商品请求体
type AssignUnitProductRequest struct {
	ProductID int `json:"product_id"`
}

// AssignUnitProduct PUT /api/units/:id/product
// product_id > 0 绑定商品；= 0 / 缺省表示解绑
func (h *Handler) AssignUnitProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		h.fail(c, http.StatusBadRequest, errors.New("单元ID必须是正整数"))
		return
	}
	var req AssignUnitProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	unit, err := h.Svc.AssignUnitProduct(id, req.ProductID)
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": unit})
}

// =============================================================================
//  商品 API
// =============================================================================

// CreateProductRequest 创建商品请求体
type CreateProductRequest struct {
	Name   string `json:"name"`
	EAN13  string `json:"ean13"`
	EAN8   string `json:"ean8"`
	QRCode string `json:"qr_code"`
}

// CreateProduct POST /api/products
func (h *Handler) CreateProduct(c *gin.Context) {
	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	p, err := h.Svc.CreateProduct(req.Name, req.EAN13, req.EAN8, req.QRCode)
	if err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": p})
}

// ListProducts GET /api/products
func (h *Handler) ListProducts(c *gin.Context) {
	list, err := h.Svc.ListProducts()
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "count": len(list)})
}

// GetProduct GET /api/products/:id
func (h *Handler) GetProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		h.fail(c, http.StatusBadRequest, errors.New("商品ID必须是正整数"))
		return
	}
	p, err := h.Svc.GetProduct(id)
	if err != nil {
		h.failWithCode(c, http.StatusNotFound, ErrCodeProductNotFound, fmt.Errorf("商品%d不存在", id))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p})
}

// GetProductByBarcode GET /api/products/by-barcode/:barcode
// 按商品码（EAN-13 / EAN-8 / QR code 任一匹配）查询商品及其存储位置。
// 注意：该路由必须注册在 /api/products/:id 之前，否则 by-barcode 会被 :id 捕获。
func (h *Handler) GetProductByBarcode(c *gin.Context) {
	barcode := strings.TrimSpace(c.Param("barcode"))
	if barcode == "" {
		h.fail(c, http.StatusBadRequest, errors.New("商品码不能为空"))
		return
	}
	p, err := h.Svc.GetProductByBarcode(barcode)
	if err != nil {
		h.failWithCode(c, http.StatusNotFound, ErrCodeProductNotFound, fmt.Errorf("未找到商品码 %s 对应的商品", barcode))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p})
}

// UpdateProduct PUT /api/products/:id
func (h *Handler) UpdateProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		h.fail(c, http.StatusBadRequest, errors.New("商品ID必须是正整数"))
		return
	}
	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	p, err := h.Svc.UpdateProduct(id, req.Name, req.EAN13, req.EAN8, req.QRCode)
	if err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": p})
}

// DeleteProduct DELETE /api/products/:id
func (h *Handler) DeleteProduct(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		h.fail(c, http.StatusBadRequest, errors.New("商品ID必须是正整数"))
		return
	}
	if err := h.Svc.DeleteProduct(id); err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "商品已删除"})
}

// =============================================================================
//  位置点 API（仓库维度）
//  位置点(地标)不承载货架单元、不存放货物，只提供网格坐标：
//  入口 entrance / 出口 exit / 装卸位 dock / 充电桩 charger /
//  搬货人员 person / 机器 robot / 一般路径点 waypoint
// =============================================================================

// CreateLandmarkRequest 创建位置点请求体
// 注意：col/row 不加 binding:required —— (0,0) 是合法坐标（入口通常就在原点）。
type CreateLandmarkRequest struct {
	Code      string `json:"code" binding:"required"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Col       int    `json:"col"`
	Row       int    `json:"row"`
	IsMovable bool   `json:"is_movable"`
	Note      string `json:"note"`
}

// UpdateLandmarkRequest 更新位置点请求体；仅传入的字段会被更新（PATCH 语义），
// 便于只刷新人员/机器人的坐标。
type UpdateLandmarkRequest struct {
	Name      *string `json:"name"`
	Kind      *string `json:"kind"`
	Col       *int    `json:"col"`
	Row       *int    `json:"row"`
	IsMovable *bool   `json:"is_movable"`
	Note      *string `json:"note"`
}

// CreateLandmark POST /api/warehouses/:warehouse_id/landmarks
func (h *Handler) CreateLandmark(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	var req CreateLandmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	lm, err := h.Svc.CreateLandmark(whID, service.LandmarkInput{
		Code: req.Code, Name: req.Name, Kind: req.Kind,
		Col: req.Col, Row: req.Row, IsMovable: req.IsMovable, Note: req.Note,
	})
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "已存在") {
			status = http.StatusConflict
		}
		h.fail(c, status, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": lm})
}

// ListLandmarks GET /api/warehouses/:warehouse_id/landmarks[?kind=entrance]
func (h *Handler) ListLandmarks(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	list, err := h.Svc.ListLandmarks(whID, c.Query("kind"))
	if err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "count": len(list), "warehouse_id": whID})
}

// GetLandmark GET /api/warehouses/:warehouse_id/landmarks/:code
func (h *Handler) GetLandmark(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	code := c.Param("code")
	lm, err := h.Svc.GetLandmark(whID, code)
	if err != nil {
		h.failWithCode(c, http.StatusNotFound, ErrCodeLandmarkNotFound,
			fmt.Errorf("仓库%d中位置%s不存在", whID, code))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": lm})
}

// UpdateLandmark PATCH /api/warehouses/:warehouse_id/landmarks/:code
func (h *Handler) UpdateLandmark(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	code := c.Param("code")
	var req UpdateLandmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	lm, err := h.Svc.PatchLandmark(whID, code, service.LandmarkPatch{
		Name: req.Name, Kind: req.Kind,
		Col: req.Col, Row: req.Row, IsMovable: req.IsMovable, Note: req.Note,
	})
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "不存在") {
			status = http.StatusNotFound
		}
		h.fail(c, status, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": lm})
}

// DeleteLandmark DELETE /api/warehouses/:warehouse_id/landmarks/:code
func (h *Handler) DeleteLandmark(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	code := c.Param("code")
	if err := h.Svc.DeleteLandmark(whID, code); err != nil {
		h.failWithCode(c, http.StatusNotFound, ErrCodeLandmarkNotFound, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "位置已删除", "code": code})
}

// =============================================================================
//  执行设备位置 API（仓库维度）
//  复用位置点存储：设备编码即位置点 code，kind 缺省 robot，is_movable 恒为 true。
//  一个用户只允许在一台设备上登录，因此无需独立的设备表。
// =============================================================================

// DevicePositionRequest 执行设备位置上报请求体
// 注意：col/row 不加 binding:required —— (0,0) 是合法坐标。
type DevicePositionRequest struct {
	Col  int    `json:"col"`
	Row  int    `json:"row"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Note string `json:"note"`
}

// UpsertDevicePosition PUT /api/warehouses/:warehouse_id/devices/:device_id/position
// 设备位置上报：位置点不存在则创建，存在则刷新坐标。例：{"col":5,"row":10}
func (h *Handler) UpsertDevicePosition(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	deviceID := strings.TrimSpace(c.Param("device_id"))
	if deviceID == "" {
		h.fail(c, http.StatusBadRequest, errors.New("设备编码不能为空"))
		return
	}
	var req DevicePositionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	lm, err := h.Svc.UpsertDevicePosition(whID, service.DevicePositionInput{
		DeviceID: deviceID,
		Name:     req.Name,
		Kind:     req.Kind,
		Col:      req.Col,
		Row:      req.Row,
		Note:     req.Note,
	})
	if err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": lm, "warehouse_id": whID})
}

// =============================================================================
//  包裹 API（仓库维度）
// =============================================================================

// CreatePackageRequest 创建包裹请求体。定位方式二选一：
//  1. 提供 barcode：按商品码找到该商品在本仓库的存放单元，落位并关联商品；
//  2. 同时提供 shelf_num 与 unit_num：直接指定目标货架与单元。
//
// 注意：shelf_num/unit_num 不加 binding:required —— 两种模式互斥，
// 且坐标/编号 0 值在 gin 的 required 校验下会被误判为“未提供”。
type CreatePackageRequest struct {
	PackageID string `json:"package_id" binding:"required"`
	Barcode   string `json:"barcode"`
	ShelfNum  int    `json:"shelf_num"`
	UnitNum   int    `json:"unit_num"`
	Priority  int    `json:"priority"`
	Note      string `json:"note"`
}

// CreatePackage POST /api/warehouses/:warehouse_id/packages
func (h *Handler) CreatePackage(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	var req CreatePackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Barcode) == "" && (req.ShelfNum <= 0 || req.UnitNum <= 0) {
		h.fail(c, http.StatusBadRequest, errors.New("必须提供 barcode，或同时提供 shelf_num 与 unit_num"))
		return
	}

	pkg, err := h.Svc.CreatePackage(whID, service.CreatePackageInput{
		PackageID: req.PackageID,
		ShelfNum:  req.ShelfNum,
		UnitNum:   req.UnitNum,
		Priority:  req.Priority,
		Note:      req.Note,
		Barcode:   req.Barcode,
	})
	if err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": pkg})
}

// CreatePackageFromBarcodeRequest 扫码建包请求体。
// device_id 与 col/row 可选：同时提供时刷新该执行设备的当前位置（复用位置点），
// 作为后续路径规划的默认起点；不提供时不影响建包。
type CreatePackageFromBarcodeRequest struct {
	PackageID  string `json:"package_id" binding:"required"`
	Barcode    string `json:"barcode"    binding:"required"`
	Priority   int    `json:"priority"`
	Note       string `json:"note"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Col        *int   `json:"col"`
	Row        *int   `json:"row"`
}

// CreatePackageFromBarcode POST /api/warehouses/:warehouse_id/packages/from-barcode
// 商品码 → 商品 → 该商品在本仓库绑定的存放单元 → 在该货架单元上创建包裹并关联商品。
// 例：{"package_id":"PKG-COLA-001","barcode":"6901234567892"}
func (h *Handler) CreatePackageFromBarcode(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	var req CreatePackageFromBarcodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}
	pkg, err := h.Svc.CreatePackageFromBarcode(whID, req.PackageID, req.Barcode, req.Priority, req.Note)
	if err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}

	// 扫码时的执行设备位置：可选，刷新失败不阻断建包
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID != "" && req.Col != nil && req.Row != nil {
		if _, perr := h.Svc.UpsertDevicePosition(whID, service.DevicePositionInput{
			DeviceID: deviceID,
			Name:     req.DeviceName,
			Col:      *req.Col,
			Row:      *req.Row,
		}); perr != nil {
			slog.Warn("刷新执行设备位置失败",
				"warehouse_id", whID, "device_id", deviceID, "err", perr)
		}
	}

	resp := gin.H{"data": pkg}
	if deviceID != "" {
		if wp, ok := h.Svc.DevicePosition(whID, deviceID); ok {
			resp["device_position"] = gin.H{
				"device_id": deviceID,
				"col":       wp.Point.X,
				"row":       wp.Point.Y,
				"label":     wp.Label,
			}
		}
	}
	c.JSON(http.StatusCreated, resp)
}

// ListPendingPackages GET /api/warehouses/:warehouse_id/packages/pending
func (h *Handler) ListPendingPackages(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	list, err := h.Svc.ListPendingPackages(whID)
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": list, "count": len(list), "warehouse_id": whID})
}

// GetPackage GET /api/packages/:id
func (h *Handler) GetPackage(c *gin.Context) {
	pid := c.Param("id")
	pkg, err := h.Svc.Repo.Packages.GetByPackageID(c.Request.Context(), pid)
	if err != nil {
		h.failWithCode(c, http.StatusNotFound, ErrCodePackageNotFound, fmt.Errorf("包裹%s不存在", pid))
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": pkg})
}

// UpdatePackageStatus PUT /api/packages/:id/status
type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=pending placed delivered"`
}

func (h *Handler) UpdatePackageStatus(c *gin.Context) {
	pid := c.Param("id")
	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}

	pkg, err := h.Svc.Repo.Packages.GetByPackageID(c.Request.Context(), pid)
	if err != nil {
		h.failWithCode(c, http.StatusNotFound, ErrCodePackageNotFound, fmt.Errorf("包裹%s不存在", pid))
		return
	}
	if err := h.Svc.Repo.Packages.UpdateStatus(c.Request.Context(), pkg.ID, req.Status); err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "状态已更新", "status": req.Status})
}

// =============================================================================
//  路径规划 API（仓库维度）
// =============================================================================

// PlanPathRequest 路径规划请求体。
// start/end 可传网格坐标 {"col":0,"row":0}，也可传位置点编码 {"landmark":"ENTRANCE"}。
// device_id 为执行设备编码：未显式传 start 时，取该设备最近一次上报的位置作为起点。
type PlanPathRequest struct {
	Tasks    []TaskInput     `json:"tasks" binding:"required,min=1,max=10"`
	Start    *GridPointInput `json:"start"`
	End      *GridPointInput `json:"end"`
	DeviceID string          `json:"device_id"`
}

// TaskInput 单个任务
type TaskInput struct {
	PackageID string `json:"package_id" binding:"required"`
	ShelfNum  int    `json:"shelf_num"   binding:"required"`
	UnitNum   int    `json:"unit_num"    binding:"required"`
}

// PlanPath POST /api/warehouses/:warehouse_id/path/plan
func (h *Handler) PlanPath(c *gin.Context) {
	whID, ok := h.resolveWarehouse(c)
	if !ok {
		return
	}
	var req PlanPathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}

	// 转换为 service 层结构
	labels := []string{"T1", "T2", "T3", "T4", "T5", "T6", "T7", "T8", "T9", "T10"}
	tasks := make([]service.PackageTask, len(req.Tasks))
	for i, t := range req.Tasks {
		desc := fmt.Sprintf("%d号·%d单元", t.ShelfNum, t.UnitNum)
		tasks[i] = service.PackageTask{
			PackageID: t.PackageID,
			ShelfNum:  t.ShelfNum,
			UnitNum:   t.UnitNum,
			Label:     labels[i],
			Desc:      desc,
		}
	}

	// 起点优先级：显式 start > 执行设备当前位置 > 仓库入口 > 原点；终点支持网格坐标或位置点编码
	start, err := h.resolveStartWaypoint(whID, req.Start, req.DeviceID)
	if err != nil {
		h.failWaypoint(c, err)
		return
	}
	end, err := h.resolveOptionalWaypoint(whID, req.End)
	if err != nil {
		h.failWaypoint(c, err)
		return
	}

	result, err := h.Svc.PlanPath(whID, tasks, start, end)
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}

	// 构建响应
	orderLabels := make([]string, len(result.Order))
	for i, idx := range result.Order {
		orderLabels[i] = tasks[idx].Label
	}

	segs := make([]gin.H, len(result.Segments))
	for i, s := range result.Segments {
		segs[i] = gin.H{
			"from":     s.FromLabel,
			"to":       s.ToLabel,
			"to_desc":  s.ToDesc,
			"distance": s.Distance,
		}
	}

	resp := gin.H{
		"start":          gin.H{"col": result.StartCol, "row": result.StartRow, "label": result.StartLabel},
		"order":          orderLabels,
		"segments":       segs,
		"total_distance": result.TotalDist,
		"optimal":        true,
		"warehouse_id":   whID,
	}
	if result.EndLabel != "" {
		resp["end"] = gin.H{"col": result.EndCol, "row": result.EndRow, "label": result.EndLabel}
	}
	c.JSON(http.StatusOK, resp)
}

// =============================================================================
//  路径规划 API（app 端约定格式 POST /api/route/plan）
//  请求体：{"tasks": ["T1","T2"], "warehouse_id": 1, "start": {"col":0,"row":0}}
//  tasks 兼容字符串（任务编号/包裹编号/条码）与旧对象 {"package_id","shelf_num","unit_num"}
// =============================================================================

// ErrTaskUnresolved 任务无法解析到货架/单元（客户端输入问题）
var ErrTaskUnresolved = errors.New("任务无法解析")

// RouteTaskInput 单个规划任务：兼容字符串与对象两种写法
type RouteTaskInput struct {
	Label     string // 原始任务编号（如 T3 / PKG-001 / 条码）
	PackageID string
	ShelfNum  int
	UnitNum   int
	Direct    bool // true 表示对象形式已直接给出货架/单元
}

// UnmarshalJSON 同时支持 "T3" 与 {"package_id":"..","shelf_num":6,"unit_num":3}
func (t *RouteTaskInput) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		t.Label = strings.TrimSpace(s)
		return nil
	}
	var obj struct {
		PackageID string `json:"package_id"`
		ShelfNum  int    `json:"shelf_num"`
		UnitNum   int    `json:"unit_num"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	t.PackageID = obj.PackageID
	t.ShelfNum = obj.ShelfNum
	t.UnitNum = obj.UnitNum
	t.Direct = obj.ShelfNum > 0 && obj.UnitNum > 0
	return nil
}

// RoutePlanRequest 路径规划请求体（app 约定格式）
// start/end 可为网格坐标，也可为位置点编码（如 "ENTRANCE" / {"landmark":"EXIT"}）。
// device_id 为执行设备编码：未显式传 start 时，取该设备最近一次上报的位置作为起点。
type RoutePlanRequest struct {
	Tasks       []RouteTaskInput `json:"tasks" binding:"required,min=1,max=10"`
	WarehouseID int              `json:"warehouse_id"`
	Start       *GridPointInput  `json:"start"`
	End         *GridPointInput  `json:"end"`
	DeviceID    string           `json:"device_id"`
}

// GridPointInput 起止位置（与货架布局坐标系一致），兼容三种写法：
//  1. 网格坐标：{"col": 3, "row": 5}
//  2. 带位置点编码的对象：{"landmark": "ENTRANCE"}
//  3. 位置点编码字符串："ENTRANCE"
type GridPointInput struct {
	Col      int    `json:"col"`
	Row      int    `json:"row"`
	Landmark string `json:"landmark"`
}

// UnmarshalJSON 同时支持坐标对象、带 landmark 的对象与纯字符串写法
func (p *GridPointInput) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		p.Landmark = strings.TrimSpace(s)
		return nil
	}
	var obj struct {
		Col      int    `json:"col"`
		Row      int    `json:"row"`
		Landmark string `json:"landmark"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	p.Col, p.Row, p.Landmark = obj.Col, obj.Row, obj.Landmark
	return nil
}

// RoutePlan POST /api/route/plan
func (h *Handler) RoutePlan(c *gin.Context) {
	var req RoutePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}

	whID := req.WarehouseID
	if whID <= 0 {
		var ok bool
		whID, ok = h.defaultWarehouseID(c)
		if !ok {
			return
		}
	}

	tasks, err := h.resolveRouteTasks(whID, req.Tasks)
	if err != nil {
		status := http.StatusBadRequest
		if !errors.Is(err, ErrTaskUnresolved) {
			status = http.StatusInternalServerError
		}
		h.fail(c, status, err)
		return
	}

	// 起点优先级：显式 start > 执行设备当前位置 > 仓库入口 > 原点
	start, err := h.resolveStartWaypoint(whID, req.Start, req.DeviceID)
	if err != nil {
		h.failWaypoint(c, err)
		return
	}
	end, err := h.resolveOptionalWaypoint(whID, req.End)
	if err != nil {
		h.failWaypoint(c, err)
		return
	}

	result, err := h.Svc.PlanPath(whID, tasks, start, end)
	if err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}

	// 构建响应：order 与 segments 均使用客户端传入的原始任务编号
	orderLabels := make([]string, len(result.Order))
	for i, idx := range result.Order {
		orderLabels[i] = tasks[idx].Label
	}

	segs := make([]gin.H, len(result.Segments))
	for i, s := range result.Segments {
		segs[i] = gin.H{
			"from":     s.FromLabel,
			"to":       s.ToLabel,
			"to_desc":  s.ToDesc,
			"distance": s.Distance,
		}
	}

	resp := gin.H{
		"start":          gin.H{"col": result.StartCol, "row": result.StartRow, "label": result.StartLabel},
		"order":          orderLabels,
		"segments":       segs,
		"total_distance": result.TotalDist,
		"optimal":        true,
		"warehouse_id":   whID,
	}
	if result.EndLabel != "" {
		resp["end"] = gin.H{"col": result.EndCol, "row": result.EndRow, "label": result.EndLabel}
	}
	c.JSON(http.StatusOK, resp)
}

// resolveRouteTasks 把客户端任务解析为货架/单元坐标：
//  1. 对象格式：直接使用给出的 shelf_num / unit_num
//  2. 字符串格式：包裹编号精确匹配 → T<序号>（待拣包裹按优先级排序取第 N 个） → 条码（商品所在单元）
func (h *Handler) resolveRouteTasks(whID int, inputs []RouteTaskInput) ([]service.PackageTask, error) {
	pending, err := h.Svc.ListPendingPackages(whID)
	if err != nil {
		return nil, err
	}

	tasks := make([]service.PackageTask, len(inputs))
	for i, in := range inputs {
		label := in.Label
		if label == "" {
			label = in.PackageID
		}
		if label == "" {
			label = fmt.Sprintf("T%d", i+1)
		}

		shelfNum, unitNum := in.ShelfNum, in.UnitNum
		if !in.Direct {
			shelfNum, unitNum = h.lookupTaskLocation(label, pending)
		}

		if shelfNum <= 0 || unitNum <= 0 {
			return nil, fmt.Errorf("%w: 任务 %q 在仓库 %d 中未找到对应的待拣包裹或货架单元", ErrTaskUnresolved, label, whID)
		}

		tasks[i] = service.PackageTask{
			PackageID: label,
			ShelfNum:  shelfNum,
			UnitNum:   unitNum,
			Label:     label,
			Desc:      fmt.Sprintf("%d号·%d单元", shelfNum, unitNum),
		}
	}
	return tasks, nil
}

// lookupTaskLocation 依次尝试：包裹编号精确匹配 → T<序号> 映射待拣包裹 → 条码查商品
func (h *Handler) lookupTaskLocation(label string, pending []*ent.GoodsPackage) (int, int) {
	// 1. 包裹编号精确匹配（如 PKG-001）
	for _, p := range pending {
		if p.PackageID == label && p.Edges.Shelf != nil && p.Edges.Unit != nil {
			return p.Edges.Shelf.ShelfNumber, p.Edges.Unit.UnitNumber
		}
	}

	// 2. T<序号>：按优先级排序后的第 N 个待拣包裹
	if n, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(label), "T")); err == nil && n >= 1 && n <= len(pending) {
		p := pending[n-1]
		if p.Edges.Shelf != nil && p.Edges.Unit != nil {
			return p.Edges.Shelf.ShelfNumber, p.Edges.Unit.UnitNumber
		}
	}

	// 3. 条码：商品首个存放单元
	if prod, err := h.Svc.GetProductByBarcode(label); err == nil && len(prod.Edges.Units) > 0 {
		u := prod.Edges.Units[0]
		if u.Edges.Shelf != nil {
			return u.Edges.Shelf.ShelfNumber, u.UnitNumber
		}
	}

	return 0, 0
}

// =============================================================================
//  设备命令 WebSocket 通道
//  app 端连接 GET /ws；服务端通过 POST /api/commands/push 下推命令（如 open_camera）
// =============================================================================

// HandleWS GET /ws — 设备命令 WebSocket 通道
func (h *Handler) HandleWS(c *gin.Context) {
	h.CommandHub.HandleWS(c)
}

// PushCommandRequest 命令推送请求体
type PushCommandRequest struct {
	Command string         `json:"command" binding:"required"`
	Params  map[string]any `json:"params"`
}

// PushCommand POST /api/commands/push
// 向所有已连接设备端推送命令，返回送达数量与在线连接数
func (h *Handler) PushCommand(c *gin.Context) {
	var req PushCommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.fail(c, http.StatusBadRequest, err)
		return
	}

	delivered := h.CommandHub.BroadcastCommand(req.Command, req.Params)
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"command":   req.Command,
			"delivered": delivered,
			"connected": h.CommandHub.Count(),
		},
	})
}

// =============================================================================
//  场景初始化 API
// =============================================================================

// InitDemo POST /api/demo/init
func (h *Handler) InitDemo(c *gin.Context) {
	whID, ok := h.defaultWarehouseID(c)
	if !ok {
		return
	}
	if err := h.Svc.InitShelves(whID); err != nil {
		h.fail(c, http.StatusInternalServerError, err)
		return
	}

	demos := []struct {
		pid      string
		shelfNum int
		unitNum  int
		priority int
	}{
		{"PKG-001", 2, 4, 1},
		{"PKG-002", 6, 3, 2},
		{"PKG-003", 11, 5, 3},
	}
	for _, d := range demos {
		if _, err := h.Svc.CreatePackage(whID, service.CreatePackageInput{
			PackageID: d.pid,
			ShelfNum:  d.shelfNum,
			UnitNum:   d.unitNum,
			Priority:  d.priority,
		}); err != nil {
			continue // 已存在则忽略
		}
	}

	// 可乐：商品主数据 + 绑定存放单元（3号货架·2单元，幂等）
	const colaBarcode = "6901234567892"
	cola, err := h.Svc.GetProductByBarcode(colaBarcode)
	if err != nil {
		cola, err = h.Svc.CreateProduct("可乐", colaBarcode, "", "")
	}
	if err == nil && cola != nil {
		if sh, serr := h.Svc.GetShelf(whID, 3); serr == nil {
			if u, uerr := h.Svc.Repo.Units.GetByShelfAndNumber(context.Background(), sh.ID, 2); uerr == nil {
				_, _ = h.Svc.AssignUnitProduct(u.ID, cola.ID)
			}
		}
	}

	// 可乐包裹：按商品码扫码建包（幂等，已存在或商品未绑定单元则忽略）
	if _, err := h.Svc.CreatePackage(whID, service.CreatePackageInput{
		PackageID: "PKG-COLA-001",
		Barcode:   colaBarcode,
		Priority:  1,
	}); err != nil {
		// 已存在则忽略
	}

	// 默认位置点：入口/出口（幂等，已存在则忽略）
	landmarks := []service.LandmarkInput{
		{Code: "ENTRANCE", Name: "入口", Kind: "entrance", Col: 0, Row: 0},
		{Code: "EXIT", Name: "出口", Kind: "exit", Col: 0, Row: 28},
	}
	for _, lm := range landmarks {
		if _, err := h.Svc.CreateLandmark(whID, lm); err != nil {
			continue
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "演示数据初始化完成",
		"packages":     []string{"PKG-001(2号·4)", "PKG-002(6号·3)", "PKG-003(11号·5)", "PKG-COLA-001(3号·2·可乐)"},
		"products":     []string{"可乐(6901234567892)"},
		"landmarks":    []string{"ENTRANCE(0,0)", "EXIT(0,28)"},
		"warehouse_id": whID,
		"hint": `POST /api/warehouses/:id/path/plan 获取最优路径；起止点可传 "ENTRANCE" / "EXIT"。` +
			`执行设备位置用 PUT /api/warehouses/:id/devices/:device_id/position 上报（复用位置点存储）；` +
			`规划时带 "device_id" 即以该设备位置为起点，未上报过位置则回退到入口`,
	})
}

// resolveWarehouse 优先取路径参数 warehouse_id；旧接口兼容时取默认仓库
func (h *Handler) resolveWarehouse(c *gin.Context) (int, bool) {
	if raw := c.Param("warehouse_id"); raw != "" {
		return h.parseWarehouseID(c)
	}
	return h.defaultWarehouseID(c)
}

// ErrLandmarkNotFound 起止位置编码不存在（客户端输入问题）
var ErrLandmarkNotFound = errors.New("位置不存在")

// resolveWaypoint 把客户端传入的网格坐标或位置点编码解析为路径点。
// in 为 nil 时返回默认坐标 (0,0)，即仓库入口。
func (h *Handler) resolveWaypoint(whID int, in *GridPointInput, defLabel string) (service.Waypoint, error) {
	wp := service.Waypoint{Point: utils.Point{X: 0, Y: 0}, Label: defLabel}
	if in == nil {
		return wp, nil
	}
	if code := strings.TrimSpace(in.Landmark); code != "" {
		resolved, err := h.Svc.ResolveLandmark(whID, code)
		if err != nil {
			return wp, fmt.Errorf("%w: %v", ErrLandmarkNotFound, err)
		}
		if resolved.Label == "" {
			resolved.Label = code
		}
		return resolved, nil
	}
	wp.Point = utils.Point{X: in.Col, Y: in.Row}
	return wp, nil
}

// resolveStartWaypoint 解析路径起点，优先级从高到低：
//  1. 客户端显式指定的 start（网格坐标或位置点编码）
//  2. 执行设备（device_id）最近一次上报的位置（复用位置点，kind=robot/person）
//  3. 仓库入口位置点（kind=entrance）
//  4. 兜底原点 (0,0)
func (h *Handler) resolveStartWaypoint(whID int, in *GridPointInput, deviceID string) (service.Waypoint, error) {
	if in != nil {
		return h.resolveWaypoint(whID, in, "起点")
	}
	if id := strings.TrimSpace(deviceID); id != "" {
		if wp, ok := h.Svc.DevicePosition(whID, id); ok {
			if wp.Label == "" {
				wp.Label = id
			}
			return wp, nil
		}
	}
	if wp, ok := h.Svc.EntranceWaypoint(whID); ok {
		return wp, nil
	}
	return service.Waypoint{Point: utils.Point{X: 0, Y: 0}, Label: "起点"}, nil
}

// resolveOptionalWaypoint 与 resolveWaypoint 一致，但 nil 输入返回 nil（表示不指定终点）
func (h *Handler) resolveOptionalWaypoint(whID int, in *GridPointInput) (*service.Waypoint, error) {
	if in == nil {
		return nil, nil
	}
	wp, err := h.resolveWaypoint(whID, in, "终点")
	if err != nil {
		return nil, err
	}
	return &wp, nil
}

// failWaypoint 起止点解析失败时统一响应：位置编码不存在 → 404，其余 → 400
func (h *Handler) failWaypoint(c *gin.Context, err error) {
	if errors.Is(err, ErrLandmarkNotFound) {
		h.failWithCode(c, http.StatusNotFound, ErrCodeLandmarkNotFound, err)
		return
	}
	h.fail(c, http.StatusBadRequest, err)
}
