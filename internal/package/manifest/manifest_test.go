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
	if m.Namespace != "demo" || m.Type != TypeWorld {
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
