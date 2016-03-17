package gentle

import (
	"sync"
	"time"

	"gigawatt-common/pkg/errorlib"

	"github.com/cenkalti/backoff"
)

type Action struct {
	Name string
	Func func() error
}

type CancellableRetryConfig struct {
	Actions         []Action
	DoneChan        chan struct{}
	BackoffProvider func() backoff.BackOff
	Debug           bool
	Quiet           bool
}

func CancellableRetry(config CancellableRetryConfig) (cancelFunc func() error) {
	var (
		cancelChan = make(chan chan error)
		done       bool
		lock       sync.Mutex
	)

	go func() {
		defer func() {
			if config.DoneChan != nil {
				select {
				case config.DoneChan <- struct{}{}:
				default:
				}
			}

			select {
			case ack := <-cancelChan:
				ack <- errorlib.NotRunningError
			default:
			}

			lock.Lock()
			done = true
			lock.Unlock()

			select {
			case ack := <-cancelChan:
				ack <- errorlib.NotRunningError
			default:
			}
		}()

		if config.BackoffProvider == nil {
			config.BackoffProvider = func() backoff.BackOff {
				// TODO: Consider using elapsed time before failure as part of the equation.
				strategy := backoff.NewExponentialBackOff()
				strategy.InitialInterval = 10 * time.Millisecond
				strategy.MaxElapsedTime = 0
				strategy.MaxInterval = 1 * time.Second
				strategy.Multiplier = 1.5
				return strategy
			}
		}

		var (
			numActions = len(config.Actions)
			strategy   backoff.BackOff
		)

		for i, action := range config.Actions {
			if config.Debug {
				log.Debug("Starting action [%v/%v] %q", i+1, numActions, action.Name)
			}
		Retry:
			if err := action.Func(); err != nil {
				if strategy == nil {
					strategy = config.BackoffProvider()
				}
				waitDuration := strategy.NextBackOff()
				if config.Debug || !config.Quiet {
					log.Error("[%v/%v] %q: %s (next wait=%v)", i+1, numActions, action.Name, err, waitDuration)
				}
				select {
				case ack := <-cancelChan:
					if config.Debug || !config.Quiet {
						log.Info("Cancelled during backoff at action [%v/%v] %q", i+1, numActions, action.Name)
					}
					ack <- nil
					return
				case <-time.After(waitDuration):
					goto Retry
				}
			}
			select {
			case ack := <-cancelChan:
				if config.Debug || !config.Quiet {
					log.Info("Cancelled after action [%v/%v] %q", i+1, numActions, action.Name)
				}
				ack <- nil
				return
			default:
			}
			strategy = nil
		}
	}()

	cancelFunc = func() error {
		lock.Lock()
		if done {
			lock.Unlock()
			return errorlib.NotRunningError
		}
		lock.Unlock()

		ack := make(chan error)
		cancelChan <- ack
		if err := <-ack; err != nil {
			return err
		}
		return nil
	}
	return
}
