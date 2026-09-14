package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// GoodsPackage 待入库包裹
type GoodsPackage struct {
	ent.Schema
}

// Annotations 保持与旧版表名一致
func (GoodsPackage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "packages"},
	}
}

func (GoodsPackage) Fields() []ent.Field {
	return []ent.Field{
		field.String("package_id").Unique().Comment("包裹业务编号"),
		field.String("status").Default("pending").Comment("pending/placed/delivered"),
		field.Int("priority").Default(0).Comment("优先级"),
		field.String("note").Default("").Comment("备注"),
		field.Int("product_id").Optional().Comment("包裹内商品ID"),
	}
}

func (GoodsPackage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("shelf", Shelf.Type).Ref("packages").Unique().Comment("目标货架"),
		edge.From("unit", ShelfUnit.Type).Ref("packages").Unique().Comment("目标单元"),
		// 包裹内商品（可选，存量包裹可能未关联商品）
		edge.From("product", Product.Type).Ref("packages").Unique().
			Field("product_id").Comment("包裹内商品"),
	}
}
