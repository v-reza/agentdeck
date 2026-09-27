package webhook

// Service: aturan domain CRUD webhook — ARCHITECTURE 6.2.18, 3.22, 16.
//
// Handler tetap tipis: parse, panggil satu method, petakan error ke status.
// Semua keputusan (validasi URL, batas panjang, arti daftar event kosong) ada
// di sini supaya bisa diuji tanpa HTTP.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"agentdeck/internal/ulid"
)

// Sentinel error service. Handler memetakannya ke status HTTP.
var (
	ErrNotFound     = errors.New("webhook: tidak ditemukan")
	ErrInvalidInput = errors.New("webhook: input tidak valid")
)

// MaxEvents menutup daftar kind yang boleh dipantau.
//
// DECISIONS §4 mencantumkan 18 kind, jadi 64 adalah ruang yang lega. Batas ini
// ada supaya `events_json` tidak bisa tumbuh jadi dokumen besar yang dikirim
// berulang setiap kali webhook dibaca.
const MaxEvents = 64

// MaxSecretLen menutup panjang secret HMAC. 13.2 tidak menyebut batas; 512
// byte jauh di atas apa pun yang wajar untuk HMAC dan menahan baris yang
// sengaja dibesarkan.
const MaxSecretLen = 512

// Repo adalah sisi penyimpanan.
type Repo interface {
	Create(ctx context.Context, w Webhook) (Webhook, error)
	ListByBoard(ctx context.Context, orgID, boardID string) ([]Webhook, error)
	Get(ctx context.Context, orgID, id string) (Webhook, error)
	Update(ctx context.Context, orgID, id, url string, active bool) (Webhook, error)
	Delete(ctx context.Context, orgID, id string) error
	ListDeliveries(ctx context.Context, webhookID string, limit int) ([]Delivery, error)
	GetDelivery(ctx context.Context, webhookID string, id int64) (Delivery, error)
	ResetDelivery(ctx context.Context, id int64) error
}

// Webhook adalah satu langganan (3.22). Secret-nya terenkripsi; bentuk
// terbukanya hanya ada sesaat di dalam Seal/Open.
type Webhook struct {
	ID        string
	OrgID     string
	BoardID   string
	URL       string
	SecretEnc []byte
	Events    []string
	Active    bool
	CreatedAt time.Time
}

// Service merakit aturan domain di atas Repo.
type Service struct {
	repo Repo
	// seal mengenkripsi secret (16: AES-256-GCM dengan AGENTDECK_MASTER_KEY).
	// Sebuah fungsi, bukan import internal/crypto langsung, supaya service ini
	// bisa diuji dengan kunci nyata tanpa menyentuh environment — pola yang
	// sama dengan providerreg.ServiceOptions.Decrypt.
	seal func(plaintext string) ([]byte, error)
	// allowLocal menyalakan pengecualian loopback §16. Default true: server
	// inference operator memang sering di mesin sendiri. Operator yang tidak
	// memakainya bisa mematikannya.
	allowLocal bool
}

// NewService merakit service. `seal` boleh nil — dan kalau nil, setiap operasi
// yang butuh secret menjawab error, bukan menyimpan secret dalam bentuk terbuka.
// Itu pilihan yang disengaja: deployment tanpa AGENTDECK_MASTER_KEY tetap
// melayani baca/tulis metadata, tapi tidak pernah menuliskan kredensial.
func NewService(repo Repo, seal func(string) ([]byte, error)) *Service {
	return &Service{repo: repo, seal: seal, allowLocal: true}
}

// WithLocalURLs mematikan atau menyalakan pengecualian loopback §16.
func (s *Service) WithLocalURLs(allow bool) *Service {
	s.allowLocal = allow
	return s
}

// CreateInput adalah payload pendaftaran webhook (6.2.18).
type CreateInput struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events_json"`
	Active *bool    `json:"active"`
}

// Create mendaftarkan satu webhook.
func (s *Service) Create(ctx context.Context, orgID, boardID string, in CreateInput) (Webhook, error) {
	if s.seal == nil {
		return Webhook{}, fmt.Errorf("%w: master key tidak tersedia", ErrInvalidInput)
	}
	if err := ValidateURL(in.URL, s.allowLocal); err != nil {
		return Webhook{}, fmt.Errorf("%w: %s", ErrInvalidInput, err)
	}
	if in.Secret == "" {
		return Webhook{}, fmt.Errorf("%w: secret wajib diisi", ErrInvalidInput)
	}
	if len(in.Secret) > MaxSecretLen {
		return Webhook{}, fmt.Errorf("%w: secret melebihi %d byte", ErrInvalidInput, MaxSecretLen)
	}
	events, err := normalizeEvents(in.Events)
	if err != nil {
		return Webhook{}, err
	}

	sealed, err := s.seal(in.Secret)
	if err != nil {
		return Webhook{}, fmt.Errorf("webhook: mengenkripsi secret: %w", err)
	}

	active := true
	if in.Active != nil {
		active = *in.Active
	}
	return s.repo.Create(ctx, Webhook{
		ID:        ulid.Must(),
		OrgID:     orgID,
		BoardID:   boardID,
		URL:       in.URL,
		SecretEnc: sealed,
		Events:    events,
		Active:    active,
	})
}

