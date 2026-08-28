package collector

func boolFloat64(value bool) float64 {
	if value {
		return 1
	}
	return 0
}
