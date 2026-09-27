package storage

// Klien objek: HEAD (untuk memverifikasi unggahan) dan key builder.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotFound: objeknya tidak ada di storage.
var ErrNotFound = errors.New("storage: objek tidak ditemukan")

// ObjectInfo adalah hasil HEAD atas sebuah objek.
type ObjectInfo struct {
	Size int64
	// ContentType apa adanya dari storage. Bisa kosong kalau pengunggah tidak
	// menyetelnya.
	ContentType string
	// ETag adalah MD5 objek menurut S3, bukan SHA-256. Karena itu ia tidak
	// dipakai untuk memverifikasi isi: N22/SHA-256 di 12.3 adalah digest yang
	// dihitung AgentDeck sendiri, dan ETag tidak bisa menggantikannya.
	ETag string
}

// Client berbicara ke endpoint S3-compatible.
type Client struct {
	cfg       Config
	presigner *Presigner
	http      *http.Client
}

// NewClient merakit klien dari konfigurasi.
func NewClient(cfg Config) (*Client, error) {
	p, err := NewPresigner(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{
		cfg:       cfg,
		presigner: p,
		http:      &http.Client{Timeout: 15 * time.Second},
	}, nil
}

// PresignPut meneruskan ke presigner.
func (c *Client) PresignPut(key, contentType string, ttl time.Duration) (string, error) {
	return c.presigner.PresignPut(key, contentType, ttl)
}

// PresignGet meneruskan ke presigner.
func (c *Client) PresignGet(key string, ttl time.Duration) (string, error) {
	return c.presigner.PresignGet(key, ttl)
}

// Head mengambil metadata objek.
//
// Dipakai untuk membuktikan bahwa unggahan benar-benar mendarat: executor
// mengirim `storage_key` setelah mengunggah, dan tanpa HEAD server hanya bisa
// percaya. `size` dari HEAD dibandingkan dengan yang didaftarkan, jadi baris
// `artifacts` tidak bisa menyebut ukuran yang tidak ada.
func (c *Client) Head(ctx context.Context, key string) (ObjectInfo, error) {
	u, err := c.presigner.PresignHead(key, time.Minute)
	if err != nil {
		return ObjectInfo{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return ObjectInfo{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return ObjectInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return ObjectInfo{}, ErrNotFound
	case http.StatusOK:
	default:
		return ObjectInfo{}, fmt.Errorf("storage: HEAD %s = %d", key, resp.StatusCode)
	}
	return ObjectInfo{
		Size:        resp.ContentLength,
		ContentType: resp.Header.Get("Content-Type"),
		ETag:        strings.Trim(resp.Header.Get("ETag"), `"`),
	}, nil
}

// Fetch mengunduh isi objek, dibatasi `limit` byte.
//
// Dipakai untuk memverifikasi SHA-256 saat pendaftaran artifact (12.3 langkah
// 6). `limit` bukan hiasan: tanpa itu, satu permintaan pendaftaran bisa memaksa
// server menarik objek sebesar apa pun ke memori, dan yang mengendalikan
// ukurannya adalah klien.
//
// Objek yang lebih besar dari `limit` TIDAK dipotong diam-diam — itu akan
// menghasilkan digest yang selalu salah dan pesan yang menyesatkan. Ia
// mengembalikan error, dan pemanggil yang memutuskan apa artinya.
func (c *Client) Fetch(ctx context.Context, key string, limit int64) ([]byte, error) {
	u, err := c.presigner.PresignGet(key, time.Minute)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("storage: GET %s = %d", key, resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("storage: objek %d byte melebihi batas baca %d", resp.ContentLength, limit)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("storage: objek melebihi batas baca %d", limit)
	}
	return body, nil
}

// ArtifactKey menyusun key objek.
//
// Bentuknya dari 3.15: `artifacts/{org_id}/{task_id}/{run_id}/{ulid}-{filename}`.
// `ulid` di depan filename, bukan menggantinya: dua unggahan dengan nama sama
// di satu run harus jadi dua objek, dan filename aslinya tetap terbaca saat
// operator melihat isi bucket.
func ArtifactKey(orgID, taskID, runID, artifactID, filename string) string {
	return strings.Join([]string{
		"artifacts", orgID, taskID, runID, artifactID + "-" + sanitizeFilename(filename),
	}, "/")
}

// sanitizeFilename membuang karakter yang membuat key ambigu atau berbahaya.
//
// `/` dibuang karena akan mengubah struktur key; `..` dibuang karena beberapa
// klien S3 memperlakukannya sebagai navigasi path. Sisanya dibiarkan apa
// adanya — nama file operator bukan tempat AgentDeck menormalkan unicode.
func sanitizeFilename(name string) string {
	// Backslash jadi slash dulu, lalu setiap segmen dinormalkan. Urutannya
	// penting: `..\\..\\x` harus terlihat sebagai dua segmen `..`, bukan satu
	// token yang kebetulan berisi titik.
	name = strings.ReplaceAll(name, "\\", "/")
	segments := strings.Split(name, "/")
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		// Segmen `..` dibuang SELURUHNYA, bukan diganti underscore: `..` yang
		// jadi `_` masih menghasilkan key yang terlihat seperti path, sementara
		// yang benar-benar diinginkan adalah nama filenya saja. Ini juga yang
		// bikin `../../etc/passwd` jadi `etc/passwd` — bentuk yang sama dengan
		// input tanpa traversal.
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		// Titik di TENGAH segmen (`laporan..pdf`) bukan traversal, tapi dibuang
		// juga supaya hasilnya tidak pernah memuat `..` — invarian yang lebih
		// mudah diperiksa daripada daftar kasus.
		seg = strings.ReplaceAll(seg, "..", "_")
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		out = append(out, seg)
	}
	if len(out) == 0 {
		return "artifact"
	}
	return strings.Join(out, "/")
}

// SHA256Hex menghitung digest isi, bentuknya yang dipakai kolom `artifacts.sha256`
// (hex 64 karakter).
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
