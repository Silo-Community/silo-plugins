package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Silo-Community/silo-plugins/catalog"
)

const (
	githubAPIVersion = "2022-11-28"

	maxBundleBytes = 256 << 10
	maxBinaryBytes = 512 << 20
)

func main() {
	var repo, tag, manifestPath, approvalsPath string
	var allowRollback bool

	flag.StringVar(&repo, "repo", "", "GitHub repository in owner/name form")
	flag.StringVar(&tag, "tag", "", "Release tag to publish, v<major>.<minor>.<patch>")
	flag.StringVar(&manifestPath, "manifest", "manifest.json", "Path to the catalog manifest")
	flag.StringVar(&approvalsPath, "approvals", "approved-plugins.json", "Path to the approved plugin registry")
	flag.BoolVar(&allowRollback, "allow-rollback", false, "Accept a version that is not newer than the listed one (deliberate rollback only)")
	flag.Parse()

	if err := run(context.Background(), repo, tag, manifestPath, approvalsPath, allowRollback); err != nil {
		exitf("%v", err)
	}
}

// run applies every check that needs no network first, then fetches the
// release, ties it to one commit (the signed one, or the tag's commit for an
// unsigned release), reads manifest.json at that commit, and only then writes
// the catalog.
func run(ctx context.Context, repo, tag, manifestPath, approvalsPath string, allowRollback bool) error {
	if err := catalog.ValidateInputs(repo, tag); err != nil {
		return fmt.Errorf("refuse release: %w", err)
	}
	registry, err := loadApprovals(approvalsPath)
	if err != nil {
		return fmt.Errorf("load approvals: %w", err)
	}
	approved, err := registry.ApprovedRepository(repo)
	if err != nil {
		return fmt.Errorf("refuse release: %w", err)
	}
	pluginID := approved.PluginID

	index, err := loadIndex(manifestPath)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}
	current := catalog.FindPackage(index, pluginID)
	version := catalog.VersionFromTag(tag)
	alreadyListed, err := catalog.CheckVersion(current, version, allowRollback)
	if err != nil {
		return fmt.Errorf("refuse release: %w", err)
	}
	if alreadyListed {
		fmt.Fprintf(os.Stderr, "The catalog already lists %s v%s; nothing to do.\n", pluginID, version)
		return nil
	}

	token := os.Getenv("GITHUB_TOKEN")
	client := &http.Client{Timeout: 30 * time.Second}
	getJSON := func(ctx context.Context, url string, dest any) error {
		return githubJSON(ctx, client, token, url, dest)
	}

	release, err := fetchRelease(ctx, getJSON, repo, tag)
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}
	if err := catalog.CheckRelease(tag, release); err != nil {
		return fmt.Errorf("refuse release: %w", err)
	}
	assets, err := catalog.ParseReleaseAssets(repo, release)
	if err != nil {
		return fmt.Errorf("refuse release: %w", err)
	}
	if err := catalog.CheckSignatureRatchet(current, assets.BundleURL); err != nil {
		return fmt.Errorf("refuse release: %w", err)
	}

	var commit string
	var checksums map[string]string
	if assets.BundleURL != "" {
		// Binaries are tens of megabytes, so downloads get more time than the
		// API calls.
		downloadClient := &http.Client{Timeout: 5 * time.Minute}
		commit, checksums, err = verifyRelease(ctx, downloadClient, repo, tag, approved.SignerWorkflow, assets)
		if err != nil {
			return fmt.Errorf("refuse release: verify signature: %w", err)
		}
		if approved.SignerWorkflow != "" {
			if err := checkOnDefaultBranch(ctx, getJSON, repo, commit); err != nil {
				return fmt.Errorf("refuse release: %w", err)
			}
		}
	} else {
		commit, err = resolveTagCommit(ctx, getJSON, repo, tag)
		if err != nil {
			return fmt.Errorf("resolve tag %s: %w", tag, err)
		}
	}

	sourceManifest, err := fetchSourceManifest(ctx, client, token, repo, commit)
	if err != nil {
		return fmt.Errorf("fetch source manifest: %w", err)
	}
	if err := approved.CheckManifestPluginID(sourceManifest.GetPluginId()); err != nil {
		return fmt.Errorf("refuse release: %w", err)
	}
	approval, err := registry.Resolve(repo, pluginID)
	if err != nil {
		return fmt.Errorf("refuse release: %w", err)
	}
	pkg, err := catalog.BuildPackageFromRelease(repo, sourceManifest, release)
	if err != nil {
		return fmt.Errorf("build catalog package: %w", err)
	}
	pkg.Approval = &approval
	// Pin each signed binary to the bytes gh verified. verifyRelease checked
	// every binary in assets, which are the ones the entry lists.
	for platform, checksum := range checksums {
		binary := pkg.Binaries[platform]
		binary.Checksum = checksum
		pkg.Binaries[platform] = binary
	}

	index = catalog.UpsertPackage(index, pkg)
	index = catalog.PruneUnapproved(index, registry)
	if err := writeIndex(manifestPath, index); err != nil {
		return fmt.Errorf("write catalog: %w", err)
	}
	return nil
}

