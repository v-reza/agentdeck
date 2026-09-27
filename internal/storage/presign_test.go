package storage

// Tes presigner — diuji lawan server S3-compatible SUNGGUHAN, bukan mock.
//
// Kenapa harus server nyata: presigned URL adalah tanda tangan kriptografis
// atas canonical request. Salah satu detail saja (urutan parameter, encoding
// `/`, `host` yang membawa port default, payload hash yang salah) menghasilkan
// `SignatureDoesNotMatch` — tapi kalau yang diperiksa hanya "URL-nya
// mengandung X-Amz-Signature", semuanya terlihat benar. Satu-satunya cara
// membuktikan adalah mengirim URL itu ke server dan melihat server menerimanya.
//
// Dijalankan kalau `AGENTDECK_TEST_S3_ENDPOINT` di-set (lihat compose/CI);
// tanpa itu tes dilewati, sama seperti tes Postgres di paket lain.

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	endpoint := os.Getenv("AGENTDECK_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("AGENTDECK_TEST_S3_ENDPOINT tidak di-set")
	}
	return Config{
		Endpoint:  endpoint,
		Region:    "auto",
		Bucket:    envOr("AGENTDECK_TEST_S3_BUCKET", "agentdeck-artifacts"),
		AccessKey: envOr("AGENTDECK_TEST_S3_ACCESS_KEY", "agentdeck"),
		SecretKey: envOr("AGENTDECK_TEST_S3_SECRET_KEY", "agentdeck_local_dev"),
		// Path-style wajib di sini: host-nya `127.0.0.1:9000`, dan
		// virtual-host style akan menghasilkan `bucket.127.0.0.1` yang bukan
		// nama host yang bisa di-resolve.
		PathStyle: true,
	}
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

// requireBucket memastikan bucket-nya ada sebelum tes berjalan.
//
// Tidak dibuat dari sini. Percobaan pertama memakai presigned PUT ke `/bucket`,
// dan MinIO menolaknya (403): pembuatan bucket bukan operasi objek, dan server
// S3-compatible tidak menerimanya lewat URL presigned. Di produksi bucket R2
// juga dibuat operator lewat dashboard, bukan oleh API ini — jadi tesnya
// menyesuaikan diri, bukan memaksa jalur yang tidak ada.
//
// Caranya membedakan dua kegagalan yang KEDUANYA 404:
//
//	NoSuchKey    -> bucket ada, signature sah, objeknya saja yang tidak ada
//	NoSuchBucket -> bucket belum dibuat
//
// Kode error-nya dibaca dari body, bukan dari status. Kalau bucket belum ada,
// tes dilewati dengan perintah yang harus dijalankan — bukan gagal, karena
// kegagalannya bukan di kode.
func requireBucket(t *testing.T, cfg Config) {
	t.Helper()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	probe := fmt.Sprintf("artifacts/probe/%d/ada-bucket-tidak.txt", time.Now().UnixNano())
	u, err := client.PresignGet(probe, time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("GET probe: %v", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Fatal("objek probe ada padahal seharusnya dibuat unik")
	}
	// 403 di sini berarti signature-nya yang salah, bukan bucketnya yang hilang:
	// itu kegagalan nyata dan harus terlihat.
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("presigned GET ditolak (signature salah): %s", body)
	}
	if strings.Contains(string(body), "NoSuchBucket") {
		t.Skipf("bucket %q belum ada; jalankan:\n"+
			"  docker exec <minio> mc alias set local %s %s %s && \\\n"+
			"  docker exec <minio> mc mb --ignore-existing local/%s",
			cfg.Bucket, cfg.Endpoint, cfg.AccessKey, cfg.SecretKey, cfg.Bucket)
	}
	if !strings.Contains(string(body), "NoSuchKey") {
		t.Fatalf("status %d, body tak dikenal: %s", resp.StatusCode, body)
	}
}

