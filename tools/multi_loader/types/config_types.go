package types

import (
	"errors"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	FORMAT_PLACEHOLDER = "{}"
)

type MultiLoaderConfiguration struct {
	Studies        []LoaderStudy `json:"Studies"`
	BaseConfigPath string        `json:"BaseConfigPath"`
	// Optional
	IatGeneration bool   `json:"IatGeneration"`
	Generated     bool   `json:"Generated"`
	PreScript     string `json:"PreScript"`
	PostScript    string `json:"PostScript"`
}

type LoaderStudy struct {
	Name   string                 `json:"Name"`
	Config map[string]interface{} `json:"Config"`
	// A combination of format and values or just dir should be specified
	TracesDir string `json:"TracesDir"`

	TracesFormat string        `json:"TracesFormat"`
	TraceValues  []interface{} `json:"TraceValues"`

	// Optional
	OutputDir     string         `json:"OutputDir"`
	Verbosity     string         `json:"Verbosity"`
	IatGeneration bool           `json:"IatGeneration"`
	Generated     bool           `json:"Generated"`
	PreScript     string         `json:"PreScript"`
	PostScript    string         `json:"PostScript"`
	Sweep         []SweepOptions `json:"Sweep"`
	SweepType     string         `json:"SweepType"`
}

type LoaderExperiment struct {
	Name          string                 `json:"Name"`
	Config        map[string]interface{} `json:"Config"`
	OutputDir     string                 `json:"OutputDir"`
	Verbosity     string                 `json:"Verbosity"`
	IatGeneration bool                   `json:"IatGeneration"`
	Generated     bool                   `json:"Generated"`
	PreScript     string                 `json:"PreScript"`
	PostScript    string                 `json:"PostScript"`
}

type SweepOptions struct {
	Field  string        `json:"Field"`
	Values []interface{} `json:"Values"`
	Format string        `json:"Format"`
}

func (so *SweepOptions) Validate() error {
	if so.Field == "" {
		return errors.New("field should not be empty")
	}

	if so.Field == "TracePath" || so.Field == "OutputDir" || so.Field == "Platform" {
		return errors.New(so.Field + " is a reserved field")
	}

	if len(so.Values) == 0 {
		return errors.New(so.Field + " missing sweep values")
	}
	if so.Format != "" && !strings.Contains(so.Format, FORMAT_PLACEHOLDER) {
		return errors.New("Invalid format, expected " + FORMAT_PLACEHOLDER + " in " + so.Format)
	}
	return nil
}

func (so *SweepOptions) GetValue(index int) interface{} {
	if index >= len(so.Values) {
		log.Fatal("Index out of range when getting value from sweep options")
	}
	// Check if format is specified
	if so.Format == "" {
		return so.Values[index]
	}
	// Use format
	return strings.ReplaceAll(so.Format, FORMAT_PLACEHOLDER, fmt.Sprintf("%v", so.Values[index]))
}
