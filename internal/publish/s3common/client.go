// Package s3common is the generic S3 client shared by S3-compatible publishers (cloudflare-r2,
// tigris, future backends). It implements the wire-level operations — SigV4 signing, bucket
// head/create, ListObjectsV2, object put/delete — and knows nothing about any provider's
// control plane (no Cloudflare API, no Tigris admin API). Put and Delete satisfy
// publish.FileWriter, so publish.CommitFiles drives a Client directly.
package s3common

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/core"
)

// Logger is the optional diagnostic sink a driver passes in (satisfied by *clog.Logger via a
// one-line adapter) so per-object put/delete lines land in the publish log without s3common
// importing clog.
type Logger interface {
	Detail(category, label string, kv ...any)
}

// Client is the shared S3 client. Constructed once per publish run; its methods build a fresh
// signed request each call, so a Client may be shared across goroutines that read it.
type Client struct {
	Name       string // publisher id, used as the log label
	Endpoint   string // e.g. https://fly.storage.tigrisdev.com (no trailing slash)
	Bucket     string
	Region     string // "auto" for R2; configurable for others
	AccessKey  string
	SecretKey  string
	HTTPClient *http.Client     // nil → http.DefaultClient
	Now        func() time.Time // nil → time.Now (overridable in tests)
	Logger     Logger           // optional
}

// New constructs a Client with a 2-minute HTTP timeout. The caller validates the bucket name
// via ValidateBucketName; Name/Logger are set by the driver afterwards.
func New(endpoint, bucket, region, accessKey, secretKey string) *Client {
	return &Client{
		Endpoint:   strings.TrimRight(endpoint, "/"),
		Bucket:     bucket,
		Region:     region,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		HTTPClient: &http.Client{Timeout: 2 * time.Minute},
	}
}

func (c *Client) clock() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// bucketNameRE matches the S3 naming rules below the length check: lowercase letters, numbers
// and hyphens, beginning and ending with a letter or number (no dots).
var bucketNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// ValidateBucketName checks a bucket name against the S3 rules so a typo fails with a clear
// message up front rather than a 400 from the API mid-publish.
func ValidateBucketName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return fmt.Errorf("must be 3–63 characters")
	}
	if !bucketNameRE.MatchString(name) {
		return fmt.Errorf("must be lowercase letters, numbers and hyphens, starting and ending with a letter or number")
	}
	return nil
}

// Head reports whether the bucket exists: 200 or 403 (exists but listing denied) → true,
// 404 → false.
func (c *Client) Head(ctx context.Context) (bool, error) {
	resp, err := c.do(ctx, http.MethodHead, "/"+c.Bucket, nil)
	if err != nil {
		return false, err
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusNotFound:
		return false, nil
	case http.StatusOK, http.StatusForbidden:
		return true, nil
	default:
		return false, fmt.Errorf("s3 head bucket %s: %s", c.Bucket, resp.Status)
	}
}

// Create creates the bucket via PUT /{bucket}. location is the optional LocationConstraint
// (R2 ignores it; others use it for region selection). A 409 (already owned) is not an error.
func (c *Client) Create(ctx context.Context, location string) error {
	resp, err := c.do(ctx, http.MethodPut, "/"+c.Bucket, createBucketBody(location))
	if err != nil {
		return err
	}
	status := resp.StatusCode
	drain(resp)
	switch {
	case status/100 == 2, status == http.StatusConflict:
		return nil
	default:
		return fmt.Errorf("s3 create bucket %s: %s", c.Bucket, http.StatusText(status))
	}
}

// listResult is the subset of an S3 ListObjectsV2 response colophon needs.
type listResult struct {
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
	Contents              []struct {
		Key  string `xml:"Key"`
		ETag string `xml:"ETag"`
	} `xml:"Contents"`
}

// List enumerates the bucket via ListObjectsV2 (paginated) into a key → ETag (MD5) manifest,
// the State the incremental planner diffs the tree against.
func (c *Client) List(ctx context.Context) (core.State, error) {
	state := core.State{}
	token := ""
	for {
		params := url.Values{"list-type": {"2"}}
		if token != "" {
			params.Set("continuation-token", token)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint+"/"+c.Bucket, nil)
		if err != nil {
			return nil, err
		}
		req.URL.RawQuery = params.Encode()
		signV4(req, "/"+c.Bucket, c.AccessKey, c.SecretKey, c.Region, emptyPayloadHash, c.clock())
		resp, err := c.httpClient().Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 != 2 {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			drain(resp)
			return nil, fmt.Errorf("s3 list %s: %s: %s", c.Bucket, resp.Status, strings.TrimSpace(string(msg)))
		}
		var lr listResult
		err = xml.NewDecoder(resp.Body).Decode(&lr)
		drain(resp)
		if err != nil {
			return nil, fmt.Errorf("s3 list %s: %w", c.Bucket, err)
		}
		for _, o := range lr.Contents {
			state[o.Key] = strings.Trim(o.ETag, `"`)
		}
		if !lr.IsTruncated || lr.NextContinuationToken == "" {
			return state, nil
		}
		token = lr.NextContinuationToken
	}
}

