package registry

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func newReg(t *testing.T) *Registry {
	t.Helper()
	ev, err := expression.New()
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(packages.Builtin(), t.TempDir(), ev)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBuiltinPacksLoad(t *testing.T) {
	r := newReg(t)
	if r.DefaultID() != "demo" {
		t.Fatalf("default = %s", r.DefaultID())
	}
	list := r.List()
	if len(list) < 2 {
		t.Fatalf("want >=2 builtin packs, got %d", len(list))
	}
	for _, e := range list {
		if !e.Builtin || !e.Playable() {
			t.Fatalf("%s: builtin=%v err=%s", e.ID(), e.Builtin, e.Err)
		}
		if _, err := r.Load(e.ID()); err != nil {
			t.Fatalf("load %s: %v", e.ID(), err)
		}
	}
}

// zipDir 把 src（fs.FS）打成 zip，prefix 为 zip 内的顶层目录（可为空）；mutate 可改写文件内容。
func zipDir(t *testing.T, src fs.FS, prefix string, mutate func(name string, b []byte) []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pack.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	err = fs.WalkDir(src, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		if mutate != nil {
			b = mutate(name, b)
			if b == nil {
				return nil
			}
		}
		w, err := zw.Create(prefix + name)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	return p
}

// renamed 把灯塔包改成一个新的 id / namespace，模拟第三方故事包。
func renamed(name string, b []byte) []byte {
	s := string(b)
	if name == "manifest.yaml" {
		s = strings.Replace(s, "id: fog_lighthouse", "id: my_pack", 1)
		s = strings.Replace(s, "namespace: fog", "namespace: mine", 1)
		s = strings.Replace(s, `name: "雾港灯塔"`, `name: "我的故事"`, 1)
	}
	return []byte(strings.ReplaceAll(s, "fog:", "mine:"))
}

func lighthouse(t *testing.T) fs.FS {
	t.Helper()
	sub, err := fs.Sub(packages.Builtin()[1], ".")
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestImportAndDelete(t *testing.T) {
	r := newReg(t)
	res, err := r.Import(zipDir(t, lighthouse(t), "my_pack/", renamed))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Entry.Manifest.ID != "my_pack" || res.Replaced {
		t.Fatalf("result = %+v", res)
	}
	e, ok := r.Get("my_pack")
	if !ok || e.Builtin || !e.Playable() {
		t.Fatalf("entry = %+v ok=%v", e, ok)
	}
	if _, err := r.Load("my_pack"); err != nil {
		t.Fatal(err)
	}
	// 再导入一次 = 覆盖更新
	res, err = r.Import(zipDir(t, lighthouse(t), "", renamed))
	if err != nil || !res.Replaced {
		t.Fatalf("reimport: %+v %v", res, err)
	}
	if err := r.Delete("demo"); err == nil {
		t.Fatal("deleting builtin should fail")
	}
	if err := r.Delete("my_pack"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("my_pack"); ok {
		t.Fatal("still listed after delete")
	}
}

func TestImportRejects(t *testing.T) {
	r := newReg(t)
	cases := map[string]struct {
		path string
		want string
	}{
		"builtin id": {zipDir(t, lighthouse(t), "", nil), "内置"},
		"no manifest": {zipDir(t, lighthouse(t), "", func(n string, b []byte) []byte {
			if n == "manifest.yaml" {
				return nil
			}
			return renamed(n, b)
		}), "manifest.yaml"},
		"namespace taken": {zipDir(t, lighthouse(t), "", func(n string, b []byte) []byte {
			b = renamed(n, b)
			if n == "manifest.yaml" {
				b = []byte(strings.Replace(string(b), "namespace: mine", "namespace: demo", 1))
			}
			return b
		}), "命名空间"},
		"bad content": {zipDir(t, lighthouse(t), "", func(n string, b []byte) []byte {
			b = renamed(n, b)
			if n == "story/letter.yaml" {
				b = []byte(strings.Replace(string(b), "world.turn >= 1", "world.turn >= ", 1))
			}
			return b
		}), ""},
		"engine too new": {zipDir(t, lighthouse(t), "", func(n string, b []byte) []byte {
			b = renamed(n, b)
			if n == "manifest.yaml" {
				b = []byte(strings.Replace(string(b), `engine: ">=0.1.2"`, `engine: ">=9.0.0"`, 1))
			}
			return b
		}), "引擎"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := r.Import(c.path)
			if err == nil {
				t.Fatal("want error")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
	if len(r.List()) != 2 {
		t.Fatalf("rejected imports must not be installed: %d", len(r.List()))
	}
}

func TestImportPathTraversal(t *testing.T) {
	r := newReg(t)
	p := filepath.Join(t.TempDir(), "evil.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("manifest.yaml")
	_, _ = w.Write([]byte("id: evil\nnamespace: evil\nname: x\nversion: 0.1.0\ntype: story\n"))
	w, _ = zw.Create("../../escape.txt")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	_ = f.Close()
	if _, err := r.Import(p); err == nil {
		t.Fatal("path traversal accepted")
	}
}
