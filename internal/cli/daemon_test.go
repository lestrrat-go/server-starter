package cli

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDaemonStatusFraming(t *testing.T) {
	testCases := []struct {
		name   string
		status byte
		body   []byte
		want   string
	}{
		{name: "ready", status: daemonReady},
		{name: "empty failure", status: daemonFailed, want: "daemon startup failed"},
		{
			name:   "failure",
			status: daemonFailed,
			body:   []byte("listener setup failed"),
			want:   "daemon startup failed: listener setup failed",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var frame bytes.Buffer
			require.NoError(t, writeDaemonStatus(&frame, tc.status, tc.body))

			err := readDaemonStatus(&frame)
			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.want)
		})
	}
}

func TestWriteDaemonStatusHandlesPartialWrites(t *testing.T) {
	writer := &partialWriter{limit: 3}
	require.NoError(t, writeDaemonStatus(writer, daemonFailed, []byte("listener setup failed")))
	require.EqualError(
		t,
		readDaemonStatus(bytes.NewReader(writer.Bytes())),
		"daemon startup failed: listener setup failed",
	)
}

func TestReadDaemonStatusRejectsMalformedFrames(t *testing.T) {
	readyWithBody := daemonStatusFrame(t, daemonReady, []byte("unexpected"))
	readyWithTrailingData := append(daemonStatusFrame(t, daemonReady, nil), byte(1))
	truncatedFailure := daemonStatusFrame(t, daemonFailed, []byte("truncated"))
	truncatedFailure = truncatedFailure[:len(truncatedFailure)-1]
	failureWithTrailingData := append(daemonStatusFrame(t, daemonFailed, []byte("failed")), byte(1))

	testCases := []struct {
		name  string
		frame []byte
		want  string
	}{
		{name: "empty", want: "daemon exited before reporting startup status"},
		{name: "truncated header", frame: []byte{daemonReady}, want: "read daemon startup status: unexpected EOF"},
		{name: "ready body", frame: readyWithBody, want: "invalid daemon readiness response"},
		{name: "ready trailing data", frame: readyWithTrailingData, want: "invalid daemon readiness response"},
		{name: "truncated failure", frame: truncatedFailure, want: "read daemon startup status: unexpected EOF"},
		{name: "failure trailing data", frame: failureWithTrailingData, want: "invalid daemon startup response"},
		{name: "unknown status", frame: daemonStatusFrame(t, byte(99), nil), want: "invalid daemon startup response"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, readDaemonStatus(bytes.NewReader(tc.frame)), tc.want)
		})
	}
}

func TestReadDaemonStatusWrapsReaderError(t *testing.T) {
	readErr := errors.New("read failed")
	require.ErrorIs(t, readDaemonStatus(errorReader{err: readErr}), readErr)

	header := daemonStatusFrame(t, daemonFailed, nil)
	require.ErrorIs(
		t,
		readDaemonStatus(io.MultiReader(bytes.NewReader(header[:daemonStatusHeaderSize]), errorReader{err: readErr})),
		readErr,
	)
}

func daemonStatusFrame(t *testing.T, status byte, body []byte) []byte {
	t.Helper()
	var frame bytes.Buffer
	require.NoError(t, writeDaemonStatus(&frame, status, body))
	return frame.Bytes()
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

type partialWriter struct {
	bytes.Buffer
	limit int
}

func (w *partialWriter) Write(data []byte) (int, error) {
	if len(data) > w.limit {
		data = data[:w.limit]
	}
	return w.Buffer.Write(data)
}
