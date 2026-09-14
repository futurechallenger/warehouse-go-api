// Package repository 使用 ent 提供仓储层实现
package repository

// =============================================================================
//  internal/repository — ent 数据访问层
// =============================================================================

import (
	"context"
	dbsql "database/sql"
	"errors"
	"fmt"
	"strings"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/lib/pq" // postgres 驱动，必须显式导入注册

	"warehouse/ent"
	"warehouse/ent/goodspackage"
	"warehouse/ent/landmark"
	"warehouse/ent/pathplan"
	"warehouse/ent/product"
	"warehouse/ent/shelf"
	"warehouse/ent/shelfunit"
	"warehouse/ent/store"
	"warehouse/ent/warehouse"
)

var ErrNotFound = errors.New("record not found")

// Repository 持有 ent 客户端
type Repository struct {
	Client     *ent.Client
	Stores     StoreRepo
	Warehouses WarehouseRepo
	Shelves    ShelfRepo
	Units      UnitRepo
	Packages   PackageRepo
	PathPlans  PathPlanRepo
	Products   ProductRepo
	Landmarks  LandmarkRepo
}

// New 连接数据库并自动迁移表结构
func New(dsn string) (*Repository, error) {
	client, err := ent.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	// 预迁移：存量 shelves 表可能缺少 warehouse_id 列，
	// 先补列（可空），否则 ent 迁移对非空表加 NOT NULL 列会失败。
	if err := ensureLegacyColumns(dsn); err != nil {
		_ = client.Close()
		return nil, err
	}

	// ent 自动迁移（增量，安全）
	if err := client.Schema.Create(context.Background()); err != nil {
		_ = client.Close()
		return nil, err
	}

	r := &Repository{
		Client:     client,
		Stores:     &storeRepo{client: client},
		Warehouses: &warehouseRepo{client: client},
		Shelves:    &shelfRepo{client: client},
		Units:      &unitRepo{client: client},
		Packages:   &packageRepo{client: client},
		PathPlans:  &pathPlanRepo{client: client},
		Products:   &productRepo{client: client},
		Landmarks:  &landmarkRepo{client: client},
	}

	// 确保默认门店/仓库存在，并把存量无归属货架挂到默认仓库
	if _, err := r.EnsureDefaultStoreWarehouse(context.Background()); err != nil {
		_ = client.Close()
		return nil, err
	}

	return r, nil
}

// ensureLegacyColumns 在 ent 迁移前补齐旧表可能缺失的外键列。
// 使用独立的原生连接执行，避免依赖 ent 客户端的底层 DB 访问。
func ensureLegacyColumns(dsn string) error {
	db, err := dbsql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	var exists bool
	if err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = 'shelves'
		)`).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE shelves ADD COLUMN IF NOT EXISTS warehouse_id bigint`)
	return err
}

// Close 关闭数据库连接
func (r *Repository) Close() error {
	return r.Client.Close()
}

// EnsureDefaultStoreWarehouse 幂等地确保默认门店与仓库存在，
// 并把所有尚无归属的货架挂接到默认仓库。
func (r *Repository) EnsureDefaultStoreWarehouse(ctx context.Context) (*ent.Warehouse, error) {
	// 1. 默认门店
	storeRow, err := r.Client.Store.Query().
		Where(store.CodeEQ("DEFAULT")).
		First(ctx)
	if ent.IsNotFound(err) {
		storeRow, err = r.Client.Store.Create().
			SetName("默认门店").
			SetCode("DEFAULT").
			Save(ctx)
	}
	if err != nil {
		return nil, err
	}

	// 2. 默认仓库（门店范围内 code=DEFAULT）
	wh, err := r.Client.Warehouse.Query().
		Where(
			warehouse.CodeEQ("DEFAULT"),
			warehouse.HasStoreWith(store.IDEQ(storeRow.ID)),
		).
		First(ctx)
	if ent.IsNotFound(err) {
		wh, err = r.Client.Warehouse.Create().
			SetName("默认仓库").
			SetCode("DEFAULT").
			SetStoreID(storeRow.ID).
			Save(ctx)
	}
	if err != nil {
		return nil, err
	}

	// 3. 回填存量货架
	if _, err := r.Client.Shelf.Update().
		Where(shelf.WarehouseIDIsNil()).
		SetWarehouseID(wh.ID).
		Save(ctx); err != nil {
		return nil, err
	}

	return wh, nil
}

