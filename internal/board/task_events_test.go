package board

// Tes: event lifecycle task benar-benar ditulis — DECISIONS 4 + 7.1.
//
// Kenapa ini perlu tes sendiri: `task.created`, `task.status_changed`, dan
// `task.assigned` dinyatakan di DECISIONS 4, dan `task.status_changed` adalah
// contoh frame di 7.1 sendiri. Tidak ada yang menulisnya sampai F11 — jadi
// timeline task kosong dan SSE tidak punya apa pun untuk dikirim, tanpa satu pun
// error di mana pun. Bug-nya hanya kelihatan dengan membandingkan kontrak
// terhadap baris yang benar-benar masuk ke tabel.
//
// Fixture: runtimeFixture (org, board, dan agent yang benar-benar ada).

import (
	"context"
	"encoding/json"
	"testing"
)

func taskEventsFixture(t *testing.T) runtimeFixture {
	t.Helper()
	return newRuntimeFixture(t, "transient_only", 3)
}

func TestPgCreateTaskRecordsTheCreatedEvent(t *testing.T) {
	f := taskEventsFixture(t)
	ctx := context.Background()

	task, err := f.svc.CreateTask(ctx, f.orgID, f.boardID, "catat pembuatan", "", newUser(t, ctx), 0, StatusBacklog, "")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	events, err := f.svc.TaskHistory(ctx, task.ID)
	if err != nil {
		t.Fatalf("TaskHistory: %v", err)
	}
	var created *Event
	for i := range events {
		if events[i].Kind == "task.created" {
			created = &events[i]
		}
	}
	if created == nil {
		t.Fatalf("task.created tidak tercatat; kind yang ada: %v", kinds(events))
	}
	var payload map[string]any
	if err := json.Unmarshal(created.PayloadJSON, &payload); err != nil {
		t.Fatalf("payload bukan JSON: %v (%s)", err, created.PayloadJSON)
	}
	if payload["task_id"] != task.ID {
		t.Fatalf("payload task_id = %v, want %s", payload["task_id"], task.ID)
	}
	if payload["status"] != string(StatusBacklog) {
		t.Fatalf("payload status = %v, want backlog", payload["status"])
	}
}

func TestPgMoveTaskRecordsTheStatusChange(t *testing.T) {
	f := taskEventsFixture(t)
	ctx := context.Background()

	task, err := f.svc.CreateTask(ctx, f.orgID, f.boardID, "pindah status", "", newUser(t, ctx), 0, StatusBacklog, "")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := f.svc.MoveTask(ctx, task.ID, f.orgID, StatusBacklog, StatusReady); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	events, err := f.svc.TaskHistory(ctx, task.ID)
	if err != nil {
		t.Fatalf("TaskHistory: %v", err)
	}
	var moved *Event
	for i := range events {
		if events[i].Kind == "task.status_changed" {
			moved = &events[i]
		}
	}
	if moved == nil {
		t.Fatalf("task.status_changed tidak tercatat; kind yang ada: %v", kinds(events))
	}
	var payload map[string]any
	if err := json.Unmarshal(moved.PayloadJSON, &payload); err != nil {
		t.Fatalf("payload bukan JSON: %v", err)
	}
	if payload["from"] != "backlog" || payload["to"] != "ready" {
		t.Fatalf("payload from/to = %v/%v, want backlog/ready", payload["from"], payload["to"])
	}
}

// TestPgFailedMoveRecordsNoEvent: transisi yang ditolak tidak boleh meninggalkan
// jejak. Kalau guard status menolak, timeline-nya harus sama seperti sebelumnya.
func TestPgFailedMoveRecordsNoEvent(t *testing.T) {
	f := taskEventsFixture(t)
	ctx := context.Background()

	task, err := f.svc.CreateTask(ctx, f.orgID, f.boardID, "pindah gagal", "", newUser(t, ctx), 0, StatusBacklog, "")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	before, err := f.svc.TaskHistory(ctx, task.ID)
	if err != nil {
		t.Fatalf("TaskHistory: %v", err)
	}
	// `from` yang salah: task ada di backlog, bukan ready.
	if _, err := f.svc.MoveTask(ctx, task.ID, f.orgID, StatusReady, StatusRunning); err == nil {
		t.Fatal("MoveTask dengan from yang salah seharusnya gagal")
	}

	after, err := f.svc.TaskHistory(ctx, task.ID)
	if err != nil {
		t.Fatalf("TaskHistory: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("transisi yang gagal menambah %d event (sebelum %d)",
			len(after)-len(before), len(before))
	}
	for _, ev := range after {
		if ev.Kind == "task.status_changed" {
			t.Fatal("transisi yang gagal meninggalkan task.status_changed di timeline")
		}
	}
}

func TestPgAssignTaskRecordsTheAssignment(t *testing.T) {
	f := taskEventsFixture(t)
	ctx := context.Background()

	task, err := f.svc.CreateTask(ctx, f.orgID, f.boardID, "assign", "", newUser(t, ctx), 0, StatusBacklog, "")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if _, err := f.svc.AssignTask(ctx, task.ID, f.orgID, f.agentID); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	events, err := f.svc.TaskHistory(ctx, task.ID)
	if err != nil {
		t.Fatalf("TaskHistory: %v", err)
	}
	var assigned *Event
	for i := range events {
		if events[i].Kind == "task.assigned" {
			assigned = &events[i]
		}
	}
	if assigned == nil {
		t.Fatalf("task.assigned tidak tercatat; kind yang ada: %v", kinds(events))
	}
	var payload map[string]any
	if err := json.Unmarshal(assigned.PayloadJSON, &payload); err != nil {
		t.Fatalf("payload bukan JSON: %v", err)
	}
	if payload["assignee_agent_id"] != f.agentID {
		t.Fatalf("payload assignee = %v, want %s", payload["assignee_agent_id"], f.agentID)
	}
}

// TestPgTaskEventsCarryTheirOwnTenant: event yang tercatat harus membawa board
// dan org yang benar. Kalau tidak, fan-out SSE akan mengirimkannya ke stream
// board yang salah.
func TestPgTaskEventsCarryTheirOwnTenant(t *testing.T) {
	f := taskEventsFixture(t)
	ctx := context.Background()

	task, err := f.svc.CreateTask(ctx, f.orgID, f.boardID, "scoping", "", newUser(t, ctx), 0, StatusBacklog, "")
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	events, err := f.svc.TaskHistory(ctx, task.ID)
	if err != nil {
		t.Fatalf("TaskHistory: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("tidak ada event sama sekali")
	}
	for _, ev := range events {
		if ev.BoardID != f.boardID {
			t.Fatalf("event %d tercatat di board %s, want %s", ev.ID, ev.BoardID, f.boardID)
		}
		if ev.OrgID != f.orgID {
			t.Fatalf("event %d tercatat di org %s, want %s", ev.ID, ev.OrgID, f.orgID)
		}
	}
}

func kinds(events []Event) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Kind)
	}
	return out
}
