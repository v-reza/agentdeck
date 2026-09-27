package storage

// Presigned URL S3-compatible — ARCHITECTURE 12.3, 6.2.16.
//
// Cloudflare R2 memakai protokol S3 untuk presigning, jadi implementasinya
// adalah AWS Signature Version 4 mode query-string. Tidak ada SDK yang
// diimpor: yang dibutuhkan cuma HMAC-SHA256, dan menambah SDK untuk empat
// rumus berarti menambah pohon dependensi yang harus diaudit.
//
// Rumusnya diambil dari spesifikasi SigV4 dan diverifikasi lawan server
// S3-compatible sungguhan (MinIO) di test — bukan dari ingatan. Bagian yang
// paling mudah salah, dan yang paling penting di sini:
//
//   - Canonical request memakai path yang di-URI-encode PER SEGMEN, dengan `/`
//     tetap `/`. Meng-encode seluruh path (termasuk `/`) menghasilkan signature
//     yang tidak cocok untuk key bersarang.
//   - Query string diurutkan berdasarkan nama parameter, dan keduanya
//     di-encode dengan aturan yang sama.
//   - Untuk presigned URL, header `host` adalah SATU-SATUNYA header yang
//     ditandatangani, dan nilainya adalah host tanpa skema dan tanpa port
//     default.
//   - Presigned PUT memakai `UNSIGNED-PAYLOAD`: isi body belum ada saat URL
//     dibuat, jadi tidak bisa di-hash.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Config adalah kredensial dan alamat endpoint S3-compatible.
type Config struct {
	Endpoint  string // mis. https://<account>.r2.cloudflarestorage.com
	Region    string // R2 memakai "auto"
	Bucket    string
	AccessKey string
	SecretKey string
	// PathStyle memaksa bucket masuk ke path, bukan subdomain. R2 dan MinIO
	// sama-sama menerima path-style, dan itu satu-satunya bentuk yang bekerja
	// untuk host non-DNS (mis. `minio:9000` di jaringan compose).
	PathStyle bool
}

// Presigner membuat presigned URL.
type Presigner struct {
	cfg  Config
	now  func() time.Time
	host string
	base string
	// pathPrefix masuk ke canonical request: `/<bucket>` di mode path-style,
	// kosong di mode virtual-host.
	pathPrefix string
}

// NewPresigner menyiapkan presigner dari konfigurasi.
func NewPresigner(cfg Config) (*Presigner, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("storage: endpoint, bucket, access key, dan secret wajib diisi")
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("storage: endpoint tidak valid: %w", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("storage: skema endpoint %q tidak didukung", u.Scheme)
	}
	region := cfg.Region
	if region == "" {
		region = "auto"
	}
	cfg.Region = region

	// Host TANPA port default. Menyertakan `:443` pada https membuat signature
	// tidak cocok dengan apa yang dihitung server.
	host := u.Host
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		host = u.Hostname()
	}

	base := u.Scheme + "://" + host
	// pathPrefix adalah bagian path yang BUKAN nama objek.
	//
	// Di mode path-style bucket ada di dalam path (`/bucket/key`), jadi
	// canonical request wajib memuatnya. Menandatangani `/key` sementara
	// permintaan yang dikirim ke `/bucket/key` menghasilkan
	// SignatureDoesNotMatch — dan itu tidak kelihatan sampai URL-nya benar-benar
	// dikirim ke server, karena URL-nya sendiri terlihat benar.
	pathPrefix := ""
	if cfg.PathStyle {
		pathPrefix = "/" + cfg.Bucket
		base += pathPrefix
	} else {
		base = u.Scheme + "://" + cfg.Bucket + "." + host
	}
	return &Presigner{cfg: cfg, now: time.Now, host: host, base: base, pathPrefix: pathPrefix}, nil
}

// PresignPut membuat URL untuk mengunggah objek (12.3 langkah 3).
//
// `contentType` ikut ditandatangani supaya pengunggah tidak bisa mengubahnya
// setelah URL keluar — nilainya dipakai server untuk menyimpan header objek.
func (p *Presigner) PresignPut(key, contentType string, ttl time.Duration) (string, error) {
	if key == "" {
		return "", fmt.Errorf("storage: key wajib diisi")
	}
	// PUT presigned: body belum ada, jadi payload-nya tidak ditandatangani.
	return p.presign("PUT", key, ttl, map[string]string{
		"content-type": contentType,
	}, "UNSIGNED-PAYLOAD")
}

// PresignGet membuat URL untuk mengunduh objek (6.2.16 download).
func (p *Presigner) PresignGet(key string, ttl time.Duration) (string, error) {
	if key == "" {
		return "", fmt.Errorf("storage: key wajib diisi")
	}
	return p.presign("GET", key, ttl, nil, "UNSIGNED-PAYLOAD")
}

