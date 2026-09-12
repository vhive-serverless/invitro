## Context
This document serves to explain how different trace types are organised and used within InVitro.

As InVitro supports more trace types, developers are required to keep track of multiple diverse trace schemas, making development tedious and error-prone. Furthermore, users of InVitro need to understand how we preprocess and convert a given trace, particularly when information is removed or added.

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
- Brief overall summary

Main Flowchart
- Indicates when an input trace has its trace type changed as it is pre-processed, sampled, or loaded.

Comparison Table
- Indicates how an input trace is transformed, to fit into loader.
- Describes how information is manipulated/added/removed.

### Individual Trace Formats Diagrams
Detailed format schema information for:
- Azure2019
- Cleaned Azure2019
- Azure2021
- Huawei2023
- IBM2026








