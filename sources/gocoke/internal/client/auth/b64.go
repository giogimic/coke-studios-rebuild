package auth

// ASBase64 provides Base64 encode/decode compatible with the legacy ActionScript
// implementation found in ActionScript/Base64.txt (A-Z a-z 0-9 + / with '=' padding).
// This matches Go's standard encoding/base64 StdEncoding.

import "encoding/base64"

func ASBase64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func ASBase64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

