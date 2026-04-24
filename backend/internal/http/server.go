package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"senti/backend/internal/analyzer"
	"senti/backend/internal/config"
	"senti/backend/internal/domain"
	"senti/backend/internal/observability"
	"senti/backend/internal/store"
)

const timeoutMessage = "这次分析花的时间有点久，可能是图片识别或模型响应暂时卡住了。你可以稍后再试，或先改用文本粘贴，我会尽量把上下文接住。"

type contextKey string

const requestIDKey contextKey = "request_id"

type Server struct {
	config         config.Config
	repo           store.Repository
	analyzer       *analyzer.Service
	logger         *slog.Logger
	rateLimiter    *rateLimiter
	analyzeTimeout time.Duration
}

func NewServer(cfg config.Config, repo store.Repository, analyzerService *analyzer.Service, logger *slog.Logger) http.Handler {
	analyzeTimeout, err := time.ParseDuration(cfg.AnalyzeTimeout)
	if err != nil {
		analyzeTimeout = 60 * time.Second
	}
	rateLimitWindow, err := time.ParseDuration(cfg.RateLimitWindow)
	if err != nil {
		rateLimitWindow = time.Hour
	}

	server := &Server{
		config:         cfg,
		repo:           repo,
		analyzer:       analyzerService,
		logger:         logger,
		rateLimiter:    newRateLimiter(rateLimitWindow, cfg.RateLimitRequests),
		analyzeTimeout: analyzeTimeout,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/metrics", server.handleMetrics)
	mux.HandleFunc("/api/ai/availability", server.handleAIAvailability)
	mux.HandleFunc("/api/auth/register", server.handleRegister)
	mux.HandleFunc("/api/auth/login", server.handleLogin)
	mux.HandleFunc("/api/auth/me", server.handleMe)
	mux.HandleFunc("/api/analyses/save", server.handleSaveAnalysis)
	mux.HandleFunc("/api/history", server.handleHistory)
	mux.HandleFunc("/api/history/", server.handleHistoryDetail)
	mux.HandleFunc("/api/analyze/text", server.handleAnalyzeText)
	mux.HandleFunc("/api/analyze/image", server.handleAnalyzeImage)

	return server.withMiddleware(mux)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		writer.Header().Set("X-Request-ID", requestID)
		request = request.WithContext(context.WithValue(request.Context(), requestIDKey, requestID))
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "same-origin")

		origin := request.Header.Get("Origin")
		if origin == s.config.CORSOrigin {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Vary", "Origin")
			writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			writer.Header().Set("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
		}

		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		observability.DefaultMetrics.RecordRequest(request.Method, request.URL.Path, recorder.status, time.Since(start))
		s.logger.Info(
			"request completed",
			"request_id", requestID,
			"method", request.Method,
			"path", request.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
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

func (s *Server) handleMetrics(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	dbUp := s.repo.Ping(request.Context()) == nil
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(observability.DefaultMetrics.Render(dbUp)))
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

func (s *Server) handleRegister(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"inviteCode"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid request body")
		return
	}
	username := strings.TrimSpace(payload.Username)
	if username == "" || len(payload.Password) < 8 {
		writeError(writer, http.StatusBadRequest, "用户名不能为空，密码至少 8 位")
		return
	}
	if s.config.InviteCode == "" {
		writeError(writer, http.StatusForbidden, "邀请码无效")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(payload.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "failed to create user")
		return
	}
	user := domain.User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.repo.CreateUserWithInvite(request.Context(), user, strings.TrimSpace(payload.InviteCode)); err != nil {
		if errors.Is(err, store.ErrInviteInvalid) {
			writeError(writer, http.StatusForbidden, "邀请码无效或已被使用")
			return
		}
		writeError(writer, http.StatusConflict, "用户已存在或无法创建")
		return
	}
	s.writeAuthResponse(writer, user)
}

func (s *Server) handleLogin(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := s.repo.GetUserByUsername(request.Context(), strings.TrimSpace(payload.Username))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(payload.Password)) != nil {
		writeError(writer, http.StatusUnauthorized, "用户名或密码不正确")
		return
	}
	s.writeAuthResponse(writer, user)
}

func (s *Server) handleMe(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"user": map[string]string{"id": claims.UserID, "username": claims.Username},
	})
}

