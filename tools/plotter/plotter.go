package main

import (
	"encoding/csv"
	"errors"
	"flag"
	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)


type Record struct {
	funcCount int
	slowdown float64
}

func main() {
	var (
		inputDir = flag.String("i", "raw_results", "Path to the directory with input CSV files")
		outputDir = flag.String("o", "figs", "Path to the directory for output figures")
		debugLevel = flag.String("d", "info", "Debug level: info, debug")
	)
	flag.Parse()
	log.SetOutput(os.Stdout)

	switch *debugLevel {
	case "info":
		log.SetLevel(log.InfoLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug mode is enabled")
	}

	records, totalFailedNum := parseFiles(*inputDir)
	log.Info("The number of failed or timed out invocation is: ", totalFailedNum)

	plotFig(*outputDir, records)
}

func plotFig(outputDir string, records []Record) {
	if _, err := os.Stat(outputDir); errors.Is(err, os.ErrNotExist) {
		log.Info("Creating the output directory")
		err := os.Mkdir(outputDir, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
	}

	p := plot.New()

	p.Title.Text = "Plotutil example"
	p.X.Label.Text = "Number of functions"
	p.Y.Label.Text = "Slowdown"
	p.Y.Min = 0
	p.Y.Max = 20

	err := plotutil.AddLinePoints(p,
		"Line", getXY(records),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Save the plot to a PNG file.
	if err := p.Save(4*vg.Inch, 4*vg.Inch, filepath.Join(outputDir, "points.png")); err != nil {
		log.Fatal(err)
	}

	for _, rec := range records {
		log.Debug("Plotting ", rec.funcCount, rec.slowdown)
	}
}

func parseFiles(inputDir string) ([]Record, int) {
	files, err := ioutil.ReadDir(inputDir)
	if err != nil {
		log.Fatal("Cannot open the input directory:", err)
	}

	filePattern, err := regexp.Compile(`^exec_sample-(\d+)_phase-2_dur-\d*.csv$`)
	if err != nil {
		log.Fatal("Error compiling: ", err)
	}

	var recs []Record
	var totalFailedNum int
	for _, file := range files {
		if matched := filePattern.MatchString(file.Name()); !matched  { continue }

		log.Debug("Open file ", file.Name())

		match := filePattern.FindStringSubmatch(file.Name())
		funcCount, err := strconv.Atoi(match[1])
		if err != nil {
			log.Fatal("Cannot convert to integer:", err)
		}
		log.Debug("Func count is ", funcCount)

		slowdown, failedInvocationNum := getSlowdown(filepath.Join(inputDir, file.Name()))
		recs = append(recs,
			Record{
				funcCount: funcCount,
				slowdown:  slowdown,
			})
		totalFailedNum += failedInvocationNum
	}

	return recs, totalFailedNum
}

func getXY(records []Record) plotter.XYs {
	sort.Slice(records, func(i, j int) bool {
		return records[i].funcCount < records[j].funcCount
	})

	pts := make(plotter.XYs, len(records))
	for i := range pts {
		pts[i].X = float64(records[i].funcCount)
		pts[i].Y = records[i].slowdown
	}
	return pts
}

type LatencyRecord struct {
	responseTime, requestedDuration int
	slowdown float64
	failed bool
}

func getSlowdown(filePath string) (float64, int) {
	// open file
	f, err := os.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}

	// remember to close the file at the end of the program
	defer f.Close()

	// read csv values using csv.Reader
	csvReader := csv.NewReader(f)
	data, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}

	var slowdownList []float64
	var failedInvocationNum int

	for i, line := range data {
		if i > 0 { // omit header line
			var rec LatencyRecord
			for j, field := range line {
				if j == 3 {
					rec.responseTime, err = strconv.Atoi(field)
					if err != nil {
						log.Fatal("Cannot convert to integer:", err)
					}
				} else if j == 4 {
					rec.requestedDuration, err = strconv.Atoi(field)
					if err != nil {
						log.Fatal("Cannot convert to integer:", err)
					}
				} else if (j == 7 || j == 8) && (field == "true") {
					// Skip the record if j=7 or 8 == true (timeout or failed, respectively)
					rec.failed = true
					failedInvocationNum++
				} else {
					continue
				}
			}
			if rec.failed {
				log.Warn("Skipping zero response time, file=", filePath)
				continue
			}
			rec.slowdown = float64(rec.responseTime) / float64(rec.requestedDuration)
			log.Debug("Parsed values: ", rec)
			slowdownList = append(slowdownList, rec.slowdown)
		}

	}

	hmean := stat.HarmonicMean(slowdownList, nil)
	log.Debug("Harmonic mean=", hmean)

	return hmean, failedInvocationNum

}