// ========================== Store ==========================

// StoreRepo 门店仓储
type StoreRepo interface {
	List(ctx context.Context) ([]*ent.Store, error)
	GetByID(ctx context.Context, id int) (*ent.Store, error)
	GetByCode(ctx context.Context, code string) (*ent.Store, error)
	Create(ctx context.Context, name, code string) (*ent.Store, error)
}

type storeRepo struct{ client *ent.Client }

func (r *storeRepo) List(ctx context.Context) ([]*ent.Store, error) {
	return r.client.Store.Query().
		Order(store.ByID()).
		All(ctx)
}

func (r *storeRepo) GetByID(ctx context.Context, id int) (*ent.Store, error) {
	return r.client.Store.Get(ctx, id)
}

func (r *storeRepo) GetByCode(ctx context.Context, code string) (*ent.Store, error) {
	return r.client.Store.Query().
		Where(store.CodeEQ(code)).
		First(ctx)
}

func (r *storeRepo) Create(ctx context.Context, name, code string) (*ent.Store, error) {
	return r.client.Store.Create().
		SetName(name).
		SetCode(code).
		Save(ctx)
}

// ========================== Warehouse ==========================

// WarehouseRepo 仓库仓储
type WarehouseRepo interface {
	ListByStore(ctx context.Context, storeID int) ([]*ent.Warehouse, error)
	ListAll(ctx context.Context) ([]*ent.Warehouse, error)
	GetByID(ctx context.Context, id int) (*ent.Warehouse, error)
	Create(ctx context.Context, storeID int, name, code string) (*ent.Warehouse, error)
	GetByStoreAndCode(ctx context.Context, storeID int, code string) (*ent.Warehouse, error)
}

type warehouseRepo struct{ client *ent.Client }

func (r *warehouseRepo) ListByStore(ctx context.Context, storeID int) ([]*ent.Warehouse, error) {
	return r.client.Warehouse.Query().
		Where(warehouse.HasStoreWith(store.IDEQ(storeID))).
		Order(warehouse.ByID()).
		All(ctx)
}

func (r *warehouseRepo) ListAll(ctx context.Context) ([]*ent.Warehouse, error) {
	return r.client.Warehouse.Query().
		WithStore().
		Order(warehouse.ByID()).
		All(ctx)
}

func (r *warehouseRepo) GetByID(ctx context.Context, id int) (*ent.Warehouse, error) {
	return r.client.Warehouse.Query().
		Where(warehouse.IDEQ(id)).
		WithStore().
		Only(ctx)
}

func (r *warehouseRepo) Create(ctx context.Context, storeID int, name, code string) (*ent.Warehouse, error) {
	return r.client.Warehouse.Create().
		SetName(name).
		SetCode(code).
		SetStoreID(storeID).
		Save(ctx)
}

func (r *warehouseRepo) GetByStoreAndCode(ctx context.Context, storeID int, code string) (*ent.Warehouse, error) {
	return r.client.Warehouse.Query().
		Where(
			warehouse.CodeEQ(code),
			warehouse.HasStoreWith(store.IDEQ(storeID)),
		).
		First(ctx)
}

// ========================== Shelf ==========================

// ShelfRepo 货架仓储
type ShelfRepo interface {
	ListByWarehouse(ctx context.Context, whID int) ([]*ent.Shelf, error)
	GetByWarehouseAndNumber(ctx context.Context, whID, num int) (*ent.Shelf, error)
	Save(ctx context.Context, whID int, s *ent.Shelf) (*ent.Shelf, error)
	// CreateWithUnits 在单个事务中创建货架及其单元
	CreateWithUnits(ctx context.Context, whID int, s *ent.Shelf, units []*ent.ShelfUnit) (*ent.Shelf, error)
}

type shelfRepo struct{ client *ent.Client }

