/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

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
	rows := readInvocations("test_output/invocations.csv")
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
