package timing

import (
	"testing"
)

const WARMUP_ITER int = 1e5

func BenchmarkTiming(b *testing.B) {
	for i := 0; i < WARMUP_ITER; i++ {
		TakeSqrts()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		TakeSqrts()
	}
}
