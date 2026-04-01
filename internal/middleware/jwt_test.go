package middleware

import (
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

func setupMiddlewareApp() *fiber.App {
	os.Setenv("JWT_SECRET", "test_secret_123")
	app := fiber.New()
	app.Get("/test", Protected(), func(c *fiber.Ctx) error {
		// Echo the userID back so we can assert it was set correctly.
		return c.SendString(c.Locals("userID").(string))
	})
	return app
}

func makeToken(secret string, sub string, exp time.Time) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": sub,
		"exp": exp.Unix(),
	})
	s, _ := token.SignedString([]byte(secret))
	return s
}

func TestProtected_MissingToken(t *testing.T) {
	app := setupMiddlewareApp()
	req := httptest.NewRequest("GET", "/test", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestProtected_MissingBearerPrefix(t *testing.T) {
	app := setupMiddlewareApp()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", makeToken("test_secret_123", "u1", time.Now().Add(time.Hour)))
	resp, _ := app.Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestProtected_InvalidToken(t *testing.T) {
	app := setupMiddlewareApp()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	resp, _ := app.Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestProtected_WrongSecret(t *testing.T) {
	app := setupMiddlewareApp()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken("wrong_secret", "u1", time.Now().Add(time.Hour)))
	resp, _ := app.Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestProtected_ExpiredToken(t *testing.T) {
	app := setupMiddlewareApp()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken("test_secret_123", "u1", time.Now().Add(-time.Hour)))
	resp, _ := app.Test(req)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestProtected_ValidToken_SetsUserID(t *testing.T) {
	app := setupMiddlewareApp()
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken("test_secret_123", "user-abc", time.Now().Add(time.Hour)))
	resp, _ := app.Test(req)

	assert.Equal(t, 200, resp.StatusCode)

	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	assert.Equal(t, "user-abc", string(buf[:n]))
}
