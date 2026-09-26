package auth

// API keys — ARCHITECTURE 3.18, 11.1, 2281-2292; US-ADxx 6.2.3.
//
// Bentuk token: `adk_<org_prefix>_<48 byte hex>`. Yang tersimpan cuma `prefix`
// (8 karakter pertama, jadi `adk_` + 4) dan SHA-256 token penuh. Plaintext-nya
// hanya ada di response 201 dan tidak bisa diambil lagi.
//
// Kenapa SHA-256 dan bukan bcrypt, persis seperti catatan kontrak: key-nya
// 48 byte acak, jadi tidak ada ruang tebak yang perlu diperlambat. Bcrypt di
// jalur autentikasi programatik hanya menambah biaya per request tanpa
// menambah keamanan.
//
// Yang membuat ini lebih dari CRUD: `APIKeyByPrefix` membawa `role` dari
// `memberships`, jadi key mewarisi peran pemiliknya (2291). Itu yang menutup
// gap "endpoint Worker" yang selama ini terpaksa didaftarkan `auth.Member`.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"agentdeck/internal/ulid"
)

// APIKeyPrefixLen adalah panjang prefix yang disimpan, dan `api_keys_prefix_chk`
// di database menegakkannya. `adk_` + 4 karakter = 8.
const APIKeyPrefixLen = 8

// apiKeyPrefixScheme adalah awalan yang membuat token bisa dikenali sebagai
// kunci API dan bukan sesi opaque saat debugging.
const apiKeyPrefixScheme = "adk_"

// apiKeyRandomBytes = 48 byte hex. Bagian yang tidak bisa ditebak.
const apiKeyRandomBytes = 48

// maxAPIKeyNameLen menjaga label tetap terbaca di daftar key.
const maxAPIKeyNameLen = 64

// ErrAPIKeyNameInvalid untuk label kosong atau kepanjangan. Dipetakan jadi 400.
var ErrAPIKeyNameInvalid = errors.New("api key name must be 1-64 characters")

