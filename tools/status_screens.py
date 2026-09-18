#!/usr/bin/env python3
"""Papan status generasi layar AgentDeck.

Menampilkan, untuk tiap layar di screens.py: apakah HTML dan PNG sudah ada,
dan apakah HTML-nya lolos gate kontrak (verify_render).

Usage:  python status_screens.py
Exit 0 kalau semua layar lengkap + lolos gate, 1 kalau masih ada yang kurang.
"""

import importlib
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import screens as S          # noqa: E402
import verify_render         # noqa: E402

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "design", "stitch-output", "v2")


def _ensure_pillow():
    """Pastikan Pillow ada; kalau tidak, coba pasang sekali.

    ponytail: gate severity butuh Pillow. `python` di shell dan `python` di kernel
    bisa interpreter berbeda, jadi modul ini memastikan sendiri alih-alih
    mengasumsikan lingkungan pemanggil.
    """
    try:
        import PIL  # noqa: F401
        return True
    except Exception:
        pass
    try:
        import subprocess as _sp
        _sp.run([sys.executable, "-m", "pip", "install", "--quiet", "pillow"],
                capture_output=True, timeout=180)
        import PIL  # noqa: F401
        return True
    except Exception:
        return False


def main():
    _ensure_pillow()
    rows = []
    for sid in sorted(S.SCREENS):
        title, route = S.SCREENS[sid][0], S.SCREENS[sid][1]
        h = os.path.join(OUT, f"{sid}.html")
        p = os.path.join(OUT, f"{sid}.png")
        has_h, has_p = os.path.exists(h), os.path.exists(p)
        gate = "-"
        if has_h:
            # severity-aware: cari render PNG supaya drift 0px tidak dihitung gagal
            stem = h[:-5]
            render = next((c for c in (stem + "-render.png", stem + ".png")
                           if os.path.exists(c)), None)
            probs, _warns = verify_render.check(h, render=render)
            gate = "FAIL" if probs else "PASS"
        rows.append((sid, title, route, has_h, has_p, gate))

    done = [r for r in rows if r[3] and r[4] and r[5] == "PASS"]
    partial = [r for r in rows if r not in done]

    print(f"{'LAYAR':34} {'HTML':5} {'PNG':5} {'GATE':5} JUDUL")
    print("-" * 88)
    for sid, title, route, h, p, gate in rows:
        mark = "ok" if (h and p and gate == "PASS") else ".."
        print(f"{mark} {sid:32} {'y' if h else '-':5} {'y' if p else '-':5} {gate:5} {title[:34]}")

    print(f"\nselesai + lolos gate : {len(done)}/{len(rows)}")
    if partial:
        print(f"belum selesai        : {len(partial)}")
        for sid, title, _r, h, p, gate in partial[:12]:
            miss = []
            if not h:
                miss.append("html")
            if not p:
                miss.append("png")
            if h and gate != "PASS":
                miss.append("gate")
            print(f"   {sid:32} kurang: {', '.join(miss)}")
        if len(partial) > 12:
            print(f"   ... dan {len(partial) - 12} lagi")

    print()
    if partial:
        print("HASIL: BELUM LENGKAP")
        return 1
    print("HASIL: SEMUA LAYAR LENGKAP DAN LOLOS GATE")
    return 0


if __name__ == "__main__":
    sys.exit(main())
