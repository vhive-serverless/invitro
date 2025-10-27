# Changelog

## [Unreleased]

### Added

### Changed

### Fixed

## Release v1.1

### Added

- **Multi-Loader Feature (by Lenson)**: A comprehensive multi-loader was introduced to run concurrent load tests. This includes documentation, a fail-fast flag, a dry run mode, and the ability to use sweep fields for experiments.
- **Cloud and Platform Integrations**:
    - **Azure Functions (by Hancheng Wang)**: Integrated deployment and execution of Azure Functions.
    - **AWS Lambda (by Lee Xuan Hua)**: Integrated AWS Lambda using the serverless.com framework, including end-to-end tests and a shift from Zip to Image deployment via ECR.
- **RPS and Dirigent Enhancements (by Lazar Cvetković & Tobias Stocker)**:
    - Implemented a new Requests Per Second (RPS) mode with warmup capabilities and data size features.
    - Added RPS support for Dirigent Dandelion workflows and GPU options.
    - Introduced a new individual function driver and improved IAT generation.
- **Tooling and Scripts**:
    - **Kind and Prometheus Setup (by Lenson)**: Added scripts for setting up Kind nodes, Prometheus, and distributing SSH keys.
    - **vSwarm Support (by aryans1204)**: Added support for vSwarm, including a parser for mapper output, deployment of vSwarm functions, and associated documentation.
    - **Mapper Tool (by KarthikL1729 & L Lakshmanan)**: Introduced a tool to map traces to vSwarm proxies, along with documentation.
    - **Failure Injection (by Lazar Cvetković)**: Added a feature to trigger failures in cluster manager components.
- **Advanced Logging and Metrics (by Lenson)**: Improved log consolidation, added advanced log collection with tests, and exposed additional metrics.

### Changed

- **CI/CD and Dependencies (by dependabot & Lazar M. Cvetković)**:
    - Upgraded GitHub Actions workflows to use Ubuntu 24.04.
    - Performed numerous dependency updates for both production and development packages, including `golangci-lint`, `numpy`, and `pytest`.
- **Code and Configuration Refinements**:
    - **API and Client Improvements (by Lazar Cvetković)**: Introduced a new gRPC interface for deployment and invocation, and a new HTTP workload client.
    - **General Refactoring (by various authors)**: Refactored Kind setup scripts, simplified SSH key storage, and moved Dirigent-specific parameters into a separate configuration.
- **Documentation Updates (by various authors)**: Updated documentation for the multi-loader, vSwarm, and the mapper tool, and fixed broken links.

### Fixed

- **Setup and Configuration Scripts (by JooYoung Park & Leonid Kondrashov)**:
    - Patched the multi-node creation script for compatibility with `invitro`.
    - Fixed `kubeadm` config YAML and issues with exposing infrastructure metrics.
- **Bug Fixes and Stability**:
    - **Core Functionality (by Lazar Cvetković)**: Addressed bugs in YAML selectors, Dirigent metadata, and Inter-Arrival Time (IAT) generation.
    - **Testing (by Leonid Kondrashov & Kway Yi Shen)**: Fixed data races in tests, incorrect CLI types, and issues with end-to-end test setups.
- **Cloud Deployment Issues (by Lee Xuan Hua)**:
    - Resolved AWS Lambda timeout issues, IAM policy size limits, and CloudFormation resource constraints.
    - Fixed issues with parallel deployment of `serverless.yml`.
- **Linting and Formatting (by various authors)**: Corrected various spelling errors and linting issues across the codebase.