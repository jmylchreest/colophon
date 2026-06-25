package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const apiBase = "https://api.cloudflare.com/client/v4"

type apiClient struct {
	token string
	hc    *http.Client
}

func newAPIClient(token string) *apiClient {
	return &apiClient{token: token, hc: &http.Client{Timeout: 2 * time.Minute}}
}

// envelope is the standard Cloudflare API response wrapper.
type envelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result json.RawMessage `json:"result"`
}

type uploadMetadata struct {
	ContentType string `json:"contentType"`
}

type uploadItem struct {
	Key      string         `json:"key"`
	Value    string         `json:"value"`
	Metadata uploadMetadata `json:"metadata"`
	Base64   bool           `json:"base64"`
}

type deployment struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// deploymentInfo is the subset of a listed deployment that pruning needs.
type deploymentInfo struct {
	ID                string    `json:"id"`
	Environment       string    `json:"environment"`
	CreatedOn         time.Time `json:"created_on"`
	DeploymentTrigger struct {
		Metadata *struct {
			Branch string `json:"branch"`
		} `json:"metadata"`
	} `json:"deployment_trigger"`
}

// Branch is the branch label recorded for the deployment, or "" if unknown.
func (d deploymentInfo) Branch() string {
	if d.DeploymentTrigger.Metadata == nil {
		return ""
	}
	return d.DeploymentTrigger.Metadata.Branch
}

func (c *apiClient) do(ctx context.Context, method, url, bearer, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	data, _ := io.ReadAll(resp.Body)
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		// A non-JSON body (typically a Cloudflare 5xx HTML/plaintext error) would otherwise
		// surface as a cryptic JSON-decode error; show the status and the raw body instead.
		return fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, truncateBody(data))
	}
	if !env.Success {
		return fmt.Errorf("%s %s: %s (%s)", method, url, apiErr(env), resp.Status)
	}
	if out != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// truncateBody renders a response body for an error message, collapsing whitespace and capping
// the length so an HTML error page or stack trace doesn't flood the log.
func truncateBody(b []byte) string {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "(empty body)"
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return s
}

func apiErr(env envelope) string {
	if len(env.Errors) == 0 {
		return "request failed"
	}
	parts := make([]string, len(env.Errors))
	for i, e := range env.Errors {
		parts[i] = fmt.Sprintf("%d %s", e.Code, e.Message)
	}
	return strings.Join(parts, "; ")
}

// jsonDo POSTs a JSON body and decodes a JSON response. Every Cloudflare control-plane call
// colophon makes is a POST; a GET (e.g. projectExists) builds its request directly.
func (c *apiClient) jsonDo(ctx context.Context, url, bearer string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	return c.do(ctx, http.MethodPost, url, bearer, "application/json", body, out)
}

func (c *apiClient) projectExists(ctx context.Context, accountID, project string) (bool, error) {
	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s", apiBase, accountID, project)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		var env envelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		return false, fmt.Errorf("GET %s: %s", url, apiErr(env))
	}
}

// perPage is the page size for listing deployments. The Pages deployments endpoint
// rejects larger values (error 8000024), so keep it at the API's supported maximum.
const perPage = 25

// listDeployments returns every deployment for the project, following pagination.
func (c *apiClient) listDeployments(ctx context.Context, accountID, project string) ([]deploymentInfo, error) {
	var all []deploymentInfo
	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments?per_page=%d&page=%d", apiBase, accountID, project, perPage, page)
		var batch []deploymentInfo
		if err := c.do(ctx, http.MethodGet, url, c.token, "", nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < perPage {
			return all, nil
		}
	}
}

// deleteDeployment removes one deployment. force handles deployments still carrying an
// alias; the current/latest deployment for a branch can never be deleted.
func (c *apiClient) deleteDeployment(ctx context.Context, accountID, project, id string) error {
	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments/%s?force=true", apiBase, accountID, project, id)
	return c.do(ctx, http.MethodDelete, url, c.token, "", nil, nil)
}

// pagesProject is the subset of a Pages project used to derive its canonical URL.
type pagesProject struct {
	SubDomain        string   `json:"subdomain"`
	Domains          []string `json:"domains"`
	ProductionBranch string   `json:"production_branch"`
}

func (c *apiClient) getProject(ctx context.Context, accountID, project string) (*pagesProject, error) {
	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s", apiBase, accountID, project)
	var p pagesProject
	if err := c.do(ctx, http.MethodGet, url, c.token, "", nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (c *apiClient) createProject(ctx context.Context, accountID, project, branch string) error {
	url := fmt.Sprintf("%s/accounts/%s/pages/projects", apiBase, accountID)
	return c.jsonDo(ctx, url, c.token,
		map[string]string{"name": project, "production_branch": branch}, nil)
}

func (c *apiClient) uploadToken(ctx context.Context, accountID, project string) (string, error) {
	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/upload-token", apiBase, accountID, project)
	var res struct {
		JWT string `json:"jwt"`
	}
	if err := c.do(ctx, http.MethodGet, url, c.token, "", nil, &res); err != nil {
		return "", err
	}
	if res.JWT == "" {
		return "", fmt.Errorf("upload-token: empty jwt")
	}
	return res.JWT, nil
}

func (c *apiClient) checkMissing(ctx context.Context, jwt string, hashes []string) ([]string, error) {
	var missing []string
	err := c.jsonDo(ctx, apiBase+"/pages/assets/check-missing", jwt,
		map[string][]string{"hashes": hashes}, &missing)
	return missing, err
}

func (c *apiClient) upload(ctx context.Context, jwt string, items []uploadItem) error {
	return c.jsonDo(ctx, apiBase+"/pages/assets/upload", jwt, items, nil)
}

func (c *apiClient) upsertHashes(ctx context.Context, jwt string, hashes []string) error {
	return c.jsonDo(ctx, apiBase+"/pages/assets/upsert-hashes", jwt,
		map[string][]string{"hashes": hashes}, nil)
}

func (c *apiClient) createDeployment(ctx context.Context, accountID, project, branch string, manifest map[string]string) (*deployment, error) {
	mb, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("manifest", string(mb)); err != nil {
		return nil, err
	}
	if branch != "" {
		if err := mw.WriteField("branch", branch); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/accounts/%s/pages/projects/%s/deployments", apiBase, accountID, project)
	var dep deployment
	if err := c.do(ctx, http.MethodPost, url, c.token, mw.FormDataContentType(), &buf, &dep); err != nil {
		return nil, err
	}
	return &dep, nil
}
