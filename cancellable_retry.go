package gentle

import (
	"strings"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/cenkalti/backoff"
	"github.com/gigawattio/errorlib"
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

func (config CancellableRetryConfig) ActionNames() string {
	names := make([]string, len(config.Actions))
	for i, action := range config.Actions {
		names[i] = action.Name
	}
	actionNames := strings.Join(names, ", ")
	return actionNames
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
				log.Debugf("Starting action [%v/%v] %q", i+1, numActions, action.Name)
			}
		Retry:
			if err := action.Func(); err != nil {
				if strategy == nil {
					strategy = config.BackoffProvider()
				}
				waitDuration := strategy.NextBackOff()
				if config.Debug || !config.Quiet {
					log.Errorf("[%v/%v] %q: %s (next wait=%v)", i+1, numActions, action.Name, err, waitDuration)
				}
				select {
				case ack := <-cancelChan:
					if config.Debug || !config.Quiet {
						log.Infof("Cancelled during backoff at action [%v/%v] %q", i+1, numActions, action.Name)
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
					log.Infof("Cancelled after action [%v/%v] %q", i+1, numActions, action.Name)
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
		if config.Debug || !config.Quiet {
			log.Infof("Cancellation finished OK for [%v]", config.ActionNames())
		}
		return nil
	}
	return
}
