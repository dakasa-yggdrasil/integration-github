package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	VisibilityPublic   = "public"
	VisibilityPrivate  = "private"
	VisibilityInternal = "internal"
)

var supportedVisibilities = []string{VisibilityPublic, VisibilityPrivate, VisibilityInternal}

// SetContainerPackageVisibility flips a package's visibility via the
// GitHub Packages API. Endpoint:
//
//	PATCH /{ownerType}/{owner}/packages/container/{package}
//
// where ownerType is "users" for personal accounts or "orgs" for
// organizations. Auth: Bearer token, must have read:packages and
// write:packages scopes (and admin:org for orgs). The token is provided
// by the caller — the helper does no credential resolution.
func SetContainerPackageVisibility(client *http.Client, token, ownerType, owner, packageName, visibility string) error {
	ownerType = strings.ToLower(strings.TrimSpace(ownerType))
	owner = strings.TrimSpace(owner)
	packageName = strings.TrimSpace(packageName)
	visibility = strings.ToLower(strings.TrimSpace(visibility))
	token = strings.TrimSpace(token)

	if ownerType != "users" && ownerType != "orgs" {
		return fmt.Errorf("ownerType must be 'users' or 'orgs', got %q", ownerType)
	}
	if owner == "" {
		return fmt.Errorf("owner is required")
	}
	if packageName == "" {
		return fmt.Errorf("package_name is required")
	}
	if !visibilityIsSupported(visibility) {
		return fmt.Errorf("visibility must be one of %v, got %q", supportedVisibilities, visibility)
	}
	if token == "" {
		return fmt.Errorf("token is required")
	}

	url := fmt.Sprintf("https://api.github.com/%s/%s/packages/container/%s", ownerType, owner, packageName)
	body, _ := json.Marshal(map[string]string{"visibility": visibility})
	req, err := http.NewRequest(http.MethodPatch, url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	payload, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("github responded %d: %s", resp.StatusCode, string(payload))
}

func visibilityIsSupported(value string) bool {
	for _, v := range supportedVisibilities {
		if v == value {
			return true
		}
	}
	return false
}
