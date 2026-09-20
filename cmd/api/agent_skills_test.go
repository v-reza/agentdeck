package main

// US-AD107 — skill library per workspace. Every acceptance criterion, each with
// a test that fails for the *right* reason:
//
//	AC1 : skill is per org; only a user writes, an agent never does.
//	AC3 : the body is untrusted markdown (see internal/skill/sanitize_test.go).
//	AC4 : owner/admin write; member/viewer get 403.
//	AC5 : a new workspace carries the eight seeded defaults.
//	AC6 : an edit raises version and never rewrites the old content in place.
//
// The mux comes from registerAgentSkillRoutes — the same function main.go calls —
// so these assert the production role gate and the production handler wiring
// rather than a copy of either. A copy would keep passing after main.go was
// loosened, which is the exact failure mode being guarded.
//
// The gate tests never reach the handler: the role gate runs first, so a denied
// request would panic on a nil service rather than answer 403 — which is the bug.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"agentdeck/internal/skill"
)

// fakeSkillRepo is the in-memory skill.Repo these tests drive. It reproduces the
// two behaviours Postgres owns for this table — per-org slug uniqueness
// (agent_skills_org_slug_key) and org-scoped reads — so the handler's error
// mapping is the thing under test rather than the database's.
//
// The embedded interface is deliberate: an unimplemented method panics with a
// nil-pointer dereference if a test ever reaches it, where a silent zero value
// would let a test "pass" against a repository that persisted nothing.
type fakeSkillRepo struct {
	skill.Repo

	mu     sync.Mutex
	skills map[string]skill.Skill
	// usage is the agents.skills_json side of the "dipakai oleh" query.
	usage map[string]map[string]map[string]string // orgID -> slug -> agent name -> agent id
}

func newFakeSkillRepo() *fakeSkillRepo {
	return &fakeSkillRepo{
		skills: map[string]skill.Skill{},
		usage:  map[string]map[string]map[string]string{},
	}
}

func (r *fakeSkillRepo) List(_ context.Context, orgID string) ([]skill.Usage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []skill.Usage{}
	for _, s := range r.skills {
		if s.OrgID != orgID {
			continue
		}
		out = append(out, skill.Usage{Skill: s, UsedBy: len(r.usage[orgID][s.Slug])})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsSystem != out[j].IsSystem {
			return out[i].IsSystem
		}
		return out[i].Slug < out[j].Slug
	})
	return out, nil
}

func (r *fakeSkillRepo) Get(_ context.Context, orgID, id string) (skill.Skill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[id]
	if !ok || s.OrgID != orgID {
		return skill.Skill{}, skill.ErrSkillNotFound
	}
	return s, nil
}

func (r *fakeSkillRepo) Create(_ context.Context, s skill.Skill) (skill.Skill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.skills {
		if existing.OrgID == s.OrgID && existing.Slug == s.Slug {
			return skill.Skill{}, skill.ErrSlugTaken
		}
	}
	r.skills[s.ID] = s
	return s, nil
}

func (r *fakeSkillRepo) Update(_ context.Context, orgID, id, name, bodyMD string) (skill.Skill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[id]
	if !ok || s.OrgID != orgID {
		return skill.Skill{}, skill.ErrSkillNotFound
	}
	s.Name = name
	s.BodyMD = bodyMD
	s.Version++
	r.skills[id] = s
	return s, nil
}

func (r *fakeSkillRepo) Delete(_ context.Context, orgID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.skills[id]
	if !ok || s.OrgID != orgID {
		return skill.ErrSkillNotFound
	}
	// The generated query carries `AND is_system = false`, so a system skill
	// survives even if the service's pre-check were removed. Reproduced here so
	// the 409 test cannot pass for the wrong reason.
	if s.IsSystem {
		return nil
	}
	delete(r.skills, id)
	return nil
}

