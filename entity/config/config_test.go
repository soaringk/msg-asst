package config

import (
	"os"
	"sync"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	os.Setenv("LLM_API_KEY", "test-key")
	os.Setenv("LLM_MODEL", "test-model")
	os.Setenv("SUMMARY_INTERVAL_MINUTES", "15")
	defer func() {
		os.Unsetenv("LLM_API_KEY")
		os.Unsetenv("LLM_MODEL")
		os.Unsetenv("SUMMARY_INTERVAL_MINUTES")
	}()

	err := Parse()
	if err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	cfg := GetConfig()
	if cfg == nil {
		t.Fatal("GetConfig() returned nil")
	}

	if cfg.LLMAPIKey != "test-key" {
		t.Errorf("LLMAPIKey = %q, want %q", cfg.LLMAPIKey, "test-key")
	}
	if cfg.LLMModel != "test-model" {
		t.Errorf("LLMModel = %q, want %q", cfg.LLMModel, "test-model")
	}
	if cfg.SummaryTrigger.IntervalMinutes != 15 {
		t.Errorf("IntervalMinutes = %d, want %d", cfg.SummaryTrigger.IntervalMinutes, 15)
	}
}

func TestGetConfigConcurrent(t *testing.T) {
	os.Setenv("LLM_API_KEY", "test-key")
	defer os.Unsetenv("LLM_API_KEY")

	if err := Parse(); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := GetConfig()
			if cfg == nil {
				t.Error("GetConfig() returned nil during concurrent access")
			}
		}()
	}
	wg.Wait()
}

func TestConfigCallback(t *testing.T) {
	os.Setenv("LLM_API_KEY", "test-key")
	defer os.Unsetenv("LLM_API_KEY")

	if err := Parse(); err != nil {
		t.Fatalf("Parse() failed: %v", err)
	}

	callbackCalled := make(chan bool, 1)
	OnConfigChange(func() {
		callbackCalled <- true
	})

	notifyConfigCallbacks()

	select {
	case <-callbackCalled:
	case <-time.After(1 * time.Second):
		t.Error("Callback was not called after notifyConfigCallbacks()")
	}
}

func TestSaveAndLoadGroups(t *testing.T) {
	testGroups := []string{"测试群1", "TestGroup-2", "群聊👍"}

	if err := SaveGroups(testGroups); err != nil {
		t.Fatalf("SaveGroups() failed: %v", err)
	}
	defer os.Remove(groupsFile)

	if err := LoadGroups(); err != nil {
		t.Fatalf("LoadGroups() failed: %v", err)
	}

	loaded := GetTargetGroups()
	if len(loaded) != len(testGroups) {
		t.Fatalf("GetTargetGroups() returned %d groups, want %d", len(loaded), len(testGroups))
	}

	for i, group := range testGroups {
		if loaded[i] != group {
			t.Errorf("Group[%d] = %q, want %q", i, loaded[i], group)
		}
	}
}

func TestGetTargetGroupsConcurrent(t *testing.T) {
	testGroups := []string{"group1", "group2"}
	if err := SaveGroups(testGroups); err != nil {
		t.Fatalf("SaveGroups() failed: %v", err)
	}
	defer os.Remove(groupsFile)

	if err := LoadGroups(); err != nil {
		t.Fatalf("LoadGroups() failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			groups := GetTargetGroups()
			if len(groups) != 2 {
				t.Errorf("GetTargetGroups() returned %d groups, want 2", len(groups))
			}
		}()
	}
	wg.Wait()
}
