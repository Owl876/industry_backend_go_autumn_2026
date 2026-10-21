package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type TaskRepo interface {
	Create(title string) (Task, error)
	Get(id string) (Task, bool)
	List(done bool) []Task
	SetDone(id string, done bool) (Task, error)
}
type Clock interface{ Now() time.Time }

var (
	ErrNotFound     = errors.New("task not found")
	ErrInvalidTitle = errors.New("invalid title")
)

type inMemoryTaskRepo struct {
	mu    sync.RWMutex
	clock Clock
	seq   uint64
	tasks map[string]Task
}

func NewInMemoryTaskRepo(clock Clock) TaskRepo {
	return &inMemoryTaskRepo{
		clock: clock,
		tasks: make(map[string]Task),
	}
}

func (r *inMemoryTaskRepo) Create(title string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, ErrInvalidTitle
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.seq++
	t := Task{
		ID:        strconv.FormatUint(r.seq, 10),
		Title:     title,
		Done:      false,
		UpdatedAt: r.clock.Now(),
	}
	r.tasks[t.ID] = t
	return t, nil
}

func (r *inMemoryTaskRepo) Get(id string) (Task, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tasks[id]
	return t, ok
}

func (r *inMemoryTaskRepo) List(done bool) []Task {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := make([]Task, 0)
	for _, t := range r.tasks {
		if t.Done == done {
			res = append(res, t)
		}
	}
	return res
}

func (r *inMemoryTaskRepo) SetDone(id string, done bool) (Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}
	if t.Done != done {
		t.Done = done
		t.UpdatedAt = r.clock.Now()
		r.tasks[id] = t
	}
	return t, nil
}

type httpHandler struct{ repo TaskRepo }

func NewHTTPHandler(repo TaskRepo) http.Handler {
	return &httpHandler{repo: repo}
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if path == "/tasks" {
		switch r.Method {
		case http.MethodPost:
			h.handleCreate(w, r)
		case http.MethodGet:
			h.handleList(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	if strings.HasPrefix(path, "/tasks/") {
		id := strings.TrimPrefix(path, "/tasks/")
		if id == "" || strings.Contains(id, "/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.handleGet(w, r, id)
		case http.MethodPatch:
			h.handlePatch(w, r, id)
		default:
			w.Header().Set("Allow", "GET, PATCH")
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

func (h *httpHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title *string `json:"title"`
	}
	if err := decodeStrictJSON(r.Body, &req); err != nil || req.Title == nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	task, err := h.repo.Create(*req.Title)
	if err != nil {
		if errors.Is(err, ErrInvalidTitle) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h *httpHandler) handleGet(w http.ResponseWriter, r *http.Request, id string) {
	task, ok := h.repo.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *httpHandler) handleList(w http.ResponseWriter, r *http.Request) {
	done := false
	if vals, ok := r.URL.Query()["done"]; ok {
		if len(vals) != 1 {
			writeError(w, http.StatusBadRequest, "invalid done")
			return
		}
		switch vals[0] {
		case "true":
			done = true
		case "false":
			done = false
		default:
			writeError(w, http.StatusBadRequest, "invalid done")
			return
		}
	}

	src := h.repo.List(done)
	tasks := make([]Task, len(src))
	copy(tasks, src)

	sort.Slice(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			return a.UpdatedAt.After(b.UpdatedAt)
		}
		return a.ID < b.ID
	})

	writeJSON(w, http.StatusOK, tasks)
}

func (h *httpHandler) handlePatch(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Done *bool `json:"done"`
	}
	if err := decodeStrictJSON(r.Body, &req); err != nil || req.Done == nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	task, err := h.repo.SetDone(id, *req.Done)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func decodeStrictJSON(r io.Reader, v any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("unexpected data after JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
