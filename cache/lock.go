package cache

import (
	"fmt"
	"strings"
	"sync"

	blk "github.com/DynamoGraph/block"
	"github.com/DynamoGraph/db"
	slog "github.com/DynamoGraph/syslog"
	"github.com/DynamoGraph/util"
)

func (g *GraphCache) LockNode(uid util.UID) {

	fmt.Printf("** Cache LockNode  Key Value: [%s]\n", uid.String())

	g.Lock()
	uids := uid.String()
	e := g.cache[uids]

	if e == nil {
		e = &entry{ready: make(chan struct{})}
		e.NodeCache = &NodeCache{}
		g.cache[uids] = e
		g.Unlock()
		close(e.ready)
	} else {
		g.Unlock()
		<-e.ready
	}
	//
	// lock e . Note: e can only be acquired from outside of this package via the Fetch* api.
	//
	e.Lock()

}

// FetchForUpdate is used as a substitute for database lock. Provided all db access is via this API (or similar) then all updates will be serialised preventing
// mutliple concurrent updates from corrupting each other. It also guarantees consistency between cache and storage copies of the data.
// FetchForUpdate performs a Query so returns multiple items. Typically we us this to access all scalar node attribute (e.g. sortk of "A#A#")
// or all uid-pred and its propagated data (e.g. "A#:G")
func (g *GraphCache) FetchForUpdate(uid util.UID, sortk ...string) (*NodeCache, error) {
	var (
		sortk_  string
		fetched bool
	)
	//
	//	g lock protects global cache with UID key
	//
	g.Lock()
	if len(sortk) > 0 {
		sortk_ = sortk[0]
	} else {
		sortk_ = "A#"
	}

	slog.Log("FetchForUpdate: ", fmt.Sprintf("** Cache FetchForUpdate Cache Key Value: [%s]   sortk: %s", uid.String(), sortk_))
	e := g.cache[uid.String()]
	if e == nil {
		e = &entry{ready: make(chan struct{})}
		g.cache[uid.String()] = e
		g.Unlock()
		// nb: type blk.NodeBlock []*DataItem
		nb, err := db.FetchNode(uid, sortk_)
		if err != nil {
			slog.Log("FetchForUpdate: ", fmt.Sprintf("db fetchnode error: %s", err.Error()))
			return nil, err
		}
		fetched = true
		e.NodeCache = &NodeCache{m: make(map[SortKey]*blk.DataItem), gc: g}
		en := e.NodeCache
		en.Uid = uid
		for _, v := range nb {
			en.m[v.SortK] = v
		}
		close(e.ready)
	} else {
		g.Unlock()
		<-e.ready
	}
	//
	// e lock protects Node Cache with sortk key.
	// lock e to prevent updates from other routines. Must explicitly Unlock() some stage later.
	//  Note: e can only be acquired from outside of this package via the Fetch* api.
	//
	e.Lock()
	//
	// fetch will always do a db fetch - why? Because db data may have changed, as some changes go direct to db e.g. attachNode/detachNode. THIS IS WRONG.
	// in attachnode processing the cached version (uid-pred only data) is updated and then written to storage.
	// The lock mechanism does the equivalent of database locks in synchronising access to data. It also has the benefit of caching data and synchronising
	// the the cached and stored versions. In this way caching can provide the speed of access while maintaining consistency between cache and storage.
	//
	// TODO - provide an algorithm to determine if sortk data is already cached.
	if e.NodeCache != nil && !fetched {
		e.fetchSortK(sortk_)
	}
	if e.NodeCache == nil {

		slog.Log("FetchForUpdate: ", "e.NodeCache == nil. Retry FetchForUpdate")
		// cache has been cleared. Start again.
		e = nil
		g.FetchForUpdate(uid, sortk_)
	}
	e.ffuEnabled = true
	e.locked = false

	return e.NodeCache, nil
}

