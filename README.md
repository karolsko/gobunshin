<h1 align="center">gobunshin</h1>

<p align="center">
  <img src="docs/logo.png" alt="gobunshin" width="280">
</p>

<h3 align="center">
  <div>
    <a href="https://github.com/karolsko/gobunshin/issues">
        <img src="https://img.shields.io/github/issues/karolsko/gobunshin?color=fab387&labelColor=303446&style=for-the-badge">
    </a>
    <a href="https://github.com/karolsko/gobunshin/stargazers">
        <img src="https://img.shields.io/github/stars/karolsko/gobunshin?color=ca9ee6&labelColor=303446&style=for-the-badge">
    </a>
    <a href="https://github.com/karolsko/gobunshin/actions/workflows/ci.yml">
        <img src="https://img.shields.io/github/actions/workflow/status/karolsko/gobunshin/ci.yml?color=a6e3a1&labelColor=303446&style=for-the-badge&label=CI">
    </a>
    <a href="https://github.com/karolsko/gobunshin/blob/main/LICENSE">
        <img src="https://img.shields.io/static/v1.svg?style=for-the-badge&label=License&message=MIT&logoColor=ca9ee6&colorA=313244&colorB=cba6f7"/>
    </a>
    <br>
    </div>
</h3>

> Fully free release automation for Go projects

**gobunshin** builds, packages, and publishes Go releases. The name is 分身, a double. There is no paid tier, no license key, and no phone-home.

## 🧰 Features

- cross-compile Go binaries, or ship binaries you already built
- `tar.gz`, `zip`, and raw binary archives
- Linux packages: `.deb`, `.rpm`, and `.apk`
- macOS `.pkg` and `.dmg` installers, and Windows NSIS installers
- checksums, changelogs, and GitHub releases
- Homebrew formulas
- config `includes`, custom variables, and `if` filters
- monorepo tag prefixes and path-scoped changelogs
- snapshots, nightlies, and `--single-target`
- hooks, template files, and signing through your own command

Docker images, Scoop, Winget, Nix, npm, Arch and OpenWrt packages, and Windows MSI installers are not in this version. macOS packages and disk images are unsigned. Windows installers need `makensis` on the machine that builds the release.

## 📋 Requirements

- Go `1.26.4` or newer
- `git`
- `makensis` when you build Windows installers

## 🛠️ Installation

### 📦 Binary Releases

