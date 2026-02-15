package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	DatabaseURL  string `yaml:"database_url"`
	ReposPath    string `yaml:"repos_path"`
	PoolPath     string `yaml:"pool_path"`
	BackLog      int    `yaml:"back_log"`
	CookieSecret string `yaml:"cookiesecret"`
}

func LoadConfig() (Config, error) {
	var config Config

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to parse yaml: %w", err)
	}

	return config, nil
}
