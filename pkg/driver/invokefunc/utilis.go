func sum(numbers []float32) float32 {
	result := float32(0.0)
	for _, num := range numbers {
		result += num
	}
	return result
}