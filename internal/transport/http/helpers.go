package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)


// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // ignore encode error
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, status int, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// parseInt parses a string as an integer.
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}

// sanitize normalizes a name to alphanumeric + underscore.
func sanitize(s string) string {
	result := make([]byte, 0, len(s))
	for _, c := range []byte(s) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			result = append(result, c)
		} else {
			result = append(result, '_')
		}
	}
	if len(result) == 0 {
		return "unknown"
	}
	return string(result)
}

// bearerMatches reports whether the Authorization header value carries the expected token.
func bearerMatches(header, want string) bool {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(header), "Bearer ")) == want
}