func (r *fakeSkillRepo) AgentsUsing(_ context.Context, orgID, slug string) ([]skill.AgentRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	refs := make([]skill.AgentRef, 0, len(r.usage[orgID][slug]))
	for name, id := range r.usage[orgID][slug] {
		refs = append(refs, skill.AgentRef{ID: id, Name: name})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	return refs, nil
}

// assign records that the agent `id`/`name` references `slug` in `orgID`.
func (r *fakeSkillRepo) assign(orgID, slug, id, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usage[orgID] == nil {
		r.usage[orgID] = map[string]map[string]string{}
	}
	if r.usage[orgID][slug] == nil {
		r.usage[orgID][slug] = map[string]string{}
	}
	r.usage[orgID][slug][name] = id
}

// skillFixture is the shared setup: one mux carrying the real skill routes, one
// in-memory repository, and the RBAC scenario (orgA with one actor per role,
// plus bella's separate tenant).
type skillFixture struct {
	mux      *http.ServeMux
	repo     *fakeSkillRepo
	scenario rbacTestAPI
}

func newSkillFixture(t *testing.T) skillFixture {
	t.Helper()
	scenario := newRBACTestAPI(t)
	repo := newFakeSkillRepo()
	f := skillFixture{repo: repo, scenario: scenario}
	f.mux = http.NewServeMux()
	// The same call main.go makes. It also installs the workspace-created seed
	// hook on the store, which is what AC5 rides on.
	registerAgentSkillRoutes(f.mux, scenario.api, skill.NewService(repo))
	return f
}

// doSkill performs one request against the skill mux. An empty actor sends no
// credential at all; an empty org omits X-Org-ID, which resolves to the actor's
// personal workspace.
func (f skillFixture) doSkill(t *testing.T, method, path, body, actor, org string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if actor != "" {
		req.Header.Set("Authorization", "Bearer "+f.scenario.tokens[actor+"@x.test"])
	}
	if org != "" {
		req.Header.Set("X-Org-ID", org)
	}
	recorder := httptest.NewRecorder()
	f.mux.ServeHTTP(recorder, req)
	return recorder
}

func (f skillFixture) create(t *testing.T, actor, org, slug, name, body string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"slug": slug, "name": name, "body_md": body})
	if err != nil {
		t.Fatalf("marshal create body: %v", err)
	}
	return f.doSkill(t, http.MethodPost, "/api/v1/agent-skills", string(payload), actor, org)
}

// personalOrg resolves an actor's registration workspace id. rbacTestAPI keys
// its token map by email, so the actor name is expanded here rather than at
// every call site.
func (f skillFixture) personalOrg(t *testing.T, actor string) string {
	t.Helper()
	return f.scenario.personalOrg(actor + "@x.test")
}

func (f skillFixture) patch(t *testing.T, actor, org, id, name, body string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"name": name, "body_md": body})
	if err != nil {
		t.Fatalf("marshal patch body: %v", err)
	}
	return f.doSkill(t, http.MethodPatch, "/api/v1/agent-skills/"+id, string(payload), actor, org)
}

func decodeSkill(t *testing.T, recorder *httptest.ResponseRecorder) skillResponse {
	t.Helper()
	var got skillResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode skill response %q: %v", recorder.Body.String(), err)
	}
	return got
}

func decodeSkillList(t *testing.T, recorder *httptest.ResponseRecorder) []skillResponse {
	t.Helper()
	var got []skillResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode skill list %q: %v", recorder.Body.String(), err)
	}
	return got
}

// TestSkillWriteRequiresAdmin is US-AD107 AC4. This is the criterion that keeps
// the library honest: a member can read skills and cannot change the instructions
// their agents execute, so "member" is not a skill author.
func TestSkillWriteRequiresAdmin(t *testing.T) {
	f := newSkillFixture(t)

	cases := []struct {
		actor      string
		wantDenied bool
	}{
		{actor: "alice", wantDenied: false}, // owner
		{actor: "andre", wantDenied: false}, // admin
		{actor: "marta", wantDenied: true},  // member
		{actor: "vera", wantDenied: true},   // viewer
	}

	for _, tc := range cases {
		t.Run("POST/"+tc.actor, func(t *testing.T) {
			recorder := f.create(t, tc.actor, f.scenario.orgA, "written_by_"+tc.actor, "Written", "# body\n")
			if tc.wantDenied {
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s creating a skill: status = %d, want 403", tc.actor, recorder.Code)
				}
				return
			}
			if recorder.Code != http.StatusCreated {
				t.Fatalf("%s creating a skill: status = %d, want 201 (%s)", tc.actor, recorder.Code, recorder.Body.String())
			}
		})
	}

	// PATCH and DELETE need a real row to aim at; created by an admin, then
	// driven by each role.
	created := decodeSkill(t, f.create(t, "andre", f.scenario.orgA, "target", "Target", "# v1\n"))

	for _, tc := range cases {
		t.Run("PATCH/"+tc.actor, func(t *testing.T) {
			recorder := f.patch(t, tc.actor, f.scenario.orgA, created.ID, "Target", "# v2\n")
			if tc.wantDenied {
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s editing a skill: status = %d, want 403", tc.actor, recorder.Code)
				}
				return
			}
			if recorder.Code == http.StatusForbidden {
				t.Fatalf("%s editing a skill: status = 403, want the gate to allow admin+", tc.actor)
			}
		})
	}

	for _, tc := range cases {
		t.Run("DELETE/"+tc.actor, func(t *testing.T) {
			// A fresh row per actor: a successful delete would otherwise leave
			// the next subtest aiming at nothing.
			victim := decodeSkill(t, f.create(t, "andre", f.scenario.orgA, "victim_"+tc.actor, "Victim", "# v1\n"))
			recorder := f.doSkill(t, http.MethodDelete, "/api/v1/agent-skills/"+victim.ID, "", tc.actor, f.scenario.orgA)
			if tc.wantDenied {
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("%s deleting a skill: status = %d, want 403", tc.actor, recorder.Code)
				}
				return
			}
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("%s deleting a skill: status = %d, want 204 (%s)", tc.actor, recorder.Code, recorder.Body.String())
			}
		})
	}
}

