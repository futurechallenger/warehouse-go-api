// Package service 业务逻辑层
package service

// =============================================================================
//  internal/service — 业务逻辑层
//  领域层级：门店(Store) → 仓库(Warehouse) → 货架(Shelf) → 单元(ShelfUnit)
// =============================================================================

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"warehouse/ent"
	"warehouse/ent/shelf"
	"warehouse/ent/store"
	"warehouse/internal/repository"
	"warehouse/internal/utils"
)

// WarehouseService 仓库业务服务
type WarehouseService struct {
	Repo *repository.Repository
}

// NewWarehouseService 构造服务
func NewWarehouseService(repo *repository.Repository) *WarehouseService {
	return &WarehouseService{Repo: repo}
}

// ========================== 门店 ==========================

// CreateStore 创建门店
func (s *WarehouseService) CreateStore(name, code string) (*ent.Store, error) {
	ctx := context.Background()
	exist, err := s.Repo.Client.Store.Query().
		Where(store.CodeEQ(code)).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exist {
		return nil, fmt.Errorf("门店编码%s已存在", code)
	}
	return s.Repo.Stores.Create(ctx, name, code)
}

// ListStores 列出所有门店
func (s *WarehouseService) ListStores() ([]*ent.Store, error) {
	return s.Repo.Stores.List(context.Background())
}

// GetStore 按 ID 获取门店
func (s *WarehouseService) GetStore(id int) (*ent.Store, error) {
	return s.Repo.Stores.GetByID(context.Background(), id)
}

// ========================== 仓库 ==========================

// CreateWarehouse 在门店下创建仓库
func (s *WarehouseService) CreateWarehouse(storeID int, name, code string) (*ent.Warehouse, error) {
	ctx := context.Background()
	exist, err := s.Repo.Warehouses.GetByStoreAndCode(ctx, storeID, code)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if exist != nil {
		return nil, fmt.Errorf("该门店下仓库编码%s已存在", code)
	}
	return s.Repo.Warehouses.Create(ctx, storeID, name, code)
}

// ListWarehouses 列出所有仓库
func (s *WarehouseService) ListWarehouses() ([]*ent.Warehouse, error) {
	return s.Repo.Warehouses.ListAll(context.Background())
}

// ListStoreWarehouses 列出门店下的仓库
func (s *WarehouseService) ListStoreWarehouses(storeID int) ([]*ent.Warehouse, error) {
	return s.Repo.Warehouses.ListByStore(context.Background(), storeID)
}

// GetWarehouse 按 ID 获取仓库
func (s *WarehouseService) GetWarehouse(id int) (*ent.Warehouse, error) {
	return s.Repo.Warehouses.GetByID(context.Background(), id)
}

// ========================== 货架 ==========================

// InitShelves 初始化指定仓库下的演示货架与单元
func (s *WarehouseService) InitShelves(whID int) error {
	ctx := context.Background()
	for _, d := range shelfDefs() {
		// 若该仓库内已存在则跳过
		exist, err := s.Repo.Client.Shelf.Query().
			Where(shelf.WarehouseID(whID), shelf.ShelfNumberEQ(d.Num)).
			Exist(ctx)
		if err != nil {
			return err
		}
		if exist {
			continue
		}

		var label string
		switch {
		case d.IsH:
			label = strconv.Itoa(d.Num) + " (×2)"
		case d.IsVL:
			label = strconv.Itoa(d.Num) + " (×3)"
		default:
			label = strconv.Itoa(d.Num)
		}

		sh, err := s.Repo.Client.Shelf.Create().
			SetWarehouseID(whID).
			SetShelfNumber(d.Num).
			SetLabel(label).
			SetBold(d.Bold).
			SetCol(d.Col).
			SetRow(d.Row).
			SetWidth(d.W).
			SetHeight(d.H).
			SetIsHorizontal(d.IsH).
			SetIsVerticalLong(d.IsVL).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create shelf %d: %w", d.Num, err)
		}

		for u := 1; u <= d.UnitCount; u++ {
			col, row := utils.CalcAccessCell(d.Col, d.Row, d.W, d.H, u, d.UnitCount, d.IsH)
			frac := utils.UnitFrac(u, d.UnitCount)
			if _, err := s.Repo.Client.ShelfUnit.Create().
				SetShelf(sh).
				SetUnitNumber(u).
				SetLabel(strconv.Itoa(d.Num) + "-" + strconv.Itoa(u)).
				SetPositionFrac(frac).
				SetAccessCol(col).
				SetAccessRow(row).
				SetCapacity(10).
				Save(ctx); err != nil {
				return fmt.Errorf("create unit %d-%d: %w", d.Num, u, err)
			}
		}
	}
	return nil
}

