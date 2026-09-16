package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

// VerifyHMAC checks a Shopify webhook's X-Shopify-Hmac-Sha256 header
// against the raw request body, using the webhook signing secret from
// Secrets Manager (never hardcoded, never logged). Uses constant-time
// comparison to avoid a timing side-channel on the signature check.
func VerifyHMAC(body []byte, headerValue, secret string) bool {
	if headerValue == "" || secret == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(headerValue))
}
