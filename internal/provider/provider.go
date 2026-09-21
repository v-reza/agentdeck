// Package provider guards BYO ("bring your own") provider base URLs against
// SSRF (US-AD106 AC3/AC4, ARCHITECTURE §16, DECISIONS §6A.F) and reads the
// model list a user's own LLM endpoint advertises.
//
// The base URL is typed by the user and is handed to our HTTP client, so it is
// a trust boundary: without the guard an attacker points AgentDeck at
// 169.254.169.254 or 10.0.0.0/8 and reads their own network back as a "model
// list". Everything here is about keeping that from happening, including the
// inet_aton spellings (decimal/octal/hex) that a naive net.ParseIP check lets
// through, DNS answers that change after validation, and redirects.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// MaxBaseURLLength bounds user input before it is parsed.
	MaxBaseURLLength = 2048
	// maxBodyBytes caps how much of a provider response we buffer, so a hostile
	// endpoint cannot exhaust our memory.
	maxBodyBytes = 2 << 20 // 2 MiB
	// requestTimeout is the per-request ceiling. http.DefaultClient is never used.
	requestTimeout = 10 * time.Second
	// maxRedirects matches net/http's own limit.
	maxRedirects = 10
)

// lookupIP is a package variable so tests can simulate DNS rebinding.
var lookupIP = func(ctx context.Context, host string) ([]net.IP, error) {
	return net.DefaultResolver.LookupIP(ctx, "ip", host)
}

// blockedNets are special-use ranges that are not publicly routable but that
// net.IP.IsGlobalUnicast still reports as global unicast.
var blockedNets = []*net.IPNet{
	mustCIDR("100.64.0.0/10"), // RFC 6598 carrier-grade NAT
	mustCIDR("192.0.0.0/24"),  // RFC 6890 IETF protocol assignments
	mustCIDR("198.18.0.0/15"), // RFC 2544 benchmarking
	mustCIDR("240.0.0.0/4"),   // RFC 1112 reserved
	mustCIDR("2001:db8::/32"), // RFC 3849 documentation
}

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic("provider: bad blocked CIDR " + s)
	}
	return n
}

// localHostAllowlist is the set of hosts an operator may point a BYO provider
// at even though they are not publicly routable.
//
// The PRD's US-AD106 AC3 rejects loopback outright. That rule was written for a
// hosted deployment, where the only loopback that exists is ours; it breaks the
// self-hosted case the product is actually for, where the operator's own
// inference server (Ollama, LM Studio, a local gateway) runs on the same
// machine and has no other address. The operator's latest decision is that
// these three hosts are allowed, and only these three.
//
// `host.docker.internal` is the third because without it the container
// deployment has no usable address at all: an endpoint on the operator's
// machine is reachable from inside a container only through that name, and the
// guard refuses plain `http` for every host not on this list. Measured —
// `localhost` and `127.0.0.1` both answer "connection refused" from inside the
// api container while the same gateway answers 200 on this name. A deployment
// that does not use Docker cannot resolve it, so the allowance is inert there.
//
// Matching is on the literal host string, never on the parsed address. That is
// what keeps the relaxation narrow: `2130706433`, `0x7f000001`, `0177.0.0.1`
// and `[::1]` all mean loopback but are not on this list, so they stay refused.
// Every RFC1918 address stays refused too. An operator who means loopback writes
// `localhost` or `127.0.0.1`.
var localHostAllowlist = map[string]bool{
	"localhost":            true,
	"127.0.0.1":            true,
	"host.docker.internal": true,
}

// IsLocalProviderHost reports whether host is one of the allowed loopback names.
func IsLocalProviderHost(host string) bool {
	return localHostAllowlist[strings.ToLower(host)]
}

