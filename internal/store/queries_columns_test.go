package store

import (
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestAgentWritePathsCoverTheSameColumns is the guard for a class of bug that
// no handler test can see: a SQL statement that silently omits a column the
// domain struct carries.
//
// The fake repository used by the handler tests does not enforce the DDL, so a
// create that forgets `base_url` passes every unit test and then fails in
// production with `agents_base_url_chk` surfacing as a 500. That is exactly what
// happened: `CreateAgent` bound 11 columns while `UpdateAgent` bound 12, so
// registering a BYO agent (US-AD106 AC1) could never succeed, and
// `reasoning_effort` sent by a client was dropped without a word.
//
// The invariant is deliberately narrow: every column the *update* path treats as
// mutable must also be written by the *create* path. It is not "create equals
// update" — create owns id/org_id/project_id, which update never touches — and
// it is not "create writes every column in the table", because several columns
// are DDL defaults or written by their own statement.
func TestAgentWritePathsCoverTheSameColumns(t *testing.T) {
	raw, err := os.ReadFile("queries/queries.sql")
	if err != nil {
		t.Fatalf("read queries.sql: %v", err)
	}
	sql := string(raw)

	create := queryBlock(t, sql, "CreateAgent")
	update := queryBlock(t, sql, "UpdateAgent")

	createCols := insertColumns(t, create)
	updateCols := setColumns(t, update)

	if len(createCols) == 0 || len(updateCols) == 0 {
		t.Fatalf("parsed nothing: create=%v update=%v", createCols, updateCols)
	}

	have := make(map[string]bool, len(createCols))
	for _, c := range createCols {
		have[c] = true
	}
	for _, c := range updateCols {
		if !have[c] {
			t.Errorf("UpdateAgent writes %q but CreateAgent does not: a client that "+
				"sets it on create has the value silently dropped, and for base_url "+
				"the insert fails agents_base_url_chk as a 500", c)
		}
	}
}

// queryBlock returns the text of one `-- name: X :one` statement, up to the
// semicolon that ends it.
func queryBlock(t *testing.T, sql, name string) string {
	t.Helper()
	marker := "-- name: " + name + " "
	i := strings.Index(sql, marker)
	if i < 0 {
		t.Fatalf("query %s not found in queries.sql", name)
	}
	rest := sql[i:]
	if j := strings.Index(rest, ";"); j >= 0 {
		return rest[:j]
	}
	return rest
}

var columnRe = regexp.MustCompile(`[a-z_]+`)

// insertColumns reads the column list of `INSERT INTO agents ( ... )`.
func insertColumns(t *testing.T, block string) []string {
	t.Helper()
	upper := strings.ToUpper(block)
	i := strings.Index(upper, "INSERT INTO AGENTS")
	if i < 0 {
		t.Fatalf("no INSERT INTO agents in block:\n%s", block)
	}
	open := strings.Index(block[i:], "(")
	if open < 0 {
		t.Fatalf("no column list in block:\n%s", block)
	}
	open += i
	close := strings.Index(block[open:], ")")
	if close < 0 {
		t.Fatalf("unterminated column list in block:\n%s", block)
	}
	body := block[open+1 : open+close]
	return columnRe.FindAllString(body, -1)
}

// setColumns reads the left-hand side of every assignment in the `SET` clause.
// The clause ends at `WHERE`, so the RETURNING list and the predicate cannot be
// mistaken for assignments.
func setColumns(t *testing.T, block string) []string {
	t.Helper()
	upper := strings.ToUpper(block)
	i := strings.Index(upper, "SET ")
	if i < 0 {
		t.Fatalf("no SET clause in block:\n%s", block)
	}
	rest := block[i+len("SET "):]
	if j := strings.Index(strings.ToUpper(rest), "WHERE"); j >= 0 {
		rest = rest[:j]
	}
	var cols []string
	for _, part := range strings.Split(rest, ",") {
		eq := strings.Index(part, "=")
		if eq < 0 {
			continue
		}
		name := strings.TrimSpace(part[:eq])
		name = strings.TrimSpace(strings.TrimSuffix(name, "\n"))
		if name == "" {
			continue
		}
		cols = append(cols, name)
	}
	return cols
}

// TestListAssignedTasksExcludesArchivedAndScopesByOrg is the SQL-level guard for
// the assignment section of screen 28-agent-detail.
//
// The handler test uses a fake repository, and a fake cannot enforce the WHERE
// clause — it re-implements it, so a statement that stopped filtering would keep
// passing there. These are the three clauses the section's two rules rest on:
//
//	org_id      — the read is tenant-scoped, or one workspace reads another's tasks.
//	assignee    — the rows belong to the agent in the path.
//	archived    — an archived task is not "work this agent holds" (AC1 is about
//	              RUNNING work surviving archiving, so the exclusion is what keeps
//	              the list honest about what is still live).
func TestListAssignedTasksExcludesArchivedAndScopesByOrg(t *testing.T) {
	raw, err := os.ReadFile("queries/queries.sql")
	if err != nil {
		t.Fatalf("read queries.sql: %v", err)
	}
	block := queryBlock(t, string(raw), "ListAssignedTasks")
	compact := strings.Join(strings.Fields(block), " ")

	for _, clause := range []string{
		"org_id = $1",
		"assignee_agent_id = $2",
		"status != 'archived'",
	} {
		if !strings.Contains(compact, clause) {
			t.Errorf("ListAssignedTasks lost %q; the assignment section would report %s",
				clause, map[string]string{
					"org_id = $1":            "another tenant's tasks",
					"assignee_agent_id = $2": "another agent's tasks",
					"status != 'archived'":   "tasks that are no longer live as current work",
				}[clause])
		}
	}
}

// Ensure the file is actually the one we think: a moved queries.sql would make
// the test vacuous rather than failing, and a vacuous guard is worse than none.
func TestQueriesFileIsTheOneWeParse(t *testing.T) {
	raw, err := os.ReadFile("queries/queries.sql")
	if err != nil {
		t.Fatalf("read queries.sql: %v", err)
	}
	sql := string(raw)
	for _, want := range []string{
		"-- name: CreateAgent :one",
		"-- name: UpdateAgent :one",
		"-- name: ListAssignedTasks :many",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("queries.sql no longer contains %q", want)
		}
	}
	if !strings.Contains(sql, "INSERT INTO agents") {
		t.Error("queries.sql no longer inserts into agents")
	}
}

var _ = io.Discard
