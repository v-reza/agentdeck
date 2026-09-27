package config

// Konfigurasi object storage untuk artifact (ARCHITECTURE 12.3, 6.2.16).
//
// Ditaruh di file sendiri, bukan ditambahkan ke Config.Load: Config.Load
// mengembalikan error untuk nilai yang wajib, dan storage TIDAK wajib —
// deployment yang tidak memakai artifact harus tetap boot. Bentuknya fungsi
// terpisah supaya perbedaan itu terlihat di signature-nya, bukan di komentar.

import (
	"strings"
)

// StorageConfig adalah kredensial S3-compatible (Cloudflare R2 di produksi).
type StorageConfig struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
}

// StorageConfigFromEnv membacanya dari environment.
//
// Nama env-nya S3 standar karena R2 memang S3-compatible; operator yang pernah
// mengisi kredensial R2 akan mengenalinya. `S3_ACCESS_KEY_ID` dan
// `S3_SECRET_ACCESS_KEY` adalah nama yang dipakai AWS SDK, jadi tidak ada
// kejutan bagi yang sudah pernah memakainya.
func StorageConfigFromEnv(getenv func(string) string) StorageConfig {
	region := strings.TrimSpace(getenv("S3_REGION"))
	if region == "" {
		// R2 memakai "auto"; MinIO mengabaikannya.
		region = "auto"
	}
	return StorageConfig{
		Endpoint:  strings.TrimRight(strings.TrimSpace(getenv("S3_ENDPOINT")), "/"),
		Region:    region,
		Bucket:    strings.TrimSpace(getenv("S3_BUCKET")),
		AccessKey: strings.TrimSpace(getenv("S3_ACCESS_KEY_ID")),
		SecretKey: getenv("S3_SECRET_ACCESS_KEY"),
	}
}

// Configured melaporkan apakah keempat nilainya terisi.
//
// Keempatnya diperiksa bersama karena setengah terisi bukan konfigurasi yang
// bisa dipakai: presigner-nya akan menolak, dan lebih baik itu ketahuan saat
// boot lewat log daripada saat worker pertama kali mengunggah.
func (c StorageConfig) Configured() bool {
	return c.Endpoint != "" && c.Bucket != "" && c.AccessKey != "" && c.SecretKey != ""
}
