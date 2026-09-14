package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Warehouse 仓库
type Warehouse struct {
	ent.Schema
}

func (Warehouse) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Default("").Comment("仓库名称"),
		field.String("code").Default("").Comment("仓库编码"),
		field.Int("store_id").Comment("所属门店ID"),
	}
}

func (Warehouse) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("store", Store.Type).Ref("warehouses").Unique().Required().Field("store_id").Comment("所属门店"),
		edge.To("shelves", Shelf.Type).Comment("该仓库下的所有货架"),
		edge.To("path_plans", PathPlan.Type).Comment("该仓库下的路径规划记录"),
		edge.To("landmarks", Landmark.Type).Comment("该仓库下的位置点（入口/出口/设备等）"),
	}
}

func (Warehouse) Indexes() []ent.Index {
	return []ent.Index{
		// 仓库编码在门店范围内唯一
		index.Fields("code").Edges("store").Unique(),
	}
}
