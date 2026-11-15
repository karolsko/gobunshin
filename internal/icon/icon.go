// Package icon turns a PNG, JPEG, or ICO into a Windows icon and an Apple ICNS.
package icon

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"strings"

	"github.com/leaanthony/dmg/icns"
	"github.com/tc-hib/winres"
)

// Windows reads a PNG, JPEG, or ICO into a Windows icon.
func Windows(path string, data []byte) (*winres.Icon, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ico":
		icon, err := winres.LoadICO(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("icon %s: %w", path, err)
		}
		return icon, nil
	case ".png", ".jpg", ".jpeg":
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("icon %s: %w", path, err)
		}
		icon, err := winres.NewIconFromResizedImage(img, nil)
		if err != nil {
			return nil, fmt.Errorf("icon %s: %w", path, err)
		}
		return icon, nil
	default:
		return nil, fmt.Errorf("icon %s must be png, jpg, or ico", path)
	}
}

// WriteICO writes icon as an .ico file.
func WriteICO(w io.Writer, icon *winres.Icon) error {
	return icon.SaveICO(w)
}

// WriteSyso writes a COFF object that the Go linker embeds into a Windows executable.
// The file must be named with a _windows_<arch>.syso suffix and live in the main package directory.
func WriteSyso(w io.Writer, goarch string, icon *winres.Icon) error {
	arch, err := windowsArch(goarch)
	if err != nil {
		return err
	}
	var set winres.ResourceSet
	if err := set.SetIcon(winres.ID(1), icon); err != nil {
		return err
	}
	return set.WriteObject(w, arch)
}

func windowsArch(goarch string) (winres.Arch, error) {
	switch goarch {
	case "386":
		return winres.ArchI386, nil
	case "amd64":
		return winres.ArchAMD64, nil
	case "arm":
		return winres.ArchARM, nil
	case "arm64":
		return winres.ArchARM64, nil
	default:
		return "", fmt.Errorf("windows icon is not supported for %s", goarch)
	}
}

// ICNS reads a PNG, JPEG, or ICNS and returns ICNS bytes for a disk image.
func ICNS(path string, data []byte) ([]byte, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".icns":
		if len(data) < 8 || string(data[:4]) != "icns" {
			return nil, fmt.Errorf("icon %s is not an icns file", path)
		}
		return data, nil
	case ".png", ".jpg", ".jpeg":
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("icon %s: %w", path, err)
		}
		var buf bytes.Buffer
		if err := icns.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("icon %s: %w", path, err)
		}
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("icon %s must be png, jpg, or icns", path)
	}
}
