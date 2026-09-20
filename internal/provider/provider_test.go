package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestValidateBaseURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		// wantErr says the URL must be refused; wantErrHas (optional) also pins
		// the reason, so a case cannot pass merely because DNS failed.
		wantErr    bool
		wantErrHas string
	}{
		// --- must be accepted -------------------------------------------------
		{"public host", "https://api.openai.com/v1", false, ""},
		{"public host no path", "https://api.anthropic.com", false, ""},
		{"public host explicit port and path", "https://api.openai.com:8443/v1/openai", false, ""},
		{"public host trailing slash", "https://api.openai.com/v1/", false, ""},
		{"public ip literal", "https://1.1.1.1/v1", false, ""},

		// --- scheme -----------------------------------------------------------
		{"http not allowed", "http://example.com", true, ""},
		{"file scheme", "file:///etc/passwd", true, ""},
		{"gopher scheme", "gopher://example.com", true, ""},
		{"ftp scheme", "ftp://example.com", true, ""},
		{"scheme relative", "example.com/v1", true, ""},
		{"empty", "", true, ""},
		{"scheme only", "https://", true, ""},

		// --- loopback ---------------------------------------------------------
		{"loopback ipv4", "https://127.0.0.1", true, "is loopback"},
		{"loopback ipv4 with port", "https://127.0.0.1:8080/v1", true, ""},
		{"loopback ipv4 short form", "https://127.1", true, "is loopback"},
		{"loopback ipv6", "https://[::1]", true, "is loopback"},
		{"loopback ipv6 mapped", "https://[::ffff:127.0.0.1]", true, ""},
		{"localhost name", "https://localhost", true, "is loopback"},
		{"localhost with port", "https://localhost:1234/v1", true, ""},

		// --- private / link-local / metadata ----------------------------------
		{"metadata service", "https://169.254.169.254/latest/meta-data", true, "is link-local"},
		{"link local", "https://169.254.1.1", true, ""},
		{"link local ipv6", "https://[fe80::1]", true, ""},
		{"rfc1918 10/8", "https://10.0.0.1", true, ""},
		{"rfc1918 192.168/16", "https://192.168.1.1", true, ""},
		{"rfc1918 172.16/12", "https://172.16.0.1", true, ""},
		{"unique local ipv6", "https://[fc00::1]", true, ""},
		{"unique local ipv6 fd", "https://[fd12:3456::1]", true, ""},
		{"unspecified ipv4", "https://0.0.0.0", true, ""},
		{"unspecified ipv6", "https://[::]", true, ""},
		{"carrier grade nat", "https://100.64.0.1", true, ""},
		{"multicast", "https://224.0.0.1", true, ""},

		// --- non-canonical IP spellings (inet_aton) ---------------------------
		{"decimal 127.0.0.1", "https://2130706433", true, "is loopback"},
		{"decimal 169.254.169.254", "https://2852039166", true, "is link-local"},
		{"hex 127.0.0.1", "https://0x7f000001", true, "is loopback"},
		{"hex mixed case", "https://0X7F.0.0.1", true, "is loopback"},
		{"hex short form", "https://0x7f.1", true, "is loopback"},
		{"octal 127.0.0.1", "https://0177.0.0.1", true, "is loopback"},
		{"octal 127.0.0.1 with path", "https://0177.0.0.1/v1/models", true, "is loopback"},
		{"octal 10.0.0.1", "https://012.0.0.1", true, "is private"},
		{"two part form", "https://127.0.1", true, ""},
		{"three part form", "https://127.0.0.1", true, ""},

		// --- credentials / malformed ------------------------------------------
		{"userinfo with private host", "https://user:pass@127.0.0.1", true, ""},
		{"userinfo with public host", "https://user:pass@api.openai.com", true, ""},
		{"bad port", "https://api.openai.com:0", true, ""},
		{"port out of range", "https://api.openai.com:99999", true, ""},
		{"bad port text", "https://api.openai.com:https", true, ""},
		{"bad escape", "https://exa mple.com", true, ""},
		{"unresolvable host", "https://no-such-host.invalid", true, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateBaseURL(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateBaseURL(%q) = %v, want error", tc.raw, got)
				}
				if tc.wantErrHas != "" && !strings.Contains(err.Error(), tc.wantErrHas) {
					t.Fatalf("ValidateBaseURL(%q) error %q does not mention %q", tc.raw, err, tc.wantErrHas)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateBaseURL(%q) unexpected error: %v", tc.raw, err)
			}
			if got == nil || got.Host == "" {
				t.Fatalf("ValidateBaseURL(%q) returned unusable URL %v", tc.raw, got)
			}
		})
	}
}

