package timing

// static double SQRTSD (double x) {
//     double r;
//     __asm__ ("sqrtsd %1, %0" : "=x" (r) : "x" (x));
//     return r;
// }
import "C"

const EXEC_UNIT int = 1e2

func TakeSqrts() {
	for i := 0; i < EXEC_UNIT; i++ {
		_ = C.SQRTSD(C.double(10))
	}
}
