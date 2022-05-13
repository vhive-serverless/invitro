package timing

import (
	"testing"
)

const WARMUP_ITER int = 1e5
const AVG_ITER_PER_1MS int = 120

func BenchmarkIterations(b *testing.B) {
	for i := 0; i < WARMUP_ITER; i++ {
		TakeSqrts()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		TakeSqrts()
	}
}

func BenchmarkTiming(b *testing.B) {
	for i := 0; i < b.N; i++ {
		for i := 0; i < AVG_ITER_PER_1MS; i++ {
			TakeSqrts()
		}
	}
}
