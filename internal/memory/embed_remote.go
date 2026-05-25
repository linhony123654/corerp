package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

// RemoteEmbedder calls a local Python embedding server (bge-small-zh).
// Falls back to local 2-gram if the server is unavailable.
type RemoteEmbedder struct {
	serverURL   string
	client      *http.Client
	mu          sync.RWMutex
	available   bool
	lastCheck   time.Time
	dim         int
}

const (
	defaultEmbedPort = "8765"
	embedTimeout     = 5 * time.Second
)

func NewRemoteEmbedder() *RemoteEmbedder {
	return &RemoteEmbedder{
		serverURL: "http://127.0.0.1:" + defaultEmbedPort,
		client:    &http.Client{Timeout: embedTimeout},
		dim:       512, // bge-small-zh-v1.5 dimension
	}
}

// StartEmbedServer launches the Python embedding server as a subprocess.
func StartEmbedServer() (*exec.Cmd, error) {
	cmd := exec.Command("python3", "embed_server.py")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("embed server start: %w", err)
	}
	// Wait for server to be ready (model loading takes ~10s first time)
	time.Sleep(5 * time.Second)
	return cmd, nil
}

// Available checks if the remote server is reachable.
func (r *RemoteEmbedder) Available() bool {
	r.mu.RLock()
	if r.available && time.Since(r.lastCheck) < 30*time.Second {
		r.mu.RUnlock()
		return true
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	resp, err := r.client.Get(r.serverURL + "/health")
	if err != nil {
		r.available = false
		return false
	}
	resp.Body.Close()
	r.available = resp.StatusCode == 200
	r.lastCheck = time.Now()
	return r.available
}

// Embed sends a single text to the remote server and returns its vector.
func (r *RemoteEmbedder) Embed(text string) ([]float64, error) {
	batch, err := r.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(batch) == 0 {
		return nil, fmt.Errorf("empty embedding result")
	}
	return batch[0], nil
}

// EmbedBatch sends multiple texts and returns their vectors.
func (r *RemoteEmbedder) EmbedBatch(texts []string) ([][]float64, error) {
	body, _ := json.Marshal(map[string]interface{}{"texts": texts})
	resp, err := r.client.Post(
		r.serverURL+"/embed",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embed server %d: %s", resp.StatusCode, string(msg))
	}

	var result struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embeddings, nil
}

// Dim returns the embedding dimension.
func (r *RemoteEmbedder) Dim() int {
	return r.dim
}