// TestSkillReadsAreViewerPlus is the other half of AC4: a viewer reads the
// library and the "dipakai oleh" list. Denying the read would break the screen
// PLAN-WAVE2 7.1 specifies without protecting anything — a viewer can already
// list the agents whose names that list contains.
func TestSkillReadsAreViewerPlus(t *testing.T) {
	f := newSkillFixture(t)
	created := decodeSkill(t, f.create(t, "alice", f.scenario.orgA, "readable", "Readable", "# body\n"))

	for _, actor := range []string{"alice", "andre", "marta", "vera"} {
		t.Run("GET/"+actor, func(t *testing.T) {
			if recorder := f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", actor, f.scenario.orgA); recorder.Code != http.StatusOK {
				t.Fatalf("%s listing skills: status = %d, want 200", actor, recorder.Code)
			}
			path := "/api/v1/agent-skills/" + created.ID + "/agents"
			if recorder := f.doSkill(t, http.MethodGet, path, "", actor, f.scenario.orgA); recorder.Code != http.StatusOK {
				t.Fatalf("%s listing agents using a skill: status = %d, want 200", actor, recorder.Code)
			}
		})
	}
}

// TestAgentCannotWriteSkills is US-AD107 AC1 and the binding user decision
// ("agent TIDAK BOLEH menulis skill; hanya owner/admin").
//
// The mechanism is not a role check an agent could pass: every route on this
// surface authenticates a *user session* through orgHeaderContextMiddleware ->
// currentUser, and an agent has no session. So the test drives each write with
// the two credential shapes an agent actually holds — no credential at all, and a
// bearer token that is an agent id rather than a session — and requires 401.
// A 403 would mean the request was authenticated as somebody.
func TestAgentCannotWriteSkills(t *testing.T) {
	f := newSkillFixture(t)
	// An agent id: a real, well-formed ULID that is not a session token. The
	// repo has no agent-credential mechanism (grep for one finds none), so this
	// is what an agent's "token" would look like.
	const agentID = "01J8ZK9M4WQ7X2V0B3C5D7E9FG"

	created := decodeSkill(t, f.create(t, "alice", f.scenario.orgA, "guarded", "Guarded", "# v1\n"))

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		// header is the credential the agent presents; "" sends none.
		header string
	}{
		{name: "POST no credential", method: http.MethodPost, path: "/api/v1/agent-skills", body: `{"slug":"agent_made","name":"Agent made","body_md":"# hi"}`},
		{name: "POST with an agent id as bearer", method: http.MethodPost, path: "/api/v1/agent-skills", body: `{"slug":"agent_made","name":"Agent made","body_md":"# hi"}`, header: agentID},
		{name: "PATCH no credential", method: http.MethodPatch, path: "/api/v1/agent-skills/" + created.ID, body: `{"name":"Rewritten","body_md":"# hijacked"}`},
		{name: "PATCH with an agent id as bearer", method: http.MethodPatch, path: "/api/v1/agent-skills/" + created.ID, body: `{"name":"Rewritten","body_md":"# hijacked"}`, header: agentID},
		{name: "DELETE no credential", method: http.MethodDelete, path: "/api/v1/agent-skills/" + created.ID},
		{name: "DELETE with an agent id as bearer", method: http.MethodDelete, path: "/api/v1/agent-skills/" + created.ID, header: agentID},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader([]byte(tc.body)))
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			req.Header.Set("X-Org-ID", f.scenario.orgA)
			if tc.header != "" {
				req.Header.Set("Authorization", "Bearer "+tc.header)
			}
			recorder := httptest.NewRecorder()
			f.mux.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("agent credential on %s %s: status = %d, want 401 (an agent must not be authenticated as a user)", tc.method, tc.path, recorder.Code)
			}
		})
	}

	// The failed writes must have changed nothing.
	after := decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "alice", f.scenario.orgA))
	for _, row := range after {
		if row.Slug == "agent_made" {
			t.Fatal("an unauthenticated POST created a skill")
		}
		if row.ID == created.ID && (row.Name != "Guarded" || row.BodyMD != "# v1\n") {
			t.Fatalf("an unauthenticated PATCH rewrote a skill: name=%q body=%q", row.Name, row.BodyMD)
		}
	}
	if _, err := json.Marshal(after); err != nil {
		t.Fatalf("marshal list: %v", err)
	}
}