// CreateShelfInput 创建货架输入
type CreateShelfInput struct {
	ShelfNumber    int
	Label          string
	Bold           bool
	Col            int
	Row            int
	Width          int
	Height         int
	IsHorizontal   bool
	IsVerticalLong bool
	UnitCount      int
	Units          []CreateUnitInput
}

// CreateUnitInput 单个单元输入
type CreateUnitInput struct {
	UnitNumber   int
	Label        string
	PositionFrac float64
	AccessCol    int
	AccessRow    int
	Capacity     int
}

// CreateShelf 在指定仓库下创建货架。提供 units 数组则按数组创建单元；
// 否则按 UnitCount 自动计算单元坐标与位置比例。
func (s *WarehouseService) CreateShelf(whID int, in CreateShelfInput) (*ent.Shelf, error) {
	ctx := context.Background()

	// 仓库内编号冲突检查
	exist, err := s.Repo.Client.Shelf.Query().
		Where(shelf.WarehouseID(whID), shelf.ShelfNumberEQ(in.ShelfNumber)).
		Exist(ctx)
	if err != nil {
		return nil, err
	}
	if exist {
		return nil, fmt.Errorf("仓库%d中货架%d已存在", whID, in.ShelfNumber)
	}

	// 构建单元列表
	var units []*ent.ShelfUnit
	if len(in.Units) > 0 {
		for _, u := range in.Units {
			units = append(units, &ent.ShelfUnit{
				UnitNumber:   u.UnitNumber,
				Label:        u.Label,
				PositionFrac: u.PositionFrac,
				AccessCol:    u.AccessCol,
				AccessRow:    u.AccessRow,
				Capacity:     u.Capacity,
			})
		}
	} else if in.UnitCount > 0 {
		for u := 1; u <= in.UnitCount; u++ {
			col, row := utils.CalcAccessCell(in.Col, in.Row, in.Width, in.Height, u, in.UnitCount, in.IsHorizontal)
			frac := utils.UnitFrac(u, in.UnitCount)
			units = append(units, &ent.ShelfUnit{
				UnitNumber:   u,
				Label:        fmt.Sprintf("%d-%d", in.ShelfNumber, u),
				PositionFrac: frac,
				AccessCol:    col,
				AccessRow:    row,
				Capacity:     10,
			})
		}
	}

	sh := &ent.Shelf{
		ShelfNumber:    in.ShelfNumber,
		Label:          in.Label,
		Bold:           in.Bold,
		Col:            in.Col,
		Row:            in.Row,
		Width:          in.Width,
		Height:         in.Height,
		IsHorizontal:   in.IsHorizontal,
		IsVerticalLong: in.IsVerticalLong,
	}
	return s.Repo.Shelves.CreateWithUnits(ctx, whID, sh, units)
}

// ListShelves 列出仓库下所有货架
func (s *WarehouseService) ListShelves(whID int) ([]*ent.Shelf, error) {
	return s.Repo.Shelves.ListByWarehouse(context.Background(), whID)
}

// GetShelf 按仓库与编号取货架
func (s *WarehouseService) GetShelf(whID, num int) (*ent.Shelf, error) {
	return s.Repo.Shelves.GetByWarehouseAndNumber(context.Background(), whID, num)
}

// ========================== 单元 ==========================

// UpdateUnitCapacity 更新单元存储数量
func (s *WarehouseService) UpdateUnitCapacity(unitID, capacity int) (*ent.ShelfUnit, error) {
	return s.Repo.Units.UpdateCapacity(context.Background(), unitID, capacity)
}

