package analyzer

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

type OCRProvider interface {
	ExtractText(ctx context.Context, filePath string) (string, error)
}

type TesseractOCR struct {
	language string
	logger   *slog.Logger
}

func NewTesseractOCR(language string, logger *slog.Logger) *TesseractOCR {
	return &TesseractOCR{language: language, logger: logger}
}

func (o *TesseractOCR) ExtractText(ctx context.Context, filePath string) (string, error) {
	command := exec.CommandContext(ctx, "tesseract", filePath, "stdout", "-l", o.language)
	output, err := command.CombinedOutput()
	if err != nil {
		o.logger.Warn("tesseract OCR failed", "error", err, "output", string(output))
		return "", fmt.Errorf("ocr failed: %w", err)
	}
	text := strings.TrimSpace(string(output))
	if text == "" {
		return "", fmt.Errorf("ocr extracted no text")
	}
	return text, nil
}
