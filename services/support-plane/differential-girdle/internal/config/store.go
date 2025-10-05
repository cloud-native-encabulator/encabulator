package config

import (
	"sync"
	"time"
)

// ConfigValue represents a configuration value with metadata
type ConfigValue struct {
	Key         string                 `json:"key"`
	Value       interface{}            `json:"value"`
	Type        string                 `json:"type"`
	Environment string                 `json:"environment"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// FeatureFlag represents a feature flag
type FeatureFlag struct {
	Name        string                 `json:"name"`
	Enabled     bool                   `json:"enabled"`
	Description string                 `json:"description"`
	Environment string                 `json:"environment"`
	Rollout     int                    `json:"rollout"` // Percentage 0-100
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Store manages configuration values and feature flags
type Store struct {
	configs map[string]*ConfigValue
	flags   map[string]*FeatureFlag
	mu      sync.RWMutex
}

// NewStore creates a new configuration store
func NewStore() *Store {
	store := &Store{
		configs: make(map[string]*ConfigValue),
		flags:   make(map[string]*FeatureFlag),
	}

	// Initialize with some default configurations
	store.initDefaults()

	return store
}

func (s *Store) initDefaults() {
	// Default configurations
	s.configs["encabulator.max_grammeters"] = &ConfigValue{
		Key:         "encabulator.max_grammeters",
		Value:       100,
		Type:        "integer",
		Environment: "all",
		UpdatedAt:   time.Now(),
	}

	s.configs["encabulator.prefabulation_enabled"] = &ConfigValue{
		Key:         "encabulator.prefabulation_enabled",
		Value:       true,
		Type:        "boolean",
		Environment: "all",
		UpdatedAt:   time.Now(),
	}

	// Default feature flags
	s.flags["sinusoidal_mode"] = &FeatureFlag{
		Name:        "sinusoidal_mode",
		Enabled:     true,
		Description: "Enable sinusoidal depleneration mode",
		Environment: "all",
		Rollout:     100,
		UpdatedAt:   time.Now(),
	}

	s.flags["logarithmic_casing"] = &FeatureFlag{
		Name:        "logarithmic_casing",
		Enabled:     false,
		Description: "Enable logarithmic casing optimization",
		Environment: "all",
		Rollout:     0,
		UpdatedAt:   time.Now(),
	}
}

// GetConfig retrieves a configuration value
func (s *Store) GetConfig(key string) (*ConfigValue, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	val, ok := s.configs[key]
	return val, ok
}

// SetConfig sets a configuration value
func (s *Store) SetConfig(config *ConfigValue) {
	s.mu.Lock()
	defer s.mu.Unlock()

	config.UpdatedAt = time.Now()
	s.configs[config.Key] = config
}

// ListConfigs returns all configuration values
func (s *Store) ListConfigs() []*ConfigValue {
	s.mu.RLock()
	defer s.mu.RUnlock()

	configs := make([]*ConfigValue, 0, len(s.configs))
	for _, config := range s.configs {
		configs = append(configs, config)
	}
	return configs
}

// DeleteConfig removes a configuration value
func (s *Store) DeleteConfig(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.configs[key]; ok {
		delete(s.configs, key)
		return true
	}
	return false
}

// GetFlag retrieves a feature flag
func (s *Store) GetFlag(name string) (*FeatureFlag, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	flag, ok := s.flags[name]
	return flag, ok
}

// SetFlag sets a feature flag
func (s *Store) SetFlag(flag *FeatureFlag) {
	s.mu.Lock()
	defer s.mu.Unlock()

	flag.UpdatedAt = time.Now()
	s.flags[flag.Name] = flag
}

// ListFlags returns all feature flags
func (s *Store) ListFlags() []*FeatureFlag {
	s.mu.RLock()
	defer s.mu.RUnlock()

	flags := make([]*FeatureFlag, 0, len(s.flags))
	for _, flag := range s.flags {
		flags = append(flags, flag)
	}
	return flags
}

// DeleteFlag removes a feature flag
func (s *Store) DeleteFlag(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.flags[name]; ok {
		delete(s.flags, name)
		return true
	}
	return false
}

// EvaluateFlag evaluates a feature flag for a given context
func (s *Store) EvaluateFlag(name string, userID string) bool {
	flag, ok := s.GetFlag(name)
	if !ok {
		return false
	}

	if !flag.Enabled {
		return false
	}

	// Simple rollout logic based on user ID hash
	if flag.Rollout < 100 {
		// Hash user ID to determine if they're in the rollout
		hash := 0
		for _, c := range userID {
			hash = (hash*31 + int(c)) % 100
		}
		return hash < flag.Rollout
	}

	return true
}