// AssignUnitProduct 绑定单元存放的商品；productID<=0 表示解绑
func (s *WarehouseService) AssignUnitProduct(unitID, productID int) (*ent.ShelfUnit, error) {
	if productID > 0 {
		if _, err := s.Repo.Products.GetByID(context.Background(), productID); err != nil {
			return nil, fmt.Errorf("商品%d不存在", productID)
		}
	}
	return s.Repo.Units.AssignProduct(context.Background(), unitID, productID)
}

// ========================== 商品 ==========================

// CreateProduct 创建商品，要求 EAN-13 / EAN-8 / QR code 至少提供一种
func (s *WarehouseService) CreateProduct(name, ean13, ean8, qrCode string) (*ent.Product, error) {
	if strings.TrimSpace(ean13) == "" && strings.TrimSpace(ean8) == "" && strings.TrimSpace(qrCode) == "" {
		return nil, errors.New("EAN-13、EAN-8、QR code 至少提供一种")
	}
	p := &ent.Product{Name: name, Ean13: ean13, Ean8: ean8, QrCode: qrCode}
	return s.Repo.Products.Create(context.Background(), p)
}

// ListProducts 列出所有商品
func (s *WarehouseService) ListProducts() ([]*ent.Product, error) {
	return s.Repo.Products.List(context.Background())
}

// GetProduct 按 ID 获取商品
func (s *WarehouseService) GetProduct(id int) (*ent.Product, error) {
	return s.Repo.Products.GetByID(context.Background(), id)
}

// GetProductByBarcode 按商品码（EAN-13 / EAN-8 / QR code）查询商品及其存储位置
func (s *WarehouseService) GetProductByBarcode(barcode string) (*ent.Product, error) {
	return s.Repo.Products.GetByBarcode(context.Background(), barcode)
}

// UpdateProduct 更新商品信息
func (s *WarehouseService) UpdateProduct(id int, name, ean13, ean8, qrCode string) (*ent.Product, error) {
	if strings.TrimSpace(ean13) == "" && strings.TrimSpace(ean8) == "" && strings.TrimSpace(qrCode) == "" {
		return nil, errors.New("EAN-13、EAN-8、QR code 至少提供一种")
	}
	p := &ent.Product{Name: name, Ean13: ean13, Ean8: ean8, QrCode: qrCode}
	return s.Repo.Products.Update(context.Background(), id, p)
}

// DeleteProduct 删除商品
func (s *WarehouseService) DeleteProduct(id int) error {
	return s.Repo.Products.Delete(context.Background(), id)
}

// ========================== 位置点（地标） ==========================

// LandmarkKindSet 支持的位置类型
var LandmarkKindSet = map[string]bool{
	"entrance": true, // 入口
	"exit":     true, // 出口
	"dock":     true, // 装卸位
	"charger":  true, // 充电桩
	"person":   true, // 搬货人员当前位置
	"robot":    true, // 机器(AGV)当前位置
	"waypoint": true, // 一般路径点
}

// LandmarkInput 位置点输入
type LandmarkInput struct {
	Code      string
	Name      string
	Kind      string
	Col       int
	Row       int
	IsMovable bool
	Note      string
}

// normalizeLandmarkKind 归一化并校验位置类型，缺省为 waypoint
func normalizeLandmarkKind(kind string) (string, error) {
	k := strings.TrimSpace(strings.ToLower(kind))
	if k == "" {
		return "waypoint", nil
	}
	if !LandmarkKindSet[k] {
		return "", fmt.Errorf("不支持的位置类型%s，可选：entrance/exit/dock/charger/person/robot/waypoint", kind)
	}
	return k, nil
}

// CreateLandmark 在仓库下创建位置点（入口/出口/设备位置等）
func (s *WarehouseService) CreateLandmark(whID int, in LandmarkInput) (*ent.Landmark, error) {
	ctx := context.Background()
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return nil, errors.New("位置编码不能为空")
	}
	kind, err := normalizeLandmarkKind(in.Kind)
	if err != nil {
		return nil, err
	}
	if exist, err := s.Repo.Landmarks.GetByWarehouseAndCode(ctx, whID, code); err == nil && exist != nil {
		return nil, fmt.Errorf("仓库%d中位置编码%s已存在", whID, code)
	}
	l := &ent.Landmark{
		Code:      code,
		Name:      in.Name,
		Kind:      kind,
		Col:       in.Col,
		Row:       in.Row,
		IsMovable: in.IsMovable,
		Note:      in.Note,
	}
	return s.Repo.Landmarks.Create(ctx, whID, l)
}

