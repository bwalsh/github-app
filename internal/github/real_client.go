package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RealClient implements the Client interface using real GitHub API calls.
type RealClient struct {
	httpClient     *http.Client
	baseURL        string
	requestTimeout time.Duration
	tokenProvider  installationTokenProvider
}

type installationTokenProvider interface {
	Token(ctx context.Context, installationID int64) (string, error)
}

type staticTokenProvider struct {
	token string
}

func (p staticTokenProvider) Token(_ context.Context, _ int64) (string, error) {
	if p.token == "" {
		return "", fmt.Errorf("github token is empty")
	}
	return p.token, nil
}

type cachedInstallationToken struct {
	token     string
	expiresAt time.Time
}

type appInstallationTokenProvider struct {
	httpClient *http.Client
	baseURL    string
	appID      int64
	privateKey *rsa.PrivateKey

	mu     sync.Mutex
	cache  map[int64]cachedInstallationToken
	nowFn  func() time.Time
	margin time.Duration
}

const (
	defaultGitHubAPIBaseURL     = "https://api.github.com"
	defaultGitHubRequestTimeout = 30 * time.Second
	defaultTokenRefreshMargin   = time.Minute
)

// NewRealClient creates a new RealClient for GitHub API interactions.
// token should be a GitHub token that is valid for the target repositories.
func NewRealClient(token string) *RealClient {
	return &RealClient{
		httpClient:     newGitHubHTTPClient(defaultGitHubRequestTimeout),
		baseURL:        defaultGitHubAPIBaseURL,
		requestTimeout: defaultGitHubRequestTimeout,
		tokenProvider:  staticTokenProvider{token: token},
	}
}

// NewAppClient creates a RealClient that mints installation tokens per request
// using GitHub App credentials.
func NewAppClient(appID int64, privateKeyPEM string) (*RealClient, error) {
	httpClient := newGitHubHTTPClient(defaultGitHubRequestTimeout)
	return newAppClient(defaultGitHubAPIBaseURL, httpClient, appID, privateKeyPEM, defaultGitHubRequestTimeout)
}

func newAppClient(baseURL string, httpClient *http.Client, appID int64, privateKeyPEM string, requestTimeout time.Duration) (*RealClient, error) {
	provider, err := newAppInstallationTokenProvider(httpClient, baseURL, appID, privateKeyPEM)
	if err != nil {
		return nil, err
	}

	return &RealClient{
		httpClient:     httpClient,
		baseURL:        strings.TrimRight(baseURL, "/"),
		requestTimeout: requestTimeout,
		tokenProvider:  provider,
	}, nil
}

func newGitHubHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func newAppInstallationTokenProvider(httpClient *http.Client, baseURL string, appID int64, privateKeyPEM string) (*appInstallationTokenProvider, error) {
	if appID <= 0 {
		return nil, fmt.Errorf("github app id must be greater than zero")
	}

	privateKey, err := parseGitHubAppPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}

	return &appInstallationTokenProvider{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		appID:      appID,
		privateKey: privateKey,
		cache:      make(map[int64]cachedInstallationToken),
		nowFn:      time.Now,
		margin:     defaultTokenRefreshMargin,
	}, nil
}

func parseGitHubAppPrivateKey(privateKeyPEM string) (*rsa.PrivateKey, error) {
	normalized := strings.TrimSpace(privateKeyPEM)
	if !strings.Contains(normalized, "\n") {
		normalized = strings.ReplaceAll(normalized, `\n`, "\n")
	}

	block, _ := pem.Decode([]byte(normalized))
	if block == nil {
		return nil, fmt.Errorf("invalid github app private key: failed to decode PEM")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("invalid github app private key: %w", err)
	}

	rsaKey, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("invalid github app private key: expected RSA private key")
	}

	return rsaKey, nil
}

