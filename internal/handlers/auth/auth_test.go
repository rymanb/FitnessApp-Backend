package auth

import (
	"bytes"
	"encoding/json"
	"fitness-backend/internal/testutils"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGoogleAuth_Failures(t *testing.T) {
	// Use the SetupTestApp we wrote earlier!
	app, _ := testutils.SetupTestApp(t)

	app.Post("/api/v1/auth/google", GoogleAuth)

	t.Run("Invalid JSON Body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/auth/google", bytes.NewReader([]byte("{bad json}")))
		req.Header.Set("Content-Type", "application/json")

		resp, _ := app.Test(req)
		assert.Equal(t, 400, resp.StatusCode, "Should return 400 Bad Request for malformed JSON")
	})

	t.Run("Invalid Google Token", func(t *testing.T) {
		// Send a fake token that Google's validation will reject
		body, _ := json.Marshal(AuthRequest{IDToken: "fake_google_token_123"})
		req := httptest.NewRequest("POST", "/api/v1/auth/google", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		resp, _ := app.Test(req)
		assert.Equal(t, 401, resp.StatusCode, "Should return 401 Unauthorized for fake Google tokens")
	})
}