// TestValidateBaseURL_DNSRebinding pins the rule that a single private answer
// poisons the whole lookup, even when a public address comes back too. Without
// this, a resolver that flips between public and private defeats the guard.
func TestValidateBaseURL_DNSRebinding(t *testing.T) {
	orig := lookupIP
	defer func() { lookupIP = orig }()

	public := net.ParseIP("93.184.216.34")
	private := net.ParseIP("10.1.2.3")

	cases := []struct {
		name    string
		answers []net.IP
		wantErr bool
	}{
		{"all public", []net.IP{public}, false},
		{"public then private", []net.IP{public, private}, true},
		{"private then public", []net.IP{private, public}, true},
		{"ipv6 loopback mixed with public", []net.IP{public, net.ParseIP("::1")}, true},
		{"mapped private", []net.IP{net.ParseIP("::ffff:192.168.0.1")}, true},
		{"no answers", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookupIP = func(context.Context, string) ([]net.IP, error) { return tc.answers, nil }
			_, err := ValidateBaseURL("https://rebind.example.com/v1")
			if tc.wantErr && err == nil {
				t.Fatal("want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestNewClient_RefusesRedirectToPrivate is the single most important test
// here: a public provider that answers 302 Location: https://127.0.0.1/ is the
// classic way to make the caller talk to a private address after the initial
// validation already passed.
func TestNewClient_RefusesRedirectToPrivate(t *testing.T) {
	var served int
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, srv.URL+"/v1/models", http.StatusFound)
			return
		}
		served++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"id":"reached-private-host"}]}`)
	}))
	defer srv.Close()

	// Control: the target is reachable, so the guard is what stops us.
	ctrl, err := srv.Client().Get(srv.URL + "/redir")
	if err != nil {
		t.Fatalf("control request failed, redirect target unreachable: %v", err)
	}
	ctrl.Body.Close()
	if ctrl.StatusCode != http.StatusOK || served == 0 {
		t.Fatalf("control: got status %d, target served %d times", ctrl.StatusCode, served)
	}

	client := NewClient()
	client.Transport = srv.Client().Transport // trust the test certificate
	resp, err := client.Get(srv.URL + "/redir")
	if err == nil {
		resp.Body.Close()
		t.Fatalf("redirect to private address was followed (status %d), want refusal", resp.StatusCode)
	}
	if !strings.Contains(err.Error(), "refusing redirect") {
		t.Fatalf("want a redirect refusal, got: %v", err)
	}
}

func TestNewClient_RefusesRedirectToMetadata(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer srv.Close()

	client := NewClient()
	client.Transport = srv.Client().Transport
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("redirect to 169.254.169.254 was followed, want refusal")
	}
	if !strings.Contains(err.Error(), "refusing redirect") {
		t.Fatalf("want a redirect refusal, got: %v", err)
	}
}

// tlsClient returns a redirect-guarding client that trusts srv's certificate.
func tlsClient(srv *httptest.Server) *http.Client {
	c := NewClient()
	c.Transport = srv.Client().Transport
	return c
}

func TestFetchModels(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/openai/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"error":{"message":"bad key"}}`)
			return
		}
		io.WriteString(w, `{"object":"list","data":[{"id":"gpt-4o","object":"model"},
			{"id":"gpt-4o-mini","object":"model"},{"id":"gpt-4o","object":"model"}]}`)
	})
	mux.HandleFunc("/ollama/v1/models", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"models":[{"name":"llama3.1:8b"},{"id":"qwen2.5"}]}`)
	})
	mux.HandleFunc("/empty/v1/models", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[]}`)
	})
	mux.HandleFunc("/garbage/v1/models", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html>not json</html>`)
	})
	mux.HandleFunc("/broken/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/huge/v1/models", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"id":"`)
		chunk := strings.Repeat("a", 64<<10)
		for i := 0; i < 40; i++ { // 2.5 MiB > 2 MiB cap
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
		}
		io.WriteString(w, `"}]}`)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	client := tlsClient(srv)

	// White-box on purpose: the exported ListModels refuses loopback base URLs
	// by design, so the httptest server (127.0.0.1) is only reachable through
	// the inner fetch step. The guard wiring is covered by
	// TestListModels_RejectsPrivateBaseURL.
	fetch := func(t *testing.T, path, key string) ([]string, error) {
		t.Helper()
		u, err := url.Parse(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		return fetchModels(context.Background(), client, u, key)
	}

	t.Run("openai shape", func(t *testing.T) {
		got, err := fetch(t, "/openai/v1", "sk-test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"gpt-4o", "gpt-4o-mini"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("base url with trailing slash", func(t *testing.T) {
		u, _ := url.Parse(srv.URL + "/openai/v1/")
		got, err := fetchModels(context.Background(), client, u, "sk-test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %v, want 2 models", got)
		}
	})

	t.Run("models shape", func(t *testing.T) {
		got, err := fetch(t, "/ollama/v1", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0] != "llama3.1:8b" || got[1] != "qwen2.5" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("bad key gives 401 without leaking key", func(t *testing.T) {
		_, err := fetch(t, "/openai/v1", "sk-wrong")
		if err == nil {
			t.Fatal("want error for 401")
		}
		if !strings.Contains(err.Error(), "401") {
			t.Fatalf("want status in error, got: %v", err)
		}
		if strings.Contains(err.Error(), "sk-wrong") {
			t.Fatalf("error leaks api key: %v", err)
		}
	})

	t.Run("empty list is an error", func(t *testing.T) {
		if _, err := fetch(t, "/empty/v1", ""); err == nil {
			t.Fatal("want error for empty model list")
		}
	})

	t.Run("non json body", func(t *testing.T) {
		if _, err := fetch(t, "/garbage/v1", ""); err == nil {
			t.Fatal("want error for non-JSON body")
		}
	})

	t.Run("http 500", func(t *testing.T) {
		if _, err := fetch(t, "/broken/v1", ""); err == nil {
			t.Fatal("want error for HTTP 500")
		}
	})

	t.Run("oversized body is refused", func(t *testing.T) {
		_, err := fetch(t, "/huge/v1", "")
		if err == nil {
			t.Fatal("want error for oversized body")
		}
		if !strings.Contains(err.Error(), "larger than") {
			t.Fatalf("want size error, got: %v", err)
		}
	})

	t.Run("timeout error does not leak key", func(t *testing.T) {
		u, _ := url.Parse(srv.URL + "/openai/v1")
		u.User = url.User("sk-secret-key") // url.Error echoes the URL
		bad := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		})}
		_, err := fetchModels(context.Background(), bad, u, "sk-secret-key")
		if err == nil {
			t.Fatal("want error")
		}
		if strings.Contains(err.Error(), "sk-secret-key") {
			t.Fatalf("error leaks api key: %v", err)
		}
		if !strings.Contains(err.Error(), "[redacted]") {
			t.Fatalf("want redaction marker, got: %v", err)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestListModels_RejectsPrivateBaseURL covers the wiring: the exported entry
// point runs the guard before any request goes out.
func TestListModels_RejectsPrivateBaseURL(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request reached the private server, guard did not run")
		io.WriteString(w, `{"data":[{"id":"leaked"}]}`)
	}))
	defer srv.Close()

	for _, raw := range []string{
		srv.URL,               // https://127.0.0.1:port
		"http://127.0.0.1/v1", // not https
		"https://169.254.169.254/v1",
		"https://2130706433/v1",
		"https://[::1]:8080/v1",
	} {
		if _, err := ListModels(context.Background(), NewClient(), raw, "sk-test"); err == nil {
			t.Errorf("ListModels(%q) succeeded, want refusal", raw)
		}
	}
}

