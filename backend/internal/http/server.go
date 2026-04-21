package http

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"senti/backend/internal/analyzer"
	"senti/backend/internal/config"
	"senti/backend/internal/store"
)

type Server struct {
	config   config.Config
	repo     store.Repository
	analyzer *analyzer.Service
	logger   *slog.Logger
}

func NewServer(cfg config.Config, repo store.Repository, analyzerService *analyzer.Service, logger *slog.Logger) http.Handler {
	server := &Server{
		config:   cfg,
		repo:     repo,
		analyzer: analyzerService,
		logger:   logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/api/ai/availability", server.handleAIAvailability)
	mux.HandleFunc("/api/history", server.handleHistory)
	mux.HandleFunc("/api/history/", server.handleHistoryDetail)
	mux.HandleFunc("/api/analyze/text", server.handleAnalyzeText)
	mux.HandleFunc("/api/analyze/image", server.handleAnalyzeImage)

	return server.withMiddleware(mux)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "same-origin")

		origin := request.Header.Get("Origin")
		if origin == s.config.CORSOrigin {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Vary", "Origin")
			writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		}

		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		start := time.Now()
		next.ServeHTTP(writer, request)
		s.logger.Info("request completed", "method", request.Method, "path", request.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.repo.Ping(request.Context()); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAIAvailability(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	result, err := s.analyzer.KimiAvailability(request.Context())
	if err != nil {
		writeJSON(writer, http.StatusBadGateway, result)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (s *Server) handleHistory(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 20
	if queryLimit := request.URL.Query().Get("limit"); queryLimit != "" {
		if parsed, err := strconv.Atoi(queryLimit); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	items, err := s.repo.ListAnalyses(request.Context(), limit)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "failed to load history")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleHistoryDetail(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := strings.TrimPrefix(request.URL.Path, "/api/history/")
	if id == "" {
		writeError(writer, http.StatusBadRequest, "missing history id")
		return
	}
	record, err := s.repo.GetAnalysis(request.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(writer, http.StatusNotFound, "analysis not found")
			return
		}
		writeError(writer, http.StatusInternalServerError, "failed to load analysis")
		return
	}
	writeJSON(writer, http.StatusOK, record)
}

func (s *Server) handleAnalyzeText(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(payload.Text) == "" {
		writeError(writer, http.StatusBadRequest, "text is required")
		return
	}

	record, err := s.analyzer.AnalyzeText(request.Context(), payload.Text)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, record)
}

func (s *Server) handleAnalyzeImage(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := request.ParseMultipartForm(20 << 20); err != nil {
		writeError(writer, http.StatusBadRequest, "failed to parse uploaded image")
		return
	}

	file, header, err := request.FormFile("image")
	if err != nil {
		writeError(writer, http.StatusBadRequest, "image is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "failed to read uploaded image")
		return
	}

	path, err := s.analyzer.StoreUpload(s.config.UploadDir, header.Filename, data)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "failed to store upload")
		return
	}

	record, err := s.analyzer.AnalyzeImage(request.Context(), path)
	if err != nil {
		_ = os.Remove(path)
		writeError(writer, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(writer, http.StatusOK, record)
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}
