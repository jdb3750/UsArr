package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// maxRequestBody caps a decoded JSON request body. Every state-changing endpoint
// here takes a small object; nothing legitimate approaches this.
const maxRequestBody = 1 << 20 // 1 MB

// errorBody is the single error shape every endpoint returns.
//
// Message is written to be shown to a user and Action names the one thing that
// fixes it, which is the same contract the Services screen holds itself to
// (§17.3). Neither ever carries a credential: both go through redactText.
type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

// apiError is a handler-level failure carrying the status it should produce.
type apiError struct {
	Status  int
	Code    string
	Message string
	Action  string
	Err     error
}

func (e *apiError) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *apiError) Unwrap() error { return e.Err }

func errStatus(status int, code, message string) *apiError {
	return &apiError{Status: status, Code: code, Message: message}
}

func (e *apiError) withAction(action string) *apiError {
	e.Action = action
	return e
}

func (e *apiError) wrapping(err error) *apiError {
	e.Err = err
	return e
}

// writeJSON renders v. It marshals into memory first so that a marshal failure
// produces a 500 rather than a 200 with a truncated body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	buf, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"encode","message":"the response could not be encoded"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

// decodeJSON reads a request body into v with a size cap and no unknown fields.
//
// DisallowUnknownFields is deliberate on a configuration API: a typo'd field name
// that is silently ignored means the user saves a service and the setting they
// typed does nothing.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errStatus(http.StatusBadRequest, "bad_request",
			"the request body is not the JSON this endpoint expects: "+redactText(err.Error())).wrapping(err)
	}
	// Exactly one JSON document, not a stream: a second one would be silently
	// discarded, and "the second half of my request was ignored" is a very hard
	// bug to see from the outside.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errStatus(http.StatusBadRequest, "bad_request", "the request body holds more than one JSON document")
	}
	return nil
}

// pathInt64 reads a {name} wildcard as an id.
func pathInt64(r *http.Request, name string) (int64, error) {
	raw := r.PathValue(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errStatus(http.StatusNotFound, "not_found", fmt.Sprintf("%q is not an id", raw))
	}
	return id, nil
}

// handler is a handler that may fail. The adapter renders the failure once, in
// one place, so no endpoint can invent its own error shape.
type handler func(http.ResponseWriter, *http.Request) error

func (s *Server) wrap(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			s.writeError(w, r, err)
		}
	}
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apiError
	if !errors.As(err, &ae) {
		ae = &apiError{
			Status:  http.StatusInternalServerError,
			Code:    "internal",
			Message: "the request could not be completed",
			Err:     err,
		}
	}

	// The log carries the underlying cause; the client does not. Both are
	// redacted — the log line because trace is not an exemption, the body
	// because an error string is one of the four places a key leaks.
	level := s.log.Info
	if ae.Status >= http.StatusInternalServerError {
		level = s.log.Error
	}
	level("request failed",
		"request", RequestLine(r.Context()),
		"status", ae.Status,
		"code", ae.Code,
		"err", redactText(ae.Error()))

	writeJSON(w, ae.Status, errorBody{
		Error:   ae.Code,
		Message: redactText(ae.Message),
		Action:  redactText(ae.Action),
	})
}

// queryInt32s parses a repeated integer query parameter. Repeated, not
// comma-separated: that is how Prowlarr's own parameters work and mixing the two
// conventions in one product is how a list silently binds to nothing.
func queryInt32s(r *http.Request, name string) ([]int32, error) {
	values := r.URL.Query()[name]
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]int32, 0, len(values))
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			n, err := strconv.ParseInt(part, 10, 32)
			if err != nil {
				return nil, errStatus(http.StatusBadRequest, "bad_request",
					fmt.Sprintf("%s=%q is not an integer", name, part))
			}
			out = append(out, int32(n))
		}
	}
	return out, nil
}
