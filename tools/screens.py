#!/usr/bin/env python3
"""Peta coverage AgentDeck: story <-> layar <-> prompt.

Satu sumber kebenaran. `screens.py` menghasilkan:
  - COVERAGE.md   : matriks dua arah (story->layar, layar->story)
  - laporan gap   : story tanpa layar, layar tanpa story

Aturan:
  * Tiap story WAJIB punya tepat satu status: 'screen' (punya layar),
    atau 'backend' (memang tidak punya wujud visual, dengan alasan tertulis).
  * Story berstatus 'screen' tanpa entri di SCREENS = GAP -> gate FAIL.
  * Layar tanpa story = layar yatim -> gate FAIL.

Jalankan: python screens.py            (tulis COVERAGE.md + laporan)
          python screens.py --check    (exit 1 kalau ada gap)
"""

import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
HERE = ROOT

# ---------------------------------------------------------------- SCREENS ----
# id: (judul, route, stories, states, catatan)
# states: varian yang WAJIB digenerate sebagai frame terpisah
SCREENS = {
    # ---- publik (tanpa shell) ----
    "01-login": ("Login", "/login", ["US-AD02"], ["default", "error"],
                 "Blueprint D centered"),
    "02-register": ("Register", "/register", ["US-AD01"], ["default", "error"],
                    "Blueprint D centered"),
    "03-reset-request": ("Lupa password — minta tautan", "/reset", ["US-AD88"], ["default", "sent"],
                         "Blueprint D centered"),
    "04-reset-confirm": ("Lupa password — password baru", "/reset/:token", ["US-AD88"],
                         ["default", "expired"], "Blueprint D centered"),
    "05-landing": ("Landing page", "/", ["US-AD100"], ["default"], "Blueprint P (publik, standalone)"),
    "06-docs-quickstart": ("Dokumentasi — mulai cepat", "/docs/quickstart", ["US-AD103"],
                           ["default"], "Blueprint P-DOCS (publik, standalone, nav dokumen)"),
    "06b-docs-api": ("Dokumentasi — REST API", "/docs/api", ["US-AD104"],
                     ["default"], "Blueprint P-DOCS (publik, standalone, nav dokumen)"),
    "06c-docs-telemetry": ("Dokumentasi — skema telemetri", "/docs/telemetry", ["US-AD105"],
                           ["default"], "Blueprint P-DOCS (publik, standalone, nav dokumen)"),
    "07-pricing": ("Harga", "/pricing", ["US-AD100"], ["default"], "Blueprint P (publik, standalone)"),
    "08-changelog": ("Changelog", "/changelog", ["US-AD99"], ["default"], "Blueprint P (publik, standalone)"),
    "09-features": ("Product / fitur", "/product", ["US-AD99"], ["default"], "Blueprint P (publik, standalone)"),
    "09b-github": ("Repositori & rilis", "/github", ["US-AD101"], ["default", "no-metadata"],
                   "Blueprint P (publik, standalone)"),
    "09c-community": ("Komunitas & dukungan", "/community", ["US-AD102"], ["default", "inactive"],
                      "Blueprint P (publik, standalone)"),

    # ---- navigasi & akun ----
    "10-board-list": ("Daftar board", "/boards", ["US-AD72", "US-AD91"],
                      ["default", "empty", "loading", "from-template"],
                      "Blueprint A + cost rail"),
    "11-project-list": ("Daftar project", "/projects", ["US-AD08", "US-AD91"],
                        ["default", "empty", "create-modal"],
                        "Blueprint A + cost rail"),
    "12-onboarding": ("First-run onboarding", "/onboarding", ["US-AD92"], ["default", "step2", "done"],
                      "Blueprint A tanpa cost rail"),
    "13-notifications": ("Notifikasi", "/notifications", ["US-AD61"], ["default", "empty"],
                         "Blueprint A + cost rail"),
    "14-command-palette": ("Command palette", "(overlay)", ["US-AD55"], ["default", "no-results"],
                           "sub-shell Blueprint B: overlay di atas board"),
    "15-profile": ("Profil akun", "/settings/profile", ["US-AD89"], ["default", "error"],
                   "Blueprint C + cost rail"),
    "16-security": ("Keamanan: password + sesi aktif", "/settings/security", ["US-AD90"],
                    ["default", "error"], "Blueprint C + cost rail"),
    "17-close-account": ("Tutup akun", "/settings/close", ["US-AD98"], ["default", "blocked"],
                         "Blueprint C + cost rail"),

    # ---- board ----
    "18-kanban": ("Board kanban", "/boards/:id", ["US-AD09", "US-AD11", "US-AD12", "US-AD15",
                                                  "US-AD16", "US-AD17", "US-AD19", "US-AD32",
                                                  "US-AD39", "US-AD59", "US-AD62", "US-AD80",
                                                  "US-AD81", "US-AD82", "US-AD97"],
                  ["default", "empty", "loading", "filtered-empty"], "Blueprint B + cost rail"),
    "19-table-view": ("Tabel task", "/boards/:id?view=table", ["US-AD54", "US-AD57"],
                      ["default", "selected", "empty"], "Blueprint A + cost rail"),
    "20-task-drawer": ("Detail task (drawer)", "(drawer)", ["US-AD13", "US-AD40", "US-AD42",
                                                             "US-AD48", "US-AD59", "US-AD71",
                                                             "US-AD79", "US-AD80"],
                       ["default", "timeline-tab", "artifacts-tab"], "Blueprint C + cost rail (drawer 420px)"),
    "21-task-create": ("Buat task (modal)", "(modal)", ["US-AD11", "US-AD14", "US-AD58"],
                       ["default", "error"], "sub-shell Blueprint A: modal"),
    "22-column-editor": ("Editor kolom board", "(panel)", ["US-AD10"], ["default", "error"],
                         "Blueprint C + cost rail (panel 420px)"),
    "23-board-settings": ("Pengaturan board", "/boards/:id/settings", ["US-AD83", "US-AD84"],
                          ["default", "confirm-delete"], "Blueprint C + cost rail"),
    "24-dependency-view": ("Graf dependency", "/boards/:id/graph", ["US-AD19"], ["default"],
                           "Blueprint B + cost rail"),

    # ---- agent ----
    "25-agent-registry": ("Registry agent", "/agents", ["US-AD20", "US-AD73"], ["default", "empty"],
                          "Blueprint A + cost rail"),
    "26-agent-form": ("Formulir agent", "/agents/new", ["US-AD96", "US-AD67", "US-AD106", "US-AD108"],
                      ["default", "no-credential", "error"], "Blueprint C + cost rail"),
    "27-agent-provider-key": ("Kredensial provider agent", "(panel)", ["US-AD86", "US-AD87"],
                              ["default", "masked", "invalid"], "Blueprint C + cost rail (panel 420px)"),
    "28-agent-detail": ("Detail agent", "/agents/:id", ["US-AD67", "US-AD73"],
                        ["default", "archived"], "Blueprint A + cost rail"),
    # Skill library belum punya mockup Stitch. Sementara dipetakan ke form agent
    # karena itu permukaan tempat user memilih/mengelola skill; ganti begitu ada
    # layar tersendiri.
    "26b-agent-skills": ("Skill library agent", "(panel)", ["US-AD107"],
                         ["default", "empty", "editor"], "belum ada mockup — menyusul"),

    # ---- biaya ----
    "29-cost-overview": ("Ringkasan biaya", "/cost", ["US-AD28", "US-AD29", "US-AD30",
                                                          "US-AD31", "US-AD69"],
                         ["default", "over-budget"], "Blueprint A + cost rail"),
    "30-ledger-explorer": ("Ledger biaya", "/cost/ledger", ["US-AD27"], ["default", "empty"],
                           "Blueprint A + cost rail"),
    "31-cost-export": ("Ekspor CSV biaya", "(modal)", ["US-AD56"], ["default", "preview"], "sub-shell Blueprint A: modal"),

    # ---- run ----
    "32-run-detail": ("Detail run", "/runs/:id", ["US-AD41"], ["default", "running", "failed"],
                      "Blueprint A + cost rail"),
    "33-run-timeline": ("Timeline step", "/runs/:id/timeline", ["US-AD26"], ["default", "running"],
                        "Blueprint B + cost rail"),
    "34-step-payload": ("Detail step & payload", "(panel)", ["US-AD94"], ["default", "truncated",
                                                                          "unavailable"], "Blueprint C + cost rail (panel 420px)"),

    # ---- approval ----
    "35-approval-inbox": ("Antrean approval", "/approvals", ["US-AD37", "US-AD38"],
                          ["default", "empty"], "Blueprint A + cost rail"),
    "36-approval-detail": ("Detail approval", "/approvals/:id", ["US-AD34", "US-AD35"],
                           ["default", "expired", "rejected"], "Blueprint B + cost rail"),

    # ---- settings ----
    "37-workspace-settings": ("Pengaturan ruang kerja", "/settings/workspace", ["US-AD03", "US-AD77"],
                              ["default"], "Blueprint C + cost rail"),
    "38-members": ("Anggota & role", "/settings/members", ["US-AD04", "US-AD78"],
                   ["default", "empty"], "Blueprint C + cost rail"),
    "39-api-keys": ("API key", "/settings/api-keys", ["US-AD06"], ["default", "created", "empty"],
                    "Blueprint C + cost rail"),
    "40-webhooks": ("Webhook", "/settings/webhooks", ["US-AD52", "US-AD53"],
                    ["default", "empty", "failing"], "Blueprint C + cost rail"),
    "41-audit-log": ("Audit log", "/settings/audit", ["US-AD95"], ["default", "empty", "forbidden"],
                     "Blueprint C + cost rail"),
    "42-dashboard": ("Dashboard", "/dashboard", ["US-AD76"], ["default", "empty"],
                     "Blueprint A + cost rail"),

    # Registry provider LLM per ruang kerja (US-AD109). SENGAJA tidak di rail:
    # rail punya 6 slot tetap yang dipatok angka di shell.md, dan menambah slot
    # ke-7 berarti mengubah shell = regen 51 layar yang sudah match. Kredensial
    # juga settings-scoped, sama seperti API key dan webhook.
    "47-providers": ("Provider LLM", "/settings/providers", ["US-AD109"],
                     ["default", "empty"], "Blueprint C + cost rail"),

    # ---- state lintas layar ----
    "43-state-loading": ("State: loading (skeleton)", "(varian)", ["US-AD63"], ["default"],
                         "sub-shell Blueprint B: skeleton di shell board"),
    "44-state-empty": ("State: empty", "(varian)", ["US-AD64"], ["default"], "sub-shell Blueprint B: empty di board"),
    "45-state-error": ("State: error", "(varian)", ["US-AD65"], ["default"], "sub-shell Blueprint A: error boundary"),
    "46-mobile-board": ("Board mobile", "/m/boards/:id", ["US-AD60"], ["default"], "sub-shell Blueprint X: viewport sempit (mobile)"),
}

