package catalog

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Approval struct {
	ApprovedAt string `json:"approved_at"`
	ReviewURL  string `json:"review_url"`
}

type ApprovedPlugin struct {
	PluginID   string `json:"plugin_id"`
	Repository string `json:"repository"`
	ApprovedAt string `json:"approved_at"`
	ReviewURL  string `json:"review_url"`
}

type ApprovalRegistry struct {
	Plugins []ApprovedPlugin `json:"plugins"`
}

func DecodeApprovalRegistry(data []byte) (ApprovalRegistry, error) {
	var registry ApprovalRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return ApprovalRegistry{}, fmt.Errorf("decode approval registry: %w", err)
	}
	seenPluginIDs := make(map[string]struct{}, len(registry.Plugins))
	seenRepositories := make(map[string]struct{}, len(registry.Plugins))
	for _, plugin := range registry.Plugins {
		pluginID := strings.TrimSpace(plugin.PluginID)
		repository := strings.TrimSpace(plugin.Repository)
		if pluginID == "" {
			return ApprovalRegistry{}, fmt.Errorf("approved plugin_id is required")
		}
		if repository == "" {
			return ApprovalRegistry{}, fmt.Errorf("approved repository is required for %s", pluginID)
		}
		if !strings.HasPrefix(strings.ToLower(repository), "silo-community/") {
			return ApprovalRegistry{}, fmt.Errorf("approved repository %q must belong to Silo-Community", repository)
		}
		if _, exists := seenPluginIDs[pluginID]; exists {
			return ApprovalRegistry{}, fmt.Errorf("approved plugin_id %q is duplicated", pluginID)
		}
		repositoryKey := strings.ToLower(repository)
		if _, exists := seenRepositories[repositoryKey]; exists {
			return ApprovalRegistry{}, fmt.Errorf("approved repository %q is duplicated", repository)
		}
		if _, err := time.Parse(time.DateOnly, plugin.ApprovedAt); err != nil {
			return ApprovalRegistry{}, fmt.Errorf("approved_at for %s must use YYYY-MM-DD: %w", pluginID, err)
		}
		if err := validateGitHubURL(plugin.ReviewURL); err != nil {
			return ApprovalRegistry{}, fmt.Errorf("review_url for %s: %w", pluginID, err)
		}
		seenPluginIDs[pluginID] = struct{}{}
		seenRepositories[repositoryKey] = struct{}{}
	}
	return registry, nil
}

func (r ApprovalRegistry) Resolve(repository, pluginID string) (Approval, error) {
	for _, approved := range r.Plugins {
		if approved.PluginID == pluginID && strings.EqualFold(approved.Repository, repository) {
			return Approval{ApprovedAt: approved.ApprovedAt, ReviewURL: approved.ReviewURL}, nil
		}
	}
	return Approval{}, fmt.Errorf("plugin %s from %s is not approved", pluginID, repository)
}

// ApprovedPluginID returns the plugin ID the registry binds to repo, or an
// error when repo is not approved. update-catalog calls it before any fetch.
// The comparison is exact, so every request and asset URL built from repo uses
// the spelling the registry records.
func (r ApprovalRegistry) ApprovedPluginID(repo string) (string, error) {
	for _, approved := range r.Plugins {
		if approved.Repository == repo {
			return approved.PluginID, nil
		}
	}
	return "", fmt.Errorf("repository %s is not in the approval registry", repo)
}

func ValidateApprovedIndex(index RepositoryIndex, registry ApprovalRegistry) error {
	seenPluginIDs := make(map[string]struct{}, len(index.Plugins))
	for _, pkg := range index.Plugins {
		if pkg.Manifest == nil {
			return fmt.Errorf("catalog package manifest is required")
		}
		pluginID := pkg.Manifest.GetPluginId()
		if _, exists := seenPluginIDs[pluginID]; exists {
			return fmt.Errorf("catalog plugin_id %q is duplicated", pluginID)
		}
		seenPluginIDs[pluginID] = struct{}{}
		repository := strings.TrimPrefix(pkg.RepoURL, "https://github.com/")
		approval, err := registry.Resolve(repository, pluginID)
		if err != nil {
			return err
		}
		if pkg.Approval == nil {
			return fmt.Errorf("catalog plugin %s is missing approval metadata", pluginID)
		}
		if *pkg.Approval != approval {
			return fmt.Errorf("catalog plugin %s approval metadata does not match the registry", pluginID)
		}
	}
	return nil
}

func validateGitHubURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") || parsed.Path == "" {
		return fmt.Errorf("must be an absolute https://github.com URL")
	}
	return nil
}
