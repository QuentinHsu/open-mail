package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"open-mail/internal/mailbox"
	"open-mail/internal/model"
)

// Server exposes REST endpoints for mailbox management.
type Server struct {
	mailboxes *mailbox.Service
	apiToken  string
}

// NewServer creates an HTTP API server.
func NewServer(mailboxes *mailbox.Service, apiToken string) *Server {
	return &Server{mailboxes: mailboxes, apiToken: apiToken}
}

// Handler builds the complete REST handler tree.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("/v1/mailboxes", s.handleMailboxes)
	mux.HandleFunc("/v1/mailboxes/", s.handleMailboxByID)
	return s.withAuth(mux)
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	if s.apiToken == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.apiToken {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"status":    "ok",
			"timestamp": time.Now().UTC(),
		},
	})
}

func (s *Server) handleMailboxes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"data": sanitizeMailboxes(s.mailboxes.List())})
	case http.MethodPost:
		var input model.MailboxInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body must be valid JSON")
			return
		}
		created, err := s.mailboxes.Create(r.Context(), input)
		if err != nil {
			s.writeMailboxError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": sanitizeMailbox(created)})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", fmt.Sprintf("%s is not supported", r.Method))
	}
}

func (s *Server) handleMailboxByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/mailboxes/")
	if path == "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	id := parts[0]

	switch r.Method {
	case http.MethodGet:
		mailboxValue, err := s.mailboxes.Get(id)
		if err != nil {
			s.writeMailboxError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": sanitizeMailbox(mailboxValue)})
	case http.MethodPut:
		var input model.MailboxInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body must be valid JSON")
			return
		}
		updated, err := s.mailboxes.Update(r.Context(), id, input)
		if err != nil {
			s.writeMailboxError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": sanitizeMailbox(updated)})
	case http.MethodDelete:
		if err := s.mailboxes.Delete(r.Context(), id); err != nil {
			s.writeMailboxError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", fmt.Sprintf("%s is not supported", r.Method))
	}
}

func (s *Server) writeMailboxError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mailbox.ErrMailboxNotFound):
		writeError(w, http.StatusNotFound, "MAILBOX_NOT_FOUND", err.Error())
	case errors.Is(err, mailbox.ErrMailboxExists):
		writeError(w, http.StatusConflict, "MAILBOX_EXISTS", err.Error())
	default:
		writeError(w, http.StatusBadRequest, "MAILBOX_ERROR", err.Error())
	}
}

func sanitizeMailboxes(mailboxes []model.Mailbox) []map[string]any {
	result := make([]map[string]any, 0, len(mailboxes))
	for _, mailboxValue := range mailboxes {
		result = append(result, sanitizeMailbox(mailboxValue))
	}
	return result
}

func sanitizeMailbox(mailboxValue model.Mailbox) map[string]any {
	return map[string]any{
		"id":              mailboxValue.ID,
		"email":           mailboxValue.Email,
		"display_name":    mailboxValue.DisplayName,
		"imap_host":       mailboxValue.IMAPHost,
		"imap_port":       mailboxValue.IMAPPort,
		"use_tls":         mailboxValue.UseTLS,
		"last_seen_uid":   mailboxValue.LastSeenUID,
		"last_checked_at": mailboxValue.LastCheckedAt,
		"last_error":      mailboxValue.LastError,
		"created_at":      mailboxValue.CreatedAt,
		"updated_at":      mailboxValue.UpdatedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
