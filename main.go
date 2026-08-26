// SnapStore — a tiny object-storage demo.
//
// It proves an object-storage binding actually works end to end: the
// /api/roundtrip endpoint writes a blob, reads it back, deletes it and
// confirms it is gone. A binding can be correctly injected and still be
// unusable, so reporting the variables alone would prove nothing.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

var release = envOr("LIFTOFF_COMMIT", "dev")

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "service": "snapstore", "release": release,
	})
}

// bindingReport lists which storage variables reached the container, without
// ever printing a secret's value.
func bindingReport(w http.ResponseWriter, _ *http.Request) {
	names := []string{
		"AZURE_STORAGE_ACCOUNT", "AZURE_STORAGE_CONTAINER", "AZURE_STORAGE_ENDPOINT",
		"AZURE_STORAGE_CONNECTION_STRING", "AZURE_STORAGE_KEY",
		"SPACES_BUCKET", "SPACES_ENDPOINT", "SPACES_REGION",
	}
	seen := map[string]string{}
	for _, n := range names {
		v := os.Getenv(n)
		switch {
		case v == "":
			seen[n] = "(unset)"
		case strings.Contains(n, "KEY") || strings.Contains(n, "CONNECTION_STRING"):
			seen[n] = fmt.Sprintf("(set, %d chars)", len(v))
		default:
			seen[n] = v
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"bindings": seen, "release": release})
}

func roundtrip(w http.ResponseWriter, r *http.Request) {
	conn := os.Getenv("AZURE_STORAGE_CONNECTION_STRING")
	container := envOr("AZURE_STORAGE_CONTAINER", "")
	if conn == "" || container == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":    false,
			"error": "AZURE_STORAGE_CONNECTION_STRING and AZURE_STORAGE_CONTAINER must both be bound",
		})
		return
	}

	client, err := azblob.NewClientFromConnectionString(conn, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "step": "client", "error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	name := fmt.Sprintf("snapstore-roundtrip-%d.txt", time.Now().UnixNano())
	payload := fmt.Sprintf("written by snapstore release %s at %s", release, time.Now().UTC().Format(time.RFC3339))
	steps := []string{}

	if _, err := client.UploadStream(ctx, container, name, strings.NewReader(payload), nil); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "step": "upload", "error": err.Error()})
		return
	}
	steps = append(steps, "upload")

	dl, err := client.DownloadStream(ctx, container, name, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "step": "download", "error": err.Error(), "steps": steps})
		return
	}
	buf := make([]byte, len(payload))
	n, _ := dl.Body.Read(buf)
	_ = dl.Body.Close()
	readBack := string(buf[:n])
	steps = append(steps, "download")

	if _, err := client.DeleteBlob(ctx, container, name, nil); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "step": "delete", "error": err.Error(), "steps": steps})
		return
	}
	steps = append(steps, "delete")

	// Confirm it is actually gone rather than trusting the delete's 202.
	gone := false
	if _, err := client.DownloadStream(ctx, container, name, &azblob.DownloadStreamOptions{}); err != nil {
		gone = true
	}
	steps = append(steps, "confirm-gone")

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        readBack == payload && gone,
		"blob":      name,
		"container": container,
		"matched":   readBack == payload,
		"gone":      gone,
		"steps":     steps,
		"release":   release,
	})
}

func list(w http.ResponseWriter, r *http.Request) {
	conn := os.Getenv("AZURE_STORAGE_CONNECTION_STRING")
	container := os.Getenv("AZURE_STORAGE_CONTAINER")
	if conn == "" || container == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "storage is not bound"})
		return
	}
	client, err := azblob.NewClientFromConnectionString(conn, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	pager := client.NewListBlobsFlatPager(container, &azblob.ListBlobsFlatOptions{})
	names := []string{}
	for pager.More() && len(names) < 100 {
		page, err := pager.NextPage(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		for _, item := range page.Segment.BlobItems {
			if item != nil && item.Name != nil {
				names = append(names, *item.Name)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"container": container, "blobs": names})
}

func index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>SnapStore</title>
<style>body{font-family:ui-sans-serif,system-ui,sans-serif;max-width:40rem;margin:4rem auto;
padding:0 1rem;line-height:1.6;color:#0f172a}code{background:#f1f5f9;padding:.15rem .4rem;
border-radius:.25rem}</style></head><body><h1>SnapStore</h1>
<p>A Go service that proves an object-storage binding really works.</p>
<ul><li><code>GET /api/bindings</code> — which storage variables arrived</li>
<li><code>GET /api/roundtrip</code> — write, read back, delete, confirm gone</li>
<li><code>GET /api/blobs</code> — list the container</li>
<li><code>GET /health</code></li></ul>
<p>Release <code>%s</code></p></body></html>`, release)
}

func main() {
	port := envOr("PORT", "3000")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/api/health", health)
	mux.HandleFunc("/api/bindings", bindingReport)
	mux.HandleFunc("/api/roundtrip", roundtrip)
	mux.HandleFunc("/api/blobs", list)
	mux.HandleFunc("/", index)

	log.Printf("snapstore listening on :%s (release %s)", port, release)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
