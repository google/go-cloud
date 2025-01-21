// Copyright 2018 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package pubsub provides an easy and portable way to interact with
// publish/subscribe systems. Subpackages contain driver implementations of
// pubsub for supported services
//
// See https://gocloud.dev/howto/pubsub/ for a detailed how-to guide.
//
// # At-most-once and At-least-once Delivery
//
// The semantics of message delivery vary across PubSub services.
// Some services guarantee that messages received by subscribers but not
// acknowledged are delivered again (at-least-once semantics). In others,
// a message will be delivered only once, if it is delivered at all
// (at-most-once semantics). Some services support both modes via options.
//
// This package accommodates both kinds of systems, but application developers
// should think carefully about which kind of semantics the application needs.
// Even though the application code may look similar, system-level
// characteristics are quite different. See the driver package
// documentation for more information about message delivery semantics.
//
// After receiving a Message via Subscription.Receive:
//   - Always call Message.Ack or Message.Nack after processing the message.
//   - For some drivers, Ack will be a no-op.
//   - For some drivers, Nack is not supported and will panic; you can call
//     Message.Nackable to see.
//
// # OpenCensus Integration
//
// OpenCensus supports tracing and metric collection for multiple languages and
// backend providers. See https://opencensus.io.
//
// This API collects OpenCensus traces and metrics for the following methods:
//   - Topic.Send
//   - Topic.Shutdown
//   - Subscription.Receive
//   - Subscription.Shutdown
//   - The internal driver methods SendBatch, SendAcks and ReceiveBatch.
//
// All trace and metric names begin with the package import path.
// The traces add the method name.
// For example, "gocloud.dev/pubsub/Topic.Send".
// The metrics are "completed_calls", a count of completed method calls by driver,
// method and status (error code); and "latency", a distribution of method latency
// by driver and method.
// For example, "gocloud.dev/pubsub/latency".
//
// To enable trace collection in your application, see "Configure Exporter" at
// https://opencensus.io/quickstart/go/tracing.
// To enable metric collection in your application, see "Exporting stats" at
// https://opencensus.io/quickstart/go/metrics.
package pubsub // import "gocloud.dev/pubsub"

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"reflect"
	"runtime"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/googleapis/gax-go/v2"
	"gocloud.dev/gcerrors"
	"gocloud.dev/internal/gcerr"
	"gocloud.dev/internal/oc"
	"gocloud.dev/internal/openurl"
	"gocloud.dev/internal/retry"
	"gocloud.dev/pubsub/batcher"
	"gocloud.dev/pubsub/driver"
	"golang.org/x/sync/errgroup"
)

// Message contains data to be published.
type Message struct {
	// LoggableID will be set to an opaque message identifer for
	// received messages, useful for debug logging. No assumptions should
	// be made about the content.
	LoggableID string

	// Body contains the content of the message.
	Body []byte

	// Metadata has key/value metadata for the message.
	//
	// When sending a message, set any key/value pairs you want associated with
	// the message. It is acceptable for Metadata to be nil.
	// Note that some services limit the number of key/value pairs per message.
	//
	// When receiving a message, Metadata will be nil if the message has no
	// associated metadata.
	Metadata map[string]string

	// BeforeSend is a callback used when sending a message. It will always be
	// set to nil for received messages.
	//
	// The callback will be called exactly once, before the message is sent.
	//
	// asFunc converts its argument to driver-specific types.
	// See https://gocloud.dev/concepts/as/ for background information.
	BeforeSend func(asFunc func(any) bool) error

	// AfterSend is a callback used when sending a message. It will always be
	// set to nil for received messages.
	//
	// The callback will be called at most once, after the message is sent.
	// If Send returns an error, AfterSend will not be called.
	//
	// asFunc converts its argument to driver-specific types.
	// See https://gocloud.dev/concepts/as/ for background information.
	AfterSend func(asFunc func(any) bool) error

	// asFunc invokes driver.Message.AsFunc.
	asFunc func(any) bool

	// ack is a closure that queues this message for the action (ack or nack).
	ack func(isAck bool)

	// nackable is true iff Nack can be called without panicking.
	nackable bool

	// mu guards isAcked in case Ack/Nack is called concurrently.
	mu sync.Mutex

	// isAcked tells whether this message has already had its Ack or Nack
	// method called.
	isAcked bool
}

