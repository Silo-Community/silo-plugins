package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Silo-Community/silo-plugins/catalog"
)

const (
	testRepo   = "Silo-Community/silo-plugins-requests-arr"
	testCommit = "29fd28771a006d9d68a311865a2b858c5afe301a"
	testTagSHA = "63837fb778d6a39f02be0ce76ff89f2bc8d74bdc"
	apiPrefix  = "https://api.github.com/repos/" + testRepo
)

// stubGitHub answers getJSON from a map of URL to JSON body. A URL not in
// the map fails the request, as an unexpected call should.
func stubGitHub(t *testing.T, responses map[string]string) getJSONFunc {
	t.Helper()
	return func(_ context.Context, url string, dest any) error {
		body, ok := responses[url]
		if !ok {
			return fmt.Errorf("unexpected request %s", url)
		}
		return json.Unmarshal([]byte(body), dest)
	}
}

func TestVerifyReleaseRefusesNoBinaries(t *testing.T) {
	assets := catalog.ReleaseAssets{
		BundleURL: "https://github.com/" + testRepo + "/releases/download/v0.1.11/" + catalog.BundleAssetName,
		Binaries:  map[string]string{},
	}
	// The client is never used: the refusal comes before any download.
	if _, _, err := verifyRelease(context.Background(), nil, testRepo, "v0.1.11", "", assets); err == nil {
		t.Fatal("verifyRelease() accepted a signed release with no binaries")
	}
}

func TestDownloadSizeCapAndChecksum(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "hello")
	}))
	defer server.Close()
	dir := t.TempDir()

	checksum, err := download(context.Background(), server.Client(), server.URL, filepath.Join(dir, "exact"), 5)
	if err != nil {
		t.Fatalf("download() at the limit error = %v", err)
	}
	// SHA-256 of "hello", lowercase hex.
	if want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"; checksum != want {
		t.Fatalf("download() checksum = %q, want %q", checksum, want)
	}

	if _, err := download(context.Background(), server.Client(), server.URL, filepath.Join(dir, "over"), 4); err == nil || !strings.Contains(err.Error(), "larger than 4 bytes") {
		t.Fatalf("download() over the limit error = %v, want the size refusal", err)
	}
}

func TestResolveTagCommit(t *testing.T) {
	refURL := apiPrefix + "/git/ref/tags/v0.1.11"
	tagURL := func(sha string) string { return apiPrefix + "/git/tags/" + sha }
	object := func(typ, sha string) string {
		return `{"object":{"type":"` + typ + `","sha":"` + sha + `"}}`
	}

	t.Run("lightweight tag", func(t *testing.T) {
		got, err := resolveTagCommit(context.Background(), stubGitHub(t, map[string]string{refURL: object("commit", testCommit)}), testRepo, "v0.1.11")
		if err != nil || got != testCommit {
			t.Fatalf("resolveTagCommit() = %q, %v; want %s", got, err, testCommit)
		}
	})
	t.Run("annotated tag", func(t *testing.T) {
		got, err := resolveTagCommit(context.Background(), stubGitHub(t, map[string]string{
			refURL:             object("tag", testTagSHA),
			tagURL(testTagSHA): object("commit", testCommit),
		}), testRepo, "v0.1.11")
		if err != nil || got != testCommit {
			t.Fatalf("resolveTagCommit() = %q, %v; want %s", got, err, testCommit)
		}
	})
	t.Run("annotated tags nested too deep", func(t *testing.T) {
		// Every tag object points at itself, so resolution never reaches a
		// commit and must stop at the nesting limit.
		_, err := resolveTagCommit(context.Background(), stubGitHub(t, map[string]string{
			refURL:             object("tag", testTagSHA),
			tagURL(testTagSHA): object("tag", testTagSHA),
		}), testRepo, "v0.1.11")
		if err == nil || !strings.Contains(err.Error(), "nested") {
			t.Fatalf("resolveTagCommit() error = %v, want the nesting refusal", err)
		}
	})
	t.Run("tag of a tree", func(t *testing.T) {
		if _, err := resolveTagCommit(context.Background(), stubGitHub(t, map[string]string{refURL: object("tree", testCommit)}), testRepo, "v0.1.11"); err == nil {
			t.Fatal("resolveTagCommit() accepted a tag that points at a tree")
		}
	})
	t.Run("malformed object SHA", func(t *testing.T) {
		if _, err := resolveTagCommit(context.Background(), stubGitHub(t, map[string]string{refURL: object("commit", "../../x")}), testRepo, "v0.1.11"); err == nil {
			t.Fatal("resolveTagCommit() accepted a malformed SHA")
		}
	})
}

func TestCheckOnDefaultBranch(t *testing.T) {
	compareURL := func(branch string) string { return apiPrefix + "/compare/" + branch + "..." + testCommit }
	tests := []struct {
		name      string
		responses map[string]string
		wantErr   bool
	}{
		{"commit is the default branch head", map[string]string{
			apiPrefix:          `{"default_branch":"main"}`,
			compareURL("main"): `{"status":"identical"}`,
		}, false},
		{"commit is behind the default branch", map[string]string{
			apiPrefix:          `{"default_branch":"main"}`,
			compareURL("main"): `{"status":"behind"}`,
		}, false},
		{"default branch is looked up, not assumed", map[string]string{
			apiPrefix:           `{"default_branch":"trunk"}`,
			compareURL("trunk"): `{"status":"behind"}`,
		}, false},
		{"branch name with a slash is escaped", map[string]string{
			apiPrefix:                   `{"default_branch":"release/1.x"}`,
			compareURL("release%2F1.x"): `{"status":"identical"}`,
		}, false},
		{"commit ahead of the default branch", map[string]string{
			apiPrefix:          `{"default_branch":"main"}`,
			compareURL("main"): `{"status":"ahead"}`,
		}, true},
		{"commit on a diverged branch", map[string]string{
			apiPrefix:          `{"default_branch":"main"}`,
			compareURL("main"): `{"status":"diverged"}`,
		}, true},
		{"no default branch", map[string]string{
			apiPrefix: `{}`,
		}, true},
		{"compare request fails", map[string]string{
			apiPrefix: `{"default_branch":"main"}`,
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkOnDefaultBranch(context.Background(), stubGitHub(t, tt.responses), testRepo, testCommit)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkOnDefaultBranch() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
