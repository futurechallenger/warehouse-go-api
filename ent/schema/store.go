package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Store 门店
type Store struct {
	ent.Schema
}

func (Store) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").Default("").Comment("门店名称"),
		field.String("code").Unique().Comment("门店编码"),
	}
}

func (Store) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("warehouses", Warehouse.Type).Comment("该店下的所有仓库"),
	}
}
