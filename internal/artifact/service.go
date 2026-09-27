package artifact

// Service: aturan domain artifact — ARCHITECTURE 12.3, 3.15, 6.2.16, N22.
//
// Batas ukuran ada di dua tempat dan keduanya perlu:
//
//   - DB (`artifacts_size_chk`, 1..26214400) adalah jaring terakhir. Ia menolak
//     baris yang tidak masuk akal dari jalur mana pun, termasuk yang belum ada.
//   - Service memeriksa LEBIH DULU supaya klien dapat 400 yang menjelaskan
//     dirinya, bukan 500 dari pelanggaran constraint.
import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"agentdeck/internal/storage"
	"agentdeck/internal/ulid"
)

// Batas dari 3.15 dan N22.
const (
	// MaxFileSize adalah batas per file (25 MB), sama dengan artifacts_size_chk.
	MaxFileSize = 25 * 1024 * 1024
	// MaxTaskBytes adalah kuota total per task (100 MB, N22). Ditegakkan di sini
	// karena DB menyimpan per baris dan tidak bisa menjumlahkan lintas baris
	// tanpa trigger.
	MaxTaskBytes = 100 * 1024 * 1024
	// UploadURLTTL sesuai 12.3 langkah 3: presigned PUT berlaku 15 menit.
	UploadURLTTL = 15 * time.Minute
	// DownloadURLTTL. Kontrak hanya menyebut TTL upload; unduhan dibuat pendek
	// karena URL-nya jawaban dari endpoint yang sudah terautentikasi — penerima
	// membukanya langsung, jadi tidak perlu masa berlaku panjang yang cuma
	// memperbesar jendela bagi URL yang bocor.
	DownloadURLTTL = 5 * time.Minute
	// MaxVerifyBytes adalah batas objek yang dibaca ulang untuk verifikasi
	// SHA-256 (12.3 langkah 6).
	//
	// Ini SENGAJA lebih kecil dari MaxFileSize meski keduanya seharusnya
	// merujuk hal yang sama. Verifikasi berarti mengunduh isinya ke memori
	// server: objek sebesar batas atas dikali beberapa pendaftaran paralel
	// adalah tekanan memori yang tidak dijanjikan siapa pun. Di atas ambang ini
	// `size` dan HEAD tetap diperiksa, tapi isinya tidak dibaca ulang.
	MaxVerifyBytes = 8 * 1024 * 1024
)

var (
	ErrNotFound      = errors.New("artifact: tidak ditemukan")
	ErrTaskNotFound  = errors.New("artifact: task tidak ditemukan")
	ErrRunNotFound   = errors.New("artifact: run tidak ditemukan")
	ErrTooLarge      = errors.New("artifact: ukuran melebihi batas")
	ErrTaskQuota     = errors.New("artifact: kuota task terlampaui")
	ErrSHA256Match   = errors.New("artifact: sha256 tidak cocok dengan isi objek")
	ErrObjectMissing = errors.New("artifact: objek belum ada di storage")
	ErrInvalidInput  = errors.New("artifact: input tidak valid")
	// ErrNotConfigured: storage belum dikonfigurasi. Handler memetakannya jadi
	// 503, bukan 500 — operator yang belum mengisi kredensial perlu tahu itu
	// pilihan, bukan kerusakan.
	ErrNotConfigured = errors.New("artifact: object storage belum dikonfigurasi")
)

// Artifact adalah satu baris `artifacts` (3.15).
type Artifact struct {
	ID          string
	OrgID       string
	TaskID      string
	RunID       string
	Filename    string
	ContentType string
	Size        int64
	StorageKey  string
	SHA256      string
	CreatedAt   time.Time
}

// UploadTicket adalah jawaban endpoint upload-url.
type UploadTicket struct {
	URL         string
	StorageKey  string
	ExpiresAt   time.Time
	MaxFileSize int64
	// ArtifactID ikut dikembalikan supaya key-nya sudah pasti unik sebelum
	// unggahan dimulai; kalau tidak, dua unggahan bersamaan bisa menulis ke key
	// yang sama dan yang satu menimpa yang lain.
	ArtifactID string
}

