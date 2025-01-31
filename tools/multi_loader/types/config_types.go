package types

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
	OutputDir     string `json:"OutputDir"`
	Verbosity     string `json:"Verbosity"`
	IatGeneration bool   `json:"IatGeneration"`
	Generated     bool   `json:"Generated"`
	PreScript     string `json:"PreScript"`
	PostScript    string `json:"PostScript"`
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
