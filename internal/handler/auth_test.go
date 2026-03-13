package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kirill/gamelogserver/internal/model"
)

// mockAuthService implements authServiceIface for handler tests.
type mockAuthService struct {
	registerFn func(ctx context.Context, username, password string) (*model.AuthResponse, error)
	loginFn    func(ctx context.Context, username, password string) (*model.AuthResponse, error)
}

func (m *mockAuthService) Register(ctx context.Context, u, p string) (*model.AuthResponse, error) {
	return m.registerFn(ctx, u, p)
}
func (m *mockAuthService) Login(ctx context.Context, u, p string) (*model.AuthResponse, error) {
	return m.loginFn(ctx, u, p)
}
func (m *mockAuthService) ValidateToken(token string) (int, string, error) { return 0, "", nil }

func newAuthHandler(svc authServiceIface) *AuthHandler {
	return &AuthHandler{authService: svc}
}

func jsonBody(t *testing.T, v interface{}) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("jsonBody: %v", err)
	}
	return bytes.NewBuffer(b)
}

// --- Register ---

func TestAuthHandler_Register_Success(t *testing.T) {
	svc := &mockAuthService{
		registerFn: func(_ context.Context, u, _ string) (*model.AuthResponse, error) {
			return &model.AuthResponse{ID: 1, Username: u, Token: "tok"}, nil
		},
	}
	h := newAuthHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/auth/register", jsonBody(t, model.AuthRequest{
		Username: "alice", Password: "secret123",
	}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
	var resp model.AuthResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token != "tok" {
		t.Errorf("expected token=tok, got %s", resp.Token)
	}
}

func TestAuthHandler_Register_ServiceError_Returns400(t *testing.T) {
	svc := &mockAuthService{
		registerFn: func(_ context.Context, _, _ string) (*model.AuthResponse, error) {
			return nil, errors.New("username must be 3-50 characters")
		},
	}
	h := newAuthHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/auth/register", jsonBody(t, model.AuthRequest{
		Username: "x", Password: "pass",
	}))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp["error"] == "" {
		t.Error("expected error field in response")
	}
}

func TestAuthHandler_Register_InvalidJSON_Returns400(t *testing.T) {
	svc := &mockAuthService{}
	h := newAuthHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString("{invalid"))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr.Code)
	}
}

// --- Login ---

func TestAuthHandler_Login_Success(t *testing.T) {
	svc := &mockAuthService{
		loginFn: func(_ context.Context, _, _ string) (*model.AuthResponse, error) {
			return &model.AuthResponse{Token: "jwt-token"}, nil
		},
	}
	h := newAuthHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(t, model.AuthRequest{
		Username: "bob", Password: "pass",
	}))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp model.AuthResponse
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Token != "jwt-token" {
		t.Errorf("expected token=jwt-token, got %s", resp.Token)
	}
}

func TestAuthHandler_Login_InvalidCredentials_Returns401(t *testing.T) {
	svc := &mockAuthService{
		loginFn: func(_ context.Context, _, _ string) (*model.AuthResponse, error) {
			return nil, errors.New("invalid credentials")
		},
	}
	h := newAuthHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", jsonBody(t, model.AuthRequest{
		Username: "ghost", Password: "wrong",
	}))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthHandler_Login_InvalidJSON_Returns400(t *testing.T) {
	svc := &mockAuthService{}
	h := newAuthHandler(svc)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString("}"))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr.Code)
	}
}

// --- Content-Type header ---

func TestAuthHandler_Register_ResponseContentType(t *testing.T) {
	svc := &mockAuthService{
		registerFn: func(_ context.Context, u, _ string) (*model.AuthResponse, error) {
			return &model.AuthResponse{Token: "t"}, nil
		},
	}
	req := httptest.NewRequest(http.MethodPost, "/", jsonBody(t, model.AuthRequest{Username: "alice", Password: "pass123"}))
	rr := httptest.NewRecorder()
	newAuthHandler(svc).Register(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