// APIKey adalah baris `api_keys` tanpa `token_hash` — hash tidak pernah keluar
// dari paket ini, jadi tipe ini aman dipakai di response HTTP.
type APIKey struct {
	ID         string
	OrgID      string
	UserID     string
	Name       string
	Prefix     string
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// Revoked melaporkan apakah key sudah dicabut. Daftar key menampilkannya, dan
// autentikasi menolaknya.
func (k APIKey) Revoked() bool { return k.RevokedAt != nil }

// APIKeyWithRole adalah baris `api_keys` beserta peran pemiliknya dari
// `memberships`. Hanya paket ini yang melihatnya — `TokenHash` sengaja ada di
// sini, bukan di `APIKey`, supaya hash tidak bisa bocor lewat response.
type APIKeyWithRole struct {
	APIKey
	TokenHash string
	Role      Role
}

// APIKeyIdentity adalah hasil autentikasi sebuah key: pemiliknya, workspace-nya,
// dan peran yang diwarisi dari keanggotaan.
type APIKeyIdentity struct {
	KeyID  string
	UserID string
	OrgID  string
	Role   Role
}

// GenerateAPIKey membuat token baru dan mengembalikan plaintext beserta prefix
// dan hash-nya. Plaintext hanya pernah ada di sini dan di response 201.
func GenerateAPIKey() (token, prefix, hash string, err error) {
	raw := make([]byte, apiKeyRandomBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", "", err
	}
	// Prefix 4 karakter dari byte acak, hex — jadi selalu [0-9a-f] dan tidak
	// perlu penyaringan karakter.
	token = apiKeyPrefixScheme + hex.EncodeToString(raw)[:4] + "_" + hex.EncodeToString(raw)
	prefix = token[:APIKeyPrefixLen]
	hash = apiKeyTokenHash(token)
	return token, prefix, hash, nil
}

// apiKeyTokenHash adalah SHA-256 heksadesimal dari token penuh. Panjangnya 64,
// yang persis `api_keys_hash_chk`.
func apiKeyTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// LooksLikeAPIKey membedakan `adk_...` dari token sesi opaque, supaya pemanggil
// bisa memilih jalur autentikasi tanpa menebak.
func LooksLikeAPIKey(token string) bool {
	return strings.HasPrefix(token, apiKeyPrefixScheme)
}

// APIKeyPrefixFromToken mengembalikan 8 karakter pertama, atau "" kalau token
// terlalu pendek untuk punya prefix. Dipakai lookup, jadi tidak boleh panic.
func APIKeyPrefixFromToken(token string) string {
	if len(token) < APIKeyPrefixLen {
		return ""
	}
	return token[:APIKeyPrefixLen]
}

// ---- Store ---------------------------------------------------------------

// CreateAPIKey membuat key untuk pemanggil di workspace ini. Plaintext hanya
// dikembalikan sekali.
func (s *Store) CreateAPIKey(ctx context.Context, orgID, userID, name string) (APIKey, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxAPIKeyNameLen {
		return APIKey{}, "", ErrAPIKeyNameInvalid
	}
	token, prefix, hash, err := GenerateAPIKey()
	if err != nil {
		return APIKey{}, "", err
	}
	key, err := s.repo.CreateAPIKey(ctx, APIKey{
		ID: ulid.Must(), OrgID: orgID, UserID: userID,
		Name: name, Prefix: prefix,
	}, hash)
	if err != nil {
		return APIKey{}, "", err
	}
	return key, token, nil
}

// APIKeys lists the caller's own keys in this workspace.
func (s *Store) APIKeys(ctx context.Context, orgID, userID string) ([]APIKey, error) {
	return s.repo.ListAPIKeys(ctx, userID, orgID)
}

// APIKey loads one key, scoped to its owner. A key belonging to somebody else
// is ErrAPIKeyNotFound, not a 403: the caller has no business knowing it exists.
func (s *Store) APIKey(ctx context.Context, orgID, userID, id string) (APIKey, error) {
	key, err := s.repo.GetAPIKey(ctx, id, userID, orgID)
	if errors.Is(err, ErrAPIKeyNotFound) {
		return APIKey{}, ErrAPIKeyNotFound
	}
	return key, err
}

// RevokeAPIKey mencabut key. `revoked` melaporkan apakah barisnya berubah —
// mencabut dua kali bukan error, tapi pemanggil tetap boleh tahu bedanya.
func (s *Store) RevokeAPIKey(ctx context.Context, orgID, userID, id string) (revoked bool, err error) {
	if _, err := s.repo.GetAPIKey(ctx, id, userID, orgID); err != nil {
		return false, err
	}
	rows, err := s.repo.RevokeAPIKey(ctx, id, userID, orgID)
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// DeleteAPIKey hapus fisik. Key yang tidak ada (atau bukan milik pemanggil)
// tetap ErrAPIKeyNotFound, jadi UI bisa membedakan "sudah hilang" dari "tidak berhak".
func (s *Store) DeleteAPIKey(ctx context.Context, orgID, userID, id string) error {
	if _, err := s.repo.GetAPIKey(ctx, id, userID, orgID); err != nil {
		return err
	}
	rows, err := s.repo.DeleteAPIKey(ctx, id, userID, orgID)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// WorkspaceByID memuat satu workspace untuk jalur autentikasi key.
//
// Keanggotaannya **tidak** diperiksa di sini: `APIKeyByPrefix` sudah
// meng-JOIN `memberships`, jadi peran yang dikembalikan hanya ada kalau
// pemilik key masih anggota. Memeriksa ulang di sini akan jadi dua sumber
// kebenaran untuk aturan yang sama.
func (s *Store) WorkspaceByID(ctx context.Context, orgID string) (Workspace, error) {
	return s.repo.GetOrgByID(ctx, orgID)
}

// AuthenticateAPIKey menukar token `adk_...` jadi identitas pemanggil.
//
// `ok=false` untuk token yang tidak ada, sudah dicabut, atau prefix-nya tidak
// dikenal — ketiganya harus tidak bisa dibedakan dari luar.
//
// `last_used_at` di-update sebagai efek samping, dan kegagalan update itu
// **tidak** menggagalkan autentikasi: itu telemetri. Menolak request yang sah
// karena kolom telemetri gagal ditulis akan membuat key tampak mati sesekali.
func (s *Store) AuthenticateAPIKey(ctx context.Context, token string) (APIKeyIdentity, bool) {
	prefix := APIKeyPrefixFromToken(token)
	if prefix == "" {
		return APIKeyIdentity{}, false
	}
	row, err := s.repo.APIKeyByPrefix(ctx, prefix)
	if err != nil {
		return APIKeyIdentity{}, false
	}
	// Bandingkan hash, bukan token: yang tersimpan memang hash-nya.
	if row.TokenHash != apiKeyTokenHash(token) {
		return APIKeyIdentity{}, false
	}
	_ = s.repo.TouchAPIKey(ctx, row.ID)
	return APIKeyIdentity{KeyID: row.ID, UserID: row.UserID, OrgID: row.OrgID, Role: row.Role}, true
}
