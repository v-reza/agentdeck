#!/usr/bin/env python3
"""Gate frontend AgentDeck. Jalankan dari root repo.

Cek hal-hal yang pernah lolos dan bikin pelanggaran kontrak:
  A. ROUTER   — nol router manual (window.location.pathname / usePathname sendiri);
                React Router v7 wajib dipakai sesuai ARCHITECTURE.md 18.2
  B. NAV      — navigasi internal wajib <Link to>; <a href="/..."> internal dilarang
                (bikin full document reload + mematikan React state)
  C. DEP      — dependency kontrak ada di frontend/package.json, dan stack yang
                dilarang kontrak tidak muncul
  D. FORMAT   — prettier --check bersih (setara gofmt -l)
  E. BANNED   — window.alert/confirm/prompt dilarang
  F. SPA      — vercel.json punya SPA fallback
  G. STRUCT   — struktur folder ARCHITECTURE.md 18.2 benar-benar ada, dan
                server state tidak diambil dengan fetch manual
  H. REACT19  — primitif memakai ref-as-prop dan <Context value> langsung
                (nol forwardRef, nol .Provider), sesuai ARCHITECTURE.md 18.2

Exit 0 kalau bersih, 1 kalau ada temuan FAIL.

ponytail: cek berbasis teks, bukan AST. Regex cukup buat menangkap pola
pelanggaran yang nyata terjadi; AST parser = dependency baru tanpa manfaat.
"""

import json
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WEB = os.path.join(ROOT, "frontend")
SRC = os.path.join(WEB, "src")

FAILED = []
WARNED = []


def fail(gate, msg):
    FAILED.append(f"{gate}: {msg}")


def warn(gate, msg):
    WARNED.append(f"{gate}: {msg}")


def strip_comments(text):
    """Buang komentar supaya cek kode tidak kena prosa.

    Gate ini pernah gagal hanya karena sebuah komentar yang menjelaskan bahwa
    `forwardRef` TIDAK dipakai. Cek harus membaca kode, bukan dokumentasi kode.
    ponytail: cukup buang //... dan /*...*/; string yang berisi '//' (URL) tidak
    dibuang, dan itu tidak masalah karena tidak ada pola terlarang di dalam URL.
    """
    text = re.sub(r"/\*.*?\*/", "", text, flags=re.S)
    text = re.sub(r"//[^\n]*", "", text)
    return text


def walk_sources():
    """Yield (relpath, text) untuk setiap .ts/.tsx/.css di frontend/src."""
    for dirpath, dirnames, filenames in os.walk(SRC):
        dirnames[:] = [d for d in dirnames if d != "node_modules"]
        for name in filenames:
            if not name.endswith((".ts", ".tsx", ".css")):
                continue
            path = os.path.join(dirpath, name)
            with open(path, encoding="utf-8") as handle:
                yield os.path.relpath(path, SRC), handle.read()


# --- A. Router manual -------------------------------------------------------
def gate_router():
    hand_rolled = []
    for rel, text in walk_sources():
        if rel.endswith(".css"):
            continue
        # File router buatan sendiri = pelanggaran langsung.
        if re.search(r"\b(usePathname|usePathnameRouter)\s*\(", text) and "react-router" not in text:
            hand_rolled.append(rel)
        # Membaca pathname mentah untuk routing juga pelanggaran.
        if "window.location.pathname" in text and rel != "main.tsx":
            hand_rolled.append(rel)

    if os.path.exists(os.path.join(SRC, "lib", "router.tsx")):
        fail("ROUTER", "src/lib/router.tsx ada — router manual harus dihapus (pakai react-router-dom)")

    if hand_rolled:
        for rel in sorted(set(hand_rolled)):
            fail("ROUTER", f"{rel} memakai routing manual; pakai react-router-dom")

    if not FAILED:
        print("ok    ROUTER: nol router manual, React Router dipakai")


# --- B. Navigasi internal ---------------------------------------------------
def gate_nav():
    internal = []
    external_ok = 0

    for rel, text in walk_sources():
        if rel.endswith(".css"):
            continue
        for match in re.finditer(r"<a\s+[^>]*?href=(?:\"([^\"]*)\"|\{([^}]*)\})", text, re.S):
            literal, expr = match.group(1), match.group(2)
            target = literal if literal is not None else (expr or "")
            line = text[: match.start()].count("\n") + 1

            # Link internal = diawali '/' tapi bukan '//'.
            is_internal = bool(re.match(r"^\s*['\"]?/", target)) and not target.startswith("//")
            if is_internal:
                internal.append(f"{rel}:{line} -> {target[:60]}")
            else:
                external_ok += 1

    if internal:
        for item in internal:
            fail("NAV", f"<a href> internal bikin full reload; pakai <Link to> — {item}")
    else:
        print(f"ok    NAV: nol <a href> internal ({external_ok} link eksternal, benar)")


