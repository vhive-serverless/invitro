package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"
)

func main() {
	f, err := os.Open("output.csv")
	// maxTime := 24 * 60 * 60 * 1000
	maxTime := 86454632 + 1
	funcCnt := make([]int, maxTime)
	memUsg := make([]int, maxTime)
	memReq := make([]int, maxTime)
	cpuUsg := make([]float64, maxTime)
	cpuReq := make([]float64, maxTime)

	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	csvReader := csv.NewReader(f)
	if header, err := csvReader.Read(); err != nil {
		log.Fatal(err)
	} else {
		log.Info(header)
	}

	for {
		rec, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		start, _ := strconv.Atoi(rec[0])
		// functionHash := rec[1]
		runtime, _ := strconv.Atoi(rec[2])
		memory, _ := strconv.Atoi(rec[3])
		maxMemory, _ := strconv.Atoi(rec[4])
		cpu, _ := strconv.ParseFloat(rec[5], 64)
		maxCpu, _ := strconv.ParseFloat(rec[6], 64)
		end := start + runtime

		for i := start; i <= end; i++ {
			funcCnt[i]++
			memUsg[i] += memory
			memReq[i] += maxMemory
			cpuUsg[i] += cpu
			cpuReq[i] += maxCpu
		}
	}
	log.Info("Done reading, writing to file")

	ff, err := os.OpenFile("processed.csv", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}

	ff.WriteString("timestamp,funcCnt,memUsg,memReq,cpuUsg,cpuReq\n")
	for i := 0; i < maxTime; i++ {
		ff.WriteString(fmt.Sprintf("%d,%d,%d,%d,%f,%f\n", i, funcCnt[i], memUsg[i], memReq[i], cpuUsg[i], cpuReq[i]))
	}
	log.Infof("Done writing to file")
	ff.Close()
}