// RegisterInput adalah payload pendaftaran setelah unggah (12.3 langkah 5).
type RegisterInput struct {
	Filename    string
	ContentType string
	Size        int64
	SHA256      string
	StorageKey  string
	RunID       string
}

// Repo adalah penyimpanan metadata artifact.
type Repo interface {
	ListByTask(ctx context.Context, orgID, taskID string) ([]Artifact, error)
	Get(ctx context.Context, orgID, id string) (Artifact, error)
	Create(ctx context.Context, a Artifact) (Artifact, error)
	// SumTaskBytes dipakai untuk menegakkan kuota N22.
	SumTaskBytes(ctx context.Context, orgID, taskID string) (int64, error)
}

// Objects adalah object storage (12.3).
type Objects interface {
	PresignPut(key, contentType string, ttl time.Duration) (string, error)
	PresignGet(key string, ttl time.Duration) (string, error)
	Head(ctx context.Context, key string) (storage.ObjectInfo, error)
	// Fetch mengunduh isi objek untuk verifikasi digest.
	Fetch(ctx context.Context, key string, limit int64) ([]byte, error)
}

// TaskLookup membuktikan task DAN run-nya ada serta milik org yang sama.
//
// Run diperiksa di sini, bukan diserahkan ke foreign key: pelanggaran FK
// muncul sebagai error driver yang harus dipetakan ke status HTTP, dan
// memetakannya berarti menebak-nebak kode error Postgres. Satu SELECT EXISTS
// memberi jawaban yang bisa dipakai langsung.
type TaskLookup interface {
	TaskExists(ctx context.Context, orgID, taskID string) (bool, error)
	RunExists(ctx context.Context, orgID, runID string) (bool, error)
}

// Service merakit aturannya.
type Service struct {
	repo  Repo
	tasks TaskLookup
	store Objects
	ids   func() string
	now   func() time.Time
}

// Options untuk New.
type Options struct {
	Repo  Repo
	Tasks TaskLookup
	Store Objects
	// IDs membuat ULID. Bisa dikosongkan di tes.
	IDs func() string
	Now func() time.Time
}

// New merakit Service.
func New(opts Options) *Service {
	s := &Service{repo: opts.Repo, tasks: opts.Tasks, store: opts.Store, ids: opts.IDs, now: opts.Now}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// Configured melaporkan apakah storage siap. Handler memakai ini untuk
// membalas 503 alih-alih gagal dengan pesan internal.
func (s *Service) Configured() bool { return s.store != nil }

// List mengembalikan metadata artifact satu task.
func (s *Service) List(ctx context.Context, orgID, taskID string) ([]Artifact, error) {
	if err := s.requireTask(ctx, orgID, taskID); err != nil {
		return nil, err
	}
	return s.repo.ListByTask(ctx, orgID, taskID)
}

// Get mengembalikan satu artifact, ter-scope org.
func (s *Service) Get(ctx context.Context, orgID, id string) (Artifact, error) {
	a, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Artifact{}, ErrNotFound
		}
		return Artifact{}, err
	}
	return a, nil
}

// DownloadURL mengembalikan presigned GET (6.2.16).
func (s *Service) DownloadURL(ctx context.Context, orgID, id string) (string, time.Time, error) {
	if !s.Configured() {
		return "", time.Time{}, ErrNotConfigured
	}
	a, err := s.Get(ctx, orgID, id)
	if err != nil {
		return "", time.Time{}, err
	}
	url, err := s.store.PresignGet(a.StorageKey, DownloadURLTTL)
	if err != nil {
		return "", time.Time{}, err
	}
	return url, s.now().Add(DownloadURLTTL), nil
}

