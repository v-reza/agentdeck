package artifact

// Adapter Postgres — Repo dan TaskLookup.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"agentdeck/internal/store"
)

// PgRepo membaca/menulis `artifacts` (3.15).
type PgRepo struct{ q *store.Queries }

// NewPgRepo merakit Repo di atas pool.
func NewPgRepo(pool *pgxpool.Pool) *PgRepo { return &PgRepo{q: store.New(pool)} }

// ListByTask mengembalikan metadata artifact satu task.
func (r *PgRepo) ListByTask(ctx context.Context, orgID, taskID string) ([]Artifact, error) {
	rows, err := r.q.ListTaskArtifacts(ctx, store.ListTaskArtifactsParams{TaskID: taskID, OrgID: orgID})
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(rows))
	for _, row := range rows {
		out = append(out, artifactFromRow(row))
	}
	return out, nil
}

// Get membaca satu artifact, ter-scope org.
func (r *PgRepo) Get(ctx context.Context, orgID, id string) (Artifact, error) {
	row, err := r.q.GetArtifact(ctx, store.GetArtifactParams{ID: id, OrgID: orgID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Artifact{}, ErrNotFound
		}
		return Artifact{}, err
	}
	return artifactFromRow(row), nil
}

// Create menulis satu baris.
func (r *PgRepo) Create(ctx context.Context, a Artifact) (Artifact, error) {
	row, err := r.q.CreateArtifact(ctx, store.CreateArtifactParams{
		ID:          a.ID,
		OrgID:       a.OrgID,
		TaskID:      a.TaskID,
		RunID:       a.RunID,
		Filename:    a.Filename,
		ContentType: a.ContentType,
		Size:        int32(a.Size),
		StorageKey:  a.StorageKey,
		Sha256:      a.SHA256,
	})
	if err != nil {
		return Artifact{}, err
	}
	return artifactFromRow(row), nil
}

// SumTaskBytes menjumlahkan ukuran artifact satu task (kuota N22).
func (r *PgRepo) SumTaskBytes(ctx context.Context, orgID, taskID string) (int64, error) {
	return r.q.SumTaskArtifactSize(ctx, store.SumTaskArtifactSizeParams{TaskID: taskID, OrgID: orgID})
}

func artifactFromRow(row store.Artifact) Artifact {
	return Artifact{
		ID:          row.ID,
		OrgID:       row.OrgID,
		TaskID:      row.TaskID,
		RunID:       row.RunID,
		Filename:    row.Filename,
		ContentType: row.ContentType,
		Size:        int64(row.Size),
		StorageKey:  row.StorageKey,
		SHA256:      row.Sha256,
		CreatedAt:   row.CreatedAt.Time,
	}
}

// PgTaskLookup membuktikan task ada dan milik org yang sama.
//
// Query-nya sengaja tidak memakai GetTask dari paket board: paket itu menarik
// seluruh graf task (assignee, label, dependency) untuk pertanyaan yang hanya
// butuh "ada atau tidak". Satu SELECT EXISTS lebih murah dan tidak menambah
// ketergantungan antar paket.
type PgTaskLookup struct{ pool *pgxpool.Pool }

// NewPgTaskLookup merakit TaskLookup.
func NewPgTaskLookup(pool *pgxpool.Pool) *PgTaskLookup { return &PgTaskLookup{pool: pool} }

// TaskExists melaporkan apakah task ada di org itu.
func (l *PgTaskLookup) TaskExists(ctx context.Context, orgID, taskID string) (bool, error) {
	var exists bool
	err := l.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM tasks WHERE id = $1 AND org_id = $2)`,
		taskID, orgID).Scan(&exists)
	return exists, err
}

// RunExists melaporkan apakah run ada di org itu.
//
// `runs` punya `org_id` sendiri, jadi penyaringan org-nya langsung — tidak
// perlu lewat task-nya.
func (l *PgTaskLookup) RunExists(ctx context.Context, orgID, runID string) (bool, error) {
	var exists bool
	err := l.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM runs WHERE id = $1 AND org_id = $2)`,
		runID, orgID).Scan(&exists)
	return exists, err
}
