package main

import (
    "bufio"
    "fmt"
    "io"
    "os/exec"
    log "github.com/sirupsen/logrus"
    "testing"
    "os"
    "github.com/stretchr/testify/assert"
    "strconv"
)

func TestSynthesizer(t *testing.T) {
    os.Chdir("..")
    // cmd := exec.Command("python3 -m unittest", "trace_synthesizer/tests/generate_test.py")
    // cmd := exec.Command("python3", "trace_synthesizer/tests/generate_test.py")
    cmd := exec.Command("python3", "generate_test.py")
    stdout, err := cmd.StdoutPipe()
    if err != nil {
	log.Fatal(err)
    }
    stderr, err := cmd.StderrPipe()
    if err != nil {
	log.Fatal(err)
    }
    err = cmd.Start()
    if err != nil {
	log.Fatal(err)
    }
    // fmt.Println(out)
    // fmt.Println(string(out))
    a := copyOutputV2(stdout)
    b, _ := strconv.Atoi(a)
    assert.Equal(t, 0, b)

    go copyOutput(stdout)
	go copyOutput(stderr)
    cmd.Wait()
}

func copyOutput(r io.Reader) {
    scanner := bufio.NewScanner(r)
    for scanner.Scan() {
        fmt.Println(scanner.Text())
    }
}

func copyOutputV2(r io.Reader) string {
    scanner := bufio.NewScanner(r)
    for scanner.Scan() {
        fmt.Println(scanner.Text())
	return scanner.Text()
    }
    return " "
}
