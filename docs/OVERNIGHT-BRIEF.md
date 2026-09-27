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

> Sumber progres yang SAH: tabel detail `docs/ARCHITECTURE.md` §6.2.1–§6.2.19
> (dijaga `verify_suite.py`, sekarang **129 ✅ / 0 ⬜**) dan `tools/checklist_status.json`
> (17 entri dilacak).
> **`docs/CHECKLIST.md` BASI** — bilang 13 PASS dari 89 story, di-generate saat
> backend baru segelintir. Jangan dipakai buat nentuin prioritas. Angka AC di
> `docs/00-PRD.md` juga nol yang tercentang, jadi bukan pelacak progres.

| # | Fase | Kenapa murah | Gate |
|---|---|---|---|
| 1 | **Artifacts tab** di `TaskDetailDrawer` | 5 endpoint + presigned URL sudah jalan & terbukti. Cuma perlu UI + RTK Query slice baru. | Klaim komentar basi di baris 44 dibuang. |
| 2 | **Logs tab** (step timeline) | `GET /api/v1/runs/{id}/events` + `GET /tasks/{id}/runs` sudah ada. Drawer sudah punya sebagian timeline. | Beda dari tab Timeline yang sudah ada? Kalau sama, **jangan** bikin duplikat — sebut di log. |
| 3 | **Approvals tab** di drawer | `stream.ts` **sudah punya** `listApprovals`/`approveApproval`/`rejectApproval`. Tinggal dipakai. | Approve/reject = Admin. Member jangan dikasih tombol yang bakal 403. |
| 4 | **SSE dipasang di kanban + drawer** | `useSseCache` tinggal dipanggil. Ini yang bikin board hidup tanpa refresh. | Board harus tetap benar saat stream mati (fallback fetch). |
| 5 | **Webhooks tab** (`settings/Webhooks.tsx`) | Stub 21 baris bilang "not available yet". Backend 7 endpoint jalan. | Admin-only; secret cuma tampil sekali saat dibuat. |
| 6 | **API Keys tab** (`settings/ApiKeys.tsx`) | Stub 21 baris, alasan sama. Backend F10 jalan. | Key penuh cuma tampil sekali. |
| 7 | **Assignee picker** di drawer | `POST /tasks/{id}/assign` ada. | — |
| 8 | **Task archived** (US-AD59) | Row archived = strikethrough + bg abu + border kiri (`.hermes.md`). | Cocokkan ke design, bukan dikarang. |

Sisa sesudah itu (belum ada filenya sama sekali, lebih besar): run detail
(`32-run-detail`), run timeline (`33-run-timeline`), step payload (`34-step-payload`),
approval detail (`36-approval-detail`), audit log (`41-audit-log`), ledger explorer
(`30-ledger-explorer`), notifications (`13-notifications`), dashboard (`42-dashboard`),
state loading/empty/error (`43`/`44`/`45`), mobile board (`46-mobile-board`).

**22 dari 52 layar belum ada filenya** (diukur dari `docs/COVERAGE.md` lawan
`frontend/src`). Daftar lengkapnya di `docs/OPEN-ISSUES.md` F13.

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