# --- C. Dependency kontrak --------------------------------------------------
# ARCHITECTURE.md 18.2 "Frontend": React 19 + Vite, TypeScript, Tailwind v4,
# shadcn/ui, Redux Toolkit + RTK Query, dnd-kit, Vitest + Playwright.
REQUIRED_DEPS = (
    "react-router-dom",
    "tailwindcss",
    "@tailwindcss/vite",
    "@reduxjs/toolkit",
    "react-redux",
    "@dnd-kit/core",
    "@dnd-kit/sortable",
    "class-variance-authority",
    "clsx",
    "tailwind-merge",
    "vitest",
    "@playwright/test",
)

# DECISIONS.md 5: "Redux Toolkit = satu-satunya manajemen state. Tidak ada
# TanStack Query, tidak ada Zustand, tidak ada Context untuk data aplikasi."
BANNED_DEPS = ("@tanstack/react-query", "zustand", "jotai", "recoil", "swr")


def gate_deps():
    path = os.path.join(WEB, "package.json")
    if not os.path.exists(path):
        fail("DEP", "frontend/package.json tidak ada")
        return

    with open(path, encoding="utf-8") as handle:
        pkg = json.load(handle)
    deps = {**pkg.get("dependencies", {}), **pkg.get("devDependencies", {})}

    missing = [name for name in REQUIRED_DEPS if name not in deps]
    for name in missing:
        fail("DEP", f"{name} hilang dari package.json (kontrak ARCHITECTURE.md 18.2)")
    if not missing:
        print(f"ok    DEP: {len(REQUIRED_DEPS)} dependency kontrak terpasang")

    for banned in BANNED_DEPS:
        if banned in deps:
            fail("DEP", f"{banned} terlarang (DECISIONS.md 5: Redux Toolkit satu-satunya state)")