// Ack acknowledges the message, telling the server that it does not need to be
// sent again to the associated Subscription. It will be a no-op for some
// drivers; see
// https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more info.
//
// Ack returns immediately, but the actual ack is sent in the background, and
// is not guaranteed to succeed. If background acks persistently fail, the error
// will be returned from a subsequent Receive.
func (m *Message) Ack() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isAcked {
		panic(fmt.Sprintf("Ack/Nack called twice on message: %+v", m))
	}
	m.ack(true)
	m.isAcked = true
}

// Nackable returns true iff Nack can be called without panicking.
//
// Some services do not support Nack; for example, at-most-once services
// can't redeliver a message. See
// https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// for more info.
func (m *Message) Nackable() bool {
	return m.nackable
}

// Nack (short for negative acknowledgment) tells the server that this Message
// was not processed and should be redelivered.
//
// Nack panics for some drivers, as Nack is meaningless when messages can't be
// redelivered. You can call Nackable to determine if Nack is available. See
// https://godoc.org/gocloud.dev/pubsub#hdr-At_most_once_and_At_least_once_Delivery
// fore more info.
//
// Nack returns immediately, but the actual nack is sent in the background,
// and is not guaranteed to succeed.
//
// Nack is a performance optimization for retrying transient failures. It
// must not be used for message parse errors or other messages that the
// application will never be able to process: calling Nack will cause them to
// be redelivered and overload the server. Instead, an application should call
// Ack and log the failure in some monitored way.
func (m *Message) Nack() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isAcked {
		panic(fmt.Sprintf("Ack/Nack called twice on message: %+v", m))
	}
	if !m.nackable {
		panic("Message.Nack is not supported by this driver")
	}
	m.ack(false)
	m.isAcked = true
}

// As converts i to driver-specific types.
// See https://gocloud.dev/concepts/as/ for background information, the "As"
// examples in this package for examples, and the driver package
// documentation for the specific types supported for that driver.
// As panics unless it is called on a message obtained from Subscription.Receive.
func (m *Message) As(i any) bool {
	if m.asFunc == nil {
		panic("As called on a Message that was not obtained from Receive")
	}
	return m.asFunc(i)
}

// Topic publishes messages to all its subscribers.
type Topic struct {
	driver  driver.Topic
	batcher *batcher.Batcher
	tracer  *oc.Tracer
	mu      sync.Mutex
	err     error

	// cancel cancels all SendBatch calls.
	cancel func()
}

// Send publishes a message. It only returns after the message has been
// sent, or failed to be sent. Send can be called from multiple goroutines
// at once.
func (t *Topic) Send(ctx context.Context, m *Message) (err error) {
	ctx = t.tracer.Start(ctx, "Topic.Send")
	defer func() { t.tracer.End(ctx, err) }()

	// Check for doneness before we do any work.
	if err := ctx.Err(); err != nil {
		return err // Return context errors unwrapped.
	}
	t.mu.Lock()
	err = t.err
	t.mu.Unlock()
	if err != nil {
		return err // t.err wrapped when set
	}
	if m.LoggableID != "" {
		return gcerr.Newf(gcerr.InvalidArgument, nil, "pubsub: Message.LoggableID should not be set when sending a message")
	}
	for k, v := range m.Metadata {
		if !utf8.ValidString(k) {
			return gcerr.Newf(gcerr.InvalidArgument, nil, "pubsub: Message.Metadata keys must be valid UTF-8 strings: %q", k)
		}
		if !utf8.ValidString(v) {
			return gcerr.Newf(gcerr.InvalidArgument, nil, "pubsub: Message.Metadata values must be valid UTF-8 strings: %q", v)
		}
	}
	dm := &driver.Message{
		Body:       m.Body,
		Metadata:   m.Metadata,
		BeforeSend: m.BeforeSend,
		AfterSend:  m.AfterSend,
	}
	return t.batcher.Add(ctx, dm)
}

var errTopicShutdown = gcerr.Newf(gcerr.FailedPrecondition, nil, "pubsub: Topic has been Shutdown")

