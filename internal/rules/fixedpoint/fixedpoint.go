package fixedpoint

import "fmt"

// Scale 定点缩放因子：1000 = 1.000。
const Scale = 1000

// Value 是三位小数精度的定点数，底层为 int64。
type Value int64

// FromInt 由整数构造定点数。
func FromInt(i int64) Value { return Value(i * Scale) }

// FromMilli 由千分位整数构造定点数，例如 FromMilli(1500) = 1.500。
func FromMilli(m int64) Value { return Value(m) }

// Add 加法。
func (v Value) Add(o Value) Value { return v + o }

// Sub 减法。
func (v Value) Sub(o Value) Value { return v - o }

// Mul 乘法，结果向零截断。
func (v Value) Mul(o Value) Value { return Value(int64(v) * int64(o) / Scale) }

// Div 除法，结果向零截断。o 不能为 0。
func (v Value) Div(o Value) Value { return Value(int64(v) * Scale / int64(o)) }

// Int 返回向零截断的整数部分。
func (v Value) Int() int64 { return int64(v) / Scale }

// String 以 "1.500" 形式输出。
func (v Value) String() string {
	sign := ""
	n := int64(v)
	if n < 0 {
		sign = "-"
		n = -n
	}
	return fmt.Sprintf("%s%d.%03d", sign, n/Scale, n%Scale)
}
