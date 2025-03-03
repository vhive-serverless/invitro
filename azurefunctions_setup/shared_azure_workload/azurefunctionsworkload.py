import time
import socket
import json
import azure.functions as func
import logging

from .exec_func import execute_function

# Global variable for hostname
hostname = socket.gethostname()

def main(req: func.HttpRequest) -> func.HttpResponse:
    logging.info("Processing request.")

    start_time = time.time()

    # Parse JSON request body
    try:
        req_body = req.get_json()
        logging.info(f"Request body: {req_body}")
    except ValueError:
        logging.error("Invalid JSON received.")
        return func.HttpResponse(
            json.dumps({"error": "Invalid JSON"}),
            status_code=400,
            mimetype="application/json"
        )

    runtime_milliseconds = req_body.get('RuntimeInMilliSec', 1000)
    memory_mebibytes = req_body.get('MemoryInMebiBytes', 128)

    logging.info(f"Runtime requested: {runtime_milliseconds} ms, Memory: {memory_mebibytes} MiB")

    # Directly call the execute_function
    duration = execute_function("",runtime_milliseconds,memory_mebibytes)
    result_msg = f"Workload completed in {duration} microseconds"

    # Prepare the response
    response = {
        "Status": "Success",
        "Function": req.url.split("/")[-1],
        "MachineName": hostname,
        "ExecutionTime": int((time.time() - start_time) * 1_000_000),  # Total time (includes HTTP, workload, and response prep)
        "DurationInMicroSec": duration,  # Time spent on the workload itself
        "MemoryUsageInKb": memory_mebibytes * 1024,
        "Message": result_msg
    }

    logging.info(f"Response: {response}")

    return func.HttpResponse(
        json.dumps(response),
        status_code=200,
        mimetype="application/json"
    )
