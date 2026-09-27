package main

// Wiring kunci master untuk webhook.
//
// Dua fungsi, bukan satu, karena arahnya berbeda dan kegagalannya berbeda:
// seal dipakai saat mendaftarkan webhook, open saat mengirim. Keduanya
// mendekode kunci per panggilan (pola credentialDecrypter) supaya deployment
// tanpa AGENTDECK_MASTER_KEY tetap melayani endpoint lain — yang gagal hanya
// jalur webhook, dengan pesan yang tidak membawa materi kunci.

import (
	"errors"
	"fmt"

	"agentdeck/internal/crypto"
)

// sealWebhookSecret mengenkripsi secret webhook sebelum disimpan (16).
func sealWebhookSecret(rawKey string) func(string) ([]byte, error) {
	return func(plaintext string) ([]byte, error) {
		key, err := crypto.LoadKey(rawKey)
		if err != nil {
			return nil, fmt.Errorf("master key unavailable: %w", err)
		}
		return crypto.Seal(key, plaintext)
	}
}

// openWebhookSecret membuka secret terenkripsi untuk menandatangani pengiriman.
func openWebhookSecret(rawKey string) func([]byte) (string, error) {
	return func(sealed []byte) (string, error) {
		key, err := crypto.LoadKey(rawKey)
		if err != nil {
			return "", fmt.Errorf("master key unavailable: %w", err)
		}
		plaintext, err := crypto.Open(key, sealed)
		if err != nil {
			return "", errors.New("stored webhook secret could not be opened")
		}
		return plaintext, nil
	}
}
