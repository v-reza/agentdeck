package main

// Wiring object storage untuk artifact (6.2.16, 12.3).
//
// Terpisah dari webhook_wiring.go karena kegagalannya berbeda arti: kunci
// master yang hilang mematikan endpoint kredensial, sementara storage yang
// belum dikonfigurasi hanya mematikan artifact. Keduanya dipilih supaya TIDAK
// menghentikan boot — deployment yang tidak memakai fitur itu tidak boleh
// gagal start karenanya.
//
// Nama env-nya mengikuti pola S3 standar (`S3_ENDPOINT`, `S3_*`) karena R2
// memang S3-compatible dan itu yang diharapkan operator yang pernah mengisi
// kredensial R2 sebelumnya.

import (
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/artifact"
	"agentdeck/internal/config"
	"agentdeck/internal/storage"
)

// artifactService merakit service artifact. Mengembalikan nil kalau storage
// belum dikonfigurasi — handler-nya membalas 503, bukan 500, karena itu pilihan
// operator dan bukan kerusakan.
func artifactService(pool *pgxpool.Pool, logger *slog.Logger) *artifact.Service {
	s3 := config.StorageConfigFromEnv(os.Getenv)
	if !s3.Configured() {
		logger.Info("artifact: storage OFF (set S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY_ID, S3_SECRET_ACCESS_KEY untuk menyalakan)")
		return nil
	}
	client, err := storage.NewClient(storage.Config{
		Endpoint:  s3.Endpoint,
		Region:    s3.Region,
		Bucket:    s3.Bucket,
		AccessKey: s3.AccessKey,
		SecretKey: s3.SecretKey,
		// R2 dan MinIO sama-sama menerima path-style, dan itu satu-satunya
		// bentuk yang bekerja untuk endpoint non-DNS seperti `minio:9000`.
		PathStyle: true,
	})
	if err != nil {
		// Konfigurasi setengah jadi adalah kesalahan operator yang harus
		// terlihat, tapi tidak boleh menghentikan API: fitur lain tetap jalan.
		logger.Error("artifact: konfigurasi storage tidak valid, endpoint artifact dimatikan", "err", err)
		return nil
	}
	logger.Info("artifact: storage ON", "endpoint", s3.Endpoint, "bucket", s3.Bucket)
	return artifact.New(artifact.Options{
		Repo:  artifact.NewPgRepo(pool),
		Tasks: artifact.NewPgTaskLookup(pool),
		Store: client,
	})
}
