package config

// =============================================================================
//  internal/config — 配置加载
// =============================================================================

import "os"

// Config 应用配置
type Config struct {
	Server   ServerConfig
	Database DBConfig
	Redis    RedisConfig
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Port string
	Mode string // debug / release / test
}

// DBConfig 数据库配置
type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	DSN      string
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	Addr     string
}

// Load 从环境变量加载配置
func Load() *Config {
	cfg := &Config{}

	// Server
	cfg.Server.Port = getEnv("PORT", "8080")
	cfg.Server.Mode = getEnv("GIN_MODE", "release")

	// Database
	cfg.Database.Host = getEnv("DB_HOST", "localhost")
	cfg.Database.Port = getEnv("DB_PORT", "5432")
	cfg.Database.User = getEnv("DB_USER", "warehouse")
	cfg.Database.Password = getEnv("DB_PASSWORD", "warehouse123")
	cfg.Database.Name = getEnv("DB_NAME", "warehouse")
	cfg.Database.DSN = "host=" + cfg.Database.Host +
		" user=" + cfg.Database.User +
		" password=" + cfg.Database.Password +
		" dbname=" + cfg.Database.Name +
		" port=" + cfg.Database.Port +
		" sslmode=disable TimeZone=Asia/Shanghai"

	// Redis
	cfg.Redis.Host = getEnv("REDIS_HOST", "localhost")
	cfg.Redis.Port = getEnv("REDIS_PORT", "6379")
	cfg.Redis.Password = getEnv("REDIS_PASSWORD", "")
	cfg.Redis.Addr = cfg.Redis.Host + ":" + cfg.Redis.Port

	return cfg
}

// getEnv 获取环境变量，带默认值
func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
