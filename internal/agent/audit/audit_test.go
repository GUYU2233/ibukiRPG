package audit

import "testing"

func TestClassify(t *testing.T) {
	fs, err := Parse("好的：\n" + `{"findings":[
	 {"kind":"omission","text":"地点没更新","fix":[{"op":"patch","target":"x:location/a","path":"fields.description","value":"焦黑"}]},
	 {"kind":"omission","text":"背包少了扳手","fix":[{"op":"patch","target":"player","path":"inventory.x:item/w","value":1,"reason":"补记"}]},
	 {"kind":"contradiction","text":"已死的人又出现","fix":[{"op":"retire","target":"x:character/t","value":"death","reason":"r"}]},
	 {"kind":"hallucination","text":"说错了身份","fix":[]},
	 {"kind":"omission","text":"  "}
	]}`)
	if err != nil {
		t.Fatal(err)
	}
	p := Classify(fs, func(id string) string {
		if id == "x:character/t" {
			return "character"
		}
		return "location"
	})
	if len(p.Findings) != 4 || len(p.Autos) != 1 || len(p.Suggestions) != 2 || len(p.Corrections) != 1 {
		t.Fatalf("plan %+v", p)
	}
	if p.Autos[0].Reason == "" || p.Corrections[0] != "说错了身份" {
		t.Fatalf("defaults not filled: %+v", p)
	}
	if _, err := Parse("not json"); err == nil {
		t.Fatal("garbage should fail")
	}
}
