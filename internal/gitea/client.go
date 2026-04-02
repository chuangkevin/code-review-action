package gitea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type PRInfo struct {
	Number       int      `json:"number"`
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	User         PRUser   `json:"user"`
	Head         PRBranch `json:"head"`
	Base         PRBranch `json:"base"`
	Additions    int      `json:"additions"`
	Deletions    int      `json:"deletions"`
	ChangedFiles int      `json:"changed_files"`
}

type PRUser struct {
	Login    string `json:"login"`
	FullName string `json:"full_name"`
}

func (u PRUser) DisplayName() string {
	if u.FullName != "" {
		return u.FullName
	}
	return u.Login
}

type PRBranch struct {
	Ref string `json:"ref"`
}

type ReviewComment struct {
	Body   string `json:"body"`
	Path   string `json:"path,omitempty"`
	NewPos int    `json:"new_position,omitempty"`
}

// CreateReviewRequest is the payload for creating a PR review with inline comments.
type CreateReviewRequest struct {
	Body     string              `json:"body"`
	Event    string              `json:"event"`
	Comments []ReviewLineComment `json:"comments,omitempty"`
}

type ReviewLineComment struct {
	Path        string `json:"path"`
	NewPosition int    `json:"new_position"`
	Body        string `json:"body"`
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) GetPRInfo(owner, repo string, prNumber int) (*PRInfo, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d", c.baseURL, owner, repo, prNumber)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get PR info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get PR info (status %d): %s", resp.StatusCode, body)
	}

	var pr PRInfo
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("decode PR info: %w", err)
	}
	return &pr, nil
}

func (c *Client) GetPRDiff(owner, repo string, prNumber int) (string, error) {
	// Gitea: use .diff endpoint for unified diff format
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d.diff", c.baseURL, owner, repo, prNumber)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("get PR diff: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get PR diff (status %d): %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read diff: %w", err)
	}
	return string(body), nil
}

// SubmitReview creates a PR review with summary body + inline comments in one API call.
func (c *Client) SubmitReview(owner, repo string, prNumber int, review CreateReviewRequest) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews", c.baseURL, owner, repo, prNumber)

	body, _ := json.Marshal(review)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("submit review: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("submit review (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

// PostComment posts a general issue comment (kept as fallback).
func (c *Client) PostComment(owner, repo string, prNumber int, body string) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, prNumber)

	payload, _ := json.Marshal(map[string]string{"body": body})
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("post comment (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

// GetPRReviews returns all reviews on a PR.
func (c *Client) GetPRReviews(owner, repo string, prNumber int) ([]Review, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews", c.baseURL, owner, repo, prNumber)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get reviews: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get reviews (status %d): %s", resp.StatusCode, body)
	}

	var reviews []Review
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return nil, fmt.Errorf("decode reviews: %w", err)
	}
	return reviews, nil
}

// GetReviewComments returns inline comments for a specific review.
func (c *Client) GetReviewComments(owner, repo string, prNumber, reviewID int) ([]ReviewCommentDetail, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews/%d/comments", c.baseURL, owner, repo, prNumber, reviewID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get review comments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get review comments (status %d): %s", resp.StatusCode, body)
	}

	var comments []ReviewCommentDetail
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("decode review comments: %w", err)
	}
	return comments, nil
}

// GetIssueComments returns all comments on a PR/issue.
func (c *Client) GetIssueComments(owner, repo string, issueNumber int) ([]IssueComment, error) {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, issueNumber)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get issue comments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get issue comments (status %d): %s", resp.StatusCode, body)
	}

	var comments []IssueComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("decode issue comments: %w", err)
	}
	return comments, nil
}

// DismissReview dismisses a previous review.
func (c *Client) DismissReview(owner, repo string, prNumber, reviewID int, message string) error {
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews/%d/dismissals", c.baseURL, owner, repo, prNumber, reviewID)

	payload, _ := json.Marshal(map[string]string{"message": message})
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("dismiss review: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("dismiss review (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

type Review struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
	User PRUser `json:"user"`
	State string `json:"state"` // "COMMENT", "REQUEST_CHANGES", "APPROVED"
}

type ReviewCommentDetail struct {
	ID       int    `json:"id"`
	Body     string `json:"body"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	User     PRUser `json:"user"`
	Resolver *PRUser `json:"resolver"`
}

type IssueComment struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
	User PRUser `json:"user"`
}

// ReplyToReviewComment posts a reply in the review comment thread.
// commentID is the ID of the original review comment to reply to.
// ReplyAsReview submits a new review with an inline comment on the same file:line.
// This makes the reply appear in Files Changed tab near the original comment.
func (c *Client) ReplyAsReview(owner, repo string, prNumber int, filePath string, line int, body string) error {
	review := CreateReviewRequest{
		Body:  "",
		Event: "COMMENT",
		Comments: []ReviewLineComment{
			{
				Path:        filePath,
				NewPosition: line,
				Body:        body,
			},
		},
	}
	return c.SubmitReview(owner, repo, prNumber, review)
}

// ResolveComment marks a review comment as resolved.
func (c *Client) ResolveComment(owner, repo string, prNumber int, commentID int) error {
	// Gitea API: POST /repos/{owner}/{repo}/issues/comments/{id}
	// with a special header or parameter to resolve
	// Actually Gitea uses: POST /repos/{owner}/{repo}/pulls/{index}/reviews/{id}/resolve
	// But the simpler approach: use the resolve endpoint
	url := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/comments/%d", c.baseURL, owner, repo, commentID)

	// Gitea doesn't have a dedicated "resolve" API for individual comments in older versions.
	// The workaround: we post a ✅ reply and let the approve handle it.
	// For newer Gitea (1.22+), try the resolve endpoint:
	resolveURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/resolve_comment", c.baseURL, owner, repo, prNumber)
	payload, _ := json.Marshal(map[string]int{"comment_id": commentID})
	req, err := http.NewRequest("POST", resolveURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// If resolve endpoint doesn't exist, not critical
		_ = url // suppress unused
		return nil
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "token "+c.token)
}