// PutCORS sets the bucket's CORS policy via the S3 PutBucketCors subresource, allowing
// cross-origin GET/HEAD from the given origins (use "*" for any). It exists so a routed search
// index (or any asset fetched via fetch()/an ES-module import, which — unlike <img> — are not
// CORS-exempt) is reachable from the site's origin. It's the only way to set CORS on R2 (no
// dashboard), and R2 rejects AllowedHeader "*", so specific headers are listed.
func (c *Client) PutCORS(ctx context.Context, origins []string) error {
	body := corsConfig(origins)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.Endpoint+"/"+c.Bucket, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.URL.RawQuery = "cors=" // the ?cors subresource; signed via the canonical query string
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/xml")
	sum := md5.Sum(body) // AWS requires Content-MD5 on PutBucketCors; harmless on R2/Tigris
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(sum[:]))
	signV4(req, "/"+c.Bucket, c.AccessKey, c.SecretKey, c.Region, hexSHA256(body), c.clock())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("s3 put cors %s: %s: %s", c.Bucket, resp.Status, strings.TrimSpace(string(msg)))
	}
	c.log("cors", c.Bucket, "origins", strings.Join(origins, " "))
	return nil
}

// corsConfig builds a CORSConfiguration allowing GET/HEAD from each origin. AllowedHeader is kept
// to specific values (not "*") because R2 rejects the wildcard there.
func corsConfig(origins []string) []byte {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	var b strings.Builder
	b.WriteString(`<CORSConfiguration>`)
	for _, o := range origins {
		b.WriteString(`<CORSRule>`)
		b.WriteString(`<AllowedOrigin>` + esc.Replace(o) + `</AllowedOrigin>`)
		b.WriteString(`<AllowedMethod>GET</AllowedMethod><AllowedMethod>HEAD</AllowedMethod>`)
		b.WriteString(`<AllowedHeader>content-type</AllowedHeader><AllowedHeader>range</AllowedHeader>`)
		b.WriteString(`<MaxAgeSeconds>3600</MaxAgeSeconds>`)
		b.WriteString(`</CORSRule>`)
	}
	b.WriteString(`</CORSConfiguration>`)
	return []byte(b.String())
}

// Put uploads an object with a content type inferred from its extension. It satisfies
// publish.FileWriter.Put.
func (c *Client) Put(ctx context.Context, name string, b []byte) error {
	resp, err := c.do(ctx, http.MethodPut, c.objectPath(name), b)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("s3 put %s: %s: %s", name, resp.Status, strings.TrimSpace(string(msg)))
	}
	c.log("put", name, "bytes", len(b))
	return nil
}

// Delete removes an object; a missing object (404) is not an error. It satisfies
// publish.FileWriter.Delete.
func (c *Client) Delete(ctx context.Context, name string) error {
	resp, err := c.do(ctx, http.MethodDelete, c.objectPath(name), nil)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("s3 delete %s: %s", name, resp.Status)
	}
	c.log("delete", name)
	return nil
}

func (c *Client) log(action, name string, kv ...any) {
	if c.Logger == nil {
		return
	}
	c.Logger.Detail("PUBLISH", c.Name, append([]any{action, name}, kv...)...)
}

func (c *Client) objectPath(key string) string { return "/" + c.Bucket + "/" + encodeKey(key) }

// do builds, signs and sends a request to an already-encoded path. A non-nil body is sent with
// its SHA-256 and a content type inferred from the path; a nil body signs as empty.
func (c *Client) do(ctx context.Context, method, encodedPath string, body []byte) (*http.Response, error) {
	hash := emptyPayloadHash
	var r io.Reader
	if body != nil {
		hash = hexSHA256(body)
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Endpoint+encodedPath, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = int64(len(body))
		if ct := mime.TypeByExtension(path.Ext(encodedPath)); ct != "" {
			req.Header.Set("Content-Type", ct)
		}
	}
	signV4(req, encodedPath, c.AccessKey, c.SecretKey, c.Region, hash, c.clock())
	return c.httpClient().Do(req)
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// createBucketBody returns the CreateBucket request body carrying the location hint, or nil
// (auto-locate) when no location is configured.
func createBucketBody(location string) []byte {
	if location == "" {
		return nil
	}
	return []byte("<CreateBucketConfiguration><LocationConstraint>" + location + "</LocationConstraint></CreateBucketConfiguration>")
}
