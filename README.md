<h1 align="center">gobunshin</h1>

<p align="center">
  <img src="docs/logo.png" alt="gobunshin" width="280">
</p>

Fully free release automation for Go projects. [GoReleaser](https://goreleaser.com) is excellent, and its Pro edition locks monorepos, nightly builds, config includes, macOS and Windows installers, and a long list of other features behind a paid license. gobunshin is an MIT-licensed tool with no paid tier, no license key, and no phone-home.

The name is 分身: a double.

## What works today

- Cross-compile Go binaries, or ship binaries you already built (`builder: prebuilt`)
- `tar.gz`, `zip`, and raw binary archives, with extra files and a Windows format override
- Linux system packages: `.deb`, `.rpm`, and `.apk`, from the same binaries
- macOS `.pkg` installers and Windows NSIS installers
- sha256 and sha512 checksums. Archive metadata uses the commit time, so the same commit packs to the same bytes
- Changelog from git, with groups, include and exclude filters, and path filters
- GitHub releases, including header and footer files
- Homebrew formulas, written locally and optionally committed to a tap
- Template files, custom variables, and `{{ .Artifacts }}`
- Before and after hooks
- Signing by running your own command (`gpg`, `cosign`, `cp`, anything on `PATH`)
- Config `includes`, merged by id
- Monorepo tag prefixes and changelogs limited to one directory
- `if` filters on builds, archives, and signing
- Smart semver tag sorting and ignored tags
- Snapshots, nightlies, and `--single-target`
- `release --skip-publish`, then `publish`, when you want to build and upload in separate steps

## Not in this version

Docker and Podman images, Scoop, Winget, Nix, npm, Arch and OpenWrt packages, macOS disk images, and Windows MSI installers are not implemented yet. macOS packages are unsigned. Windows installers are NSIS executables and need `makensis` on the release machine. Signing and notarizing are available only by shelling out through `signs` and hooks.

## Install

```sh
go install github.com/karolsko/gobunshin/cmd/gobunshin@latest
```

Or from a checkout:

```sh
go run ./cmd/gobunshin version
```

## Quick start

```sh
gobunshin init
gobunshin check
gobunshin release --snapshot --single-target
```

A snapshot never uploads anything. Tag a commit and run `gobunshin release` to publish. Set `GITHUB_TOKEN` or `GH_TOKEN`. The token needs permission to create releases, and to write contents on a Homebrew tap when you publish a formula.

## GitHub Actions

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
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: karolsko/gobunshin@v1
        with:
          args: release
```

Until the first `v1` tag exists, call the action from the checkout with `uses: ./`. This repository's [release workflow](.github/workflows/release.yml) does that.

`args` is split on spaces. `version` installs a published module version instead of the action checkout, for example `v1.2.3` or `latest`. `workdir` selects a module in a monorepo. `token` overrides the workflow token when the release repository needs a different credential.

```yaml
- uses: karolsko/gobunshin@v1
  with:
    version: v1.2.3
    args: release --verbose
    workdir: services/api
```

`check` does not publish anything:

```yaml
- uses: ./
  with:
    args: check
```


## Commands

| Command | What it does |
| --- | --- |
| `init` | Write `.gobunshin.yaml` |
| `check` | Validate the config and print the build matrix |
| `build` | Run before hooks and compile binaries into `dist/build` |
| `release` | Build, archive, checksum, sign, and publish. Requires a semver tag unless you pass `--snapshot` or `--nightly` |
| `publish` | Upload the artifacts recorded in `dist/artifacts.json` |
| `changelog` | Print release notes for the current tag, or for commits since the latest tag |

Useful flags: `--snapshot`, `--nightly`, `--single-target`, `--id` (repeatable), `--skip-publish`, `--skip-hooks`, `--skip-clean`, `--timeout`, `--parallel`, `--verbose`, `-f`.

`release.mode: replace` deletes the existing GitHub release and moves the tag to the current commit. Nightlies always do that for `nightly.tag_name` (default `nightly`).

## Config

`.gobunshin.yaml` in the project root. Unknown keys are an error. `version` must be `1`.

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

`nfpms` packs every Linux binary into `.deb`, `.rpm`, and `.apk`. The binary is installed to `bindir` (`/usr/bin` by default). Darwin and Windows binaries are skipped, so `--single-target` on a Mac still succeeds and simply produces no system packages. `package_name` is a lowercase package name. `maintainer` is required. A description, homepage, and license fall back to `metadata`. Checksums cover the packages, and the default signer signs them together with the archives.

`installers` writes a macOS component package for each Darwin binary and an NSIS installer for each Windows binary. The `.pkg` installs into `bindir` (`/usr/local/bin` by default). The Windows installer copies `name.exe` into `Program Files\name` and registers an uninstaller. Building the `.exe` requires `makensis`.

Included files are merged first. Scalars in the including file win. `builds` and `archives` merge by `id` when every entry has an id. Other lists are replaced.

`if` is a template. The build or archive is kept when the result is anything other than empty, `false`, `no`, or `0`.

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

## Templates

`{{ .Version }}`, `{{ .Tag }}`, `{{ .Commit }}`, `{{ .ShortCommit }}`, `{{ .Date }}`, `{{ .Timestamp }}`, `{{ .Now }}`, `{{ .ProjectName }}`, `{{ .Os }}`, `{{ .Arch }}`, `{{ .Arm }}`, `{{ .Ext }}`, `{{ .Binary }}`, `{{ .Major }}`, `{{ .Minor }}`, `{{ .Patch }}`, `{{ .Prerelease }}`, `{{ .IsSnapshot }}`, `{{ .IsNightly }}`, `{{ .Changelog }}`, `{{ .Env.NAME }}`, `{{ .Var.NAME }}`, `{{ .Artifacts }}`.

Functions: `incpatch`, `env`, `time`, `tolower`, `toupper`, `trim`, `replace`, `contains`, `hasPrefix`. A missing field is an error.

In `snapshot.version_template`, `.Version` is still the latest tag (or `0.0.0`). `incpatch` turns `1.2.3` into `1.2.4` and drops a prerelease. After that name is chosen, variables are rendered again for the build.

Signing arguments understand `${artifact}` and `${artifactName}` in addition to templates. The default signature path is `${artifact}.sig`.

## Tag rules

The commit being released must be tagged with a semantic version (`v1.2.3` or `1.2.3`), unless you pass `--snapshot` or `--nightly`. `monorepo.tag_prefix` or `git.tag_prefix` is stripped first, so `demo/v1.2.3` becomes version `1.2.3`.

`1`, `1.2`, and `1.2.3` are all accepted. Prereleases sort below the final release. For a stable tag, the previous release used for the changelog is the previous stable tag, so `v1.2.3` is compared with `v1.2.2` rather than `v1.2.3-rc.1`.

`git.ignore_tags` is a list of regular expressions matched against the full tag.

## License

MIT. See [LICENSE](LICENSE).
