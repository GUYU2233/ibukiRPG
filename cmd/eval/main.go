// Command eval 运行 LLM Eval 用例（tests/eval）。
//
//	go run ./cmd/eval                 # 离线解析 + 录音回放 + 叙事守卫
//	go run ./cmd/eval -synthesize     # 按用例 llm_output 重新生成合成录音
//	IBUKI_AI_KEY=... go run ./cmd/eval -record   # 用真实 DeepSeek 录制缺失录音
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"

	"github.com/GUYU2233/ibukiRPG/internal/eval"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/packages"
)

type authTransport struct{ key string }

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	r2.Header.Set("Authorization", "Bearer "+a.key)
	return http.DefaultTransport.RoundTrip(r2)
}

func main() {
	dir := flag.String("dir", "tests/eval", "用例目录")
	rec := flag.String("recordings", "tests/eval/recordings", "录音目录")
	synth := flag.Bool("synthesize", false, "根据 llm_output 生成合成录音")
	record := flag.Bool("record", false, "真实录制缺失的录音（需要 IBUKI_AI_KEY）")
	verbose := flag.Bool("v", false, "显示每个用例")
	flag.Parse()

	if err := run(*dir, *rec, *synth, *record, *verbose); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}

func run(dir, rec string, synth, record, verbose bool) error {
	ev, err := expression.New()
	if err != nil {
		return err
	}
	pkg, err := loader.Load(packages.Demo(), ev)
	if err != nil {
		return err
	}
	cases, err := eval.LoadCases(dir)
	if err != nil {
		return err
	}
	r := &eval.Runner{Pkg: pkg, RecordingsDir: rec}
	ctx := context.Background()
	if synth {
		n, err := r.Synthesize(ctx, cases)
		if err != nil {
			return err
		}
		fmt.Printf("已生成 %d 条合成录音 → %s\n", n, rec)
	}
	if record {
		key := os.Getenv("IBUKI_AI_KEY")
		if key == "" {
			return fmt.Errorf("-record 需要环境变量 IBUKI_AI_KEY")
		}
		r.Upstream = authTransport{key: key}
	}
	results := r.Run(ctx, cases)
	pass, total, by := eval.Summary(results)
	for _, res := range results {
		if verbose || !res.Pass {
			mark := "PASS"
			if !res.Pass {
				mark = "FAIL"
			}
			fmt.Printf("%s  %-8s %s %s\n", mark, res.Mode, res.Case, res.Detail)
		}
	}
	keys := make([]string, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("—— 汇总 ——")
	for _, k := range keys {
		fmt.Printf("  %-36s %d/%d\n", k, by[k][0], by[k][1])
	}
	fmt.Printf("总计 %d/%d 通过\n", pass, total)
	if pass != total {
		return fmt.Errorf("%d 个用例失败", total-pass)
	}
	return nil
}
