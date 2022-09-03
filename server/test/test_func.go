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

	rpc "github.com/eth-easl/loader/server"
)

type funcServer struct {
	rpc.UnimplementedExecutorServer
}

func busySpin(wg *sync.WaitGroup) C.double {
	defer wg.Done()
	wg.Add(1)

	log.Println("Start spining ...")
	var tmp C.double
	for i := 0; i < 1e15; i++ {
		tmp = C.SQRTSD(C.double(10))
	}
	return tmp
}

func main() {
	wg := sync.WaitGroup{}
	go busySpin(&wg)
	wg.Wait()
}
