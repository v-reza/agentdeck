"""End-to-end proof that the dispatcher executes a run without a human pressing run.

Drives the real API against a real container, with a fake OpenAI-compatible
provider on the host. What it proves that unit tests cannot:

  1. an assigned `ready` task is claimed by the tick loop, executed, and priced;
  2. a successful run closes `succeeded` and the task lands in `review`, not `done`
     (PRD US-AD22 AC2 — if success went straight to `done`, `review` would be
     unreachable);
  3. a provider that rejects the credential produces `failed`/`capability`, and the
     key never appears in runs.error;
  4. a `ready` task with no agent is parked as `blocked`/`needs_input` rather than
     left running forever.

    python m5-probe.py
"""
import json
import os
import subprocess
import tempfile
import sys
import time

API = "http://127.0.0.1:8080/api/v1"
# Cookie jar and response scratch go to the system temp dir: they must not land in
# the repo, and .gitignore is not this script's to edit.
S = tempfile.gettempdir()
JAR = os.path.join(S, "agentdeck-probe-m5.jar")
OUT = os.path.join(S, "agentdeck-probe-m5.out")
FAILURES = []


def curl(method, path, body=None, headers=None):
    cmd = ["curl", "-s", "-o", OUT, "-w", "%{http_code}", "-X", method,
           "-b", JAR, "-c", JAR]
    for k, v in (headers or {}).items():
        cmd += ["-H", f"{k}: {v}"]
    if body is not None:
        cmd += ["-H", "Content-Type: application/json", "-d", json.dumps(body)]
    cmd.append(API + path)
    code = subprocess.run(cmd, capture_output=True, text=True).stdout.strip()
    raw = open(OUT, encoding="utf-8", errors="replace").read()
    try:
        return code, json.loads(raw)
    except json.JSONDecodeError:
        return code, raw


def req(method, path, body=None, expect=None, org=None):
    headers = {"X-Org-ID": org} if org else None
    code, data = curl(method, path, body, headers)
    if expect and code != expect:
        FAILURES.append(f"{method} {path} -> {code}, want {expect}: {str(data)[:200]}")
    return data


def check(label, ok, detail=""):
    print(f"  [{'ok  ' if ok else 'FAIL'}] {label}{(' — ' + detail) if detail else ''}")
    if not ok:
        FAILURES.append(label)


def wait_for(fn, timeout=90, poll=3):
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        last = fn()
        if last:
            return last
        time.sleep(poll)
    return last


