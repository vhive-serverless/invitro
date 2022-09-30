package main

import (
    "encoding/csv"
    "fmt"
    log "github.com/sirupsen/logrus"
    "github.com/stretchr/testify/assert"
    "os"
    "os/exec"
    "strconv"
    "testing"
)

func TestSynthesizer(t *testing.T) {
    err := os.Chdir("..")
    if err != nil {
        log.Fatalf("Couldn't change directory: %s", err)
    }
    cmd := exec.Command("python3", "generate_test.py")
    output, err := cmd.CombinedOutput()
    if err != nil {
        fmt.Println(fmt.Sprint(err) + ": " + string(output))
    }
    rows := readInvocations("test_output/2_inv.csv")
    sum := calculate(rows)
    assert.Equal(t, 16200, sum)
}

func readInvocations(name string) [][]string {
    f, err := os.Open(name)
    if err != nil {
        log.Fatal("Cannot open test output")
    }

    defer f.Close()
    r := csv.NewReader(f)
    rows, err := r.ReadAll()
    if err != nil {
        log.Fatal("Cannot read CSV data:", err.Error())
    }

    return rows
}

func calculate(rows [][]string) int {
    sum := 0
    for i := range rows {
        if i == 0 {
            continue
        }
        for j := range rows[i] {
            if j == 0 || j == 1 || j == 2 {
                continue
            }
            v, err := strconv.Atoi(rows[i][j])
            if err != nil {
                log.Fatalf("Couldn't convert to integer: %s", err)
            }
            sum += v

        }
    }

    return sum
}
