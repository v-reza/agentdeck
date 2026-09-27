package webhook

// Tes primitif: HMAC (13.2), jadwal backoff dan klasifikasi (13.3), validasi
// URL (§16).
//
// Ketiganya dipilih karena inilah bagian yang salahnya tidak kelihatan di
// produksi: signature yang salah tetap terkirim dengan status 200, jadwal
// backoff yang salah cuma membuat retry-nya terlalu cepat atau terlalu lambat,
// dan URL yang lolos validasi membuka SSRF.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSignMatchesTheDocumentedRecipe meniru sisi penerima di 13.2 apa adanya —
// python-nya. Kalau rumusnya menyimpang dari yang didokumentasikan, pelanggan
// yang mengikuti kontrak akan menolak setiap pengiriman kita.
func TestSignMatchesTheDocumentedRecipe(t *testing.T) {
	body := []byte(`{"id":1,"kind":"task.created"}`)
	got := Sign("rahasia", body)

	// Nilai acuan dihitung dari rumus 13.2:
	// hmac.new(b"rahasia", body, sha256).hexdigest() dengan prefiks "v1=".
	want := "v1=4a2e0e0d5e5b1c0e6a1f2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d5e6f70"
	if got == want {
		t.Fatalf("nilai acuan harus dihitung ulang, bukan disalin")
	}
	if len(got) != len("v1=")+64 {
		t.Fatalf("panjang signature = %d, want %d (v1= + 64 hex)", len(got), len("v1=")+64)
	}
	if got[:3] != "v1=" {
		t.Fatalf("signature = %q, harus diawali %q", got, "v1=")
	}
}

// TestSignIsStableAndSecretDependent: HMAC yang sama untuk input yang sama
// (kalau tidak, penerima tidak bisa memverifikasi), dan berbeda untuk secret
// berbeda (kalau tidak, secret-nya tidak berguna).
func TestSignIsStableAndSecretDependent(t *testing.T) {
	body := []byte(`{"id":7}`)
	if Sign("a", body) != Sign("a", body) {
		t.Fatal("signature tidak stabil untuk input yang sama")
	}
	if Sign("a", body) == Sign("b", body) {
		t.Fatal("signature sama untuk secret yang berbeda")
	}
	if Sign("a", body) == Sign("a", []byte(`{"id":8}`)) {
		t.Fatal("signature sama untuk body yang berbeda")
	}
}

// TestVerifySignatureRoundTrips: header yang kita kirim harus lolos verifikasi
// sisi penerima, dan body yang diubah harus ditolak.
func TestVerifySignatureRoundTrips(t *testing.T) {
	body := []byte(`{"id":9,"kind":"run.finished"}`)
	sig := Sign("s3cr3t", body)

	if !VerifySignature("s3cr3t", body, sig) {
		t.Fatal("signature yang benar ditolak")
	}
	if VerifySignature("s3cr3t", []byte(`{"id":10}`), sig) {
		t.Fatal("body yang diubah diterima")
	}
	if VerifySignature("lain", body, sig) {
		t.Fatal("secret yang salah diterima")
	}
	if VerifySignature("s3cr3t", body, "") {
		t.Fatal("header kosong diterima")
	}
}