// ValidateOperatorBaseURL validates a base URL the operator typed, allowing the
// two loopback hosts above — and, for those two only, plain `http`.
//
// `http` is permitted there and nowhere else because a local inference server
// commonly serves plain HTTP, and traffic to loopback never leaves the machine,
// so there is no network to eavesdrop on. A public host keeps the https-only
// rule.
//
// This is deliberately a different function from ValidateURL rather than a flag
// on it: ValidateURL also guards redirects, and a redirect from a public host to
// loopback is the classic SSRF pivot. Relaxing one must not relax the other.
func ValidateOperatorBaseURL(ctx context.Context, raw string) (*url.URL, error) {
	u, err := parseBaseURL(raw)
	if err != nil {
		return nil, err
	}
	host := strings.ToLower(u.Hostname())
	if IsLocalProviderHost(host) {
		if u.Scheme != "https" && u.Scheme != "http" {
			return nil, fmt.Errorf("provider: base URL scheme %q is not allowed, only https or http", u.Scheme)
		}
		return u, nil
	}
	return finishValidateURL(ctx, u)
}

// ValidateBaseURL rejects a base URL that could be used for SSRF and returns
// the parsed URL on success. It resolves DNS and inspects every answer.
func ValidateBaseURL(raw string) (*url.URL, error) {
	return ValidateURL(context.Background(), raw)
}

// ValidateAddressOnly applies every check that needs only the URL text plus a
// literal-IP test, and deliberately does NOT resolve DNS.
//
// The write path needs this. Resolving on save makes registering an agent
// depend on live DNS: a host that is down, behind a VPN, or not deployed yet
// would be refused, and that is the ordinary state of a form the operator is
// still filling in. It also makes the check depend on whatever the host's
// resolver returns at that instant, so the same body can succeed and fail on
// consecutive attempts.
//
// What is lost: a hostname that *resolves* to a blocked address (DNS rebinding,
// `evil.example.com → 127.0.0.1`). That case is still caught by
// `ValidateURL` on every path that actually connects, which is where it
// matters. A name is not an address until something dials it.
func ValidateAddressOnly(raw string) (*url.URL, error) {
	u, err := parseBaseURL(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("provider: base URL scheme %q is not allowed, only https", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	// A literal address is decidable from the text alone, so it is still checked
	// here — that is the metadata endpoint and the private-range case.
	if ip := parseIPLiteral(host); ip != nil {
		if err := checkIP(ip); err != nil {
			return nil, fmt.Errorf("provider: base URL host %q: %w", host, err)
		}
	}
	return u, nil
}

// ValidateURL is ValidateBaseURL with a caller-supplied context, so a request
// path can bound the DNS lookup. Redirects are validated through this too.
func ValidateURL(ctx context.Context, raw string) (*url.URL, error) {
	u, err := parseBaseURL(raw)
	if err != nil {
		return nil, err
	}
	return finishValidateURL(ctx, u)
}

// parseBaseURL applies the checks that need only the URL text.
func parseBaseURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("provider: base URL is required")
	}
	if len(raw) > MaxBaseURLLength {
		return nil, fmt.Errorf("provider: base URL longer than %d characters", MaxBaseURLLength)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("provider: invalid base URL: %w", err)
	}
	if u.User != nil {
		return nil, errors.New("provider: base URL must not embed credentials")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil, errors.New("provider: base URL has no host")
	}
	if port := u.Port(); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("provider: base URL has an invalid port %q", port)
		}
	}
	return u, nil
}