// Shutdown flushes pending message sends and disconnects the Topic.
// It only returns after all pending messages have been sent.
func (t *Topic) Shutdown(ctx context.Context) (err error) {
	ctx = t.tracer.Start(ctx, "Topic.Shutdown")
	defer func() { t.tracer.End(ctx, err) }()

	t.mu.Lock()
	if errors.Is(t.err, errTopicShutdown) {
		defer t.mu.Unlock()
		return t.err
	}
	t.err = errTopicShutdown
	t.mu.Unlock()
	c := make(chan struct{})
	go func() {
		defer close(c)
		t.batcher.Shutdown()
	}()
	select {
	case <-ctx.Done():
	case <-c:
	}
	t.cancel()
	if err := t.driver.Close(); err != nil {
		return wrapError(t.driver, err)
	}
	return ctx.Err()
}

// As converts i to driver-specific types.
// See https://gocloud.dev/concepts/as/ for background information, the "As"
// examples in this package for examples, and the driver package
// documentation for the specific types supported for that driver.
func (t *Topic) As(i any) bool {
	return t.driver.As(i)
}

// ErrorAs converts err to driver-specific types.
// ErrorAs panics if i is nil or not a pointer.
// ErrorAs returns false if err == nil.
// See https://gocloud.dev/concepts/as/ for background information.
func (t *Topic) ErrorAs(err error, i any) bool {
	return gcerr.ErrorAs(err, i, t.driver.ErrorAs)
}

// NewTopic is for use by drivers only. Do not use in application code.
var NewTopic = newTopic

// newSendBatcher creates a batcher for topics, for use with NewTopic.
func newSendBatcher(ctx context.Context, t *Topic, dt driver.Topic, opts *batcher.Options) *batcher.Batcher {
	handler := func(items any) error {
		dms := items.([]*driver.Message)
		err := retry.Call(ctx, gax.Backoff{}, dt.IsRetryable, func() (err error) {
			ctx2 := t.tracer.Start(ctx, "driver.Topic.SendBatch")
			defer func() { t.tracer.End(ctx2, err) }()
			return dt.SendBatch(ctx2, dms)
		})
		if err != nil {
			return wrapError(dt, err)
		}
		return nil
	}
	return batcher.New(reflect.TypeOf(&driver.Message{}), opts, handler)
}

// newTopic makes a pubsub.Topic from a driver.Topic.
//
// opts may be nil to accept defaults.
func newTopic(d driver.Topic, opts *batcher.Options) *Topic {
	ctx, cancel := context.WithCancel(context.Background())
	t := &Topic{
		driver: d,
		tracer: newTracer(d),
		cancel: cancel,
	}
	t.batcher = newSendBatcher(ctx, t, d, opts)
	return t
}

const pkgName = "gocloud.dev/pubsub"

var (
	latencyMeasure = oc.LatencyMeasure(pkgName)

	// OpenCensusViews are predefined views for OpenCensus metrics.
	// The views include counts and latency distributions for API method calls.
	// See the example at https://godoc.org/go.opencensus.io/stats/view for usage.
	OpenCensusViews = oc.Views(pkgName, latencyMeasure)
)

func newTracer(driver any) *oc.Tracer {
	return &oc.Tracer{
		Package:        pkgName,
		Provider:       oc.ProviderName(driver),
		LatencyMeasure: latencyMeasure,
	}
}

// Subscription receives published messages.
type Subscription struct {
	driver driver.Subscription
	tracer *oc.Tracer
	// ackBatcher makes batches of acks and nacks and sends them to the server.
	ackBatcher    *batcher.Batcher
	canNack       bool            // true iff the driver supports Nack
	backgroundCtx context.Context // for background SendAcks and ReceiveBatch calls
	cancel        func()          // for canceling backgroundCtx

	recvBatchOpts *batcher.Options

	mu               sync.Mutex        // protects everything below
	q                []*driver.Message // local queue of messages downloaded from server
	err              error             // permanent error
	unreportedAckErr error             // permanent error from background SendAcks that hasn't been returned to the user yet
	waitc            chan struct{}     // for goroutines waiting on ReceiveBatch
	runningBatchSize float64           // running number of messages to request via ReceiveBatch
	throughputStart  time.Time         // start time for throughput measurement
	throughputCount  int               // number of msgs given out via Receive since throughputStart

	// Used in tests.
	preReceiveBatchHook func(maxMessages int)
}

