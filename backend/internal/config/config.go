package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config 是应用的完整配置树，对应 configs/config.yaml。
// 业务代码只依赖结构化配置，避免在代码里散落硬编码。
type Config struct {
	App     AppConfig     `mapstructure:"app"`
	Server  ServerConfig  `mapstructure:"server"`
	Log     LogConfig     `mapstructure:"log"`
	MySQL   MySQLConfig   `mapstructure:"mysql"`
	Redis   RedisConfig   `mapstructure:"redis"`
	Milvus  MilvusConfig  `mapstructure:"milvus"`
	Storage StorageConfig `mapstructure:"storage"`
	AI      AIConfig      `mapstructure:"ai"`
}

// AppConfig 描述应用自身的运行环境和启动行为。
type AppConfig struct {
	Name        string `mapstructure:"name"`
	Env         string `mapstructure:"env"`
	AutoMigrate bool   `mapstructure:"auto_migrate"`
}

// ServerConfig 描述 Gin HTTP 服务监听配置。
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

// Addr 返回 net/http Server 可直接使用的监听地址。
func (c ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// LogConfig 描述 zap 日志输出格式和级别。
type LogConfig struct {
	Level      string `mapstructure:"level"`
	Encoding   string `mapstructure:"encoding"`
	Stacktrace bool   `mapstructure:"stacktrace"`
}

// MySQLConfig 描述 Gorm 连接池和 DSN。
type MySQLConfig struct {
	DSN             string `mapstructure:"dsn"`
	MaxIdleConns    int    `mapstructure:"max_idle_conns"`
	MaxOpenConns    int    `mapstructure:"max_open_conns"`
	ConnMaxLifetime string `mapstructure:"conn_max_lifetime"`
}

// RedisConfig 预留给缓存、限流、异步任务状态等场景。
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	Enabled  bool   `mapstructure:"enabled"`
}

// MilvusConfig 描述向量库地址、集合名和 embedding 维度。
type MilvusConfig struct {
	Addr           string `mapstructure:"addr"`
	CollectionName string `mapstructure:"collection_name"`
	EmbeddingDim   int    `mapstructure:"embedding_dim"`
	MetricType     string `mapstructure:"metric_type"`
	IndexType      string `mapstructure:"index_type"`
}

// StorageConfig 描述本地上传文件保存位置。
// 第一版先保存到本地目录，后续可以替换成对象存储。
type StorageConfig struct {
	UploadDir string `mapstructure:"upload_dir"`
}

// AIConfig 描述 LLM 和 embedding 服务，第一版按 OpenAI-compatible 接口预留。
type AIConfig struct {
	Provider         string `mapstructure:"provider"`
	AgentMode        string `mapstructure:"agent_mode"`
	ChatModel        string `mapstructure:"chat_model"`
	EmbeddingModel   string `mapstructure:"embedding_model"`
	BaseURL          string `mapstructure:"base_url"`
	APIKey           string `mapstructure:"api_key"`
	ChatBaseURL      string `mapstructure:"chat_base_url"`
	ChatAPIKey       string `mapstructure:"chat_api_key"`
	EmbeddingBaseURL string `mapstructure:"embedding_base_url"`
	EmbeddingAPIKey  string `mapstructure:"embedding_api_key"`
}

// Load 读取配置文件并合并 BAGU_ 前缀的环境变量。
// 例如 BAGU_MYSQL_DSN 可以覆盖 mysql.dsn。
func Load(path string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	if path == "" {
		path = "configs/config.yaml"
	}
	if err := loadDotEnv(path); err != nil {
		return nil, err
	}
	v.SetConfigFile(path)
	v.SetEnvPrefix("BAGU")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

// setDefaults 提供本地开发可用的默认值，配置文件仍然拥有更高优先级。
// loadDotEnv 读取本地 .env 文件，方便开发环境保存 API Key。
// 已存在的系统环境变量优先级更高，因此手动设置的值不会被 .env 覆盖。
func loadDotEnv(configPath string) error {
	candidates := []string{".env"}
	if configPath != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(configPath), "..", ".env"))
	}
	for _, candidate := range candidates {
		if err := loadDotEnvFile(filepath.Clean(candidate)); err != nil {
			return err
		}
	}
	return nil
}

func loadDotEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open .env file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("parse .env file %s line %d: missing '='", path, lineNumber)
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		value = trimEnvValue(value)
		if key == "" {
			return fmt.Errorf("parse .env file %s line %d: empty key", path, lineNumber)
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s from %s: %w", key, path, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read .env file %s: %w", path, err)
	}
	return nil
}

func trimEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if commentIdx := strings.Index(value, " #"); commentIdx >= 0 {
		value = strings.TrimSpace(value[:commentIdx])
	}
	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			return strings.Trim(value, `"`)
		}
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return strings.Trim(value, `'`)
		}
	}
	return value
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "BaguAgent")
	v.SetDefault("app.env", "local")
	v.SetDefault("app.auto_migrate", false)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.encoding", "console")
	v.SetDefault("log.stacktrace", false)
	v.SetDefault("mysql.max_idle_conns", 5)
	v.SetDefault("mysql.max_open_conns", 20)
	v.SetDefault("mysql.conn_max_lifetime", "1h")
	v.SetDefault("redis.enabled", false)
	v.SetDefault("milvus.collection_name", "bagu_chunks_v1")
	v.SetDefault("milvus.embedding_dim", 1024)
	v.SetDefault("milvus.metric_type", "COSINE")
	v.SetDefault("milvus.index_type", "HNSW")
	v.SetDefault("storage.upload_dir", "uploads")
	v.SetDefault("ai.agent_mode", "graph")
}
