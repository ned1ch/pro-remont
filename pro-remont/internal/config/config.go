// Package config загружает настройки приложения из переменных окружения
// и (необязательно) из файла .env рядом с исполняемым каталогом.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr            string
	DatabaseURL     string
	JWTSecret       string
	TokenTTL        time.Duration
	WebDir          string
	SeedDemo        bool
	LogLevel        string
	AllowedOrigins  []string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Load читает .env (если он есть) и переменные окружения.
// Переменные окружения имеют приоритет над .env.
func Load() (Config, error) {
	_ = loadDotEnv(".env")

	cfg := Config{
		Addr:            env("APP_ADDR", ":8080"),
		DatabaseURL:     env("DATABASE_URL", "postgres://postgres@127.0.0.1:5432/pro_remont?sslmode=disable"),
		JWTSecret:       env("JWT_SECRET", ""),
		TokenTTL:        envDuration("TOKEN_TTL", 24*time.Hour),
		WebDir:          env("WEB_DIR", "web"),
		SeedDemo:        envBool("SEED_DEMO", true),
		LogLevel:        env("LOG_LEVEL", "info"),
		AllowedOrigins:  splitList(env("ALLOWED_ORIGINS", "")),
		ReadTimeout:     envDuration("READ_TIMEOUT", 15*time.Second),
		WriteTimeout:    envDuration("WRITE_TIMEOUT", 30*time.Second),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	if cfg.JWTSecret == "" {
		// Для локального запуска без .env генерируем временный секрет.
		// При перезапуске сервера сессии станут недействительны.
		secret, err := randomSecret(32)
		if err != nil {
			return Config{}, fmt.Errorf("generate jwt secret: %w", err)
		}
		cfg.JWTSecret = secret
	}

	return cfg, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err == nil {
			return b
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err == nil {
			return d
		}
	}
	return def
}

func splitList(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// loadDotEnv — минимальная реализация без внешних зависимостей.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return sc.Err()
}

func randomSecret(n int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, n)
	if _, err := cryptoRead(buf); err != nil {
		return "", err
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf), nil
}
