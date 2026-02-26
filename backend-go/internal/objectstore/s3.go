package objectstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type S3Config struct {
	Endpoint     string
	Region       string
	Bucket       string
	AccessKeyID  string
	SecretKey    string
	UsePathStyle bool
	UseTLS       bool
}

type s3Store struct {
	endpoint     *url.URL
	region       string
	bucket       string
	accessKeyID  string
	secretKey    string
	usePathStyle bool
	httpClient   *http.Client
}

func NewS3(ctx context.Context, cfg S3Config) (Store, error) {
	rawEndpoint := strings.TrimSpace(cfg.Endpoint)
	if rawEndpoint == "" {
		return nil, fmt.Errorf("S3 endpoint is required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("S3 bucket is required")
	}
	if strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, fmt.Errorf("S3 credentials are required")
	}
	if !strings.HasPrefix(rawEndpoint, "http://") && !strings.HasPrefix(rawEndpoint, "https://") {
		if cfg.UseTLS {
			rawEndpoint = "https://" + rawEndpoint
		} else {
			rawEndpoint = "http://" + rawEndpoint
		}
	}
	ep, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, fmt.Errorf("parse S3 endpoint: %w", err)
	}
	if ep.Host == "" {
		return nil, fmt.Errorf("invalid S3 endpoint host")
	}

	store := &s3Store{
		endpoint:     ep,
		region:       region,
		bucket:       bucket,
		accessKeyID:  cfg.AccessKeyID,
		secretKey:    cfg.SecretKey,
		usePathStyle: cfg.UsePathStyle,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}

	if err := store.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *s3Store) Put(ctx context.Context, key string, contentType string, data []byte) error {
	key = normalizeKey(key)
	if key == "" {
		return fmt.Errorf("object key is required")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	resp, err := s.doSignedRequest(ctx, http.MethodPut, s.objectPath(key), nil, data, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("put object failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *s3Store) Get(ctx context.Context, key string) ([]byte, string, error) {
	key = normalizeKey(key)
	if key == "" {
		return nil, "", fmt.Errorf("object key is required")
	}
	resp, err := s.doSignedRequest(ctx, http.MethodGet, s.objectPath(key), nil, nil, "")
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, "", fmt.Errorf("get object failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return payload, resp.Header.Get("Content-Type"), nil
}

func (s *s3Store) ensureBucket(ctx context.Context) error {
	headResp, err := s.doSignedRequest(ctx, http.MethodHead, s.bucketPath(), nil, nil, "")
	if err != nil {
		return err
	}
	headResp.Body.Close()
	if headResp.StatusCode == http.StatusOK {
		return nil
	}
	if headResp.StatusCode != http.StatusNotFound && headResp.StatusCode != http.StatusForbidden {
		return fmt.Errorf("head bucket failed: status=%d", headResp.StatusCode)
	}
	createResp, err := s.doSignedRequest(ctx, http.MethodPut, s.bucketPath(), nil, nil, "")
	if err != nil {
		return err
	}
	defer createResp.Body.Close()
	if createResp.StatusCode >= 400 && createResp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(io.LimitReader(createResp.Body, 2048))
		return fmt.Errorf("create bucket failed: status=%d body=%s", createResp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *s3Store) bucketPath() string {
	return "/" + escapePath(s.bucket)
}

func (s *s3Store) objectPath(key string) string {
	return "/" + escapePath(s.bucket) + "/" + escapePath(key)
}

func normalizeKey(key string) string {
	return strings.TrimSpace(strings.TrimPrefix(key, "/"))
}

func escapePath(v string) string {
	parts := strings.Split(v, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

func (s *s3Store) doSignedRequest(
	ctx context.Context,
	method string,
	canonicalPath string,
	query url.Values,
	body []byte,
	contentType string,
) (*http.Response, error) {
	if query == nil {
		query = url.Values{}
	}
	payloadHash := sha256Hex(body)
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	u := *s.endpoint
	u.Path = canonicalPath
	u.RawQuery = query.Encode()

	host := u.Host
	canonicalHeaders := "host:" + host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := method + "\n" +
		canonicalPath + "\n" +
		query.Encode() + "\n" +
		canonicalHeaders + "\n" +
		signedHeaders + "\n" +
		payloadHash

	credentialScope := dateStamp + "/" + s.region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" +
		amzDate + "\n" +
		credentialScope + "\n" +
		sha256Hex([]byte(canonicalRequest))
	signature := hex.EncodeToString(hmacSHA256(signingKey(s.secretKey, dateStamp, s.region, "s3"), []byte(stringToSign)))
	authHeader := "AWS4-HMAC-SHA256 Credential=" + s.accessKeyID + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders +
		", Signature=" + signature

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Host", host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("Authorization", authHeader)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return s.httpClient.Do(req)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

func signingKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}
