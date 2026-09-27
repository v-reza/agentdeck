# OVERNIGHT-BRIEF — AgentDeck

Kontrak kerja buat run semalaman. Dibaca **di awal tiap turn**, bukan sekali.
Kalau brief ini dan ingatan gw berbeda, brief ini yang benar.

- **Mulai:** 2026-09-27 malam (sesi `20260921_155418_6ef46c`)
- **Fase sebelumnya (endpoint §6.2):** SELESAI. **129 ✅ / 0 ⬜** — seluruh endpoint
  backend terpasang dan ter-push (`e450c48`). Brief lama (F1–F13) sudah tidak berlaku.
- **Gate:** `tools\gate-overnight.cmd` (wajib, sudah diuji dua arah)
- **Jejak:** `docs/OVERNIGHT-LOG.md` — satu entri per fase, ditulis sebelum lanjut

---

## 1. Misi malam ini

**Frontend.** Backend sudah tamat: 129/129 endpoint §6.2 terpasang, terverifikasi
lawan API nyata, ter-push. Yang tersisa **semua** di UI.

Masalah sebenarnya bukan "belum ada layar", tapi **layar yang ditulis lalu tidak
disambungkan**. Tiga contoh nyata, diverifikasi ke kode:

| Yang ada | Kenyataannya |
|---|---|
| `store/api/stream.ts` 158 baris + test | Implementasi SSE lengkap (EventSource, patch cache RTK, `Last-Event-ID`). **Nol komponen yang memakainya.** |
| `hooks/use-sse-cache.ts` | Hook siap pakai. **Nol pemanggil** — didefinisikan, tidak pernah dipasang. |
| `TaskDetailDrawer.tsx:44-46` | Komentarnya bilang Logs/Artifacts/Approvals tab "need endpoints that do not exist (`runs`, `artifacts`, `approvals`)". **Ketiganya sudah ada sekarang.** |

Jadi misi malam ini: **sambungkan yang sudah ditulis, baru bangun yang benar-benar
belum ada.**

## 2. Urutan fase (termurah dulu — yang termurah itu yang paling kelihatan)

> **Sumber progres yang SAH, sudah diverifikasi lawan kode (2026-09-27):**
> - `docs/CHECKLIST.md` → **68 PASS / 4 dikerjakan / 16 belum / 1 ditunda**
>   dari 89 story M0–M4. Dihasilkan `tools/update_checklist.py` dari
>   `tools/checklist_status.json`. **Jangan diedit tangan** — ubah sidecar, lalu
>   jalankan ulang tool-nya (`python tools/update_checklist.py`).
> - Tabel detail `docs/ARCHITECTURE.md` §6.2.1–§6.2.19 → **129 ✅ / 0 ⬜**,
>   dijaga `verify_suite.py`.
> - `docs/00-PRD.md` **bukan** pelacak progres: 387 item AC, nol tercentang.
>
> "PASS" berarti **layarnya ada filenya dan komponennya terpasang** (di router
> atau dirender induknya). **Bukan** berarti design match — itu belum diukur
> untuk semua layar.

**Yang pertama dikerjakan bukan story baru, tapi yang sudah ditulis lalu tidak
disambungkan.** Ini pola sebenarnya di repo ini:

| Ada | Kenyataannya |
|---|---|
| `store/api/stream.ts` 158 baris + test | SSE lengkap (EventSource, patch cache RTK, `Last-Event-ID`). **Nol komponen memakainya.** |
| `hooks/use-sse-cache.ts` | Hook siap. **Nol pemanggil.** |
| `settings/ApiKeys.tsx`, `settings/Webhooks.tsx` | Masing-masing 21 baris, bilang "not available yet". Backend 5 dan 7 endpoint jalan. |
| `TaskDetailDrawer.tsx` | Tab Artifacts/Logs/Approvals/assignee picker belum dibangun. Endpoint-nya semua ada. |

Maka urutannya:

| # | Fase | Story | Kenapa murah |
|---|---|---|---|
| 0 | **Pasang `useSseCache`** di kanban + drawer | US-AD39 | Hook sudah ada. Board jadi hidup tanpa refresh. Fallback fetch wajib jalan saat stream mati. |
| 1 | **Artifacts tab** di drawer | US-AD48 | 5 endpoint + presigned URL sudah terbukti lawan MinIO. Cuma UI. |
| 2 | **Logs tab** (step timeline) | US-AD26, US-AD94 | `GET /runs/{id}/events` + step payload ada. |
| 3 | **Approvals tab** di drawer | US-AD34, US-AD35 | `stream.ts` sudah punya `listApprovals`/`approveApproval`/`rejectApproval`. |
| 4 | **Approval detail** `36-approval-detail` | US-AD34, US-AD35 | Layar belum ada; datanya sudah. |
| 5 | **Run detail** `32-run-detail` | US-AD41 | `GET /runs/{id}` + `/summary` ada. |
| 6 | **Notifications** `13-notifications` | US-AD61 | Backend notifikasi ada sejak F9. |
| 7 | **Webhooks tab** (isi stub) | US-AD52, US-AD53 | Backend 7 endpoint jalan sejak F12. Admin-only; secret tampil sekali. |
| 8 | **API Keys tab** (isi stub) | US-AD06 | Backend 5 endpoint jalan sejak F10. Key penuh tampil sekali. |
| 9 | **Dashboard** `42-dashboard` | US-AD76 | — |
| 10 | **Dependency view** `24-dependency-view` | US-AD19 | DAG endpoint ada. |
| 11 | **State screens** `43`/`44`/`45` | US-AD63, US-AD64, US-AD65 | Skeleton, empty, error boundary. Kecil-kecil. |
| 12 | **Skill library** `26b-agent-skills` | US-AD107 | Backend ada. |
| 13 | **Security** `16-security` | US-AD90 | Ganti password + sesi aktif; endpoint ada. |
| 14 | **Mobile board** `46-mobile-board` | US-AD60 | — |
| 15 | **Ledger explorer** `30-ledger-explorer` | US-AD27 | — |
| 16 | **Rate limit** | US-AD85 | **Backend-only, benar-benar kosong** — nol rate limiter di repo. |
| 17 | **Deteksi string keras di CI** | US-AD50 | **Backend-only, benar-benar kosong** — nol workflow CI. |

