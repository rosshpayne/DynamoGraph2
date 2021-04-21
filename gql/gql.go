package gql

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DynamoGraph/db"
	"github.com/DynamoGraph/gql/ast"
	stat "github.com/DynamoGraph/gql/monitor"
	"github.com/DynamoGraph/gql/parser"
	"github.com/DynamoGraph/rdf/grmgr"
	slog "github.com/DynamoGraph/syslog"
)

var (
	ctx    context.Context
	cancel context.CancelFunc
	ctxEnd sync.WaitGroup
	//
	replyCh        chan interface{}
	statTouchNodes stat.Request
	statTouchLvl   stat.Request
	statDbFetches  stat.Request
	//
	expectedJSON       string
	expectedTouchLvl   []int
	expectedTouchNodes int
	//
	t0, t1, t2 time.Time
)

func syslog(s string) {
	slog.Log("gql: ", s)
}

func init() {

	replyCh = make(chan interface{})
	statTouchNodes = stat.Request{Id: stat.TouchNodeFiltered, ReplyCh: replyCh}
	statTouchLvl = stat.Request{Id: stat.TouchLvlFiltered, ReplyCh: replyCh}
	statDbFetches = stat.Request{Id: stat.NodeFetch, ReplyCh: replyCh}

	Startup()

	fmt.Println("====================== STARTUP =====================")
}

func validate(t *testing.T, result string, abort ...bool) {

	var msg string

	t.Log(result)

	stat.GetCh <- statTouchNodes
	nodes := <-replyCh

	stat.GetCh <- statTouchLvl
	levels := <-replyCh

	stat.GetCh <- statDbFetches
	fetches := <-replyCh

	status := "P" // Passed
	if compareStat(nodes, expectedTouchNodes) {
		status = "F" // Failed
		msg = fmt.Sprintf("Error: in nodes touched. Expected %d got %d", expectedTouchNodes, nodes)
		t.Error(msg)
	}
	if compareStat(levels, expectedTouchLvl) {
		status = "F" // Failed
		msg += fmt.Sprintf(" | Error: in nodes touched at levels. Expected %v got %v", expectedTouchLvl, levels)
		t.Error(msg)
	}

	if len(expectedJSON) > 0 && compareJSON(result, expectedJSON) {
		t.Error("JSON is not as expected: ")
	}
	//
	// must check if stats have been populated which will not be the case when all nodes have failed to pass the filter.
	// note: this code presumes expected variables always have values even when nothing is expected (in which case they will be populated with zero values)
	var (
		fetches_, nodes_ int
		levels_          []int
		abort_           bool
	)
	if len(abort) > 0 {
		abort_ = abort[0]
	} else {
		abort_ = false
	}
	if levels != nil {
		levels_ = levels.([]int)
	}
	if fetches != nil {
		fetches_ = fetches.(int)
	}
	if nodes != nil {
		nodes_ = nodes.(int)
	}
	db.SaveTestResult(t.Name(), status, nodes_, levels_, t1.Sub(t0).String(), t2.Sub(t1).String(), msg, result, fetches_, abort_)
	//
	// clear
	//
	expectedJSON = ``
	expectedTouchNodes = -1
	expectedTouchLvl = []int{}
}

func Execute(graph string, query string) *ast.RootStmt {

	//clear monitor stats
	stat.ClearCh <- struct{}{}

	golimiter := grmgr.New("execute", 6)

	t0 = time.Now()
	p := parser.New(graph, query)
	stmt, errs := p.ParseInput()
	if len(errs) > 0 {
		for _, v := range errs {
			fmt.Println("error: ", v)
		}
		panic(errs[0])
	}
	//
	t1 = time.Now()
	stmt.Execute(golimiter)
	t2 = time.Now()

	fmt.Printf("Duration:  Parse  %s  Execute: %s    \n", t1.Sub(t0), t2.Sub(t1))
	syslog(fmt.Sprintf("Duration: Parse  %s  Execute: %s ", t1.Sub(t0), t2.Sub(t1)))
	time.Sleep(2 * time.Second) // give time for stat to empty its channel queues

	//stat.PrintCh <- struct{}{}
	//Shutdown()

	return stmt

}

func Startup() {

	var (
		wpStart sync.WaitGroup
	)
	syslog("Startup...")
	wpStart.Add(2)
	// check verify and saveNode have finished. Each goroutine is responsible for closing and waiting for all routines they spawn.
	ctxEnd.Add(2)
	// l := lexer.New(input)
	// p := New(l)
	//
	// context - used to shutdown goroutines that are not part fo the pipeline
	//
	ctx, cancel = context.WithCancel(context.Background())

	go grmgr.PowerOn(ctx, &wpStart, &ctxEnd)
	go stat.PowerOn(ctx, &wpStart, &ctxEnd)

	wpStart.Wait()
	syslog(fmt.Sprintf("services started "))
}

func Shutdown() {

	syslog("Shutdown commenced...")
	cancel()

	ctxEnd.Wait()
	syslog("Shutdown Completed")
}
