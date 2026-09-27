package webhook

// Tipe dan jadwal retry — ARCHITECTURE 13.3.

import (
	"time"
)

// Event adalah event yang akan dikirim, sudah dalam bentuk datar.
type Event struct {
	ID        int64
	Kind      string
	OrgID     string
	BoardID   string
	TaskID    string
	RunID     string
	Payload   []byte
	CreatedAt time.Time
}

// Subscription adalah satu webhook yang cocok dengan sebuah event.
type Subscription struct {
	ID        string
	OrgID     string
	BoardID   string
	URL       string
	SecretEnc []byte
}

// Delivery adalah satu percobaan pengiriman yang tercatat di 3.23.
type Delivery struct {
	ID           int64
	WebhookID    string
	EventID      int64
	Status       string
	Attempts     int
	ResponseCode *int
	LastError    string
	CreatedAt    time.Time
}

// Status delivery (3.23).
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
	StatusDead      = "dead"
)

// maxAttempts adalah batas di 13.3: enam percobaan lalu dead-letter. Nilainya
// juga batas atas CHECK `webhook_deliveries_attempt_chk` (0..10), jadi
// menaikkannya melewati 10 akan ditolak database.
const maxAttempts = 6

// deliveryTimeout adalah batas 10 detik di 13.3. Ini timeout **total** request,
// bukan hanya koneksi: endpoint yang menerima koneksi lalu menggantung selamanya
// harus tetap dihitung gagal, kalau tidak retry-nya tidak pernah berjalan.
const deliveryTimeout = 10 * time.Second

// retrySchedule adalah jeda sebelum setiap percobaan ulang, dari 13.3.
// Elemen ke-i adalah jeda sebelum percobaan ke-(i+1); percobaan pertama
// dikirim segera (jeda 0), jadi panjangnya satu lebih sedikit dari maxAttempts.
//
// §13.3 menuliskan enam nilai (1m, 5m, 15m, 30m, 1j, 2j) dan menyebut "total 6
// percobaan", dan kedua angka itu tidak bisa benar bersamaan: enam jeda berarti
// tujuh percobaan. Yang dipakai di sini adalah aturan yang mengikat — kalimat
// dead-letter "setelah 6 percobaan dan tidak ada sukses" — sehingga totalnya
// enam percobaan dan jedanya lima. Nilai 2 jam dari daftar itu karena itu tidak
// terpakai; selisihnya dicatat di docs/OPEN-ISSUES.md alih-alih dibiarkan
// diam-diam.
var retrySchedule = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
}

// nextDelay mengembalikan jeda sebelum percobaan berikutnya, dan apakah masih
// ada percobaan tersisa.
//
// `attempts` adalah jumlah percobaan yang **sudah** dilakukan. attempts == 0
// berarti belum pernah dikirim: jeda nol, kirim sekarang.
func nextDelay(attempts int) (time.Duration, bool) {
	if attempts >= maxAttempts {
		return 0, false
	}
	if attempts == 0 {
		return 0, true
	}
	// attempts == 1 -> retrySchedule[0] (1 menit), dan seterusnya.
	idx := attempts - 1
	if idx >= len(retrySchedule) {
		return 0, false
	}
	return retrySchedule[idx], true
}

// classify memutuskan apa yang terjadi pada sebuah percobaan.
//
// Aturan 13.3, dan satu-satunya tempat keputusan itu diambil:
//
//   - 2xx          → delivered, berhenti.
//   - 4xx          → failed TANPA retry: klien menolak, mengulanginya tidak akan
//     mengubah apa pun kecuali menambah beban (URL mati, signature mismatch).
//   - 5xx / error  → failed, retry selama jatah percobaan tersisa.
//
// Setelah jatah habis, statusnya `dead` — bukan `failed`. Perbedaannya penting:
// `failed` berarti "akan dicoba lagi", `dead` berarti "menyerah, tunggu retry
// manual". Menyamakan keduanya membuat riwayat pengiriman berbohong soal mana
// yang masih hidup.
func classify(code int, err error, attempts int) (status string, retry bool) {
	switch {
	case err == nil && code >= 200 && code < 300:
		return StatusDelivered, false
	case err == nil && code >= 400 && code < 500:
		return StatusFailed, false
	default:
		if attempts >= maxAttempts {
			return StatusDead, false
		}
		return StatusFailed, true
	}
}
