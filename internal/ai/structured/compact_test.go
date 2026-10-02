package structured

import "testing"

// 本地模型的精简格式（edits / moves / rel）转成标准变更；不合法的条目被丢弃。
func TestCompactExpand(t *testing.T) {
	w, _, err := Parse(`{"v":1,"edits":[{"id":"x:location/a","field":"description","text":"墙塌了一角","why":"打斗"},{"id":"x:location/a","field":"a.b","text":"bad"}],
	 "moves":[{"id":"x:character/b","to":"x:location/c"}],"rel":[{"a":"x:character/b","b":"player","dim":"trust","delta":-3},{"a":"x","b":"y","dim":"trust","delta":0}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Changes) != 3 || w.Edits != nil || w.Empty() {
		t.Fatalf("%+v", w.Changes)
	}
	if c := w.Changes[0]; c.Path != "fields.description" || c.Reason != "打斗" {
		t.Fatalf("edit %+v", c)
	}
	if c := w.Changes[1]; c.Path != "location" || string(c.Value) != `"x:location/c"` {
		t.Fatalf("move %+v", c)
	}
	if c := w.Changes[2]; c.Target != "x:character/b>player" || c.Path != "dims.trust" || string(c.Value) != "-3" {
		t.Fatalf("rel %+v", c)
	}
}