Pre-compiled binaries for Linux, Windows, and macOS are available on the [Releases](https://github.com/karolsko/gobunshin/releases) page.

### 🐹 Using Go

You can install `gobunshin` directly using `go install`:

```bash
go install github.com/karolsko/gobunshin/cmd/gobunshin@latest
```

### 🏗️ Build from Source

To build from source, you need to have [Go](https://go.dev/) installed.

```bash
git clone https://github.com/karolsko/gobunshin.git
cd gobunshin
go build -o gobunshin ./cmd/gobunshin
```

## 📖 Usage

Create a config, check it, and build a local snapshot. A snapshot is never published.

```bash
gobunshin init
gobunshin check
gobunshin release --snapshot --single-target
```

Tag a commit and run `gobunshin release` to publish. Set `GITHUB_TOKEN` or `GH_TOKEN`. The token needs permission to create releases, and to write contents on a Homebrew tap when you publish a formula.

### ⌨️ Commands

| Command     | Action                                                                                          |
| ----------- | ----------------------------------------------------------------------------------------------- |
| `init`      | write `.gobunshin.yaml`                                                                         |
| `check`     | validate the config and print the build matrix                                                 |
| `build`     | run before hooks and compile binaries into `dist/build`                                        |
| `release`   | build, package, and publish. Needs a semver tag unless you pass `--snapshot` or `--nightly`    |
| `publish`   | upload the artifacts recorded in `dist/artifacts.json`                                          |
| `changelog` | print release notes for the current tag, or for commits since the latest tag                   |
| `next`      | print the next tag from Conventional Commits                                                    |

Flags: `--snapshot`, `--nightly`, `--single-target`, `--id` (repeatable), `--skip-publish`, `--skip-hooks`, `--skip-clean`, `--timeout`, `--parallel`, `--verbose`, `-f`.

`release.mode: replace` deletes the existing GitHub release and moves the tag to the current commit. Nightlies always do that for `nightly.tag_name` (default `nightly`).

### 🚀 GitHub Actions

The repository contains an action. On a tag push it installs gobunshin, fetches the full history when the checkout is shallow, and runs the release. `contents: write` lets the workflow token upload the release.

```yaml
name: release
on:
  push:
    tags: ["v*"]
permissions:
  contents: write
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0
      - name: Install NSIS
        run: sudo apt-get update && sudo apt-get install -y nsis
      - uses: karolsko/gobunshin@v1
        with:
          args: release
```

Until the first `v1` tag exists, call the action from the checkout with `uses: ./`. This repository's [release workflow](.github/workflows/release.yml) does that.

`args` is split on spaces. `version` installs a published module version instead of the action checkout. `workdir` selects a module in a monorepo. `token` overrides the workflow token.

```yaml
- uses: karolsko/gobunshin@v1
  with:
    version: v1.2.3
    args: release --verbose
    workdir: services/api
```

## ⚙️ Configuration

`.gobunshin.yaml` lives in the project root. Unknown keys are an error. `version` must be `1`.

```yaml
version: 1
project_name: demo

metadata:
  description: Demo
  homepage: https://example.com
  license: MIT

variables:
  channel: stable

includes:
  - .gobunshin/base.yaml

monorepo:
  dir: services/demo
  tag_prefix: demo/

git:
  ignore_tags:
    - '^nightly$'

before:
  hooks:
    - go test ./...
after:
  hooks:
    - cmd: 'echo released {{ .Tag }}'
      dir: .

builds:
  - id: demo
    main: ./cmd/demo
    binary: demo
    env: [CGO_ENABLED=0]
    flags: [-trimpath]
    ldflags:
      - '-s -w -X main.version={{ .Version }}'
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ignore:
      - goos: darwin
        goarch: amd64
    if: '{{ ne .Os "windows" }}'

archives:
  - formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    files:
      - LICENSE
      - src: README.md
        dst: README.md

checksum:
  name_template: checksums.txt
  algorithm: sha256

nfpms:
  - package_name: demo
    maintainer: Dev <dev@example.com>
    formats: [deb, rpm, apk]
    bindir: /usr/bin
    depends: [libc6]
    contents:
      - src: LICENSE
        dst: /usr/share/licenses/demo/LICENSE
    scripts:
      postinstall: scripts/postinstall.sh

installers:
  - name: demo
    identifier: com.example.demo
    formats: [pkg, dmg, nsis]

signs:
  - cmd: gpg
    args: ["--batch", "--detach-sign", "--output", "${artifact}.sig", "${artifact}"]

template_files:
  - src: RELEASE.md.tmpl
    dst: RELEASE.md

changelog:
  sort: asc
  groups:
    - title: Features
      regexp: '^feat'
    - title: Fixes
      regexp: '^fix'
  filters:
    exclude: ['^docs:', '^chore:']
    include_paths: [services/demo]

snapshot:
  version_template: "{{ incpatch .Version }}-SNAPSHOT-{{ .ShortCommit }}"

nightly:
  version_template: "0.0.0-nightly.{{ .ShortCommit }}"
  tag_name: nightly
  name_template: "Nightly {{ .Date }}"

release:
  mode: fail
  prerelease: auto
  name_template: "{{ .Tag }}"
  header_file: .github/release-header.md
  footer: "Full notes: {{ .Tag }}"
  github:
    owner: acme
    name: demo

homebrews:
  - name: demo
    description: "{{ .ProjectName }}"
    license: MIT
    branch: main
    repository:
      owner: acme
      name: homebrew-tap
```

`nfpms` packs every Linux binary into `.deb`, `.rpm`, and `.apk`. The binary is installed to `bindir` (`/usr/bin` by default). `package_name` is a lowercase package name. `maintainer` is required. Description, homepage, and license fall back to `metadata`.

`installers` writes a `.pkg` and a `.dmg` for each Darwin binary, and an NSIS `.exe` for each Windows binary. The package installs into `bindir` (`/usr/local/bin` by default). The disk image is an HFS+ volume whose root contains the binary. The Windows installer copies `name.exe` into `Program Files\name` and registers an uninstaller.

Included files are merged first. Scalars in the including file win. `builds` and `archives` merge by `id` when every entry has an id. Other lists are replaced.

`if` is a template. The build, archive, package, or installer is kept when the result is anything other than empty, `false`, `no`, or `0`.

A prebuilt binary:

```yaml
builds:
  - id: demo
    builder: prebuilt
    binary: demo
    prebuilt:
      path: "out/{{ .Os }}_{{ .Arch }}/demo{{ .Ext }}"
```

The path must stay inside the project.

### 🧩 Templates

`{{ .Version }}`, `{{ .Tag }}`, `{{ .Commit }}`, `{{ .ShortCommit }}`, `{{ .Date }}`, `{{ .Timestamp }}`, `{{ .Now }}`, `{{ .ProjectName }}`, `{{ .Os }}`, `{{ .Arch }}`, `{{ .Arm }}`, `{{ .Ext }}`, `{{ .Binary }}`, `{{ .Major }}`, `{{ .Minor }}`, `{{ .Patch }}`, `{{ .Prerelease }}`, `{{ .IsSnapshot }}`, `{{ .IsNightly }}`, `{{ .Changelog }}`, `{{ .Env.NAME }}`, `{{ .Var.NAME }}`, `{{ .Artifacts }}`.

Functions: `incpatch`, `env`, `time`, `tolower`, `toupper`, `trim`, `replace`, `contains`, `hasPrefix`. A missing field is an error.

In `snapshot.version_template`, `.Version` is still the latest tag (or `0.0.0`). `incpatch` turns `1.2.3` into `1.2.4` and drops a prerelease. After that name is chosen, variables are rendered again for the build.

Signing arguments understand `${artifact}` and `${artifactName}` in addition to templates. The default signature path is `${artifact}.sig`.

### 🏷️ Tags

With `versioning.conventional_commits`, `gobunshin next` prints the next tag and `gobunshin release` creates it from commits since the previous tag.

- `feat` raises the minor version
- `fix`, `perf`, and `revert` raise the patch version
- `!` or a `BREAKING CHANGE:` footer raises the major version
- other types, including `docs`, `chore`, and `ci`, do not release

```yaml
versioning:
  conventional_commits: true
  tag_prefix: v
```

```bash
gobunshin next
gobunshin release
```

You can still tag a commit yourself. A semver tag on `HEAD` (`v1.2.3` or `1.2.3`) is used as-is. `--snapshot` and `--nightly` skip this. `monorepo.tag_prefix` or `git.tag_prefix` is stripped first, so `demo/v1.2.3` becomes version `1.2.3`.

`1`, `1.2`, and `1.2.3` are all accepted. Prereleases sort below the final release. For a stable tag, the previous release used for the changelog is the previous stable tag, so `v1.2.3` is compared with `v1.2.2` rather than `v1.2.3-rc.1`.

`git.ignore_tags` is a list of regular expressions matched against the full tag.

## 👨🏻‍💻 Development

1. **Clone the repository**

   ```sh
   git clone https://github.com/karolsko/gobunshin.git
   cd gobunshin
   ```

1. **Run the tests**

   ```sh
   go test ./...
   ```

1. **Build a binary**

   ```sh
   go build -o gobunshin ./cmd/gobunshin
   ```

## 🗒️ Credits

### 🎨 Inspiration

I was inspired by:

- [goreleaser/goreleaser](https://github.com/goreleaser/goreleaser)

## 📜 License

This project is licensed under the **MIT License**. See the [LICENSE](LICENSE) file for details.
