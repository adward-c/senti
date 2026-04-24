package analyzer

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"senti/backend/internal/observability"
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
	start := time.Now()
	command := exec.CommandContext(ctx, "tesseract", filePath, "stdout", "-l", o.language)
	output, err := command.CombinedOutput()
	if err != nil {
		observability.DefaultMetrics.RecordOCR(err, time.Since(start))
		o.logger.Warn("tesseract OCR failed", "error", err, "output_bytes", len(output))
		return "", fmt.Errorf("ocr failed: %w", err)
	}
	text := strings.TrimSpace(string(output))
	if text == "" {
		err := fmt.Errorf("ocr extracted no text")
		observability.DefaultMetrics.RecordOCR(err, time.Since(start))
		return "", err
	}
	observability.DefaultMetrics.RecordOCR(nil, time.Since(start))
	return text, nil
}
