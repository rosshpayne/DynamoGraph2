package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/DynamoGraph/cache"
	"github.com/DynamoGraph/dp/internal/db"
	"github.com/DynamoGraph/ds"
	stat "github.com/DynamoGraph/gql/monitor"
	"github.com/DynamoGraph/rdf/grmgr"
	slog "github.com/DynamoGraph/syslog"
	"github.com/DynamoGraph/types"
	"github.com/DynamoGraph/util"
)

const (
	dpPart = "G"
)

var (
	ctx    context.Context
	cancel context.CancelFunc
	ctxEnd sync.WaitGroup
	//
	replyCh     chan interface{}
	statDPitems stat.Request
	//
	t0, t1, t2 time.Time
)

func syslog(s string) {
	slog.Log("gql: ", s)
}

func main() {

	types.SetGraph("Movie")

	Startup()

	//clear monitor stats
	stat.ClearCh <- struct{}{}
	dpCh := make(chan db.DataT)

	runlimiter := grmgr.New("run", 6)

	t0 = time.Now()
	//
	var c float64
	var wg sync.WaitGroup
	for _, attr := range []string{"P", "Fm"} { // types containing nested 1:1 type

		// start scan service
		go db.ScanForDPitems(attr, dpCh)

		for v := range dpCh {

			if v.Eod {
				fmt.Println("============== EOD ============")
				break
			}
			fmt.Println("Process Uid: ", v.Uid.String())
			c++
			if math.Mod(c, 100) == 0 {
				fmt.Printf("\t\t\t %g\n", c)
			}
			runlimiter.Ask()
			<-runlimiter.RespCh()
			wg.Add(1)

			go processDP(runlimiter, &wg, v.Uid)

		}
		fmt.Println("Waiting for processDPs to finish....")
		wg.Wait()
		fmt.Println("finished....")
	}

	Shutdown()

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
	syslog("Shutdown...")
}

func processDP(limit *grmgr.Limiter, wg *sync.WaitGroup, pUID util.UID) {

	defer limit.EndR()
	defer wg.Done()

	gc := cache.GetCache()
	// fetch all p's uid-pred data into cache
	nc, err := gc.FetchForUpdate(pUID, "A#G")
	if err != nil {
		panic(fmt.Errorf("FetchForUpdate error: %s", err.Error()))
	}
	// p's type
	ty, _ := nc.GetType()
	//
	// for each performance type in p
	//
	tyAttrs := types.TypeC.TyC[ty]
	//
	// for all performance types in p
	//
	var (
		nds [][][]byte
		ok  bool
	)
	for _, v := range tyAttrs {
		if v.Ty != "Performance" {
			continue
		}
		// generate NV
		un := v.Name + ":"
		nv := &ds.NV{Name: un}
		nvc := make(ds.ClientNV, 1, 1)
		nvc[0] = nv
		psortk := "A#" + dpPart + "#:" + v.C

		//
		// assign cached data to NV
		//
		//fmt.Println("about to .. UnmarshalNodeCache, result.tyS ", result.tyS)
		err = nc.UnmarshalNodeCache(nvc, ty)
		if err != nil {
			panic(err)
		}
		//
		//
		data := nvc[0]
		if nds, ok = data.Value.([][][]byte); !ok {
			panic(fmt.Errorf("filterRootResult: data.Value is of wrong type")) // TODO: replace panic with error msg???
		}
		// child uids for particular performance attribute type
		var init bool

		for _, u := range nds {
			// for each child in outer uid-pred (x.Name)
			for _, uid := range u {
				//
				// fetch child propagated data into cache
				ncc, err := gc.FetchNode(uid, "A#G")
				if err != nil {
					panic(fmt.Errorf("FetchForUpdate error: %s", err.Error()))
				}
				// for k, _ := range ncc.GetMap() {
				// 	fmt.Println("child sortk: ", k)
				// }
				ty := "Performance" // 1:1 attribute
				if !init {
					init = true
					for _, t := range types.TypeC.TyC[ty] {
						if t.Card != "1:1" {
							continue
						}
						sk := "A#G#:" + t.C
						db.InitPromoteUID(pUID, psortk+"#"+sk[2:])
						for _, t_ := range types.TypeC.TyC[t.Ty] {
							//
							compare := sk + "#:" + t_.C
							save := sk
							for k, _ := range ncc.GetMap() {
								if k == compare {
									sk += "#:" + t_.C
									switch t_.DT {
									case "S":
										db.InitPromoteString(pUID, psortk+"#"+sk[2:])
									}
								}
								sk = save
							}
						}
					}
				}
				for _, t := range types.TypeC.TyC[ty] {
					if t.Card != "1:1" {
						continue
					}
					sk := "A#G#:" + t.C
					for k, m := range ncc.GetMap() {
						if k == sk {
							n, xf, _ := m.GetNd()
							//fmt.Printf("PromoteUID: %s %s %T [%s] %v \n", psortk+"#"+sk[2:], k, m, n[1], xf[1])
							db.PromoteUID(pUID, uid, psortk+"#"+sk[2:], n[1], xf[1])
						}
					}
					for _, t_ := range types.TypeC.TyC[t.Ty] {
						//
						compare := sk + "#:" + t_.C
						save := sk
						for k, m := range ncc.GetMap() {
							if k == compare {
								sk += "#:" + t_.C
								switch t_.DT {
								case "S":
									s, bl := m.GetULS()
									db.PromoteString(pUID, uid, psortk+"#"+sk[2:], s[1], bl[1])
								}
							}
							sk = save
						}
					}
				}
			}
		}
	}
	//
	// post: update IX to Y
	//
	db.UpdateIX(pUID)

}