// ListLandmarks 列出仓库下的位置点；kind 非空时按类型过滤
func (s *WarehouseService) ListLandmarks(whID int, kind string) ([]*ent.Landmark, error) {
	ctx := context.Background()
	if k := strings.TrimSpace(kind); k != "" {
		normalized, err := normalizeLandmarkKind(k)
		if err != nil {
			return nil, err
		}
		return s.Repo.Landmarks.ListByWarehouseAndKind(ctx, whID, normalized)
	}
	return s.Repo.Landmarks.ListByWarehouse(ctx, whID)
}

// GetLandmark 按编码取位置点
func (s *WarehouseService) GetLandmark(whID int, code string) (*ent.Landmark, error) {
	return s.Repo.Landmarks.GetByWarehouseAndCode(context.Background(), whID, strings.TrimSpace(code))
}

// UpdateLandmark 更新位置点；只改坐标即可用于动态更新人员/机器位置
func (s *WarehouseService) UpdateLandmark(whID int, code string, in LandmarkInput) (*ent.Landmark, error) {
	ctx := context.Background()
	exist, err := s.Repo.Landmarks.GetByWarehouseAndCode(ctx, whID, strings.TrimSpace(code))
	if err != nil {
		return nil, fmt.Errorf("仓库%d中位置%s不存在", whID, code)
	}
	kind, err := normalizeLandmarkKind(in.Kind)
	if err != nil {
		return nil, err
	}
	exist.Name = in.Name
	exist.Kind = kind
	exist.Col = in.Col
	exist.Row = in.Row
	exist.IsMovable = in.IsMovable
	exist.Note = in.Note
	return s.Repo.Landmarks.Update(ctx, exist.ID, exist)
}

// LandmarkPatch 位置点部分更新输入；nil 字段保持原值
type LandmarkPatch struct {
	Name      *string
	Kind      *string
	Col       *int
	Row       *int
	IsMovable *bool
	Note      *string
}

// PatchLandmark 部分更新位置点；仅更新传入的字段，
// 便于高频刷新搬货人员/机器人的实时坐标。
func (s *WarehouseService) PatchLandmark(whID int, code string, in LandmarkPatch) (*ent.Landmark, error) {
	ctx := context.Background()
	exist, err := s.Repo.Landmarks.GetByWarehouseAndCode(ctx, whID, strings.TrimSpace(code))
	if err != nil {
		return nil, fmt.Errorf("仓库%d中位置%s不存在", whID, code)
	}
	if in.Name != nil {
		exist.Name = *in.Name
	}
	if in.Kind != nil {
		kind, err := normalizeLandmarkKind(*in.Kind)
		if err != nil {
			return nil, err
		}
		exist.Kind = kind
	}
	if in.Col != nil {
		exist.Col = *in.Col
	}
	if in.Row != nil {
		exist.Row = *in.Row
	}
	if in.IsMovable != nil {
		exist.IsMovable = *in.IsMovable
	}
	if in.Note != nil {
		exist.Note = *in.Note
	}
	return s.Repo.Landmarks.Update(ctx, exist.ID, exist)
}

// DeleteLandmark 删除位置点
func (s *WarehouseService) DeleteLandmark(whID int, code string) error {
	ctx := context.Background()
	exist, err := s.Repo.Landmarks.GetByWarehouseAndCode(ctx, whID, strings.TrimSpace(code))
	if err != nil {
		return fmt.Errorf("仓库%d中位置%s不存在", whID, code)
	}
	return s.Repo.Landmarks.Delete(ctx, exist.ID)
}

// ResolveLandmark 把位置编码解析为带显示名的路径点
func (s *WarehouseService) ResolveLandmark(whID int, code string) (Waypoint, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Waypoint{}, errors.New("位置编码不能为空")
	}
	lm, err := s.Repo.Landmarks.GetByWarehouseAndCode(context.Background(), whID, code)
	if err != nil {
		return Waypoint{}, fmt.Errorf("仓库%d中位置%s不存在", whID, code)
	}
	label := lm.Name
	if label == "" {
		label = lm.Code
	}
	return Waypoint{Point: utils.Point{X: lm.Col, Y: lm.Row}, Label: label}, nil
}

