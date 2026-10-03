package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"opsflow/backend/internal/model"
	"opsflow/backend/internal/service"
)

const maxItemRequestBody = 1 << 20

type itemHandlers struct {
	service *service.ItemService
	logger  *slog.Logger
}

func mountItemRoutes(router chi.Router, authService *service.AuthService, itemService *service.ItemService, logger *slog.Logger) {
	handlers := itemHandlers{service: itemService, logger: logger}
	router.With(requireAuth(authService, logger)).Post("/items", handlers.create)
	router.With(requireAuth(authService, logger)).Get("/items", handlers.list)
	router.With(requireAuth(authService, logger)).Get("/views/counts", handlers.counts)
	router.With(requireAuth(authService, logger)).Get("/items/{itemID}", handlers.get)
	router.With(requireAuth(authService, logger)).Patch("/items/{itemID}", handlers.patch)
	router.With(requireAuth(authService, logger)).Post("/items/{itemID}/claim", handlers.claim)
	router.With(requireAuth(authService, logger)).Post("/items/{itemID}/assign", handlers.assign)
	router.With(requireAuth(authService, logger)).Post("/items/{itemID}/transition", handlers.transition)
	router.With(requireAuth(authService, logger)).Post("/items/{itemID}/approvals/{approvalID}/decide", handlers.decideApproval)
	router.With(requireAuth(authService, logger)).Get("/items/{itemID}/events", handlers.events)
	router.With(requireAuth(authService, logger)).Get("/teams/{teamID}/field-schemas", handlers.teamFieldSchemas)
	router.With(requireAuth(authService, logger)).Get("/teams/{teamID}/members", handlers.teamMembers)
}

func (h itemHandlers) create(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	idempotencyKey, err := requireIdempotencyKey(r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	body, err := readRequestBody(w, r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	var input service.CreateItemInput
	if err := decodeJSON(body, &input); err != nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "request body is invalid", nil), h.logger)
		return
	}
	result, err := h.service.Create(r.Context(), user, input, idempotencyKey, service.HashRequest(r.Method, r.URL.Path, body))
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeMutation(w, result)
}

func (h itemHandlers) list(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	query := r.URL.Query()
	input := service.ItemListInput{
		View: query.Get("view"), TeamID: query.Get("team"), Status: query.Get("status"),
		AssigneeID: query.Get("assignee"), Query: query.Get("q"), Cursor: query.Get("cursor"),
	}
	if rawPriority := query.Get("priority"); rawPriority != "" {
		priority, err := strconv.ParseInt(rawPriority, 10, 16)
		if err != nil {
			writeError(w, r, service.NewAppError(service.KindValidation, "priority must be an integer from 1 to 4", nil), h.logger)
			return
		}
		input.Priority = int16(priority)
		input.PrioritySet = true
	}
	if rawLimit := query.Get("limit"); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, r, service.NewAppError(service.KindValidation, "limit must be an integer", nil), h.logger)
			return
		}
		input.Limit = limit
		input.LimitSet = true
	}
	result, err := h.service.List(r.Context(), user, input)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, result)
}

func (h itemHandlers) get(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	item, err := h.service.Get(r.Context(), user, chi.URLParam(r, "itemID"))
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	w.Header().Set("ETag", formatETag(item.Version))
	writeJSON(w, stdhttp.StatusOK, item)
}

func (h itemHandlers) patch(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	rawVersion := strings.TrimSpace(r.Header.Get("If-Match"))
	if rawVersion == "" {
		writeError(w, r, service.ErrPreconditionRequired, h.logger)
		return
	}
	version, err := parseIfMatch(rawVersion)
	if err != nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "If-Match must contain one positive item version", nil), h.logger)
		return
	}
	body, err := readRequestBody(w, r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	var patch map[string]json.RawMessage
	if err := decodeJSON(body, &patch); err != nil || patch == nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "request body must be a JSON object", nil), h.logger)
		return
	}
	result, err := h.service.Patch(r.Context(), user, chi.URLParam(r, "itemID"), version, patch, service.HashRequest(r.Method, r.URL.Path, body))
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	var item model.WorkItem
	if err := json.Unmarshal(result.Body, &item); err == nil {
		w.Header().Set("ETag", formatETag(item.Version))
	}
	writeMutation(w, result)
}

func (h itemHandlers) claim(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	idempotencyKey, err := requireIdempotencyKey(r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	body, err := readRequestBody(w, r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	itemID := chi.URLParam(r, "itemID")
	result, err := h.service.Claim(r.Context(), user, itemID, idempotencyKey, service.HashRequest(r.Method, r.URL.Path, body))
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeMutation(w, result)
}

func (h itemHandlers) assign(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	rawVersion := strings.TrimSpace(r.Header.Get("If-Match"))
	if rawVersion == "" {
		writeError(w, r, service.ErrPreconditionRequired, h.logger)
		return
	}
	version, err := parseIfMatch(rawVersion)
	if err != nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "If-Match must contain one positive item version", nil), h.logger)
		return
	}
	body, err := readRequestBody(w, r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	var input struct {
		AssigneeID string `json:"assignee_id"`
		Reason     string `json:"reason"`
	}
	if err := decodeJSON(body, &input); err != nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "request body is invalid", nil), h.logger)
		return
	}
	itemID := chi.URLParam(r, "itemID")
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	result, err := h.service.Assign(r.Context(), user, itemID, version, input.AssigneeID, input.Reason, idempotencyKey, service.HashRequest(r.Method, r.URL.Path, body))
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeMutation(w, result)
}

