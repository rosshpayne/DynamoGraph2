package ast

import (
	"fmt"
	"strings"
	"sync"

	blk "github.com/DynamoGraph/block"
	"github.com/DynamoGraph/cache"
	"github.com/DynamoGraph/ds"
	mon "github.com/DynamoGraph/gql/monitor"
	"github.com/DynamoGraph/rdf/grmgr"
	"github.com/DynamoGraph/types"
	"github.com/DynamoGraph/util"
)

type rootResult struct {
	uid   util.UID    //returned from root query
	tyS   string      //returned from root query
	sortk string      // returned from root query
	path  string      // #root (R) #root#Siblings#Friends - not currently used
	nv    ds.ClientNV // the data from the uid item. Populated during execution phase with results of filter operation.
}

// index into node data UL structures (see UnmarshalNodeCache). Used to get node scalar data.
type index struct {
	i, j int
}

func (r *RootStmt) Execute(grl *grmgr.Limiter) {
	//
	// execute root func - get back slice of unfiltered results
	//
	result := r.RootFunc.F(r.RootFunc.Farg, r.RootFunc.Value)

	if len(result) == 0 {
		return
	}
	var wgRoot sync.WaitGroup
	stat := mon.Stat{Id: mon.Candidate, Value: len(result)}
	mon.StatCh <- stat
	stat2 := mon.Stat{Id: mon.TouchNode, Lvl: 0}

	for _, v := range result {

		//grl.Ask()
		//<-grl.RespCh()

		mon.StatCh <- stat2

		wgRoot.Add(1)
		result := &rootResult{uid: v.PKey, tyS: v.Ty, sortk: v.SortK, path: "root"}

		r.filterRootResult(grl, &wgRoot, result)

	}
	wgRoot.Wait()

}

func (r *RootStmt) filterRootResult(grl *grmgr.Limiter, wg *sync.WaitGroup, result *rootResult) {
	var (
		err error
		nc  *cache.NodeCache
	)
	//defer grl.EndR()
	defer wg.Done()
	//
	// save: filter-visit-node uid
	//
	// generate NV from GQL stmt - will also hold data from query response once UmarshalNodeCache is run.
	// query->cache->unmarshal(nv)
	//
	nvc := r.genNV(result.tyS)
	// fmt.Println("==== Root genNV_ =====")
	// for _, n := range nvc {
	// 	fmt.Println("Root genNV__: ", n.Name, n.Ignore)
	// }
	//
	// generate sortk - determines extent of node data to be loaded into cache. Tries to keep it as norrow (specific) as possible.
	//
	sortkS := cache.GenSortK(nvc, result.tyS)
	// for _, s := range sortkS {
	// 	fmt.Println("Ysortk: ", s)
	// }
	//
	// fetch data - with optimised fetch - perform queries sequentially becuase of mutex lock on node map
	//
	gc := cache.GetCache()
	for _, sortk := range sortkS {
		//	//xxfmt.Println("filterRoot - FetchNodeNonCache for : ", result.uid, sortk)
		stat := mon.Stat{Id: mon.NodeFetch}
		mon.StatCh <- stat
		nc, _ = gc.FetchNodeNonCache(result.uid, sortk)
	}
	// for k, _ := range nc.GetMap() {
	// 	fmt.Println("GetMap sortk: ", k)
	// }
	//
	// assign cached data to NV
	//
	// assign the cached data to the Value field in the nvc for each sortkS
	err = nc.UnmarshalNodeCache(nvc, result.tyS)
	if err != nil {
		panic(err)
	}
	//
	// root filter
	//
	if r.Filter != nil && !r.Filter.RootApply(nvc, result.tyS) {
		// nc.ClearCache() to free memory // TODO: implement
		return
	}
	//
	// save result node data (represented by uid - nvm) to root stmt
	//
	nvm := r.assignData(result.uid.String(), nvc, index{0, 0})
	//
	//
	stat := mon.Stat{Id: mon.PassRootFilter}
	mon.StatCh <- stat
	//
	var wgNode sync.WaitGroup

	for _, p := range r.Select {

		switch x := p.Edge.(type) {

		case *ScalarPred:
			// do nothing as in cache

		case *UidPred: // child of child, R.N.N - this data is cached in parent node
			var (
				aty blk.TyAttrD
				ok  bool
			)
			x.lvl = 1

			if aty, ok = types.TypeC.TyAttrC[result.tyS+":"+x.Name()]; !ok {
				panic(fmt.Errorf("% not in ", result.tyS, x.Name()))
				continue // ignore this attribute as it is in current type
			}
			// filter by setting STATE value for each edge in NVM. NVM has been saved to root stmt
			// and is used by MarshalJSON to output edges from the root node.
			if x.Filter != nil {
				x.Filter.Apply(nvm, aty.Ty, x.Name()) // AAA - on first uid-pred - on each edge mark as EdgeFiltered true|false
			}

			for _, p := range x.Select {

				switch y := p.Edge.(type) {

				case *ScalarPred, *Variable:
					// do nothing as UnmarshalNodeCode has already assigned scalar results in n

				case *UidPred:
					// data will need to be sourced from db
					// execute query on each x.Name() item and use the propagated uid-pred data to resolve this uid-pred
					var (
						idx index
						nds [][][]byte
					)
					//
					// to get the UIDs in y we need to perform a query on each UID in the parent uid-pred (ie. x).
					// data will contain the parent uids we will want to query. Each uid in y represents a child node to
					// an individual uid in x. If there are 10 child nodes to a parent uid then there will be 10 UIDs in y uid-pred.
					//
					data, ok := nvm[x.Name()+":"]
					if !ok {
						panic(fmt.Errorf("%q not in NV map", x.Name()+":"))
					}
					if nds, ok = data.Value.([][][]byte); !ok {
						panic(fmt.Errorf("filterRootResult: data.Value is of wrong type")) // TODO: replace panic with error msg???
					}
					// for each Nd uid (on uid edge)
					for i, u := range nds {
						// for each child in outer uid-pred (x.Name)
						for j, uid := range u {

							// check the result of the filter condition on x determined at AAA ie. filter on child nodes whose age > 62
							if data.State[i][j] == blk.UIDdetached || data.State[i][j] == blk.EdgeFiltered { // soft delete set
								continue
							}
							// i,j - defined key for looking up child node UID in cache block.
							//grl.EndR()
							//grl.Ask()
							//<-grl.RespCh()

							wgNode.Add(1)
							idx = index{i, j} // child node location in UL cache
							sortk := "A#G#:" + aty.C
							//fmt.Printf("\nUid: %s   %s   %s  sortk: [%s]\n", x.Name(), util.UID(uid).String(), y.Name(), sortk)

							y.execNode(nil, &wgNode, util.UID(uid), aty.Ty, 2, y.Name(), idx, result.uid, sortk)
						}
					}
				}
			}
		}
	}
	wgNode.Wait()

}

