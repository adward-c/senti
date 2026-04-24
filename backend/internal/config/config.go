package config

import (
	"errors"
	"os"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	CORSOrigin        string
	UploadDir         string
	TempUploadDir     string
	KimiAPIKey        string
	KimiBaseURL       string
	KimiModel         string
	OCRLanguage       string
	InviteCode        string
	AuthTokenSecret   string
	AnalyzeTimeout    string
	RateLimitWindow   string
	RateLimitRequests int
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		CORSOrigin:        getEnv("CORS_ORIGIN", "http://localhost"),
		UploadDir:         getEnv("UPLOAD_DIR", "/app/uploads"),
		TempUploadDir:     getEnv("TEMP_UPLOAD_DIR", "/app/uploads/tmp"),
		KimiAPIKey:        firstEnv("KIMI_API_KEY", "MOONSHOT_API_KEY"),
		KimiBaseURL:       getEnv("KIMI_BASE_URL", "https://api.moonshot.cn/v1"),
		KimiModel:         getEnv("KIMI_MODEL", "kimi-k2.6"),
		OCRLanguage:       getEnv("OCR_LANG", "chi_sim+eng"),
		InviteCode:        os.Getenv("INVITE_CODE"),
		AuthTokenSecret:   getEnv("AUTH_TOKEN_SECRET", "dev-secret-change-me"),
		AnalyzeTimeout:    getEnv("ANALYZE_TIMEOUT", "60s"),
		RateLimitWindow:   getEnv("RATE_LIMIT_WINDOW", "24h"),
		RateLimitRequests: getEnvInt("RATE_LIMIT_REQUESTS", 15),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	return cfg, nil
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	var parsed int
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return fallback
		}
		parsed = parsed*10 + int(ch-'0')
	}
	if parsed <= 0 {
		return fallback
	}
	return parsed
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}
