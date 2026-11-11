package tmpl

import "testing"

func TestRender(t *testing.T) {
	data := Data{
		Version:     "1.2.3",
		Tag:         "v1.2.3",
		ShortCommit: "abc1234",
		ProjectName: "demo",
		Os:          "linux",
		Arch:        "amd64",
		Env:         map[string]string{"FOO": "bar"},
		Var:         map[string]string{"channel": "stable"},
		Major:       1,
		Minor:       2,
		Patch:       3,
	}
	got, err := Render("{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}", data)
	if err != nil {
		t.Fatal(err)
	}
	if got != "demo_1.2.3_linux_amd64" {
		t.Fatalf("got %s", got)
	}
	got, err = Render("{{ incpatch .Version }}-SNAPSHOT-{{ .ShortCommit }}", data)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.2.4-SNAPSHOT-abc1234" {
		t.Fatalf("snapshot: %s", got)
	}
	got, err = Render("{{ .Var.channel }}-{{ .Env.FOO }}", data)
	if err != nil {
		t.Fatal(err)
	}
	if got != "stable-bar" {
		t.Fatalf("var: %s", got)
	}
	if _, err := Render("{{ .Missing }}", data); err == nil {
		t.Fatal("expected missing key error")
	}
}

func TestTruthy(t *testing.T) {
	if Truthy("false") || Truthy(" NO ") || Truthy("0") || Truthy("") {
		t.Fatal("expected false")
	}
	if !Truthy("true") || !Truthy("linux") {
		t.Fatal("expected true")
	}
}
