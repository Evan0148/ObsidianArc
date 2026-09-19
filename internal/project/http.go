package project

import (
	"errors"
	"net/http"

	"github.com/OnyxAxisOwO/ObsidianArc/internal/auth"
	"github.com/OnyxAxisOwO/ObsidianArc/internal/httpx"
)

// Handlers is where an account manages its own projects. Every route is
// scoped to the caller and there is no "list projects for user X", not even
// for an administrator: a project's instructions are part of the prompt of a
// session that holds tools, so reading somebody else's is reading what their
// agent is told to do, and writing one is telling it.
type Handlers struct{ projects *Store }

func NewHandlers(projects *Store) *Handlers { return &Handlers{projects: projects} }

func (h *Handlers) Routes(mux *http.ServeMux) {
	protected := func(handler httpx.Handler) http.Handler {
		return auth.RequireUser(httpx.Wrap(handler))
	}

	mux.Handle("GET /api/projects", protected(h.list))
	mux.Handle("POST /api/projects", protected(h.create))
	mux.Handle("GET /api/projects/{id}", protected(h.show))
	mux.Handle("PATCH /api/projects/{id}", protected(h.update))
	mux.Handle("DELETE /api/projects/{id}", protected(h.delete))
}

const maxBody = 64 * 1024

type listResponse struct {
	Projects []Project `json:"projects"`
	Max      int       `json:"max"`
}

type writeRequest struct {
	Name         *string `json:"name"`
	Instructions *string `json:"instructions"`
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())
	records, err := h.projects.List(r.Context(), account.ID)
	if err != nil {
		return httpx.Internal(err)
	}
	if records == nil {
		records = []Project{}
	}
	return httpx.WriteJSON(w, http.StatusOK, listResponse{Projects: records, Max: MaxPerUser})
}

func (h *Handlers) show(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())
	record, err := h.projects.Get(r.Context(), nil, account.ID, r.PathValue("id"))
	if err != nil {
		return translate(err)
	}
	return httpx.WriteJSON(w, http.StatusOK, record)
}

func (h *Handlers) create(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())

	var body writeRequest
	if err := httpx.DecodeJSON(w, r, &body, maxBody); err != nil {
		return err
	}
	record, err := h.projects.Create(r.Context(), account.ID,
		deref(body.Name), deref(body.Instructions))
	if err != nil {
		return translate(err)
	}
	return httpx.WriteJSON(w, http.StatusCreated, record)
}

func (h *Handlers) update(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())

	var body writeRequest
	if err := httpx.DecodeJSON(w, r, &body, maxBody); err != nil {
		return err
	}
	record, err := h.projects.Update(r.Context(), account.ID, r.PathValue("id"),
		Update{Name: body.Name, Instructions: body.Instructions})
	if err != nil {
		return translate(err)
	}
	return httpx.WriteJSON(w, http.StatusOK, record)
}

func (h *Handlers) delete(w http.ResponseWriter, r *http.Request) error {
	account := auth.MustUser(r.Context())
	if err := h.projects.Delete(r.Context(), account.ID, r.PathValue("id")); err != nil {
		return translate(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// translate turns the store's errors into the answers the screen shows. A
// project that belongs to somebody else is reported as absent rather than as
// forbidden, which is the same answer the query itself gives: the owner
// scope is in the WHERE clause, so there is nothing here that knows the
// difference.
func translate(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return httpx.NotFound("No such project.")
	case errors.Is(err, ErrTooMany):
		return httpx.BadRequest("This account already holds the maximum number of projects.")
	case errors.Is(err, ErrNameRequired):
		return httpx.BadRequest("A project needs a name.")
	case errors.Is(err, ErrNameTooLong):
		return httpx.BadRequest("That name is too long.")
	case errors.Is(err, ErrInstructionsLong):
		return httpx.BadRequest("Those instructions are too long.")
	}
	return httpx.Internal(err)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
