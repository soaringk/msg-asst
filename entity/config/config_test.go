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

func TestSaveAndLoadRooms(t *testing.T) {
	testRooms := []string{"测试群1", "TestRoom-2", "群聊👍"}

	if err := SaveRooms(testRooms); err != nil {
		t.Fatalf("SaveRooms() failed: %v", err)
	}
	defer os.Remove(roomsFile)

	if err := LoadRooms(); err != nil {
		t.Fatalf("LoadRooms() failed: %v", err)
	}

	loaded := GetTargetRooms()
	if len(loaded) != len(testRooms) {
		t.Fatalf("GetTargetRooms() returned %d rooms, want %d", len(loaded), len(testRooms))
	}

	for i, room := range testRooms {
		if loaded[i] != room {
			t.Errorf("Room[%d] = %q, want %q", i, loaded[i], room)
		}
	}
}

func TestGetTargetRoomsConcurrent(t *testing.T) {
	testRooms := []string{"room1", "room2"}
	if err := SaveRooms(testRooms); err != nil {
		t.Fatalf("SaveRooms() failed: %v", err)
	}
	defer os.Remove(roomsFile)

	if err := LoadRooms(); err != nil {
		t.Fatalf("LoadRooms() failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rooms := GetTargetRooms()
			if len(rooms) != 2 {
				t.Errorf("GetTargetRooms() returned %d rooms, want 2", len(rooms))
			}
		}()
	}
	wg.Wait()
}
