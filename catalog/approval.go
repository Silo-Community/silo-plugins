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
