// Package artifact describes files produced by a release.
package artifact

import "fmt"

// Type classifies a produced file.
type Type string

const (
	TypeBinary    Type = "binary"
	TypeArchive   Type = "archive"
	TypeChecksum  Type = "checksum"
	TypeSignature Type = "signature"
	TypeFile      Type = "file"
	TypePackage   Type = "package"
	TypeBrew      Type = "brew"
)

// Target is one GOOS/GOARCH pair, with GOARM set for 32-bit ARM.
type Target struct {
	Goos   string
	Goarch string
	Goarm  string
}

// String renders the target the way build logs show it.
func (t Target) String() string {
	if t.Goarch == "arm" && t.Goarm != "" {
		return fmt.Sprintf("%s/%s/v%s", t.Goos, t.Goarch, t.Goarm)
	}
	return t.Goos + "/" + t.Goarch
}

// Ext is the executable suffix for this target.
func (t Target) Ext() string {
	if t.Goos == "windows" {
		return ".exe"
	}
	return ""
}

// Artifact is one file in dist/.
type Artifact struct {
	Name      string
	Path      string
	Type      Type
	Goos      string
	Goarch    string
	Goarm     string
	Binary    string
	BuildID   string
	ArchiveID string
	Format    string
	Wrapped   bool
	InnerPath string
	SHA256    string
}
