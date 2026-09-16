package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestVerifyHMAC_ValidSignature(t *testing.T) {
	body := []byte(`{"id":123}`)
	secret := "test-webhook-secret"
	sig := sign(body, secret)

	if !VerifyHMAC(body, sig, secret) {
		t.Error("expected valid signature to verify")
	}
}

func TestVerifyHMAC_TamperedBody(t *testing.T) {
	secret := "test-webhook-secret"
	sig := sign([]byte(`{"id":123}`), secret)

	if VerifyHMAC([]byte(`{"id":456}`), sig, secret) {
		t.Error("expected tampered body to fail verification")
	}
}

func TestVerifyHMAC_WrongSecret(t *testing.T) {
	body := []byte(`{"id":123}`)
	sig := sign(body, "correct-secret")

	if VerifyHMAC(body, sig, "wrong-secret") {
		t.Error("expected wrong secret to fail verification")
	}
}

func TestVerifyHMAC_MissingHeaderOrSecret(t *testing.T) {
	body := []byte(`{"id":123}`)
	if VerifyHMAC(body, "", "some-secret") {
		t.Error("expected empty header to fail")
	}
	if VerifyHMAC(body, "some-sig", "") {
		t.Error("expected empty secret to fail")
	}
}