func TestListModels_UsesDefaultClientWhenNil(t *testing.T) {
	// Must not panic and must still refuse the private URL.
	if _, err := ListModels(context.Background(), nil, "https://127.0.0.1/v1", ""); err == nil {
		t.Fatal("want refusal for loopback base URL")
	}
}

func TestParseAton(t *testing.T) {
	cases := map[string]string{
		"2130706433":     "127.0.0.1",
		"0x7f000001":     "127.0.0.1",
		"0177.0.0.1":     "127.0.0.1",
		"2852039166":     "169.254.169.254",
		"0x7f.1":         "127.0.0.1",
		"127.1":          "127.0.0.1",
		"3232235777":     "192.168.1.1",
		"0177.0.0.01":    "127.0.0.1",
		"0x100000000":    "",
		"1.2.3.4.5":      "",
		"999.1.1.1":      "",
		"0x7f000001x":    "",
		"api.openai.com": "",
		"":               "",
	}
	for in, want := range cases {
		got := parseAton(in)
		if want == "" {
			if got != nil {
				t.Errorf("parseAton(%q) = %v, want nil", in, got)
			}
			continue
		}
		if got == nil || got.String() != want {
			t.Errorf("parseAton(%q) = %v, want %s", in, got, want)
		}
	}
}

func TestParseIPLiteral(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1":        "127.0.0.1",
		"::1":              "::1",
		"::ffff:127.0.0.1": "127.0.0.1",
		"2130706433":       "127.0.0.1",
		"0x7f000001":       "127.0.0.1",
		"0177.0.0.1":       "127.0.0.1",
		"api.openai.com":   "",
		"example.com":      "",
		"localhost":        "",
	}
	for in, want := range cases {
		got := parseIPLiteral(in)
		if want == "" {
			if got != nil {
				t.Errorf("parseIPLiteral(%q) = %v, want nil", in, got)
			}
			continue
		}
		if got == nil || got.String() != want {
			t.Errorf("parseIPLiteral(%q) = %v, want %s", in, got, want)
		}
	}
}

func TestCheckIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"0.0.0.0", "::1", "fc00::1", "fe80::1", "::ffff:10.0.0.1", "100.64.0.1",
		"198.18.0.1", "240.0.0.1", "2001:db8::1", "224.0.0.1",
	}
	for _, s := range blocked {
		if err := checkIP(net.ParseIP(s)); err == nil {
			t.Errorf("checkIP(%s) = nil, want error", s)
		}
	}
	allowed := []string{"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700::1111"}
	for _, s := range allowed {
		if err := checkIP(net.ParseIP(s)); err != nil {
			t.Errorf("checkIP(%s) = %v, want nil", s, err)
		}
	}
}

func TestRedact(t *testing.T) {
	secret := "sk-live-abc"
	err := fmt.Errorf("provider: GET https://%s@api.example.com/models: boom", secret)
	got := redact(err, secret)
	if strings.Contains(got.Error(), secret) {
		t.Fatalf("redact left the secret in: %v", got)
	}
	if redact(nil, secret) != nil {
		t.Fatal("redact(nil) must stay nil")
	}
	if err := redact(errors.New("no secret here"), secret); err.Error() != "no secret here" {
		t.Fatalf("unrelated error was rewritten: %v", err)
	}
}
