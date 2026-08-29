package cli

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDaemonStatusFramingPreservesLargeFailures(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 70_000)
	var frame bytes.Buffer
	require.NoError(t, writeDaemonStatus(&frame, daemonFailed, body))

	err := readDaemonStatus(&frame)
	require.EqualError(t, err, "daemon startup failed: "+string(body))
}

func TestDaemonEnvironmentReplacesReadinessVariables(t *testing.T) {
	t.Setenv(daemonizedEnv, "stale")
	t.Setenv(daemonReadinessEnv, "99")

	env := daemonEnvironment()
	values := make(map[string]string, len(env))
	for _, entry := range env {
		parts := bytes.SplitN([]byte(entry), []byte("="), 2)
		values[string(parts[0])] = string(parts[1])
	}
	require.Equal(t, "1", values[daemonizedEnv])
	require.Equal(t, "3", values[daemonReadinessEnv])
}

func TestDaemonStatusFramingRejectsIncompleteOrTrailingData(t *testing.T) {
	frame := daemonStatusFrame(t, daemonFailed, []byte("failed"))
	testCases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "empty", want: "daemon exited before reporting startup status"},
		{name: "truncated header", data: frame[:daemonStatusHeaderSize-1], want: "read daemon startup status: unexpected EOF"},
		{name: "truncated body", data: frame[:len(frame)-1], want: "read daemon startup status: unexpected EOF"},
		{name: "trailing data", data: append(frame, 1), want: "invalid daemon startup response"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, readDaemonStatus(bytes.NewReader(tc.data)), tc.want)
		})
	}
}

func TestDaemonStatusFramingBoundsFailureAllocation(t *testing.T) {
	frame := make([]byte, daemonStatusHeaderSize)
	frame[0] = daemonFailed
	binary.BigEndian.PutUint64(frame[1:], uint64(maxDaemonStatusBodySize)+1)
	require.EqualError(t, readDaemonStatus(bytes.NewReader(frame)), "daemon startup status length exceeds limit")
}

func TestDaemonStatusFramingWrapsReaderErrors(t *testing.T) {
	readErr := errors.New("read failed")
	require.ErrorIs(t, readDaemonStatus(errorReader{err: readErr}), readErr)
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

func TestWriteDaemonStatusHandlesShortWrites(t *testing.T) {
	writer := &shortWriter{limit: 3}
	require.NoError(t, writeDaemonStatus(writer, daemonFailed, []byte("failed")))
	require.EqualError(t, readDaemonStatus(bytes.NewReader(writer.Bytes())), "daemon startup failed: failed")
}

type shortWriter struct {
	bytes.Buffer
	limit int
}

func (w *shortWriter) Write(data []byte) (int, error) {
	if len(data) > w.limit {
		data = data[:w.limit]
	}
	return w.Buffer.Write(data)
}
