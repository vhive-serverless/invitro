package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"
)

var (
	inputFile  = flag.String("inputFile", "output.csv", "name of input file")
	outputFile = flag.String("outputFile", "processed.csv", "name of output file")
)

func main() {
	flag.Parse()
	f, err := os.Open(*inputFile)
	defer f.Close()

	var maxTime int

	csvReader := csv.NewReader(f)
	if _, err := csvReader.Read(); err != nil {
		log.Fatal(err)
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
		runtime, _ := strconv.Atoi(rec[2])
		if start+runtime > maxTime {
			maxTime = start + runtime
		}
	}
	log.Infof("Max time: %d", maxTime)

	funcCnt := make([]int, maxTime+1)
	memUsg := make([]int, maxTime+1)
	memReq := make([]int, maxTime+1)
	cpuUsg := make([]float64, maxTime+1)
	cpuReq := make([]float64, maxTime+1)

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		log.Fatal(err)
	}

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

	ff, err := os.OpenFile(*outputFile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}

	ff.WriteString("timestamp,funcCnt,memUsg,memReq,cpuUsg,cpuReq\n")
	for i := 0; i <= maxTime; i++ {
		ff.WriteString(fmt.Sprintf("%d,%d,%d,%d,%f,%f\n", i, funcCnt[i], memUsg[i], memReq[i], cpuUsg[i], cpuReq[i]))
	}
	log.Infof("Done writing to file")
	ff.Close()
}
