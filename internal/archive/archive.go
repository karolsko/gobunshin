// Package archive writes tar.gz, zip, and raw binary release files.
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"
)

// Entry is one file inside an archive.
type Entry struct {
	Name   string
	Source string
	Mode   int64
}

// Create writes dst in the given format. format is tar.gz, tgz, zip, or binary.
// Member timestamps are normalized so the same inputs produce the same bytes.
func Create(dst, format string, entries []Entry, modTime time.Time) error {
	if len(entries) == 0 {
		return fmt.Errorf("archive has no files")
	}
	modTime = stableTime(modTime)
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	switch format {
	case "tar.gz", "tgz":
		return writeTarGZ(dst, sorted, modTime)
	case "zip":
		return writeZip(dst, sorted, modTime)
	case "binary":
		if len(sorted) != 1 {
			return fmt.Errorf("binary format accepts one file, got %d", len(sorted))
		}
		return copyFile(sorted[0].Source, dst, sorted[0].Mode)
	default:
		return fmt.Errorf("unknown archive format %q", format)
	}
}

func writeTarGZ(dst string, entries []Entry, mod time.Time) error {
	f, done, err := create(dst)
	if err != nil {
		return err
	}
	ok := false
	defer func() { _ = done(ok) }()
	gz, err := gzip.NewWriterLevel(f, gzip.DefaultCompression)
	if err != nil {
		return err
	}
	gz.Header = gzip.Header{ModTime: mod, OS: 255}
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		if err := writeTar(tw, e, mod); err != nil {
			_ = tw.Close()
			_ = gz.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func writeTar(tw *tar.Writer, e Entry, mod time.Time) error {
	info, err := os.Stat(e.Source)
	if err != nil {
		return err
	}
	name := slash(e.Name)
	hdr := &tar.Header{
		Name:     name,
		Mode:     modeOf(e, info),
		Size:     info.Size(),
		ModTime:  mod,
		Typeflag: tar.TypeReg,
		Format:   tar.FormatUSTAR,
	}
	if len(name) > 100 {
		hdr.Format = tar.FormatPAX
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	in, err := os.Open(e.Source)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(tw, in)
	return err
}

func writeZip(dst string, entries []Entry, mod time.Time) error {
	f, done, err := create(dst)
	if err != nil {
		return err
	}
	ok := false
	defer func() { _ = done(ok) }()
	zw := zip.NewWriter(f)
	for _, e := range entries {
		if err := writeZipEntry(zw, e, mod); err != nil {
			_ = zw.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func writeZipEntry(zw *zip.Writer, e Entry, mod time.Time) error {
	info, err := os.Stat(e.Source)
	if err != nil {
		return err
	}
	fh := &zip.FileHeader{
		Name:     slash(e.Name),
		Method:   zip.Deflate,
		Modified: mod,
	}
	fh.SetMode(fs.FileMode(modeOf(e, info)))
	w, err := zw.CreateHeader(fh)
	if err != nil {
		return err
	}
	in, err := os.Open(e.Source)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(w, in)
	return err
}

func copyFile(src, dst string, mode int64) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if mode == 0 {
		mode = 0o755
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fs.FileMode(mode))
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return err
	}
	return out.Close()
}

func create(dst string) (*os.File, func(bool) error, error) {
	f, err := os.Create(dst)
	if err != nil {
		return nil, nil, err
	}
	return f, func(ok bool) error {
		cerr := f.Close()
		if !ok {
			_ = os.Remove(dst)
		}
		return cerr
	}, nil
}

func modeOf(e Entry, info os.FileInfo) int64 {
	if e.Mode != 0 {
		return e.Mode
	}
	if info.Mode()&0o111 != 0 {
		return 0o755
	}
	return 0o644
}

func slash(name string) string {
	return strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "/")
}

func stableTime(t time.Time) time.Time {
	if t.IsZero() {
		t = time.Unix(0, 0)
	}
	t = t.UTC().Truncate(2 * time.Second)
	return t
}
