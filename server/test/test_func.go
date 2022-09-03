package main

// static double SQRTSD (double x) {
//     double r;
//     __asm__ ("sqrtsd %1, %0" : "=x" (r) : "x" (x));
//     return r;
// }
import "C"
import (
	"log"
	"sync"
)

func busySpin(wg *sync.WaitGroup) C.double {
	defer wg.Done()

	log.Println("Start spining ...")
	var tmp C.double
	for i := 0; i < 1e15; i++ {
		tmp = C.SQRTSD(C.double(10))
	}
	return tmp
}

func main() {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go busySpin(&wg)
	wg.Wait()
}
