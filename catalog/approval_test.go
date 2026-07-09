package catalog

import "testing"

func TestDecodeApprovalRegistryAndResolve(t *testing.T) {
	registry, err := DecodeApprovalRegistry([]byte(`{
  "plugins": [{
    "plugin_id": "silo.requests.arr",
    "repository": "Silo-Community/silo-plugins-requests-arr",
    "approved_at": "2026-07-09",
    "review_url": "https://github.com/Silo-Community/silo-plugins/issues/1"
  }]
}`))
	if err != nil {
		t.Fatalf("DecodeApprovalRegistry() error = %v", err)
	}
	approval, err := registry.Resolve("silo-community/silo-plugins-requests-arr", "silo.requests.arr")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if approval.ApprovedAt != "2026-07-09" {
		t.Fatalf("ApprovedAt = %q", approval.ApprovedAt)
	}
}

func TestDecodeApprovalRegistryRejectsUnapprovedOwner(t *testing.T) {
	_, err := DecodeApprovalRegistry([]byte(`{
  "plugins": [{
    "plugin_id": "silo.bad",
    "repository": "someone/plugin",
    "approved_at": "2026-07-09",
    "review_url": "https://github.com/someone/plugin/issues/1"
  }]
}`))
	if err == nil {
		t.Fatal("DecodeApprovalRegistry() unexpectedly succeeded")
	}
}

func TestPruneUnapproved(t *testing.T) {
	registry, err := DecodeApprovalRegistry([]byte(`{
  "plugins": [{
    "plugin_id": "silo.requests.arr",
    "repository": "Silo-Community/silo-plugins-requests-arr",
    "approved_at": "2026-07-09",
    "review_url": "https://github.com/Silo-Community/silo-plugins/issues/1"
  }]
}`))
	if err != nil {
		t.Fatalf("DecodeApprovalRegistry() error = %v", err)
	}
	index := RepositoryIndex{Plugins: []CatalogPackage{
		{
			Manifest: &SourceManifest{PluginId: "silo.requests.arr"},
			RepoURL:  "https://github.com/Silo-Community/silo-plugins-requests-arr",
		},
		{
			Manifest: &SourceManifest{PluginId: "silo.unapproved"},
			RepoURL:  "https://github.com/Silo-Community/unapproved",
		},
	}}
	got := PruneUnapproved(index, registry)
	if len(got.Plugins) != 1 {
		t.Fatalf("Plugins length = %d, want 1", len(got.Plugins))
	}
	if got.Plugins[0].Approval == nil {
		t.Fatal("approved package is missing approval metadata")
	}
}
