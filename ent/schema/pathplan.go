package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// PathPlan 最优路径规划结果
type PathPlan struct {
	ent.Schema
}

func (PathPlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("plan_key").Unique().Comment("任务组合哈希键"),
		field.Int("total_dist").Default(0).Comment("总距离"),
		field.Text("order_json").Default("").Comment("最优访问顺序JSON"),
		field.Text("segments_json").Default("").Comment("各段路径JSON"),
		field.Int("warehouse_id").Optional().Comment("所属仓库ID"),
	}
}

func (PathPlan) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("warehouse", Warehouse.Type).Ref("path_plans").Unique().Field("warehouse_id").Comment("所属仓库"),
	}
}