const (
	// The desired duration of a subscription's queue of messages (the messages pulled
	// and waiting in memory to be doled out to Receive callers). This is how long
	// it would take to drain the queue at the current processing rate.
	// The relationship to queue length (number of messages) is
	//
	//      lengthInMessages = desiredQueueDuration / averageProcessTimePerMessage
	//
	// In other words, if it takes 100ms to process a message on average, and we want
	// 2s worth of queued messages, then we need 2/.1 = 20 messages in the queue.
	//
	// If desiredQueueDuration is too small, then there won't be a large enough buffer
	// of messages to handle fluctuations in processing time, and the queue is likely
	// to become empty, reducing throughput. If desiredQueueDuration is too large, then
	// messages will wait in memory for a long time, possibly timing out (that is,
	// their ack deadline will be exceeded). Those messages could have been handled
	// by another process receiving from the same subscription.
	desiredQueueDuration = 2 * time.Second

	// Expected duration of calls to driver.ReceiveBatch, at some high percentile.
	// We'll try to fetch more messages when the current queue is predicted
	// to be used up in expectedReceiveBatchDuration.
	expectedReceiveBatchDuration = 1 * time.Second

	// s.runningBatchSize holds our current best guess for how many messages to
	// fetch in order to have a buffer of desiredQueueDuration. When we have
	// fewer than prefetchRatio * s.runningBatchSize messages left, that means
	// we expect to run out of messages in expectedReceiveBatchDuration, so we
	// should initiate another ReceiveBatch call.
	prefetchRatio = float64(expectedReceiveBatchDuration) / float64(desiredQueueDuration)

	// The initial # of messages to request via ReceiveBatch.
	initialBatchSize = 1

	// The factor by which old batch sizes decay when a new value is added to the
	// running value. The larger this number, the more weight will be given to the
	// newest value in preference to older ones.
	//
	// The delta based on a single value is capped by the constants below.
	decay = 0.5

	// The maximum growth factor in a single jump. Higher values mean that the
	// batch size can increase more aggressively. For example, 2.0 means that the
	// batch size will at most double from one ReceiveBatch call to the next.
	maxGrowthFactor = 2.0

	// Similarly, the maximum shrink factor. Lower values mean that the batch size
	// can shrink more aggressively. For example; 0.75 means that the batch size
	// will at most shrink to 75% of what it was before. Note that values less
	// than (1-decay) will have no effect because the running value can't change
	// by more than that.
	maxShrinkFactor = 0.75

	// The maximum batch size to request. Setting this too low doesn't allow
	// drivers to get lots of messages at once; setting it too small risks having
	// drivers spend a long time in ReceiveBatch trying to achieve it.
	maxBatchSize = 3000
)

// updateBatchSize updates the number of messages to request in ReceiveBatch
// based on the previous batch size and the rate of messages being pulled from
// the queue, measured using s.throughput*.
//
// It returns the number of messages to request in this ReceiveBatch call.
//
// s.mu must be held.
func (s *Subscription) updateBatchSize() int {
	// If we're always only doing one at a time, there's no point in this.
	if s.recvBatchOpts != nil && s.recvBatchOpts.MaxBatchSize == 1 && s.recvBatchOpts.MaxHandlers == 1 {
		return 1
	}
	now := time.Now()
	if s.throughputStart.IsZero() {
		// No throughput measurement; don't update s.runningBatchSize.
	} else {
		// Update s.runningBatchSize based on throughput since our last time here,
		// as measured by the ratio of the number of messages returned to elapsed
		// time.
		elapsed := now.Sub(s.throughputStart)
		if elapsed < 100*time.Millisecond {
			// Avoid divide-by-zero and huge numbers.
			elapsed = 100 * time.Millisecond
		}
		msgsPerSec := float64(s.throughputCount) / elapsed.Seconds()

		// The "ideal" batch size is how many messages we'd need in the queue to
		// support desiredQueueDuration at the msgsPerSec rate.
		idealBatchSize := desiredQueueDuration.Seconds() * msgsPerSec

		// Move s.runningBatchSize towards the ideal.
		// We first combine the previous value and the new value, with weighting
		// based on decay, and then cap the growth/shrinkage.
		newBatchSize := s.runningBatchSize*(1-decay) + idealBatchSize*decay
		if maxSize := s.runningBatchSize * maxGrowthFactor; newBatchSize > maxSize {
			s.runningBatchSize = maxSize
		} else if minSize := s.runningBatchSize * maxShrinkFactor; newBatchSize < minSize {
			s.runningBatchSize = minSize
		} else {
			s.runningBatchSize = newBatchSize
		}
	}

	// Reset throughput measurement markers.
	s.throughputStart = now
	s.throughputCount = 0

	// Using Ceil guarantees at least one message.
	return int(math.Ceil(math.Min(s.runningBatchSize, maxBatchSize)))
}