func (s *Server) writeAuthResponse(writer http.ResponseWriter, user domain.User) {
	token, err := signToken(s.config.AuthTokenSecret, authClaims{
		UserID:   user.ID,
		Username: user.Username,
		Expires:  time.Now().Add(14 * 24 * time.Hour).Unix(),
	})
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "failed to issue token")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"token": token,
		"user":  map[string]string{"id": user.ID, "username": user.Username},
	})
}

func (s *Server) handleHistory(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	limit := 20
	if queryLimit := request.URL.Query().Get("limit"); queryLimit != "" {
		if parsed, err := strconv.Atoi(queryLimit); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	items, err := s.repo.ListAnalyses(request.Context(), claims.UserID, limit)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "failed to load history")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleHistoryDetail(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodDelete {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	id := strings.TrimPrefix(request.URL.Path, "/api/history/")
	if id == "" {
		writeError(writer, http.StatusBadRequest, "missing history id")
		return
	}
	if request.Method == http.MethodDelete {
		imagePath, err := s.repo.DeleteAnalysis(request.Context(), claims.UserID, id)
		observability.DefaultMetrics.RecordDelete(imagePath != "", err)
		if err != nil {
			if err == store.ErrNotFound {
				writeError(writer, http.StatusNotFound, "analysis not found")
				return
			}
			s.logger.Warn("analysis delete failed", "request_id", requestID(request), "user_id", claims.UserID, "reason", "repository")
			writeError(writer, http.StatusInternalServerError, "failed to delete analysis")
			return
		}
		if imagePath != "" {
			_ = os.Remove(imagePath)
		}
		s.logger.Info("analysis deleted", "request_id", requestID(request), "user_id", claims.UserID, "had_image", imagePath != "")
		writeJSON(writer, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	record, err := s.repo.GetAnalysis(request.Context(), claims.UserID, id)
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
	claims, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	if !s.rateLimiter.Allow(claims.UserID) {
		observability.DefaultMetrics.RecordRateLimited()
		writeError(writer, http.StatusTooManyRequests, "分析次数有点密集，请稍后再试")
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(request.Context(), s.analyzeTimeout)
	defer cancel()
	record, err := s.analyzer.AnalyzeText(ctx, payload.Text)
	observability.DefaultMetrics.RecordAnalysis("text", false, err, time.Since(start))
	if err != nil {
		s.logger.Warn("analysis failed", "request_id", requestID(request), "user_id", claims.UserID, "input_type", "text", "reason", safeErrorReason(err))
		s.writeAnalysisError(writer, err)
		return
	}
	record.UserID = claims.UserID
	s.logger.Info("analysis completed", "request_id", requestID(request), "user_id", claims.UserID, "input_type", "text", "stage", record.Result.Stage, "saved", false, "ocr_input", false)
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

	claims, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	if !s.rateLimiter.Allow(claims.UserID) {
		observability.DefaultMetrics.RecordRateLimited()
		writeError(writer, http.StatusTooManyRequests, "分析次数有点密集，请稍后再试")
		return
	}

	s.analyzer.CleanupTempUploads(s.config.TempUploadDir, 24*time.Hour)
	path, err := s.analyzer.StoreUpload(s.config.TempUploadDir, header.Filename, data)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "failed to store upload")
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(request.Context(), s.analyzeTimeout)
	defer cancel()
	record, err := s.analyzer.AnalyzeImage(ctx, path)
	observability.DefaultMetrics.RecordAnalysis("image", false, err, time.Since(start))
	if err != nil {
		_ = os.Remove(path)
		s.logger.Warn("analysis failed", "request_id", requestID(request), "user_id", claims.UserID, "input_type", "image", "reason", safeErrorReason(err), "ocr_input", true)
		s.writeAnalysisError(writer, err)
		return
	}
	record.UserID = claims.UserID
	s.logger.Info("analysis completed", "request_id", requestID(request), "user_id", claims.UserID, "input_type", "image", "stage", record.Result.Stage, "saved", false, "ocr_input", true)
	writeJSON(writer, http.StatusOK, record)
}

func (s *Server) handleSaveAnalysis(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	claims, ok := s.authenticate(writer, request)
	if !ok {
		return
	}
	var record domain.AnalysisRecord
	if err := json.NewDecoder(request.Body).Decode(&record); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid request body")
		return
	}
	if record.ID == "" || record.InputType == "" || record.CreatedAt.IsZero() {
		writeError(writer, http.StatusBadRequest, "invalid analysis record")
		return
	}
	if record.ImagePath != "" {
		path, err := s.analyzer.PromoteUpload(record.ImagePath, s.config.UploadDir)
		if err != nil {
			observability.DefaultMetrics.RecordSave(record.InputType, err)
			s.logger.Warn("analysis save failed", "request_id", requestID(request), "user_id", claims.UserID, "reason", "persist_upload")
			writeError(writer, http.StatusInternalServerError, "failed to persist upload")
			return
		}
		record.ImagePath = path
	}
	record.UserID = claims.UserID
	record.Saved = true
	err := s.repo.CreateAnalysis(request.Context(), claims.UserID, record)
	observability.DefaultMetrics.RecordSave(record.InputType, err)
	if err != nil {
		s.logger.Warn("analysis save failed", "request_id", requestID(request), "user_id", claims.UserID, "input_type", record.InputType, "stage", record.Result.Stage, "reason", "repository")
		writeError(writer, http.StatusInternalServerError, "failed to save analysis")
		return
	}
	s.logger.Info("analysis saved", "request_id", requestID(request), "user_id", claims.UserID, "input_type", record.InputType, "stage", record.Result.Stage, "saved", true, "ocr_input", record.InputType == "image")
	writeJSON(writer, http.StatusOK, record)
}

func (s *Server) authenticate(writer http.ResponseWriter, request *http.Request) (authClaims, bool) {
	token, err := bearerToken(request.Header.Get("Authorization"))
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "请先登录")
		return authClaims{}, false
	}
	claims, err := verifyToken(s.config.AuthTokenSecret, token)
	if err != nil {
		writeError(writer, http.StatusUnauthorized, "登录状态已失效，请重新登录")
		return authClaims{}, false
	}
	return claims, true
}

func (s *Server) writeAnalysisError(writer http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		writeError(writer, http.StatusGatewayTimeout, timeoutMessage)
		return
	}
	message := err.Error()
	if strings.Contains(message, "Client.Timeout") || strings.Contains(message, "context deadline exceeded") {
		writeError(writer, http.StatusGatewayTimeout, timeoutMessage)
		return
	}
	if strings.Contains(message, "ocr") {
		writeError(writer, http.StatusBadGateway, "截图识别失败，可能是图片过长、过糊或文字太小。你可以改用文本粘贴再试。")
		return
	}
	writeError(writer, http.StatusInternalServerError, "分析失败，请稍后再试")
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]string{"error": message})
}

func requestID(request *http.Request) string {
	if value, ok := request.Context().Value(requestIDKey).(string); ok {
		return value
	}
	return ""
}

func safeErrorReason(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(message, "deadline exceeded"), strings.Contains(message, "Client.Timeout"):
		return "timeout"
	case strings.Contains(message, "ocr"):
		return "ocr_failed"
	case strings.Contains(message, "kimi"):
		return "kimi_failed"
	case strings.Contains(message, "parse"):
		return "parse_failed"
	default:
		return "analysis_failed"
	}
}