func (r *shelfRepo) ListByWarehouse(ctx context.Context, whID int) ([]*ent.Shelf, error) {
	return r.client.Shelf.Query().
		Where(shelf.WarehouseID(whID)).
		WithUnits().
		Order(shelf.ByShelfNumber()).
		All(ctx)
}

func (r *shelfRepo) GetByWarehouseAndNumber(ctx context.Context, whID, num int) (*ent.Shelf, error) {
	return r.client.Shelf.Query().
		Where(
			shelf.WarehouseID(whID),
			shelf.ShelfNumberEQ(num),
		).
		WithUnits().
		First(ctx)
}

func (r *shelfRepo) Save(ctx context.Context, whID int, s *ent.Shelf) (*ent.Shelf, error) {
	// 不存在则创建，存在则更新
	if s.ID > 0 {
		return r.client.Shelf.UpdateOne(s).
			SetWarehouseID(whID).
			SetLabel(s.Label).
			SetBold(s.Bold).
			SetCol(s.Col).
			SetRow(s.Row).
			SetWidth(s.Width).
			SetHeight(s.Height).
			SetIsHorizontal(s.IsHorizontal).
			SetIsVerticalLong(s.IsVerticalLong).
			Save(ctx)
	}
	return r.client.Shelf.Create().
		SetWarehouseID(whID).
		SetShelfNumber(s.ShelfNumber).
		SetLabel(s.Label).
		SetBold(s.Bold).
		SetCol(s.Col).
		SetRow(s.Row).
		SetWidth(s.Width).
		SetHeight(s.Height).
		SetIsHorizontal(s.IsHorizontal).
		SetIsVerticalLong(s.IsVerticalLong).
		Save(ctx)
}

