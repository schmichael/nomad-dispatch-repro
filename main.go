// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/nomad/api"
)

type result struct {
	i   int
	p   string
	err error
}

func main() {
	// CLI Flags
	jobFile := "sleeper.nomad.hcl"
	flag.StringVar(&jobFile, "job", jobFile, "name of parameterized job file")
	goroutines := 50
	flag.IntVar(&goroutines, "goroutines", goroutines, "number of concurrent goroutines")
	maxSleep := 3
	flag.IntVar(&maxSleep, "max-sleep", maxSleep, "max number of seconds each dispatch sleeps")
	loops := 10
	flag.IntVar(&loops, "n", loops, "number of dispatches per goroutine")

	flag.Parse()

	// Handle ctrl-c
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Register job
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating client: %s\n", err)
		os.Exit(1)
	}

	jobHCL, err := os.ReadFile(jobFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading jobspec: %s\n", err)
		os.Exit(1)
	}

	job, err := client.Jobs().ParseHCL(string(jobHCL), true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing hcl: %s\n", err)
		os.Exit(1)
	}

	_, _, err = client.Jobs().Register(job, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error registering job: %s\n", err)
		os.Exit(1)
	}

	// >>> Start the dispatching! <<<
	errCh := make(chan result, 100)

	wg := &sync.WaitGroup{}

	start := time.Now()
	wg.Add(goroutines)
	for i := 1; i < goroutines+1; i++ {
		p := strconv.Itoa(i % maxSleep) // limit max sleep
		go run(ctx, loops, p, errCh, wg.Done)
	}

	// Close errCh when all goroutines are complete
	go func() {
		wg.Wait()
		close(errCh)
	}()

	// Process results and exit
	oks, errs := 0, 0
	for r := range errCh {
		prefix := fmt.Sprintf("[%2s:%-4d] ", r.p, r.i)
		if r.err != nil {
			errs++
			log.Printf(prefix + r.err.Error())
		} else {
			oks++
			log.Printf(prefix + "ok")
		}
	}
	elapsed := time.Now().Sub(start).Round(time.Millisecond)
	log.Printf("%d done after %s with %d errors", oks+errs, elapsed, errs)
}

func run(ctx context.Context, loops int, param string, errCh chan result, done func()) {
	defer done()

	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		errCh <- result{
			p:   "<init>",
			err: err,
		}
		return
	}

	for i := 0; i < loops; i++ {
		_, err := backup(ctx, param, client)
		errCh <- result{
			i:   i,
			p:   param,
			err: err,
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// backup is as close as possible to the internal code
func backup(ctx context.Context, param string, client *api.Client) (any, error) {
	const (
		jobID        = "sleeper"
		taskName     = "sleeper"
		pollInterval = 3 * time.Second
	)
	parameters := map[string]string{
		"dur": param,
	}
	q := api.WriteOptions{}
	job, _, err := client.Jobs().Dispatch(jobID, parameters, nil, "", q.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to dispatch nomad job: %v", err.Error())
	}

	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
			q := &api.QueryOptions{}
			allocs, _, err := client.Jobs().Allocations(job.DispatchedJobID, true, q.WithContext(ctx))
			if err != nil {
				return nil, fmt.Errorf("unexpected error fetching allocation from dispatch ID %s: %v", job.DispatchedJobID, err.Error())
			}
			if len(allocs) == 0 {
				continue
			}
			// only one allocation because task is dispatched as a single container - there should not be
			// any other copy of the task for this specific job.
			// -- there are multiple ways to validate states of the allocations/tasks.
			// -- choosing to drill down into taskStates rather than relying on allocs[0].ClientStatus
			// -- because we'll need other information like allocs[0].Failed to determine exit code
			// -- and allocs[0].Events to provide context in case of errors
			state, ok := allocs[0].TaskStates[taskName]
			if !ok {
				out, _ := os.Create(fmt.Sprintf("%s.alloc.json", allocs[0].ID))
				e := json.NewEncoder(out)
				e.SetIndent("", "  ")
				e.Encode(allocs)
				out.Close()
				return nil, fmt.Errorf("expected %q task but none is found for dispatch ID %s", taskName, job.DispatchedJobID)
			}
			if state.State != "dead" {
				continue
			}
			if state.Failed {
				var msg string
				for _, event := range state.Events {
					msg += "<" + event.DisplayMessage + "> "
				}
				return nil, fmt.Errorf("%s task failed %s: events: %s", taskName, job.DispatchedJobID, msg)
			}
			return struct{}{}, nil
		}
	}

}
