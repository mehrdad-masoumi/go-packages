package notificationclient

import (
	"context"
	"sync"
)

// Fake is an in-memory Client for use in callers' unit tests. It records
// every command it receives (for assertions) and returns configurable
// responses/errors, so tests don't need a real HTTP server.
//
// Fake is safe for concurrent use.
type Fake struct {
	mu sync.Mutex

	sentCommands       []Command
	sentDirectCommands []DirectCommand

	// SendResponse/SendErr are returned by Send when SendFunc is nil.
	SendResponse AcceptedResponse
	SendErr      error

	// SendDirectResponse/SendDirectErr are returned by SendDirect when
	// SendDirectFunc is nil.
	SendDirectResponse AcceptedResponse
	SendDirectErr      error

	// SendFunc, when set, takes precedence over SendResponse/SendErr and
	// lets tests script per-call behavior (e.g. fail on the Nth call).
	SendFunc func(ctx context.Context, command Command) (AcceptedResponse, error)

	// SendDirectFunc, when set, takes precedence over
	// SendDirectResponse/SendDirectErr.
	SendDirectFunc func(ctx context.Context, command DirectCommand) (AcceptedResponse, error)
}

var _ Client = (*Fake)(nil)

// NewFake returns a ready-to-use Fake that accepts every command.
func NewFake() *Fake {
	return &Fake{}
}

// Send records command and returns the configured response/error.
func (f *Fake) Send(ctx context.Context, command Command) (AcceptedResponse, error) {
	f.mu.Lock()
	f.sentCommands = append(f.sentCommands, command)
	fn := f.SendFunc
	resp, err := f.SendResponse, f.SendErr
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, command)
	}
	return resp, err
}

// SendDirect records command and returns the configured response/error.
func (f *Fake) SendDirect(ctx context.Context, command DirectCommand) (AcceptedResponse, error) {
	f.mu.Lock()
	f.sentDirectCommands = append(f.sentDirectCommands, command)
	fn := f.SendDirectFunc
	resp, err := f.SendDirectResponse, f.SendDirectErr
	f.mu.Unlock()

	if fn != nil {
		return fn(ctx, command)
	}
	return resp, err
}

// Commands returns a copy of every Command passed to Send, in order.
func (f *Fake) Commands() []Command {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Command, len(f.sentCommands))
	copy(out, f.sentCommands)
	return out
}

// DirectCommands returns a copy of every DirectCommand passed to
// SendDirect, in order.
func (f *Fake) DirectCommands() []DirectCommand {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]DirectCommand, len(f.sentDirectCommands))
	copy(out, f.sentDirectCommands)
	return out
}

// Reset clears all recorded commands.
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sentCommands = nil
	f.sentDirectCommands = nil
}