func (p *appInstallationTokenProvider) Token(ctx context.Context, installationID int64) (string, error) {
	if installationID <= 0 {
		return "", fmt.Errorf("invalid installation id %d", installationID)
	}

	now := p.nowFn()
	p.mu.Lock()
	if cached, ok := p.cache[installationID]; ok && now.Add(p.margin).Before(cached.expiresAt) {
		p.mu.Unlock()
		return cached.token, nil
	}
	p.mu.Unlock()

	jwtToken, err := p.signJWT(now)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", p.baseURL, installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("github installation token request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Token == "" {
		return "", fmt.Errorf("github installation token response missing token")
	}

	p.mu.Lock()
	p.cache[installationID] = cachedInstallationToken{token: result.Token, expiresAt: result.ExpiresAt}
	p.mu.Unlock()

	return result.Token, nil
}

func (p *appInstallationTokenProvider) signJWT(now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}

	claims, err := json.Marshal(map[string]interface{}{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": strconv.FormatInt(p.appID, 10),
	})
	if err != nil {
		return "", err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claims)
	signingInput := encodedHeader + "." + encodedClaims
	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (c *RealClient) requestContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.requestTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.requestTimeout)
}

func (c *RealClient) authorizationToken(ctx context.Context, installationID int64) (string, error) {
	if c.tokenProvider == nil {
		return "", fmt.Errorf("github token provider is not configured")
	}
	return c.tokenProvider.Token(ctx, installationID)
}

func (c *RealClient) setRequestHeaders(req *http.Request, token string) {
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
}

// CreateCheckRun creates a new check run via the GitHub API.
func (c *RealClient) CreateCheckRun(ctx context.Context, installationID int64, repo, name, sha string) (int64, error) {
	reqCtx, cancel := c.requestContext(ctx)
	defer cancel()

	token, err := c.authorizationToken(reqCtx, installationID)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("%s/repos/%s/check-runs", c.baseURL, repo)

	body := map[string]interface{}{
		"name":     name,
		"head_sha": sha,
		"status":   "in_progress",
	}

	data, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	c.setRequestHeaders(req, token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[github] CreateCheckRun error: %v", err)
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[github] CreateCheckRun failed: status=%d body=%s", resp.StatusCode, string(body))
		return 0, fmt.Errorf("github api error: %d", resp.StatusCode)
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	log.Printf("[github] CREATE check_run id=%d installation=%d repo=%s name=%q sha=%s status=in_progress",
		result.ID, installationID, repo, name, sha)

	return result.ID, nil
}

// UpdateCheckRun updates an existing check run status.
func (c *RealClient) UpdateCheckRun(ctx context.Context, installationID int64, repo string, checkRunID int64, status CheckStatus) error {
	reqCtx, cancel := c.requestContext(ctx)
	defer cancel()

	token, err := c.authorizationToken(reqCtx, installationID)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/repos/%s/check-runs/%d", c.baseURL, repo, checkRunID)

	body := map[string]interface{}{
		"status":     status.Status,
		"conclusion": status.Conclusion,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPatch, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	c.setRequestHeaders(req, token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[github] UpdateCheckRun error: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[github] UpdateCheckRun failed: status=%d body=%s", resp.StatusCode, string(body))
		return fmt.Errorf("github api error: %d", resp.StatusCode)
	}

	log.Printf("[github] UPDATE check_run id=%d installation=%d repo=%s status=%s conclusion=%s",
		checkRunID, installationID, repo, status.Status, status.Conclusion)
	return nil
}

// CreateCommitStatus records a commit status update via GitHub API.
func (c *RealClient) CreateCommitStatus(ctx context.Context, installationID int64, repo, sha string, status CommitStatus) error {
	reqCtx, cancel := c.requestContext(ctx)
	defer cancel()

	token, err := c.authorizationToken(reqCtx, installationID)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/repos/%s/statuses/%s", c.baseURL, repo, sha)

	body := map[string]interface{}{
		"state":       status.State,
		"description": status.Description,
		"context":     status.Context,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	c.setRequestHeaders(req, token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("[github] CreateCommitStatus error: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[github] CreateCommitStatus failed: status=%d body=%s", resp.StatusCode, string(body))
		return fmt.Errorf("github api error: %d", resp.StatusCode)
	}

	log.Printf("[github] COMMIT status installation=%d repo=%s sha=%s state=%s context=%s description=%q",
		installationID, repo, sha, status.State, status.Context, status.Description)
	return nil
}
