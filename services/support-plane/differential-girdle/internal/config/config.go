package config

import "os"

// Config holds application configuration
type Config struct {
	Port        string
	Environment string
}

// Load loads configuration from environment variables
func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "development"
	}

	return &Config{
		Port:        port,
		Environment: env,
	}
}
