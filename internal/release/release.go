// Package release downloads, verifies, activates and rolls back Selfishell CLI
// releases in the versioned install layout.
package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/jiminu/selfishell/internal/managed"
	"github.com/jiminu/selfishell/internal/process"
)

// Operation downloads and switches releases for the installation at Root.
// Process runs curl and carries the output and warning streams.
type Operation struct {
	Root    string
	Process process.Process
	link    func(target, path string) error
}

var releaseVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?$`)

// ValidVersion reports whether version is an accepted release version:
// SemVer without build metadata or leading zeros in numeric identifiers.
func ValidVersion(version string) bool {
	if !releaseVersionPattern.MatchString(version) {
		return false
	}
	_, prerelease, found := strings.Cut(version, "-")
	if !found {
		return true
	}
	for _, id := range strings.Split(prerelease, ".") {
		if len(id) > 1 && id[0] == '0' && strings.Trim(id, "0123456789") == "" {
			return false
		}
	}
	return true
}

// CompareVersions follows SemVer precedence for accepted release versions.
// Invalid inputs compare equal; callers validate user and remote input first.
func CompareVersions(a, b string) int {
	if !ValidVersion(a) || !ValidVersion(b) {
		return 0
	}
	baseA, preA, hasA := strings.Cut(a, "-")
	baseB, preB, hasB := strings.Cut(b, "-")
	aa, bb := strings.Split(baseA, "."), strings.Split(baseB, ".")
	for i := range aa {
		if n := compareNumeric(aa[i], bb[i]); n != 0 {
			return n
		}
	}
	if !hasA && hasB {
		return 1
	}
	if hasA && !hasB {
		return -1
	}
	if !hasA {
		return 0
	}
	aa, bb = strings.Split(preA, "."), strings.Split(preB, ".")
	for i := 0; i < len(aa) && i < len(bb); i++ {
		aNumeric, bNumeric := isDigits(aa[i]), isDigits(bb[i])
		if aNumeric && !bNumeric {
			return -1
		}
		if !aNumeric && bNumeric {
			return 1
		}
		var n int
		if aNumeric {
			n = compareNumeric(aa[i], bb[i])
		} else {
			n = strings.Compare(aa[i], bb[i])
		}
		if n != 0 {
			return n
		}
	}
	if len(aa) < len(bb) {
		return -1
	}
	if len(aa) > len(bb) {
		return 1
	}
	return 0
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func compareNumeric(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}

const officialReleaseRoot = "https://github.com/jiminu/selfishell/releases"

func releaseRootURL(env process.Environment) string {
	if s := env.Get("SELFISHELL_RELEASE_ROOT"); s != "" {
		return strings.TrimRight(s, "/")
	}
	return officialReleaseRoot
}

func (o Operation) fetch(ctx context.Context, mode, url, destination string, headers ...string) error {
	args := []string{}
	for _, h := range headers {
		args = append(args, "-H", h)
	}
	args = append(args, url, "-o", destination)
	code, err := o.Process.Curl(ctx, mode, args...)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("curl exited %d for %s", code, url)
	}
	return nil
}

// Latest returns the newest published release version from the latest
// VERSION asset, else from the newest tag, whose VERSION asset must agree.
func (o Operation) Latest(ctx context.Context) (string, error) {
	root := releaseRootURL(o.Process.Environment())
	dir, err := os.MkdirTemp("", "selfishell-release-metadata.")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	versionFile := dir + "/VERSION"
	if err = o.fetch(ctx, "metadata", root+"/latest/download/VERSION", versionFile); err == nil {
		data, e := os.ReadFile(versionFile)
		if e == nil {
			if v, ok := parseReleaseVersion(data); ok {
				return v, nil
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	api := o.Process.Environment().Get("SELFISHELL_RELEASE_TAGS_API_URL")
	if root != officialReleaseRoot && api == "" {
		return "", fmt.Errorf("unable to determine latest Selfishell release")
	}
	if api == "" {
		api = "https://api.github.com/repos/jiminu/selfishell/tags?per_page=1"
	}
	tags := dir + "/tags.json"
	if err = o.fetch(ctx, "metadata", api, tags, "Accept: application/vnd.github+json", "X-GitHub-Api-Version: 2022-11-28"); err != nil {
		return "", err
	}
	version, err := releaseTagVersion(tags)
	if err != nil {
		return "", err
	}
	if err = o.fetch(ctx, "metadata", root+"/download/v"+version+"/VERSION", versionFile); err != nil {
		return "", err
	}
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return "", err
	}
	published, ok := parseReleaseVersion(data)
	if !ok || published != version {
		return "", fmt.Errorf("release tag VERSION mismatch")
	}
	return version, nil
}

func parseReleaseVersion(data []byte) (string, bool) {
	s := strings.TrimSuffix(string(data), "\n")
	s = strings.TrimPrefix(s, "v")
	return s, ValidVersion(s)
}

func releaseTagVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var tags []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &tags); err != nil {
		return "", err
	}
	if len(tags) == 0 {
		return "", fmt.Errorf("release tags are empty")
	}
	version := strings.TrimPrefix(tags[0].Name, "v")
	if !ValidVersion(version) {
		return "", fmt.Errorf("invalid release tag: %s", tags[0].Name)
	}
	return version, nil
}

// ArchiveName names the release asset for this platform and architecture.
func ArchiveName(version string) (string, error) {
	platform := map[string]string{"darwin": "macos", "linux": "linux"}[runtime.GOOS]
	arch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[runtime.GOARCH]
	if platform == "" || arch == "" {
		return "", fmt.Errorf("unsupported release platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return fmt.Sprintf("selfishell-%s-%s-%s.tar.gz", version, platform, arch), nil
}

// Layout keeps the executable's resolved root independent of cwd and current.
// Previous is the path of the previous-release link.
type Layout struct{ share, releases, current, Previous string }

// Installed resolves the versioned layout around root, the running release
// directory, and rejects a development checkout or an incomplete release.
func Installed(root string) (Layout, error) {
	var l Layout
	if root == "" {
		return l, fmt.Errorf("This command requires a versioned Selfishell installation.")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return l, err
	}
	l.releases = filepath.Dir(resolved)
	l.share = filepath.Dir(l.releases)
	if filepath.Base(l.releases) != "releases" || !ValidVersion(filepath.Base(resolved)) {
		return l, fmt.Errorf("This command requires a versioned Selfishell installation.")
	}
	if _, err := Directory(l.releases, filepath.Base(resolved)); err != nil {
		return l, fmt.Errorf("This command requires a versioned Selfishell installation: %w", err)
	}
	l.current = l.share + "/current"
	l.Previous = l.share + "/previous"
	info, err := os.Lstat(l.current)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return l, fmt.Errorf("This command requires a versioned Selfishell installation.")
	}
	return l, nil
}

// Directory returns releases/<version> once it is a complete release: a real
// directory with an executable bin/selfishell and a matching VERSION.
func Directory(releases, version string) (string, error) {
	if !ValidVersion(version) {
		return "", fmt.Errorf("invalid semantic version: %s", version)
	}
	dir := releases + "/" + version
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("invalid retained release directory: %s", dir)
	}
	exe, err := os.Stat(dir + "/bin/selfishell")
	if err != nil {
		return "", err
	}
	if !exe.Mode().IsRegular() || exe.Mode()&0111 == 0 {
		return "", fmt.Errorf("invalid retained release executable: %s", dir)
	}
	data, err := os.ReadFile(dir + "/VERSION")
	if err != nil {
		return "", err
	}
	if strings.TrimSuffix(string(data), "\n") != version {
		return "", fmt.Errorf("retained release version mismatch: %s", dir)
	}
	return dir, nil
}

// Retained validates a previous-release link target exactly as
// rollback accepts it: releases/<version> naming a complete release.
func Retained(releases, link string) (string, error) {
	version := strings.TrimPrefix(link, "releases/")
	if link != "releases/"+version || !ValidVersion(version) {
		return "", fmt.Errorf("invalid retained release link: %s", link)
	}
	if _, err := Directory(releases, version); err != nil {
		return "", err
	}
	return version, nil
}

func atomicReleaseLink(target, path string) error {
	if err := releaseLinkReplaceable(path); err != nil {
		return err
	}
	f, err := managed.CreateRawTemp(path)
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	if err := os.Symlink(target, name); err != nil {
		return err
	}
	defer os.Remove(name)
	return os.Rename(name, path)
}

func releaseLinkReplaceable(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("occupied release link: %s", path)
		}
		target, err := os.Readlink(path)
		if err != nil {
			return err
		}
		version := strings.TrimPrefix(target, "releases/")
		if target != "releases/"+version || !ValidVersion(version) {
			return fmt.Errorf("foreign release link: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func pruneInactiveReleases(l Layout) {
	current, err := os.Readlink(l.current)
	if err != nil || !validReleaseLinkTarget(current) {
		return
	}
	keep := map[string]bool{filepath.Base(current): true}
	previous, err := os.Readlink(l.Previous)
	if err == nil {
		if !validReleaseLinkTarget(previous) {
			return
		}
		keep[filepath.Base(previous)] = true
	} else if !os.IsNotExist(err) {
		return
	}
	entries, err := os.ReadDir(l.releases)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		info, err := entry.Info()
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		path := l.releases + "/" + name
		stagingVersion, stagingSuffix, stagingName := strings.Cut(strings.TrimPrefix(name, "."), ".tmp.")
		if strings.HasPrefix(name, ".") && stagingName && stagingSuffix != "" && ValidVersion(stagingVersion) {
			if time.Since(info.ModTime()) > 24*time.Hour {
				_ = os.RemoveAll(path)
			}
			continue
		}
		if ValidVersion(name) && !keep[name] {
			if _, err := Directory(l.releases, name); err == nil {
				_ = os.RemoveAll(path)
			}
		}
	}
}

func validReleaseLinkTarget(target string) bool {
	version := strings.TrimPrefix(target, "releases/")
	return target == "releases/"+version && ValidVersion(version)
}

func selectedChecksum(data []byte, name string) (string, error) {
	var selected string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			if len(fields[0]) != 64 {
				return "", fmt.Errorf("malformed checksum for %s", name)
			}
			if _, err := hex.DecodeString(fields[0]); err != nil {
				return "", fmt.Errorf("malformed checksum for %s", name)
			}
			value := strings.ToLower(fields[0])
			if selected != "" && selected != value {
				return "", fmt.Errorf("conflicting checksums for %s", name)
			}
			selected = value
		} else if len(fields) == 1 && fields[0] == name || len(fields) >= 2 && strings.TrimPrefix(fields[1], "*") == name {
			return "", fmt.Errorf("malformed checksum for %s", name)
		}
	}
	if selected == "" {
		return "", fmt.Errorf("missing checksum for %s", name)
	}
	return selected, nil
}

// Install downloads, verifies and activates version, retains the replaced
// release as previous and returns the activated release directory.
func (o Operation) Install(ctx context.Context, version string) (string, error) {
	if !ValidVersion(version) {
		return "", fmt.Errorf("invalid semantic version: %s", version)
	}
	l, err := Installed(o.Root)
	if err != nil {
		return "", err
	}
	name, err := ArchiveName(version)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Preflight occupied user paths before contacting a release source.
	if err := releaseLinkReplaceable(l.current); err != nil {
		return "", err
	}
	if err := releaseLinkReplaceable(l.Previous); err != nil {
		return "", err
	}
	target := l.releases + "/" + version
	if _, err := os.Lstat(target); err == nil {
		if _, err := Directory(l.releases, version); err != nil {
			return "", fmt.Errorf("Existing release is incomplete: %s: %w", target, err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	download, err := os.MkdirTemp("", "selfishell-update.")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(download)
	root := releaseRootURL(o.Process.Environment()) + "/download/v" + version
	archive := download + "/" + name
	checksums := download + "/SHA256SUMS"
	if err := o.fetch(ctx, "transfer", root+"/"+name, archive); err != nil {
		return "", err
	}
	if err := o.fetch(ctx, "transfer", root+"/SHA256SUMS", checksums); err != nil {
		return "", err
	}
	data, err := os.ReadFile(checksums)
	if err != nil {
		return "", err
	}
	expected, err := selectedChecksum(data, name)
	if err != nil {
		return "", err
	}
	file, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return "", fmt.Errorf("Checksum mismatch for %s.", name)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		staging, err := os.MkdirTemp(l.releases, "."+version+".tmp.")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(staging)
		payload := staging + "/payload"
		if err := extractReleaseArchive(archive, payload); err != nil {
			return "", err
		}
		if err := validateReleasePayload(payload, version); err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := os.Lstat(target); os.IsNotExist(err) {
			if err := os.Rename(payload, target); err != nil {
				if _, validErr := Directory(l.releases, version); validErr != nil {
					return "", err
				}
			}
		} else if err != nil {
			return "", err
		}
	} else if _, err := scanReleaseArchive(archive); err != nil {
		// Reusing a release skips extraction, but must still validate the download.
		return "", err
	}
	if _, err := Directory(l.releases, version); err != nil {
		return "", fmt.Errorf("Existing release is incomplete: %s: %w", target, err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	old, err := os.Readlink(l.current)
	if err != nil {
		return "", err
	}
	previous, previousErr := os.Readlink(l.Previous)
	if previousErr != nil && !os.IsNotExist(previousErr) {
		return "", fmt.Errorf("Failed to retain previous Selfishell release: %w", previousErr)
	}
	link := o.link
	if link == nil {
		link = atomicReleaseLink
	}
	if old != "releases/"+version {
		if err := link(old, l.Previous); err != nil {
			return "", fmt.Errorf("Failed to retain previous Selfishell release: %w", err)
		}
	}
	if err := link("releases/"+version, l.current); err != nil {
		if old != "releases/"+version {
			var restoreErr error
			if got, readErr := os.Readlink(l.Previous); readErr != nil || got != old {
				restoreErr = fmt.Errorf("previous release link changed during activation")
			} else if previousErr == nil {
				restoreErr = atomicReleaseLink(previous, l.Previous)
			} else {
				restoreErr = os.Remove(l.Previous)
			}
			if restoreErr != nil && o.Process.Err != nil {
				if o.Process.Progress != nil {
					o.Process.Progress.Pause()
				}
				fmt.Fprintln(o.Process.Err, "selfishell: warning: Failed to restore the previous release link: "+restoreErr.Error())
			}
		}
		return "", fmt.Errorf("Failed to activate Selfishell %s: %w", version, err)
	}
	pruneInactiveReleases(l)
	return target, nil
}

func validateReleasePayload(dir, version string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("invalid release payload")
	}
	exe, err := os.Stat(dir + "/bin/selfishell")
	if err != nil {
		return err
	}
	if !exe.Mode().IsRegular() || exe.Mode()&0111 == 0 {
		return fmt.Errorf("invalid release executable")
	}
	data, err := os.ReadFile(dir + "/VERSION")
	if err != nil {
		return err
	}
	if strings.TrimSuffix(string(data), "\n") != version {
		return fmt.Errorf("release archive has wrong VERSION")
	}
	return nil
}