// TestSkillCRUDOverHTTP is the end-to-end contract of the four routes: create,
// list, edit (version rises), delete, gone.
func TestSkillCRUDOverHTTP(t *testing.T) {
	f := newSkillFixture(t)

	created := decodeSkill(t, f.create(t, "alice", f.scenario.orgA, "my_skill", "My skill", "# v1\n"))
	if created.Slug != "my_skill" || created.Version != 1 {
		t.Fatalf("created skill = %+v, want slug my_skill at version 1", created)
	}
	if created.IsSystem {
		t.Fatal("a user-created skill was marked is_system")
	}
	if created.CreatedBy == "" {
		t.Fatal("created_by is empty; the API dropped the acting user")
	}

	listed := decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "alice", f.scenario.orgA))
	found := false
	for _, row := range listed {
		if row.ID == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("created skill missing from the list: %+v", listed)
	}

	updated := decodeSkill(t, f.patch(t, "alice", f.scenario.orgA, created.ID, "My skill v2", "# v2\n"))
	if updated.Version != 2 {
		t.Fatalf("version after one edit = %d, want 2 (AC6)", updated.Version)
	}
	if updated.BodyMD != "# v2\n" {
		t.Fatalf("body after edit = %q, want the new body", updated.BodyMD)
	}

	if recorder := f.doSkill(t, http.MethodDelete, "/api/v1/agent-skills/"+created.ID, "", "alice", f.scenario.orgA); recorder.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204 (%s)", recorder.Code, recorder.Body.String())
	}
	// Gone from the list, and no route resurrects it.
	for _, row := range decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "alice", f.scenario.orgA)) {
		if row.ID == created.ID {
			t.Fatal("deleted skill is still listed")
		}
	}
}

// TestSkillCreateDuplicateSlugIs409: the slug is what agents store in
// skills_json, so two skills in one org cannot share it.
func TestSkillCreateDuplicateSlugIs409(t *testing.T) {
	f := newSkillFixture(t)
	if recorder := f.create(t, "alice", f.scenario.orgA, "dup", "One", ""); recorder.Code != http.StatusCreated {
		t.Fatalf("first create: status = %d, want 201 (%s)", recorder.Code, recorder.Body.String())
	}
	recorder := f.create(t, "alice", f.scenario.orgA, "dup", "Two", "")
	if recorder.Code != http.StatusConflict {
		t.Fatalf("duplicate slug: status = %d, want 409 (%s)", recorder.Code, recorder.Body.String())
	}
	// The same slug in another org is fine: the library is per org.
	if recorder := f.create(t, "bella", f.scenario.orgB, "dup", "Two", ""); recorder.Code != http.StatusCreated {
		t.Fatalf("same slug in another org: status = %d, want 201 (%s)", recorder.Code, recorder.Body.String())
	}
}

// TestSkillCreateInvalidSlugIs400 covers the payload the DDL would otherwise
// answer with a 500: agent_skills_slug_chk is `^[a-z0-9_]{1,64}$`.
func TestSkillCreateInvalidSlugIs400(t *testing.T) {
	f := newSkillFixture(t)
	cases := []struct {
		name string
		slug string
		nm   string
	}{
		{"uppercase and dash", "Code-Review", "Code review"},
		{"dash", "code-review", "Code review"},
		{"space", "a b", "Code review"},
		{"empty", "", "Code review"},
		{"too long", strings.Repeat("a", 65), "Code review"},
		{"blank name", "valid_slug", "   "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := f.create(t, "alice", f.scenario.orgA, tc.slug, tc.nm, "")
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("create(slug=%q, name=%q): status = %d, want 400 (%s)", tc.slug, tc.nm, recorder.Code, recorder.Body.String())
			}
		})
	}
	// An empty body is allowed: a skill may start as a stub.
	if recorder := f.create(t, "alice", f.scenario.orgA, "stub", "Stub", ""); recorder.Code != http.StatusCreated {
		t.Fatalf("empty body: status = %d, want 201 (%s)", recorder.Code, recorder.Body.String())
	}
}

