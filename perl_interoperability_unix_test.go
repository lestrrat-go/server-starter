package starter_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	starter "github.com/lestrrat-go/server-starter/v2"
	"github.com/lestrrat-go/server-starter/v2/internal/cli"
	"github.com/stretchr/testify/require"
)

const (
	interopRequiredEnv     = "SERVER_STARTER_INTEROP_REQUIRED"
	interopGoWorkerEnv     = "SERVER_STARTER_INTEROP_GO_WORKER"
	interopGoSupervisorEnv = "SERVER_STARTER_INTEROP_GO_SUPERVISOR"
	interopReportEnv       = "SERVER_STARTER_INTEROP_REPORT"
)

func TestPerlInteroperabilityPerlSupervisorRunsGoWorker(t *testing.T) {
	perlPath, startServerPath := requirePerlServerStarter(t)

	executable, err := os.Executable()
	require.NoError(t, err)

	reportPath := filepath.Join(t.TempDir(), "go-worker-address")
	cmd := exec.CommandContext(
		interopTestContext(t, 30*time.Second),
		perlPath,
		startServerPath,
		"--port=127.0.0.1:0",
		"--",
		executable,
		"-test.run=^TestPerlInteroperabilityGoWorkerProcess$",
	)
	cmd.Env = append(
		environmentWithout(
			interopGoWorkerEnv,
			interopReportEnv,
			starter.PortEnvName,
			starter.GenerationEnvName,
		),
		interopGoWorkerEnv+"=1",
		interopReportEnv+"="+reportPath,
	)
	process := startInteropProcess(t, cmd)

	address := readInteropReport(t, reportPath, process)
	requireHTTPResponse(t, address, "go worker\n")
	require.NoError(t, process.stop(), process.output.String())
}

func TestPerlInteroperabilityGoSupervisorRunsPerlWorker(t *testing.T) {
	perlPath, _ := requirePerlServerStarter(t)

	executable, err := os.Executable()
	require.NoError(t, err)

	dir := t.TempDir()
	reportPath := filepath.Join(dir, "perl-worker-address")
	workerPath := filepath.Join(dir, "worker.pl")
	require.NoError(t, os.WriteFile(workerPath, []byte(perlWorker), 0o600))

	cmd := exec.CommandContext(
		interopTestContext(t, 30*time.Second),
		executable,
		"-test.run=^TestPerlInteroperabilityGoSupervisorProcess$",
		"--",
		"--port=127.0.0.1:0",
		"--",
		perlPath,
		workerPath,
		reportPath,
	)
	cmd.Env = append(
		environmentWithout(
			interopGoSupervisorEnv,
			starter.PortEnvName,
			starter.GenerationEnvName,
		),
		interopGoSupervisorEnv+"=1",
	)
	process := startInteropProcess(t, cmd)

	address := readInteropReport(t, reportPath, process)
	requireHTTPResponse(t, address, "perl worker\n")
	require.NoError(t, process.stop(), process.output.String())
}

func TestPerlInteroperabilityGoWorkerProcess(t *testing.T) {
	if os.Getenv(interopGoWorkerEnv) != "1" {
		return
	}

	require.True(t, starter.IsUnderStartServer())
	listeners, err := starter.ListenAll()
	require.NoError(t, err)
	require.Len(t, listeners, 1)

	reportPath := os.Getenv(interopReportEnv)
	require.NotEmpty(t, reportPath)
	publishInteropReport(t, reportPath, listeners[0].Addr().String())

	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("go worker\n"))
		}),
	}
	require.NoError(t, server.Serve(listeners[0]))
}

func TestPerlInteroperabilityGoSupervisorProcess(t *testing.T) {
	if os.Getenv(interopGoSupervisorEnv) != "1" {
		return
	}

	separator := slices.Index(os.Args, "--")
	require.NotEqual(t, -1, separator)
	os.Args = append([]string{"start_server"}, os.Args[separator+1:]...)
	os.Exit(cli.Run())
}

type interopProcess struct {
	cmd    *exec.Cmd
	output *interopOutput
	wait   chan error
}

type interopOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (o *interopOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.Write(data)
}

func (o *interopOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}

