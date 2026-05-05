package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dakasa-yggdrasil/integration-github/internal/protocol"
)

// pull_request.go implements OperationCreatePullRequest using GitHub's Git
// Data API to atomically commit one or more files to a feature branch and
// open a PR against the base branch.
//
// Wire-level flow:
//
//   1. GET  /repos/{owner}/{repo}/git/refs/heads/{base}        — head SHA of base branch
//   2. GET  /repos/{owner}/{repo}/git/commits/{base_sha}       — base tree SHA
//   3. POST /repos/{owner}/{repo}/git/blobs   (one per file)   — blob SHAs
//   4. POST /repos/{owner}/{repo}/git/trees                    — new tree
//   5. POST /repos/{owner}/{repo}/git/commits                  — new commit
//   6. POST /repos/{owner}/{repo}/git/refs                     — new head/{branch} ref
//   7. POST /repos/{owner}/{repo}/pulls                        — open PR
//
// This is the canonical "create commit + PR atomically" recipe; the
// alternative (Contents API) creates one commit per file which produces
// noisy history.
//
// We reuse the existing doGitHubRequest helper so the adapter stays single
// HTTP-client (and can be swapped in tests).

func createPullRequest(req protocol.AdapterExecuteIntegrationRequest) (protocol.AdapterExecuteIntegrationResponse, error) {
	apiBaseURL, token, instanceConfig, _, err := resolveExecuteConfig(req)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	repository := firstString(req.Input, []string{"repository"})
	if repository == "" {
		repository = firstString(instanceConfig, []string{"default_repository"})
	}
	if repository == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, errors.New("repository is required (e.g. dakasa-co/dakasa-hall)")
	}
	owner, repo, err := splitOwnerRepo(repository)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}

	branch := firstString(req.Input, []string{"branch"})
	if branch == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, errors.New("branch is required (feature branch name to create)")
	}
	base := firstString(req.Input, []string{"base", "base_branch"})
	if base == "" {
		base = firstString(instanceConfig, []string{"default_base_branch"})
	}
	if base == "" {
		base = "main"
	}

	commitMessage := firstString(req.Input, []string{"commit_message"})
	if commitMessage == "" {
		return protocol.AdapterExecuteIntegrationResponse{}, errors.New("commit_message is required")
	}
	prTitle := firstString(req.Input, []string{"pr_title", "title"})
	if prTitle == "" {
		prTitle = commitMessage
	}
	prBody := firstString(req.Input, []string{"pr_body", "body"})

	files, err := parsePRFiles(req.Input["files"])
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, err
	}
	if len(files) == 0 {
		return protocol.AdapterExecuteIntegrationResponse{}, errors.New("files is required (non-empty array of {path, content})")
	}

	// 1+2. Resolve base SHA and base tree SHA.
	baseSHA, err := getRefSHA(apiBaseURL, token, owner, repo, base)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("create_pull_request: resolve base ref %q: %w", base, err)
	}
	baseTreeSHA, err := getCommitTreeSHA(apiBaseURL, token, owner, repo, baseSHA)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("create_pull_request: resolve base tree: %w", err)
	}

	// 3. Create blobs for each file.
	blobs := make([]map[string]any, 0, len(files))
	for _, f := range files {
		blobSHA, err := createBlob(apiBaseURL, token, owner, repo, f.Content)
		if err != nil {
			return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("create_pull_request: create blob %q: %w", f.Path, err)
		}
		blobs = append(blobs, map[string]any{
			"path": f.Path,
			"mode": "100644",
			"type": "blob",
			"sha":  blobSHA,
		})
	}

	// 4. Create tree referencing parent base tree + new blobs.
	newTreeSHA, err := createTree(apiBaseURL, token, owner, repo, baseTreeSHA, blobs)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("create_pull_request: create tree: %w", err)
	}

	// 5. Create commit pointing at the new tree, parent = base SHA.
	newCommitSHA, err := createCommit(apiBaseURL, token, owner, repo, commitMessage, newTreeSHA, []string{baseSHA})
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("create_pull_request: create commit: %w", err)
	}

	// 6. Create new ref refs/heads/{branch} pointing at the commit.
	if err := createRef(apiBaseURL, token, owner, repo, branch, newCommitSHA); err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("create_pull_request: create ref %q: %w", branch, err)
	}

	// 7. Open the PR.
	prURL, prNumber, err := openPullRequest(apiBaseURL, token, owner, repo, prTitle, prBody, branch, base)
	if err != nil {
		return protocol.AdapterExecuteIntegrationResponse{}, fmt.Errorf("create_pull_request: open PR: %w", err)
	}

	return protocol.AdapterExecuteIntegrationResponse{
		Operation:  OperationCreatePullRequest,
		Capability: OperationCreatePullRequest,
		Status:     "succeeded",
		Output: map[string]any{
			"repository": repository,
			"branch":     branch,
			"base":       base,
			"commit_sha": newCommitSHA,
			"pr_number":  prNumber,
			"pr_url":     prURL,
			"file_count": len(files),
		},
	}, nil
}