// UploadURL membuat presigned PUT (12.3 langkah 3).
//
// Kuota diperiksa di sini, bukan hanya saat pendaftaran: kalau tidak, executor
// bisa mengunggah 25 MB berkali-kali lalu ditolak di akhir, dan bandwidth yang
// sudah terpakai tidak bisa dikembalikan.
func (s *Service) UploadURL(ctx context.Context, orgID, taskID, runID, filename, contentType string, size int64) (UploadTicket, error) {
	if !s.Configured() {
		return UploadTicket{}, ErrNotConfigured
	}
	if err := s.requireTask(ctx, orgID, taskID); err != nil {
		return UploadTicket{}, err
	}
	// run_id WAJIB, dan alasannya ada di 3.15: key-nya berbentuk
	// `artifacts/{org}/{task}/{run}/{id}-{filename}`. Tanpa run, key-nya tidak
	// bisa dibentuk sesuai kontrak — dan mengarang segmen pengganti berarti
	// `artifacts.run_id` menunjuk run yang tidak ada, yang ditolak
	// `artifacts_run_fk`. 6.2.16 meringkas request-nya sebagai `(filename,
	// size)`; ringkasan itu tidak bisa dipenuhi bersamaan dengan DDL-nya, dan
	// DDL yang mengikat.
	if err := s.requireRun(ctx, orgID, runID); err != nil {
		return UploadTicket{}, err
	}
	if size < 1 || size > MaxFileSize {
		return UploadTicket{}, fmt.Errorf("%w: %d byte (batas %d)", ErrTooLarge, size, MaxFileSize)
	}
	used, err := s.repo.SumTaskBytes(ctx, orgID, taskID)
	if err != nil {
		return UploadTicket{}, err
	}
	if used+size > MaxTaskBytes {
		return UploadTicket{}, fmt.Errorf("%w: sudah %d byte, ditambah %d melewati %d",
			ErrTaskQuota, used, size, MaxTaskBytes)
	}
	if filename == "" {
		return UploadTicket{}, fmt.Errorf("%w: filename wajib", ErrInvalidInput)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	artifactID := s.newID()
	key := storage.ArtifactKey(orgID, taskID, runID, artifactID, filename)

	url, err := s.store.PresignPut(key, contentType, UploadURLTTL)
	if err != nil {
		return UploadTicket{}, err
	}
	return UploadTicket{
		URL:         url,
		StorageKey:  key,
		ExpiresAt:   s.now().Add(UploadURLTTL),
		MaxFileSize: MaxFileSize,
		ArtifactID:  artifactID,
	}, nil
}

// Register mencatat artifact setelah unggahan selesai (12.3 langkah 5-6).
//
// Inilah tempat `12.3` langkah 6 dijalankan: "backend memverifikasi SHA-256
// matching". Verifikasi yang sebenarnya berarti objeknya dibaca dan digest-nya
// dihitung ulang — kalau hanya membandingkan string yang dikirim klien dengan
// string yang dikirim klien, tidak ada yang diverifikasi.
func (s *Service) Register(ctx context.Context, orgID, taskID string, in RegisterInput) (Artifact, error) {
	if !s.Configured() {
		return Artifact{}, ErrNotConfigured
	}
	if err := s.requireTask(ctx, orgID, taskID); err != nil {
		return Artifact{}, err
	}
	in.SHA256 = strings.ToLower(strings.TrimSpace(in.SHA256))
	if len(in.SHA256) != 64 || !isHex(in.SHA256) {
		return Artifact{}, fmt.Errorf("%w: sha256 harus hex 64 karakter", ErrInvalidInput)
	}
	if in.Size < 1 || in.Size > MaxFileSize {
		return Artifact{}, fmt.Errorf("%w: %d byte (batas %d)", ErrTooLarge, in.Size, MaxFileSize)
	}
	if in.StorageKey == "" || in.Filename == "" {
		return Artifact{}, fmt.Errorf("%w: filename dan storage_key wajib", ErrInvalidInput)
	}
	// Key harus berada di bawah prefix ORG DAN TASK ini.
	//
	// Keduanya, bukan hanya org: key-nya berbentuk
	// `artifacts/{org}/{task}/{run}/{id}-{filename}` (3.15), jadi memeriksa
	// prefix org saja masih membolehkan artifact task lain didaftarkan ke task
	// ini — baris yang menunjuk pekerjaan orang lain. Tanpa pemeriksaan ini
	// sama sekali, seorang worker bisa mendaftarkan objek milik org lain hanya
	// dengan menyebut key-nya, dan `artifacts` jadi jembatan lintas tenant.
	wantPrefix := "artifacts/" + orgID + "/" + taskID + "/"
	if !strings.HasPrefix(in.StorageKey, wantPrefix) {
		return Artifact{}, fmt.Errorf("%w: storage_key harus berawalan %q", ErrInvalidInput, wantPrefix)
	}

	used, err := s.repo.SumTaskBytes(ctx, orgID, taskID)
	if err != nil {
		return Artifact{}, err
	}
	if used+in.Size > MaxTaskBytes {
		return Artifact{}, fmt.Errorf("%w: sudah %d byte, ditambah %d melewati %d",
			ErrTaskQuota, used, in.Size, MaxTaskBytes)
	}

	// Objeknya harus benar-benar ada. Tanpa ini, baris artifact bisa menunjuk
	// key yang tidak pernah diunggah dan download-nya gagal belakangan.
	info, err := s.store.Head(ctx, in.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return Artifact{}, ErrObjectMissing
		}
		return Artifact{}, err
	}
	if info.Size != in.Size {
		return Artifact{}, fmt.Errorf("%w: storage melaporkan %d byte, didaftarkan %d",
			ErrInvalidInput, info.Size, in.Size)
	}
	if in.Size <= MaxVerifyBytes {
		body, err := s.store.Fetch(ctx, in.StorageKey, MaxVerifyBytes)
		if err != nil {
			return Artifact{}, err
		}
		if got := storage.SHA256Hex(body); got != in.SHA256 {
			return Artifact{}, fmt.Errorf("%w: hitung ulang %s, didaftarkan %s",
				ErrSHA256Match, got, in.SHA256)
		}
	}

	contentType := in.ContentType
	if contentType == "" {
		contentType = info.ContentType
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := s.requireRun(ctx, orgID, in.RunID); err != nil {
		return Artifact{}, err
	}
	// Key yang didaftarkan harus milik run ini juga. Tanpa pemeriksaan ini,
	// artifact bisa tercatat di run 1 sementara objeknya tersimpan di prefix
	// run 2 — baris yang benar dengan isi yang salah.
	if got := runIDFromKey(in.StorageKey); got != in.RunID {
		return Artifact{}, fmt.Errorf("%w: storage_key memuat run %q, bukan %q",
			ErrInvalidInput, got, in.RunID)
	}

	return s.repo.Create(ctx, Artifact{
		ID:          s.newID(),
		OrgID:       orgID,
		TaskID:      taskID,
		RunID:       in.RunID,
		Filename:    in.Filename,
		ContentType: contentType,
		Size:        in.Size,
		StorageKey:  in.StorageKey,
		SHA256:      in.SHA256,
		CreatedAt:   s.now(),
	})
}

func (s *Service) requireTask(ctx context.Context, orgID, taskID string) error {
	if taskID == "" {
		return fmt.Errorf("%w: task id wajib", ErrInvalidInput)
	}
	ok, err := s.tasks.TaskExists(ctx, orgID, taskID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTaskNotFound
	}
	return nil
}

func (s *Service) requireRun(ctx context.Context, orgID, runID string) error {
	if runID == "" {
		return fmt.Errorf("%w: run_id wajib", ErrInvalidInput)
	}
	ok, err := s.tasks.RunExists(ctx, orgID, runID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRunNotFound
	}
	return nil
}

func (s *Service) newID() string {
	if s.ids != nil {
		return s.ids()
	}
	return ulid.Must()
}

// runIDFromKey membaca segmen keempat key 3.15:
// `artifacts/{org}/{task}/{run}/{id}-{filename}`.
func runIDFromKey(key string) string {
	parts := strings.Split(key, "/")
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}
