package installer

import (
	"bytes"
	"fmt"
	"path"
	"time"

	"github.com/leaanthony/dmg/hfsplus"
	"github.com/leaanthony/dmg/udif"
)

// Dmg builds a mountable UDZO disk image containing files at the volume root.
// The image is HFS+ and can be created on any operating system.
func Dmg(volume string, files []File, when time.Time) ([]byte, error) {
	if volume == "" {
		return nil, fmt.Errorf("volume name is required")
	}
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
		children = append(children, &hfsplus.Entry{
			Name: name,
			Kind: hfsplus.KindFile,
			Data: f.Data,
			Mode: mode,
		})
	}
	img, err := hfsplus.Build(&hfsplus.Volume{
		Name:      volume,
		CreatedAt: when,
		Children:  children,
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
