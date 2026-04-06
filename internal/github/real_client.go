package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// RealClient implements the Client interface using real GitHub API calls.
type RealClient struct {
	httpClient *http.Client
	baseURL    string
	token      string // GitHub App installation token
}

// NewRealClient creates a new RealClient for GitHub API interactions.
// token should be a GitHub App installation token (obtained from JWT flow).
func NewRealClient(token string) *RealClient {
	return &RealClient{
		httpClient: &http.Client{Timeout: 0}, // Use default timeout
		baseURL:    "https://api.github.com",
		token:      token,
	}
}

// CreateCheckRun creates a new check run via the GitHub API.
func (c *RealClient) CreateCheckRun(ctx context.Context, installationID int64, repo, name, sha string) (int64, error) {
	url := fmt.Sprintf("%s/repos/%s/check-runs", c.baseURL, repo)

	body := map[string]interface{}{
		"name":     name,
		"head_sha": sha,
		"status":   "in_progress",
	}

	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

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
	json.NewDecoder(resp.Body).Decode(&result)

	log.Printf("[github] CREATE check_run id=%d installation=%d repo=%s name=%q sha=%s status=in_progress",
		result.ID, installationID, repo, name, sha)

	return result.ID, nil
}

// UpdateCheckRun updates an existing check run status.
func (c *RealClient) UpdateCheckRun(ctx context.Context, installationID int64, repo string, checkRunID int64, status CheckStatus) error {
	url := fmt.Sprintf("%s/repos/%s/check-runs/%d", c.baseURL, repo, checkRunID)

	body := map[string]interface{}{
		"status":     status.Status,
		"conclusion": status.Conclusion,
	}

	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(data))
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

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
	url := fmt.Sprintf("%s/repos/%s/statuses/%s", c.baseURL, repo, sha)

	body := map[string]interface{}{
		"state":       status.State,
		"description": status.Description,
		"context":     status.Context,
	}

	data, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

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
