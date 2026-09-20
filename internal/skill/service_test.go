package skill

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeRepo is the in-memory Repo the unit tests drive. It reproduces the two
// behaviours Postgres owns for this table — the per-org slug uniqueness
// (agent_skills_org_slug_key) and org-scoped reads — so the service's error
// mapping is the thing under test rather than the database's.
type fakeRepo struct {
	mu     sync.Mutex
	skills map[string]Skill
	// usage is the agents.skills_json side of the "dipakai oleh" query:
	// orgID -> slug -> set of agent names. Held per org so a cross-tenant test
	// cannot accidentally see another tenant's agents.
	usage map[string]map[string]map[string]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		skills: map[string]Skill{},
		usage:  map[string]map[string]map[string]bool{},
	}
}

func (r *fakeRepo) List(_ context.Context, orgID string) ([]Usage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Usage{}
	for _, s := range r.skills {
		if s.OrgID != orgID {
			continue
		}
		out = append(out, Usage{Skill: s, UsedBy: len(r.usage[orgID][s.Slug])})
	}
	// Mirrors the query's ORDER BY s.is_system DESC, s.slug.
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsSystem != out[j].IsSystem {
			return out[i].IsSystem
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

func (r *fakeRepo) Get(_ context.Context, orgID, id string) (Skill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[id]
	if !ok || s.OrgID != orgID {
		return Skill{}, ErrSkillNotFound
	}
	return s, nil
}

func (r *fakeRepo) Create(_ context.Context, s Skill) (Skill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.skills {
		if existing.OrgID == s.OrgID && existing.Slug == s.Slug {
			return Skill{}, ErrSlugTaken
		}
	}
	r.skills[s.ID] = s
	return s, nil
}

func (r *fakeRepo) Update(_ context.Context, orgID, id, name, bodyMD string) (Skill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[id]
	if !ok || s.OrgID != orgID {
		return Skill{}, ErrSkillNotFound
	}
	s.Name = name
	s.BodyMD = bodyMD
	s.Version++
	r.skills[id] = s
	return s, nil
}

func (r *fakeRepo) Delete(_ context.Context, orgID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[id]
	if !ok || s.OrgID != orgID {
		return ErrSkillNotFound
	}
	if s.IsSystem {
		// The generated query carries `AND is_system = false`; a no-op delete
		// would be reported as success by a count-less exec, which is why the
		// service pre-checks. Reproduced here so a service that lost that
		// pre-check still cannot pass the system-skill test.
		return nil
	}
	delete(r.skills, id)
	return nil
}

func (r *fakeRepo) AgentsUsing(_ context.Context, orgID, slug string) ([]AgentRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	refs := make([]AgentRef, 0, len(r.usage[orgID][slug]))
	for name := range r.usage[orgID][slug] {
		refs = append(refs, AgentRef{ID: "agent-" + strings.ReplaceAll(name, " ", "-"), Name: name})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	return refs, nil
}

// assign records that an agent named `name` references `slug` in `orgID`.
func (r *fakeRepo) assign(orgID, slug, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usage[orgID] == nil {
		r.usage[orgID] = map[string]map[string]bool{}
	}
	if r.usage[orgID][slug] == nil {
		r.usage[orgID][slug] = map[string]bool{}
	}
	r.usage[orgID][slug][name] = true
}

func (r *fakeRepo) count(orgID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, s := range r.skills {
		if s.OrgID == orgID {
			n++
		}
	}
	return n
}

// ---- US-AD107 AC5: the seeded baseline -------------------------------------

// TestSeedSystemSkillsIsIdempotent is AC5 plus the explicit requirement that a
// second seed neither errors nor duplicates. A workspace is seeded when it is
// created, and a retried signup reuses the personal workspace, so this path is
// reachable in production and not just in a test.
func TestSeedSystemSkillsIsIdempotent(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if err := svc.SeedSystemSkills(ctx, "org-a"); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if got := repo.count("org-a"); got != len(SystemSkillSlugs) {
		t.Fatalf("after first seed: %d skills, want %d", got, len(SystemSkillSlugs))
	}

	if err := svc.SeedSystemSkills(ctx, "org-a"); err != nil {
		t.Fatalf("second seed: %v (must not error on an already-seeded org)", err)
	}
	if got := repo.count("org-a"); got != len(SystemSkillSlugs) {
		t.Fatalf("after second seed: %d skills, want %d (duplicates were created)", got, len(SystemSkillSlugs))
	}
}

// TestSeedSystemSkillsAreExactlyTheEightDefaults pins the *contents* of AC5, not
// just the count: the eight slugs, marked system, owned by nobody. A ninth skill
// added to the seed is a failing test, not a silent scope change.
func TestSeedSystemSkillsAreExactlyTheEightDefaults(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if err := svc.SeedSystemSkills(ctx, "org-a"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	listed, err := svc.List(ctx, "org-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	got := make([]string, 0, len(listed))
	for _, row := range listed {
		got = append(got, row.Slug)
		if !row.IsSystem {
			t.Errorf("seeded skill %q: is_system = false, want true", row.Slug)
		}
		if row.CreatedBy != nil {
			t.Errorf("seeded skill %q: created_by = %q, want NULL", row.Slug, *row.CreatedBy)
		}
		if row.BodyMD == "" {
			t.Errorf("seeded skill %q has an empty body; the default must teach something", row.Slug)
		}
		if row.Version != 1 {
			t.Errorf("seeded skill %q: version = %d, want 1", row.Slug, row.Version)
		}
	}

	// List is ordered system-first then by slug, so the expected order is the
	// slugs sorted.
	want := append([]string(nil), SystemSkillSlugs...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("seeded slugs = %v, want %v", got, want)
	}
}

// TestSeedIsPerOrg: one workspace's baseline must never leak into another's.
// The seed runs once per new org, and each org gets its own eight rows.
func TestSeedIsPerOrg(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if err := svc.SeedSystemSkills(ctx, "org-a"); err != nil {
		t.Fatalf("seed org-a: %v", err)
	}
	if err := svc.SeedSystemSkills(ctx, "org-b"); err != nil {
		t.Fatalf("seed org-b: %v", err)
	}
	if got := repo.count("org-a"); got != len(SystemSkillSlugs) {
		t.Fatalf("org-a: %d skills, want %d", got, len(SystemSkillSlugs))
	}
	if got := repo.count("org-b"); got != len(SystemSkillSlugs) {
		t.Fatalf("org-b: %d skills, want %d", got, len(SystemSkillSlugs))
	}

	// A slug that exists in both orgs is the *point* (the same eight names),
	// and a skill created in one org must not be visible from the other.
	if _, err := svc.Create(ctx, "org-a", "user-1", "only_in_a", "Only in A", "# A\n"); err != nil {
		t.Fatalf("create in org-a: %v", err)
	}
	bSkills, err := svc.List(ctx, "org-b")
	if err != nil {
		t.Fatalf("list org-b: %v", err)
	}
	for _, row := range bSkills {
		if row.Slug == "only_in_a" {
			t.Fatal("a skill created in org-a is visible from org-b")
		}
	}
}

// TestSeedDoesNotClobberAnEditedSystemSkill: a system skill is editable data
// (DECISIONS 6A.G). If a user edited its body, a later seed must leave that edit
// alone — re-seeding is "insert what is missing", never "restore the default".
func TestSeedDoesNotClobberAnEditedSystemSkill(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if err := svc.SeedSystemSkills(ctx, "org-a"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	listed, err := svc.List(ctx, "org-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var target string
	for _, row := range listed {
		if row.Slug == "debug" {
			target = row.ID
		}
	}
	if target == "" {
		t.Fatal("seeded library has no `debug` skill")
	}
	if _, err := svc.Update(ctx, "org-a", target, "Debug (mine)", "# Edited\n"); err != nil {
		t.Fatalf("update: %v", err)
	}

	if err := svc.SeedSystemSkills(ctx, "org-a"); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	after, err := svc.Get(ctx, "org-a", target)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if after.Name != "Debug (mine)" || after.BodyMD != "# Edited\n" {
		t.Fatalf("re-seed restored the default over the user's edit: name=%q body=%q", after.Name, after.BodyMD)
	}
	if after.Version != 2 {
		t.Fatalf("version = %d, want 2 (one edit)", after.Version)
	}
}

// ---- CRUD, versioning, and the system-skill guard --------------------------

// TestCreateUpdateDeleteRaisesVersion is AC6: an edit raises version and the old
// content is never rewritten in place. The old *version number* is what a run
// that already loaded v1 recorded, so it must not stay 1 after an edit.
func TestCreateUpdateDeleteRaisesVersion(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	created, err := svc.Create(ctx, "org-a", "user-1", "my_skill", "My skill", "# v1\n")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("created version = %d, want 1", created.Version)
	}
	if created.IsSystem {
		t.Fatal("a user-created skill was marked is_system")
	}
	if created.CreatedBy == nil || *created.CreatedBy != "user-1" {
		t.Fatalf("created_by = %v, want user-1", created.CreatedBy)
	}

	updated, err := svc.Update(ctx, "org-a", created.ID, "My skill", "# v2\n")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version after one edit = %d, want 2", updated.Version)
	}
	if updated.Slug != "my_skill" {
		t.Fatalf("update changed the slug to %q; agents reference the slug", updated.Slug)
	}

	// The list must show the new version too, not a cached one.
	listed, err := svc.List(ctx, "org-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, row := range listed {
		if row.ID == created.ID && row.Version != 2 {
			t.Fatalf("list reports version %d after one edit, want 2", row.Version)
		}
	}

	if err := svc.Delete(ctx, "org-a", created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, "org-a", created.ID); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("get after delete = %v, want ErrSkillNotFound", err)
	}
}

// TestDeleteSystemSkillIsRefused is the US-AD107 AC1/DECISIONS 6A.G rule: a
// seeded default may be edited but never deleted, because agents already
// reference its slug. The refusal must be ErrSystemSkill (409), not a silent
// success — the generated query is a count-less exec, so a no-op would otherwise
// look like a successful delete.
func TestDeleteSystemSkillIsRefused(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if err := svc.SeedSystemSkills(ctx, "org-a"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	listed, err := svc.List(ctx, "org-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("seed produced no skills")
	}
	target := listed[0].ID

	err = svc.Delete(ctx, "org-a", target)
	if !errors.Is(err, ErrSystemSkill) {
		t.Fatalf("delete system skill = %v, want ErrSystemSkill", err)
	}
	// Still there: a refused delete must not have removed the row.
	if _, err := svc.Get(ctx, "org-a", target); err != nil {
		t.Fatalf("system skill vanished after a refused delete: %v", err)
	}
}

// TestCreateRejectsDuplicateSlugWithinOneOrg: the same slug twice in one org is
// a 409 (agent_skills_org_slug_key), while the same slug in *another* org is
// fine — the library is per org.
func TestCreateRejectsDuplicateSlugWithinOneOrg(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "org-a", "user-1", "dup", "One", ""); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := svc.Create(ctx, "org-a", "user-1", "dup", "Two", "")
	if !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("duplicate slug = %v, want ErrSlugTaken", err)
	}
	if _, err := svc.Create(ctx, "org-b", "user-2", "dup", "Two", ""); err != nil {
		t.Fatalf("same slug in another org: %v (slugs are per org)", err)
	}
}

// TestCreateRejectsInvalidSlugAndName covers the validation the DDL would
// otherwise answer with a 500: the pattern is agent_skills_slug_chk, and a blank
// name would render as an empty row.
func TestCreateRejectsInvalidSlugAndName(t *testing.T) {
	svc := NewService(newFakeRepo())
	ctx := context.Background()

	cases := []struct {
		name string
		slug string
		nm   string
	}{
		{"uppercase", "Code-Review", "Code review"},
		{"dash", "code-review", "Code review"},
		{"space", "a b", "Code review"},
		{"empty slug", "", "Code review"},
		{"too long", strings.Repeat("a", 65), "Code review"},
		{"blank name", "code_review_2", "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(ctx, "org-a", "user-1", tc.slug, tc.nm, ""); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("create(%q, %q) = %v, want ErrInvalidInput", tc.slug, tc.nm, err)
			}
		})
	}

	// 64 characters is the inclusive upper bound of the DDL pattern.
	if _, err := svc.Create(ctx, "org-a", "user-1", strings.Repeat("a", 64), "Long", ""); err != nil {
		t.Fatalf("64-character slug rejected: %v", err)
	}
}

// TestAgentsUsingAndUsageCount: the list's "dipakai N agent" column and the
// editor's "dipakai oleh" list must agree, and both must be org-scoped.
func TestAgentsUsingAndUsageCount(t *testing.T) {
	repo := newFakeRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if err := svc.SeedSystemSkills(ctx, "org-a"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	repo.assign("org-a", "code_review", "Agent A")
	repo.assign("org-a", "code_review", "Agent B")
	repo.assign("org-b", "code_review", "Someone Else")

	listed, err := svc.List(ctx, "org-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, row := range listed {
		if row.Slug != "code_review" {
			continue
		}
		if row.UsedBy != 2 {
			t.Fatalf("code_review used_by = %d, want 2 (org-b's agent must not count)", row.UsedBy)
		}
	}

	refs, err := svc.AgentsUsing(ctx, "org-a", "code_review")
	if err != nil {
		t.Fatalf("agents using: %v", err)
	}
	if len(refs) != 2 || refs[0].Name != "Agent A" || refs[1].Name != "Agent B" {
		t.Fatalf("agents using code_review = %v, want [Agent A Agent B]", refs)
	}
}

// TestTenantBoundaryOnGetAndUpdate: a skill id belonging to another org answers
// ErrSkillNotFound (404), never a 403 and never the row. This is the same rule
// every other org-scoped resource in the repo follows (US-AD07).
func TestTenantBoundaryOnGetAndUpdate(t *testing.T) {
	svc := NewService(newFakeRepo())
	ctx := context.Background()

	created, err := svc.Create(ctx, "org-a", "user-1", "mine", "Mine", "# secret\n")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := svc.Get(ctx, "org-b", created.ID); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("cross-tenant get = %v, want ErrSkillNotFound", err)
	}
	if _, err := svc.Update(ctx, "org-b", created.ID, "Stolen", "# stolen\n"); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("cross-tenant update = %v, want ErrSkillNotFound", err)
	}
	if err := svc.Delete(ctx, "org-b", created.ID); !errors.Is(err, ErrSkillNotFound) {
		t.Fatalf("cross-tenant delete = %v, want ErrSkillNotFound", err)
	}
	// And the row is untouched.
	after, err := svc.Get(ctx, "org-a", created.ID)
	if err != nil {
		t.Fatalf("owner get: %v", err)
	}
	if after.Name != "Mine" || after.Version != 1 {
		t.Fatalf("cross-tenant write mutated the row: name=%q version=%d", after.Name, after.Version)
	}
}
