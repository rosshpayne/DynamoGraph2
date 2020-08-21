package grmgr

import (
	"context"
	"fmt"
	"time"

	"sync"

	slog "github.com/DynamoGraph/syslog"
)

type Routine = string

type Ceiling = int

/////////////////////////////////////
//
// register gRoutine start
//
var StartCh = make(chan Routine)

type rCntMap map[Routine]Ceiling

var rCnt rCntMap

type rWaitMap map[Routine]bool

var rWait rWaitMap

//
// Channels
//
var EndCh = make(chan Routine)
var rAskCh = make(chan Routine)

//
// Limiter
//
type respCh chan struct{}

type Limiter struct {
	c  Ceiling
	r  Routine
	ch respCh
	on bool // send Wait response
}

func (l Limiter) Ask() {
	rAskCh <- l.r
}

func (l Limiter) StartR() {
	StartCh <- l.r
}

func (l Limiter) EndR() {
	EndCh <- l.r
}

func (l Limiter) RespCh() respCh {
	return l.ch
}
func (l Limiter) Routine() Routine {
	return l.r
}

type rLimiterMap map[Routine]*Limiter

var rLimit rLimiterMap

var registerCh = make(chan *Limiter)

//
//
//

func init() {
	rCnt = make(rCntMap)
	rLimit = make(rLimiterMap)
	rWait = make(rWaitMap)
}

// Note: this package provides a slight enhancement to scaling goroutines the the channel buffer provides.
// It is designed to throttle the number of running instances of a go Routine, i.e. it sets a ceiling on the number of concurrent goRoutines of a particular routine.
// I cannot think of how to get the sync.WaitGroup to provide this feature. It is good for waiting on goRoutines to finish but
// I don't know how to configure sync to set a ceiling on the number of concurrent goRoutines.

// var eventCh chan struct{}{}

//   main
//   	eventCh=make(chan struct{}{},5)
//   	for {
//   		eventCh <- x  // the buffers will fill only if the receiptent of the message does not run a goroutine i.e. is synchronised. if the recipient is not a goroutine their will be only one process
//                        // so to keep the main program from waiting for it to finish we include a buffer on the channel. Hopefully before the buffer fills the recipient will finish and
//                        // execute again.
//   	}                 // if the recipeient runs as go routine then the recipient will empty the buffer as fast as the main will fill it. This may lead to func X spawning a very large
//                        //. number of goroutines the number of which are not impacted by the channel buffer size.
//   }

//   func_ X1
//  	for e = range eventCh { // this will read from channel, start goRoutine and then read from channel again until it is closed
//			go Routine          // The buffer will limit the number of active groutines. As one finishes this will free up a buffer slot and main will fill it with another request to be immediately read by X.
//  	}
//  }
//   func_ X2
//  	for e = range eventCh { // this will read from channel, start goRoutine and then read from channel again until it is closed
//			Routine            // The buffer will limit the number of active groutines. As one finishes this will free up a buffer slot and main will fill it with another request to be immediately read by X.
//  	}
//  }
//
//   So channel buffers are not useful for recipients of channel events that execute go routines. They are useful when the recipient is synchronised with the execution.
//    For goroutine recipients we need a mechanism that can throttle the running of goroutines. This package provides this service.
//
//   func_ Y
//   	z := grmgr.New(<routine>, 5)
//
//   		for e = range eventCh
//   			go Routine          // same as above, unlimited concurrent go routines run. go routine includes Start and End channel messages that increments & decrements internal counter.
//				<-z.Wait()          //  grmgr will send event  on channel if there are less than Ceiling number of concurrent go routines.
//   	}							// Note grmgr limit must be less than channel buffer. So set a large channel buffer and use grmgr to fluctuate between.
//   }

//   func_ Routine {

//   }

func New(r string, c Ceiling) Limiter {
	l := Limiter{c: c, r: Routine(r), ch: make(chan struct{}), on: true}
	registerCh <- &l
	return l
}

// use channels to synchronise access to shared memory ie. the various maps, rLimiterMap.rCntMap.
// "don't communicate by sharing memory, share memory by communicating"
// grmgr runs as a single goroutine with sole access to the shared memory objects. Clients request or update data via channel requests.
func PowerOn(ctx context.Context, wp *sync.WaitGroup, wgEnd *sync.WaitGroup) {

	defer wgEnd.Done()
	var (
		r Routine
		l *Limiter
	)

	slog.Log("grmgr: ", "Powering on...")
	wp.Done()

	for {

		select {

		case r = <-StartCh:

			rCnt[r] += 1

			slog.Log("grmgr: ", fmt.Sprintf("StartCh received for %s. rCnt = %d ", r, rCnt[r]))

		case r = <-EndCh:

			rCnt[r] -= 1

			slog.Log("grmgr: ", fmt.Sprintf("EndCh received for %s. rCnt = %d ", r, rCnt[r]))

			if b, ok := rWait[r]; ok {
				if b && rCnt[r] < rLimit[r].c {
					slog.Log("grmgr: ", fmt.Sprintf("Send ack to waiting %s...", r))
					rLimit[r].ch <- struct{}{}
					rWait[r] = false
				}
			}

		case l = <-registerCh: // change the ceiling by passing in Limiter struct. As struct is a non-ref type, l is a copy of struct passed into channel. Ref typs, spmfc - slice, pointer, map, func, channel

			rLimit[l.r] = l
			rCnt[r] = 0

		case r = <-rAskCh:

			// check if any goroutines are starting...particular of the current reoutine
			for caught, i := false, 0; i < 5; i++ {
				slog.Log("grmgr: ", fmt.Sprintf("Looping..... %d", i))
				select {
				case rr := <-StartCh:
					rCnt[rr] += 1
					if r == rr {
						slog.Log("grmgr: ", fmt.Sprintf("CAUGHT - goroutine started by previous Ask for %s", r))
						caught = true
					}
				default:
				}
				if caught {
					break
				}
				time.Sleep(5 * time.Microsecond)
			}

			if rCnt[r] < rLimit[r].c {
				slog.Log("grmgr: ", fmt.Sprintf("has ASKed. Under cnt limit. Send ACK on routine channel..%s", r))
				rLimit[r].ch <- struct{}{} // proceed to run gr
			} else {
				slog.Log("grmgr: ", fmt.Sprintf("has ASKed. Cnt is above limit. Mark %s as waiting", r))
				rWait[r] = true // log routine as waiting to proceed
			}

		case <-ctx.Done():

			// TODO: Done should be in a separate select. If a request and Done occur simultaneously then go will randomly pick one.
			// separating them means we have control. Is that the solution. Ideally we should control outside of uuid func().
			slog.Log("grmgr: ", "Powering down...")
			return

		}

	}
}