package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// 端到端试玩：新游戏 → 自然语言输入 → 快捷建议 → 退出 → 重新启动并读档继续。
func TestPlaythroughAndResume(t *testing.T) {
	dir := t.TempDir()
	o := options{dataDir: dir, provider: "offline", newGame: true, player: "阿澈", seed: 20260930}
	var out bytes.Buffer
	script := "/help\n环顾四周\n和老板打个招呼\n1\n来一杯麦酒\n我在酒馆里唱一首歌\n说服\n1\n/me\n/inv\n/npc\n/log\n/quit\n"
	if err := run(strings.NewReader(script), &out, o); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"新的旅程", "锈酒杯酒馆", "[检定]", "铜币 -2", "你想说服谁", "再见"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q", want)
		}
	}
	if testing.Verbose() {
		_, _ = os.Stdout.WriteString(s)
	}
	out.Reset()
	o.newGame = false
	if err := run(strings.NewReader("/load 1\n/scene\n/quit\n"), &out, o); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "继续：阿澈的旅程") {
		t.Fatalf("resume failed:\n%s", out.String())
	}
}
