package authassist

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
)

const maxOperatorSignalBytes = 8

// LineOperator turns empty terminal lines into control signals. Non-empty
// input is rejected without echoing it, so credentials and MFA responses stay
// in the headed browser rather than becoming command input.
type LineOperator struct {
	mu      sync.Mutex
	scanner *bufio.Scanner
	out     io.Writer
}

// NewLineOperator creates the terminal control adapter.
func NewLineOperator(input io.Reader, output io.Writer) *LineOperator {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, maxOperatorSignalBytes), maxOperatorSignalBytes)
	return &LineOperator{scanner: scanner, out: output}
}

// Await asks the operator to complete an authored interaction in the browser
// and accepts only an empty line as the completion signal.
func (o *LineOperator) Await(ctx context.Context, instruction Instruction) error {
	if o == nil || o.scanner == nil || o.out == nil {
		return fmt.Errorf("operator control channel is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, err := fmt.Fprintf(o.out, "auth-assist: %s (%s): complete this step directly in the headed browser; type no credentials here, then press Enter only\n", instruction.Path, instruction.Kind); err != nil {
		return err
	}
	type scanResult struct {
		text string
		ok   bool
		err  error
	}
	ready := make(chan scanResult, 1)
	go func() {
		if o.scanner.Scan() {
			ready <- scanResult{text: o.scanner.Text(), ok: true}
			return
		}
		ready <- scanResult{err: o.scanner.Err()}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-ready:
		if result.err != nil {
			return fmt.Errorf("read operator control signal: %w", result.err)
		}
		if !result.ok {
			return fmt.Errorf("operator control channel ended before an empty-line signal")
		}
		if strings.TrimSuffix(result.text, "\r") != "" {
			return fmt.Errorf("operator control signal must be an empty line; credential and MFA values belong only in the browser")
		}
		return nil
	}
}
