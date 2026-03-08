package ebpf_agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
	slog.Default().Info("loading spans", "path", a.dataPath)

	data, err := os.ReadFile(a.dataPath)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var spansFile models.RawSpansFile
	if err := json.Unmarshal(data, &spansFile); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	slog.Default().Info("spans loaded", "count", len(spansFile.Spans))
	return spansFile.Spans, nil
}