// TestPresignedPutAndGetRoundTrip adalah bukti intinya: URL yang kita tanda
// tangani diterima server, dan byte yang diunggah bisa dibaca kembali persis.
func TestPresignedPutAndGetRoundTrip(t *testing.T) {
	cfg := testConfig(t)
	requireBucket(t, cfg)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	key := fmt.Sprintf("artifacts/test/%d/roundtrip.txt", time.Now().UnixNano())
	content := []byte("halo dari presigned URL\n")

	putURL, err := client.PresignPut(key, "text/plain", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	req, err := http.NewRequest(http.MethodPut, putURL, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("new PUT: %v", err)
	}
	// Content-Type harus SAMA dengan yang ditandatangani; server membandingkan
	// header yang ditandatangani dengan yang dikirim.
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT presigned = %d: %s", resp.StatusCode, body)
	}

	getURL, err := client.PresignGet(key, 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	got, err := http.Get(getURL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = got.Body.Close() }()
	read, _ := io.ReadAll(got.Body)
	if got.StatusCode != http.StatusOK {
		t.Fatalf("GET presigned = %d", got.StatusCode)
	}
	if !bytes.Equal(read, content) {
		t.Fatalf("isi = %q, want %q", read, content)
	}
}

// TestPresignedURLIsRejectedWithTheWrongSecret: tanda tangan harus benar-benar
// bergantung pada secret. Tanpa ini, "URL diterima" tidak membuktikan apa pun.
func TestPresignedURLIsRejectedWithTheWrongSecret(t *testing.T) {
	cfg := testConfig(t)
	requireBucket(t, cfg)

	bad := cfg
	bad.SecretKey = "secret-yang-salah"
	client, err := NewClient(bad)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	key := fmt.Sprintf("artifacts/test/%d/wrong-secret.txt", time.Now().UnixNano())
	putURL, err := client.PresignPut(key, "text/plain", 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPut, putURL, bytes.NewReader([]byte("x")))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("server menerima URL bertanda tangan secret yang salah")
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 SignatureDoesNotMatch", resp.StatusCode)
	}
}

// TestPresignedURLExpires: TTL-nya benar-benar berlaku di server.
func TestPresignedURLExpires(t *testing.T) {
	cfg := testConfig(t)
	requireBucket(t, cfg)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	key := fmt.Sprintf("artifacts/test/%d/expired.txt", time.Now().UnixNano())
	// TTL negatif: URL lahir sudah kedaluwarsa.
	putURL, err := client.PresignPut(key, "text/plain", -time.Minute)
	if err != nil {
		t.Fatalf("PresignPut: %v", err)
	}
	req, _ := http.NewRequest(http.MethodPut, putURL, bytes.NewReader([]byte("x")))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("URL kedaluwarsa masih diterima")
	}
}

// TestHeadReportsSizeAndMissingKey: HEAD adalah cara server membuktikan
// unggahan benar-benar mendarat.
func TestHeadReportsSizeAndMissingKey(t *testing.T) {
	cfg := testConfig(t)
	requireBucket(t, cfg)

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	key := fmt.Sprintf("artifacts/test/%d/head.bin", time.Now().UnixNano())
	content := bytes.Repeat([]byte("a"), 1234)

	putURL, _ := client.PresignPut(key, "application/octet-stream", 5*time.Minute)
	req, _ := http.NewRequest(http.MethodPut, putURL, bytes.NewReader(content))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT = %d", resp.StatusCode)
	}

	info, err := client.Head(context.Background(), key)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if info.Size != int64(len(content)) {
		t.Fatalf("size = %d, want %d", info.Size, len(content))
	}

	missing := fmt.Sprintf("artifacts/test/%d/tidak-ada.bin", time.Now().UnixNano())
	if _, err := client.Head(context.Background(), missing); err != ErrNotFound {
		t.Fatalf("Head objek hilang = %v, want ErrNotFound", err)
	}
}

// TestPresignRejectsIncompleteConfig.
func TestPresignRejectsIncompleteConfig(t *testing.T) {
	cases := []Config{
		{},
		{Endpoint: "https://x.example", Bucket: "b", AccessKey: "a"},
		{Bucket: "b", AccessKey: "a", SecretKey: "s"},
		{Endpoint: "ftp://x.example", Bucket: "b", AccessKey: "a", SecretKey: "s"},
	}
	for i, cfg := range cases {
		if _, err := NewPresigner(cfg); err == nil {
			t.Errorf("kasus %d: konfigurasi tidak lengkap diterima", i)
		}
	}
}

// TestArtifactKeyShape menaku bentuk key 3.15.
func TestArtifactKeyShape(t *testing.T) {
	got := ArtifactKey("org1", "task1", "run1", "01ABC", "report.pdf")
	want := "artifacts/org1/task1/run1/01ABC-report.pdf"
	if got != want {
		t.Fatalf("ArtifactKey = %q, want %q", got, want)
	}
}

// TestArtifactKeySanitizesFilename: filename tidak boleh mengubah struktur key.
func TestArtifactKeySanitizesFilename(t *testing.T) {
	cases := map[string]string{
		"../../etc/passwd":    "etc/passwd",
		`..\\..\\etc\\passwd`: "etc/passwd",
		"a/b/c.txt":           "a/b/c.txt",
		"":                    "artifact",
		"   ":                 "artifact",
		"laporan akhir.pdf":   "laporan akhir.pdf",
		// Path absolut tidak boleh menghasilkan leading slash (key kosong di depan).
		"/etc/shadow": "etc/shadow",
		"a//b.txt":    "a/b.txt",
	}
	for in, want := range cases {
		got := sanitizeFilename(in)
		if got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
		// Invarian yang benar-benar melindungi: tidak ada segmen `..` yang lolos,
		// dan tidak ada leading slash.
		if strings.HasPrefix(got, "/") {
			t.Errorf("sanitizeFilename(%q) = %q, berawalan slash", in, got)
		}
		for _, seg := range strings.Split(got, "/") {
			if seg == ".." {
				t.Errorf("sanitizeFilename(%q) = %q, masih memuat segmen ..", in, got)
			}
		}
	}
}

