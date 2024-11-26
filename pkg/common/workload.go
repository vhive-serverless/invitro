package common

// static double SQRTSD (double x) {
//     double r;
//     __asm__ ("sqrtsd %1, %0" : "=x" (r) : "x" (x));
//     return r;
// }
import "C"
import (
	"time"
)

const (
	// ContainerImageSizeMB was chosen as a median of the container physical memory usage.
	// Allocate this much less memory inside the actual function.
	ContainerImageSizeMB = 15

	ExecUnit int = 1e2
)

func takeSqrts() C.double {
	var tmp C.double // Circumvent compiler optimizations
	for i := 0; i < ExecUnit; i++ {
		tmp = C.SQRTSD(C.double(10))
	}
	return tmp
}

func busySpin(multiplier, runtimeMilli uint32) {
	totalIterations := int(multiplier * runtimeMilli)

	for i := 0; i < totalIterations; i++ {
		takeSqrts()
	}
}

func TraceFunctionExecution(start time.Time, IterationsMultiplier uint32, timeLeftMilliseconds uint32) (msg string) {
	timeConsumedMilliseconds := uint32(time.Since(start).Milliseconds())
	if timeConsumedMilliseconds < timeLeftMilliseconds {
		timeLeftMilliseconds -= timeConsumedMilliseconds
		if timeLeftMilliseconds > 0 {
			busySpin(uint32(IterationsMultiplier), timeLeftMilliseconds)
		}
	}

	return msg
}
