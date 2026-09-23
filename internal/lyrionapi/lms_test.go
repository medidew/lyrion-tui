package lyrionapi_test

import (
	"testing"

	"github.com/medidew/lyrion-tui/internal/config"
	"github.com/medidew/lyrion-tui/internal/lyrionapi"
)

func TestConnection(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skipf("no LMS server configured: %v", err)
	}

	server, err := lyrionapi.Connect(cfg.ServerAddress)
	if err != nil {
		t.Fatalf("Failed to connect to test server: %v", err)
	}
	defer server.Close()

	response, err := server.Query("nonsense")
	if err != nil {
		t.Errorf("Failed to query test server: %v", err)
	}
	if response != "nonsense" {
		t.Errorf("Server did not echo expected response.")
	}
}

// TODO: rest of basic LMS function tests
