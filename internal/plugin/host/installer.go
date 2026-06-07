package host

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ManifestFileName is the required name for a plugin manifest inside the archive.
const ManifestFileName = "virta-plugin.json"

// Installer fetches, verifies, and extracts remote plugins from Git URLs / release archives.
type Installer struct {
	cacheDir string // base directory where plugins are installed
	client   *http.Client
}

// NewInstaller creates an Installer that stores plugins under cacheDir.
func NewInstaller(cacheDir string) *Installer {
	return &Installer{
		cacheDir: cacheDir,
		client:   &http.Client{Timeout: 60 * time.Second},
	}
}

// InstallResult contains the outcome of a successful installation.
type InstallResult struct {
	Manifest   *Manifest
	InstallDir string
	Digest     string // SHA-256 hex of the downloaded archive
}

// Install fetches the plugin from rawURL, verifies the manifest, and extracts it to the cache.
// rawURL may be:
//   - A direct HTTPS URL to a .zip or .tar.gz archive
//   - A GitHub repo URL (github.com/user/repo → resolves to latest release .zip)
//   - A GitHub release URL
func (inst *Installer) Install(ctx context.Context, rawURL string) (*InstallResult, error) {
	archiveURL, err := inst.resolveURL(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("installer: resolve URL: %w", err)
	}

	data, digest, err := inst.download(ctx, archiveURL)
	if err != nil {
		return nil, fmt.Errorf("installer: download: %w", err)
	}

	manifest, files, err := inst.extractManifest(archiveURL, data)
	if err != nil {
		return nil, fmt.Errorf("installer: extract: %w", err)
	}

	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("installer: manifest invalid: %w", err)
	}

	// Reject built-in-only ID collisions.
	if manifest.BuiltIn {
		return nil, errors.New("installer: remote plugins may not declare built_in")
	}

	// Write to cache directory, keyed by plugin id + digest prefix.
	installDir := filepath.Join(inst.cacheDir, manifest.ID+"@"+digest[:12])
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		return nil, fmt.Errorf("installer: create dir: %w", err)
	}
	// canonInstall is the canonical form of installDir with a guaranteed trailing sep,
	// so filepath.Abs + HasPrefix is immune to the "/home/user/plugins-evil" edge case.
	canonInstall := filepath.Clean(installDir) + string(os.PathSeparator)
	for name, content := range files {
		// Resolve the target path absolutely so symlinks and ".." cannot escape.
		rel := filepath.Clean(name)
		if rel == "." || strings.HasPrefix(rel, "..") {
			continue // reject relative escapes
		}
		path := filepath.Join(installDir, rel)
		// Canonical check after Join+Clean to defeat all traversal variants.
		if !strings.HasPrefix(filepath.Clean(path)+string(os.PathSeparator), canonInstall) &&
			filepath.Clean(path) != filepath.Clean(installDir) {
			continue // path traversal guard
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			return nil, err
		}
	}

	return &InstallResult{
		Manifest:   manifest,
		InstallDir: installDir,
		Digest:     digest,
	}, nil
}

// Uninstall removes the plugin's cache directory.
func (inst *Installer) Uninstall(installDir string) error {
	if installDir == "" || installDir == inst.cacheDir {
		return errors.New("installer: refusing to remove root cache dir")
	}
	return os.RemoveAll(installDir)
}

// resolveURL turns a GitHub repo URL into a release archive URL, or passes through others.
func (inst *Installer) resolveURL(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("only HTTPS URLs are allowed (got %q)", u.Scheme)
	}

	// GitHub repo URL: resolve to latest release .zip
	if u.Host == "github.com" && !strings.Contains(u.Path, "/releases/") && !strings.HasSuffix(u.Path, ".zip") && !strings.HasSuffix(u.Path, ".tar.gz") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 2 {
			return "", errors.New("invalid GitHub URL")
		}
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", parts[0], parts[1])
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := inst.client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("GitHub API %d for %s", resp.StatusCode, apiURL)
		}
		var rel struct {
			ZipballURL string `json:"zipball_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
			return "", err
		}
		if rel.ZipballURL == "" {
			return "", errors.New("no release found")
		}
		return rel.ZipballURL, nil
	}
	return rawURL, nil
}

// download fetches the archive and returns its bytes plus a SHA-256 hex digest.
func (inst *Installer) download(ctx context.Context, archiveURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := inst.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, archiveURL)
	}

	const maxSize = 32 << 20 // 32 MB max archive
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxSize {
		return nil, "", errors.New("archive exceeds 32 MB limit")
	}

	h := sha256.Sum256(data)
	return data, hex.EncodeToString(h[:]), nil
}

// extractManifest unpacks a .zip or .tar.gz archive, finds virta-plugin.json, and returns
// the parsed manifest plus a flat map of filename → content for all extracted files.
func (inst *Installer) extractManifest(archiveURL string, data []byte) (*Manifest, map[string][]byte, error) {
	lower := strings.ToLower(archiveURL)
	if strings.Contains(lower, ".tar.gz") || strings.Contains(lower, ".tgz") {
		return inst.extractTarGz(data)
	}
	return inst.extractZip(data)
}

func (inst *Installer) extractZip(data []byte) (*Manifest, map[string][]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, err
	}
	files := map[string][]byte{}
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// Strip leading directory component (GitHub adds repo-sha prefix).
		name := stripFirstDir(f.Name)
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 4<<20))
		rc.Close()
		if err != nil {
			return nil, nil, err
		}
		files[name] = b
	}
	return parseManifestFromFiles(files)
}

func (inst *Installer) extractTarGz(data []byte) (*Manifest, map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		name := stripFirstDir(hdr.Name)
		b, err := io.ReadAll(io.LimitReader(tr, 4<<20))
		if err != nil {
			return nil, nil, err
		}
		files[name] = b
	}
	return parseManifestFromFiles(files)
}

func parseManifestFromFiles(files map[string][]byte) (*Manifest, map[string][]byte, error) {
	raw, ok := files[ManifestFileName]
	if !ok {
		return nil, nil, fmt.Errorf("archive does not contain %s", ManifestFileName)
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return nil, nil, err
	}
	return m, files, nil
}

func stripFirstDir(p string) string {
	parts := strings.SplitN(filepath.ToSlash(p), "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return p
}