Kalau fase 0–5 kelar, itu hasil yang bagus. Jangan mulai fase baru sebelum yang
lama ter-push.

## 3. Loop tiap fase

1. Baca kontraknya dulu: design `design/stitch-output/v2/*.html` (layout = sumber
   kebenaran, **teks mockup jangan dibawa ke produk**) + `docs/DESIGN.md` (token).
2. Tulis plan singkat di `docs/OVERNIGHT-LOG.md` (fase, layar, file, risiko).
3. RTK Query slice di `frontend/src/store/api/`. Server state **hanya** lewat RTK
   Query; Redux Toolkit = satu-satunya state management.
4. Bahasa: **`en` DAN `id` sekaligus** atau tidak compile. `t['key']`, bukan `t('key')`.
5. Ukur DOM / screenshot **elemen** buat kerjaan layout. **Nol unit test buat CSS.**
6. `tsc -b` + `prettier --check src/` tiap edit (gate cepat).
7. Commit + push + tulis hasilnya di `docs/OVERNIGHT-LOG.md`.

## 4. Kontrak gate (terukur, bukan tebakan)

`tools\gate-overnight.cmd` = `gofmt -l .` → `go build ./...` → `tsc -b` →
`vitest run` → `prettier --check src/`. Terukur **20–52 detik**, di bawah plafon
300 detik. Sudah diuji dua arah (kotor → 1, bersih → 0).

**`go test ./...` DIKELUARKAN dari gate, dan itu disengaja** — terukur 490 detik,
mustahil masuk plafon 300. Konsekuensinya: gate hijau **tidak** berarti test Go
hijau. Kalau sebuah fase menyentuh Go, jalankan `go test` paket itu **manual** dan
tulis hasilnya di log. Jangan pernah bilang "gate hijau jadi aman".

`prettier` di-scope ke `src/` — `prettier --check .` merah karena file milik sesi
lain (`frontend/e2e/demo/measure-aksi.mjs`).

## 5. Aturan keras

- **Jangan** nyalain dispatcher AgentDeck (`AGENTDECK_DISPAT`). Keputusan user: OFF.
- **Jangan** sentuh board kanban, `tools/gate.cmd`, `frontend/e2e/demo/`,
  `tools/demo/`, atau `.gitignore`. Itu milik sesi lain.
- **Jangan** commit: `node_modules`, build output, cache, screenshot sementara, secrets.
- Kredensial/token/connection string → `[REDACTED]`. Nol nomor WA/JID/chatId di file.
- **Jargon spec (`US-AD`, `AC`, `M1`–`M6`, `HTTP \d{3}`) DILARANG masuk UI yang
  dirender.** Komentar kode boleh. `/github` pengecualian.
- **Combobox** buat semua select — token design wajib, bukan `<select>` bawaan browser.
- **Modal**: role/ARIA, focus trap, Escape, scroll lock, return focus, ikut `DESIGN.md`.
- Icon pakai **Lucide**.
- `git ls-remote origin refs/heads/main` = satu-satunya bukti push yang sah.
- Edit lewat python bisa ngubah line-ending → repo ini LF. Cek `file <path>` setelahnya.
- `curl` di MSYS itu program Windows native: `-o /tmp/x` nulis ke `C:\tmp\`. Pakai path native.

## 6. Kapan berhenti

- Fase 1–8 kelar dan masih ada waktu → lanjut, tetap satu commit per fase.
- Ada blocker yang butuh keputusan produk → **stop**, tulis di log, jangan ngarang
  jawaban. Kontrak yang bertabrakan: sebut konfliknya, pakai keputusan terbaru.
- Gate merah dan nggak kelar dalam 3 percobaan → stop, tulis apa adanya.
- Kehabisan turn (`goals.max_turns`) → goal auto-pause; watchdog yang ngabarin.

## 7. Yang TIDAK dijamin malam ini

- Semua 22 layar yang belum ada. Realistis **3–6 fase** dengan disiplin kontrak.
- Full e2e Playwright (butuh ~5 menit dan Docker hidup; bukan gate).
- Design match penuh tiap layar — yang wajib: **inventory elemen** (ada / beda /
  **ilang**), dan yang ilang disebut, bukan didiemin (`docs/DESIGN-INVENTORY.md`).
