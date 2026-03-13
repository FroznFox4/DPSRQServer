package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kirill/gamelogserver/internal/model"
)

// mockUserRepo implements the userRepository interface in-memory.
type mockUserRepo struct {
	users  map[string]*model.User
	nextID int
	// failCreate: if set, Create returns this error
	failCreate error
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*model.User), nextID: 1}
}

func (m *mockUserRepo) Create(_ context.Context, username, hash string) (*model.User, error) {
	if m.failCreate != nil {
		return nil, m.failCreate
	}
	if _, exists := m.users[username]; exists {
		return nil, errors.New("username already exists")
	}
	u := &model.User{ID: m.nextID, Username: username, PasswordHash: hash, CreatedAt: time.Now()}
	m.nextID++
	m.users[username] = u
	return u, nil
}

func (m *mockUserRepo) GetByUsername(_ context.Context, username string) (*model.User, error) {
	u, ok := m.users[username]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	return u, nil
}

func newTestAuthService() (*AuthService, *mockUserRepo) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, "test-secret-key")
	return svc, repo
}

// --- Register ---

func TestAuthService_Register_Success(t *testing.T) {
	svc, _ := newTestAuthService()

	resp, err := svc.Register(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Username != "alice" {
		t.Errorf("expected username=alice, got %s", resp.Username)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.ID == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestAuthService_Register_UsernameTooShort(t *testing.T) {
	svc, _ := newTestAuthService()

	_, err := svc.Register(context.Background(), "ab", "password123")
	if err == nil {
		t.Fatal("expected error for short username")
	}
}

func TestAuthService_Register_UsernameTooLong(t *testing.T) {
	svc, _ := newTestAuthService()
	longName := make([]byte, 51)
	for i := range longName {
		longName[i] = 'x'
	}

	_, err := svc.Register(context.Background(), string(longName), "password123")
	if err == nil {
		t.Fatal("expected error for username > 50 chars")
	}
}

func TestAuthService_Register_PasswordTooShort(t *testing.T) {
	svc, _ := newTestAuthService()

	_, err := svc.Register(context.Background(), "alice", "12345")
	if err == nil {
		t.Fatal("expected error for password < 6 chars")
	}
}

func TestAuthService_Register_DuplicateUsername(t *testing.T) {
	svc, _ := newTestAuthService()

	if _, err := svc.Register(context.Background(), "alice", "password123"); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	_, err := svc.Register(context.Background(), "alice", "other-pass")
	if err == nil {
		t.Fatal("expected error for duplicate username")
	}
}

// --- Login ---

func TestAuthService_Login_Success(t *testing.T) {
	svc, _ := newTestAuthService()

	if _, err := svc.Register(context.Background(), "bob", "secret456"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	resp, err := svc.Login(context.Background(), "bob", "secret456")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	svc, _ := newTestAuthService()

	if _, err := svc.Register(context.Background(), "bob", "secret456"); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err := svc.Login(context.Background(), "bob", "wrongpass")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestAuthService_Login_UnknownUser(t *testing.T) {
	svc, _ := newTestAuthService()

	_, err := svc.Login(context.Background(), "ghost", "password")
	if err == nil {
		t.Fatal("expected error for unknown user")
	}
}

// --- ValidateToken ---

func TestAuthService_ValidateToken_Valid(t *testing.T) {
	svc, _ := newTestAuthService()

	resp, err := svc.Register(context.Background(), "carol", "password999")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	userID, username, err := svc.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if username != "carol" {
		t.Errorf("expected username=carol, got %s", username)
	}
	if userID == 0 {
		t.Error("expected non-zero userID")
	}
}

func TestAuthService_ValidateToken_Tampered(t *testing.T) {
	svc, _ := newTestAuthService()

	resp, _ := svc.Register(context.Background(), "dave", "password999")
	_, _, err := svc.ValidateToken(resp.Token + "tampered")
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}

func TestAuthService_ValidateToken_WrongSecret(t *testing.T) {
	svcA := NewAuthService(newMockUserRepo(), "secret-A")
	svcB := NewAuthService(newMockUserRepo(), "secret-B")

	resp, _ := svcA.Register(context.Background(), "eve", "password999")

	_, _, err := svcB.ValidateToken(resp.Token)
	if err == nil {
		t.Fatal("expected error when validating token from different secret")
	}
}

func TestAuthService_ValidateToken_Empty(t *testing.T) {
	svc, _ := newTestAuthService()

	_, _, err := svc.ValidateToken("")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestAuthService_ValidateToken_Garbage(t *testing.T) {
	svc, _ := newTestAuthService()

	_, _, err := svc.ValidateToken("not.a.jwt")
	if err == nil {
		t.Fatal("expected error for garbage token")
	}
}
