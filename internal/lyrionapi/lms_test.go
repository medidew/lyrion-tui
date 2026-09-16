package lyrionapi_test

import (
	"testing"

	lyrionapi "github.com/medidew/lyrion-tui/internal"
)

func TestConnection(t *testing.T) {
	server, err := lyrionapi.Connect("192.168.1.4:9090") // TODO: abstract this into a config file/env variable
	if err != nil {
		t.Errorf("Failed to connect to test server: %v", err)
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
