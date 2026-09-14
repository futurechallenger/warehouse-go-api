# Warehouse Go API

基于 Gin + ent + PostgreSQL + Redis 的仓库管理系统后端。

## 项目结构

```
warehouse/
├── cmd/
│   └── main.go            # 入口
├── internal/
│   ├── config/
│   │   └── config.go     # 配置加载
│   ├── ent/              # ent 生成代码（schema 在 ent/schema/）
│   ├── repository/
│   │   └── repository.go # 数据访问层
│   ├── service/
│   │   └── service.go    # 业务逻辑（路径规划等）
│   ├── handler/
│   │   └── handler.go    # Gin HTTP handlers
│   ├── router/
│   │   └── router.go     # 路由注册
│   └── utils/
│       └── cal.go         # 工具函数（BFS + TSP）
├── Dockerfile              # 生产构建
├── Dockerfile.dev          # 开发构建（含 air）
├── docker-compose.yml      # 完整服务编排
├── .air.toml              # air 热重载配置
├── go.mod
└── go.sum
```

## 快速启动

```bash
# 一键启动所有服务
docker compose up -d

# 查看日志
docker compose logs -f app

# 测试
curl http://localhost:8080/ping
curl http://localhost:8080/api/shelves
curl http://localhost:8080/api/path/plan
```

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| GIN_MODE | release | Gin 运行模式 |
| DB_HOST | postgres | 数据库地址 |
| DB_PORT | 5432 | 数据库端口 |
| DB_USER | warehouse | 数据库用户 |
| DB_PASSWORD | warehouse123 | 数据库密码 |
| DB_NAME | warehouse | 数据库名 |
| REDIS_HOST | redis | Redis 地址 |
| REDIS_PORT | 6379 | Redis 端口 |