// FetchUIDpredForUpdate is called as part of attach node processing. It performs a GetItem on the uid-pred attribute only. The cache copy will be
// read and updated according to its contents. The cache copy will then be written to storage (API: SaveCompleteUpred*()) before associated lock is released.
// At that point the cache and storage are in sync. Accessing the cache copy is valid from that point on.
func (g *GraphCache) FetchUIDpredForUpdate(uid util.UID, sortk string) (*NodeCache, error) {
	//
	//	g lock protects global cache
	//
	g.Lock()
	slog.Log("FetchUIDpredForUpdate: ", fmt.Sprintf("Acquired g.Lock(). FetchUIDpredForUpdate Cache Key Value: [%s]   sortk: %s", uid.String(), sortk))
	e := g.cache[uid.String()]
	if e == nil {
		//
		// first time e cache is being accessed
		//
		e = &entry{ready: make(chan struct{})}
		g.cache[uid.String()] = e
		g.Unlock()
		// nb: type blk.NodeBlock []*DataItem
		nb, err := db.FetchNodeItem(uid, sortk)
		if err != nil {
			slog.Log("FetchUIDpredForUpdate: ", fmt.Sprintf("db fetchnode error: %s", err.Error()))
			return nil, err
		}
		//
		// create new NodeCache and populate with fetched blk.NodeBlock
		//
		e.NodeCache = &NodeCache{m: make(map[SortKey]*blk.DataItem), gc: g}
		en := e.NodeCache
		en.Uid = uid
		for _, v := range nb {
			en.m[v.SortK] = v
		}
		close(e.ready)
	} else {
		g.Unlock()
		<-e.ready
	}
	//
	// e lock protects NodeCache.m
	// given we are about to read and potentially update m we must first get a lock
	// note: in this case the cache is acting as a cache as well as a database lock on the node cache.
	//
	e.Lock()
	//
	// check if sortk is in node cache
	//
	if _, ok := e.m[sortk]; !ok {
		slog.Log("FetchUIDpredForUpdate: ", fmt.Sprintf("About to db.FetchNodeItem() for %s %s", uid, sortk))
		nb, err := db.FetchNodeItem(uid, sortk)
		if err != nil {
			slog.Log("FetchUIDpredForUpdate: ", fmt.Sprintf("db fetchnode error: %s", err.Error()))
			return nil, err
		}
		//
		// while in possession of lock, add sortk entry to node cache m
		//
		for _, v := range nb {
			e.m[v.SortK] = v
		}
	} else {
		slog.Log("FetchUIDpredForUpdate: ", fmt.Sprintf("uidPred is already cached. %s %s", uid, sortk))
	}
	e.ffuEnabled = true
	e.locked = false

	// e must be unlocked by app
	return e.NodeCache, nil
}

func (g *GraphCache) LockPredR(uid util.UID, sortk ...string) error {

	g.rsync.Lock()

	uids := uid.String()
	r := g.cacheR[uids]

	if r == nil {
		r = &Rentry{ready: make(chan struct{})}
		g.cacheR[uids] = r
		g.rsync.Unlock()
		close(r.ready)
	} else {
		g.rsync.Unlock()
		<-r.ready
	}

	r.Lock()

	return nil
}

func (g *GraphCache) FetchNodeExec(uid util.UID, ty string) (*NodeCache, error) {
	return nil, nil
}
func (g *GraphCache) FetchNodeExec_(uid util.UID, sortk string, ty string) (*NodeCache, error) {
	return nil, nil
}

