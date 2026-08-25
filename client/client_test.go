package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nemirlev/zenmoney-go-sdk/v2/models"
)

func tagRequest() models.Request {
	return models.Request{
		CurrentClientTimestamp: 1,
		ServerTimestamp:        2,
		Tag:                    []models.Tag{{ID: "tag-1", User: 7, Title: "Банковская техника"}},
	}
}

func TestPushTags_SendsNullStaticID(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer token-1" {
			t.Errorf("Authorization = %q, want bearer token", auth)
		}
		if !strings.Contains(string(body), `"staticId":null`) {
			t.Errorf("body must send staticId as null, got %s", body)
		}
		w.Write([]byte(`{"serverTimestamp":99}`))
	}))
	defer srv.Close()

	c := &Client{token: "token-1", http: &http.Client{Timeout: time.Minute}, pushTagsURL: srv.URL}
	resp, err := c.PushTags(context.Background(), tagRequest())
	if err != nil {
		t.Fatalf("PushTags() error = %v", err)
	}
	if resp.ServerTimestamp != 99 {
		t.Fatalf("ServerTimestamp = %d, want 99", resp.ServerTimestamp)
	}
	tags, ok := got["tag"].([]any)
	if !ok || len(tags) != 1 {
		t.Fatalf("request carried %v, want one tag", got["tag"])
	}
	if sid, present := tags[0].(map[string]any)["staticId"]; !present || sid != nil {
		t.Fatalf("staticId = %v (present=%v), want explicit null", sid, present)
	}
}

func TestPushTags_ReportsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":"validationError","message":"Invalid property \"staticId\""}}`))
	}))
	defer srv.Close()

	c := &Client{token: "token-1", http: &http.Client{Timeout: time.Minute}, pushTagsURL: srv.URL}
	_, err := c.PushTags(context.Background(), tagRequest())
	if err == nil {
		t.Fatal("PushTags() error = nil, want the server's message")
	}
	if !strings.Contains(err.Error(), "staticId") {
		t.Fatalf("error = %v, want it to quote the server response", err)
	}
}
