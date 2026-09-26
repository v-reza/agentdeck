# Commit plan — sesi 2026-09-21

53 file kotor di working tree. Semua gate hijau. Belum di-commit karena user
belum minta. Ini urutan commit yang diusulkan kalau nanti di-commit.

**Jangan push ke remote publik tanpa konfirmasi user.**

---

## Commit 1 — `feat(agents): provider BYO dengan validasi alamat tanpa DNS`

Isi: form register agent BYO + endpoint probe + pisah validator.

- `cmd/api/provider_models.go` + `_test.go` (BARU) — `POST /api/v1/provider/models`,
  stateless, Admin floor, 400 user-error / 502 upstream
- `internal/provider/provider.go` — `ValidateAddressOnly` (nol DNS) vs
  `ValidateOperatorBaseURL` (resolve), `localHostAllowlist`
- `internal/board/service.go` — jalur tulis pakai `ValidateAddressOnly`
- `internal/board/pgx.go`, `internal/store/queries*` — `base_url` + `reasoning_effort`
- `cmd/api/agents_baseurl_test.go` (BARU) — 8 case ditolak, 2 localhost lolos,
  + `TestSavingAnAgentDoesNotResolveDNS`
- `cmd/api/agents_model_gate_test.go` (BARU) — US-AD67 AC1/AC2
- `frontend/src/store/api/agents.ts` — `probeProviderModels` mutation
- `frontend/src/lib/field-error.ts` (BARU), `components/ui/combobox.tsx` (BARU),
  `components/agents/` (BARU), `CreateAgentFields.tsx` (BARU)
- `CreateAgentForm.tsx`, `AgentRegistry.tsx` (`table-fixed` + `<colgroup>`),
  `StatusFilter.tsx`, `ui/input.tsx`, `ui/modal.tsx`, `lib/i18n.ts`
- `e2e/agent-form.spec.ts` (BARU), `e2e/combobox.ts` (BARU), `e2e/agents.spec.ts`,
  `e2e/agent-detail.spec.ts`
- `internal/store/queries_columns_test.go` (BARU), `store/api/agents.test.ts` (BARU)

## Commit 2 — `docs: kontrak provider registry (US-AD109, fase 0)`

- `docs/00-PRD.md` — US-AD109, 10 AC, baris traceability
- `docs/DECISIONS.md` — §6A.J (12 aturan mengikat), §6A.F dikasih pointer
- `docs/ARCHITECTURE.md` — DDL `providers`, `agents.provider_id`, §6.2.8 Providers
- `docs/CONCEPT-PROVIDER-REGISTRY.md` (BARU) — status DISETUJUI
- `design/landing/prompt-landing.md` + `source.html` — 26 tabel / 123 route

## Commit 3 — `docs(arch): §18 struktur nyata + gate drift (fase 0.5)`

- `docs/ARCHITECTURE.md` — §18 ditulis ulang (9 dir palsu → 21 dir nyata),
  §18.3 modul belum dibangun, kolom `Status` di 123 baris §6.2, §6.2.20 per modul
- `tools/verify_suite.py` — `check_structure()` + cek kolom `Status` dua arah
- `docs/HANDOFF.md` (BARU), `docs/DESIGN-INVENTORY.md` (BARU), `.hermes.md` (BARU)
- Arsip: `docs/PLAN-M0-M4.md`, `OVERNIGHT-BRIEF.md`, `OVERNIGHT-LOG.md`
  → `archive/reports/`; hapus `docs/PLAN-WAVE2.md`, `docs/.backup/`, `api.exe`
- `CHANGELOG.md`, `release-notes-v0.2.0.md` — link ke path arsip baru
- `cmd/api/agent_skills*.go` — rujukan `PLAN-WAVE2 7.1` → `ARCHITECTURE 6.2.9 + US-AD107`

## Commit 4 — `chore: env untuk master key kredensial`

- `.env.example`, `compose.yaml` — `AGENTDECK_MASTER_KEY` (opsional, API tetap
  boot tanpa itu)

---

## Yang BELUM dikerjain (jangan di-commit sebagai "selesai")

- Fase 1–6 provider registry (migrasi `0010` dan seterusnya)
- Fix design: warna status 10/10, icon Lucide, sparkline, skeleton, archived row
- Gate `index.css` vs `DESIGN.md`
