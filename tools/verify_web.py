#!/usr/bin/env python3
"""Gate frontend AgentDeck. Jalankan dari root repo.

Cek hal-hal yang pernah lolos dan bikin pelanggaran kontrak:
  A. ROUTER   — nol router manual (window.location.pathname / usePathname sendiri);
                React Router v7 wajib dipakai sesuai ARCHITECTURE.md:134
  B. NAV      — navigasi internal wajib <Link to>; <a href="/..."> internal dilarang
                (bikin full document reload + mematikan React state)
  C. DEP      — dependency kontrak ada di apps/web/package.json
  D. FORMAT   — prettier --check bersih (setara gofmt -l)
  E. BANNED   — window.alert/confirm/prompt dilarang

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
WEB = os.path.join(ROOT, "apps", "web")
SRC = os.path.join(WEB, "src")

FAILED = []
WARNED = []


def fail(gate, msg):
    FAILED.append(f"{gate}: {msg}")


def warn(gate, msg):
    WARNED.append(f"{gate}: {msg}")


def walk_sources():
    """Yield (relpath, text) untuk setiap .ts/.tsx/.css di apps/web/src."""
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
def gate_deps():
    path = os.path.join(WEB, "package.json")
    if not os.path.exists(path):
        fail("DEP", "apps/web/package.json tidak ada")
        return

    with open(path, encoding="utf-8") as handle:
        pkg = json.load(handle)
    deps = {**pkg.get("dependencies", {}), **pkg.get("devDependencies", {})}

    # Wajib: sudah dipakai sekarang.
    for name in ("react-router-dom",):
        if name not in deps:
            fail("DEP", f"{name} hilang dari package.json (kontrak ARCHITECTURE.md:134)")
        else:
            print(f"ok    DEP: {name}@{deps[name]} terpasang")

    # Terlarang: DECISIONS.md melarang TanStack Query, Zustand, Context untuk data.
    for banned in ("@tanstack/react-query", "zustand"):
        if banned in deps:
            fail("DEP", f"{banned} terlarang (DECISIONS.md: Redux Toolkit satu-satunya state)")

    # Ditunda atas keputusan user 19 Sep: jangan migrasi tanpa izin.
    for deferred in ("tailwindcss", "tailwindcss-v4"):
        if deferred in deps:
            warn("DEP", f"{deferred} terpasang — migrasi Tailwind masih DITUNDA (keputusan user)")


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
def gate_banned():
    found = []
    for rel, text in walk_sources():
        if rel.endswith(".css"):
            continue
        for call in ("window.alert", "window.confirm", "window.prompt"):
            if call in text:
                found.append(f"{rel} -> {call}")
    if found:
        for item in found:
            fail("BANNED", item)
    else:
        print("ok    BANNED: nol window.alert/confirm/prompt")


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