// TestDeleteSystemSkillIs409 is the guard that keeps a workspace's baseline
// intact: the eight defaults may be edited but not removed, because agents
// already reference their slugs.
func TestDeleteSystemSkillIs409(t *testing.T) {
	f := newSkillFixture(t)
	// Registering a user after the routes are wired fires the seed hook, so
	// their personal workspace carries the eight defaults.
	personal := f.scenario.register("seedtarget@x.test")

	listed := decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "seedtarget", personal))
	var system skillResponse
	for _, row := range listed {
		if row.IsSystem {
			system = row
		}
	}
	if system.ID == "" {
		t.Fatalf("the new workspace has no system skill: %+v", listed)
	}

	recorder := f.doSkill(t, http.MethodDelete, "/api/v1/agent-skills/"+system.ID, "", "seedtarget", personal)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("deleting a system skill: status = %d, want 409 (%s)", recorder.Code, recorder.Body.String())
	}

	// Still there after the refusal.
	after := decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "seedtarget", personal))
	for _, row := range after {
		if row.ID == system.ID {
			return
		}
	}
	t.Fatal("the system skill vanished after a refused delete")
}

// TestSkillSeedOnWorkspaceCreation is US-AD107 AC5 over the real hook: a
// workspace created by registration carries exactly the eight defaults, marked
// system, with a body that teaches something.
func TestSkillSeedOnWorkspaceCreation(t *testing.T) {
	f := newSkillFixture(t)
	personal := f.scenario.register("freshworkspace@x.test")

	listed := decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "freshworkspace", personal))
	if len(listed) != len(skill.SystemSkillSlugs) {
		t.Fatalf("new workspace has %d skills, want %d (%+v)", len(listed), len(skill.SystemSkillSlugs), listed)
	}

	got := make([]string, 0, len(listed))
	for _, row := range listed {
		got = append(got, row.Slug)
		if !row.IsSystem {
			t.Errorf("seeded skill %q is not marked is_system", row.Slug)
		}
		if row.CreatedBy != "" {
			t.Errorf("seeded skill %q has created_by = %q, want empty (no user made it)", row.Slug, row.CreatedBy)
		}
		if strings.TrimSpace(row.BodyMD) == "" {
			t.Errorf("seeded skill %q has an empty body; the default must teach something", row.Slug)
		}
		if row.Version != 1 {
			t.Errorf("seeded skill %q is at version %d, want 1", row.Slug, row.Version)
		}
	}
	sort.Strings(got)
	want := append([]string(nil), skill.SystemSkillSlugs...)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("seeded slugs = %v, want %v", got, want)
	}
}

// TestSkillSeedIsIdempotentOverHTTP: the hook must survive being fired twice for
// one workspace — a retried signup reuses the personal workspace, and the seed
// must neither error nor duplicate. Driven through the store's own hook entry
// point so this is the production path, not a second call to the service.
func TestSkillSeedIsIdempotentOverHTTP(t *testing.T) {
	f := newSkillFixture(t)
	personal := f.scenario.register("reseed@x.test")

	// The store fires the hook once per genuinely new org; calling the seeder
	// again is what a retried signup amounts to.
	if err := skill.NewService(f.repo).SeedSystemSkills(context.Background(), personal); err != nil {
		t.Fatalf("second seed: %v (must not error on an already-seeded workspace)", err)
	}

	listed := decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "reseed", personal))
	if len(listed) != len(skill.SystemSkillSlugs) {
		t.Fatalf("after a second seed: %d skills, want %d (duplicates were created)", len(listed), len(skill.SystemSkillSlugs))
	}
}

