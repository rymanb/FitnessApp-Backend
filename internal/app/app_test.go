package app

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthEndpoint(t *testing.T) {
	app := Setup()
	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestUnknownRoute_Returns404(t *testing.T) {
	app := Setup()
	// Use a path outside the /api/v1/ group — protected group middleware would
	// intercept and return 401 for unknown paths under /api/v1/.
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 404, resp.StatusCode)
}

// Verify that protected routes are actually guarded — no token should yield 401.
func TestProtectedRoutes_RequireAuth(t *testing.T) {
	app := Setup()

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/plans"},
		{"POST", "/api/v1/plans/sync"},
		{"POST", "/api/v1/plans/generate"},
		{"GET", "/api/v1/history"},
		{"POST", "/api/v1/history/sync"},
		{"POST", "/api/v1/chat"},
	}

	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		resp, _ := app.Test(req)
		assert.Equal(t, 401, resp.StatusCode, "%s %s should require authentication", r.method, r.path)
	}
}