func (h itemHandlers) transition(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	idempotencyKey, err := requireIdempotencyKey(r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	rawVersion := strings.TrimSpace(r.Header.Get("If-Match"))
	if rawVersion == "" {
		writeError(w, r, service.ErrPreconditionRequired, h.logger)
		return
	}
	version, err := parseIfMatch(rawVersion)
	if err != nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "If-Match must contain one positive item version", nil), h.logger)
		return
	}
	body, err := readRequestBody(w, r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	var input struct {
		To     string  `json:"to"`
		Reason *string `json:"reason"`
	}
	if err := decodeJSON(body, &input); err != nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "request body is invalid", nil), h.logger)
		return
	}
	itemID := chi.URLParam(r, "itemID")
	result, err := h.service.Transition(r.Context(), user, itemID, version, input.To, input.Reason, idempotencyKey, service.HashRequest(r.Method, r.URL.Path, body))
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeMutation(w, result)
}

func (h itemHandlers) decideApproval(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	idempotencyKey, err := requireIdempotencyKey(r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	body, err := readRequestBody(w, r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	var input struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := decodeJSON(body, &input); err != nil {
		writeError(w, r, service.NewAppError(service.KindValidation, "request body is invalid", nil), h.logger)
		return
	}
	itemID := chi.URLParam(r, "itemID")
	approvalID := chi.URLParam(r, "approvalID")
	result, err := h.service.DecideApproval(r.Context(), user, itemID, approvalID, input.Decision, input.Reason, idempotencyKey, service.HashRequest(r.Method, r.URL.Path, body))
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeMutation(w, result)
}

func (h itemHandlers) counts(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	counts, err := h.service.Counts(r.Context(), user)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, counts)
}

func (h itemHandlers) events(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	limit, err := queryLimit(r)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	result, err := h.service.Events(r.Context(), user, chi.URLParam(r, "itemID"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, result)
}

func (h itemHandlers) teamFieldSchemas(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	schemas, err := h.service.TeamFieldSchemas(r.Context(), user, chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, schemas)
}

func (h itemHandlers) teamMembers(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	user, ok := contextUser(r)
	if !ok {
		writeError(w, r, service.ErrUnauthorized, h.logger)
		return
	}
	members, err := h.service.TeamMembers(r.Context(), user, chi.URLParam(r, "teamID"))
	if err != nil {
		writeError(w, r, err, h.logger)
		return
	}
	writeJSON(w, stdhttp.StatusOK, members)
}

func contextUser(r *stdhttp.Request) (service.User, bool) {
	user, ok := r.Context().Value(currentUserKey{}).(service.User)
	return user, ok
}

func readRequestBody(w stdhttp.ResponseWriter, r *stdhttp.Request) ([]byte, error) {
	body, err := io.ReadAll(stdhttp.MaxBytesReader(w, r.Body, maxItemRequestBody))
	if err != nil {
		return nil, service.NewAppError(service.KindValidation, "request body is too large or unreadable", nil)
	}
	return body, nil
}

func decodeJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("request must contain exactly one JSON value")
	}
	return nil
}

func writeMutation(w stdhttp.ResponseWriter, result service.MutationResponse) {
	if result.Replayed {
		w.Header().Set("Idempotent-Replay", "true")
	}
	var item model.WorkItem
	if json.Unmarshal(result.Body, &item) == nil && item.Version > 0 {
		w.Header().Set("ETag", formatETag(item.Version))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(result.Body)
	_, _ = w.Write([]byte("\n"))
}

func requireIdempotencyKey(r *stdhttp.Request) (string, error) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || !service.IsUUID(key) {
		return "", service.NewAppError(service.KindValidation, "Idempotency-Key header is required and must be a UUID", nil)
	}
	return key, nil
}

func formatETag(version int) string {
	return `"` + strconv.Itoa(version) + `"`
}

func parseIfMatch(value string) (int, error) {
	if strings.Contains(value, ",") || strings.HasPrefix(value, "W/") {
		return 0, fmt.Errorf("If-Match must contain one strong entity tag")
	}
	if strings.HasPrefix(value, `"`) || strings.HasSuffix(value, `"`) {
		if len(value) < 2 || !strings.HasPrefix(value, `"`) || !strings.HasSuffix(value, `"`) {
			return 0, fmt.Errorf("invalid quoted If-Match version")
		}
		value = value[1 : len(value)-1]
	} else if strings.Contains(value, `"`) {
		return 0, fmt.Errorf("invalid quoted If-Match version")
	}
	version, err := strconv.Atoi(value)
	if err != nil || version < 1 {
		return 0, fmt.Errorf("invalid If-Match version")
	}
	return version, nil
}

func queryLimit(r *stdhttp.Request) (int, error) {
	rawLimit := r.URL.Query().Get("limit")
	if rawLimit == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(rawLimit)
	if err != nil {
		return 0, service.NewAppError(service.KindValidation, "limit must be an integer", nil)
	}
	if limit < 1 {
		return 0, service.NewAppError(service.KindValidation, "limit must be positive", nil)
	}
	return limit, nil
}
