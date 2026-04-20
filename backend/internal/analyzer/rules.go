package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
)

type Rules struct {
	System      string
	InputRules  string
	Quantizer   string
	Algorithm   string
	OutputRules string
	StageModel  string
}

func LoadRules(root string) (Rules, error) {
	read := func(parts ...string) (string, error) {
		path := filepath.Join(append([]string{root}, parts...)...)
		bytes, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		return string(bytes), nil
	}

	system, err := read("SKILL.md")
	if err != nil {
		return Rules{}, err
	}
	inputRules, err := read("references", "input-rules.md")
	if err != nil {
		return Rules{}, err
	}
	quantizer, err := read("references", "data-quantization-sop.md")
	if err != nil {
		return Rules{}, err
	}
	algorithm, err := read("references", "algorithm-weight.md")
	if err != nil {
		return Rules{}, err
	}
	outputRules, err := read("references", "output-rules.md")
	if err != nil {
		return Rules{}, err
	}
	stageModel, err := read("references", "stage-model.md")
	if err != nil {
		return Rules{}, err
	}

	return Rules{
		System:      system,
		InputRules:  inputRules,
		Quantizer:   quantizer,
		Algorithm:   algorithm,
		OutputRules: outputRules,
		StageModel:  stageModel,
	}, nil
}
