//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

type apiClient struct {
	baseURL string
	http    *http.Client
}

type errorEnvelope struct {
	Error struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

type reserveResponse struct {
	Status        string    `json:"status"`
	ReservationID string    `json:"reservation_id"`
	ItemID        string    `json:"item_id"`
	Quantity      int       `json:"quantity"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type confirmResponse struct {
	Status        string    `json:"status"`
	ReservationID string    `json:"reservation_id"`
	ConfirmedAt   time.Time `json:"confirmed_at"`
}

type stockResponse struct {
	ItemID         string `json:"item_id"`
	TotalStock     int    `json:"total_stock"`
	ReservedStock  int    `json:"reserved_stock"`
	AvailableStock int    `json:"available_stock"`
}

func (c apiClient) do(method, path, contentType string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return 0, nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, out, nil
}

func (c apiClient) send(t *testing.T, method, path, contentType string, body []byte) (int, []byte) {
	t.Helper()
	status, out, err := c.do(method, path, contentType, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return status, out
}

func (c apiClient) post(t *testing.T, path string, payload any) (int, []byte) {
	t.Helper()
	return c.send(t, http.MethodPost, path, "application/json", jsonBody(payload))
}

func (c apiClient) postRaw(t *testing.T, path, contentType, raw string) (int, []byte) {
	t.Helper()
	return c.send(t, http.MethodPost, path, contentType, []byte(raw))
}

func (c apiClient) get(t *testing.T, path string) (int, []byte) {
	t.Helper()
	return c.send(t, http.MethodGet, path, "", nil)
}

func jsonBody(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func decode[T any](t *testing.T, body []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, body)
	}
	return v
}

func requireStatus(t *testing.T, got, want int, body []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("status: got %d, want %d\nbody: %s", got, want, body)
	}
}

func requireErrorCode(t *testing.T, body []byte, want string) {
	t.Helper()
	env := decode[errorEnvelope](t, body)
	if env.Error.Code != want {
		t.Fatalf("error.code: got %q, want %q\nbody: %s", env.Error.Code, want, body)
	}
	if env.Error.Message == "" {
		t.Fatalf("error.message is empty\nbody: %s", body)
	}
}
