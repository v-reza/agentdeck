package webhook

// Pengiriman HTTP — ARCHITECTURE 13.1, 13.2, 13.3.
//
// Guard SSRF-nya bukan di sini: URL webhook divalidasi saat **didaftarkan**
// (endpoint 6.2.18) memakai `internal/provider` yang sudah memegang aturan §16
// (loopback yang diizinkan persis, RFC1918 dan link-local ditolak). Mengulang
// aturan itu di sini akan jadi salinan kedua yang bisa menyimpang dari yang
// pertama — dan yang menyimpang adalah yang tidak dipakai saat validasi.
// Yang dijaga di sini adalah bentuk request-nya.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
)

// httpClient adalah klien yang dipakai semua pengiriman.
type httpClient struct {
	c *http.Client
}

func newHTTPClient() *httpClient {
	return &httpClient{c: &http.Client{
		// 13.3: timeout 10 detik. Dipasang di klien, bukan per-request, supaya
		// tidak ada jalur pengiriman yang bisa lupa memasangnya.
		Timeout: deliveryTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// Redirect tidak diikuti. URL tujuan sudah diverifikasi operator dan
			// disetujui admin board; mengikuti redirect berarti endpoint itu
			// yang memilih tujuan akhir pengiriman, dan tujuan itu tidak pernah
			// lewat validasi pendaftaran.
			return errors.New("webhook: redirect tidak diikuti")
		},
	}}
}

// send mengirim satu event dan mengembalikan status HTTP serta error jaringan.
//
// Body dibangun SEKALI dan byte yang sama dipakai untuk HMAC dan untuk request
// (13.2). `Content-Type: application/json` dan header signature adalah
// satu-satunya header yang dikirim — tidak ada kredensial AgentDeck yang ikut.
func (w *Worker) send(ctx context.Context, p Pending, secret string) (int, error) {
	body, err := BuildBody(p.Event)
	if err != nil {
		return 0, fmt.Errorf("menyusun body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Subscription.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeader, Sign(secret, body))

	resp, err := w.client.c.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() {
		// Body dibaca habis lalu ditutup: koneksi yang tidak di-drain tidak bisa
		// dipakai ulang, dan setiap pengiriman akan membuka koneksi baru.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	return resp.StatusCode, nil
}

// ValidateURL memeriksa target webhook saat pendaftaran.
//
// Aturannya dari §16 (yang mengalahkan §3.22 dan US-AD106 AC3): skema wajib
// `https`, kecuali tiga nama loopback yang diizinkan persis — `localhost`,
// `127.0.0.1`, `host.docker.internal` — yang boleh lewat `http`.
//
// `allowLocal` adalah sakelar operator: deployment yang tidak pernah memakai
// server inference lokal bisa mematikannya, dan pengecualian yang tidak
// diperlukan adalah pengecualian yang lebih baik tidak ada.
func ValidateURL(raw string, allowLocal bool) error {
	if raw == "" {
		return errors.New("webhook: url wajib diisi")
	}
	u, err := parseURL(raw)
	if err != nil {
		return err
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme != "http" {
		return fmt.Errorf("webhook: skema %q tidak didukung, wajib https", u.Scheme)
	}
	host := u.Hostname()
	if !allowLocal || !isAllowedLoopbackHost(host) {
		return fmt.Errorf("webhook: %q hanya boleh lewat https", host)
	}
	// Loopback yang ditulis sebagai alamat IP tidak boleh menyamar lewat nama
	// lain: `127.0.0.1` yang di-resolve dari DNS publik adalah host lain.
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return fmt.Errorf("webhook: %q bukan loopback", host)
	}
	return nil
}

// parseURL memakai net/url, dan dipisah supaya tes bisa memanggil validasi
// tanpa membangun server.
func parseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("webhook: url tidak valid: %w", err)
	}
	if u.Host == "" {
		return nil, errors.New("webhook: url tanpa host")
	}
	return u, nil
}

// isAllowedLoopbackHost adalah daftar tiga nama di §16, persis sebagai string.
//
// Alias loopback lain (`2130706433`, `0x7f000001`, `0177.0.0.1`, `[::1]`)
// sengaja TIDAK ada di sini: §16 menolaknya secara eksplisit.
func isAllowedLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "host.docker.internal":
		return true
	}
	return false
}
