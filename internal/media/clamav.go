package media

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"
)

// ClamAVClient communicates with a ClamAV daemon over TCP using the
// INSTREAM protocol. The client connects per-scan and closes the
// connection afterward — ClamAV is not designed for persistent
// connections, and the per-scan connect pattern avoids stale socket
// issues during long-running worker processes.
//
// ClamAVClient implements the worker.MediaScanner interface, allowing the
// media_scan task handler to delegate virus scanning without importing the
// media package directly (avoids circular dependency).
type ClamAVClient struct {
	addr    string
	timeout time.Duration
}

// NewClamAVClient returns a client that connects to the ClamAV daemon at addr.
// addr should be "host:port" (default ClamAV port is 3310).
func NewClamAVClient(addr string) *ClamAVClient {
	return &ClamAVClient{
		addr:    addr,
		timeout: 30 * time.Second,
	}
}

// ScanResult represents the outcome of a ClamAV scan.
type ScanResult struct {
	Clean     bool
	VirusName string
}

// ErrClamAVUnavailable marks daemon-connectivity failures (dial refused,
// unreachable host) as opposed to scan-protocol errors, so callers can log
// the degraded path once at WARN instead of ERROR per file.
var ErrClamAVUnavailable = errors.New("clamav daemon unreachable")

// Scan sends the file data to ClamAV via the INSTREAM command and returns
// the scan result. The INSTREAM protocol sends the file in 2048-byte
// chunks, terminated by a zero-length chunk, then reads the response line.
//
// Response parsing:
//   - "stream: OK"           → clean
//   - "stream: {NAME} FOUND" → infected, VirusName = {NAME}
//   - "stream: ERROR ..."    → scan error, returned as an error
func (c *ClamAVClient) Scan(reader io.Reader) (ScanResult, error) {
	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return ScanResult{}, fmt.Errorf("clamav connect %s: %w: %w", c.addr, err, ErrClamAVUnavailable)
	}

	defer func() { _ = conn.Close() }()

	// Set overall deadline for the scan operation.
	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return ScanResult{}, fmt.Errorf("clamav set deadline: %w", err)
	}

	// Send INSTREAM command.
	if _, err := fmt.Fprint(conn, "zINSTREAM\r\n"); err != nil {
		return ScanResult{}, fmt.Errorf("clamav send INSTREAM: %w", err)
	}

	// Stream file data in chunks. ClamAV expects big-endian uint32 length
	// prefix followed by that many bytes, repeated, terminated by a
	// zero-length chunk.
	const chunkSize = 2048
	buf := make([]byte, chunkSize+4) // 4-byte length prefix + data

	for {
		n, readErr := io.ReadFull(reader, buf[4:4+chunkSize])
		if n > 0 {
			// Write big-endian uint32 length prefix.
			// #nosec G115 // value bounded by caller
			buf[0] = byte(n >> 24)
			// #nosec G115 // value bounded by caller
			buf[1] = byte(n >> 16)
			// #nosec G115 // value bounded by caller
			buf[2] = byte(n >> 8)
			// #nosec G115 // value bounded by caller
			buf[3] = byte(n)

			if _, err := conn.Write(buf[:4+n]); err != nil {
				return ScanResult{}, fmt.Errorf("clamav send chunk: %w", err)
			}
		}

		if readErr != nil {
			break // EOF or error — both terminate the stream
		}
	}

	// Send zero-length terminator.
	zeroBuf := [4]byte{0, 0, 0, 0}
	if _, err := conn.Write(zeroBuf[:]); err != nil {
		return ScanResult{}, fmt.Errorf("clamav send terminator: %w", err)
	}

	// Read the response line.
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return ScanResult{}, errors.New("clamav: no response from daemon")
	}

	line := strings.TrimSpace(scanner.Text())

	return parseClamAVResponse(line), nil
}

// parseClamAVResponse interprets a single-line ClamAV response.
func parseClamAVResponse(line string) ScanResult {
	if line == "stream: OK" {
		return ScanResult{Clean: true}
	}

	// "stream: {VirusName} FOUND"
	if strings.HasSuffix(line, " FOUND") {
		virus := strings.TrimSuffix(line, " FOUND")
		virus = strings.TrimPrefix(virus, "stream: ")
		slog.Warn("clamav: virus detected", "virus", virus)

		return ScanResult{Clean: false, VirusName: virus}
	}

	// "stream: ..." error or unexpected response — treat as scan failure.
	slog.Error("clamav: unexpected response", "response", line)

	return ScanResult{Clean: false, VirusName: "scan_error: " + line}
}
