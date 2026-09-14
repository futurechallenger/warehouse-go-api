package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Shelf 货架（仓位）
type Shelf struct {
	ent.Schema
}

func (Shelf) Fields() []ent.Field {
	return []ent.Field{
		field.Int("shelf_number").Comment("货架编号"),
		field.String("label").Default("").Comment("显示标签"),
		field.Bool("bold").Default(false).Comment("是否加深"),
		field.Int("col").Default(0).Comment("起始列"),
		field.Int("row").Default(0).Comment("起始行"),
		field.Int("width").Default(1).Comment("占格列数"),
		field.Int("height").Default(1).Comment("占格行数"),
		field.Bool("is_horizontal").Default(false).Comment("是否横向"),
		field.Bool("is_vertical_long").Default(false).Comment("是否加长"),
		field.Int("warehouse_id").Optional().Comment("所属仓库ID"),
	}
}

func (Shelf) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("warehouse", Warehouse.Type).Ref("shelves").Unique().Field("warehouse_id").Comment("所属仓库"),
		edge.To("units", ShelfUnit.Type).Comment("该货架下的所有单元"),
		edge.To("packages", GoodsPackage.Type).Comment("该货架下的包裹"),
	}
}

func (Shelf) Indexes() []ent.Index {
	return []ent.Index{
		// 货架编号在仓库范围内唯一
		index.Fields("shelf_number").Edges("warehouse").Unique(),
	}
}