func startInteropProcess(t *testing.T, cmd *exec.Cmd) *interopProcess {
	t.Helper()

	output := &interopOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	require.NoError(t, cmd.Start())

	process := &interopProcess{
		cmd:    cmd,
		output: output,
		wait:   make(chan error, 1),
	}
	go func() {
		process.wait <- cmd.Wait()
	}()
	t.Cleanup(func() {
		if process.wait != nil {
			_ = process.stop()
		}
	})
	return process
}

func (p *interopProcess) stop() error {
	select {
	case err := <-p.wait:
		p.wait = nil
		return err
	default:
	}

	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}

	select {
	case err := <-p.wait:
		p.wait = nil
		return err
	case <-time.After(10 * time.Second):
		_ = p.cmd.Process.Kill()
		err := <-p.wait
		p.wait = nil
		return fmt.Errorf("supervisor did not stop after SIGTERM: %w", err)
	}
}

func readInteropReport(t *testing.T, path string, process *interopProcess) string {
	t.Helper()

	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		data, err := os.ReadFile(path)
		if err == nil && len(data) != 0 {
			return strings.TrimSpace(string(data))
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			require.NoError(t, err)
		}

		select {
		case err := <-process.wait:
			process.wait = nil
			require.NoError(t, err, process.output.String())
			t.Fatal("supervisor exited before its worker published an address")
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for worker address: %s", process.output.String())
		}
	}
}

func requireHTTPResponse(t *testing.T, address string, want string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address, nil)
	require.NoError(t, err)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	body := &bytes.Buffer{}
	_, err = body.ReadFrom(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, want, body.String())
}

func publishInteropReport(t *testing.T, path string, address string) {
	t.Helper()

	tmpPath := path + ".tmp"
	require.NoError(t, os.WriteFile(tmpPath, []byte(address+"\n"), 0o600))
	require.NoError(t, os.Rename(tmpPath, path))
}

func requirePerlServerStarter(t *testing.T) (string, string) {
	t.Helper()

	perlPath, perlErr := exec.LookPath("perl")
	startServerPath, startServerErr := exec.LookPath("start_server")
	moduleErr := exec.CommandContext(
		interopTestContext(t, 5*time.Second),
		perlPath,
		"-MServer::Starter",
		"-e",
		"1",
	).Run()
	scriptErr := exec.CommandContext(
		interopTestContext(t, 5*time.Second),
		perlPath,
		startServerPath,
		"--version",
	).Run()
	if perlErr == nil && startServerErr == nil && moduleErr == nil && scriptErr == nil {
		return perlPath, startServerPath
	}

	message := "original Server::Starter is unavailable; install libserver-starter-perl"
	if os.Getenv(interopRequiredEnv) == "1" {
		t.Fatal(message)
	}
	t.Skip(message)
	return "", ""
}

// testing.T.Context was added in Go 1.24, while this module supports Go 1.23.
func interopTestContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func environmentWithout(keys ...string) []string {
	environment := os.Environ()
	return slices.DeleteFunc(environment, func(entry string) bool {
		for _, key := range keys {
			if strings.HasPrefix(entry, key+"=") {
				return true
			}
		}
		return false
	})
}

const perlWorker = `use strict;
use warnings;
use IO::Socket::INET;
use Server::Starter qw(server_ports);

my $report_path = shift @ARGV or die "report path is required\n";
my $ports = server_ports();
die "expected one inherited listener\n" unless keys(%{$ports}) == 1;
my ($name, $fd) = each %{$ports};

my $listener = IO::Socket::INET->new(Proto => 'tcp')
    or die "failed to create socket object: $!\n";
$listener->fdopen($fd, 'w')
    or die "failed to open inherited listener $name=$fd: $!\n";

my $address = $listener->sockhost . ':' . $listener->sockport;
my $tmp_path = "$report_path.tmp.$$";
open my $report, '>', $tmp_path or die "failed to open report: $!\n";
print {$report} "$address\n" or die "failed to write report: $!\n";
close $report or die "failed to close report: $!\n";
rename $tmp_path, $report_path or die "failed to publish report: $!\n";

my $connection = $listener->accept or die "failed to accept: $!\n";
while (my $line = <$connection>) {
    last if $line =~ /^\r?\n$/;
}
print {$connection} "HTTP/1.1 200 OK\r\n";
print {$connection} "Content-Length: 12\r\n";
print {$connection} "Connection: close\r\n\r\n";
print {$connection} "perl worker\n";
close $connection;
`