// Receive receives and returns the next message from the Subscription's queue,
// blocking and polling if none are available. It can be called
// concurrently from multiple goroutines.
//
// Receive retries retryable errors from the underlying driver forever.
// Therefore, if Receive returns an error, either:
// 1. It is a non-retryable error from the underlying driver, either from
//
//	an attempt to fetch more messages or from an attempt to ack messages.
//	Operator intervention may be required (e.g., invalid resource, quota
//	error, etc.). Receive will return the same error from then on, so the
//	application should log the error and either recreate the Subscription,
//	or exit.
//
// 2. The provided ctx is Done. Error() on the returned error will include both
//
//	the ctx error and the underlying driver error, and ErrorAs on it
//	can access the underlying driver error type if needed. Receive may
//	be called again with a fresh ctx.
//
// Callers can distinguish between the two by checking if the ctx they passed
// is Done, or via xerrors.Is(err, context.DeadlineExceeded or context.Canceled)
// on the returned error.
//
// The Ack method of the returned Message must be called once the message has
// been processed, to prevent it from being received again.
func (s *Subscription) Receive(ctx context.Context) (_ *Message, err error) {
	ctx = s.tracer.Start(ctx, "Subscription.Receive")
	defer func() { s.tracer.End(ctx, err) }()

	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		// The lock is always held here, at the top of the loop.
		if s.err != nil {
			// The Subscription is in a permanent error state. Return the error.
			s.unreportedAckErr = nil
			return nil, s.err // s.err wrapped when set
		}

		// Short circuit if ctx is Done.
		// Otherwise, we'll continue to return messages from the queue, and even
		// get new messages if driver.ReceiveBatch doesn't return an error when
		// ctx is done.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if s.waitc == nil && float64(len(s.q)) <= s.runningBatchSize*prefetchRatio {
			// We think we're going to run out of messages in expectedReceiveBatchDuration,
			// and there's no outstanding ReceiveBatch call, so initiate one in the
			// background.
			// Completion will be signalled to this goroutine, and to any other
			// waiting goroutines, by closing s.waitc.
			s.waitc = make(chan struct{})
			batchSize := s.updateBatchSize()
			// log.Printf("BATCH SIZE %d", batchSize)

			go func() {
				if s.preReceiveBatchHook != nil {
					s.preReceiveBatchHook(batchSize)
				}
				resultChannel := s.getNextBatch(batchSize)
				for msgsOrError := range resultChannel {
					if len(msgsOrError.msgs) > 0 {
						// messages received from channel
						s.mu.Lock()
						s.q = append(s.q, msgsOrError.msgs...)
						s.mu.Unlock()
						// notify that queue should now have messages
						s.waitc <- struct{}{}
					} else if msgsOrError.err != nil {
						// err can receive message only after batch group completes
						// Non-retryable error from ReceiveBatch -> permanent error
						s.mu.Lock()
						s.err = msgsOrError.err
						s.mu.Unlock()
					}
				}
				// batch reception finished
				s.mu.Lock()
				close(s.waitc)
				s.waitc = nil
				s.mu.Unlock()
			}()
		}
		if len(s.q) > 0 {
			// At least one message is available. Return it.
			m := s.q[0]
			s.q = s.q[1:]
			s.throughputCount++

			// Convert driver.Message to Message.
			id := m.AckID
			md := m.Metadata
			if len(md) == 0 {
				md = nil
			}
			loggableID := m.LoggableID
			if loggableID == "" {
				// This shouldn't happen, but just in case it's better to be explicit.
				loggableID = "unknown"
			}
			m2 := &Message{
				LoggableID: loggableID,
				Body:       m.Body,
				Metadata:   md,
				asFunc:     m.AsFunc,
				nackable:   s.canNack,
			}
			m2.ack = func(isAck bool) {
				// Ignore the error channel. Errors are dealt with
				// in the ackBatcher handler.
				_ = s.ackBatcher.AddNoWait(&driver.AckInfo{AckID: id, IsAck: isAck})
			}
			// Add a finalizer that complains if the Message we return isn't
			// acked or nacked.
			_, file, lineno, ok := runtime.Caller(1) // the caller of Receive
			runtime.SetFinalizer(m2, func(m *Message) {
				m.mu.Lock()
				defer m.mu.Unlock()
				if !m.isAcked {
					var caller string
					if ok {
						caller = fmt.Sprintf(" (%s:%d)", file, lineno)
					}
					log.Printf("A pubsub.Message was never Acked or Nacked%s", caller)
				}
			})
			return m2, nil
		}
		// A call to ReceiveBatch must be in flight. Wait for it.
		waitc := s.waitc
		s.mu.Unlock() // unlock to allow message or error processing from background goroutine
		select {
		case <-waitc:
			// Continue to top of loop.
			s.mu.Lock()
		case <-ctx.Done():
			s.mu.Lock()
			return nil, ctx.Err()
		}
	}
}