// FetchNodeNonCache will perform a db fetch for each execution.
// Why? For testing purposes it's more realistic to access non-cached node data.
// This API is used in GQL testing.
func (g *GraphCache) FetchNodeNonCache(uid util.UID, sortk ...string) (*NodeCache, error) {
	// 	if len(sortk) > 0 {
	// 		return g.FetchNode(uid, sortk...)
	// 	}
	// 	return g.FetchNode(uid)
	// }

	var sortk_ string

	g.Lock()
	if len(sortk) > 0 {
		sortk_ = sortk[0]
	} else {
		sortk_ = "A#"
	}
	uids := uid.String()
	e := g.cache[uids]
	//
	// force db read by setting e to nil
	//
	e = nil
	//
	if e == nil {
		e = &entry{ready: make(chan struct{})}
		g.cache[uids] = e
		g.Unlock()
		// nb: type blk.NodeBlock []*DataIte
		nb, err := db.FetchNode(uid, sortk_)
		if err != nil {
			return nil, err
		}

		e.NodeCache = &NodeCache{m: make(map[SortKey]*blk.DataItem), gc: g}
		en := e.NodeCache
		en.Uid = uid
		for _, v := range nb {
			en.m[v.SortK] = v
		}
		close(e.ready)
	} else {
		g.Unlock()
		<-e.ready
	}
	//
	// lock node cache. TODO: when is it unlocked?????
	//

	return e.NodeCache, nil
}

func (g *GraphCache) FetchNode(uid util.UID, sortk ...string) (*NodeCache, error) {
	var sortk_ string

	g.Lock()
	if len(sortk) > 0 {
		sortk_ = sortk[0]
	} else {
		sortk_ = "A#"
	}
	uids := uid.String()
	e := g.cache[uids]

	if e == nil {
		e = &entry{ready: make(chan struct{})}
		g.cache[uids] = e
		g.Unlock()
		// nb: type blk.NodeBlock []*DataIte
		nb, err := db.FetchNode(uid, sortk_)
		if err != nil {
			return nil, err
		}
		e.NodeCache = &NodeCache{m: make(map[SortKey]*blk.DataItem), gc: g}
		en := e.NodeCache
		en.Uid = uid
		for _, v := range nb {
			en.m[v.SortK] = v
		}
		close(e.ready)
	} else {
		g.Unlock()
		<-e.ready
	}
	//
	// lock node cache.
	//
	e.RLock()
	//e.Lock()
	// if e.NodeCache == nil {
	// 	// cache has been cleared. Start again.
	// 	e = nil
	// 	g.FetchNode(uid, sortk_)
	// }
	e.locked = true // TODO - this cannot be done under a read lock
	e.ffuEnabled = false
	var cached bool
	// check sortk is cached
	for k := range e.m {
		if strings.HasPrefix(k, sortk_) {
			cached = true
			break
		}
	}
	if !cached {
		e.Unlock()
		e.fetchSortK(sortk_)
		return e.NodeCache, nil
	}

	e.Unlock()
	return e.NodeCache, nil
}

func (g *GraphCache) FetchUOB(uid util.UID, wg *sync.WaitGroup, ncCh chan<- *NodeCache) {
	var sortk_ string

	g.Lock()

	sortk_ = "A#"

	uids := uid.String()
	e := g.cache[uids]

	if e == nil {
		e = &entry{ready: make(chan struct{})}
		g.cache[uids] = e
		g.Unlock()
		// nb: type blk.NodeBlock []*DataIte
		nb, err := db.FetchNode(uid, sortk_)
		if err != nil {
			return
		}
		e.NodeCache = &NodeCache{m: make(map[SortKey]*blk.DataItem), gc: g}
		en := e.NodeCache
		en.Uid = uid
		for _, v := range nb {
			en.m[v.SortK] = v
		}
		close(e.ready)
	} else {
		g.Unlock()
		<-e.ready
	}
	//
	// lock node cache.
	//
	e.RLock()
	//e.Lock()
	// if e.NodeCache == nil {
	// 	// cache has been cleared. Start again.
	// 	e = nil
	// 	g.FetchNode(uid, sortk_)
	// }
	e.locked = true // TODO - this cannot be done under a read lock
	e.ffuEnabled = false
	var cached bool
	// check sortk is cached
	for k := range e.m {
		if strings.HasPrefix(k, sortk_) {
			cached = true
			break
		}
	}
	if !cached {
		e.Unlock()
		e.fetchSortK(sortk_)
		ncCh <- e.NodeCache
		return
	}

	e.Unlock()

	ncCh <- e.NodeCache
}

