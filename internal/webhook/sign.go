package webhook

// Penandatanganan pengiriman — ARCHITECTURE 13.2.
//
// Satu hal yang menentukan modul ini benar: byte yang di-HMAC harus **persis**
// byte yang dikirim. Karena itu body dibangun sekali sebagai []byte dan byte
// yang sama dipakai untuk header dan untuk request — bukan di-marshal dua kali,
// yang menghasilkan JSON identik secara isi tapi belum tentu identik secara
// byte (urutan kunci map, spasi).

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// SignatureHeader adalah nama header di 13.2.
const SignatureHeader = "X-AgentDeck-Signature"

// Envelope adalah body yang dikirim ke endpoint pelanggan.
//
// `payload` disimpan sebagai json.RawMessage supaya byte yang tersimpan di
// `events.payload_json` diteruskan apa adanya: memarshal ulang ke map[string]any
// akan menormalkan urutan kunci dan mengubah byte yang ditandatangani.
type Envelope struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	OrgID     string          `json:"org_id"`
	BoardID   string          `json:"board_id"`
	TaskID    string          `json:"task_id,omitempty"`
	RunID     string          `json:"run_id,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// BuildBody menyusun body final sebuah pengiriman.
//
// Payload yang kosong menjadi `null`, bukan string kosong: `events.payload_json`
// boleh NULL (event tanpa muatan), dan mengirim `""` akan membuat penerima
// gagal mem-parse JSON yang seharusnya valid.
func BuildBody(ev Event) ([]byte, error) {
	payload := ev.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("null")
	}
	return json.Marshal(Envelope{
		ID:        ev.ID,
		Kind:      ev.Kind,
		OrgID:     ev.OrgID,
		BoardID:   ev.BoardID,
		TaskID:    ev.TaskID,
		RunID:     ev.RunID,
		CreatedAt: ev.CreatedAt,
		Payload:   payload,
	})
}

// Sign menghitung nilai header `X-AgentDeck-Signature` untuk body dan secret.
//
// Bentuknya `v1=<hex>` seperti 13.2. Versi skema disertakan supaya rotasi
// algoritma nanti bisa diterima bersamaan alih-alih memutus semua pelanggan
// sekaligus.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature adalah sisi penerima dari 13.2 — dipakai tes dan probe untuk
// membuktikan header yang dikirim benar-benar cocok dengan body yang dikirim,
// bukan hanya "ada header".
func VerifySignature(secret string, body []byte, header string) bool {
	return hmac.Equal([]byte(Sign(secret, body)), []byte(header))
}