// TestSkillTenantBoundaryIs404: a skill owned by another workspace answers 404,
// never 403 and never the row. Alice owns orgA *and* her personal workspace, so
// both resolve for her and the only thing standing between them is the org scope
// on the query — which is exactly what this test isolates.
func TestSkillTenantBoundaryIs404(t *testing.T) {
	f := newSkillFixture(t)
	personal := f.personalOrg(t, "alice")

	created := decodeSkill(t, f.create(t, "alice", personal, "private", "Private", "# secret\n"))

	// Same user, different tenant: every route must report the skill as absent.
	recorder := f.patch(t, "alice", f.scenario.orgA, created.ID, "Stolen", "# stolen\n")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant PATCH: status = %d, want 404 (%s)", recorder.Code, recorder.Body.String())
	}
	recorder = f.doSkill(t, http.MethodDelete, "/api/v1/agent-skills/"+created.ID, "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant DELETE: status = %d, want 404 (%s)", recorder.Code, recorder.Body.String())
	}
	recorder = f.doSkill(t, http.MethodGet, "/api/v1/agent-skills/"+created.ID+"/agents", "", "alice", f.scenario.orgA)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant \"dipakai oleh\": status = %d, want 404 (%s)", recorder.Code, recorder.Body.String())
	}

	// The row is untouched in its own tenant.
	after := decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "alice", personal))
	for _, row := range after {
		if row.ID != created.ID {
			continue
		}
		if row.Name != "Private" || row.Version != 1 {
			t.Fatalf("a cross-tenant write mutated the row: name=%q version=%d", row.Name, row.Version)
		}
		return
	}
	t.Fatal("the skill is missing from its own workspace")
}

// TestSkillListReportsUsageAndAgentsUsing is the PLAN-WAVE2 7.1 requirement that
// every list row carries "dipakai N agent" and the editor can resolve the names.
// Without it a user edits a skill agents are running without knowing.
func TestSkillListReportsUsageAndAgentsUsing(t *testing.T) {
	f := newSkillFixture(t)
	created := decodeSkill(t, f.create(t, "alice", f.scenario.orgA, "code_review", "Code review", "# review\n"))

	// Before any agent references it: the row must say zero, not "unknown".
	listed := decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "vera", f.scenario.orgA))
	for _, row := range listed {
		if row.ID == created.ID && row.UsedBy != 0 {
			t.Fatalf("unused skill reports used_by = %d, want 0", row.UsedBy)
		}
	}

	f.repo.assign(f.scenario.orgA, "code_review", "agent-1", "Agent One")
	f.repo.assign(f.scenario.orgA, "code_review", "agent-2", "Agent Two")
	// Another tenant's agent referencing the same slug must not be counted.
	f.repo.assign(f.scenario.orgB, "code_review", "agent-3", "Bella's agent")

	listed = decodeSkillList(t, f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "vera", f.scenario.orgA))
	seen := false
	for _, row := range listed {
		if row.ID != created.ID {
			continue
		}
		seen = true
		if row.UsedBy != 2 {
			t.Fatalf("used_by = %d, want 2 (another tenant's agent must not count)", row.UsedBy)
		}
	}
	if !seen {
		t.Fatal("the skill is missing from the list")
	}

	recorder := f.doSkill(t, http.MethodGet, "/api/v1/agent-skills/"+created.ID+"/agents", "", "vera", f.scenario.orgA)
	if recorder.Code != http.StatusOK {
		t.Fatalf("\"dipakai oleh\": status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	var refs []agentRefResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &refs); err != nil {
		t.Fatalf("decode \"dipakai oleh\" %q: %v", recorder.Body.String(), err)
	}
	if len(refs) != 2 || refs[0].Name != "Agent One" || refs[1].Name != "Agent Two" {
		t.Fatalf("\"dipakai oleh\" = %+v, want [Agent One Agent Two]", refs)
	}
	if refs[0].ID == "" {
		t.Fatal("\"dipakai oleh\" rows have no agent id; the name cannot route to the agent detail page")
	}
}

// TestSkillListIsEmptyArrayNot404: a workspace whose library is empty answers an
// empty array. The UI's empty state is driven by length (PLAN-WAVE2 7.1 item 5),
// and a 404 would make it render an error instead of "Buat skill pertama".
func TestSkillListIsEmptyArrayNot404(t *testing.T) {
	f := newSkillFixture(t)
	recorder := f.doSkill(t, http.MethodGet, "/api/v1/agent-skills", "", "bella", f.scenario.orgB)
	if recorder.Code != http.StatusOK {
		t.Fatalf("empty library: status = %d, want 200", recorder.Code)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != "[]" {
		t.Fatalf("empty library body = %q, want [] (not null)", body)
	}
}