func (nc *NodeCache) fetchSortK(sortk string) error {

	slog.Log("fetchSortK: ", fmt.Sprintf("fetchSortK for %s UID: [%s] \n", sortk, nc.Uid.String()))
	nb, err := db.FetchNode(nc.Uid, sortk)
	if err != nil {
		return err
	}
	// add data items to node cache
	for _, v := range nb {
		nc.m[v.SortK] = v
	}

	return nil
}

func (g *GraphCache) LockAndClearNodeCache(uid util.UID) *entry {

	fmt.Println()
	fmt.Println("================================ LOCK and CLEAR NODE CACHE =======================================")
	fmt.Printf(" Clear node cache for: %s", uid.String())
	fmt.Println()
	//
	// check if node is cached
	//
	var (
		//		ok bool
		e *entry
	)
	uids := uid.String()
	fmt.Println("Acquire gLock")
	g.Lock()
	fmt.Println("Acquired gLock")
	//	if e, ok = g.cache[uids]; !ok {
	e = &entry{ready: make(chan struct{})}
	e.NodeCache = &NodeCache{gc: g}
	g.cache[uids] = e
	g.Unlock()
	fmt.Println("REleased gLock")
	// 	close(e.ready)
	// } else {
	// 	fmt.Println("RElease gLock")
	// 	g.Unlock()
	// 	fmt.Printf("REleased gLock e is %#v\n ", e)
	// 	<-e.ready
	// }
	//
	// lock node and then clear cache
	//
	fmt.Println("Acquire eLock")
	e.Lock()
	fmt.Println("Acquired eLock")
	fmt.Printf("== EXIT LOCK and CLEAR NODE CACHE == %#v\n", e)
	return e
}

// UnlockNode used for blocks that are not cached.
func (e *entry) UnlockNode() error {

	// var (
	// 	e  *entry
	// 	ok bool
	// )
	fmt.Printf("** Cache UnlockNode  K\n")
	//g := e.gc
	// g.Lock()
	// uids := uid.String()
	// g.Unlock()
	// if e, ok = g.cache[uids]; !ok {
	// 	return fmt.Errorf("No lock on %q", uids)
	// }
	if e == nil {
		return fmt.Errorf("e is nil for ")
	}
	fmt.Println("Unlock e")
	if e.NodeCache == nil {
		fmt.Println("UnlockNode: for e.NodeCache == nul.......")
	}
	e.Unlock("from e UnlockNode")
	fmt.Println("Unlocked e")

	return nil
}

func (n *NodeCache) ClearCache(sortk string, subs ...bool) error {

	slog.Log("ClearCache: ", fmt.Sprintf("sortk %s", sortk))
	//
	// check if node is cached
	//
	delete(n.m, sortk)
	// if clear all sub-sortk's
	if len(subs) > 0 && subs[0] {
		for k := range n.m {
			if strings.HasPrefix(k, sortk) {
				delete(n.m, k)
			}
		}
	}

	return nil
}

func (g *GraphCache) ClearNodeCache(uid util.UID) error {

	fmt.Println()
	fmt.Println("================================ CLEAR NODE CACHE =======================================")
	fmt.Printf(" Clear node cache for: %s", uid.String())
	fmt.Println()
	//
	// check if node is cached
	//
	var (
		// ty  string
		// tab blk.TyAttrBlock
		ok bool
		e  *entry
	)
	g.Lock()
	if e, ok = g.cache[uid.String()]; !ok {
		fmt.Println("Nothing to clear")
		g.Unlock()
		return nil
	}
	g.Unlock()
	//
	// lock node
	//
	fmt.Println("ClearNodeCache: FetchForUpdate")
	nc, err := g.FetchForUpdate(uid)
	defer nc.Unlock("set by FetchforUPdate in Clear node cache ")
	if err != nil {
		return err
	}
	//
	// remove any overflow blocks
	//
	// get type definition and list its uid-predicates (e.g. siblings, friends)
	// if ty, ok = nc.GetType(); !ok {
	// 	return NoNodeTypeDefinedErr
	// }
	// if tab, err = FetchType(ty); err != nil {
	// 	return err
	// }

	// for _, c := range tab.GetUIDpredC() {
	// 	sortk := "A#G#:" + c
	// 	// get sortk's overflow Block UIDs if any
	// 	for _, uid_ := range nc.GetOvflUIDs(sortk) {
	// 		if _, ok = g.cache[uid_.String()]; ok {
	// 			// delete map entry will mean e is unassigned and allow GC to purge e and associated node cache.
	// 			g.Lock()
	// 			delete(g.cache, uid_.String())
	// 			g.Unlock()
	// 		}
	// 	}
	// }
	//
	// clear NodeCache forcing any waiting readers on uid node to refresh from db
	//
	e.NodeCache = nil
	//
	// remove node from g cache and
	//
	g.Lock()
	delete(g.cache, uid.String())
	g.Unlock()
	//
	fmt.Println("==Clear cache finished ==")
	return nil
}

