package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// ShelfUnit 货架单元（单元格）
type ShelfUnit struct {
	ent.Schema
}

func (ShelfUnit) Fields() []ent.Field {
	return []ent.Field{
		field.Int("unit_number").Comment("单元编号"),
		field.String("label").Default("").Comment("单元标签如2-4"),
		field.Float("position_frac").Default(0).Comment("单元在货架内的竖向位置(0~1)"),
		field.Int("access_col").Default(0).Comment("存取通道列"),
		field.Int("access_row").Default(0).Comment("存取通道行"),
		field.Int("capacity").Default(10).Comment("存储数量(可容纳包裹数)"),
		field.Int("product_id").Optional().Comment("存放的商品ID"),
	}
}

func (ShelfUnit) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("shelf", Shelf.Type).Ref("units").Unique().Required().Comment("所属货架"),
		edge.To("packages", GoodsPackage.Type).Comment("该单元下的包裹"),
		// 一个单元只放一种商品（可选，存量单元可能未绑定）
		edge.To("product", Product.Type).Unique().Field("product_id").Comment("存放的商品"),
	}
}
