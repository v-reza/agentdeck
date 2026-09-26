package auth

// API keys terhadap Postgres nyata — ARCHITECTURE 3.18, 6.2.3.
//
// Empat hal di sini cuma bisa dibuktikan lawan database sungguhan:
//
//  1. `api_keys_prefix_uniq` benar-benar unik HANYA antar-key aktif (partial
//     index). Key yang dicabut boleh berbagi prefix — dan itu memang yang
//     diinginkan: mencabut lalu membuat ulang tidak boleh gagal.
//  2. Constraint `char_length` (26/8/64) dan `btrim(name) <> ''` ditegakkan
//     database, bukan cuma divalidasi Go.
//  3. `APIKeyByPrefix` mengembalikan peran dari `memberships`, dan **tidak
//     mengembalikan baris** untuk key yang dicabut — jadi key mati tidak bisa
//     dibedakan dari key yang tidak pernah ada.
//  4. `last_used_at` benar-benar terisi saat key dipakai.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"agentdeck/internal/ulid"
)

// apiKeyFixture mendaftarkan satu pemilik dan mengembalikan store, user, dan
// workspace pribadinya.
func apiKeyFixture(t *testing.T) (*Store, User, Workspace) {
	t.Helper()
	store := pgTestStore(t)
	user, workspace, _, err := store.Register(context.Background(),
		uniqueEmail(t, "keyowner@example.com"), "password123", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return store, user, workspace
}

// TestPgAPIKeyRoundTripAndTokenShape: token-nya `adk_` + prefix 8 karakter, dan
// hash-nya yang tersimpan — bukan tokennya.
func TestPgAPIKeyRoundTripAndTokenShape(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()

	key, token, err := store.CreateAPIKey(ctx, workspace.ID, user.ID, "ci")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if !strings.HasPrefix(token, "adk_") {
		t.Fatalf("token = %q, want adk_ prefix", token)
	}
	if key.Prefix != token[:APIKeyPrefixLen] {
		t.Fatalf("prefix = %q, want %q", key.Prefix, token[:APIKeyPrefixLen])
	}
	if len(key.Prefix) != 8 {
		t.Fatalf("prefix len = %d, want 8", len(key.Prefix))
	}
	// Plaintext tidak boleh bisa diambil lagi dari store.
	loaded, err := store.APIKey(ctx, workspace.ID, user.ID, key.ID)
	if err != nil {
		t.Fatalf("APIKey: %v", err)
	}
	if loaded.Prefix != key.Prefix || loaded.Name != "ci" {
		t.Fatalf("key yang dimuat berbeda: %+v", loaded)
	}

	// Yang tersimpan adalah SHA-256 token, bukan tokennya.
	var stored string
	if err := pgPool(t).QueryRow(ctx,
		"select token_hash from api_keys where id = $1", key.ID).Scan(&stored); err != nil {
		t.Fatalf("baca token_hash: %v", err)
	}
	if stored == token {
		t.Fatal("token tersimpan plaintext di database")
	}
	if stored != apiKeyTokenHash(token) {
		t.Fatalf("token_hash = %q, want SHA-256 token", stored)
	}
	if len(stored) != 64 {
		t.Fatalf("token_hash len = %d, want 64", len(stored))
	}
}

// TestPgAPIKeyAuthenticatesAndInheritsTheRole is the point of the module: the
// key resolves to its owner, its workspace, and the role from memberships.
func TestPgAPIKeyAuthenticatesAndInheritsTheRole(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()

	_, token, err := store.CreateAPIKey(ctx, workspace.ID, user.ID, "sdk")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	identity, ok := store.AuthenticateAPIKey(ctx, token)
	if !ok {
		t.Fatal("key yang sah ditolak")
	}
	if identity.UserID != user.ID || identity.OrgID != workspace.ID {
		t.Fatalf("identitas = %+v, want user %s di org %s", identity, user.ID, workspace.ID)
	}
	// Pendaftar adalah owner workspace pribadinya.
	if identity.Role != Owner {
		t.Fatalf("role = %q, want %q (diwarisi dari memberships)", identity.Role, Owner)
	}
}

// TestPgAPIKeyAuthenticateRejectsWhatItShould: token asing, token yang bukan
// `adk_`, dan token yang hash-nya tidak cocok.
func TestPgAPIKeyAuthenticateRejectsWhatItShould(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()

	_, token, err := store.CreateAPIKey(ctx, workspace.ID, user.ID, "sdk")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// Token dengan prefix yang benar tapi isi berbeda: hash tidak cocok.
	foreign := token[:APIKeyPrefixLen] + strings.Repeat("f", len(token)-APIKeyPrefixLen)
	for name, candidate := range map[string]string{
		"token asing":           foreign,
		"bukan adk_":            "bukan-token-sama-sekali",
		"prefix tak ada":        "adk_zzzz_" + strings.Repeat("a", 96),
		"kosong":                "",
		"prefix terlalu pendek": "adk_ab",
	} {
		if _, ok := store.AuthenticateAPIKey(ctx, candidate); ok {
			t.Errorf("%s diterima, want ditolak", name)
		}
	}
}

// TestPgRevokedAPIKeyStopsWorkingImmediately is 2292: revoke is immediate, and
// a revoked key is indistinguishable from one that never existed.
func TestPgRevokedAPIKeyStopsWorkingImmediately(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()

	key, token, err := store.CreateAPIKey(ctx, workspace.ID, user.ID, "deploy")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if _, ok := store.AuthenticateAPIKey(ctx, token); !ok {
		t.Fatal("key aktif ditolak")
	}

	revoked, err := store.RevokeAPIKey(ctx, workspace.ID, user.ID, key.ID)
	if err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if !revoked {
		t.Fatal("revoke pertama melaporkan nol baris")
	}
	if _, ok := store.AuthenticateAPIKey(ctx, token); ok {
		t.Fatal("key yang dicabut masih bisa dipakai")
	}

	// Idempoten: kedua kali melaporkan "sudah begitu", bukan error.
	again, err := store.RevokeAPIKey(ctx, workspace.ID, user.ID, key.ID)
	if err != nil {
		t.Fatalf("revoke kedua: %v", err)
	}
	if again {
		t.Fatal("revoke kedua melaporkan perubahan")
	}
}

// TestPgAPIKeyPrefixIsUniqueOnlyAmongActiveKeys pins the partial index. This is
// the behaviour that decides whether "revoke then re-mint" works, and it cannot
// be observed from Go — the uniqueness is the database's.
func TestPgAPIKeyPrefixIsUniqueOnlyAmongActiveKeys(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()

	// Prefix unik per-run: tes lain memakai uniqueEmail untuk isolasi, dan
	// prefix di sini harus sama-sama unik — kalau tidak, baris aktif dari run
	// sebelumnya menabrak run berikutnya dan tesnya gagal karena sisa data,
	// bukan karena index-nya salah.
	// Empat karakter TERAKHIR, bukan pertama: ULID diawali timestamp, jadi
	// empat karakter pertamanya hampir selalu identik untuk dua ULID yang
	// dibuat dalam milidetik yang sama — dan prefix-nya jadi bertabrakan.
	u := ulid.Must()
	shared := "adk_" + strings.ToLower(u[len(u)-4:])
	if len(shared) != APIKeyPrefixLen {
		t.Fatalf("prefix uji = %q, want %d char", shared, APIKeyPrefixLen)
	}
	first, err := store.repo.CreateAPIKey(ctx, APIKey{
		ID: ulid.Must(), OrgID: workspace.ID, UserID: user.ID, Name: "first", Prefix: shared,
	}, apiKeyTokenHash(shared+"_"+strings.Repeat("1", 96)))
	if err != nil {
		t.Fatalf("key aktif pertama: %v", err)
	}
	if _, err := store.RevokeAPIKey(ctx, workspace.ID, user.ID, first.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	second, err := store.repo.CreateAPIKey(ctx, APIKey{
		ID: ulid.Must(), OrgID: workspace.ID, UserID: user.ID, Name: "second", Prefix: shared,
	}, apiKeyTokenHash(shared+"_"+strings.Repeat("2", 96)))
	if err != nil {
		t.Fatalf("prefix sama setelah revoke ditolak: %v", err)
	}

	// Tapi dua key AKTIF dengan prefix sama harus ditolak.
	third, err := store.repo.CreateAPIKey(ctx, APIKey{
		ID: ulid.Must(), OrgID: workspace.ID, UserID: user.ID, Name: "third", Prefix: shared,
	}, apiKeyTokenHash(shared+"_"+strings.Repeat("3", 96)))
	if err == nil {
		// Bisa jadi ULID yang sama... tidak. Ini harus gagal.
		t.Fatalf("prefix sama antar-key aktif diterima (id %s)", third.ID)
	}
	_ = second
}

// TestPgAPIKeyConstraintsAreReal: panjang prefix/hash dan nama kosong dijaga
// database, bukan cuma Go. Kalau Go yang salah, database tetap menolak.
func TestPgAPIKeyConstraintsAreReal(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()

	cases := map[string]APIKey{
		"prefix pendek": {ID: ulid.Must(), OrgID: workspace.ID, UserID: user.ID, Name: "x", Prefix: "adk_ab"},
		"nama kosong":   {ID: ulid.Must(), OrgID: workspace.ID, UserID: user.ID, Name: "  ", Prefix: "adk_abcd"},
	}
	for name, key := range cases {
		if _, err := store.repo.CreateAPIKey(ctx, key, strings.Repeat("a", 64)); err == nil {
			t.Errorf("%s diterima database, want ditolak", name)
		}
	}
	// Hash dengan panjang salah juga ditolak.
	if _, err := store.repo.CreateAPIKey(ctx, APIKey{
		ID: ulid.Must(), OrgID: workspace.ID, UserID: user.ID, Name: "x", Prefix: "adk_abcd",
	}, "terlalu-pendek"); err == nil {
		t.Error("token_hash pendek diterima database, want ditolak")
	}
}

// TestPgAPIKeyTouchRecordsLastUse: `last_used_at` adalah telemetri yang
// ditampilkan di daftar key, jadi harus benar-benar terisi.
func TestPgAPIKeyTouchRecordsLastUse(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()

	key, token, err := store.CreateAPIKey(ctx, workspace.ID, user.ID, "touch")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if key.LastUsedAt != nil {
		t.Fatalf("key baru sudah punya last_used_at: %v", key.LastUsedAt)
	}
	if _, ok := store.AuthenticateAPIKey(ctx, token); !ok {
		t.Fatal("autentikasi gagal")
	}

	loaded, err := store.APIKey(ctx, workspace.ID, user.ID, key.ID)
	if err != nil {
		t.Fatalf("APIKey: %v", err)
	}
	if loaded.LastUsedAt == nil {
		t.Fatal("last_used_at masih kosong setelah key dipakai")
	}
	if time.Since(*loaded.LastUsedAt) > time.Minute {
		t.Fatalf("last_used_at = %v, want sekitar sekarang", loaded.LastUsedAt)
	}
}

// TestPgAPIKeyIsScopedToItsOwner: key anggota lain 404, bukan 403.
func TestPgAPIKeyIsScopedToItsOwner(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()
	other, _, _, err := store.Register(ctx, uniqueEmail(t, "keyother@example.com"),
		"password123", "", "", SessionMeta{})
	if err != nil {
		t.Fatalf("register kedua: %v", err)
	}

	key, _, err := store.CreateAPIKey(ctx, workspace.ID, user.ID, "mine")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	// Orang lain di workspace yang sama: tidak melihat, tidak bisa menghapus.
	if _, err := store.APIKey(ctx, workspace.ID, other.ID, key.ID); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("APIKey oleh orang lain = %v, want ErrAPIKeyNotFound", err)
	}
	if err := store.DeleteAPIKey(ctx, workspace.ID, other.ID, key.ID); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("DeleteAPIKey oleh orang lain = %v, want ErrAPIKeyNotFound", err)
	}
	// Pemiliknya masih melihat.
	if _, err := store.APIKey(ctx, workspace.ID, user.ID, key.ID); err != nil {
		t.Fatalf("APIKey oleh pemilik: %v", err)
	}
	// Daftarnya juga hanya milik sendiri.
	mine, err := store.APIKeys(ctx, workspace.ID, user.ID)
	if err != nil {
		t.Fatalf("APIKeys: %v", err)
	}
	theirs, err := store.APIKeys(ctx, workspace.ID, other.ID)
	if err != nil {
		t.Fatalf("APIKeys orang lain: %v", err)
	}
	if len(mine) == 0 {
		t.Fatal("daftar pemilik kosong")
	}
	if len(theirs) != 0 {
		t.Fatalf("daftar orang lain = %d, want 0", len(theirs))
	}
}

