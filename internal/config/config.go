package config

// Config 应用配置（从环境变量或配置文件加载）
type Config struct {
	// 服务器
	ListenAddr string // 默认 ":8080"

	// 数据库
	DBPath string // SQLite 文件路径，默认 "./data/app.db"

	// 认证
	JWTSecret      string // HMAC 签名密钥，生产环境必须设置
	JWTExpireHours int    // Token 有效期（小时），默认 72

	// 日志
	LogLevel string // "DEBUG" | "INFO" | "WARN" | "ERROR"，默认 "INFO"

	// AI 批改（写作模块）
	AIAPIKey      string // LLM API Key（Claude 或 OpenAI）
	AIAPIEndpoint string // API 端点 URL
	AITimeoutSec  int    // 请求超时秒数，默认 15

	// 文件存储（口语录音）
	AudioStorePath string // 录音文件存储目录，默认 "./data/audio"
}

// Load 从指定路径加载配置文件，返回 Config 指针。
// 当前为函数签名占位，实现在 Phase 2 补全。
func Load(path string) (*Config, error) {
	return nil, nil
}
