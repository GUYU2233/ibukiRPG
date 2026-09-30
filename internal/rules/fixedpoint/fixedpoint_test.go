package fixedpoint

import "testing"

func TestArithmetic(t *testing.T) {
	a := FromMilli(1500) // 1.5
	b := FromInt(2)
	cases := []struct {
		got  Value
		want string
	}{
		{a.Add(b), "3.500"},
		{a.Sub(b), "-0.500"},
		{a.Mul(b), "3.000"},
		{a.Div(b), "0.750"},
		{FromInt(1).Div(FromInt(3)), "0.333"},
	}
	for i, c := range cases {
		if c.got.String() != c.want {
			t.Errorf("case %d: got %s want %s", i, c.got, c.want)
		}
	}
	if FromMilli(2999).Int() != 2 {
		t.Error("Int should truncate")
	}
}
