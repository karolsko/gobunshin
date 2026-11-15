package installer

import (
	"bytes"
	"fmt"
	"path"
	"time"

	"github.com/leaanthony/dmg/hfsplus"
	"github.com/leaanthony/dmg/udif"
)

// kHasCustomIcon marks the volume as having a custom icon in .VolumeIcon.icns.
// kIsInvisible hides that file in the Finder window.
const (
	kHasCustomIcon = 0x0400
	kIsInvisible   = 0x4000
)

// Dmg builds a mountable UDZO disk image containing files at the volume root.
// The image is HFS+ and can be created on any operating system.
// volumeIcon is optional ICNS data shown when the image is mounted.
func Dmg(volume string, files []File, volumeIcon []byte, when time.Time) ([]byte, error) {
	if volume == "" {
		return nil, fmt.Errorf("volume name is required")
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("disk image has no files")
	}
	if len(volumeIcon) > 0 && (len(volumeIcon) < 8 || string(volumeIcon[:4]) != "icns") {
		return nil, fmt.Errorf("volume icon is not an icns file")
	}
	children := make([]*hfsplus.Entry, 0, len(files)+1)
	for _, f := range files {
		name := path.Base(f.Path)
		if name == "" || name == "." || name == "/" {
			return nil, fmt.Errorf("disk image file %q has no name", f.Path)
		}
		mode := uint16(0o755)
		if f.Mode != 0 {
			mode = uint16(f.Mode & 0o777)
		}
		children = append(children, &hfsplus.Entry{
			Name: name,
			Kind: hfsplus.KindFile,
			Data: f.Data,
			Mode: mode,
		})
	}
	var rootFlags uint16
	if len(volumeIcon) > 0 {
		children = append(children, &hfsplus.Entry{
			Name:        ".VolumeIcon.icns",
			Kind:        hfsplus.KindFile,
			Data:        volumeIcon,
			Mode:        0o644,
			FinderFlags: kIsInvisible,
		})
		rootFlags = kHasCustomIcon
	}
	img, err := hfsplus.Build(&hfsplus.Volume{
		Name:            volume,
		CreatedAt:       when,
		Children:        children,
		RootFinderFlags: rootFlags,
	})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := udif.Build(&buf, img, "whole disk (Apple_HFS : 0)"); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