// CreateWithUnits 在单个事务中创建货架及其单元，保证原子性
func (r *shelfRepo) CreateWithUnits(ctx context.Context, whID int, s *ent.Shelf, units []*ent.ShelfUnit) (*ent.Shelf, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()

	created, err := tx.Shelf.Create().
		SetWarehouseID(whID).
		SetShelfNumber(s.ShelfNumber).
		SetLabel(s.Label).
		SetBold(s.Bold).
		SetCol(s.Col).
		SetRow(s.Row).
		SetWidth(s.Width).
		SetHeight(s.Height).
		SetIsHorizontal(s.IsHorizontal).
		SetIsVerticalLong(s.IsVerticalLong).
		Save(ctx)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	for _, u := range units {
		if _, err := tx.ShelfUnit.Create().
			SetShelfID(created.ID).
			SetUnitNumber(u.UnitNumber).
			SetLabel(u.Label).
			SetPositionFrac(u.PositionFrac).
			SetAccessCol(u.AccessCol).
			SetAccessRow(u.AccessRow).
			SetCapacity(u.Capacity).
			Save(ctx); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// 返回带单元的完整实体
	return r.client.Shelf.Query().
		Where(shelf.ID(created.ID)).
		WithUnits().
		Only(ctx)
}

// ========================== ShelfUnit ==========================

// UnitRepo 货架单元仓储
type UnitRepo interface {
	ListByShelf(ctx context.Context, shelfID int) ([]*ent.ShelfUnit, error)
	GetByShelfAndNumber(ctx context.Context, shelfID, num int) (*ent.ShelfUnit, error)
	GetByID(ctx context.Context, id int) (*ent.ShelfUnit, error)
	Create(ctx context.Context, u *ent.ShelfUnit) (*ent.ShelfUnit, error)
	UpdateCapacity(ctx context.Context, id, capacity int) (*ent.ShelfUnit, error)
	// AssignProduct 绑定单元存放的商品；productID<=0 表示解绑
	AssignProduct(ctx context.Context, unitID, productID int) (*ent.ShelfUnit, error)
}

type unitRepo struct{ client *ent.Client }

func (r *unitRepo) ListByShelf(ctx context.Context, shelfID int) ([]*ent.ShelfUnit, error) {
	return r.client.ShelfUnit.Query().
		Where(shelfunit.HasShelfWith(shelf.IDEQ(shelfID))).
		WithProduct().
		Order(shelfunit.ByUnitNumber()).
		All(ctx)
}

func (r *unitRepo) GetByShelfAndNumber(ctx context.Context, shelfID, num int) (*ent.ShelfUnit, error) {
	return r.client.ShelfUnit.Query().
		Where(
			shelfunit.UnitNumberEQ(num),
			shelfunit.HasShelfWith(shelf.IDEQ(shelfID)),
		).
		WithProduct().
		First(ctx)
}

func (r *unitRepo) GetByID(ctx context.Context, id int) (*ent.ShelfUnit, error) {
	return r.client.ShelfUnit.Query().
		Where(shelfunit.IDEQ(id)).
		WithProduct().
		WithShelf().
		Only(ctx)
}

func (r *unitRepo) Create(ctx context.Context, u *ent.ShelfUnit) (*ent.ShelfUnit, error) {
	create := r.client.ShelfUnit.Create().
		SetUnitNumber(u.UnitNumber).
		SetLabel(u.Label).
		SetPositionFrac(u.PositionFrac).
		SetAccessCol(u.AccessCol).
		SetAccessRow(u.AccessRow).
		SetCapacity(u.Capacity)
	if u.Edges.Shelf != nil {
		create.SetShelfID(u.Edges.Shelf.ID)
	}
	return create.Save(ctx)
}

func (r *unitRepo) UpdateCapacity(ctx context.Context, id, capacity int) (*ent.ShelfUnit, error) {
	return r.client.ShelfUnit.UpdateOneID(id).
		SetCapacity(capacity).
		Save(ctx)
}

func (r *unitRepo) AssignProduct(ctx context.Context, unitID, productID int) (*ent.ShelfUnit, error) {
	if productID <= 0 {
		return r.client.ShelfUnit.UpdateOneID(unitID).
			ClearProduct().
			Save(ctx)
	}
	return r.client.ShelfUnit.UpdateOneID(unitID).
		SetProductID(productID).
		Save(ctx)
}

// ========================== GoodsPackage ==========================

// PackageRepo 包裹仓储
type PackageRepo interface {
	ListPending(ctx context.Context) ([]*ent.GoodsPackage, error)
	ListPendingByWarehouse(ctx context.Context, whID int) ([]*ent.GoodsPackage, error)
	GetByPackageID(ctx context.Context, pid string) (*ent.GoodsPackage, error)
	GetByID(ctx context.Context, id int) (*ent.GoodsPackage, error)
	Create(ctx context.Context, p *ent.GoodsPackage) (*ent.GoodsPackage, error)
	UpdateStatus(ctx context.Context, id int, status string) error
	CountByStatus(ctx context.Context, status string) (int, error)
}

type packageRepo struct{ client *ent.Client }

func (r *packageRepo) ListPending(ctx context.Context) ([]*ent.GoodsPackage, error) {
	return r.client.GoodsPackage.Query().
		Where(goodspackage.StatusNEQ("delivered")).
		WithShelf().
		WithUnit().
		WithProduct().
		Order(goodspackage.ByPriority(), goodspackage.ByID()).
		All(ctx)
}

func (r *packageRepo) ListPendingByWarehouse(ctx context.Context, whID int) ([]*ent.GoodsPackage, error) {
	return r.client.GoodsPackage.Query().
		Where(
			goodspackage.StatusNEQ("delivered"),
			goodspackage.HasShelfWith(shelf.WarehouseID(whID)),
		).
		WithShelf().
		WithUnit().
		WithProduct().
		Order(goodspackage.ByPriority(), goodspackage.ByID()).
		All(ctx)
}

func (r *packageRepo) GetByPackageID(ctx context.Context, pid string) (*ent.GoodsPackage, error) {
	return r.client.GoodsPackage.Query().
		Where(goodspackage.PackageIDEQ(pid)).
		WithShelf().
		WithUnit().
		WithProduct().
		First(ctx)
}

func (r *packageRepo) GetByID(ctx context.Context, id int) (*ent.GoodsPackage, error) {
	return r.client.GoodsPackage.Query().
		Where(goodspackage.IDEQ(id)).
		WithShelf().
		WithUnit().
		WithProduct().
		First(ctx)
}

func (r *packageRepo) Create(ctx context.Context, p *ent.GoodsPackage) (*ent.GoodsPackage, error) {
	create := r.client.GoodsPackage.Create().
		SetPackageID(p.PackageID).
		SetStatus(p.Status).
		SetPriority(p.Priority).
		SetNote(p.Note)
	if p.Edges.Shelf != nil {
		create.SetShelfID(p.Edges.Shelf.ID)
	}
	if p.Edges.Unit != nil {
		create.SetUnitID(p.Edges.Unit.ID)
	}
	if p.Edges.Product != nil {
		create.SetProductID(p.Edges.Product.ID)
	}
	return create.Save(ctx)
}

func (r *packageRepo) UpdateStatus(ctx context.Context, id int, status string) error {
	return r.client.GoodsPackage.UpdateOneID(id).
		SetStatus(status).
		Exec(ctx)
}

func (r *packageRepo) CountByStatus(ctx context.Context, status string) (int, error) {
	return r.client.GoodsPackage.Query().
		Where(goodspackage.StatusEQ(status)).
		Count(ctx)
}

// ========================== PathPlan ==========================

// PathPlanRepo 路径规划仓储
type PathPlanRepo interface {
	Upsert(ctx context.Context, p *ent.PathPlan) (*ent.PathPlan, error)
	GetByKey(ctx context.Context, key string) (*ent.PathPlan, error)
}

type pathPlanRepo struct{ client *ent.Client }

func (r *pathPlanRepo) Upsert(ctx context.Context, p *ent.PathPlan) (*ent.PathPlan, error) {
	create := r.client.PathPlan.Create().
		SetPlanKey(p.PlanKey).
		SetTotalDist(p.TotalDist).
		SetOrderJSON(p.OrderJSON).
		SetSegmentsJSON(p.SegmentsJSON)
	if p.Edges.Warehouse != nil {
		create.SetWarehouseID(p.Edges.Warehouse.ID)
	}
	id, err := create.OnConflict(
		entsql.ConflictColumns(pathplan.FieldPlanKey),
	).UpdateNewValues().ID(ctx)
	if err != nil {
		return nil, err
	}
	return r.client.PathPlan.Get(ctx, id)
}

func (r *pathPlanRepo) GetByKey(ctx context.Context, key string) (*ent.PathPlan, error) {
	return r.client.PathPlan.Query().
		Where(pathplan.PlanKeyEQ(key)).
		First(ctx)
}

// ========================== Product ==========================

// ProductRepo 商品仓储
type ProductRepo interface {
	List(ctx context.Context) ([]*ent.Product, error)
	GetByID(ctx context.Context, id int) (*ent.Product, error)
	// GetByBarcode 按商品码（EAN-13 / EAN-8 / QR code 任一匹配）查询商品，
	// 并带出存放该商品的货架单元（含货架信息），用于根据扫码结果定位存储位置。
	GetByBarcode(ctx context.Context, barcode string) (*ent.Product, error)
	Create(ctx context.Context, p *ent.Product) (*ent.Product, error)
	Update(ctx context.Context, id int, p *ent.Product) (*ent.Product, error)
	Delete(ctx context.Context, id int) error
}

type productRepo struct{ client *ent.Client }

func (r *productRepo) List(ctx context.Context) ([]*ent.Product, error) {
	return r.client.Product.Query().
		WithUnits().
		Order(product.ByID()).
		All(ctx)
}

func (r *productRepo) GetByID(ctx context.Context, id int) (*ent.Product, error) {
	return r.client.Product.Query().
		Where(product.IDEQ(id)).
		WithUnits().
		Only(ctx)
}

func (r *productRepo) GetByBarcode(ctx context.Context, barcode string) (*ent.Product, error) {
	barcode = strings.TrimSpace(barcode)
	if barcode == "" {
		return nil, fmt.Errorf("商品码不能为空")
	}
	return r.client.Product.Query().
		Where(
			product.Or(
				product.Ean13EQ(barcode),
				product.Ean8EQ(barcode),
				product.QrCodeEQ(barcode),
			),
		).
		WithUnits(func(uq *ent.ShelfUnitQuery) {
			// 带出单元所属货架，便于返回"几号货架几单元"
			uq.WithShelf()
		}).
		Only(ctx)
}

func (r *productRepo) Create(ctx context.Context, p *ent.Product) (*ent.Product, error) {
	return r.client.Product.Create().
		SetName(p.Name).
		SetEan13(p.Ean13).
		SetEan8(p.Ean8).
		SetQrCode(p.QrCode).
		Save(ctx)
}

func (r *productRepo) Update(ctx context.Context, id int, p *ent.Product) (*ent.Product, error) {
	return r.client.Product.UpdateOneID(id).
		SetName(p.Name).
		SetEan13(p.Ean13).
		SetEan8(p.Ean8).
		SetQrCode(p.QrCode).
		Save(ctx)
}

func (r *productRepo) Delete(ctx context.Context, id int) error {
	return r.client.Product.DeleteOneID(id).Exec(ctx)
}

// ========================== Landmark ==========================

// LandmarkRepo 位置点（地标）仓储。
// 位置点不承载存储单元，仅提供网格坐标：入口、出口、装卸位、充电桩，
// 以及搬货人员/机器人的当前位置。
type LandmarkRepo interface {
	ListByWarehouse(ctx context.Context, whID int) ([]*ent.Landmark, error)
	ListByWarehouseAndKind(ctx context.Context, whID int, kind string) ([]*ent.Landmark, error)
	GetByWarehouseAndCode(ctx context.Context, whID int, code string) (*ent.Landmark, error)
	Create(ctx context.Context, whID int, l *ent.Landmark) (*ent.Landmark, error)
	Update(ctx context.Context, id int, l *ent.Landmark) (*ent.Landmark, error)
	Delete(ctx context.Context, id int) error
}

type landmarkRepo struct{ client *ent.Client }

func (r *landmarkRepo) ListByWarehouse(ctx context.Context, whID int) ([]*ent.Landmark, error) {
	return r.client.Landmark.Query().
		Where(landmark.HasWarehouseWith(warehouse.IDEQ(whID))).
		Order(landmark.ByKind(), landmark.ByCode()).
		All(ctx)
}

func (r *landmarkRepo) ListByWarehouseAndKind(ctx context.Context, whID int, kind string) ([]*ent.Landmark, error) {
	return r.client.Landmark.Query().
		Where(
			landmark.HasWarehouseWith(warehouse.IDEQ(whID)),
			landmark.KindEQ(kind),
		).
		Order(landmark.ByCode()).
		All(ctx)
}

func (r *landmarkRepo) GetByWarehouseAndCode(ctx context.Context, whID int, code string) (*ent.Landmark, error) {
	return r.client.Landmark.Query().
		Where(
			landmark.CodeEQ(code),
			landmark.HasWarehouseWith(warehouse.IDEQ(whID)),
		).
		WithWarehouse().
		Only(ctx)
}

func (r *landmarkRepo) Create(ctx context.Context, whID int, l *ent.Landmark) (*ent.Landmark, error) {
	return r.client.Landmark.Create().
		SetCode(l.Code).
		SetName(l.Name).
		SetKind(l.Kind).
		SetCol(l.Col).
		SetRow(l.Row).
		SetIsMovable(l.IsMovable).
		SetNote(l.Note).
		SetWarehouseID(whID).
		Save(ctx)
}

func (r *landmarkRepo) Update(ctx context.Context, id int, l *ent.Landmark) (*ent.Landmark, error) {
	return r.client.Landmark.UpdateOneID(id).
		SetName(l.Name).
		SetKind(l.Kind).
		SetCol(l.Col).
		SetRow(l.Row).
		SetIsMovable(l.IsMovable).
		SetNote(l.Note).
		Save(ctx)
}

func (r *landmarkRepo) Delete(ctx context.Context, id int) error {
	return r.client.Landmark.DeleteOneID(id).Exec(ctx)
}
