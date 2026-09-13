## Context
This document serves to explain how different trace types are organised and used within InVitro.

As InVitro supports more trace types, developers are required to keep track of multiple diverse trace schema, making development tedious and error-prone. Furthermore, users of InVitro need to understand how we preprocess and convert a given trace, particularly when information is removed or added.

While this document aims to be as complete as possible, the most up-to-date information should be referenced from the direct documentation files (`loader.md` and `sampler.md`) or read from source directly.

Current documentation includes:
- This markdown document
- Diagrams 
  - Made using [draw.io](https://www.drawio.com/). 

## Internal Structure 
Internally, loader accepts 2 main formats.
- Cleaned Azure2019 (per-function aggregated percentile statistics)
- Azure2021 (per-invocation data)

Sampler accepts Cleaned Azure2019 format. 
- Sampler support for Azure2021 is achieved by converting it to Cleaned Azure2019 format to be sampled, before being converted back.

Loader and Sampler support for other formats is achieved by transforming them into either of the 2 main formats.
- Huawei2023 -> Cleaned Azure2019
- IBM2026 -> Azure2021

## Diagrams
The diagrams can be viewed through the following methods:
- Online through this [public link](https://viewer.diagrams.net/?tags=%7B%7D&lightbox=1&highlight=0000ff&edit=_blank&layers=1&nav=1&title=Invitro%20Trace%20Format%20Table.drawio&dark=auto#Uhttps%3A%2F%2Fdrive.google.com%2Fuc%3Fid%3D1OfL72TZTf9I1kZA0CnPJlyEsK-hys4Ah%26export%3Ddownload). 
- Reopened in the [draw.io browser](https://app.diagrams.net/) by importing the workspace file at `./supported_traces/trace_format_diagrams.drawio`.
- Viewing the [pdf directly](./supported_traces/trace_format_diagrams.pdf).

### Summary Diagrams
Simplified Schema Table
- Brief summary of trace schema

Main Flowchart
- Indicates when an input trace has its trace type changed as it is pre-processed, sampled, or loaded.

Comparison Table
- Describes how information in an input trace is manipulated/added/removed, before being loaded.

### Individual Trace Formats Diagrams
Detailed format schema information for:
- Azure2019
- Cleaned Azure2019
- Azure2021
- Huawei2023
- IBM2026

## Trace Processing Methodology
This section describes how a trace is processed in InVitro, and highlights the reasoning behind certain choices.

Internally, Loader models each function as a sequence of invocations. Each invocation has an `inovcation timestamp`, `duration` and `memory use`.
The following sections will describe how these parameters are extracted from each supported trace type.
The diagram ["Comparison Table"](./supported_traces/trace_format_diagrams.pdf) is a useful reference and provides a summary.

### Azure2019
Trace only states the number of invocations within each time-unit (minute). A user-defined distribution is used (uniform, normal), and each `invocation timestamp` is taken as a sample from that distribution.

Duration data in the trace consists of percentile distribution statistics for each function. Each `duration` takes a random sample from this distribution.

Memory use data in the trace consists of percentile distribution statistics for each Application. Each application is split into its functions, dividing the described memory distribution equally across each function. Each `memory use` takes a random sample from this distribution.

### Huawei2023
Converted to `Cleaned Azure2019` trace format.

Trace states the number of invocations within each time-unit (minute, `Requests`). A user-defined distribution is used (uniform, normal), and each `invocation timestamp` is taken as a sample from that distribution. 

Duration data in the trace consists of readings for each function at each minute (`Function delay`). Percentile distribution statistics is generated from this array of values, and used in `Cleaned Azure2019` trace format. Each `duration` takes a random sample from this distribution.

Memory use data in the trace consists of readings for each function at each minute (`Memory limit`). Percentile distribution statistics is generated from this array of values, and used in `Cleaned Azure2019` trace format. Each `memory use` takes a random sample from this distribution.

The original trace does have duration and invocation data information at each second time-unit. The minute time-unit is actually an aggregated version of the second time-unit data. The minute time-unit data was used as it more similar to our Azure2019 trace format with minute-binned data.

### Azure2021
Preprocessing remove functions with invocation rate of 0ms duration above threshold rate. Default threshold of 50%. (Performed to replicate pre-processing done in Azure2019)

The trace is simple, and already has information in a per-invocation basis. `invocation timestamp` and `duration` are taken directly from the trace.

The trace has no `memory use` field. A surrogate value of 200MB is used for each invocation. We use this value as it was empirically found to be the average memory value in the Azure2019 trace.

### IBM2026
Converted to `Azure2021` trace format.

At a per-invocation basis, the `invocation timestamp` and `duration` are available in the original trace and are used directly in loader (the fields `InvocationTimes` and `AppExecTimes`). Only minor preprocessing is required to handle array unpacking (unpacking individual invocation data out from an array).

It should be noted that the `InvocationTimes` in IBM2026 appears to have a zero offset of 4 hours, 59 minutes, 59 seconds.
For example, `week_1.pickle` contains timestamps from 0 days, 4:59:59 to 7 days 4:59:59. This offset is deemed as a mistake, and is handled internally.

The trace has no `memory use` field. While it does have request memory through the field `AppContainerRequestMemory`, it does not have actual memory usage statistics. A surrogate value of 200MB is used for each invocation. We use this value as it was empirically found to be the average memory value in the Azure2019 trace.


