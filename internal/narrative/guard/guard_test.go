package guard

import "testing"

func TestGuard(t *testing.T) {
	f := Facts{PlayerLocation: "锈酒杯酒馆", Outcome: "failure", GoldDelta: -2, Gold: 13, PresentNPCs: []string{"伯林"}}
	c := Context{
		AllowedNames: []string{"伯林"}, AbsentNames: []string{"米拉"}, OtherLocations: []string{"小镇广场"},
		OtherItems: []string{"火把"}, SecretKeywords: []string{"私生女"},
	}
	ok := Check("你把两枚铜币拍在柜台上，伯林摇了摇头。", f, c)
	if !ok.OK {
		t.Fatalf("clean narrative flagged: %+v", ok)
	}
	bad := map[string]string{
		"secret_leak": "伯林悄悄告诉你，米拉其实是私生女。",
		"death":       "伯林倒在地上，断气了。",
		"absent_npc":  "米拉从门外走了进来。",
		"outcome":     "你终于说服了伯林。",
		"location":    "你来到小镇广场。",
		"item":        "你得到了火把。",
		"gold":        "你付了五枚铜币。",
		"empty":       "   ",
	}
	for rule, text := range bad {
		r := Check(text, f, c)
		found := false
		for _, v := range r.Violations {
			if v.Rule == rule {
				found = true
			}
		}
		if !found {
			t.Errorf("%s not detected in %q: %+v", rule, text, r)
		}
	}
	if n, ok := parseNum("十二"); !ok || n != 12 {
		t.Fatal(n)
	}
	if n, ok := parseNum("两"); !ok || n != 2 {
		t.Fatal(n)
	}
}
