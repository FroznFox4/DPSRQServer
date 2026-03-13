package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kirill/gamelogserver/internal/middleware"
	"github.com/kirill/gamelogserver/internal/repository"
	"github.com/kirill/gamelogserver/internal/service"
)

// makeToken creates a HS256 JWT with the same claim layout that AuthService uses.
func makeToken(t *testing.T, secret string, userID int, username string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("makeToken: %v", err)
	}
	return signed
}

// newAuthSvc creates an AuthService that can validate tokens (DB is never hit here).
func newAuthSvc(secret string) *service.AuthService {
	return service.NewAuthService((*repository.UserRepo)(nil), secret)
}

func TestJWTAuth_ValidToken_SetsContextValues(t *testing.T) {
	const secret = "test-secret"
	token := makeToken(t, secret, 42, "alice")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	var gotUserID int
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = middleware.GetUserID(r.Context())
	})

	rr := httptest.NewRecorder()
	middleware.JWTAuth(newAuthSvc(secret))(next).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if gotUserID != 42 {
		t.Errorf("expected userID=42, got %d", gotUserID)
	}
}

func TestJWTAuth_MissingAuthHeader_Returns401(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	middleware.JWTAuth(newAuthSvc("secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not be reached")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTAuth_WrongScheme_Returns401(t *testing.T) {
	token := makeToken(t, "secret", 1, "bob")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Token "+token) // should be "Bearer"
	rr := httptest.NewRecorder()

	middleware.JWTAuth(newAuthSvc("secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not be reached")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTAuth_TamperedToken_Returns401(t *testing.T) {
	token := makeToken(t, "secret", 1, "carol")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token+"X")
	rr := httptest.NewRecorder()

	middleware.JWTAuth(newAuthSvc("secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not be reached")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTAuth_WrongSecret_Returns401(t *testing.T) {
	token := makeToken(t, "secret-A", 1, "dave")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	// Validator uses a different secret → should reject
	middleware.JWTAuth(newAuthSvc("secret-B"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not be reached")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTAuth_ExpiredToken_Returns401(t *testing.T) {
	claims := jwt.MapClaims{
		"user_id":  1,
		"username": "eve",
		"exp":      time.Now().Add(-1 * time.Hour).Unix(), // expired 1h ago
		"iat":      time.Now().Add(-2 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte("secret"))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	rr := httptest.NewRecorder()

	middleware.JWTAuth(newAuthSvc("secret"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not be reached for expired token")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rr.Code)
	}
}

func TestGetUserID_NotInContext_ReturnsZero(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if id := middleware.GetUserID(req.Context()); id != 0 {
		t.Errorf("expected 0 for missing userID, got %d", id)
	}
}