// ========================== 执行设备位置（复用位置点） ==========================

// DevicePositionInput 执行设备位置上报输入。
// 一个用户只允许在一台设备上登录，因此设备编码可直接作为位置点编码复用位置点表，
// 无需为"设备当前位置"单独建表。
type DevicePositionInput struct {
	DeviceID string
	Name     string
	Kind     string // 缺省 robot
	Col      int
	Row      int
	Note     string
}

// UpsertDevicePosition 上报执行设备当前位置：位置点不存在则创建，存在则刷新坐标。
// 复用位置点表，kind 缺省为 robot，is_movable 恒为 true。
func (s *WarehouseService) UpsertDevicePosition(whID int, in DevicePositionInput) (*ent.Landmark, error) {
	ctx := context.Background()
	code := strings.TrimSpace(in.DeviceID)
	if code == "" {
		return nil, errors.New("设备编码不能为空")
	}

	kind := "robot"
	if raw := strings.TrimSpace(in.Kind); raw != "" {
		normalized, err := normalizeLandmarkKind(raw)
		if err != nil {
			return nil, err
		}
		kind = normalized
	}

	exist, err := s.Repo.Landmarks.GetByWarehouseAndCode(ctx, whID, code)
	if err != nil || exist == nil {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = code
		}
		return s.Repo.Landmarks.Create(ctx, whID, &ent.Landmark{
			Code:      code,
			Name:      name,
			Kind:      kind,
			Col:       in.Col,
			Row:       in.Row,
			IsMovable: true,
			Note:      in.Note,
		})
	}

	exist.Kind = kind
	exist.Col = in.Col
	exist.Row = in.Row
	exist.IsMovable = true
	if name := strings.TrimSpace(in.Name); name != "" {
		exist.Name = name
	}
	if note := strings.TrimSpace(in.Note); note != "" {
		exist.Note = note
	}
	return s.Repo.Landmarks.Update(ctx, exist.ID, exist)
}

// DevicePosition 取执行设备最近一次上报的位置；设备尚未上报过位置时返回 ok=false。
func (s *WarehouseService) DevicePosition(whID int, deviceID string) (Waypoint, bool) {
	if strings.TrimSpace(deviceID) == "" {
		return Waypoint{}, false
	}
	wp, err := s.ResolveLandmark(whID, deviceID)
	if err != nil {
		return Waypoint{}, false
	}
	return wp, true
}

// EntranceWaypoint 取仓库入口位置点（kind=entrance 的第一条）；未配置入口时返回 ok=false。
func (s *WarehouseService) EntranceWaypoint(whID int) (Waypoint, bool) {
	list, err := s.Repo.Landmarks.ListByWarehouseAndKind(context.Background(), whID, "entrance")
	if err != nil || len(list) == 0 {
		return Waypoint{}, false
	}
	lm := list[0]
	label := lm.Name
	if label == "" {
		label = lm.Code
	}
	return Waypoint{Point: utils.Point{X: lm.Col, Y: lm.Row}, Label: label}, true
}

// ========================== 包裹 ==========================

// CreatePackageInput 创建包裹输入。定位方式二选一：
//  1. Barcode 非空：按商品码自动找到该商品在本仓库的存放单元，并关联商品；
//  2. ShelfNum + UnitNum：直接指定目标货架与单元。
type CreatePackageInput struct {
	PackageID string
	ShelfNum  int
	UnitNum   int
	Priority  int
	Note      string
	Barcode   string
}