// TestPgDeleteAPIKeyRemovesTheRow: DELETE is physical (6.2.3).
func TestPgDeleteAPIKeyRemovesTheRow(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()

	key, token, err := store.CreateAPIKey(ctx, workspace.ID, user.ID, "temp")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := store.DeleteAPIKey(ctx, workspace.ID, user.ID, key.ID); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}
	var n int
	if err := pgPool(t).QueryRow(ctx, "select count(*) from api_keys where id = $1", key.ID).Scan(&n); err != nil {
		t.Fatalf("hitung baris: %v", err)
	}
	if n != 0 {
		t.Fatalf("baris masih ada setelah delete: %d", n)
	}
	if _, ok := store.AuthenticateAPIKey(ctx, token); ok {
		t.Fatal("key yang dihapus masih bisa dipakai")
	}
}

// TestPgAPIKeyNameIsValidated: label kosong/kepanjangan ditolak sebelum
// menyentuh database.
func TestPgAPIKeyNameIsValidated(t *testing.T) {
	store, user, workspace := apiKeyFixture(t)
	ctx := context.Background()

	for _, name := range []string{"", "   ", strings.Repeat("x", maxAPIKeyNameLen+1)} {
		if _, _, err := store.CreateAPIKey(ctx, workspace.ID, user.ID, name); !errors.Is(err, ErrAPIKeyNameInvalid) {
			t.Errorf("nama %q = %v, want ErrAPIKeyNameInvalid", name, err)
		}
	}
}
