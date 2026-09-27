package selfishell

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

type releaseMember struct {
	kind   byte
	target string
}

func scanReleaseArchive(filename string) (map[string]releaseMember, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	members := map[string]releaseMember{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimPrefix(h.Name, "./")
		if name == "." || name == "" {
			if h.Typeflag == tar.TypeDir {
				continue
			}
			return nil, fmt.Errorf("unsafe release archive root")
		}
		if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
			return nil, fmt.Errorf("unsafe release archive path: %s", name)
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." {
				return nil, fmt.Errorf("unsafe release archive path: %s", name)
			}
		}
		clean := path.Clean(name)
		if clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
			return nil, fmt.Errorf("unsafe release archive path: %s", name)
		}
		if _, exists := members[clean]; exists {
			return nil, fmt.Errorf("duplicate release archive member: %s", clean)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA && h.Typeflag != tar.TypeDir && h.Typeflag != tar.TypeSymlink {
			return nil, fmt.Errorf("unsupported release archive member: %s", clean)
		}
		if h.Typeflag == tar.TypeSymlink && (path.IsAbs(h.Linkname) || strings.Contains(h.Linkname, "\\") || h.Linkname == "") {
			return nil, fmt.Errorf("unsafe release archive link: %s", clean)
		}
		if h.Typeflag == tar.TypeSymlink {
			// Reject parent traversal, including
			// links whose lexical path looks safe before another link resolves.
			for _, part := range strings.Split(h.Linkname, "/") {
				if part == ".." {
					return nil, fmt.Errorf("unsafe release archive link: %s", clean)
				}
			}
		}
		members[clean] = releaseMember{h.Typeflag, h.Linkname}
	}
	dirs := map[string]bool{".": true}
	for name, m := range members {
		if m.kind == tar.TypeDir {
			dirs[name] = true
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			dirs[parent] = true
		}
	}
	for name, m := range members {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if item, ok := members[parent]; ok && item.kind != tar.TypeDir {
				return nil, fmt.Errorf("release archive path collision: %s", name)
			}
		}
		if m.kind != tar.TypeSymlink {
			continue
		}
		if err := validateReleaseLink(name, path.Join(path.Dir(name), m.target), members, dirs); err != nil {
			return nil, err
		}
	}
	return members, nil
}

func validateReleaseLink(name, target string, members map[string]releaseMember, dirs map[string]bool) error {
	_, _, err := resolveReleasePath(name, target, members, dirs, map[string]bool{})
	return err
}

// active tracks only links whose definitions are currently being resolved.
// A directory alias may be traversed again after its definition has resolved.
func resolveReleasePath(name, target string, members map[string]releaseMember, dirs map[string]bool, active map[string]bool) (string, bool, error) {
	if target == ".." || strings.HasPrefix(target, "../") || path.IsAbs(target) {
		return "", false, fmt.Errorf("escaping release archive link: %s", name)
	}
	if target == "." {
		return ".", true, nil
	}
	resolved := "."
	parts := strings.Split(target, "/")
	for i, part := range parts {
		candidate := path.Join(resolved, part)
		member, exists := members[candidate]
		isDir := dirs[candidate]
		if exists && member.kind == tar.TypeSymlink {
			if active[candidate] {
				return "", false, fmt.Errorf("cyclic release archive link: %s", name)
			}
			active[candidate] = true
			var err error
			resolved, isDir, err = resolveReleasePath(name, path.Join(path.Dir(candidate), member.target), members, dirs, active)
			delete(active, candidate)
			if err != nil {
				return "", false, err
			}
		} else if exists || isDir {
			resolved = candidate
		} else {
			return "", false, fmt.Errorf("dangling release archive link: %s", name)
		}
		if i < len(parts)-1 && !isDir {
			return "", false, fmt.Errorf("dangling release archive link: %s", name)
		}
		if i == len(parts)-1 {
			return resolved, isDir, nil
		}
	}
	return resolved, true, nil
}

// extractReleaseArchive completes archive validation before creating staging.
func extractReleaseArchive(filename, staging string) error {
	if _, err := scanReleaseArchive(filename); err != nil {
		return err
	}
	if err := os.Mkdir(staging, 0700); err != nil {
		return err
	}
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(h.Name, "./")
		if name == "." || name == "" {
			continue
		}
		target := staging + "/" + path.Clean(name)
		if err := os.MkdirAll(path.Dir(target), 0700); err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, tr)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if err := os.Chmod(target, os.FileMode(h.Mode)&0777); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.Symlink(h.Linkname, target); err != nil {
				return err
			}
		}
	}
}