// CreatePackage 在指定仓库内创建包裹
func (s *WarehouseService) CreatePackage(whID int, in CreatePackageInput) (*ent.GoodsPackage, error) {
	ctx := context.Background()

	var product *ent.Product
	shelfNum, unitNum := in.ShelfNum, in.UnitNum

	if code := strings.TrimSpace(in.Barcode); code != "" {
		prod, sh, u, err := s.resolveBarcodeLocation(ctx, whID, code)
		if err != nil {
			return nil, err
		}
		product, shelfNum, unitNum = prod, sh.ShelfNumber, u.UnitNumber
	}
	if shelfNum <= 0 || unitNum <= 0 {
		return nil, errors.New("必须提供商品码 barcode，或同时提供 shelf_num 与 unit_num")
	}

	sh, err := s.Repo.Shelves.GetByWarehouseAndNumber(ctx, whID, shelfNum)
	if err != nil {
		return nil, fmt.Errorf("仓库%d中货架%d不存在", whID, shelfNum)
	}
	unit, err := s.Repo.Units.GetByShelfAndNumber(ctx, sh.ID, unitNum)
	if err != nil {
		return nil, fmt.Errorf("货架%d的单元%d不存在", shelfNum, unitNum)
	}

	p := &ent.GoodsPackage{
		PackageID: in.PackageID,
		Status:    "pending",
		Priority:  in.Priority,
		Note:      in.Note,
	}
	p.Edges.Shelf = sh
	p.Edges.Unit = unit
	p.Edges.Product = product
	return s.Repo.Packages.Create(ctx, p)
}

// CreatePackageFromBarcode 扫码建包：商品码 → 商品 → 该商品绑定的存放单元。
func (s *WarehouseService) CreatePackageFromBarcode(whID int, pid, barcode string, priority int, note string) (*ent.GoodsPackage, error) {
	return s.CreatePackage(whID, CreatePackageInput{
		PackageID: pid,
		Barcode:   barcode,
		Priority:  priority,
		Note:      note,
	})
}

// resolveBarcodeLocation 按商品码在本仓库内定位存储单元，返回商品/货架/单元。
func (s *WarehouseService) resolveBarcodeLocation(ctx context.Context, whID int, barcode string) (*ent.Product, *ent.Shelf, *ent.ShelfUnit, error) {
	prod, err := s.Repo.Products.GetByBarcode(ctx, barcode)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("商品码%s未匹配到商品", barcode)
	}
	for _, u := range prod.Edges.Units {
		sh := u.Edges.Shelf
		if sh == nil || sh.WarehouseID != whID {
			continue
		}
		return prod, sh, u, nil
	}
	return nil, nil, nil, fmt.Errorf("商品%s(%s)尚未在仓库%d中绑定存放单元，请先在货架单元上绑定该商品",
		prod.Name, barcode, whID)
}

// ListPendingPackages 列出指定仓库未送达包裹
func (s *WarehouseService) ListPendingPackages(whID int) ([]*ent.GoodsPackage, error) {
	return s.Repo.Packages.ListPendingByWarehouse(context.Background(), whID)
}

// ListPendingPackagesAll 列出全部未送达包裹（兼容旧接口）
func (s *WarehouseService) ListPendingPackagesAll() ([]*ent.GoodsPackage, error) {
	return s.Repo.Packages.ListPending(context.Background())
}

// ========================== 路径规划 ==========================

// PackageTask 单个包裹任务
type PackageTask struct {
	PackageID string
	ShelfNum  int
	UnitNum   int
	Label     string
	Desc      string
}

// Segment 路径段
type Segment struct {
	FromLabel string
	ToLabel   string
	ToDesc    string
	Distance  int
}

// Waypoint 带显示名的路径点（起点/终点），用于把位置点(地标)解析为坐标
type Waypoint struct {
	Point utils.Point
	Label string
}

// PlanResult 路径规划结果
type PlanResult struct {
	Order      []int
	Segments   []Segment
	TotalDist  int
	StartCol   int
	StartRow   int
	StartLabel string
	EndCol     int
	EndRow     int
	EndLabel   string
}