// Get membaca satu webhook di dalam workspace.
func (s *Service) Get(ctx context.Context, orgID, id string) (Webhook, error) {
	return s.repo.Get(ctx, orgID, id)
}

// ListByBoard mendaftar webhook sebuah board.
func (s *Service) ListByBoard(ctx context.Context, orgID, boardID string) ([]Webhook, error) {
	return s.repo.ListByBoard(ctx, orgID, boardID)
}

// Delete menghapus satu webhook.
func (s *Service) Delete(ctx context.Context, orgID, id string) error {
	return s.repo.Delete(ctx, orgID, id)
}

// ListDeliveries membaca riwayat pengiriman satu webhook.
func (s *Service) ListDeliveries(ctx context.Context, webhookID string, limit int) ([]Delivery, error) {
	return s.repo.ListDeliveries(ctx, webhookID, limit)
}

// RetryDelivery mengembalikan satu pengiriman ke antrean (13.3).
//
// `attempts` di-reset ke nol, bukan dibiarkan: delivery yang sudah `dead` punya
// attempts = 6, dan tanpa reset percobaan berikutnya langsung dinilai habis
// jatah dan kembali `dead` tanpa pernah dikirim. Retry manual yang tidak
// mengirim apa pun lebih buruk daripada tidak ada tombol retry.
//
// Kepemilikannya diperiksa di sini: `webhookID` harus webhook yang memiliki
// delivery itu, supaya id delivery yang benar tidak bisa dipakai lewat webhook
// lain di workspace yang sama.
func (s *Service) RetryDelivery(ctx context.Context, webhookID string, deliveryID int64) (Delivery, error) {
	d, err := s.repo.GetDelivery(ctx, webhookID, deliveryID)
	if err != nil {
		return Delivery{}, err
	}
	if err := s.repo.ResetDelivery(ctx, deliveryID); err != nil {
		return Delivery{}, err
	}
	d.Status = StatusPending
	d.Attempts = 0
	d.ResponseCode = nil
	d.LastError = ""
	return d, nil
}

// UpdateInput adalah payload PATCH (6.2.18: aktifkan/nonaktifkan atau ubah URL).
//
// URL dan Active pointer supaya "tidak dikirim" bisa dibedakan dari "dikirim
// sebagai nilai nol" — tanpa itu, PATCH yang hanya menonaktifkan akan
// menuliskan URL kosong.
type UpdateInput struct {
	URL    *string `json:"url"`
	Active *bool   `json:"active"`
}

// Update menerapkan perubahan.
func (s *Service) Update(ctx context.Context, orgID, id string, in UpdateInput) (Webhook, error) {
	current, err := s.repo.Get(ctx, orgID, id)
	if err != nil {
		return Webhook{}, err
	}
	url := current.URL
	if in.URL != nil {
		if err := ValidateURL(*in.URL, s.allowLocal); err != nil {
			return Webhook{}, fmt.Errorf("%w: %s", ErrInvalidInput, err)
		}
		url = *in.URL
	}
	active := current.Active
	if in.Active != nil {
		active = *in.Active
	}
	return s.repo.Update(ctx, orgID, id, url, active)
}

// normalizeEvents membersihkan daftar kind yang dipantau.
//
// Daftar kosong berarti "semua event" (lihat ListMatchingWebhooks) — jadi
// kosong itu sah, bukan error. Yang ditolak adalah daftar yang terlalu panjang
// atau memuat kind kosong, karena keduanya menghasilkan filter yang tidak
// pernah cocok dan webhook yang diam tanpa penjelasan.
func normalizeEvents(events []string) ([]string, error) {
	if len(events) > MaxEvents {
		return nil, fmt.Errorf("%w: maksimal %d event", ErrInvalidInput, MaxEvents)
	}
	out := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, e := range events {
		e = strings.TrimSpace(e)
		if e == "" {
			return nil, fmt.Errorf("%w: nama event kosong", ErrInvalidInput)
		}
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	// Urutan tetap supaya payload dan baris DB deterministik; tanpa ini,
	// daftar yang sama bisa tersimpan dalam urutan berbeda dan diff-nya
	// berisik tanpa alasan.
	sort.Strings(out)
	return out, nil
}

// EventsJSON mengubah daftar menjadi bentuk yang disimpan kolom JSONB.
func EventsJSON(events []string) []byte {
	if events == nil {
		events = []string{}
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		// Tidak mungkin untuk []string; jangan sampai panic di jalur request.
		return []byte("[]")
	}
	return encoded
}