// execNode takes parent node (depth-1)and performs UmarshalCacheNode on its uid-preds.
// ty   type of parent node
// us is the current uid-pred from filterRootResult
// uidp is uid current node - not used anymore.
func (u *UidPred) execNode(grl *grmgr.Limiter, wg *sync.WaitGroup, uid_ util.UID, ty string, lvl int, uidp string, idx index, ruid util.UID, sortk_ string) {

	var (
		err error
		nc  *cache.NodeCache
		nvm ds.NVmap // where map key is NV.Name
		nvc ds.ClientNV
		ok  bool
		uty blk.TyAttrD // uid-pred and parent-to-uid-pred type
	)
	uid := uid_.String() // TODO: chanve to pass uuid into execNode as string

	//fmt.Printf("**************************************************** in execNode() %s, %s Depth: %d  current uidpred: %s\n", uid, ty, lvl, uidp)
	uty = types.TypeC.TyAttrC[ty+":"+uidp]
	// note: source of data (nvm) for u is sourced from u's parent propagated data ie. u's data is in the list structures of u-parent (propagated data)
	//
	// if grl != nil {
	// 	defer grl.EndR()
	// }
	defer wg.Done()
	//
	u.lvl = lvl // depth in graph as determined from GQL stmt

	if nvm, nvc, ok = u.Parent.getData(uid); !ok {
		//
		// first instance of a uid-pred in node to be executed. All other uid-preds in this node can ignore fetching data from db as its data was included in the first uid-pred query.
		//
		// generate NV from GQL stmt - for each uid-pred even though this is not strictly necessary. If nvm, nvc was saved to u then it would only need to be generated once for u.
		// query->cache->unmarshal(nv). Generate from parent of u, as it contains u. The uid is sourced from the parent node's relevant uid-pred attribute.
		// we need to perform a query on each uid as it represents the children containing u.
		//
		// as the data is sourced from u-parent so must the NV listing. Only interested in the uid-preds and its scalar types, as this includes the data for u (and its uid-pred siblings)
		//
		nvc = u.Parent.genNV(ty)
		//
		// generate sortk - source from node type and NV - merge of two.
		//                  determines extent of node data to be loaded into cache. Tries to keep it as norrow (specific) as possible to minimise RCUs.
		//                  ty is the type of the parent uid-pred (uid passed in)
		//
		sortkS := cache.GenSortK(nvc, ty)
		//
		switch uty.Card {

		case "1:N":
			// fetch data - with optimised fetch - perform queries sequentially because of mutex lock on node map
			// uid is sourced from u's parent uid-pred.
			gc := cache.GetCache()
			for _, sortk := range sortkS {
				stat := mon.Stat{Id: mon.NodeFetch}
				mon.StatCh <- stat
				nc, _ = gc.FetchNodeNonCache(uid_, sortk) // BBB
			}
			//
			// unmarshal cache contents into nvc
			//
			err = nc.UnmarshalNodeCache(nvc, ty)
			if err != nil {
				panic(err)
			}

		case "1:1":

			var (
				nvc_ ds.ClientNV
			)
			switch x := u.Parent.(type) {

			case *UidPred:
				switch x := x.Parent.(type) {
				case *UidPred:
					_, nvc_, ok = x.getData(ruid.String())
				case *RootStmt:
					_, nvc_, ok = x.getData(ruid.String())
				}

			case *RootStmt:
				_, nvc_, ok = x.getData(ruid.String())
			}
			//xxfmt.Println("assign from ", sortk_+"#G#:"+uty.C)
			//
			// load data from nvm_, nvc_ in parent
			// note: there is a one to one correspondence in valudes between list type
			// in A#G#? and nvc_
			type by [][]byte
			for _, n := range nvc { // needs to be populated
				for _, v := range nvc_ { // sourced from parent NVC
					if strings.HasSuffix(v.Name, n.Name) {

						switch n.Name[len(n.Name)-1] {
						case ':': // uid-pred
							// search for uid
							// idx entry
							//xxfmt.Println("v.Name = ", v.Name, n.Name)
							uids := v.Value.([][][]byte)
							uu := make([][]byte, 1, 1)
							uuu := make([][][]byte, 1, 1)
							//
							uu[0] = uids[idx.i][idx.j] // one to one between uids and A#G#? values
							uuu[0] = uu
							n.Value = uuu
							n.State = [][]int{{v.State[idx.i][idx.j]}}

						default: // scalar
							switch x := v.Value.(type) {
							case [][]string:
								val := make([]string, 1, 1)
								val2 := make([][]string, 1, 1)
								val[0] = x[idx.i][idx.j]
								val2[0] = val
								//
								s := make([]int, 1, 1)
								s2 := make([][]int, 1, 1)
								s[0] = v.State[idx.i][idx.j]
								s2[0] = s
								n.Value = val2
								n.State = s2
							}
						}
					}
				}
			}
		}
		//
		// save NV data to a map with uid key and map to u's parent, as it is the source of the NV
		//
		nvm = u.Parent.assignData(uid, nvc, idx)
	}
	// for _, v := range nvc {
	// 	fmt.Printf("execnode nvc: %#v\n\n", v)
	// }

	//
	// for a filter: update nvm edges related to u. Note: filter  is the only component  we make use of u directly. Most other access is via u's parent uid-pred
	// as u.Filter will modify the map elements (which are pointers to NV), any change will be visible to u's parent, where NV has been assigned.
	//
	if u.Filter != nil {
		u.Filter.Apply(nvm, uty.Ty, u.Name())
	}

	for _, p := range u.Select {
		//
		switch x := p.Edge.(type) {

		case *ScalarPred, *Variable: // R.p ignore, already processed

		case *UidPred:
			// NV entry contains child UIDs i.e nv[upred].Value -> [][][]byte
			var (
				nds [][][]byte
				//	aty blk.TyAttrD
				idx index
			)

			// //xxfmt.Println("uty+x.Name()  ", p, u.Name(), u.Name())
			// // get type of the uid-pred
			// if aty, ok = types.TypeC.TyAttrC[uty.Ty+":"+x.Name()]; !ok {
			// 	panic(fmt.Errorf("%s.%s not exists", uty, x.Name()))
			// 	continue // ignore this attribute as it is not in current type
			// }
			// //xxfmt.Println("aty.Ty : ", aty.Ty)
			// results not in nv for this depth in graph. Must query uids stored in nv[i].Value -> [][][]byte
			data, ok := nvm[u.Name()+":"]
			if !ok {
				// for k, v := range nvm {
				// 	fmt.Printf("nvm: %s  %#v\n", k, *v)
				// }
				panic(fmt.Errorf("%q not in NV map", x.Name()+":"))
			}
			if nds, ok = data.Value.([][][]byte); !ok {
				panic(fmt.Errorf(": data.Value is of wrong type"))
			}

			for i, k := range nds {
				for j, cUid := range k {

					if data.State[i][j] == blk.UIDdetached || data.State[i][j] == blk.EdgeFiltered {
						continue // soft delete set or failed filter condition
					}

					// grl.EndR()
					// grl.Ask()
					// <-grl.RespCh()

					wg.Add(1)
					idx = index{i, j}
					sortk := sortk_ + "#" + uty.C
					//fmt.Printf("\n>>>Uid: u.Name(): %s   %s  x.Name(): %s  sortk: %s\n", u.Name(), util.UID(cUid).String(), x.Name(), sortk)
					x.execNode(nil, wg, util.UID(cUid), uty.Ty, lvl+1, x.Name(), idx, uid_, sortk)
				}
			}
		}
	}
}
