package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

const demoDir = "../../../packages/demo"

func TestLoadDemoManifest(t *testing.T) {
	m, err := Load(filepath.Join(demoDir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if m.Namespace != "demo" || m.Type != TypeStory {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if got := len(m.Content["characters"]); got < 4 {
		t.Fatalf("want >=4 characters (player + 3 NPC), got %d", got)
	}
	// 清单引用的所有文件必须存在。
	for kind, files := range m.Content {
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(demoDir, f)); err != nil {
				t.Errorf("%s: %v", kind, err)
			}
		}
	}
}

func TestValidation(t *testing.T) {
	bad := []string{
		"namespace: demo\nversion: 1\ntype: world\n",
		"id: x\nnamespace: Demo\nversion: 1\ntype: world\n",
		"id: x\nnamespace: demo\ntype: world\n",
		"id: x\nnamespace: demo\nversion: 1\ntype: weird\n",
	}
	for _, b := range bad {
		if _, err := Parse([]byte(b)); err == nil {
			t.Errorf("expected error for %q", b)
		}
	}
}

func TestValidID(t *testing.T) {
	for _, id := range []string{"core:item/iron_sword", "demo:character/lena", "magic.mod:effect/black_blood"} {
		if !ValidID(id) {
			t.Errorf("%s should be valid", id)
		}
	}
	for _, id := range []string{"lena", "demo/lena", "Demo:character/lena", "demo:/x"} {
		if ValidID(id) {
			t.Errorf("%s should be invalid", id)
		}
	}
	if NamespaceOf("magic.mod:effect/x") != "magic.mod" {
		t.Error("NamespaceOf")
	}
}

func TestVersionConstraints(t *testing.T) {
	cases := []struct {
		v, c string
		want bool
	}{
		{"0.1.2rc1", ">=0.1.0", true},
		{"0.1.2-rc1", ">=0.2.0", false},
		{"0.2.0", ">=0.1.0 <0.3.0", true},
		{"0.3.0", ">=0.1.0, <0.3.0", false},
		{"1.4.0", "^1.2", true},
		{"2.0.0", "^1.2", false},
		{"0.1.0", "", true},
		{"dev", ">=9.0.0", true},
	}
	for _, c := range cases {
		got, err := Satisfies(c.v, c.c)
		if err != nil || got != c.want {
			t.Errorf("Satisfies(%q,%q)=%v,%v want %v", c.v, c.c, got, err, c.want)
		}
	}
	if _, err := ParseConstraint(">=abc"); err == nil {
		t.Error("expected invalid constraint")
	}
	a, _ := ParseVersion("0.1.2rc1")
	b, _ := ParseVersion("0.1.2")
	if a.Compare(b) >= 0 {
		t.Error("prerelease should sort before release")
	}
}
