package installer

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"compress/zlib"
)

// File is one payload installed by a macOS or Windows installer.
type File struct {
	// Path is the install path. macOS paths are absolute, such as /usr/local/bin/gobunshin.
	Path string
	Data []byte
	Mode uint32
}

type payloadFile struct {
	Path string
	Data []byte
}

// Pkg builds an unsigned macOS component package that installs files under /.
func Pkg(identifier, version string, files []File, when time.Time) ([]byte, error) {
	if identifier == "" {
		return nil, fmt.Errorf("package identifier is required")
	}
	if version == "" {
		return nil, fmt.Errorf("package version is required")
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("package has no files")
	}
	var payload []payloadFile
	var total int
	for _, f := range files {
		if !strings.HasPrefix(f.Path, "/") || strings.Contains(f.Path, "..") {
			return nil, fmt.Errorf("install path %q must be absolute", f.Path)
		}
		payload = append(payload, payloadFile{Path: strings.TrimPrefix(f.Path, "/"), Data: f.Data})
		total += len(f.Data)
	}
	cpio := gzipCpio(payload, when)
	bom := writeBOM(payload, when)
	kbytes := total / 1024
	if kbytes < 1 {
		kbytes = 1
	}
	info := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<pkg-info overwrite-permissions="true" relocatable="false" identifier="%s" postinstall-action="none" version="%s" format-version="2" install-location="/" auth="root">
    <payload numberOfFiles="%d" installKBytes="%d"/>
    <bundle-version/>
    <upgrade-bundle/>
    <update-bundle/>
    <atomic-update-bundle/>
    <strict-identifier/>
    <relocate/>
</pkg-info>
`, xmlEscape(identifier), xmlEscape(version), len(payload)+dirCount(payload), kbytes)
	return writeXar([]xarFile{
		{Name: "PackageInfo", Data: []byte(info)},
		{Name: "Bom", Data: bom},
		{Name: "Payload", Data: cpio},
	}, when)
}

func dirCount(files []payloadFile) int {
	dirs := map[string]bool{".": true}
	for _, f := range files {
		parts := strings.Split(strings.Trim(f.Path, "/"), "/")
		acc := "."
		for _, part := range parts[:len(parts)-1] {
			acc += "/" + part
			dirs[acc] = true
		}
	}
	return len(dirs)
}

func gzipCpio(files []payloadFile, when time.Time) []byte {
	raw := cpioODC(files, when)
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	zw.Header.ModTime = when.UTC()
	zw.Header.OS = 255
	_, _ = zw.Write(raw)
	_ = zw.Close()
	return buf.Bytes()
}

func cpioODC(files []payloadFile, when time.Time) []byte {
	var body bytes.Buffer
	mtime := when.Unix()
	write := func(path string, mode, size int64, data []byte) {
		name := append([]byte(path), 0)
		body.WriteString("070707")
		fmtOct(&body, 6, 0) // dev
		fmtOct(&body, 6, 0) // ino
		fmtOct(&body, 6, mode)
		fmtOct(&body, 6, 0) // uid
		fmtOct(&body, 6, 0) // gid
		fmtOct(&body, 6, 1) // nlink
		fmtOct(&body, 6, 0) // rdev
		fmtOct(&body, 11, mtime)
		fmtOct(&body, 6, int64(len(name)))
		fmtOct(&body, 11, size)
		body.Write(name)
		body.Write(data)
	}
	dirs := map[string]bool{}
	var order []string
	for _, f := range files {
		parts := strings.Split(strings.Trim(f.Path, "/"), "/")
		acc := ""
		for _, part := range parts[:len(parts)-1] {
			if acc == "" {
				acc = part
			} else {
				acc += "/" + part
			}
			if !dirs[acc] {
				dirs[acc] = true
				order = append(order, acc)
			}
		}
	}
	write(".", 0o40755, 0, nil)
	for _, dir := range order {
		write(dir, 0o40755, 0, nil)
	}
	for _, f := range files {
		write(strings.Trim(f.Path, "/"), 0o100755, int64(len(f.Data)), f.Data)
	}
	write("TRAILER!!!", 0, 0, nil)
	if pad := (512 - body.Len()%512) % 512; pad != 0 {
		body.Write(make([]byte, pad))
	}
	return body.Bytes()
}

func fmtOct(b *bytes.Buffer, width int, v int64) {
	s := fmt.Sprintf("%0*o", width, v)
	if len(s) > width {
		s = s[len(s)-width:]
	}
	b.WriteString(s)
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

type xarFile struct {
	Name string
	Data []byte
}

func writeXar(files []xarFile, when time.Time) ([]byte, error) {
	var heap bytes.Buffer
	type item struct {
		name              string
		length, size      int
		offset            int
		extracted, stored string
	}
	var items []item
	for _, f := range files {
		sum := sha1.Sum(f.Data)
		digest := hex.EncodeToString(sum[:])
		items = append(items, item{
			name: f.Name, length: len(f.Data), size: len(f.Data),
			offset: 20 + heap.Len(), extracted: digest, stored: digest,
		})
		heap.Write(f.Data)
	}
	var toc bytes.Buffer
	toc.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	toc.WriteString("<xar>\n <toc>\n")
	fmt.Fprintf(&toc, "  <creation-time>%s</creation-time>\n", when.UTC().Format("2006-01-02T15:04:05"))
	toc.WriteString("  <checksum style=\"sha1\">\n   <offset>0</offset>\n   <size>20</size>\n  </checksum>\n")
	for i, item := range items {
		fmt.Fprintf(&toc, "  <file id=\"%d\">\n", i+1)
		fmt.Fprintf(&toc, "   <name>%s</name>\n", xmlEscape(item.name))
		toc.WriteString("   <type>file</type>\n")
		toc.WriteString("   <data>\n")
		fmt.Fprintf(&toc, "    <length>%d</length>\n", item.length)
		fmt.Fprintf(&toc, "    <offset>%d</offset>\n", item.offset)
		fmt.Fprintf(&toc, "    <size>%d</size>\n", item.size)
		toc.WriteString("    <encoding style=\"application/octet-stream\"/>\n")
		fmt.Fprintf(&toc, "    <extracted-checksum style=\"sha1\">%s</extracted-checksum>\n", item.extracted)
		fmt.Fprintf(&toc, "    <archived-checksum style=\"sha1\">%s</archived-checksum>\n", item.stored)
		toc.WriteString("   </data>\n")
		toc.WriteString("  </file>\n")
	}
	toc.WriteString(" </toc>\n</xar>\n")

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	if _, err := zw.Write(toc.Bytes()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	hdr := make([]byte, 28)
	copy(hdr[0:4], "xar!")
	binary.BigEndian.PutUint16(hdr[4:6], 28)
	binary.BigEndian.PutUint16(hdr[6:8], 1)
	binary.BigEndian.PutUint64(hdr[8:16], uint64(compressed.Len()))
	binary.BigEndian.PutUint64(hdr[16:24], uint64(toc.Len()))
	binary.BigEndian.PutUint32(hdr[24:28], 1)
	out.Write(hdr)
	out.Write(compressed.Bytes())
	sum := sha1.Sum(compressed.Bytes())
	out.Write(sum[:])
	out.Write(heap.Bytes())
	return out.Bytes(), nil
}