// finishValidateURL rejects any address that is not publicly routable, including
// every loopback spelling. It enforces the https-only rule too.
func finishValidateURL(ctx context.Context, u *url.URL) (*url.URL, error) {
	if u.Scheme != "https" {
		return nil, fmt.Errorf("provider: base URL scheme %q is not allowed, only https", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())

	if ip := parseIPLiteral(host); ip != nil {
		if err := checkIP(ip); err != nil {
			return nil, fmt.Errorf("provider: base URL host %q: %w", host, err)
		}
		return u, nil
	}

	ips, err := lookupIP(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("provider: cannot resolve host %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("provider: host %q did not resolve to any address", host)
	}
	// One bad answer is enough to refuse: a resolver that returns both a public
	// and a private address (DNS rebinding) must not get a second chance at
	// connect time.
	for _, ip := range ips {
		if err := checkIP(ip); err != nil {
			return nil, fmt.Errorf("provider: host %q resolves to a blocked address: %w", host, err)
		}
	}
	return u, nil
}

// checkIP rejects every address that is not publicly routable.
func checkIP(ip net.IP) error {
	if ip == nil {
		return errors.New("empty address")
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4 // collapse ::ffff:127.0.0.1 to 127.0.0.1
	}
	switch {
	case ip.IsUnspecified():
		return fmt.Errorf("address %s is unspecified", ip)
	case ip.IsLoopback():
		return fmt.Errorf("address %s is loopback", ip)
	case ip.IsPrivate():
		return fmt.Errorf("address %s is private", ip)
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return fmt.Errorf("address %s is link-local", ip)
	case ip.IsMulticast(), ip.IsInterfaceLocalMulticast():
		return fmt.Errorf("address %s is multicast", ip)
	case !ip.IsGlobalUnicast():
		return fmt.Errorf("address %s is not globally routable", ip)
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return fmt.Errorf("address %s is in reserved range %s", ip, n)
		}
	}
	return nil
}

// parseIPLiteral decodes host as an IP literal, including the inet_aton
// spellings that libc, curl and browsers accept: "2130706433" (decimal),
// "0x7f000001" (hex) and "0177.0.0.1" (octal) all mean 127.0.0.1.
func parseIPLiteral(host string) net.IP {
	if ip := net.ParseIP(host); ip != nil {
		return ip
	}
	if strings.Contains(host, ":") { // not an IPv6 literal then
		return nil
	}
	return parseAton(host)
}

// parseAton implements inet_aton: one to four dot-separated parts, each part
// decimal, octal (leading 0) or hex (0x), with the last part taking whatever
// bits are left.
func parseAton(host string) net.IP {
	parts := strings.Split(host, ".")
	if len(parts) > 4 {
		return nil
	}
	vals := make([]uint64, len(parts))
	for i, p := range parts {
		v, ok := parseAtonPart(p)
		if !ok {
			return nil
		}
		vals[i] = v
	}
	var addr uint64
	switch len(vals) {
	case 1:
		addr = vals[0]
	case 2:
		if vals[0] > 0xff || vals[1] > 0xffffff {
			return nil
		}
		addr = vals[0]<<24 | vals[1]
	case 3:
		if vals[0] > 0xff || vals[1] > 0xff || vals[2] > 0xffff {
			return nil
		}
		addr = vals[0]<<24 | vals[1]<<16 | vals[2]
	default:
		for _, v := range vals {
			if v > 0xff {
				return nil
			}
		}
		addr = vals[0]<<24 | vals[1]<<16 | vals[2]<<8 | vals[3]
	}
	if addr > 0xffffffff {
		return nil
	}
	return net.IPv4(byte(addr>>24), byte(addr>>16), byte(addr>>8), byte(addr))
}

func parseAtonPart(s string) (uint64, bool) {
	base := 10
	switch {
	case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
		base, s = 16, s[2:]
	case len(s) > 1 && s[0] == '0':
		base, s = 8, s[1:]
	}
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// NewClient returns an HTTP client that refuses to follow a redirect to an
// address ValidateURL would reject (US-AD106 AC4). A public host that answers
// 302 Location: https://169.254.169.254/ is the classic bypass, so the
// destination is validated on every hop, not just the first URL.
func NewClient() *http.Client {
	return &http.Client{
		Timeout: requestTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("provider: stopped after %d redirects", len(via))
			}
			if _, err := ValidateURL(req.Context(), req.URL.String()); err != nil {
				return fmt.Errorf("provider: refusing redirect: %w", err)
			}
			return nil
		},
	}
}

// ListModels calls GET {baseURL}/models and returns the advertised model ids,
// in the order the provider sent them.
//
// The client is passed in rather than defaulted so callers can inject one that
// refuses redirects to private addresses (NewClient); a nil client gets
// NewClient.
func ListModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]string, error) {
	u, err := ValidateOperatorBaseURL(ctx, baseURL)
	if err != nil {
		return nil, redact(err, apiKey)
	}
	if client == nil {
		client = NewClient()
	}
	return fetchModels(ctx, client, u, apiKey)
}

