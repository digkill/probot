package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type TelegramConfig struct {
	Enabled     bool
	AppID       int
	AppHash     string
	RPCRate     float64
	MaxAccounts int
	AuthTTL     time.Duration
}

type Config struct {
	TelegramClient TelegramConfig
	AppEnv         string
	HTTPAddr       string
	DatabaseURL    string
	RedisURL       string
	JWTSecret      string
	EncryptionKey  string

	OpenAIAPIKey  string
	OpenAIBaseURL string
	OpenAIModel   string

	AnthropicAPIKey string
	AnthropicModel  string
	GrokAPIKey      string
	GrokModel       string
	GeminiAPIKey    string
	GeminiModel     string

	ImageAPIKey  string
	ImageBaseURL string
	ImageModel   string

	TelegramBotToken string
	VKAccessToken    string
	BlueskyHandle    string
	BlueskyAppPass   string

	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool

	PublicBaseURL string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AppEnv:           getEnv("APP_ENV", "development"),
		HTTPAddr:         getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		RedisURL:         getEnv("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:        getEnv("JWT_SECRET", "dev-secret"),
		EncryptionKey:    getEnv("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef"),
		OpenAIAPIKey:     getEnv("OPENAI_API_KEY", ""),
		OpenAIBaseURL:    getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIModel:      getEnv("OPENAI_MODEL", "gpt-4o-mini"),
		AnthropicAPIKey:  firstEnv("ANTHROPIC_API_KEY", "CLAUDE_API_KEY"),
		AnthropicModel:   getEnv("ANTHROPIC_MODEL", "claude-sonnet-4-5"),
		GrokAPIKey:       firstEnv("GROK_API_KEY", "XAI_API_KEY"),
		GrokModel:        getEnv("GROK_MODEL", "grok-3"),
		GeminiAPIKey:     firstEnv("GEMINI_API_KEY", "GOOGLE_API_KEY"),
		GeminiModel:      getEnv("GEMINI_MODEL", "gemini-2.5-flash"),
		ImageAPIKey:      getEnv("IMAGE_API_KEY", ""),
		ImageBaseURL:     getEnv("IMAGE_BASE_URL", "https://api.openai.com/v1"),
		ImageModel:       getEnv("IMAGE_MODEL", "dall-e-3"),
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		VKAccessToken:    getEnv("VK_ACCESS_TOKEN", ""),
		BlueskyHandle:    getEnv("BLUESKY_HANDLE", ""),
		BlueskyAppPass:   getEnv("BLUESKY_APP_PASSWORD", ""),
		S3Endpoint:       getEnv("S3_ENDPOINT", "http://localhost:9000"),
		S3AccessKey:      getEnv("S3_ACCESS_KEY", "probot"),
		S3SecretKey:      getEnv("S3_SECRET_KEY", "probotsecret"),
		S3Bucket:         getEnv("S3_BUCKET", "probot"),
		S3UseSSL:         getEnvBool("S3_USE_SSL", false),
		PublicBaseURL:    getEnv("PUBLIC_BASE_URL", "http://localhost:8080"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.EncryptionKey) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must be 32 bytes")
	}
	enabled, err := strconv.ParseBool(getEnv("TELEGRAM_CLIENT_ENABLED", "false"))
	if err != nil {
		return nil, fmt.Errorf("TELEGRAM_CLIENT_ENABLED must be a boolean")
	}
	cfg.TelegramClient = TelegramConfig{Enabled: enabled, RPCRate: 1, MaxAccounts: 10, AuthTTL: 5 * time.Minute}
	if enabled {
		cfg.TelegramClient.AppID, err = strconv.Atoi(getEnv("TELEGRAM_APP_ID", "0"))
		if err != nil || cfg.TelegramClient.AppID <= 0 {
			return nil, fmt.Errorf("TELEGRAM_APP_ID must be a positive integer")
		}
		cfg.TelegramClient.AppHash = getEnv("TELEGRAM_APP_HASH", "")
		if cfg.TelegramClient.AppHash == "" {
			return nil, fmt.Errorf("TELEGRAM_APP_HASH is required")
		}
		cfg.TelegramClient.RPCRate, err = strconv.ParseFloat(getEnv("TELEGRAM_RPC_RATE", "1"), 64)
		if err != nil || !(cfg.TelegramClient.RPCRate > 0 && cfg.TelegramClient.RPCRate <= 10) {
			return nil, fmt.Errorf("TELEGRAM_RPC_RATE must be between 0 and 10")
		}
		cfg.TelegramClient.MaxAccounts, err = strconv.Atoi(getEnv("TELEGRAM_MAX_ACCOUNTS", "10"))
		if err != nil || cfg.TelegramClient.MaxAccounts < 1 || cfg.TelegramClient.MaxAccounts > 100 {
			return nil, fmt.Errorf("TELEGRAM_MAX_ACCOUNTS must be between 1 and 100")
		}
		if cfg.EncryptionKey == "0123456789abcdef0123456789abcdef" {
			return nil, fmt.Errorf("set a private ENCRYPTION_KEY before enabling Telegram clients")
		}
	}
	return cfg, nil
}

func (c *Config) ResolveAPIKey(envName string) string {
	if envName == "" {
		return c.OpenAIAPIKey
	}
	if v := os.Getenv(envName); v != "" {
		return v
	}
	switch strings.ToUpper(envName) {
	case "OPENAI_API_KEY":
		return c.OpenAIAPIKey
	case "ANTHROPIC_API_KEY", "CLAUDE_API_KEY":
		return c.AnthropicAPIKey
	case "GROK_API_KEY", "XAI_API_KEY":
		return c.GrokAPIKey
	case "GEMINI_API_KEY", "GOOGLE_API_KEY":
		return c.GeminiAPIKey
	case "IMAGE_API_KEY":
		if c.ImageAPIKey != "" {
			return c.ImageAPIKey
		}
		return c.OpenAIAPIKey
	default:
		return ""
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
