package hmac

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

func Canonical(runID, sequence, emit, body string) string {
	return runID + "\n" + sequence + "\n" + emit + "\n" + body
}

func Sign(key []byte, runID, sequence, emit, body string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(Canonical(runID, sequence, emit, body)))
	return hex.EncodeToString(mac.Sum(nil))
}

func Verify(key []byte, runID, sequence, emit, body, signature string) bool {
	if signature == "" || runID == "" || sequence == "" || emit == "" {
		return false
	}
	want := Sign(key, runID, sequence, emit, body)
	return hmac.Equal([]byte(want), []byte(signature))
}

func LoadKey(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key := bytes.TrimSpace(raw)
	if len(key) == 0 {
		return nil, fmt.Errorf("empty key file %s", path)
	}
	return key, nil
}
