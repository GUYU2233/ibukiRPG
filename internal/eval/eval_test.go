package eval

import (
	"context"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/packages"
)

// TestEvalSuite 在 CI 中回放全部 Eval 用例（不访问真实 API）。
// Prompt 改动导致哈希变化时，运行 `make eval-synthesize` 重新生成合成录音。
func TestEvalSuite(t *testing.T) {
	ev, err := expression.New()
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := loader.Load(packages.Demo(), ev)
	if err != nil {
		t.Fatal(err)
	}
	cases, err := LoadCases("../../tests/eval")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 20 {
		t.Fatalf("expected ≥20 eval cases, got %d", len(cases))
	}
	r := &Runner{Pkg: pkg, RecordingsDir: "../../tests/eval/recordings"}
	results := r.Run(context.Background(), cases)
	for _, res := range results {
		if !res.Pass {
			t.Errorf("%s [%s]: %s", res.Case, res.Mode, res.Detail)
		}
	}
	pass, total, _ := Summary(results)
	t.Logf("eval: %d/%d passed", pass, total)
}
