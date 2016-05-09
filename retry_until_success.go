package gentle

import (
	"time"

	"github.com/cenkalti/backoff"
)

// retryUntilSuccess will keep attempting an operation until it succeeds.
//
// Exponential backoff is used to prevent failing attempts from looping madly.
func RetryUntilSuccess(name string, operation func() error, strategy backoff.BackOff) {
	errNotifReceiver := func(err error, nextWait time.Duration) {
		log.ExtraCalldepth = 1
		log.Error("%v notified of error: %s [next wait=%s]", name, err, nextWait)
		log.ExtraCalldepth = 0
	}
	for {
		if err := backoff.RetryNotify(operation, strategy, errNotifReceiver); err != nil {
			log.ExtraCalldepth = 1
			log.Error("%v failure: %s [will keep trying]", name, err)
			log.ExtraCalldepth = 0
			strategy.Reset()
			continue
		}
		break
	}
}
