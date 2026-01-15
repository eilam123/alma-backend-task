package ebpf_agent

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/alma/assignment/models"
)

type EBPFAgent struct {
	dataPath string
}

func NewEBPFAgent(dataPath string) *EBPFAgent {
	return &EBPFAgent{dataPath: dataPath}
}

func (a *EBPFAgent) GetSpans() ([]models.RawSpan, error) {
	data, err := os.ReadFile(a.dataPath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var spansFile models.RawSpansFile
	if err := json.Unmarshal(data, &spansFile); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	return spansFile.Spans, nil
}
