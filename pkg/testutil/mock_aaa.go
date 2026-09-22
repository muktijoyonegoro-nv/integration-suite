package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SetupWireMockAAA configures WireMock to return valid AAA authentication claims for any auth request.
func SetupWireMockAAA(ctx context.Context, wireMockBaseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	// Catch-all stub for AAA token validation
	stub := map[string]interface{}{
		"request": map[string]interface{}{
			"method":         "ANY",
			"urlPathPattern": ".*",
		},
		"response": map[string]interface{}{
			"status": 200,
			"headers": map[string]string{
				"Content-Type": "application/json",
			},
			"jsonBody": map[string]interface{}{
				"status":         "OK",
				"id":             1001,
				"accessToken":    "test-valid-token",
				"expirationDate": "2035-01-01T00:00:00Z",
				"data": map[string]interface{}{
					"user_id":   1001,
					"username":  "integration_test_operator",
					"email":     "test@ninjavan.co",
					"client_id": "integration-harness",
					"scopes": []string{
						"SORT_TASKS_ADMIN",
						"SORT_WAREHOUSE_TASKS_MANAGE",
						"SORT_WAREHOUSE_TASKS_VIEW",
						"CORE_GET_WAREHOUSE_SWEEP",
						"MANAGE_FACILITIES",
					},
				},
				"scopes": []string{
					"SORT_TASKS_ADMIN",
					"SORT_WAREHOUSE_TASKS_MANAGE",
					"SORT_WAREHOUSE_TASKS_VIEW",
					"CORE_GET_WAREHOUSE_SWEEP",
					"MANAGE_FACILITIES",
				},
				"globalScopes": map[string]interface{}{
					"sg": []string{
						"SORT_TASKS_ADMIN",
						"SORT_WAREHOUSE_TASKS_MANAGE",
						"SORT_WAREHOUSE_TASKS_VIEW",
						"CORE_GET_WAREHOUSE_SWEEP",
						"MANAGE_FACILITIES",
					},
					"global": []string{
						"SORT_TASKS_ADMIN",
						"SORT_WAREHOUSE_TASKS_MANAGE",
						"SORT_WAREHOUSE_TASKS_VIEW",
						"CORE_GET_WAREHOUSE_SWEEP",
						"MANAGE_FACILITIES",
					},
				},
			},
		},
	}

	payload, err := json.Marshal(stub)
	if err != nil {
		return fmt.Errorf("failed marshaling wiremock stub: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wireMockBaseURL+"/__admin/mappings", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed creating wiremock request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed sending stub to wiremock at %s: %w", wireMockBaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("wiremock admin returned non-2xx status: %d", resp.StatusCode)
	}

	return nil
}