type prFile struct {
	Path    string
	Content string
}

func parsePRFiles(raw any) ([]prFile, error) {
	if raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("files must be an array, got %T", raw)
	}
	out := make([]prFile, 0, len(list))
	for i, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("files[%d] must be an object", i)
		}
		path := strings.TrimSpace(asGitHubString(obj["path"]))
		if path == "" {
			return nil, fmt.Errorf("files[%d].path is required", i)
		}
		content := asGitHubString(obj["content"])
		out = append(out, prFile{Path: path, Content: content})
	}
	return out, nil
}

func splitOwnerRepo(repository string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(repository), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repository must be owner/repo, got %q", repository)
	}
	return parts[0], parts[1], nil
}

func getRefSHA(apiBaseURL, token, owner, repo, branch string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/refs/heads/%s", owner, repo, branch)
	body, _, err := doGitHubRequest(apiBaseURL, token, "GET", path, nil, 200)
	if err != nil {
		return "", err
	}
	var resp struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode ref: %w", err)
	}
	if resp.Object.SHA == "" {
		return "", errors.New("ref response missing object.sha")
	}
	return resp.Object.SHA, nil
}

func getCommitTreeSHA(apiBaseURL, token, owner, repo, sha string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/commits/%s", owner, repo, sha)
	body, _, err := doGitHubRequest(apiBaseURL, token, "GET", path, nil, 200)
	if err != nil {
		return "", err
	}
	var resp struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode commit: %w", err)
	}
	return resp.Tree.SHA, nil
}

func createBlob(apiBaseURL, token, owner, repo, content string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/blobs", owner, repo)
	body, _, err := doGitHubRequest(apiBaseURL, token, "POST", path, map[string]any{
		"content":  content,
		"encoding": "utf-8",
	}, 201)
	if err != nil {
		return "", err
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

func createTree(apiBaseURL, token, owner, repo, baseTreeSHA string, entries []map[string]any) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/trees", owner, repo)
	body, _, err := doGitHubRequest(apiBaseURL, token, "POST", path, map[string]any{
		"base_tree": baseTreeSHA,
		"tree":      entries,
	}, 201)
	if err != nil {
		return "", err
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

func createCommit(apiBaseURL, token, owner, repo, message, treeSHA string, parents []string) (string, error) {
	path := fmt.Sprintf("/repos/%s/%s/git/commits", owner, repo)
	body, _, err := doGitHubRequest(apiBaseURL, token, "POST", path, map[string]any{
		"message": message,
		"tree":    treeSHA,
		"parents": parents,
	}, 201)
	if err != nil {
		return "", err
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

func createRef(apiBaseURL, token, owner, repo, branch, sha string) error {
	path := fmt.Sprintf("/repos/%s/%s/git/refs", owner, repo)
	_, _, err := doGitHubRequest(apiBaseURL, token, "POST", path, map[string]any{
		"ref": "refs/heads/" + branch,
		"sha": sha,
	}, 201)
	return err
}

func openPullRequest(apiBaseURL, token, owner, repo, title, body, head, base string) (string, int, error) {
	path := fmt.Sprintf("/repos/%s/%s/pulls", owner, repo)
	respBody, _, err := doGitHubRequest(apiBaseURL, token, "POST", path, map[string]any{
		"title": title,
		"body":  body,
		"head":  head,
		"base":  base,
	}, 201)
	if err != nil {
		return "", 0, err
	}
	var resp struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", 0, err
	}
	return resp.HTMLURL, resp.Number, nil
}

func asGitHubString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}
