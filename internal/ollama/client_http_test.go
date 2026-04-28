package ollama

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient_DefaultBaseURL(t *testing.T) {
	t.Parallel()
	c := NewClient("")
	if c.baseURL != defaultBaseURL {
		t.Fatalf("baseURL=%q want %q", c.baseURL, defaultBaseURL)
	}
	if c.httpClient == nil {
		t.Fatal("httpClient should be initialized")
	}
	if c.httpClient.Timeout != 0 {
		t.Fatalf("default http timeout=%v want 0 (no global timeout)", c.httpClient.Timeout)
	}
}

func TestListModels_Success(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"m1","modified_at":"now","size":1}]}`))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0].Name != "m1" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestListModels_HTTPError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListModels_DecodeError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not-json}"))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestModelContextLength_Success(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model_info":{"gemma4.context_length":262144}}`))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	ctxLen, err := c.ModelContextLength(context.Background(), "gemma4:26b")
	if err != nil {
		t.Fatalf("ModelContextLength: %v", err)
	}
	if ctxLen != 262144 {
		t.Fatalf("context_length=%d want 262144", ctxLen)
	}
}

func TestModelContextLength_StatusAndMissing(t *testing.T) {
	t.Parallel()
	t.Run("status error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad", http.StatusBadRequest)
		}))
		defer ts.Close()

		c := NewClient(ts.URL)
		_, err := c.ModelContextLength(context.Background(), "m")
		if err == nil || !strings.Contains(err.Error(), fmt.Sprint(http.StatusBadRequest)) {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("missing context", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model_info":{"foo":1}}`))
		}))
		defer ts.Close()

		c := NewClient(ts.URL)
		_, err := c.ModelContextLength(context.Background(), "m")
		if err == nil {
			t.Fatal("expected missing-context error")
		}
	})
}

func TestChatStream_Success(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"hi\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"\"},\"done\":true}\n"))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	var seen int
	err := c.ChatStream(context.Background(), ChatRequest{Model: "m", Messages: []Message{{Role: RoleUser, Content: "x"}}}, func(resp ChatResponse) error {
		seen++
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if seen != 2 {
		t.Fatalf("seen=%d want 2", seen)
	}
}

func TestChatStream_BadChunk(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{not-json}\n"))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	err := c.ChatStream(context.Background(), ChatRequest{Model: "m"}, func(resp ChatResponse) error { return nil })
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestChatStream_CallbackError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"model\":\"m\",\"message\":{\"role\":\"assistant\",\"content\":\"hi\"},\"done\":false}\n"))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	want := errors.New("stop")
	err := c.ChatStream(context.Background(), ChatRequest{Model: "m"}, func(resp ChatResponse) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want=%v", err, want)
	}
}

func TestChatStream_StatusError(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	err := c.ChatStream(context.Background(), ChatRequest{Model: "m"}, func(resp ChatResponse) error { return nil })
	if err == nil {
		t.Fatal("expected status error")
	}
}

func TestChat_SuccessAndStatusError(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"m","message":{"role":"assistant","content":"ok"},"done":true}`))
		}))
		defer ts.Close()

		c := NewClient(ts.URL)
		resp, err := c.Chat(context.Background(), ChatRequest{Model: "m"})
		if err != nil {
			t.Fatalf("Chat: %v", err)
		}
		if resp.Message.Content != "ok" {
			t.Fatalf("content=%q", resp.Message.Content)
		}
	})

	t.Run("status error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "bad", http.StatusBadRequest)
		}))
		defer ts.Close()

		c := NewClient(ts.URL)
		_, err := c.Chat(context.Background(), ChatRequest{Model: "m"})
		if err == nil || !strings.Contains(err.Error(), fmt.Sprint(http.StatusBadRequest)) {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("decode error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{not-json}"))
		}))
		defer ts.Close()

		c := NewClient(ts.URL)
		_, err := c.Chat(context.Background(), ChatRequest{Model: "m"})
		if err == nil {
			t.Fatal("expected decode error")
		}
	})
}