type msgsOrError struct {
	msgs []*driver.Message
	err  error
}

// getNextBatch gets the next batch of messages from the server. It will return a channel that will itself return the
// messages as they come from each independent batch, or an operation error
func (s *Subscription) getNextBatch(nMessages int) chan msgsOrError {
	// Split nMessages into batches based on recvBatchOpts; we'll make a
	// separate ReceiveBatch call for each batch, and aggregate the results in
	// msgs.
	batches := batcher.Split(nMessages, s.recvBatchOpts)
	result := make(chan msgsOrError, len(batches))
	g, ctx := errgroup.WithContext(s.backgroundCtx)
	for _, maxMessagesInBatch := range batches {
		// Make a copy of the loop variable since it will be used by a goroutine.
		curMaxMessagesInBatch := maxMessagesInBatch
		g.Go(func() error {
			var msgs []*driver.Message
			err := retry.Call(ctx, gax.Backoff{}, s.driver.IsRetryable, func() error {
				var err error
				ctx2 := s.tracer.Start(ctx, "driver.Subscription.ReceiveBatch")
				defer func() { s.tracer.End(ctx2, err) }()
				msgs, err = s.driver.ReceiveBatch(ctx2, curMaxMessagesInBatch)
				return err
			})
			if err != nil {
				return wrapError(s.driver, err)
			}
			result <- msgsOrError{msgs: msgs}
			return nil
		})
	}
	go func() {
		// wait on group completion on the background and proper channel closing
		if err := g.Wait(); err != nil {
			result <- msgsOrError{err: err}
		}
		close(result)
	}()
	return result
}

var errSubscriptionShutdown = gcerr.Newf(gcerr.FailedPrecondition, nil, "pubsub: Subscription has been Shutdown")