// verifyRelease downloads the bundle and every catalogued binary, checks each
// binary against the bundle with gh, and returns the one commit all of them
// were built from and each binary's SHA-256 keyed by platform. Any failure
// refuses the release.
func verifyRelease(ctx context.Context, client *http.Client, repo, tag, signerWorkflow string, assets catalog.ReleaseAssets) (string, map[string]string, error) {
	if len(assets.Binaries) == 0 {
		return "", nil, fmt.Errorf("release has no plugin binaries")
	}
	dir, err := os.MkdirTemp("", "silo-plugin-verify-")
	if err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(dir)

	bundlePath := filepath.Join(dir, catalog.BundleAssetName)
	if _, err := download(ctx, client, assets.BundleURL, bundlePath, maxBundleBytes); err != nil {
		return "", nil, fmt.Errorf("download %s: %w", catalog.BundleAssetName, err)
	}

	platforms := make([]string, 0, len(assets.Binaries))
	for platform := range assets.Binaries {
		platforms = append(platforms, platform)
	}
	slices.Sort(platforms)
	var commit string
	checksums := make(map[string]string, len(platforms))
	for i, platform := range platforms {
		binaryPath := filepath.Join(dir, fmt.Sprintf("binary-%d", i))
		checksum, err := download(ctx, client, assets.Binaries[platform], binaryPath, maxBinaryBytes)
		if err != nil {
			return "", nil, fmt.Errorf("download %s binary: %w", platform, err)
		}
		cmd := exec.CommandContext(ctx, "gh", catalog.VerifyArgs(binaryPath, bundlePath, repo, tag, signerWorkflow)...)
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		if err != nil {
			return "", nil, fmt.Errorf("%s binary: gh attestation verify: %w", platform, err)
		}
		binaryCommit, err := catalog.CheckVerifyOutput(out)
		if err != nil {
			return "", nil, fmt.Errorf("%s binary: %w", platform, err)
		}
		if commit != "" && binaryCommit != commit {
			return "", nil, fmt.Errorf("%s binary was built from %s, other binaries from %s", platform, binaryCommit, commit)
		}
		commit = binaryCommit
		checksums[platform] = checksum
		fmt.Fprintf(os.Stderr, "Verified the %s binary of %s %s, built from %s, SHA-256 %s.\n", platform, repo, tag, commit, checksum)
	}
	return commit, checksums, nil
}

// getJSONFunc fetches a GitHub API URL and decodes the JSON response into
// dest. Tests replace it with a stub.
type getJSONFunc func(ctx context.Context, url string, dest any) error

