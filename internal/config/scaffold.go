package config

import (
	"fmt"
	"strings"
)

// Scaffold returns a starter config for the module in dir.
func Scaffold(dir string) string {
	module := readModule(dir)
	name := "app"
	if module != "" {
		parts := strings.Split(module, "/")
		name = parts[len(parts)-1]
	}
	owner, repo := githubFromModule(module)
	main := detectMain(dir, name)
	var b strings.Builder
	b.WriteString("# gobunshin release config. Every feature here is free.\n")
	b.WriteString("version: 1\n")
	fmt.Fprintf(&b, "project_name: %s\n", name)
	if module != "" {
		b.WriteString("metadata:\n")
		b.WriteString("  license: MIT\n")
		fmt.Fprintf(&b, "  description: %s\n", name)
	}
	b.WriteString("before:\n")
	b.WriteString("  hooks:\n")
	b.WriteString("    - go test ./...\n")
	b.WriteString("builds:\n")
	fmt.Fprintf(&b, "  - id: %s\n", name)
	if main != "." {
		fmt.Fprintf(&b, "    main: %s\n", main)
	} else {
		b.WriteString("    main: .\n")
	}
	fmt.Fprintf(&b, "    binary: %s\n", name)
	b.WriteString("    env:\n")
	b.WriteString("      - CGO_ENABLED=0\n")
	b.WriteString("    flags:\n")
	b.WriteString("      - -trimpath\n")
	b.WriteString("    # ldflags:\n")
	b.WriteString("    #   - -s -w -X main.version={{ .Version }} -X main.commit={{ .Commit }} -X main.date={{ .Date }}\n")
	b.WriteString("    goos: [linux, darwin, windows]\n")
	b.WriteString("    goarch: [amd64, arm64]\n")
	b.WriteString("archives:\n")
	b.WriteString("  - formats: [tar.gz]\n")
	b.WriteString("    format_overrides:\n")
	b.WriteString("      - goos: windows\n")
	b.WriteString("        formats: [zip]\n")
	b.WriteString("checksum:\n")
	b.WriteString("  name_template: checksums.txt\n")
	b.WriteString("nfpms:\n")
	b.WriteString("  - maintainer: Your Name <you@example.com>\n")
	b.WriteString("    formats: [deb, rpm, apk]\n")
	b.WriteString("installers:\n")
	b.WriteString("  - formats: [pkg, nsis]\n")
	b.WriteString("versioning:\n")
	b.WriteString("  conventional_commits: true\n")
	b.WriteString("  tag_prefix: v\n")
	b.WriteString("changelog:\n")
	b.WriteString("  sort: asc\n")
	b.WriteString("  groups:\n")
	b.WriteString("    - title: Features\n")
	b.WriteString("      regexp: '^feat'\n")
	b.WriteString("    - title: Fixes\n")
	b.WriteString("      regexp: '^fix'\n")
	b.WriteString("  filters:\n")
	b.WriteString("    exclude:\n")
	b.WriteString("      - '^docs:'\n")
	b.WriteString("      - '^chore:'\n")
	b.WriteString("release:\n")
	b.WriteString("  github:\n")
	if owner != "" {
		fmt.Fprintf(&b, "    owner: %s\n    name: %s\n", owner, repo)
	} else {
		b.WriteString("    owner: your-org\n    name: your-repo\n")
	}
	b.WriteString("# homebrews:\n")
	fmt.Fprintf(&b, "#   - name: %s\n", name)
	b.WriteString("#     repository:\n")
	b.WriteString("#       owner: your-org\n")
	b.WriteString("#       name: homebrew-tap\n")
	return b.String()
}
