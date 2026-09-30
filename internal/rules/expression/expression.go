package expression

import (
	"fmt"
	"sync"

	"cel.dev/cel-go/cel"
)

// Evaluator 编译并缓存 CEL 表达式，用于 Action requirements、Story 条件等。
//
// 可用变量：actor、target、scene、world（均为 map<string, dyn>）。
// 表达式只做判定与简单数值计算，不演化为通用脚本。
type Evaluator struct {
	env   *cel.Env
	mu    sync.RWMutex
	cache map[string]cel.Program
}

// Vars 是表达式求值时注入的变量。
type Vars map[string]any

// New 创建求值器。
func New() (*Evaluator, error) {
	env, err := cel.NewEnv(
		cel.Variable("actor", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("target", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("scene", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("world", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return nil, fmt.Errorf("create cel env: %w", err)
	}
	return &Evaluator{env: env, cache: map[string]cel.Program{}}, nil
}

// Compile 编译表达式并缓存，可在加载 Package 时提前校验语法。
func (e *Evaluator) Compile(expr string) (cel.Program, error) {
	e.mu.RLock()
	prg, ok := e.cache[expr]
	e.mu.RUnlock()
	if ok {
		return prg, nil
	}
	ast, iss := e.env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("compile %q: %w", expr, iss.Err())
	}
	prg, err := e.env.Program(ast, cel.CostLimit(10_000))
	if err != nil {
		return nil, fmt.Errorf("program %q: %w", expr, err)
	}
	e.mu.Lock()
	e.cache[expr] = prg
	e.mu.Unlock()
	return prg, nil
}

// Eval 求值任意表达式。未提供的标准变量以空 map 注入。
func (e *Evaluator) Eval(expr string, vars Vars) (any, error) {
	prg, err := e.Compile(expr)
	if err != nil {
		return nil, err
	}
	in := map[string]any{
		"actor":  map[string]any{},
		"target": map[string]any{},
		"scene":  map[string]any{},
		"world":  map[string]any{},
	}
	for k, v := range vars {
		in[k] = v
	}
	out, _, err := prg.Eval(in)
	if err != nil {
		return nil, fmt.Errorf("eval %q: %w", expr, err)
	}
	return out.Value(), nil
}

// EvalBool 求值布尔表达式。
func (e *Evaluator) EvalBool(expr string, vars Vars) (bool, error) {
	v, err := e.Eval(expr, vars)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("expression %q returned %T, want bool", expr, v)
	}
	return b, nil
}
