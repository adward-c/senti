package config

import (
	"errors"
	"os"
)

type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	CORSOrigin    string
	UploadDir     string
	ChatSkillsDir string
	KimiAPIKey    string
	KimiBaseURL   string
	KimiModel     string
	OCRLanguage   string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:      getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		CORSOrigin:    getEnv("CORS_ORIGIN", "http://localhost"),
		UploadDir:     getEnv("UPLOAD_DIR", "/app/uploads"),
		ChatSkillsDir: getEnv("CHAT_SKILLS_DIR", "/app/chat-skills"),
		KimiAPIKey:    firstEnv("KIMI_API_KEY", "MOONSHOT_API_KEY"),
		KimiBaseURL:   getEnv("KIMI_BASE_URL", "https://api.moonshot.cn/v1"),
		KimiModel:     getEnv("KIMI_MODEL", "kimi-k2.6"),
		OCRLanguage:   getEnv("OCR_LANG", "chi_sim+eng"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	return cfg, nil
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
