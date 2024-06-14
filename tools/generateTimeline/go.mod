module github.com/vhive-serverless/sampler/tools/generateTimeline

go 1.21

require (
	github.com/gocarina/gocsv v0.0.0-20240520201108-78e41c74b4b1
	github.com/sirupsen/logrus v1.9.3
	github.com/vhive-serverless/loader v0.0.0-00010101000000-000000000000
)

require (
	golang.org/x/exp v0.0.0-20231110203233-9a3e6036ecaa // indirect
	golang.org/x/sys v0.22.0 // indirect
	gonum.org/v1/gonum v0.15.0 // indirect
)

replace github.com/vhive-serverless/loader => ../../
