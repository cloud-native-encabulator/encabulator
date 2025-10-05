package config

import (
	"testing"
)

func TestNewStore(t *testing.T) {
	store := NewStore()

	if store == nil {
		t.Fatal("NewStore returned nil")
	}

	// Check default configs exist
	configs := store.ListConfigs()
	if len(configs) == 0 {
		t.Error("Expected default configs, got none")
	}

	// Check default flags exist
	flags := store.ListFlags()
	if len(flags) == 0 {
		t.Error("Expected default flags, got none")
	}
}

func TestConfigOperations(t *testing.T) {
	store := NewStore()

	// Test SetConfig and GetConfig
	cfg := &ConfigValue{
		Key:         "test.key",
		Value:       "test value",
		Type:        "string",
		Environment: "test",
	}

	store.SetConfig(cfg)

	retrieved, ok := store.GetConfig("test.key")
	if !ok {
		t.Fatal("Failed to retrieve config")
	}

	if retrieved.Key != cfg.Key {
		t.Errorf("Expected key %s, got %s", cfg.Key, retrieved.Key)
	}

	if retrieved.Value != cfg.Value {
		t.Errorf("Expected value %v, got %v", cfg.Value, retrieved.Value)
	}

	// Test DeleteConfig
	if !store.DeleteConfig("test.key") {
		t.Error("Failed to delete config")
	}

	_, ok = store.GetConfig("test.key")
	if ok {
		t.Error("Config still exists after deletion")
	}
}

func TestFlagOperations(t *testing.T) {
	store := NewStore()

	// Test SetFlag and GetFlag
	flag := &FeatureFlag{
		Name:        "test_flag",
		Enabled:     true,
		Description: "Test flag",
		Environment: "test",
		Rollout:     50,
	}

	store.SetFlag(flag)

	retrieved, ok := store.GetFlag("test_flag")
	if !ok {
		t.Fatal("Failed to retrieve flag")
	}

	if retrieved.Name != flag.Name {
		t.Errorf("Expected name %s, got %s", flag.Name, retrieved.Name)
	}

	if retrieved.Enabled != flag.Enabled {
		t.Errorf("Expected enabled %v, got %v", flag.Enabled, retrieved.Enabled)
	}

	// Test DeleteFlag
	if !store.DeleteFlag("test_flag") {
		t.Error("Failed to delete flag")
	}

	_, ok = store.GetFlag("test_flag")
	if ok {
		t.Error("Flag still exists after deletion")
	}
}

func TestEvaluateFlag(t *testing.T) {
	store := NewStore()

	// Test with enabled flag at 100% rollout
	flag := &FeatureFlag{
		Name:        "full_rollout",
		Enabled:     true,
		Description: "Full rollout flag",
		Environment: "test",
		Rollout:     100,
	}
	store.SetFlag(flag)

	if !store.EvaluateFlag("full_rollout", "user123") {
		t.Error("Expected flag to be enabled for user")
	}

	// Test with disabled flag
	disabledFlag := &FeatureFlag{
		Name:        "disabled_flag",
		Enabled:     false,
		Description: "Disabled flag",
		Environment: "test",
		Rollout:     100,
	}
	store.SetFlag(disabledFlag)

	if store.EvaluateFlag("disabled_flag", "user123") {
		t.Error("Expected flag to be disabled for user")
	}

	// Test with non-existent flag
	if store.EvaluateFlag("nonexistent", "user123") {
		t.Error("Expected non-existent flag to be disabled")
	}

	// Test with partial rollout
	partialFlag := &FeatureFlag{
		Name:        "partial_rollout",
		Enabled:     true,
		Description: "Partial rollout flag",
		Environment: "test",
		Rollout:     50,
	}
	store.SetFlag(partialFlag)

	// Test with multiple users to verify rollout logic
	enabledCount := 0
	for i := 0; i < 100; i++ {
		userID := string(rune('a' + i))
		if store.EvaluateFlag("partial_rollout", userID) {
			enabledCount++
		}
	}

	// With 50% rollout, we expect roughly 50% of users to have it enabled
	// Allow for some variance (30-70%)
	if enabledCount < 30 || enabledCount > 70 {
		t.Errorf("Expected roughly 50%% rollout, got %d%%", enabledCount)
	}
}
