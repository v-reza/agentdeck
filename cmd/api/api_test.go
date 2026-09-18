package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentdeck/internal/auth"
)

func newTestAPI(t *testing.T) (authAPI, *auth.Store) {
	t.Helper()

	store := auth.NewStore()
	return authAPI{store: store}, store
}

// registerBody is the request payload for POST /api/v1/auth/register.
type registerBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name,omitempty"`
	OrgName  string `json:"org_name,omitempty"`
}

func register(api authAPI, body registerBody) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(string(payload)))
	recorder := httptest.NewRecorder()
	api.register(recorder, request)
	return recorder
}

func sessionCookie(recorder *httptest.ResponseRecorder) string {
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == auth.SessionCookieName {
			return cookie.Value
		}
	}
	return ""
}

func TestRegisterEndpoint(t *testing.T) {
	api, _ := newTestAPI(t)

	recorder := register(api, registerBody{Email: "Ada@Example.com", Password: "password1"})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201 (body: %s)", recorder.Code, recorder.Body.String())
	}
	var response struct {
		UserID      string `json:"user_id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("register response: %v", err)
	}
	if response.UserID == "" || response.WorkspaceID == "" {
		t.Fatalf("register response missing ids: %+v", response)
	}

	cookie := sessionCookie(recorder)
	if cookie == "" {
		t.Fatal("register did not set the session cookie")
	}
	if setCookie := recorder.Header().Get("Set-Cookie"); !strings.Contains(setCookie, "HttpOnly") {
		t.Fatalf("session cookie is not HttpOnly: %s", setCookie)
	}

	// US-AD01 AC4: the identical request now conflicts.
	duplicate := register(api, registerBody{Email: "ada@example.com", Password: "password1"})
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate register status = %d, want 409", duplicate.Code)
	}
}

func TestRegisterEndpointRejectsInvalidInput(t *testing.T) {
	api, _ := newTestAPI(t)

	cases := []struct {
		name     string
		email    string
		password string
	}{
		{"empty email", "", "password1"},
		{"malformed email", "notanemail", "password1"},
		{"short password", "a@example.com", "short"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := register(api, registerBody{Email: tc.email, Password: tc.password})
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestRegisterEndpointWorksMissingName(t *testing.T) {
	api, _ := newTestAPI(t)

	// US-AD01 AC6: no name in the body must still succeed.
	recorder := register(api, registerBody{Email: "john.doe@example.com", Password: "password1"})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}
}

func TestRegisterEndpointStoresHashedPassword(t *testing.T) {
	api, store := newTestAPI(t)

	recorder := register(api, registerBody{Email: "hash@example.com", Password: "password1"})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", recorder.Code)
	}

	user, ok := store.Authenticate(sessionCookie(recorder))
	if !ok {
		t.Fatal("session cookie did not authenticate")
	}
	if !strings.HasPrefix(user.PasswordHash, "$argon2id$") {
		t.Fatalf("stored password is not argon2id: %q", user.PasswordHash)
	}
	if strings.Contains(user.PasswordHash, "password1") {
		t.Fatal("plaintext password present in the stored hash")
	}
}

func TestLoginEndpoint(t *testing.T) {
	api, _ := newTestAPI(t)
	register(api, registerBody{Email: "a@example.com", Password: "password1"})

	payload := `{"email":"a@example.com","password":"password1"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(payload))
	recorder := httptest.NewRecorder()
	api.login(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", recorder.Code)
	}
	if sessionCookie(recorder) == "" {
		t.Fatal("login did not set the session cookie")
	}
}

func TestLoginEndpointRejectsBadCredentials(t *testing.T) {
	api, _ := newTestAPI(t)
	register(api, registerBody{Email: "a@example.com", Password: "password1"})

	payload := `{"email":"a@example.com","password":"wrong"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(payload))
	recorder := httptest.NewRecorder()
	api.login(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want 401", recorder.Code)
	}
	if sessionCookie(recorder) != "" {
		t.Fatal("failed login set a session cookie")
	}
}

func TestLoginEndpointLocksAfterFiveFailures(t *testing.T) {
	api, _ := newTestAPI(t)
	register(api, registerBody{Email: "lock@example.com", Password: "password1"})

	for i := 0; i < 5; i++ {
		payload := `{"email":"lock@example.com","password":"wrong"}`
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(payload))
		recorder := httptest.NewRecorder()
		api.login(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", i+1, recorder.Code)
		}
	}

	payload := `{"email":"lock@example.com","password":"password1"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(payload))
	recorder := httptest.NewRecorder()
	api.login(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("locked login status = %d, want 429", recorder.Code)
	}
	if recorder.Header().Get("Retry-After") == "" {
		t.Fatal("locked login did not send Retry-After")
	}
}

func TestMeEndpoint(t *testing.T) {
	api, _ := newTestAPI(t)
	recorder := register(api, registerBody{Email: "a@example.com", Password: "password1", Name: "Alice"})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionCookie(recorder)})
	meRecorder := httptest.NewRecorder()
	api.me(meRecorder, request)

	if meRecorder.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200", meRecorder.Code)
	}
	var profile struct {
		ID         string `json:"id"`
		Email      string `json:"email"`
		Name       string `json:"name"`
		Workspaces []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(meRecorder.Body.Bytes(), &profile); err != nil {
		t.Fatalf("me response: %v", err)
	}
	if profile.Email != "a@example.com" || profile.Name != "Alice" {
		t.Fatalf("me profile = %+v", profile)
	}
	if len(profile.Workspaces) != 1 || profile.Workspaces[0].Role != "owner" {
		t.Fatalf("me workspaces = %+v", profile.Workspaces)
	}
}

func TestMeEndpointRejectsMissingSession(t *testing.T) {
	api, _ := newTestAPI(t)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	recorder := httptest.NewRecorder()
	api.me(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("me without session status = %d, want 401", recorder.Code)
	}
}

func TestMeEndpointAcceptsBearerToken(t *testing.T) {
	api, _ := newTestAPI(t)
	recorder := register(api, registerBody{Email: "a@example.com", Password: "password1"})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer "+sessionCookie(recorder))
	meRecorder := httptest.NewRecorder()
	api.me(meRecorder, request)

	if meRecorder.Code != http.StatusOK {
		t.Fatalf("me with bearer status = %d, want 200", meRecorder.Code)
	}
}

func TestLogoutEndpoint(t *testing.T) {
	api, _ := newTestAPI(t)
	recorder := register(api, registerBody{Email: "a@example.com", Password: "password1"})
	token := sessionCookie(recorder)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	outRecorder := httptest.NewRecorder()
	api.logout(outRecorder, request)

	if outRecorder.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", outRecorder.Code)
	}
	if !strings.Contains(outRecorder.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout did not expire the session cookie: %s", outRecorder.Header().Get("Set-Cookie"))
	}

	// US-AD02: the revoked token must not authenticate anymore.
	meRequest := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	meRequest.Header.Set("Authorization", "Bearer "+token)
	meRecorder := httptest.NewRecorder()
	api.me(meRecorder, meRequest)
	if meRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want 401", meRecorder.Code)
	}
}

func TestLogoutEndpointRejectsMissingSession(t *testing.T) {
	api, _ := newTestAPI(t)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	recorder := httptest.NewRecorder()
	api.logout(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("logout without session status = %d, want 401", recorder.Code)
	}
}

func TestMalformedRegisterBody(t *testing.T) {
	api, _ := newTestAPI(t)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader("not json"))
	recorder := httptest.NewRecorder()
	api.register(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed register status = %d, want 400", recorder.Code)
	}
}
