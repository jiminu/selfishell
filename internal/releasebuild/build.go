package releasebuild

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jiminu/selfishell/internal/selfishell"
)

var targets = []struct{ platform, goos, arch string }{
	{"linux", "linux", "amd64"}, {"linux", "linux", "arm64"},
	{"macos", "darwin", "amd64"}, {"macos", "darwin", "arm64"},
}

// Build creates the four release archives, checksums, and VERSION in output.
// Relative output paths are resolved against root; an empty output uses root/dist.
func Build(ctx context.Context, root, version, output string) error {
	if !selfishell.ValidReleaseVersion(version) {
		return fmt.Errorf("a valid semantic version is required")
	}
	var err error
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	if output == "" {
		output = "dist"
	}
	if !filepath.IsAbs(output) {
		output = filepath.Join(root, output)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("Go toolchain required on PATH: %w", err)
	}
	pinned, err := pinnedGo(root)
	if err != nil {
		return err
	}
	env := controlledEnv(os.Environ())
	check := exec.CommandContext(ctx, goPath, "env", "GOVERSION")
	check.Dir, check.Env = root, env
	got, err := check.Output()
	if err != nil {
		return fmt.Errorf("check Go toolchain: %w", err)
	}
	if strings.TrimSpace(string(got)) != "go"+pinned {
		return fmt.Errorf("Go %s required on PATH; found %s", pinned, strings.TrimSpace(string(got)))
	}
	stageParent := output
	if info, statErr := os.Stat(output); os.IsNotExist(statErr) {
		stageParent = filepath.Dir(output)
		if err := os.MkdirAll(stageParent, 0755); err != nil {
			return err
		}
	} else if statErr != nil {
		return statErr
	} else if !info.IsDir() {
		return fmt.Errorf("output path is not a directory: %s", output)
	}
	stage, err := os.MkdirTemp(stageParent, "selfishell-native-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	var checksums strings.Builder
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		binary := filepath.Join(stage, "selfishell-"+target.goos+"-"+target.arch)
		cmd := exec.CommandContext(ctx, goPath, "build", "-trimpath", "-buildvcs=false", "-ldflags=-buildid=", "-o", binary, "./cmd/selfishell")
		cmd.Dir = root
		cmd.Env = append(append([]string{}, env...), "GOOS="+target.goos, "GOARCH="+target.arch)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("build %s/%s: %w: %s", target.goos, target.arch, err, strings.TrimSpace(string(output)))
		}
		name := fmt.Sprintf("selfishell-%s-%s-%s.tar.gz", version, target.platform, target.arch)
		archive := filepath.Join(stage, name)
		if err := writeArchive(root, binary, version, archive); err != nil {
			return fmt.Errorf("archive %s: %w", name, err)
		}
		contents, err := os.ReadFile(archive)
		if err != nil {
			return err
		}
		fmt.Fprintf(&checksums, "%x  %s\n", sha256.Sum256(contents), name)
		names = append(names, name)
	}
	if err := os.WriteFile(filepath.Join(stage, "SHA256SUMS"), []byte(checksums.String()), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "VERSION"), []byte(version+"\n"), 0644); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		return err
	}
	// Invalidate any previous manifest before replacing artifacts in an existing output.
	if err := os.Remove(filepath.Join(output, "SHA256SUMS")); err != nil && !os.IsNotExist(err) {
		return err
	}
	// The checksum manifest is moved last, so failed builds never publish a verified set.
	for _, name := range append(names, "VERSION", "SHA256SUMS") {
		from, to := filepath.Join(stage, name), filepath.Join(output, name)
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("publish %s: %w", name, err)
		}
	}
	return nil
}

func pinnedGo(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "go" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("go.mod has no pinned Go version")
}

func controlledEnv(in []string) []string {
	excluded := map[string]bool{"GOOS": true, "GOARCH": true, "GOFLAGS": true, "CGO_ENABLED": true, "GOAMD64": true, "GOARM64": true, "GOTOOLCHAIN": true, "GOENV": true, "GOWORK": true, "GOEXPERIMENT": true, "GOPROXY": true, "GOSUMDB": true}
	out := make([]string, 0, len(in)+10)
	for _, item := range in {
		key, _, _ := strings.Cut(item, "=")
		if !excluded[key] {
			out = append(out, item)
		}
	}
	return append(out, "CGO_ENABLED=0", "GOAMD64=v1", "GOARM64=v8.0", "GOTOOLCHAIN=local", "GOENV=off", "GOWORK=off", "GOEXPERIMENT=none", "GOPROXY=off", "GOSUMDB=off", "GOFLAGS=")
}

type member struct {
	name, source, link string
	mode               fs.FileMode
	data               []byte
}

func writeArchive(root, binary, version, destination string) (err error) {
	members := []member{{name: "bin/selfishell", source: binary, mode: 0755}, {name: "bin/sfs", link: "selfishell", mode: os.ModeSymlink | 0777}, {name: "VERSION", data: []byte(version + "\n"), mode: 0644}}
	for _, name := range []string{"packages.conf", "dependencies.conf"} {
		info, e := os.Lstat(filepath.Join(root, name))
		if e != nil {
			return e
		}
		members = append(members, member{name: name, source: filepath.Join(root, name), mode: info.Mode()})
	}
	err = filepath.WalkDir(filepath.Join(root, "config"), func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		info, e := os.Lstat(path)
		if e != nil {
			return e
		}
		m := member{name: filepath.ToSlash(rel), source: path, mode: info.Mode()}
		if info.Mode()&os.ModeSymlink != 0 {
			m.link, e = os.Readlink(path)
			if e != nil {
				return e
			}
		}
		if !info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("unsupported payload type: %s", path)
		}
		members = append(members, m)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(members, func(i, j int) bool { return members[i].name < members[j].name })
	f, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
	}()
	gz := gzip.NewWriter(f)
	gz.ModTime = time.Unix(946684800, 0).UTC()
	gz.OS = 255
	defer func() {
		if e := gz.Close(); err == nil {
			err = e
		}
	}()
	tw := tar.NewWriter(gz)
	defer func() {
		if e := tw.Close(); err == nil {
			err = e
		}
	}()
	for _, m := range members {
		h := &tar.Header{Name: m.name, Mode: int64(m.mode.Perm()), Uid: 0, Gid: 0, ModTime: time.Unix(946684800, 0).UTC(), Format: tar.FormatPAX}
		switch {
		case m.mode.IsDir():
			h.Typeflag = tar.TypeDir
			h.Name += "/"
		case m.mode&os.ModeSymlink != 0:
			h.Typeflag = tar.TypeSymlink
			h.Linkname = m.link
		default:
			h.Typeflag = tar.TypeReg
		}
		if h.Typeflag == tar.TypeReg {
			if m.source != "" {
				info, e := os.Stat(m.source)
				if e != nil {
					return e
				}
				h.Size = info.Size()
			} else {
				h.Size = int64(len(m.data))
			}
		}
		if err = tw.WriteHeader(h); err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		if m.source == "" {
			_, err = tw.Write(m.data)
		} else {
			var in *os.File
			in, err = os.Open(m.source)
			if err == nil {
				_, err = io.Copy(tw, in)
				closeErr := in.Close()
				if err == nil {
					err = closeErr
				}
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}