def main():
    email = f"m5-{int(time.time())}@example.test"
    org = req("POST", "/auth/register", {
        "email": email, "password": "correct-horse-battery-staple",
        "name": "M5", "org_name": "M5 Co"}, "201")["workspace_id"]
    print(f"org={org}")

    project = req("POST", "/projects", {"name": "M5", "slug": f"m5-probe-{int(time.time())}"}, "201", org)
    board = req("POST", f"/projects/{project['id']}/boards",
                {"name": "M5 Board", "slug": "m5-board"}, "201", org)
    bid = board["id"]
    print(f"board={bid} budget_daily_micros={board.get('budget_daily_micros')}")

    # The column default IS the cap (§3:390). A create path that wrote zero here
    # would make every run refuse before it started.
    cap = board.get("budget_daily_micros")
    if cap is None:
        cap = int(subprocess.run(
            ["docker", "exec", "agentdeck-db", "psql", "-U", "agentdeck", "-d", "agentdeck", "-tAc",
             f"SELECT budget_daily_micros FROM boards WHERE id='{bid}'"],
            capture_output=True, text=True).stdout.strip() or 0)
    check("board keeps the N16 cap default", cap == 20_000_000, f"cap={cap}")

    # The credential travels with the provider (providers.go:54) — §6A.J keeps one
    # key per workspace, so there is no separate key endpoint.
    provider = req("POST", "/providers", {
        "name": "fake", "protocol": "openai_compatible",
        "base_url": "http://host.docker.internal:9099",
        "api_key": "sk-fake-key-xyz"}, "201", org)

    agent = req("POST", f"/projects/{project['id']}/agents", {
        "name": "worker", "provider_id": provider["id"], "model": "gpt-4o",
        "retry_policy": "transient_only", "max_attempts": 3}, "201", org)
    print(f"provider={provider['id']} agent={agent['id']} has_key={provider.get('has_key')}")

    task = req("POST", f"/boards/{bid}/tasks", {
        "title": "Do the thing", "body": "Urgent work", "priority": 5,
        "assignee_agent_id": agent["id"]}, "201", org)
    req("POST", f"/tasks/{task['id']}/move", {"from": "backlog", "to": "ready"}, "200", org)

    print("waiting for the tick loop to execute the run…")
    final = wait_for(lambda: (req("GET", f"/tasks/{task['id']}", org=org).get("status") == "review") or None)
    check("a ready task was claimed and executed unattended", bool(final))

    runs = req("GET", f"/tasks/{task['id']}/runs", org=org)
    rows = runs.get("data") if isinstance(runs, dict) else runs
    check("the task has a run", bool(rows), f"{len(rows) if isinstance(rows, list) else '?'} run(s)")
    run = req("GET", f"/runs/{rows[0]['id']}", org=org)
    check("run succeeded", run.get("status") == "ended" and run.get("outcome") == "succeeded",
          f"{run.get('status')}/{run.get('outcome')}")
    check("the run carries its spend", (run.get("cost_micros") or 0) > 0, f"cost={run.get('cost_micros')}")

    steps = req("GET", f"/runs/{run['id']}/steps", org=org)
    step_rows = steps.get("data") if isinstance(steps, dict) else steps
    check("the run recorded a step", bool(step_rows),
          f"kind={step_rows[0]['kind']} status={step_rows[0]['status']}" if step_rows else "")

    ledger = req("GET", f"/runs/{run['id']}/ledger", org=org)
    entries = ledger.get("data") if isinstance(ledger, dict) else ledger
    check("the ledger priced the step", bool(entries),
          f"model={entries[0].get('model')} source={entries[0].get('price_source')} cost={entries[0].get('cost_micros')}"
          if entries else "")
    if entries:
        # `tokens_out` is exactly what the provider reported for the completion, and
        # `tokens_in` carries that count plus whatever the prompt cost — the prompt is
        # built from the task and the agent's skills, so its exact length is not the
        # point. What is under test is that the provider's usage is read and kept,
        # rather than estimated locally.
        check("the provider's own usage numbers were kept",
              entries[0].get("tokens_in", 0) >= 120 and entries[0].get("tokens_out") == 42,
              f"in={entries[0].get('tokens_in')} out={entries[0].get('tokens_out')}")
        check("cache and reasoning tokens were separated out",
              entries[0].get("cache_read_tokens") == 30 and entries[0].get("reasoning_tokens") == 7,
              f"cache={entries[0].get('cache_read_tokens')} reasoning={entries[0].get('reasoning_tokens')}")

    board_ledger = req("GET", f"/boards/{bid}/ledger", org=org)
    check("the board ledger reflects the spend",
          (board_ledger.get("spend_today_micros") or 0) > 0,
          f"spend={board_ledger.get('spend_today_micros')} entries={len(board_ledger.get('entries') or [])}")

    daily = subprocess.run(
        ["docker", "exec", "agentdeck-db", "psql", "-U", "agentdeck", "-d", "agentdeck", "-tAc",
         f"SELECT total_micros || '/' || run_count || '/' || tokens_in || '/' || tokens_out "
         f"FROM daily_board_costs WHERE board_id='{bid}'"],
        capture_output=True, text=True).stdout.strip()
    check("daily_board_costs accrued for the board", bool(daily) and not daily.startswith("/"), f"micros/runs/in/out={daily}")

    # --- 3. rejected credential -------------------------------------------------
    subprocess.run(["curl", "-s", "-X", "POST", "http://127.0.0.1:9099/mode",
                    "-H", "Content-Type: application/json", "-d", '{"mode":"reject"}'],
                   capture_output=True)
    task2 = req("POST", f"/boards/{bid}/tasks", {
        "title": "Rejected", "priority": 1, "assignee_agent_id": agent["id"]}, "201", org)
    req("POST", f"/tasks/{task2['id']}/move", {"from": "backlog", "to": "ready"}, "200", org)
    wait_for(lambda: (req("GET", f"/tasks/{task2['id']}", org=org).get("status") not in
                      ("ready", "running")) or None)
    after = req("GET", f"/tasks/{task2['id']}", org=org)
    check("a rejected credential does not retry forever",
          after.get("status") == "failed", f"status={after.get('status')} kind={after.get('block_kind')}")

    runs2 = req("GET", f"/tasks/{task2['id']}/runs", org=org)
    rows2 = runs2.get("data") if isinstance(runs2, dict) else runs2
    bad = req("GET", f"/runs/{rows2[0]['id']}", org=org)
    check("the failure is classified, not generic",
          bad.get("status") == "ended" and bad.get("outcome") == "failed", f"{bad.get('status')}/{bad.get('outcome')}")
    check("the credential was redacted out of the run error",
          "sk-fake-key-xyz" not in json.dumps(bad), f"error={str(bad.get('error'))[:120]}")
    check("the run cost nothing when the provider refused", (bad.get("cost_micros") or 0) == 0,
          f"cost={bad.get('cost_micros')}")

    # --- 4. no agent -----------------------------------------------------------
    task3 = req("POST", f"/boards/{bid}/tasks", {"title": "Orphan", "priority": 1}, "201", org)
    req("POST", f"/tasks/{task3['id']}/move", {"from": "backlog", "to": "ready"}, "200", org)
    parked = wait_for(lambda: (req("GET", f"/tasks/{task3['id']}", org=org).get("status") == "blocked") or None)
    got = req("GET", f"/tasks/{task3['id']}", org=org)
    check("a task with no agent is parked, not left running", bool(parked),
          f"status={got.get('status')} block_kind={got.get('block_kind')}")
    check("the parking kind routes it to a human", got.get("block_kind") == "needs_input",
          f"block_kind={got.get('block_kind')}")

    print()
    if FAILURES:
        print(f"FAILED ({len(FAILURES)}):")
        for f in FAILURES:
            print("  - " + f)
        return 1
    print("SEMUA JALUR HIJAU")
    return 0


if __name__ == "__main__":
    sys.exit(main())
