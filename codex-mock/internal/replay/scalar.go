package replay

import "math"

// scalar is the JSON value of a JS string, number or boolean the script wrote.
func scalar(v any) (any, bool) {
	switch x := v.(type) {
	case string, bool:
		return x, true
	case number:
		if math.IsInf(x.f, 0) || math.IsNaN(x.f) {
			return nil, false
		}
		if x.f == math.Trunc(x.f) && math.Abs(x.f) <= 1<<31 {
			return int(x.f), true
		}
		return x.f, true
	}
	return nil, false
}
