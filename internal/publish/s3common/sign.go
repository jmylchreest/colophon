package s3common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

// emptyPayloadHash is SHA-256 of "" — used for requests with no body (HEAD/GET).
const emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// signV4 adds an AWS Signature Version 4 Authorization header to req for the S3 service.
// canonicalURI is the already-encoded request path (it must match what the server sees);
// payloadHash is the hex SHA-256 of the body. R2 uses region "auto". Only host, x-amz-date
// and x-amz-content-sha256 are signed; any other header (e.g. Content-Type) rides unsigned.
func signV4(req *http.Request, canonicalURI, accessKey, secretKey, region, payloadHash string, now time.Time) {
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	host := req.URL.Host

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	const signedHeaders = "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+
		", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func hexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// encodeKey percent-encodes an object key for use in the request path, preserving "/" as a
// separator and the RFC 3986 unreserved set; everything else is %-escaped (S3 canonical
// form). Typical colophon keys (slugified paths) need no escaping.
func encodeKey(key string) string {
	var b strings.Builder
	for i := 0; i < len(key); i++ {
		c := key[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~', c == '/':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			const hexDigits = "0123456789ABCDEF"
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0xf])
		}
	}
	return b.String()
}