# --------------------------------------------------- STORY TANPA LAYAR ----
# Story yang memang tidak punya wujud visual. Alasan wajib ditulis —
# "backend" bukan tempat sampah, ini keputusan yang diaudit.
BACKEND_ONLY = {
    "US-AD05": "pencabutan sesi dari jarak jauh — aksi API; wujud visualnya adalah daftar sesi di layar 16-security",
    "US-AD07": "isolasi data antar tenant — invarian keamanan, diuji lewat test; tidak ada elemen UI",
    "US-AD18": "penyimpanan edge DAG — datanya ditampilkan di layar 24-dependency-view",
    "US-AD21": "dispatcher mengklaim task — loop server, hasilnya terlihat sebagai status running di board",
    "US-AD22": "siklus hidup run claim→finish — mesin status, wujudnya di layar 32-run-detail",
    "US-AD23": "heartbeat berkala — ping server; kegagalannya terlihat sebagai reclaim",
    "US-AD24": "reclaim task stale — pemulihan server; hasilnya terlihat di board",
    "US-AD25": "menulis step saat run — penulisan DB; dibaca di layar 33-run-timeline",
    "US-AD33": "permintaan approval dari agent — endpoint; kartunya di layar 35-approval-inbox",
    "US-AD36": "approval kedaluwarsa otomatis — pekerjaan terjadwal; state terlihat di 36-approval-detail",
    "US-AD43": "klasifikasi failure otomatis — logika server; labelnya tampil di 32-run-detail",
    "US-AD44": "retry otomatis berbasis failure_kind — logika dispatcher",
    "US-AD45": "max attempts & dead letter — kebijakan server; penanda dead-letter tampil di board",
    "US-AD46": "unggah artifact ke object storage — I/O server; daftarnya di 20-task-drawer",
    "US-AD47": "unduh artifact — endpoint streaming; tombolnya di 20-task-drawer",
    "US-AD49": "dwibahasa EN/ID — lintas seluruh layar, bukan layar tersendiri",
    "US-AD50": "deteksi string keras di CI — gerbang build, tidak punya UI",
    "US-AD51": "pencatatan audit — penulisan DB; pembacaannya di 41-audit-log",
    "US-AD66": "max runtime per task — penghentian oleh server",
    "US-AD68": "snapshot harga per version — data referensi; angkanya tampil di 30-ledger-explorer",
    "US-AD70": "proteksi siklus dependency — validasi server; galatnya muncul di 24-dependency-view",
    "US-AD74": "provider LLM down — penanganan galat server; tampil sebagai failed di board",
    "US-AD75": "penentuan workspace_kind — kolom data; terlihat sebagai jenis ruang kerja di 37-workspace-settings",
    "US-AD85": "rate limit per endpoint — middleware; tampil sebagai toast 429",
    "US-AD93": "konteks ruang kerja aktif — perilaku lintas layar (pemilih ruang kerja di top bar)",
}


