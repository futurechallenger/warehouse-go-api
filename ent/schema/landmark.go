package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Landmark 仓库内的位置点（地标）。
// 与货架不同，它不承载任何存储单元、也不存放货物，仅提供网格坐标，
// 用于表示入口、出口、装卸位、充电桩，以及搬货人员/机器人的当前位置。
type Landmark struct {
	ent.Schema
}

func (Landmark) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").Comment("仓库内唯一编码，如 ENTRANCE / EXIT / ROBOT-01"),
		field.String("name").Default("").Comment("显示名称"),
		field.String("kind").Default("waypoint").
			Comment("entrance/exit/dock/charger/person/robot/waypoint"),
		field.Int("col").Default(0).Comment("网格列坐标"),
		field.Int("row").Default(0).Comment("网格行坐标"),
		field.Bool("is_movable").Default(false).Comment("是否为可移动位置（人员/机器人）"),
		field.String("note").Default("").Comment("备注"),
		field.Int("warehouse_id").Optional().Comment("所属仓库ID"),
	}
}

func (Landmark) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("warehouse", Warehouse.Type).Ref("landmarks").Unique().
			Field("warehouse_id").Comment("所属仓库"),
	}
}

func (Landmark) Indexes() []ent.Index {
	return []ent.Index{
		// 位置编码在仓库范围内唯一
		index.Fields("code").Edges("warehouse").Unique(),
	}
}
