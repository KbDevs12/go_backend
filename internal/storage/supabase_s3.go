package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"backend/internal/config"
)

type UploadResult struct {
	ObjectKey string `json:"object_key"`
	PublicURL string `json:"public_url"`
}

func UploadPaymentProof(ctx context.Context, originalFilename string, contentType string, body []byte) (UploadResult, error) {
	if config.App.SupabaseS3Endpoint == "" || config.App.SupabaseS3AccessKeyID == "" || config.App.SupabaseS3SecretAccessKey == "" || config.App.SupabaseS3Bucket == "" {
		return UploadResult{}, errors.New("konfigurasi Supabase S3 belum lengkap")
	}

	ext := extensionFromContentType(contentType)
	if ext == "" {
		ext = strings.ToLower(path.Ext(originalFilename))
	}
	if ext == "" {
		ext = ".jpg"
	}

	objectKey := fmt.Sprintf("payment-proofs/%s/%s%s", time.Now().Format("2006/01/02"), randomHex(16), ext)
	if err := putObject(ctx, config.App.SupabaseS3Endpoint, config.App.SupabaseS3Region, config.App.SupabaseS3Bucket, objectKey, contentType, body); err != nil {
		return UploadResult{}, err
	}

	return UploadResult{
		ObjectKey: objectKey,
		PublicURL: buildPublicURL(config.App.SupabaseS3Endpoint, config.App.SupabaseStoragePublicBaseURL, config.App.SupabaseS3Bucket, objectKey),
	}, nil
}

func putObject(ctx context.Context, endpoint string, region string, bucket string, key string, contentType string, body []byte) error {
	if region == "" {
		region = "ap-southeast-1"
	}

	endpoint = strings.TrimRight(endpoint, "/")
	objectURL := endpoint + "/" + url.PathEscape(bucket) + "/" + escapePath(key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL, bytes.NewReader(body))
	if err != nil {
		return err
	}

	payloadHash := sha256Hex(body)
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("X-Amz-Date", amzDate)
	req.ContentLength = int64(len(body))

	authorization, err := signV4(req, payloadHash, dateStamp, amzDate, region, config.App.SupabaseS3AccessKeyID, config.App.SupabaseS3SecretAccessKey)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authorization)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("upload ke Supabase Storage gagal: status=%d body=%s", res.StatusCode, strings.TrimSpace(string(message)))
	}

	return nil
}

func signV4(req *http.Request, payloadHash string, dateStamp string, amzDate string, region string, accessKey string, secretKey string) (string, error) {
	canonicalURI := req.URL.EscapedPath()
	canonicalQuery := req.URL.RawQuery

	canonicalHeaderMap := map[string]string{
		"content-type":         req.Header.Get("Content-Type"),
		"host":                 req.URL.Host,
		"x-amz-content-sha256": req.Header.Get("X-Amz-Content-Sha256"),
		"x-amz-date":           req.Header.Get("X-Amz-Date"),
	}

	headerKeys := make([]string, 0, len(canonicalHeaderMap))
	for key := range canonicalHeaderMap {
		headerKeys = append(headerKeys, key)
	}
	sort.Strings(headerKeys)

	var canonicalHeaders strings.Builder
	for _, key := range headerKeys {
		canonicalHeaders.WriteString(key)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(canonicalHeaderMap[key]))
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(headerKeys, ";")

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := getSignatureKey(secretKey, dateStamp, region, "s3")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))

	return fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", accessKey, credentialScope, signedHeaders, signature), nil
}

func getSignatureKey(secretKey string, dateStamp string, regionName string, serviceName string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, regionName)
	kService := hmacSHA256(kRegion, serviceName)
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func escapePath(value string) string {
	parts := strings.Split(value, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func extensionFromContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func buildPublicURL(s3Endpoint string, explicitBaseURL string, bucket string, objectKey string) string {
	base := strings.TrimRight(explicitBaseURL, "/")
	if base == "" {
		base = strings.TrimRight(s3Endpoint, "/")
		base = strings.TrimSuffix(base, "/storage/v1/s3") + "/storage/v1/object/public"
	}
	return base + "/" + url.PathEscape(bucket) + "/" + escapePath(objectKey)
}