// Shutdown flushes pending ack sends and disconnects the Subscription.
func (s *Subscription) Shutdown(ctx context.Context) (err error) {
	ctx = s.tracer.Start(ctx, "Subscription.Shutdown")
	defer func() { s.tracer.End(ctx, err) }()

	s.mu.Lock()
	if errors.Is(s.err, errSubscriptionShutdown) {
		// Already Shutdown.
		defer s.mu.Unlock()
		return s.err
	}
	s.err = errSubscriptionShutdown
	s.mu.Unlock()
	c := make(chan struct{})
	go func() {
		defer close(c)
		if s.ackBatcher != nil {
			s.ackBatcher.Shutdown()
		}
	}()
	select {
	case <-ctx.Done():
	case <-c:
	}
	s.cancel()
	if err := s.driver.Close(); err != nil {
		return wrapError(s.driver, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.unreportedAckErr; err != nil {
		s.unreportedAckErr = nil
		return err
	}
	return ctx.Err()
}

// As converts i to driver-specific types.
// See https://gocloud.dev/concepts/as/ for background information, the "As"
// examples in this package for examples, and the driver package
// documentation for the specific types supported for that driver.
func (s *Subscription) As(i any) bool {
	return s.driver.As(i)
}

// ErrorAs converts err to driver-specific types.
// ErrorAs panics if i is nil or not a pointer.
// ErrorAs returns false if err == nil.
// See Topic.As for more details.
func (s *Subscription) ErrorAs(err error, i any) bool {
	return gcerr.ErrorAs(err, i, s.driver.ErrorAs)
}

// NewSubscription is for use by drivers only. Do not use in application code.
var NewSubscription = newSubscription

// newSubscription creates a Subscription from a driver.Subscription.
//
// recvBatchOpts sets options for Receive batching. May be nil to accept
// defaults. The ideal number of messages to receive at a time is determined
// dynamically, then split into multiple possibly concurrent calls to
// driver.ReceiveBatch based on recvBatchOptions.
//
// ackBatcherOpts sets options for ack+nack batching. May be nil to accept
// defaults.
func newSubscription(ds driver.Subscription, recvBatchOpts, ackBatcherOpts *batcher.Options) *Subscription {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Subscription{
		driver:           ds,
		tracer:           newTracer(ds),
		cancel:           cancel,
		backgroundCtx:    ctx,
		recvBatchOpts:    recvBatchOpts,
		runningBatchSize: initialBatchSize,
		canNack:          ds.CanNack(),
	}
	s.ackBatcher = newAckBatcher(ctx, s, ds, ackBatcherOpts)
	return s
}

func newAckBatcher(ctx context.Context, s *Subscription, ds driver.Subscription, opts *batcher.Options) *batcher.Batcher {
	handler := func(items any) error {
		var acks, nacks []driver.AckID
		for _, a := range items.([]*driver.AckInfo) {
			if a.IsAck {
				acks = append(acks, a.AckID)
			} else {
				nacks = append(nacks, a.AckID)
			}
		}
		g, ctx := errgroup.WithContext(ctx)
		if len(acks) > 0 {
			g.Go(func() error {
				return retry.Call(ctx, gax.Backoff{}, ds.IsRetryable, func() (err error) {
					ctx2 := s.tracer.Start(ctx, "driver.Subscription.SendAcks")
					defer func() { s.tracer.End(ctx2, err) }()
					return ds.SendAcks(ctx2, acks)
				})
			})
		}
		if len(nacks) > 0 {
			g.Go(func() error {
				return retry.Call(ctx, gax.Backoff{}, ds.IsRetryable, func() (err error) {
					ctx2 := s.tracer.Start(ctx, "driver.Subscription.SendNacks")
					defer func() { s.tracer.End(ctx2, err) }()
					return ds.SendNacks(ctx2, nacks)
				})
			})
		}
		err := g.Wait()
		// Remember a non-retryable error from SendAcks/Nacks. It will be returned on the
		// next call to Receive.
		if err != nil {
			err = wrapError(s.driver, err)
			s.mu.Lock()
			s.err = err
			s.unreportedAckErr = err
			s.mu.Unlock()
		}
		return err
	}
	return batcher.New(reflect.TypeOf([]*driver.AckInfo{}).Elem(), opts, handler)
}

type errorCoder interface {
	ErrorCode(error) gcerrors.ErrorCode
}

func wrapError(ec errorCoder, err error) error {
	if err == nil {
		return nil
	}
	if gcerr.DoNotWrap(err) {
		return err
	}
	return gcerr.New(ec.ErrorCode(err), err, 2, "pubsub")
}

// TopicURLOpener represents types than can open Topics based on a URL.
// The opener must not modify the URL argument. OpenTopicURL must be safe to
// call from multiple goroutines.
//
// This interface is generally implemented by types in driver packages.
type TopicURLOpener interface {
	OpenTopicURL(ctx context.Context, u *url.URL) (*Topic, error)
}

// SubscriptionURLOpener represents types than can open Subscriptions based on a URL.
// The opener must not modify the URL argument. OpenSubscriptionURL must be safe to
// call from multiple goroutines.
//
// This interface is generally implemented by types in driver packages.
type SubscriptionURLOpener interface {
	OpenSubscriptionURL(ctx context.Context, u *url.URL) (*Subscription, error)
}

// URLMux is a URL opener multiplexer. It matches the scheme of the URLs
// against a set of registered schemes and calls the opener that matches the
// URL's scheme.
// See https://gocloud.dev/concepts/urls/ for more information.
//
// The zero value is a multiplexer with no registered schemes.
type URLMux struct {
	subscriptionSchemes openurl.SchemeMap
	topicSchemes        openurl.SchemeMap
}

// TopicSchemes returns a sorted slice of the registered Topic schemes.
func (mux *URLMux) TopicSchemes() []string { return mux.topicSchemes.Schemes() }

// ValidTopicScheme returns true iff scheme has been registered for Topics.
func (mux *URLMux) ValidTopicScheme(scheme string) bool { return mux.topicSchemes.ValidScheme(scheme) }

// SubscriptionSchemes returns a sorted slice of the registered Subscription schemes.
func (mux *URLMux) SubscriptionSchemes() []string { return mux.subscriptionSchemes.Schemes() }

// ValidSubscriptionScheme returns true iff scheme has been registered for Subscriptions.
func (mux *URLMux) ValidSubscriptionScheme(scheme string) bool {
	return mux.subscriptionSchemes.ValidScheme(scheme)
}

// RegisterTopic registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterTopic panics.
func (mux *URLMux) RegisterTopic(scheme string, opener TopicURLOpener) {
	mux.topicSchemes.Register("pubsub", "Topic", scheme, opener)
}

// RegisterSubscription registers the opener with the given scheme. If an opener
// already exists for the scheme, RegisterSubscription panics.
func (mux *URLMux) RegisterSubscription(scheme string, opener SubscriptionURLOpener) {
	mux.subscriptionSchemes.Register("pubsub", "Subscription", scheme, opener)
}

// OpenTopic calls OpenTopicURL with the URL parsed from urlstr.
// OpenTopic is safe to call from multiple goroutines.
func (mux *URLMux) OpenTopic(ctx context.Context, urlstr string) (*Topic, error) {
	opener, u, err := mux.topicSchemes.FromString("Topic", urlstr)
	if err != nil {
		return nil, err
	}
	return opener.(TopicURLOpener).OpenTopicURL(ctx, u)
}

// OpenSubscription calls OpenSubscriptionURL with the URL parsed from urlstr.
// OpenSubscription is safe to call from multiple goroutines.
func (mux *URLMux) OpenSubscription(ctx context.Context, urlstr string) (*Subscription, error) {
	opener, u, err := mux.subscriptionSchemes.FromString("Subscription", urlstr)
	if err != nil {
		return nil, err
	}
	return opener.(SubscriptionURLOpener).OpenSubscriptionURL(ctx, u)
}

// OpenTopicURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenTopicURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenTopicURL(ctx context.Context, u *url.URL) (*Topic, error) {
	opener, err := mux.topicSchemes.FromURL("Topic", u)
	if err != nil {
		return nil, err
	}
	return opener.(TopicURLOpener).OpenTopicURL(ctx, u)
}

// OpenSubscriptionURL dispatches the URL to the opener that is registered with the
// URL's scheme. OpenSubscriptionURL is safe to call from multiple goroutines.
func (mux *URLMux) OpenSubscriptionURL(ctx context.Context, u *url.URL) (*Subscription, error) {
	opener, err := mux.subscriptionSchemes.FromURL("Subscription", u)
	if err != nil {
		return nil, err
	}
	return opener.(SubscriptionURLOpener).OpenSubscriptionURL(ctx, u)
}

var defaultURLMux = &URLMux{}

// DefaultURLMux returns the URLMux used by OpenTopic and OpenSubscription.
//
// Driver packages can use this to register their TopicURLOpener and/or
// SubscriptionURLOpener on the mux.
func DefaultURLMux() *URLMux {
	return defaultURLMux
}

// OpenTopic opens the Topic identified by the URL given.
// See the URLOpener documentation in driver subpackages for
// details on supported URL formats, and https://gocloud.dev/concepts/urls
// for more information.
func OpenTopic(ctx context.Context, urlstr string) (*Topic, error) {
	return defaultURLMux.OpenTopic(ctx, urlstr)
}

// OpenSubscription opens the Subscription identified by the URL given.
// See the URLOpener documentation in driver subpackages for
// details on supported URL formats, and https://gocloud.dev/concepts/urls
// for more information.
func OpenSubscription(ctx context.Context, urlstr string) (*Subscription, error) {
	return defaultURLMux.OpenSubscription(ctx, urlstr)
}
