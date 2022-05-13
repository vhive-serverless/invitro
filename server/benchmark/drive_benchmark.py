import os

if __name__ == "__main__":
    print('Benchmarking function timing ...\n')

    cmd = "go test -bench=BenchmarkIterations -benchtime 1ms -cpu=1 -count=20 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    avg_iter_per_1ms = int(sum(results) / len(results))

    print(f"{avg_iter_per_1ms=}")

    cmd = "go test -bench=BenchmarkTiming -benchtime 1000ms -cpu=1 -count=20 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    avg_duration = int(sum(results) / len(results))

    print(f"Expected #iterations: 1000. Actaul #iterations: {avg_duration} (avg)")
    
    cmd = "go test -bench=BenchmarkTiming -benchtime 200ms -cpu=1 -count=20 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    avg_duration = int(sum(results) / len(results))

    print(f"Expected #iterations: 200. Actaul #iterations: {avg_duration} (avg)")
