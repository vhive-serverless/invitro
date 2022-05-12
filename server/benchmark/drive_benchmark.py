import os

if __name__ == "__main__":
    print('Benchmarking function timing ...\n')

    cmd = "cd server/benchmark/ && go test -bench=. -benchtime 1ms -cpu=1 -count=20 | tail -n +5 | head -n -2 | awk '{print $2}'"
    results = list(map(int, os.popen(cmd).read().strip().split()))
    avg_iter_per_1ms = int(sum(results) / len(results))

    print(f"{avg_iter_per_1ms=}")