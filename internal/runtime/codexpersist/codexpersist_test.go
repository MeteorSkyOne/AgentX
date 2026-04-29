package codexpersist

import "testing"

func TestInitializeParamsEnableExperimentalAPI(t *testing.T) {
	params := initializeParams()

	capabilities, ok := params["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("capabilities = %#v, want map", params["capabilities"])
	}
	if got := capabilities["experimentalApi"]; got != true {
		t.Fatalf("experimentalApi = %v, want true", got)
	}
}