def stories_from_prd():
    """Ambil semua story + judul + prioritas dari 00-PRD.md."""
    path = os.path.join(HERE, "docs", "00-PRD.md")
    text = open(path, encoding="utf-8").read()
    out = {}
    for m in re.finditer(
        r"(?m)^\*\*(US-AD(\d+))\*\*\s+—\s+(.+?)\s+`(Must|Should|Could|Won't)`\s+·\s+`(M\d)`",
        text,
    ):
        out[m.group(1)] = {"title": m.group(3), "prio": m.group(4), "ms": m.group(5)}
    return out


def build():
    stories = stories_from_prd()
    screen_of = {}
    for sid, (_t, _r, sids, _st, _n) in SCREENS.items():
        for s in sids:
            screen_of.setdefault(s, []).append(sid)

    gaps_screen = []   # story harus punya layar tapi belum ada
    gaps_backend = []  # ditandai backend tapi alasan belum ditulis
    orphans = []       # layar tanpa story
    unknown = []       # layar merujuk story yang tidak ada

    for s in stories:
        has_screen = s in screen_of
        has_reason = s in BACKEND_ONLY
        if not has_screen and not has_reason:
            gaps_screen.append(s)
        if has_screen and has_reason:
            gaps_backend.append(s)
    for sid, (_t, _r, sids, _st, _n) in SCREENS.items():
        if not sids:
            orphans.append(sid)
        for s in sids:
            if s not in stories:
                unknown.append(f"{sid}->{s}")

    return stories, screen_of, gaps_screen, gaps_backend, orphans, unknown