// Unlock method shadows the RWMutex Unlock
func (nd *NodeCache) Unlock(s ...string) {
	if len(s) > 0 {
		slog.Log("Unlock: ", fmt.Sprintf("******* IN UNLOCK NC ********************  %s", s[0]))
	} else {
		slog.Log("Unlock: ", "******* IN UNLOCK NC ********************")
	}
	if nd == nil {
		return
	}
	if nd.m != nil && len(nd.m) == 0 {
		// locked by LockNode() - without caching daa
		slog.Log("Unlock: ", "Success RWMutex.Unlock() len(nd.m)=0")
		nd.RWMutex.Unlock()
		return
	}
	if nd.ffuEnabled {

		nd.RWMutex.Unlock()
		//nd.Unlock()
		slog.Log("Unlock: ", "Success Unlock()")
		nd.ffuEnabled = false

	} else if nd.locked {

		//	nd.RUnlock()
		nd.RWMutex.RUnlock()
		//slog.Log("Unlock: ", "Success RUnlock()")
		slog.Log("Unlock: ", "Success Unlock()")
		nd.locked = false
	} else {
		slog.Log("Unlock: ", "Error: Nothing unlocked ")
	}
	// locked by LockAndClearNodeCache or LockNode
	//nd.RWMutex.Unlock()
	//	fmt.Println("Exit UNLOCK NC..")
}

// // ClearOverflowCache: remove overflow blocks from the cache
// func (g *GraphCache) ClearOverflowCache(uid util.UID) error {

// 	fmt.Println()
// 	fmt.Println("================================ CLEAR NODE CACHE =======================================")
// 	fmt.Printf(" Clear any Overflow caches for: %s", uid.String())
// 	fmt.Println()
// 	//
// 	// check if node is cached
// 	//
// 	var (
// 		ty  string
// 		tab blk.TyAttrBlock
// 		ok  bool
// 	)
// 	g.Lock()
// 	if _, ok := g.cache[uid.String()]; !ok {
// 		fmt.Println("Nothing to clear")
// 		g.Unlock()
// 		return nil
// 	}
// 	g.Unlock()
// 	//
// 	// lock node
// 	//
// 	nc, err := g.FetchForUpdate(uid)
// 	defer nc.Unlock()
// 	if err != nil {
// 		return err
// 	}
// 	//
// 	// remove any overflow blocks
// 	//
// 	// get type definition and list its uid-predicates (e.g. siblings, friends)
// 	if ty, ok = nc.GetType(); !ok {
// 		return NoNodeTypeDefinedErr
// 	}
// 	if tab, err = FetchType(ty); err != nil {
// 		return err
// 	}

// 	for _, c := range tab.GetUIDpredC() {
// 		sortk := "A#G#:" + c
// 		// get sortk's overflow Block UIDs if any
// 		for _, uid_ := range nc.GetOvflUIDs(sortk) {
// 			g.Lock()
// 			if _, ok = g.cache[uid_.String()]; ok {
// 				delete(g.cache, uid_.String())
// 				g.Unlock()
// 			} else {
// 				g.Unlock()
// 			}
// 		}
// 	}

// 	return nil
// }
