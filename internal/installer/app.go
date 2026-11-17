package installer

import (
	"fmt"
	"html"
	"strings"

	"github.com/leaanthony/dmg/hfsplus"
)

// AppBundle is a macOS application bundle. Icon is optional ICNS data.
// Finder launches the bundle itself, so a double-click does not open Terminal.
func AppBundle(name, identifier, version, executable string, binary, icon []byte) (*hfsplus.Entry, error) {
	if !singleName(name) {
		return nil, fmt.Errorf("app name %q is not a single file name", name)
	}
	if !singleName(executable) {
		return nil, fmt.Errorf("executable %q is not a single file name", executable)
	}
	if identifier == "" || version == "" {
		return nil, fmt.Errorf("app bundle needs an identifier and a version")
	}
	if len(binary) == 0 {
		return nil, fmt.Errorf("app bundle has no executable")
	}
	var resources []*hfsplus.Entry
	iconFile := ""
	if len(icon) > 0 {
		if len(icon) < 8 || string(icon[:4]) != "icns" {
			return nil, fmt.Errorf("app icon is not an icns file")
		}
		iconFile = "icon"
		resources = append(resources, &hfsplus.Entry{
			Name: "icon.icns",
			Kind: hfsplus.KindFile,
			Data: icon,
			Mode: 0o644,
		})
	}
	return &hfsplus.Entry{
		Name: name + ".app",
		Kind: hfsplus.KindDir,
		Mode: 0o755,
		Children: []*hfsplus.Entry{{
			Name: "Contents",
			Kind: hfsplus.KindDir,
			Mode: 0o755,
			Children: []*hfsplus.Entry{
				{
					Name: "Info.plist",
					Kind: hfsplus.KindFile,
					Data: infoPlist(name, identifier, version, executable, iconFile),
					Mode: 0o644,
				},
				{
					Name: "MacOS",
					Kind: hfsplus.KindDir,
					Mode: 0o755,
					Children: []*hfsplus.Entry{{
						Name: executable,
						Kind: hfsplus.KindFile,
						Data: binary,
						Mode: 0o755,
					}},
				},
				{
					Name:     "Resources",
					Kind:     hfsplus.KindDir,
					Mode:     0o755,
					Children: resources,
				},
			},
		}},
	}, nil
}

func infoPlist(name, identifier, version, executable, icon string) []byte {
	b := &strings.Builder{}
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	writePlistString(b, "CFBundleName", name)
	writePlistString(b, "CFBundleDisplayName", name)
	writePlistString(b, "CFBundleIdentifier", identifier)
	writePlistString(b, "CFBundleVersion", version)
	writePlistString(b, "CFBundleShortVersionString", version)
	writePlistString(b, "CFBundleExecutable", executable)
	writePlistString(b, "CFBundlePackageType", "APPL")
	writePlistString(b, "CFBundleInfoDictionaryVersion", "6.0")
	writePlistString(b, "LSMinimumSystemVersion", "11.0")
	b.WriteString("\t<key>NSHighResolutionCapable</key>\n\t<true/>\n")
	b.WriteString("\t<key>NSPrincipalClass</key>\n\t<string>NSApplication</string>\n")
	if icon != "" {
		writePlistString(b, "CFBundleIconFile", icon)
	}
	b.WriteString("</dict>\n</plist>\n")
	return []byte(b.String())
}

func writePlistString(b *strings.Builder, key, value string) {
	fmt.Fprintf(b, "\t<key>%s</key>\n\t<string>%s</string>\n", html.EscapeString(key), html.EscapeString(value))
}

func singleName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}
