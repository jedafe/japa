// Command mcpbridge is a stdio→HTTP bridge for MCP hosts that only support
// local stdio servers (no remote/streamable-http transport). It reads
// newline-delimited JSON-RPC 2.0 messages from stdin, POSTs each to the
// JapaConnect MCP endpoint, and writes each JSON response to stdout.
// Notification responses (HTTP 202, empty body) produce no output.
//
// Configuration is env-only so the binary can be dropped into a host's
// MCP server config without flags:
//
//	JC_MCP_URL      — endpoint (default https://api.japa.to/api/v1/mcp)
//	JC_MCP_API_KEY  — sent as X-API-Key (preferred credential), or
//	JC_MCP_TOKEN    — sent as Authorization: Bearer (OAuth token or JWT)
//
// Diagnostics go to stderr only — stdout carries the protocol stream.
//
// Ref: japaconnect-engineering/sections/120-mcp-agent-distribution.md
// Ref: japaconnect-engineering/adr/ADR-116-mcp-server.md (stdio follow-up)
package main

import (
	"errors"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

const (
	defaultMCPURL = "https://api.japa.to/api/v1/mcp"
	// perRequestTimeout bounds a single upstream call. Tool executions can
	// be slow, so this is generous; the MCP host applies its own deadline.
	perRequestTimeout = 5 * time.Minute
	// maxMessageBytes bounds one stdin line — oversized input is dropped.
	maxMessageBytes = 4 << 20 // 4 MiB
)

func main() {
	if err := run(); err != nil {
		slog.Error("mcpbridge failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	url := os.Getenv("JC_MCP_URL")
	if url == "" {
		url = defaultMCPURL
	}

	apiKey := os.Getenv("JC_MCP_API_KEY")
	token := os.Getenv("JC_MCP_TOKEN")

	if apiKey == "" && token == "" {
		return errors.New("set JC_MCP_API_KEY or JC_MCP_TOKEN — no credential configured")
	}

	client := &http.Client{}

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush() //nolint:errcheck

	in := bufio.NewReaderSize(os.Stdin, 64<<10)

	for {
		line, err := in.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			relay(client, url, apiKey, token, bytes.TrimSpace(line), out)
		}

		if err != nil {
			return nil // EOF or read error — stdin closed, exit clean
		}
	}
}

// relay forwards one JSON-RPC message upstream and writes the response to
// stdout. Requests are relayed sequentially — stdout ordering must match
// stdin ordering for hosts that correlate by position as well as by id.
func relay(client *http.Client, url, apiKey, token string, msg []byte, out *bufio.Writer) {
	if len(msg) > maxMessageBytes {
		slog.Warn("mcpbridge: dropping oversized message", "bytes", len(msg))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), perRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(msg)) // #nosec G704 -- upstream URL is operator-configured, not request-derived
	if err != nil {
		slog.Error("mcpbridge: build request", "error", err)
		writeSyntheticError(msg, out, "bridge request build failed")

		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req) // #nosec G704 -- upstream URL is operator-configured, not request-derived
	if err != nil {
		slog.Error("mcpbridge: upstream call", "error", err)
		writeSyntheticError(msg, out, "upstream request failed")

		return
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMessageBytes))
	if err != nil {
		slog.Error("mcpbridge: read upstream body", "error", err)
		writeSyntheticError(msg, out, "upstream response unreadable")

		return
	}

	// Notifications get a 202 with no body — emit nothing.
	if len(bytes.TrimSpace(body)) == 0 {
		return
	}

	// Any non-2xx body is the platform error envelope or an HTML error
	// page — neither is valid JSON-RPC, so surface a synthesized error
	// instead of corrupting the protocol stream.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Error("mcpbridge: upstream status", "status", resp.Status)
		writeSyntheticError(msg, out, fmt.Sprintf("upstream HTTP %d", resp.StatusCode))

		return
	}

	var probe struct {
		JSONRPC string `json:"jsonrpc"`
	}

	if json.Unmarshal(body, &probe) != nil || probe.JSONRPC != "2.0" {
		slog.Error("mcpbridge: upstream body is not JSON-RPC")
		writeSyntheticError(msg, out, "upstream returned a non-JSON-RPC response")

		return
	}

	// Pass the upstream response through verbatim — it is a valid
	// JSON-RPC envelope the host understands.
	if _, err := out.Write(append(bytes.TrimRight(body, "\r\n"), '\n')); err != nil {
		slog.Error("mcpbridge: write stdout", "error", err)
		return
	}

	if err := out.Flush(); err != nil {
		slog.Error("mcpbridge: flush stdout", "error", err)
	}
}

// writeSyntheticError emits a JSON-RPC error echoing the request's id so
// the host can correlate the failure instead of timing out silently. A
// message with no id member is a notification — the spec forbids answering
// those, so nothing is written.
func writeSyntheticError(msg []byte, out *bufio.Writer, message string) {
	var probe map[string]json.RawMessage
	if json.Unmarshal(msg, &probe) != nil {
		probe = nil
	}

	id, hasID := probe["id"]
	if !hasID {
		return
	}

	if len(id) == 0 {
		id = json.RawMessage("null")
	}

	resp, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": -32603, "message": message},
	})
	if _, err := out.Write(append(resp, '\n')); err != nil {
		slog.Error("mcpbridge: write synthetic error", "error", err)
		return
	}

	_ = out.Flush()
}
