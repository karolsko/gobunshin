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
// volumeIcon is optional ICNS data. It is the mounted volume icon and the icon
// of each file in the image.
func Dmg(volume string, files []File, volumeIcon []byte, when time.Time) ([]byte, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("disk image has no files")
	}
	children := make([]*hfsplus.Entry, 0, len(files))
	for _, f := range files {
		name := path.Base(f.Path)
		if name == "" || name == "." || name == "/" {
			return nil, fmt.Errorf("disk image file %q has no name", f.Path)
		}
		mode := uint16(0o755)
		if f.Mode != 0 {
			mode = uint16(f.Mode & 0o777)
		}
		entry := &hfsplus.Entry{
			Name: name,
			Kind: hfsplus.KindFile,
			Data: f.Data,
			Mode: mode,
		}
		if len(volumeIcon) > 0 {
			entry.FinderFlags = kHasCustomIcon
		}
		children = append(children, entry)
	}
	img, err := finishDmg(volume, children, volumeIcon, when)
	if err != nil {
		return nil, err
	}
	if len(volumeIcon) > 0 {
		for _, f := range files {
			var err error
			img, err = setFileIcon(img, path.Base(f.Path), len(f.Data), volumeIcon)
			if err != nil {
				return nil, err
			}
		}
	}
	return wrapUDIF(img)
}

// DmgApp builds a disk image whose root is one application bundle.
// Finder launches that bundle itself, instead of opening Terminal for a raw executable.
func DmgApp(volume, appName, identifier, version, executable string, binary, icon []byte, when time.Time) ([]byte, error) {
	app, err := AppBundle(appName, identifier, version, executable, binary, icon)
	if err != nil {
		return nil, err
	}
	img, err := finishDmg(volume, []*hfsplus.Entry{app}, icon, when)
	if err != nil {
		return nil, err
	}
	return wrapUDIF(img)
}

func finishDmg(volume string, children []*hfsplus.Entry, volumeIcon []byte, when time.Time) ([]byte, error) {
	if volume == "" {
		return nil, fmt.Errorf("volume name is required")
	}
	if len(children) == 0 {
		return nil, fmt.Errorf("disk image has no files")
	}
	if len(volumeIcon) > 0 && (len(volumeIcon) < 8 || string(volumeIcon[:4]) != "icns") {
		return nil, fmt.Errorf("volume icon is not an icns file")
	}
	if len(volumeIcon) > 0 {
		children = append(children, &hfsplus.Entry{
			Name:        ".VolumeIcon.icns",
			Kind:        hfsplus.KindFile,
			Data:        volumeIcon,
			Mode:        0o644,
			FinderFlags: kIsInvisible,
		})
	}
	var rootFlags uint16
	if len(volumeIcon) > 0 {
		rootFlags = kHasCustomIcon
	}
	return hfsplus.Build(&hfsplus.Volume{
		Name:            volume,
		CreatedAt:       when,
		Children:        children,
		RootFinderFlags: rootFlags,
	})
}

func wrapUDIF(img []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := udif.Build(&buf, img, "whole disk (Apple_HFS : 0)"); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
