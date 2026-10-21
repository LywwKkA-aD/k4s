package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// deps are the parts of a run that tests replace.
type deps struct {
	sample sampler
	pods   podLister // nil turns the pod check off
}

// result is what one run observed.
type result struct {
	window      windowStats
	output      outputStats // written by k4s during the window
	outputTotal outputStats // written from start to the end of the window
	pods        podCheck
	quit        string // how k4s ended, e.g. "exit 0 on q"
}

// child is k4s running on the pty.
type child struct {
	cmd  *exec.Cmd
	ptmx *os.File
	out  *outputCounter
	done chan struct{} // closed once cmd.Wait returns
	err  error         // cmd.Wait's result, set before done is closed
}

// measure starts k4s, waits out the warm-up, samples it over the window,
// then quits it.
func measure(ctx context.Context, cfg config, d deps) (result, error) {
	c, err := startChild(cfg)
	if err != nil {
		return result{}, err
	}
	res, err := c.observe(ctx, cfg, d)
	res.quit = c.stop(cfg.quitKeys, cfg.quitTimeout)
	c.close()
	if err != nil {
		return result{}, err
	}
	return res, nil
}

func startChild(cfg config) (*child, error) {
	cmd := exec.Command(cfg.bin, cfg.args...) //nolint:gosec // G204: the binary and its flags are the operator's own
	cmd.Env = childEnv(os.Environ(), cfg.term, cfg.kubeconfig, cfg.env)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cfg.cols, Rows: cfg.rows})
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", cfg.bin, err)
	}
	c := &child{cmd: cmd, ptmx: ptmx, out: newOutputCounter(), done: make(chan struct{})}
	go func() {
		c.err = cmd.Wait()
		close(c.done)
	}()
	go drain(ptmx, ptmx, c.out, newResponder(terminalQueries))
	return c, nil
}

func (c *child) observe(ctx context.Context, cfg config, d deps) (result, error) {
	if err := c.sleep(ctx, cfg.warmup, "warm-up"); err != nil {
		return result{}, err
	}
	var before podSet
	var beforeErr error
	if d.pods != nil {
		before, beforeErr = d.pods(ctx)
	}
	samples, outStart, outEnd, err := c.sampleWindow(ctx, cfg, d.sample)
	if err != nil {
		return result{}, err
	}
	stats, err := summarize(samples)
	if err != nil {
		return result{}, err
	}
	res := result{window: stats, output: outEnd.sub(outStart), outputTotal: outEnd}
	if d.pods != nil {
		after, afterErr := d.pods(ctx)
		res.pods = compareSnapshots(before, beforeErr, after, afterErr)
	}
	return res, nil
}

// sampleWindow samples every interval until duration has passed since the
// first sample, and snapshots the output counters at both ends.
func (c *child) sampleWindow(ctx context.Context, cfg config, s sampler) ([]sample, outputStats, outputStats, error) {
	first, err := c.sampleNow(ctx, s)
	if err != nil {
		return nil, outputStats{}, outputStats{}, err
	}
	outStart := c.out.snapshot()
	deadline := first.at.Add(cfg.duration)
	samples := []sample{first}
	for last := first; last.at.Before(deadline); {
		next := last.at.Add(cfg.interval)
		if next.After(deadline) {
			next = deadline
		}
		if err := c.sleep(ctx, time.Until(next), "window"); err != nil {
			return nil, outputStats{}, outputStats{}, err
		}
		if last, err = c.sampleNow(ctx, s); err != nil {
			return nil, outputStats{}, outputStats{}, err
		}
		samples = append(samples, last)
	}
	return samples, outStart, c.out.snapshot(), nil
}

func (c *child) sampleNow(ctx context.Context, s sampler) (sample, error) {
	if err := ctx.Err(); err != nil {
		return sample{}, fmt.Errorf("measure: %w", err)
	}
	got, err := s(ctx, c.cmd.Process.Pid)
	if err != nil {
		return sample{}, fmt.Errorf("sample pid %d: %w", c.cmd.Process.Pid, err)
	}
	got.at = time.Now()
	return got, nil
}

// errExited reports that k4s ended before the run was over.
var errExited = errors.New("k4s exited")

// sleep waits for d unless k4s exits or ctx ends first; phase names the
// part of the run for the error.
func (c *child) sleep(ctx context.Context, d time.Duration, phase string) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", phase, ctx.Err())
	case <-c.done:
		return fmt.Errorf("%w during the %s: %s; last output: %q", errExited, phase, describeExit(c.err), c.out.lastOutput())
	}
}

// stop sends the quit keys, then SIGTERM, then SIGKILL, each after timeout,
// and says how k4s ended.
func (c *child) stop(keys string, timeout time.Duration) string {
	select {
	case <-c.done:
		return describeExit(c.err)
	default:
	}
	if _, err := io.WriteString(c.ptmx, keys); err == nil && c.exited(timeout) {
		return describeExit(c.err) + " on q"
	}
	if err := c.cmd.Process.Signal(syscall.SIGTERM); err == nil && c.exited(timeout) {
		return describeExit(c.err) + " after SIGTERM"
	}
	_ = c.cmd.Process.Kill()
	<-c.done
	return describeExit(c.err) + " after SIGKILL"
}

func (c *child) exited(within time.Duration) bool {
	t := time.NewTimer(within)
	defer t.Stop()
	select {
	case <-c.done:
		return true
	case <-t.C:
		return false
	}
}

func (c *child) close() {
	_ = c.ptmx.Close()
}

// describeExit words cmd.Wait's result: "exit 0", "exit status 3" or
// "signal: killed".
func describeExit(err error) string {
	if err == nil {
		return "exit 0"
	}
	return err.Error()
}
