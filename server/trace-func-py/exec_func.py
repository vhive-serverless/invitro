import math
from time import process_time_ns
from numpy import empty, float32
from psutil import virtual_memory


def execute_function(input, runTime, totalMem):
    startTime = process_time_ns()

    chunkSize = 2**10 # size of a kb or 1024
    totalMem = totalMem*(2**10) # convert Mb to kb
    memory = virtual_memory()
    used = (memory.total - memory.available) // chunkSize # convert to kb
    additional = max(1, (totalMem - used))
    array = empty(additional*chunkSize, dtype=float32) # make an uninitialized array of that size, uninitialized to keep it fast
    # convert to ns
    runTime = (runTime - 1)*(10**6) # -1 because it should be slighly below that runtime
    memoryIndex = 0
    while process_time_ns() - startTime < runTime:
        for i in range(0, chunkSize):
            sin_i = math.sin(i)
            cos_i = math.cos(i)
            sqrt_i = math.sqrt(i)
            array[memoryIndex + i] = sin_i
        memoryIndex = (memoryIndex + chunkSize) % additional*chunkSize
    return (process_time_ns() - startTime) // 1000