// TestEncodePathKeepsSeparators: `/` tetap `/`, sisanya di-encode.
func TestEncodePathKeepsSeparators(t *testing.T) {
	cases := map[string]string{
		"a/b/c.txt":       "a/b/c.txt",
		"a/laporan final": "a/laporan%20final",
		"a/b+c":           "a/b%2Bc",
		"a/b&c":           "a/b%26c",
		"a/b=c":           "a/b%3Dc",
		"a/ünicode.txt":   "a/%C3%BCnicode.txt",
	}
	for in, want := range cases {
		if got := encodePath(in); got != want {
			t.Errorf("encodePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestEncodeQueryIsNotQueryEscape: `+` bukan pengganti spasi di SigV4.
func TestEncodeQueryIsNotQueryEscape(t *testing.T) {
	if got := encodeQuery("a b"); got != "a%20b" {
		t.Fatalf("encodeQuery(\"a b\") = %q, want a%%20b (bukan a+b)", got)
	}
	if got := encodeQuery("a+b"); got != "a%2Bb" {
		t.Fatalf("encodeQuery(\"a+b\") = %q, want a%%2Bb", got)
	}
	if got := encodeQuery("~-_.~"); got != "~-_.~" {
		t.Fatalf("encodeQuery unreserved = %q, want tidak diubah", got)
	}
}

// TestCanonicalQueryIsSortedAfterEncoding: urutan ditentukan hasil encoding.
func TestCanonicalQueryIsSortedAfterEncoding(t *testing.T) {
	q := map[string]string{"b": "2", "a": "1", "Z": "3"}
	got := canonicalQueryString(q)
	want := "Z=3&a=1&b=2"
	if got != want {
		t.Fatalf("canonicalQueryString = %q, want %q", got, want)
	}
	parts := strings.Split(got, "&")
	if !sort.StringsAreSorted(parts) {
		t.Fatal("hasil tidak terurut")
	}
}

// TestHostDropsDefaultPort: `:443` di https membuat signature tidak cocok.
func TestHostDropsDefaultPort(t *testing.T) {
	p, err := NewPresigner(Config{
		Endpoint: "https://r2.example.com:443", Bucket: "b",
		AccessKey: "a", SecretKey: "s",
	})
	if err != nil {
		t.Fatalf("NewPresigner: %v", err)
	}
	if p.host != "r2.example.com" {
		t.Fatalf("host = %q, want r2.example.com tanpa port", p.host)
	}
	// Port non-default HARUS dipertahankan.
	p2, err := NewPresigner(Config{
		Endpoint: "http://127.0.0.1:9000", Bucket: "b",
		AccessKey: "a", SecretKey: "s", PathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewPresigner: %v", err)
	}
	if p2.host != "127.0.0.1:9000" {
		t.Fatalf("host = %q, want 127.0.0.1:9000", p2.host)
	}
}

// TestDeriveSigningKeyMatchesTheSpec memaku rantai HMAC SigV4 dengan vektor
// dari dokumentasi AWS, service `s3`.
//
// Dua vektor dari dokumentasi yang sama dipakai, dengan date/region/secret
// identik, supaya perbedaan hasilnya hanya ditentukan service — itu yang
// membuktikan `s3` yang dipakai, bukan `iam`. Vektor yang beredar untuk
// service `iam` menghasilkan nilai yang berbeda, dan memakainya di sini akan
// memaku rantai yang salah.
func TestDeriveSigningKeyMatchesTheSpec(t *testing.T) {
	cases := []struct {
		service string
		want    string
	}{
		{"s3", "32f78051dcde24c552811d654f4a769112bb834b03975cdd6b1fd7d16248c269"},
		{"iam", "c4afb1cc5771d871763a393e44b703571b55cc28424d1a5e86da6ed3c154a4b9"},
	}
	const secret = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	for _, c := range cases {
		got := hex.EncodeToString(deriveSigningKeyFor(secret, "20150830", "us-east-1", c.service))
		if got != c.want {
			t.Errorf("deriveSigningKeyFor(service=%s) = %s, want %s", c.service, got, c.want)
		}
	}
}

// TestSHA256HexShape: kolom artifacts.sha256 adalah hex 64 karakter.
func TestSHA256HexShape(t *testing.T) {
	got := SHA256Hex([]byte("abc"))
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("SHA256Hex = %s, want %s", got, want)
	}
	if len(got) != 64 {
		t.Fatalf("panjang = %d, want 64", len(got))
	}
}
