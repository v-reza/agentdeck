package auth

// Adapter pgx untuk `api_keys` (3.18, 6.2.3).
//
// Konversi tipe ada di satu tempat per arah: `apiKeyRow` dari sqlc -> `APIKey`
// (tanpa hash) dan `APIKeyWithRole` (dengan hash, hanya untuk autentikasi).

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"agentdeck/internal/store"
)

func (r *pgxRepository) CreateAPIKey(ctx context.Context, key APIKey, tokenHash string) (APIKey, error) {
	row, err := r.q.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		ID: key.ID, OrgID: key.OrgID, UserID: key.UserID,
		Name: key.Name, Prefix: key.Prefix, TokenHash: tokenHash,
	})
	if err != nil {
		return APIKey{}, err
	}
	return apiKeyRow(row), nil
}

func (r *pgxRepository) ListAPIKeys(ctx context.Context, userID, orgID string) ([]APIKey, error) {
	rows, err := r.q.ListAPIKeys(ctx, store.ListAPIKeysParams{UserID: userID, OrgID: orgID})
	if err != nil {
		return nil, err
	}
	out := make([]APIKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, apiKeyRow(store.CreateAPIKeyRow(row)))
	}
	return out, nil
}

func (r *pgxRepository) GetAPIKey(ctx context.Context, id, userID, orgID string) (APIKey, error) {
	row, err := r.q.GetAPIKey(ctx, store.GetAPIKeyParams{ID: id, UserID: userID, OrgID: orgID})
	if err != nil {
		return APIKey{}, apiKeyError(err)
	}
	return apiKeyRow(store.CreateAPIKeyRow(row)), nil
}

func (r *pgxRepository) RevokeAPIKey(ctx context.Context, id, userID, orgID string) (int64, error) {
	return r.q.RevokeAPIKey(ctx, store.RevokeAPIKeyParams{ID: id, UserID: userID, OrgID: orgID})
}

func (r *pgxRepository) DeleteAPIKey(ctx context.Context, id, userID, orgID string) (int64, error) {
	return r.q.DeleteAPIKey(ctx, store.DeleteAPIKeyParams{ID: id, UserID: userID, OrgID: orgID})
}

func (r *pgxRepository) APIKeyByPrefix(ctx context.Context, prefix string) (APIKeyWithRole, error) {
	row, err := r.q.APIKeyByPrefix(ctx, prefix)
	if err != nil {
		return APIKeyWithRole{}, apiKeyError(err)
	}
	return APIKeyWithRole{
		APIKey: APIKey{
			ID: row.ID, OrgID: row.OrgID, UserID: row.UserID, Name: row.Name,
			Prefix: row.Prefix, CreatedAt: row.CreatedAt.Time,
			LastUsedAt: optionalTime(row.LastUsedAt), RevokedAt: optionalTime(row.RevokedAt),
		},
		TokenHash: row.TokenHash,
		Role:      Role(row.Role),
	}, nil
}

func (r *pgxRepository) TouchAPIKey(ctx context.Context, id string) error {
	return r.q.TouchAPIKey(ctx, id)
}

// apiKeyRow memetakan baris sqlc ke tipe domain. Hash-nya sengaja tidak
// disalin: `APIKey` tidak punya field untuk menampungnya.
//
// Tiga query mengembalikan tiga named type dengan field yang persis sama
// (Create/List/Get), jadi pemanggil mengonversinya ke satu bentuk dulu —
// Go mengizinkan konversi antar struct dengan underlying type identik, dan itu
// lebih murah daripada tiga fungsi salin yang harus diubah bersamaan.
func apiKeyRow(row store.CreateAPIKeyRow) APIKey {
	return APIKey{
		ID: row.ID, OrgID: row.OrgID, UserID: row.UserID, Name: row.Name,
		Prefix: row.Prefix, CreatedAt: row.CreatedAt.Time,
		LastUsedAt: optionalTime(row.LastUsedAt), RevokedAt: optionalTime(row.RevokedAt),
	}
}

// apiKeyError memetakan "tidak ada baris" ke sentinel paket ini, sehingga
// handler bisa menjawab 404 alih-alih 500 tanpa tahu apa itu pgx.
func apiKeyError(err error) error {
	if err == pgx.ErrNoRows {
		return ErrAPIKeyNotFound
	}
	return err
}

// optionalTime mengubah kolom TIMESTAMPTZ yang boleh NULL jadi *time.Time.
// `Valid=false` (kolom NULL) menghasilkan nil, bukan waktu nol — waktu nol akan
// terbaca sebagai "1 Januari tahun 1" di response.
func optionalTime(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	v := ts.Time
	return &v
}
