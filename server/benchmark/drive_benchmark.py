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
    print(f"Expected #iterations: 1000.\t Actual #iterations: {avg_duration} (avg)")
    
    cmd = "go test -bench=BenchmarkTiming -benchtime 100ms -cpu=1 -count=50 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    avg_duration = int(sum(results) / len(results))
    print(f"Expected #iterations: 100.\t Actual #iterations: {avg_duration} (avg)")

    cmd = "go test -bench=BenchmarkTiming -benchtime 10ms -cpu=1 -count=100 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    avg_duration = int(sum(results) / len(results))
    print(f"Expected #iterations: 10.\t Actual #iterations: {avg_duration} (avg)\n")