// PresignHead membuat URL untuk HEAD objek.
//
// SigV4 menandatangani method, jadi HEAD butuh signature HEAD sendiri — bukan
// signature GET yang dipakai dengan method lain. Dipisah begini supaya
// verifikasi unggahan tidak perlu trik Range yang bergantung pada server
// menghormatinya.
func (p *Presigner) PresignHead(key string, ttl time.Duration) (string, error) {
	if key == "" {
		return "", fmt.Errorf("storage: key wajib diisi")
	}
	return p.presign("HEAD", key, ttl, nil, "UNSIGNED-PAYLOAD")
}

// presign merakit URL final.
func (p *Presigner) presign(method, key string, ttl time.Duration, extra map[string]string, payloadHash string) (string, error) {
	now := p.now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	scope := dateStamp + "/" + p.cfg.Region + "/s3/aws4_request"

	query := map[string]string{
		"X-Amz-Algorithm":     "AWS4-HMAC-SHA256",
		"X-Amz-Credential":    p.cfg.AccessKey + "/" + scope,
		"X-Amz-Date":          amzDate,
		"X-Amz-Expires":       fmt.Sprintf("%d", int(ttl.Seconds())),
		"X-Amz-SignedHeaders": "host",
	}
	// Header yang ikut ditandatangani harus muncul di SignedHeaders juga.
	signedHeaders := []string{"host"}
	for name := range extra {
		signedHeaders = append(signedHeaders, strings.ToLower(name))
	}
	sort.Strings(signedHeaders)
	query["X-Amz-SignedHeaders"] = strings.Join(signedHeaders, ";")

	// Canonical query: diurutkan berdasarkan nama, keduanya di-encode.
	canonicalQuery := canonicalQueryString(query)

	// Canonical headers: satu baris per header, urut, nilai di-trim.
	var hdrLines []string
	for _, name := range signedHeaders {
		value := p.host
		if name != "host" {
			value = extra[name]
		}
		hdrLines = append(hdrLines, name+":"+strings.TrimSpace(value)+"\n")
	}
	canonicalHeaders := strings.Join(hdrLines, "")

	canonicalPath := p.pathPrefix + "/" + encodePath(key)
	canonicalRequest := strings.Join([]string{
		method,
		canonicalPath,
		canonicalQuery,
		canonicalHeaders,
		strings.Join(signedHeaders, ";"),
		payloadHash,
	}, "\n")

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(sha256Sum([]byte(canonicalRequest))),
	}, "\n")

	signingKey := deriveSigningKey(p.cfg.SecretKey, dateStamp, p.cfg.Region)
	signature := hex.EncodeToString(hmacSum(signingKey, stringToSign))

	// p.base SUDAH memuat prefix bucket di mode path-style, dan canonicalPath
	// memuatnya lagi untuk keperluan signing. Jadi URL-nya dibangun dari base +
	// key saja — memakai canonicalPath di sini menghasilkan `/bucket/bucket/key`.
	return p.base + "/" + encodePath(key) + "?" + canonicalQuery + "&X-Amz-Signature=" + signature, nil
}

// canonicalQueryString meng-encode dan mengurutkan parameter.
//
// Pengurutan dilakukan SETELAH encoding, karena itulah yang dibandingkan
// server: dua nama yang berbeda setelah encoding harus diurutkan menurut hasil
// encoding-nya, bukan menurut nama aslinya.
func canonicalQueryString(query map[string]string) string {
	encoded := make([]string, 0, len(query))
	for name, value := range query {
		encoded = append(encoded, encodeQuery(name)+"="+encodeQuery(value))
	}
	sort.Strings(encoded)
	return strings.Join(encoded, "&")
}

// deriveSigningKey menjalankan rantai HMAC SigV4 untuk service `s3`.
//
// Service-nya dipaku di sini, bukan jadi parameter: satu nilai yang salah di
// rantai ini menghasilkan signature yang ditolak server, dan tidak ada alasan
// AgentDeck menandatangani untuk service AWS lain.
func deriveSigningKey(secret, dateStamp, region string) []byte {
	return deriveSigningKeyFor(secret, dateStamp, region, "s3")
}

// deriveSigningKeyFor adalah rantai generiknya. Dipisah supaya tes bisa menguji
// service lain terhadap vektor resmi AWS — itu satu-satunya cara membuktikan
// `s3` benar-benar yang dipakai, bukan hanya "hasilnya berbeda dari iam".
func deriveSigningKeyFor(secret, dateStamp, region, service string) []byte {
	kDate := hmacSum([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSum(kDate, region)
	kService := hmacSum(kRegion, service)
	return hmacSum(kService, "aws4_request")
}

func hmacSum(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func sha256Sum(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}
