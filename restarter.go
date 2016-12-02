package starter

import (
	"bufio"
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

func NewRestarter(pidFile, statusFile string) *Restarter {
	return &Restarter{
		pidFile:    pidFile,
		statusFile: statusFile,
	}
}

func (s *Restarter) Run(ctx context.Context) error {
	pidbuf, err := ioutil.ReadFile(s.pidFile)
	if err != nil {
		return errors.Wrapf(err, "failed to open file:%s", s.pidFile)
	}
	pid, err := strconv.ParseInt(string(pidbuf), 10, 64)
	if err != nil {
		return errors.Wrap(err, "failed to parse pid")
	}

	p, err := os.FindProcess(int(pid))
	if err != nil {
		return errors.Wrapf(err, "failed to find process:%s", pidbuf)
	}

	if err := p.Signal(syscall.SIGHUP); err != nil {
		return errors.Wrap(err, "failed to send SIGHUP to the server process")
	}

	getGenerations := func(file string) ([]int, error) {
		genbuf, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open file:%s", file)
		}
		scanner := bufio.NewScanner(bytes.NewReader(genbuf))
		genmap := make(map[int]struct{})
		for scanner.Scan() {
			txt := scanner.Text()
			i := strings.IndexByte(txt, ':')
			if i <= 0 {
				continue
			}
			gen, err := strconv.ParseInt(string(txt[:i]), 10, 64)
			if err != nil {
				continue
			}

			genmap[int(gen)] = struct{}{}
		}

		var generations []int
		for k := range genmap {
			generations = append(generations, k)
		}
		sort.Ints(generations)
		return generations, nil
	}

	var waitFor int
	{
		generations, err := getGenerations(s.statusFile)
		if err != nil {
			return errors.Wrap(err, "failed to find generations")
		}

		if len(generations) == 0 {
			return errors.New("no active process found in the status file")
		}

		waitFor = generations[len(generations)-1] + 1
	}

	t := time.NewTicker(time.Second)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			generations, err := getGenerations(s.statusFile)
			if err != nil {
				return errors.Wrap(err, "failed to find generations")
			}
			if len(generations) == 1 && generations[0] == waitFor {
				return nil
			}
		}
	}
}
