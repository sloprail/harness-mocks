package replay

func (c *canon) measured(key string) bool {
	for _, m := range c.r.Measured {
		if m == key {
			return true
		}
	}
	return false
}

// Measure is what a measurement says without its value: whether it is zero.
func Measure(n float64) string {
	if n == 0 {
		return "<zero>"
	}
	return "<positive>"
}
