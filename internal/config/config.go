package config

import (
	"os"
	"strconv"
)

type Config struct {
	DBHost              string
	DBPort              string
	DBUser              string
	DBPassword          string
	DBName              string
	ServerPort          string
	RedisAddr           string
	RedisPassword       string
	RedisChannel        string
	WorkerConcurrency   int
	WorkerMaxRetry      int
	MetricsPort         string
	ExternalProviderURL string
}

func LoadConfig() *Config {
	return &Config{
		DBHost:              getEnv("DB_HOST", "localhost"),
		DBPort:              getEnv("DB_PORT", "10000"),
		DBUser:              getEnv("DB_USER", "postgres"),
		DBPassword:          getEnv("DB_PASSWORD", "postgres"),
		DBName:              getEnv("DB_NAME", "notif_db"),
		ServerPort:          getEnv("SERVER_PORT", "8080"),
		RedisAddr:           getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:       getEnv("REDIS_PASSWORD", ""),
		RedisChannel:        getEnv("REDIS_CHANNEL", "notifications.batch.created"),
		WorkerConcurrency:   getEnvInt("WORKER_CONCURRENCY", 100),
		WorkerMaxRetry:      getEnvInt("WORKER_MAX_RETRY", 3),
		MetricsPort:         getEnv("METRICS_PORT", "9091"),
		ExternalProviderURL: getEnv("EXTERNAL_PROVIDER_URL", ""),
	}
}

func getEnv(key, defaultVal string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultVal
}
