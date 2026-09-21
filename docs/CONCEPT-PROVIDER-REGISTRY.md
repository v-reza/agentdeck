# Konsep: Provider Registry

Status: **DISETUJUI 2026-09-21.** Keputusan mengikatnya sekarang ada di
`DECISIONS.md` §6A.J, ceritanya di `00-PRD.md` US-AD109, dan schema/endpoint-nya di
`ARCHITECTURE.md` §3 + §6.2.8. Dokumen ini jadi catatan alasan — kalau ada
pertentangan, tiga dokumen kontrak itu yang menang.

---

## Masalahnya

Hari ini kredensial nempel **per-agent**: `agents.base_url` + API key
terenkripsi per baris agent. Efeknya, operator yang punya 10 agent di provider
yang sama **nyalin base URL dan API key 10 kali**. Ganti key sekali = harus
ngulang 10 kali.

Di DB dev sekarang: **1.023 agent**. 963 di antaranya `openai` tanpa base URL,
47 `openai_compatible` yang semuanya punya base URL, 10 `anthropic`, 2 `google`,
1 `deepseek`. Dari 47 yang BYO itu, **15 menyimpan key yang sama persis di 15
baris terpisah** — itu 15 tempat key yang sama disimpen sendiri-sendiri.

## Yang dibangun

**Provider jadi entitas sendiri.** Didaftarkan sekali per ruang kerja, dipakai
berkali-kali.

- **Nav baru**, sejajar `All Projects` / `Agent Registry`.
- Nama di UI: **Provider**.
- **Kosong total** — nggak ada provider bawaan. User ngetik sendiri base URL +
  API key, dan **uji kredensial di situ juga**. Nggak ada template OpenAI/
  Anthropic yang tinggal diisi key.
- Isi satu provider: nama, base URL, API key, **daftar model hasil tarik**,
  `last_verified_at`.

## Jawaban: gimana ngecek base URL + API key valid

**`GET {base_url}/models` TIDAK CUKUP.** Ini kesalahan pertama gw, dan ketauan
dari tes, bukan dari teori:

| | `GET /v1/models` | `POST /v1/chat/completions` |
|---|---|---|
| OpenAI, tanpa key | **401** | — |
| 9Router (lokal), tanpa key | **200** | — |
| 9Router (lokal), key ngaco | **200** | **401** |

Jadi ada provider yang **ngabaikan auth di `/models`** tapi **ngecek auth di
inference**. `/models` cuma membuktikan **nyala/nggak**, bukan **key-nya
bener/nggak**. Kredensial yang salah akan lolos.

### Yang bener: dua panggilan, dua tugas

**1. `GET {base_url}/models`** → **discovery**. Isi dropdown model. Kalau dia
balas `401`, itu sinyal bonus, bukan sinyal utama.

**2. `POST {base_url}/chat/completions`** dengan `max_tokens: 1` →
**satu-satunya** bukti kredensial valid. Ini juga cara resmi yang dipakai
orang buat ngecek key OpenAI: bikin panggilan API minimal, lihat lolos/nggak.
Biaya ~1 token — nggak ada artinya dibanding nilai kepastiannya.

Yang **jangan** dipakai buat tes: panggilan chat yang beneran (boros), atau
`/models` sendirian (nggak membuktikan apa-apa soal key).

### Selisih hasilnya justru informatif

| `/models` | `/chat/completions` | Artinya |
|---|---|---|
| 200 | 200 | Beres. Key valid, daftar model kekumpul. |
| 200 | 401 | **Key salah** — persis perilaku 9Router di tabel atas |
| 401 | — | Key salah, atau endpoint `/models` nggak ada |
| error koneksi | — | base URL nggak kejangkau |

**UI cuma boleh bilang "Terhubung" kalau probe chat lolos.** `/models` sukses
sendirian bukan bukti apa-apa.

**Catatan penting:** panggilan `chat/completions` **belum ada sama sekali** di
kode (`grep chat/completions` → nol). Runtime-nya belum pernah manggil LLM.
Jadi "cek pas agent mau jalan" yang dipilih di bawah **belum ada
mekanismenya** — itu bagian milestone runtime, bukan pekerjaan Provider
Registry. Buat sekarang indikatornya cuma `last_verified_at` dari tes manual.

**Guard SSRF tetap jalan** di kedua panggilan. `ValidateOperatorBaseURL`
dipanggil sebelum nembak — `localhost` dan `127.0.0.1` boleh (provider lokal),
alias loopback dan RFC1918 tetap ditolak.

**Jalur tulis nggak boleh resolve DNS.** Nyimpen provider/agent pakai
`ValidateAddressOnly` (teks + IP literal, nol DNS); yang resolve cuma jalur yang
beneran nyambung. Kalau nggak, nyimpen agent ditolak `400` cuma gara-gara host
provider lagi down atau di balik VPN. Trade-off: hostname yang resolve ke alamat
terblokir (DNS rebinding) nggak ketangkep di write path, tapi ketangkep di tiap
jalur yang mendial. Lihat `DECISIONS.md` §6A.F.

## Bentuk data

DDL lengkapnya di `ARCHITECTURE.md` §3. Ringkasnya:

```
providers
  id, org_id, name, protocol, base_url, api_key_enc,
  models_json, models_fetched_at, last_verified_at, is_default, created_at
```