// fetchModels performs the guarded request against an already-validated base
// URL. It is split out so tests can point it at an httptest server, whose
// address is loopback and therefore rejected by ValidateURL by design.
func fetchModels(ctx context.Context, client *http.Client, u *url.URL, apiKey string) ([]string, error) {
	u.Path = strings.TrimRight(u.Path, "/") + "/models"
	u.RawQuery, u.Fragment = "", ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, redact(fmt.Errorf("provider: building request: %w", err), apiKey)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, redact(withLocalhostHint(
			fmt.Errorf("provider: GET %s: %w", u.Host, err), u), apiKey)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, redact(fmt.Errorf("provider: reading %s: %w", u.Host, err), apiKey)
	}
	if len(body) > maxBodyBytes {
		return nil, fmt.Errorf("provider: response from %s is larger than %d bytes", u.Host, maxBodyBytes)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider: %s returned HTTP %d", u.Host, resp.StatusCode)
	}

	models, err := parseModels(body)
	if err != nil {
		return nil, fmt.Errorf("provider: %s: %w", u.Host, err)
	}
	return models, nil
}

// ProbeInference makes the one call that actually proves a credential: a
// minimal completion against {baseURL}/chat/completions.
//
// A model list is not proof, and that is measured rather than assumed
// (DECISIONS 6A.J): a local gateway answers 200 on /models with no key at all
// and 401 on inference. So /models says "the endpoint is up" and this says "the
// key is right". The request asks for a single token, the cheapest call that
// still passes through authentication (AC3).
//
// The model name is a parameter because a hardcoded one would fail on every
// endpoint that does not serve it; the caller supplies a name from the
// provider's own list.
func ProbeInference(ctx context.Context, client *http.Client, baseURL, apiKey, model string) error {
	u, err := ValidateOperatorBaseURL(ctx, baseURL)
	if err != nil {
		return redact(err, apiKey)
	}
	if client == nil {
		client = NewClient()
	}
	return fetchInference(ctx, client, u, apiKey, model)
}

// probeMaxTokens is AC3's ceiling: enough to exercise authentication, ~1 token
// of spend.
//
// It stays 1 deliberately, even though some gateways want more. 9Router raised
// its own ping to 1024 because it reads the *body* and a reasoning model can
// spend a whole budget on chain-of-thought and return no choices (their issue
// #3010). We never read the body: the status code is the entire answer, so a
// 200 with an empty completion is still a 200. One token is enough to prove the
// credential, and reading only the status is what keeps it that cheap.
const probeMaxTokens = 1

// Errors that classify an upstream answer. The caller cannot derive this split
// from the status code alone, and getting it wrong is not cosmetic: a gateway
// fronting many upstreams answers 401 for a single model whose upstream is down
// while the credential it was handed is fine, and it answers 502 for a model
// that is merely misconfigured.
//
//   - ErrCredentialRejected — the upstream actively refused the credential. The
//     only statuses that say this, and only when no model has yet answered 2xx.
//   - ErrUpstreamError — the upstream answered, badly. The credential is not
//     what it complained about, so the probe may try another model.
//   - ErrUnreachable — nothing answered at all.
var (
	ErrCredentialRejected = errors.New("credential rejected")
	ErrUpstreamError      = errors.New("upstream error")
	ErrUnreachable        = errors.New("upstream unreachable")
)

// classifyStatus maps an upstream status to the sentinel above. 401 and 403 are
// the whole "rejected" set: every other non-2xx (400 malformed model, 402 out of
// credit, 429 rate limit, 5xx upstream down) says something about the request or
// the upstream's health, not about whether the credential is genuine.
//
// This is 9Router's rule — "400/529 still confirms key accepted; only 401/403 =
// bad key" — with the same two statuses, because it is the only line the HTTP
// spec actually draws.
func classifyStatus(code int) error {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrCredentialRejected
	default:
		return ErrUpstreamError
	}
}

