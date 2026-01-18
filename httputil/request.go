package httputil

import (
	"bytes"
	"io"
	"net/http"
)

// CloneRequest creates a clone of an HTTP request with its body preserved.
// This is useful for retrying requests, as the body can only be read once.
//
// If req.GetBody is set (e.g., by http.NewRequest), it uses that to recreate the body.
// Otherwise, it reads the body, restores the original, and sets the clone's body.
func CloneRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())

	if req.Body == nil {
		return clone, nil
	}

	// If GetBody is available, use it (more efficient)
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = body
		return clone, nil
	}

	// Fallback: read the body, restore original, and set clone
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	// Restore original body
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Set clone body
	clone.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	return clone, nil
}
