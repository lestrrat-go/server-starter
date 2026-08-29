//go:build !windows && !darwin && !linux

package supervisor

var platformWorkerStartErrorPolicy workerStartErrorPolicy
