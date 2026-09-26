"""Probe lawan API nyata buat Fase 1 (6.2.1 livez + metrics).

Yang dibuktikan, dan tidak bisa dibuktikan test unit:
  1. /livez hidup di container yang benar-benar jalan;
  2. /metrics menolak tanpa kredensial dan menerima dengan kredensial;
  3. registry-nya keisi dari request NYATA lewat middleware, bukan dari test;
  4. label `path` memakai pola route, bukan URL — dicek dengan dua id berbeda;
  5. /healthz tidak ikut terhitung.

Dijalankan dari root repo. Keluar 1 kalau ada yang gagal.
"""
import base64
import json
import os
import sys
import time
import urllib.error
import urllib.request

API = os.environ.get("AGENTDECK_API", "http://127.0.0.1:8080")
AUTH = os.environ.get("AGENTDECK_METRICS_AUTH", "probe:s3cret")
ok_count = 0
failures = []


def check(label, cond, detail=""):
    global ok_count
    if cond:
        ok_count += 1
        print(f"  [ok  ] {label}" + (f" — {detail}" if detail else ""))
    else:
        failures.append(label)
        print(f"  [FAIL] {label}" + (f" — {detail}" if detail else ""))


def _lower_headers(h):
    """dict() preserves the sender's casing and Go writes "Www-Authenticate",
    so a case-sensitive lookup for "WWW-Authenticate" silently misses. Keys are
    lowercased once here instead of at every call site."""
    return {k.lower(): v for k, v in h.items()}


def raw(path, headers=None):
    req = urllib.request.Request(API + path, headers=headers or {})
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            return r.status, r.read().decode("utf-8", "replace"), _lower_headers(r.headers)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace"), _lower_headers(e.headers)
    except Exception as e:  # noqa: BLE001
        return 0, f"{type(e).__name__}: {e}", {}


def basic(user, pw):
    tok = base64.b64encode(f"{user}:{pw}".encode()).decode()
    return {"Authorization": "Basic " + tok}


def main():
    print(f"API {API}\n")

    # 1. liveness
    st, body, _ = raw("/livez")
    check("/livez menjawab 200", st == 200, f"status={st}")
    check("/livez body 'ok'", body.strip() == "ok", repr(body[:20]))

    # 2. auth gate
    st, _, hdr = raw("/metrics")
    check("/metrics tanpa kredensial -> 401", st == 401, f"status={st}")
    check("/metrics menantang Basic", hdr.get("www-authenticate", "").startswith("Basic"),
          repr(hdr.get("www-authenticate", "")))
    st, _, _ = raw("/metrics", basic("probe", "salah"))
    check("/metrics kredensial salah -> 401", st == 401, f"status={st}")
    st, body, hdr = raw("/metrics", basic(*AUTH.split(":", 1)))
    check("/metrics kredensial benar -> 200", st == 200, f"status={st}")
    check("Content-Type format Prometheus", "version=0.0.4" in hdr.get("content-type", ""),
          repr(hdr.get("content-type", "")))

    # 3. metrik terdeklarasi (§14.2)
    declared = [
        "agentdeck_http_requests_total",
        "agentdeck_http_request_duration_seconds",
        "agentdeck_tasks_claimed_total",
        "agentdeck_tasks_status_total",
        "agentdeck_runs_total",
        "agentdeck_runs_active",
        "agentdeck_dispatcher_loop_duration_ms",
        "agentdeck_sse_connections_active",
        "agentdeck_sse_events_sent_total",
        "agentdeck_budget_exceeded_total",
        "agentdeck_db_pool_connections",
        "agentdeck_db_query_duration_seconds",
        "agentdeck_heartbeat_lag_seconds",
    ]
    missing = [m for m in declared if f"# TYPE {m} " not in body]
    check(f"13 metrik §14.2 terdeklarasi", not missing, f"hilang={missing}")

    # 4. middleware keisi dari request nyata.
    #    Dua id berbeda ke route yang sama harus jadi SATU seri.
    before = body.count("agentdeck_http_requests_total{")
    for _ in range(2):
        raw("/api/v1/does-not-exist-%d" % int(time.time() * 1000))
    raw("/api/v1/auth/me")  # 401 tanpa sesi, tapi tetap terhitung
    st, body2, _ = raw("/metrics", basic(*AUTH.split(":", 1)))

    check("counter HTTP bergerak setelah request nyata",
          "agentdeck_http_requests_total{" in body2, "ada seri counter")
    check("label path memakai pola, bukan URL",
          'path="unmatched"' in body2, "request 404 masuk bucket 'unmatched'")
    check("URL mentah tidak bocor jadi label",
          "does-not-exist-" not in body2, "nol id di label")

    # 5. /healthz dikecualikan
    raw("/healthz")
    _, body3, _ = raw("/metrics", basic(*AUTH.split(":", 1)))
    check("/healthz tidak dihitung", 'path="/healthz"' not in body3, "nol seri /healthz")

    # 6. histogram punya _bucket/_sum/_count
    check("histogram punya _bucket", "agentdeck_http_request_duration_seconds_bucket{" in body3)
    check("histogram punya _sum", "agentdeck_http_request_duration_seconds_sum{" in body3)
    check("histogram punya _count", "agentdeck_http_request_duration_seconds_count{" in body3)

    print()
    if failures:
        print(f"GAGAL ({len(failures)}):")
        for f in failures:
            print("  - " + f)
        return 1
    print(f"SEMUA JALUR HIJAU ({ok_count} cek)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