`protocol` wajib ada sejak migrasi pertama (lihat `DECISIONS.md` §6A.J) —
tiga nilai, tapi baru `openai_compatible` yang diimplementasi.

`agents.provider` (string) **tetap ada**, tapi **artinya berubah**: dulu dia
nama vendor sekaligus kunci tabel harga, sekarang dia **protokol/dialect** yang
**diturunkan dari `providers.protocol`**, bukan diketik user. Gate US-AD67
berubah arti, bukan hilang — lihat `DECISIONS.md` §6A.J.

`agents.base_url` jadi **redundan** — diambil dari provider.

**Constraint yang harus hilang:**
```sql
agents_base_url_chk: (provider = 'openai_compatible') = (base_url IS NOT NULL)
```
Constraint ini ngelock base_url cuma buat satu jenis provider. Begitu base URL
pindah ke provider, dia nggak ada artinya lagi.

## Aturan yang udah diputusin

| Pertanyaan | Keputusan |
|---|---|
| Edit provider (ganti key/base URL) | **Ikut berubah** — agent yang pakai dia langsung ke-update. Nggak nyimpen nilai sendiri. |
| Provider masih dipakai agent, boleh dihapus? | **Nggak boleh.** Ditolak, sebut agent mana yang pakai. |
| Siapa yang boleh bikin/edit | **Admin/owner aja.** Key dibagi se-org — ini soal keamanan. |
| Daftar model | **Di-cache**, refresh otomatis kalau hasil tarik > **24 jam**, plus tombol refresh manual. |
| Form nambah agent | **Cuma pilih provider + model.** base_url/key/probe pindah ke halaman Provider. |
| Kalau provider di-edit bikin agent rusak | Nampilin **"terakhir diverifikasi"** + peringatan. |
| Kapan "agent rusak" ketahuan | **Pas agent mau jalan.** Nol biaya, ketahuan telat. Mekanismenya **belum ada** — itu milestone runtime. |
| Provider default | **Satu per ruang kerja.** Form agent milih itu kalau ada. Default dihapus → jadi kosong, bukan error. |
| Agent ganti provider | **Boleh.** Pilihan model **dikosongkan** — model provider lama belum tentu ada di provider baru. |
| Provider bawaan/template | **Kosong total.** Nggak ada template OpenAI/Anthropic siap pakai. |
| Probe kredensial kapan jalan | **Cuma waktu tombol "Uji" ditekan**, plus **sekali otomatis** pas provider baru disimpan. Nggak ada probe rutin. |

## Yang berubah dari keputusan sebelumnya

**Form register BYO-only (keputusan 2026-09-21) batal sendiri.** Waktu itu
provider-nya cuma satu opsi hardcoded (`openai_compatible`). Sekarang
provider-nya datang dari daftar milik user. `CREATE_PROVIDER_CHOICES = ['openai_compatible']`
diganti jadi daftar provider org.

Guard US-AD67 AC1/AC2 (`validateCatalogModel`) yang kemarin dibuang dari form
**hidup lagi** dalam bentuk baru: model harus ada di daftar provider itu.

## Konsekuensi

- Migrasi `0010` + backfill dari agent yang ada.
- `ARCHITECTURE.md` — tabel `providers`, endpoint CRUD, RBAC.
- `DECISIONS.md` — supersede bagian BYO.
- `00-PRD.md` — story baru, AC-nya.
- Halaman Provider (design baru, ikut pola Stitch yang ada).
- Form agent disederhanakan.
- Endpoint probe `POST /provider/models` **pindah konteks**: dari "dipakai form
  agent" jadi "dipakai halaman Provider waktu daftar + refresh".

## Alasan di balik dua keputusan

Dua ini dulunya pertanyaan terbuka. Keputusannya sudah ada di tabel atas —
yang disimpan di sini cuma **alasannya**, biar nggak dipertanyakan ulang.

**Refresh daftar model = 24 jam + tombol manual.** Daftar model provider jarang
berubah; yang berubah biasanya pas user nambah model sendiri di sisi provider.
24 jam cukup segar, dan nggak nembak API tiap jam cuma buat itu.

**Probe kredensial = manual + sekali saat simpan, nol probe rutin.** Ini muncul
dari temuan bahwa `/models` nggak membuktikan key valid. Opsi yang ditolak:
probe tiap refresh (24 jam) — lebih proaktif, tapi nembak `chat/completions`
tiap hari per provider. Hemat kuota menang, dan provider lokal user biasanya
punya limit ketat.

Tiga pertanyaan lain di daftar lama (provider lintas project, agent ganti
provider, provider default) jawabannya **cuma diulang** dari tabel aturan, jadi
duplikatnya dibuang — satu sumber kebenaran, bukan dua.

## Temuan tambahan (fase 0.5)

`ARCHITECTURE.md` §18 dulu menggambarkan struktur **rencana**, bukan yang nyata:
9 dari 9 direktori dan 29 dari 31 nama file yang dikontrak tidak ada, sementara
8 modul yang benar-benar ada tidak tercatat. Sudah diganti dengan struktur nyata
plus §18.3 untuk modul yang belum dibangun, dan `tools/verify_suite.py` sekarang
memeriksa §18 dua arah supaya drift ini tidak terulang.