// checkOnDefaultBranch refuses a commit that is not on repo's default branch.
// The default branch is looked up, not assumed.
func checkOnDefaultBranch(ctx context.Context, getJSON getJSONFunc, repo, commit string) error {
	var repository struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := getJSON(ctx, "https://api.github.com/repos/"+repo, &repository); err != nil {
		return fmt.Errorf("look up default branch: %w", err)
	}
	if repository.DefaultBranch == "" {
		return fmt.Errorf("repository %s reports no default branch", repo)
	}
	if err := catalog.ValidateCommit(commit); err != nil {
		return err
	}
	var comparison struct {
		Status string `json:"status"`
	}
	compareURL := "https://api.github.com/repos/" + repo + "/compare/" + url.PathEscape(repository.DefaultBranch) + "..." + commit
	if err := getJSON(ctx, compareURL, &comparison); err != nil {
		return fmt.Errorf("compare %s with %s: %w", commit, repository.DefaultBranch, err)
	}
	return catalog.CheckOnDefaultBranch(comparison.Status, repository.DefaultBranch, commit)
}

// resolveTagCommit returns the commit a tag points at, following annotated
// tags. An unsigned release has no certificate naming its commit, so this
// pins manifest.json to one commit rather than a ref a branch could shadow.
func resolveTagCommit(ctx context.Context, getJSON getJSONFunc, repo, tag string) (string, error) {
	var ref struct {
		Object gitObject `json:"object"`
	}
	if err := getJSON(ctx, "https://api.github.com/repos/"+repo+"/git/ref/tags/"+tag, &ref); err != nil {
		return "", err
	}
	object := ref.Object
	for range 5 {
		if err := catalog.ValidateCommit(object.SHA); err != nil {
			return "", err
		}
		switch object.Type {
		case "commit":
			return object.SHA, nil
		case "tag":
			var annotated struct {
				Object gitObject `json:"object"`
			}
			if err := getJSON(ctx, "https://api.github.com/repos/"+repo+"/git/tags/"+object.SHA, &annotated); err != nil {
				return "", err
			}
			object = annotated.Object
		default:
			return "", fmt.Errorf("tag points at a %s, not a commit", object.Type)
		}
	}
	return "", fmt.Errorf("tag is nested more than 5 annotated tags deep")
}

type gitObject struct {
	SHA  string `json:"sha"`
	Type string `json:"type"`
}

func loadApprovals(path string) (catalog.ApprovalRegistry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return catalog.ApprovalRegistry{}, fmt.Errorf("read %s: %w", path, err)
	}
	registry, err := catalog.DecodeApprovalRegistry(data)
	if err != nil {
		return catalog.ApprovalRegistry{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return registry, nil
}

// download saves url to path, refusing a body larger than limit, and returns
// the lowercase hex SHA-256 of the bytes written.
func download(ctx context.Context, client *http.Client, url, path string, limit int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "silo-plugins-catalog-updater")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, limit+1))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}
	if written > limit {
		return "", fmt.Errorf("GET %s: larger than %d bytes", url, limit)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fetchRelease(ctx context.Context, getJSON getJSONFunc, repo, tag string) (catalog.Release, error) {
	var release catalog.Release
	if err := getJSON(ctx, "https://api.github.com/repos/"+repo+"/releases/tags/"+tag, &release); err != nil {
		return catalog.Release{}, err
	}
	return release, nil
}

// fetchSourceManifest reads manifest.json at a validated commit SHA.
func fetchSourceManifest(ctx context.Context, client *http.Client, token, repo, commit string) (*catalog.SourceManifest, error) {
	url := "https://raw.githubusercontent.com/" + repo + "/" + commit + "/manifest.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "silo-plugins-catalog-updater")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read source manifest: %w", err)
	}
	return catalog.DecodeSourceManifest(body)
}

func githubJSON(ctx context.Context, client *http.Client, token, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "silo-plugins-catalog-updater")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

func loadIndex(path string) (catalog.RepositoryIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return catalog.RepositoryIndex{}, fmt.Errorf("read %s: %w", path, err)
	}
	var index catalog.RepositoryIndex
	if len(bytes.TrimSpace(data)) == 0 {
		return index, nil
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return catalog.RepositoryIndex{}, fmt.Errorf("decode %s: %w", path, err)
	}
	return index, nil
}

func writeIndex(path string, index catalog.RepositoryIndex) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
