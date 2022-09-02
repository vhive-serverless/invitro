package main

// static double SQRTSD (double x) {
//     double r;
//     __asm__ ("sqrtsd %1, %0" : "=x" (r) : "x" (x));
//     return r;
// }
import "C"
import "sync"

func busySpin(wg *sync.WaitGroup) C.double {
	defer wg.Done()
	wg.Add(1)

	var tmp C.double
	for i := 0; i < 1e15; i++ {
		tmp = C.SQRTSD(C.double(10))
	}
	return tmp
}

func main() {
	wg := sync.WaitGroup{}

	for i := 0; i < 16; i++ {
		go busySpin(&wg)
	}
	wg.Wait()
}