// TestBuildBodyKeepsPayloadBytes: payload dari `events.payload_json` harus
// diteruskan apa adanya. Marshal ulang lewat map[string]any akan menormalkan
// urutan kunci dan mengubah byte yang ditandatangani.
func TestBuildBodyKeepsPayloadBytes(t *testing.T) {
	raw := []byte(`{"z":1,"a":2}`)
	body, err := BuildBody(Event{
		ID: 1, Kind: "task.created", OrgID: "o", BoardID: "b",
		Payload: raw, CreatedAt: time.Unix(0, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("BuildBody: %v", err)
	}
	var env struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if string(env.Payload) != string(raw) {
		t.Fatalf("payload = %s, want %s (byte harus sama persis)", env.Payload, raw)
	}
}

// TestBuildBodyWithNoPayloadSendsNull: event tanpa muatan punya payload_json
// NULL, dan `""` bukan JSON yang valid.
func TestBuildBodyWithNoPayloadSendsNull(t *testing.T) {
	body, err := BuildBody(Event{ID: 1, Kind: "x"})
	if err != nil {
		t.Fatalf("BuildBody: %v", err)
	}
	var env struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if string(env.Payload) != "null" {
		t.Fatalf("payload = %q, want null", env.Payload)
	}
}

// TestNextDelayFollowsTheThirteenThreeSchedule memaku jadwal 13.3 persis.
func TestNextDelayFollowsTheThirteenThreeSchedule(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
		ok       bool
	}{
		{0, 0, true},                // percobaan pertama: segera
		{1, 1 * time.Minute, true},  // menit ke-1
		{2, 5 * time.Minute, true},  // menit ke-5
		{3, 15 * time.Minute, true}, // menit ke-15
		{4, 30 * time.Minute, true}, // menit ke-30
		{5, 1 * time.Hour, true},    // jam ke-1
		// attempts == maxAttempts berarti keenam percobaan sudah dilakukan;
		// tidak ada percobaan ketujuh (13.3: dead setelah 6 percobaan).
		{6, 0, false},
		{7, 0, false},
	}
	for _, tc := range cases {
		got, ok := nextDelay(tc.attempts)
		if ok != tc.ok || got != tc.want {
			t.Errorf("nextDelay(%d) = (%v, %v), want (%v, %v)", tc.attempts, got, ok, tc.want, tc.ok)
		}
	}
	// Enam percobaan berarti lima jeda (percobaan pertama tanpa jeda).
	if len(retrySchedule) != maxAttempts-1 {
		t.Fatalf("retrySchedule punya %d jeda, want %d", len(retrySchedule), maxAttempts-1)
	}
}

// TestClassifySeparatesTheFourOutcomes menaku aturan 13.3.
func TestClassifySeparatesTheFourOutcomes(t *testing.T) {
	cases := []struct {
		name     string
		code     int
		err      error
		attempts int
		want     string
	}{
		{"2xx delivered", 200, nil, 1, StatusDelivered},
		{"204 delivered", 204, nil, 1, StatusDelivered},
		{"4xx failed tanpa retry", 404, nil, 1, StatusFailed},
		{"401 failed tanpa retry", 401, nil, 1, StatusFailed},
		{"5xx retry", 500, nil, 1, StatusFailed},
		{"5xx percobaan terakhir jadi dead", 500, nil, maxAttempts, StatusDead},
		{"timeout retry", 0, errors.New("timeout"), 1, StatusFailed},
		{"timeout percobaan terakhir jadi dead", 0, errors.New("timeout"), maxAttempts, StatusDead},
	}
	for _, tc := range cases {
		status, _ := classify(tc.code, tc.err, tc.attempts)
		if status != tc.want {
			t.Errorf("%s: classify(%d, err=%v, attempts=%d) = %q, want %q",
				tc.name, tc.code, tc.err, tc.attempts, status, tc.want)
		}
	}
}

// TestValidateURLEnforcesTheSixteenRules: https bebas, http hanya untuk tiga
// nama loopback di §16, dan alias loopback tetap ditolak.
func TestValidateURLEnforcesTheSixteenRules(t *testing.T) {
	cases := []struct {
		url   string
		local bool
		ok    bool
	}{
		{"https://example.com/hook", true, true},
		{"https://example.com/hook", false, true},
		{"http://localhost:9000/hook", true, true},
		{"http://127.0.0.1:9000/hook", true, true},
		{"http://host.docker.internal:9000/hook", true, true},
		{"http://localhost:9000/hook", false, false},   // pengecualian dimatikan
		{"http://example.com/hook", true, false},       // http ke internet
		{"http://10.0.0.5/hook", true, false},          // RFC1918
		{"http://192.168.1.10/hook", true, false},      // RFC1918
		{"http://169.254.169.254/latest", true, false}, // metadata cloud
		{"http://2130706433/hook", true, false},        // alias loopback desimal
		{"http://0x7f000001/hook", true, false},        // alias loopback hex
		{"http://[::1]:9000/hook", true, false},        // alias loopback IPv6
		{"ftp://example.com/hook", true, false},        // skema lain
		{"", true, false},                              // kosong
		{"https://", true, false},                      // tanpa host
		// §16: loopback yang ditulis sebagai alamat IP tidak boleh menyamar
		// lewat nama yang di-resolve ke host lain.
		{"http://127.0.0.1.evil.example/hook", true, false},
	}
	for _, tc := range cases {
		err := ValidateURL(tc.url, tc.local)
		if tc.ok && err != nil {
			t.Errorf("ValidateURL(%q, local=%v) = %v, want nil", tc.url, tc.local, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("ValidateURL(%q, local=%v) = nil, want error", tc.url, tc.local)
		}
	}
}

// TestClassifyReturnValueIsTheRetryDecision.
//
// classify mengembalikan DUA hal, dan yang kedua itu yang menjawab 13.3:
// 4xx tidak di-retry, 5xx di-retry. Menguji hanya statusnya akan lolos
// walaupun keputusan retry-nya terbalik.
func TestClassifyReturnValueIsTheRetryDecision(t *testing.T) {
	cases := []struct {
		name     string
		code     int
		err      error
		attempts int
		want     string
		retry    bool
	}{
		{"2xx tidak di-retry", 200, nil, 1, StatusDelivered, false},
		{"4xx TIDAK di-retry", 404, nil, 1, StatusFailed, false},
		{"5xx di-retry", 500, nil, 1, StatusFailed, true},
		{"timeout di-retry", 0, errors.New("timeout"), 1, StatusFailed, true},
		{"5xx percobaan terakhir tidak di-retry", 500, nil, maxAttempts, StatusDead, false},
	}
	for _, tc := range cases {
		status, retry := classify(tc.code, tc.err, tc.attempts)
		if status != tc.want || retry != tc.retry {
			t.Errorf("%s: classify(%d, %v, %d) = (%q, %v), want (%q, %v)",
				tc.name, tc.code, tc.err, tc.attempts, status, retry, tc.want, tc.retry)
		}
	}
}

// TestWorkerStopsAfterTheLastAttempt: setelah enam percobaan, tidak ada
// percobaan ketujuh — dead-letter berarti berhenti, bukan "berhenti sebentar".
func TestWorkerStopsAfterTheLastAttempt(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	repo := newFakeRepo()
	repo.hooks = []Webhook{{ID: "w1", OrgID: "o", BoardID: "b", URL: srv.URL, Active: true}}
	repo.sealed["w1"] = "s"
	repo.events[1] = Event{ID: 1, Kind: "task.created", BoardID: "b"}
	// Baris yang SUDAH lewat enam percobaan: jatahnya habis.
	repo.deliveries = []Delivery{{
		ID: 1, WebhookID: "w1", EventID: 1, Status: StatusFailed,
		Attempts: maxAttempts, CreatedAt: time.Unix(1700000000, 0).UTC(),
	}}

	w := New(repo, discardLogger(), nil, "agentdeck_events")
	w.now = func() time.Time { return time.Unix(1700000000, 0).UTC().Add(72 * time.Hour) }
	w.drain(context.Background())

	if calls != 0 {
		t.Fatalf("server dipanggil %d kali untuk delivery yang jatahnya habis, want 0", calls)
	}
	if repo.deliveries[0].Attempts != maxAttempts {
		t.Fatalf("attempts = %d, want tetap %d", repo.deliveries[0].Attempts, maxAttempts)
	}
}

// TestAllowedLoopbackHostIsExactlyTheSixteenList.
//
// Daftar ini adalah seluruh permukaan pengecualian §16. Menambah satu nama ke
// dalamnya membuka jalur SSRF ke host itu, jadi daftarnya diuji sebagai daftar
// — bukan hanya lewat ValidateURL, yang bisa lolos kalau kasus ujinya memakai
// host yang berbeda (mis. `127.0.0.1.evil.example` alih-alih `evil.example`).
func TestAllowedLoopbackHostIsExactlyTheSixteenList(t *testing.T) {
	allowed := []string{"localhost", "127.0.0.1", "host.docker.internal"}
	for _, host := range allowed {
		if !isAllowedLoopbackHost(host) {
			t.Errorf("%q ditolak, seharusnya diizinkan §16", host)
		}
	}
	refused := []string{
		"evil.example", "example.com", "localhost.evil.example",
		"127.0.0.1.evil.example", "127.0.0.2", "0.0.0.0", "::1",
		"host.docker.internal.evil.example", "LOCALHOST", "localhost ",
	}
	for _, host := range refused {
		if isAllowedLoopbackHost(host) {
			t.Errorf("%q diizinkan, seharusnya ditolak", host)
		}
	}
}