// PlanPath 在指定仓库内贪心最近邻求解访问顺序，从 start 出发；end 非空时在末尾追加一段收尾路径
// （例如从最后一个任务点走到出口）。start 不指定时调用方应使用 utils.Point{X:0, Y:0}（仓库入口）。
func (s *WarehouseService) PlanPath(whID int, tasks []PackageTask, start Waypoint, end *Waypoint) (*PlanResult, error) {
	if len(tasks) == 0 {
		return nil, errors.New("任务列表为空")
	}
	if len(tasks) > 10 {
		return nil, errors.New("一次最多规划10个任务")
	}

	// 加载该仓库下所有货架布局（动态，非硬编码）
	shelves, err := s.Repo.Shelves.ListByWarehouse(context.Background(), whID)
	if err != nil {
		return nil, err
	}
	layouts := make(map[int]utils.ShelfLayout, len(shelves))
	for _, sh := range shelves {
		layouts[sh.ShelfNumber] = utils.LayoutFromShelf(sh)
	}

	// 每个任务的存取点坐标
	points := make([]utils.Point, len(tasks))
	for i, t := range tasks {
		l, ok := layouts[t.ShelfNum]
		if !ok {
			return nil, fmt.Errorf("任务%d(%s): 仓库%d中不存在货架%d", i+1, t.PackageID, whID, t.ShelfNum)
		}
		col, row, ok2 := utils.AccessPoint(l, t.UnitNum)
		if !ok2 {
			return nil, fmt.Errorf("任务%d(%s)坐标无效", i+1, t.PackageID)
		}
		points[i] = utils.Point{X: col, Y: row}
	}

	startLabel := start.Label
	if startLabel == "" {
		startLabel = "起点"
	}

	// 起点：默认 (0,0) 货仓入口，或客户端上报的设备当前位置
	cur := start.Point
	visited := make([]bool, len(tasks))
	order := make([]int, 0, len(tasks))
	segments := make([]Segment, 0, len(tasks)+1)
	total := 0

	for len(order) < len(tasks) {
		bestIdx, bestDist := -1, 1<<30
		for i := range tasks {
			if visited[i] {
				continue
			}
			d := utils.Distance(cur, points[i])
			if d < bestDist {
				bestDist = d
				bestIdx = i
			}
		}

		if len(order) == 0 {
			segments = append(segments, Segment{
				FromLabel: startLabel,
				ToLabel:   tasks[bestIdx].Label,
				ToDesc:    tasks[bestIdx].Desc,
				Distance:  bestDist,
			})
		} else {
			segments = append(segments, Segment{
				FromLabel: tasks[order[len(order)-1]].Label,
				ToLabel:   tasks[bestIdx].Label,
				ToDesc:    tasks[bestIdx].Desc,
				Distance:  bestDist,
			})
		}
		total += bestDist
		order = append(order, bestIdx)
		visited[bestIdx] = true
		cur = points[bestIdx]
	}

	// 收尾：走到终点（如出口/装卸位）
	if end != nil {
		lastLabel := startLabel
		if len(order) > 0 {
			lastLabel = tasks[order[len(order)-1]].Label
		}
		endLabel := end.Label
		if endLabel == "" {
			endLabel = "终点"
		}
		d := utils.Distance(cur, end.Point)
		segments = append(segments, Segment{
			FromLabel: lastLabel,
			ToLabel:   endLabel,
			ToDesc:    endLabel,
			Distance:  d,
		})
		total += d
	}

	res := &PlanResult{
		Order:      order,
		Segments:   segments,
		TotalDist:  total,
		StartCol:   start.Point.X,
		StartRow:   start.Point.Y,
		StartLabel: startLabel,
	}
	if end != nil {
		res.EndCol = end.Point.X
		res.EndRow = end.Point.Y
		res.EndLabel = end.Label
	}
	return res, nil
}

// ========================== 货架定义 ==========================

// shelfDef 货架定义
type shelfDef struct {
	Num, Col, Row, W, H, UnitCount int
	Bold, IsH, IsVL                bool
}

func shelfDefs() []shelfDef {
	return []shelfDef{
		{10, 3, 0, 6, 1, 1, false, true, false},
		{11, 0, 3, 1, 9, 6, false, false, true},
		{9, 3, 13, 1, 3, 4, false, false, false},
		{4, 6, 13, 1, 3, 4, false, false, false},
		{3, 9, 13, 1, 3, 4, false, false, false},
		{8, 3, 17, 1, 3, 4, false, false, false},
		{5, 6, 17, 1, 3, 3, false, false, false},
		{7, 3, 21, 1, 3, 4, false, false, false},
		{6, 6, 21, 1, 3, 4, false, false, false},
		{2, 9, 21, 1, 3, 4, true, false, false},
		{1, 9, 25, 1, 3, 4, true, false, false},
	}
}