# --- D. Formatting ----------------------------------------------------------
def gate_format():
    # Di Windows, node_modules/.bin/prettier adalah shell script, bukan exe —
    # jalankan entry JS-nya langsung lewat node supaya portable.
    entry = os.path.join(WEB, "node_modules", "prettier", "bin", "prettier.cjs")
    if not os.path.exists(entry):
        warn("FORMAT", "prettier belum terpasang; skip cek format")
        return

    result = subprocess.run(
        ["node", entry, "--check", "."],
        cwd=WEB,
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        fail("FORMAT", "prettier --check gagal (setara gofmt -l tidak kosong)")
        for line in (result.stdout + result.stderr).splitlines():
            if line.strip().startswith("[warn]"):
                print(f"      {line.strip()}")
    else:
        print("ok    FORMAT: prettier bersih")


# --- E. API terlarang -------------------------------------------------------
# Komentar sering MENYEBUT API terlarang ("bukan window.confirm(), pakai Modal").
# Scan teks mentah bikin gate nangkap prosa sendiri, jadi komentar dibuang dulu.
# `(?<!:)//` melindungi "https://" di string literal.
_COMMENT_BLOCK = re.compile(r"/\*.*?\*/", re.S)
_COMMENT_LINE = re.compile(r"(?<!:)//[^\n]*")


def strip_comments(text):
    return _COMMENT_LINE.sub("", _COMMENT_BLOCK.sub("", text))


def gate_banned():
    found = []
    for rel, text in walk_sources():
        if rel.endswith(".css"):
            continue
        code = strip_comments(text)
        for call in ("window.alert", "window.confirm", "window.prompt"):
            if call in code:
                found.append(f"{rel} -> {call}")
    if found:
        for item in found:
            fail("BANNED", item)
    else:
        print("ok    BANNED: nol window.alert/confirm/prompt")


# --- F. SPA fallback --------------------------------------------------------
def gate_spa_fallback():
    """BrowserRouter butuh rewrite semua path ke index.html.

    Tanpa ini, deep link (/github) jadi 404 di hosting statis: server nyari
    file fisik bernama 'github' dan gak ketemu. Pernah kejadian di Vercel.
    """
    path = os.path.join(WEB, "vercel.json")
    if not os.path.exists(path):
        fail("SPA", "frontend/vercel.json hilang — deep link bakal 404 di Vercel")
        return

    with open(path, encoding="utf-8") as handle:
        try:
            config = json.load(handle)
        except json.JSONDecodeError as exc:
            fail("SPA", f"vercel.json bukan JSON valid: {exc}")
            return

    rewrites = config.get("rewrites") or []
    to_index = [
        rule
        for rule in rewrites
        if str(rule.get("destination", "")).rstrip("/").endswith("index.html")
        or str(rule.get("destination", "")) == "/"
    ]
    if not to_index:
        fail("SPA", "vercel.json tidak punya rewrite ke /index.html — deep link 404")
        return

    # Rewrite harus menangkap SEMUA path; kalau cuma '/' maka deep link tetap 404.
    source = str(to_index[0].get("source", ""))
    if source not in ("/(.*)", "/:path*", "/(.*)/"):
        warn("SPA", f"rewrite source '{source}' mungkin tidak menangkap semua deep link")

    print(f"ok    SPA: vercel.json punya SPA fallback ({source} -> /index.html)")


# --- G. Struktur ARCHITECTURE.md 18.2 ---------------------------------------
# File yang kontrak sebut by name. Kalau salah satu hilang, struktur sudah
# menyimpang dari dokumen — dan itu persis penyebab refactor ulang sebelumnya.
REQUIRED_PATHS = (
    "main.tsx",
    "App.tsx",
    "app/router.tsx",
    "routes/auth/Login.tsx",
    "routes/auth/Register.tsx",
    "routes/dashboard/Layout.tsx",
    "routes/dashboard/boards/BoardList.tsx",
    "routes/dashboard/boards/KanbanBoard.tsx",
    "routes/dashboard/boards/TableView.tsx",
    "routes/dashboard/boards/TaskDetailDrawer.tsx",
    "routes/dashboard/boards/BoardSettings.tsx",
    "routes/dashboard/approvals/ApprovalInbox.tsx",
    "routes/dashboard/agents/AgentRegistry.tsx",
    "routes/dashboard/agents/AgentDetail.tsx",
    "routes/dashboard/finops/CostOverview.tsx",
    "routes/dashboard/finops/LedgerExplorer.tsx",
    "routes/dashboard/settings/Members.tsx",
    "routes/dashboard/settings/ApiKeys.tsx",
    "routes/dashboard/settings/Webhooks.tsx",
    "store/index.ts",
    "store/hooks.ts",
    "store/api/base.ts",
    "store/api/boards.ts",
    "store/api/agents.ts",
    "store/api/finops.ts",
    "store/api/stream.ts",
    "store/slices/uiSlice.ts",
    "store/slices/langSlice.ts",
    "store/slices/sessionSlice.ts",
    "components/ui",
    "components/kanban",
    "components/approvals",
    "components/terminal",
    "components/layout",
    "hooks/use-optimistic-card.ts",
    "hooks/use-action-form.ts",
    "hooks/use-sse-cache.ts",
    "lib/formatters.ts",
)


def gate_structure():
    missing = [
        rel for rel in REQUIRED_PATHS if not os.path.exists(os.path.join(SRC, *rel.split("/")))
    ]
    if missing:
        for rel in missing:
            fail("STRUCT", f"frontend/src/{rel} hilang (ARCHITECTURE.md 18.2)")
    else:
        print(f"ok    STRUCT: {len(REQUIRED_PATHS)} path kontrak 18.2 ada")

    # Server state wajib lewat RTK Query. fetch manual di luar store/api adalah
    # pelanggaran yang sudah pernah terjadi dan bikin dua sumber kebenaran.
    manual = []
    for rel, text in walk_sources():
        if rel.endswith(".css"):
            continue
        if rel.replace(os.sep, "/").startswith("store/api/"):
            continue
        if re.search(r"\bfetch\s*\(", text) or re.search(r"from\s+['\"]axios['\"]", text):
            manual.append(rel)
    if manual:
        for rel in sorted(set(manual)):
            fail("STRUCT", f"{rel} memanggil fetch/axios langsung; server state wajib RTK Query")
    else:
        print("ok    STRUCT: nol fetch/axios manual di luar store/api")

    # God component: satu file route yang menampung seluruh shell + data.
    for rel, text in walk_sources():
        if not rel.endswith(".tsx"):
            continue
        lines = text.count("\n") + 1
        if lines > 400:
            warn("STRUCT", f"{rel} {lines} baris — pecah jadi komponen (hindari god component)")


# --- H. React 19 idioms -----------------------------------------------------
def gate_react19():
    """ARCHITECTURE.md 18.2: primitif shadcn menerima `ref` langsung, nol
    forwardRef; provider ditulis `<X value>` tanpa `.Provider`.
    """
    offenders = []
    for rel, text in walk_sources():
        if rel.endswith(".css"):
            continue
        code = strip_comments(text)
        if "forwardRef" in code:
            offenders.append(f"{rel} -> forwardRef (pakai ref as a prop)")
        if re.search(r"\.Provider\b", code):
            offenders.append(f"{rel} -> .Provider (pakai <Context value>)")

    if offenders:
        for item in offenders:
            fail("REACT19", item)
    else:
        print("ok    REACT19: nol forwardRef, nol .Provider")


def main():
    if not os.path.isdir(SRC):
        print(f"FAIL: {SRC} tidak ada — jalankan dari root repo agentdeck")
        return 1

    print(f"== AgentDeck frontend gate  ({ROOT})\n")
    gate_router()
    gate_nav()
    gate_deps()
    gate_format()
    gate_banned()
    gate_spa_fallback()
    gate_structure()
    gate_react19()

    print()
    for item in WARNED:
        print(f"warn  {item}")

    if FAILED:
        print()
        for item in FAILED:
            print(f"FAIL  {item}")
        print(f"\nHASIL: {len(FAILED)} FAIL, {len(WARNED)} warn")
        return 1

    print(f"\nHASIL: SEMUA GATE BERSIH (0 FAIL, {len(WARNED)} warn)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
