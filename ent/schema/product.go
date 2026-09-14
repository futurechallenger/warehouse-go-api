package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Product 商品（货物主数据），通过 EAN-13 / EAN-8 / QR code 识别
type Product struct {
	ent.Schema
}

func (Product) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Comment("商品名称"),
		field.String("ean13").Optional().Default("").Comment("EAN-13 商品码"),
		field.String("ean8").Optional().Default("").Comment("EAN-8 商品码"),
		field.String("qr_code").Optional().Default("").Comment("QR code 内容"),
	}
}

func (Product) Edges() []ent.Edge {
	return []ent.Edge{
		// 一种商品可存放于多个单元
		edge.From("units", ShelfUnit.Type).Ref("product").Comment("存放该商品的货架单元"),
		// 一种商品可对应多个包裹
		edge.To("packages", GoodsPackage.Type).Comment("该商品的包裹"),
	}
}
