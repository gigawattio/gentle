package gentle_test

import (
	"errors"
	"testing"
	"time"

	"github.com/gigawattio/errorlib"
	"github.com/gigawattio/gentle"
)

func TestCancellableRetry(t *testing.T) {
	testCases := []struct {
		Action       gentle.Action
		InvokeCancel bool
	}{
		{
			Action: gentle.Action{
				Name: "Will never work",
				Func: func() error {
					return errors.New("told ya")
				},
			},
			InvokeCancel: true,
		},
		{
			Action: gentle.Action{
				Name: "Will eventually work",
				Func: func() error {
					time.Sleep(1 * time.Millisecond)
					return nil
				},
			},
			InvokeCancel: true,
		},
		{
			Action: gentle.Action{
				Name: "Will eventually work",
				Func: func() error {
					return nil
				},
			},
			InvokeCancel: false,
		},
	}
	for i, testCase := range testCases {
		doneChan := make(chan struct{}, 1)
		cancelFunc := gentle.CancellableRetry(gentle.CancellableRetryConfig{
			Actions: []gentle.Action{
				testCase.Action,
			},
			DoneChan: doneChan,
			Debug:    true,
		})
		if testCase.InvokeCancel {
			if err := cancelFunc(); err != nil {
				t.Fatalf("[i=%v/testCase=%+v] %s", i, testCase, err)
			}
		} else {
			<-doneChan
		}
		if expected, actual := errorlib.NotRunningError, cancelFunc(); actual != expected {
			t.Fatalf("[i=%v/testCase=%+v] Expected late invocation of cancelFunc() to produce err=%v but actual=%v", i, testCase, expected, actual)
		}
	}
}
