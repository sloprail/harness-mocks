package toolspec

// holds reports whether the decoded JSON value v is of type t.
func (t Type) holds(v any) bool {
	switch t {
	case String:
		_, ok := v.(string)
		return ok
	case Number:
		_, ok := v.(float64)
		return ok
	case Integer:
		n, ok := v.(float64)
		return ok && n == float64(int64(n))
	case Boolean:
		_, ok := v.(bool)
		return ok
	case Object:
		_, ok := v.(map[string]any)
		return ok
	case Array:
		_, ok := v.([]any)
		return ok
	}
	return false
}

func kindOf(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "a string"
	case float64:
		return "a number"
	case bool:
		return "a boolean"
	case map[string]any:
		return "an object"
	}
	return "an array"
}
