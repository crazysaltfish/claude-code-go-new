package appagent

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
)

// Handler serves the protocol on a private socket allocated to one App process.
func Handler(agent Agent) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/complete", func(w http.ResponseWriter, r *http.Request) {
		var input Completion
		if !decode(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 40<<10 ||
			len(input.System) > 8<<10 || input.MaxTokens < 1 || input.MaxTokens > 4096 {
			send(w, http.StatusBadRequest, reply{Error: "invalid completion input"})
			return
		}
		answer, err := agent.Complete(r.Context(), input)
		if err != nil {
			send(w, http.StatusBadGateway, reply{Error: err.Error()})
			return
		}
		send(w, http.StatusOK, reply{Text: answer})
	})
	mux.HandleFunc("POST /v1/search", func(w http.ResponseWriter, r *http.Request) {
		var input Search
		if !decode(w, r, &input) {
			return
		}
		if strings.TrimSpace(input.Query) == "" || len(input.Query) > 1000 || len(input.English) > 1000 {
			send(w, http.StatusBadRequest, reply{Error: "invalid search input"})
			return
		}
		result, err := agent.Search(r.Context(), input)
		if err != nil {
			send(w, http.StatusBadGateway, reply{Error: err.Error()})
			return
		}
		send(w, http.StatusOK, result)
	})
	inFlight := make(chan struct{}, 2)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case inFlight <- struct{}{}:
			defer func() { <-inFlight }()
			mux.ServeHTTP(w, r)
		default:
			send(w, http.StatusTooManyRequests, reply{Error: "App Agent is busy"})
		}
	})
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "application/json" {
		send(w, http.StatusUnsupportedMediaType, reply{Error: "Content-Type must be application/json"})
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil || decoder.Decode(new(any)) != io.EOF {
		send(w, http.StatusBadRequest, reply{Error: "invalid App Agent request"})
		return false
	}
	return true
}

func send(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
