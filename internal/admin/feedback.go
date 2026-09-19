package admin

import (
	"net/http"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/feedback"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/httpx"
)

// The operator's half of internal/feedback: read everything, mark one dealt
// with, throw one away. Writing a report is the author's own endpoint, and
// an operator sending one uses that same one — this file never creates a
// report on somebody else's behalf, because a report is a thing a person
// said.

func (h *Handlers) listFeedback(w http.ResponseWriter, r *http.Request) error {
	query := r.URL.Query()
	filter := feedback.Filter{
		Kind:     feedback.Kind(query.Get("kind")),
		Priority: feedback.Priority(query.Get("priority")),
		Status:   feedback.Status(query.Get("status")),
		UserID:   query.Get("user_id"),
		Search:   query.Get("q"),
		Limit:    intParam(query.Get("limit"), 20),
		Offset:   intParam(query.Get("offset"), 0),
	}
	// An unknown enum would narrow to nothing and read as "there is no
	// feedback", which is the one answer this screen must never give by
	// accident.
	if filter.Kind != "" && !filter.Kind.Valid() {
		return httpx.BadRequest("Unknown feedback kind.")
	}
	if filter.Priority != "" && !filter.Priority.Valid() {
		return httpx.BadRequest("Unknown priority.")
	}
	if filter.Status != "" && !filter.Status.Valid() {
		return httpx.BadRequest("Unknown status.")
	}

	records, total, err := h.feedback.List(r.Context(), nil, filter)
	if err != nil {
		return httpx.Internal(err)
	}
	summary, err := h.feedback.Counts(r.Context(), nil)
	if err != nil {
		return httpx.Internal(err)
	}

	return httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"feedback": records,
		"total":    total,
		"offset":   filter.Offset,
		"summary":  summary,
	})
}

type feedbackUpdateRequest struct {
	Status string `json:"status"`
}

func (h *Handlers) updateFeedback(w http.ResponseWriter, r *http.Request) error {
	feedbackID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var body feedbackUpdateRequest
	if err := httpx.DecodeJSON(w, r, &body, 4*1024); err != nil {
		return err
	}
	record, err := h.feedback.SetStatus(r.Context(), feedbackID, feedback.Status(body.Status))
	if err != nil {
		return feedback.TranslateError(err)
	}
	return httpx.WriteJSON(w, http.StatusOK, record)
}

func (h *Handlers) deleteFeedback(w http.ResponseWriter, r *http.Request) error {
	feedbackID, err := pathID(r, "id")
	if err != nil {
		return err
	}
	if err := h.feedback.Delete(r.Context(), feedbackID); err != nil {
		return feedback.TranslateError(err)
	}
	return httpx.NoContent(w)
}