// localhostHint explains the one mistake a non-routable base URL invites, and it
// is attached only to a *dial* failure against one of those hosts — never to a
// validation error, and never as a rejection.
//
// The failure it describes is real and confusing: a container's `localhost` is
// the container, not the host, so an endpoint the operator can reach from their
// own shell is unreachable from AgentDeck. Measured on the deployment this was
// written for — from inside the api container, `localhost:20128` gives
// "connection refused" while the same gateway answers 200 on
// `host.docker.internal:20128`.
//
// The replacement it names is the same string the allowlist accepts, so the hint
// cannot send the operator to an address the guard would refuse. That is checked
// by a test rather than by memory.
//
// It is a hint rather than a validation rule on purpose. Whether loopback works
// depends on how AgentDeck is deployed, and the process cannot know that
// reliably: sniffing /.dockerenv would make validation depend on a hidden
// runtime condition, and would reject a perfectly valid address in the
// non-container deployment the product also supports. The address is legal; the
// operator just may not know which machine it names.
const localhostHint = " (this address is not reachable from AgentDeck: inside a container, " +
	"localhost is the container itself — if the endpoint runs on the host, use host.docker.internal)"

// withLocalhostHint appends the container hint when err is a dial failure
// against a loopback address. Split out so both the model fetch and the
// inference probe explain the same failure the same way.
func withLocalhostHint(err error, u *url.URL) error {
	if err == nil || u == nil || !IsLocalProviderHost(strings.ToLower(u.Hostname())) {
		return err
	}
	return fmt.Errorf("%w%s", err, localhostHint)
}

// inferenceProbe is the smallest OpenAI-compatible completion request.
type inferenceProbe struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	Messages  []probeMessage `json:"messages"`
}

type probeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// fetchInference performs the guarded probe against an already-validated base
// URL. Split out for the same reason fetchModels is: tests point it at an
// httptest server, and only the loopback allowlist makes that reachable.
func fetchInference(ctx context.Context, client *http.Client, u *url.URL, apiKey, model string) error {
	u.Path = strings.TrimRight(u.Path, "/") + "/chat/completions"
	u.RawQuery, u.Fragment = "", ""

	payload, err := json.Marshal(inferenceProbe{
		Model:     model,
		MaxTokens: probeMaxTokens,
		Messages:  []probeMessage{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		return fmt.Errorf("provider: encoding probe: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return redact(fmt.Errorf("provider: building request: %w", err), apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return redact(withLocalhostHint(
			fmt.Errorf("provider: POST %s: %w: %w", u.Host, ErrUnreachable, err), u), apiKey)
	}
	defer resp.Body.Close()

	// Drain a bounded amount so the connection can be reused, and so a hostile
	// endpoint cannot stream forever. The body is never read for content: the
	// status code is the whole answer, and a completion body could echo the
	// prompt back into an error message.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("provider: %s rejected the inference probe for model %q with HTTP %d: %w",
			u.Host, model, resp.StatusCode, classifyStatus(resp.StatusCode))
	}
	return nil
}

// modelsEnvelope covers both shapes seen in the wild: the OpenAI-compatible
// {"data":[{"id":...}]} and the {"models":[...]} used by Ollama and some
// gateways.
type modelsEnvelope struct {
	Data   []modelEntry `json:"data"`
	Models []modelEntry `json:"models"`
}

type modelEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func parseModels(body []byte) ([]string, error) {
	var env modelsEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	entries := env.Data
	if len(entries) == 0 {
		entries = env.Models
	}
	out := make([]string, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		id := e.ID
		if id == "" {
			id = e.Name
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, errors.New("no models in response")
	}
	return out, nil
}

// redact strips the API key from an error message. Nothing we build puts the
// key in the URL, but the key travels through net/http and url.Error, so this
// is the belt to that braces.
func redact(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, secret) {
		return err
	}
	return errors.New(strings.ReplaceAll(msg, secret, "[redacted]"))
}
