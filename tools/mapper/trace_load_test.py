import mapper
import argparse

def main():
    parser = argparse.ArgumentParser(description="Trace load test")
    parser.add_argument(
        "-t",
        "--trace-directory",
        type=str,
        help="Path to the directory containing the trace files",
        required=True,
    )
    args = parser.parse_args()
    dir_path = args.trace_directory
    try:
        trace_functions, err = mapper.load_trace(dir_path)
        if err == -1:
            print("Trace loading failed")
            return
        elif err == 0:
            print("Trace loaded")
    except Exception as e:
        print(f"Error: {e}")
        return
    
if __name__ == "__main__":
    main()