def write_coverage(stories, screen_of):
    # COVERAGE.md hidup di docs/, bukan root repo. Sebelumnya ditulis ke root
    # sehingga file yang dilacak (docs/COVERAGE.md) tidak pernah diperbarui dan
    # muncul salinan nyasar di root.
    path = os.path.join(HERE, "docs", "COVERAGE.md")
    n_screen = sum(1 for s in stories if s in screen_of)
    n_backend = len(stories) - n_screen
    total_frames = sum(len(st) for (_t, _r, _s, st, _n) in SCREENS.values())

    L = []
    L.append("# COVERAGE — AgentDeck: story ↔ layar ↔ prompt\n")
    L.append("Dihasilkan otomatis oleh `screens.py`. Jangan diedit tangan.\n")
    L.append("## Ringkasan\n")
    L.append("| Item | Jumlah |")
    L.append("|---|---|")
    L.append(f"| Story | {len(stories)} |")
    L.append(f"| Story dengan layar | {n_screen} |")
    L.append(f"| Story backend-only (alasan tertulis) | {n_backend} |")
    L.append(f"| Layar | {len(SCREENS)} |")
    L.append(f"| Frame state yang wajib digenerate | {total_frames} |")
    L.append("")

    L.append("## A. Story → Layar\n")
    L.append("| Story | Prioritas | MS | Judul | Layar |")
    L.append("|---|---|---|---|---|")
    for s in sorted(stories, key=lambda x: int(x.split("-")[1][2:])):
        v = stories[s]
        if s in screen_of:
            scr = ", ".join(f"`{x}`" for x in screen_of[s])
        else:
            scr = "*backend-only*"
        L.append(f"| `{s}` | {v['prio']} | {v['ms']} | {v['title']} | {scr} |")
    L.append("")

    L.append("## B. Layar → Story\n")
    L.append("| Layar | Judul | Route | Story | State wajib |")
    L.append("|---|---|---|---|---|")
    for sid in sorted(SCREENS):
        t, r, sids, st, _n = SCREENS[sid]
        L.append(f"| `{sid}` | {t} | `{r}` | {', '.join('`'+x+'`' for x in sids)} | "
                 f"{', '.join(st)} |")
    L.append("")

    L.append("## C. Story backend-only (tanpa layar, dengan alasan)\n")
    L.append("| Story | Alasan tidak punya layar |")
    L.append("|---|---|")
    for s in sorted(BACKEND_ONLY, key=lambda x: int(x.split("-")[1][2:])):
        L.append(f"| `{s}` | {BACKEND_ONLY[s]} |")
    L.append("")

    open(path, "w", encoding="utf-8", newline="").write("\n".join(L))
    return path, n_screen, n_backend, total_frames


def main(argv):
    stories, screen_of, gaps_screen, gaps_backend, orphans, unknown = build()
    path, n_screen, n_backend, frames = write_coverage(stories, screen_of)

    print(f"story            : {len(stories)}")
    print(f"  dengan layar   : {n_screen}")
    print(f"  backend-only   : {n_backend}")
    print(f"layar            : {len(SCREENS)}")
    print(f"frame state      : {frames}")
    print(f"-> {os.path.basename(path)}")

    fail = False
    if gaps_screen:
        print(f"\nGAP story tanpa layar ({len(gaps_screen)}):")
        for s in gaps_screen:
            print(f"   {s}  {stories[s]['title']}")
        fail = True
    if gaps_backend:
        print(f"\nKONFLIK: punya layar sekaligus ditandai backend ({len(gaps_backend)}): "
              + ", ".join(gaps_backend))
        fail = True
    if orphans:
        print(f"\nGAP layar tanpa story: {', '.join(orphans)}")
        fail = True
    if unknown:
        print(f"\nGAP layar merujuk story tak dikenal: {', '.join(unknown)}")
        fail = True

    if fail:
        print("\nHASIL: COVERAGE ADA GAP")
        return 1
    print("\nHASIL: COVERAGE LENGKAP (0 GAP)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
