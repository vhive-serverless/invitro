#  MIT License
#
#  Copyright (c) 2023 EASL and the vHive community
#
#  Permission is hereby granted, free of charge, to any person obtaining a copy
#  of this software and associated documentation files (the "Software"), to deal
#  in the Software without restriction, including without limitation the rights
#  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#  copies of the Software, and to permit persons to whom the Software is
#  furnished to do so, subject to the following conditions:
#
#  The above copyright notice and this permission notice shall be included in all
#  copies or substantial portions of the Software.
#
#  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
#  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
#  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
#  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
#  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
#  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
#  SOFTWARE.

#  MIT License
#
#
#  Permission is hereby granted, free of charge, to any person obtaining a copy
#  of this software and associated documentation files (the "Software"), to deal
#  in the Software without restriction, including without limitation the rights
#  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#  copies of the Software, and to permit persons to whom the Software is
#  furnished to do so, subject to the following conditions:
#
#
import os

if __name__ == "__main__":
    print('Benchmarking function timing ...\n')

    cmd = "go test -bench=BenchmarkColdIterations -benchtime 1ms -cpu=1 -count=10 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    cold_iter_per_1ms = int(sum(results) / len(results))
    print(f"{cold_iter_per_1ms=}")

    cmd = "go test -bench=BenchmarkWarmIterations -benchtime 1ms -cpu=1 -count=20 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    warm_iter_per_1ms = int(sum(results) / len(results))
    print(f"{warm_iter_per_1ms=}\n")

    cmd = "go test -bench=BenchmarkTiming -benchtime 1000ms -cpu=1 -count=20 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    avg_duration = int(sum(results) / len(results))
    print(f"Expected #iterations: 1000.\t Actaul #iterations: {avg_duration} (avg)")
    
    cmd = "go test -bench=BenchmarkTiming -benchtime 100ms -cpu=1 -count=50 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    avg_duration = int(sum(results) / len(results))
    print(f"Expected #iterations: 100.\t Actaul #iterations: {avg_duration} (avg)")

    cmd = "go test -bench=BenchmarkTiming -benchtime 10ms -cpu=1 -count=100 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    avg_duration = int(sum(results) / len(results))
    print(f"Expected #iterations: 10.\t Actaul #iterations: {avg_duration} (avg)\n")
