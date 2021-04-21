package cache

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	blk "github.com/DynamoGraph/block"
	"github.com/DynamoGraph/db"
	"github.com/DynamoGraph/ds"
	param "github.com/DynamoGraph/dygparam"
	slog "github.com/DynamoGraph/syslog"
	"github.com/DynamoGraph/types"
	"github.com/DynamoGraph/util"

	"github.com/DynamoGraph/rdf/grmgr"
)

func syslog(s string) {
	slog.Log("Cache: ", s)
}

// errors
var ErrCacheEmpty = errors.New("Cache is empty")

//  ItemCache struct is the transition between Dynamodb types and the actual attribute type defined in the DD.
//  Number (dynamodb type) -> float64 (transition) -> int (internal app & as defined in DD)
//  process: dynamodb -> ItemCache -> DD conversion if necessary to application variables -> ItemCache -> Dynamodb
//	types that require conversion from ItemCache to internal are:
//   DD:   int         conversion: float64 -> int
//   DD:   datetime    conversion: string -> time.Time
//  all the other datatypes do not need to be converted.

type SortKey = string

// type NodeCache struct {
// 	m map[SortKey]*blk.DataItem
// 	sync.Mutex
// }

//type block map[SortKey]*blk.DataItem

// ************************************ Node cache ********************************************

// data associated with a single node
type NodeCache struct {
	sync.RWMutex // used for querying the cache data items. Promoted methods RLock(), Unlock()
	m            map[SortKey]*blk.DataItem
	Uid          util.UID
	ffuEnabled   bool // true for fetch-for-update operations
	locked       bool
	gc           *GraphCache // point back to graph-cache
}

type entry struct {
	ready chan struct{} // a channel for each entry - to synchronise access when the data is being sourced
	*NodeCache
}
type Rentry struct {
	ready chan struct{} // a channel for each entry - to synchronise access when the data is being sourced
	sync.RWMutex
}

// graph cache consisting of all nodes loaded into memory
type GraphCache struct {
	sync.RWMutex
	cache  map[util.UIDb64s]*entry
	rsync  sync.RWMutex
	cacheR map[util.UIDb64s]*Rentry // not used?
}

var GraphC GraphCache

func NewCache() *GraphCache {
	return &GraphC
}

func GetCache() *GraphCache {
	return &GraphC
}

func (n *NodeCache) GetMap() map[SortKey]*blk.DataItem {
	return n.m
}

// ====================================== init =====================================================

func init() {
	// cache of nodes
	GraphC = GraphCache{cache: make(map[util.UIDb64s]*entry)}
	//
	//FacetC = make(map[types.TyAttr][]FacetTy)
}

func (g *GraphCache) IsCached(uid util.UID) (ok bool) {
	g.Lock()
	_, ok = g.cache[uid.String()]
	g.Unlock()
	return ok
}

func (np *NodeCache) GetOvflUIDs(sortk string) []util.UID {
	// TODO: replace A#G#:S" with generic term
	// get np.uidPreds
	if di, ok := np.m[sortk]; ok { // np.GetDataItem("A#G#:S"); ok {
		_, _, ofUID := di.GetNd()
		ids := len(ofUID)
		if ids > 0 {
			m := make([]util.UID, ids, ids)
			for i, v := range ofUID {
				m[i] = util.UID(v)
			}
			return m
		}
	}
	return nil
}

func (n *NodeCache) GetDataItem(sortk string) (*blk.DataItem, bool) {
	if x, ok := n.m[sortk]; ok {
		return x, ok
	}
	return nil, false
}

// // UpdatePropagationBlock func associated with Event processing
// func UpdatePropagationBlock(sortK string, pUID, cUID, targetUID util.UID, state byte) error {
// 	gc := NewCache()
// 	nd, err := gc.FetchForUpdate(pUID)
// 	if err != nil {

// 	}
// 	err = nd.UpdatePropagationBlock(sortK, pUID, cUID, targetUID, state)
// 	nd.Unlock()
// 	return err

// }

// SetUpredAvailable called from client as part of AttachNode operation SetUpredState
// targetUID is the propagation block that contains the child scalar data.
// id - overflow block id
// cnt - increment counter by 0 (if errored) or 1 (if node attachment successful)
// ty - type of the  parent
func (nc *NodeCache) SetUpredAvailable(sortK string, pUID, cUID, targetUID util.UID, id int, cnt int, ty string) error {
	var (
		attachAttrNm string
		found        bool
		err          error
	)
	syslog(fmt.Sprintf("In SetUpredAvailable:  pUid, tUID:  %s %s %s", pUID, targetUID, sortK))
	//
	// TyAttrC populated in NodeAttach(). Get Name of attribute that is the attachment point, based on sortk
	//
	i := strings.IndexByte(sortK, ':')
	fmt.Println("SetUpredAvailable, ty, attachpoint, sortK ", ty, sortK[i+1:], sortK, len(types.TypeC.TyC[ty]))
	// find attribute name of parent attach predicate
	for _, v := range types.TypeC.TyC[ty] {
		//	fmt.Println("SetUpredAvailable, k,v ", k, v.C, sortK[i+1:], sortK)
		if v.C == sortK[i+1:] {
			attachAttrNm = v.Name
			found = true
			break
		}
	}
	if !found {
		panic(fmt.Errorf(fmt.Sprintf("Error in SetUpredAvailable. Attach point attribute not found in type map for sortk %q", sortK)))
	}
	//
	// get type short name
	//
	tyShortNm, ok := types.GetTyShortNm(ty)
	if !ok {
		panic(fmt.Errorf("SetUpredAvailable: type not found in  types.GetTyShortNm"))
	}
	// cache: update pUID block with latest propagation state; targetUID, XF state
	//
	di := nc.m[sortK]

	found = false
	// if target UID is the parent node UID
	if bytes.Equal(pUID, targetUID) {
		// target is current parent UID block
		// search from most recent (end of slice)
		for i := len(di.Nd); i > 0; i-- {
			if bytes.Equal(di.Nd[i-1], cUID) {
				di.XF[i-1] = blk.ChildUID
				fmt.Println("SetUpredAvailable: about to db.SvaeUpredState()...")
				err = db.SaveUpredState(di, cUID, blk.ChildUID, i-1, cnt, attachAttrNm, tyShortNm)
				if err != nil {
					return err
				}
				found = true
				break
			}
		}
	} else {
		// target is an Overflow block
		// search from most recent (end of slice)
		for i := len(di.Nd); i > 0; i-- {
			if bytes.Equal(di.Nd[i-1], targetUID) {
				di.XF[i-1] = blk.OvflBlockUID
				di.Id[i-1] = id
				fmt.Println("SetUpredAvailable: about to db.SvaeUpredState()...")
				err = db.SaveUpredState(di, targetUID, blk.OvflBlockUID, i-1, cnt, attachAttrNm, tyShortNm)
				if err != nil {
					return err
				}
				found = true
				break
			}
		}
	}
	if !found {
		fmt.Println("SetUpredAvailable: Failed ")
		return fmt.Errorf("AttachNode: target UID not found in Nd attribute of parent node")
	}
	fmt.Println("SetUpredAvailable: Succeeded")
	return nil
}

var NoNodeTypeDefinedErr error = errors.New("No type defined in node data")

type NoTypeDefined struct {
	ty string
}

func (e NoTypeDefined) Error() string {
	return fmt.Sprintf("Type %q not defined", e.ty)
}

// func NewNoTypeDefined(ty string) error {
// 	return NoTypeDefined{ty: ty}
// }

// genSortK, generate one or more SortK given NV.
func GenSortK(nvc ds.ClientNV, ty string) []string {
	//genSortK := func(attr string) (string, bool) {
	var (
		ok                    bool
		sortkS                []string
		aty                   blk.TyAttrD
		scalarPreds, uidPreds int
	)
	//
	// count predicates, scalar & uid.
	// ":" used to identify uid-preds
	//
	if len(ty) == 0 {
		panic(fmt.Errorf("Error in GenSortK: argument ty is empty"))
	}
	for _, nv := range nvc {
		if strings.IndexByte(nv.Name, ':') == -1 {
			scalarPreds++
		} else {
			uidPreds++
		}
	}
	//
	// get type info
	//
	// if tyc, ok :=  types.TypeC.TyC[ty]; !ok {
	// 	panic(fmt.Errorf(`genSortK: Type %q does not exist`, ty))
	// }
	// get long type name
	ty, _ = types.GetTyLongNm(ty)
	var s strings.Builder

	switch {

	case uidPreds == 0 && scalarPreds == 1:
		s.WriteString("A#")
		if aty, ok = types.TypeC.TyAttrC[ty+":"+nvc[0].Name]; !ok {
			panic(fmt.Errorf("Predicate %q does not exist in type %q", nvc[0].Name, ty))
		} else {
			s.WriteString(aty.P)
			s.WriteString("#:")
			s.WriteString(aty.C)
		}

	case uidPreds == 0 && scalarPreds > 1:
		// get partitions involved
		var parts map[string]bool

		parts = make(map[string]bool)
		for i, nv := range nvc {
			if aty, ok = types.TypeC.TyAttrC[ty+":"+nv.Name]; !ok {
				panic(fmt.Errorf("Predicate %q does not exist in type %q", nvc[i].Name, ty))
			} else {
				if !parts[aty.P] {
					parts[aty.P] = true
				}
			}
		}
		for k, _ := range parts {
			s.WriteString("A#")
			s.WriteString(k)
			sortkS = append(sortkS, s.String())
			s.Reset()
		}

	case uidPreds == 1 && scalarPreds == 0:
		s.WriteString("A#")
		if aty, ok = types.TypeC.TyAttrC[ty+":"+nvc[0].Name]; !ok {
			panic(fmt.Errorf("Predicate %q does not exist in type %q", nvc[0].Name, ty))
		} else {
			s.WriteString("G#:")
			s.WriteString(aty.C)
		}

	case uidPreds == 1 && scalarPreds > 0:
		s.WriteString("A#")
		// all items

	case uidPreds > 1 && scalarPreds == 0:
		s.WriteString("A#G#")

	default:
		// case uidPreds > 1 && scalarPReds > 0:
		s.WriteString("A#")
	}
	//
	if len(sortkS) == 0 {
		sortkS = append(sortkS, s.String())
	}
	return sortkS
}

func (nc *NodeCache) UnmarshalCache(nv ds.ClientNV) error {
	return nc.UnmarshalNodeCache(nv)
}

// UnmarshalCache, unmarshalls the nodecache into the ds.ClientNV
// currently it presumes all propagated scalar data must be prefixed with A#.
// TODO: extend to include G# prefix.
// nc must have been acquired using either
// * FetchForUpdaate(uid)
// * FetchNode(uid)
//
// Type differences between query and data.
// ----------------------------------------
// NV is generated from the query statement which is usually based around around a known type.
// Consequently, NV.Name is based the predicates in the known type.
// However the results from the root query don't necessarily have to match the type used to define the query.
// When the types differ only those predicates that match (based on predicate name - NV.Name) can be unmarshalled.
// ty_ should be the type of the item resulting from the root query which will necessarily match the type from the item cache.
// If ty_ is not passed then the type is sourced from the cache, at the potental cost of a read, so its better to pass the type if known
// which should always be the case.
//
func (nc *NodeCache) UnmarshalNodeCache(nv ds.ClientNV, ty_ ...string) error {
	if nc == nil {
		return ErrCacheEmpty
	}
	var (
		sortk  string
		attrDT string
		ok     bool
		// sortk2  string
		// attrDT2 string
		// ok2     bool
		attrKey string
		ty      string // short name for item type e.g. Pn (for Person)

		err error
	)

	// for k := range nc.m {
	// 	fmt.Println(" UnmarshalNodeCache  key: ", k)
	// }
	if len(ty_) > 0 {
		ty = ty_[0]
	} else {
		if ty, ok = nc.GetType(); !ok {
			return NoNodeTypeDefinedErr
		}
		fmt.Println("ty 2= ", ty)
	}
	// if ty is short name convert to long name
	if x, ok := types.GetTyLongNm(ty); ok {
		ty = x
	}
	// current Type (long name)
	//fmt.Println("UnmarshalNodeCache  ty: ", ty)
	// types.FetchType populates  struct cache.TypeC with map types TyAttr, TyC
	if _, err = types.FetchType(ty); err != nil {
		return err
	}

	genSortK := func(attr string) (string, string, bool) {
		var (
			pd     strings.Builder
			aty    blk.TyAttrD
			attrDT string
			ok     bool
		)
		// Scalar attribute
		attr_ := strings.Split(attr, ":")
		ty := ty //cTys[0]
		pd.WriteString("A#")
		c := 1
		colons := strings.Count(attr, ":")
		for _, j := range attr_ {
			if len(j) == 0 {
				break
			}
			if aty, ok = types.TypeC.TyAttrC[ty+":"+j]; !ok {
				return "", "", false
			}
			attrDT = aty.DT
			// uid-predicates:
			// two promotes child.updpred:scalar
			// two promotes child.updpred:grandchild.uidpred:scalar
			switch colons {
			case 0:
				// scalar
				pd.WriteString(aty.P)
				pd.WriteString("#:")
				pd.WriteString(aty.C)
			case 1:
				if aty.DT != "Nd" {
					attrDT = "UL" + aty.DT
				}
				// single promote of scalar
				// scalar only
				switch c {
				case 1:
					pd.WriteString("G#:")
					pd.WriteString(aty.C)
					c++
				case 2:
					pd.WriteString("#:")
					pd.WriteString(aty.C)
				}
			case 2:
				// double promote of scalars
				// uid-preds only
				if aty.DT != "Nd" {
					attrDT = "UL" + aty.DT
				}
				switch c {
				case 1:
					pd.WriteString("G#:")
					pd.WriteString(aty.C)
					c++
				case 2:
					pd.WriteString("#G#:")
					pd.WriteString(aty.C)
					c++
				case 3:
					pd.WriteString("#:")
					pd.WriteString(aty.C)
				}
			}
			if len(aty.Ty) > 0 {
				// change current type
				ty = aty.Ty
			}

		}
		return pd.String(), attrDT, true
	}
	// This data is stored in uid-pred UID item that needs to be assigned to each child data item
	var State [][]int
	var OfUIDs [][]byte

	sortK := func(key string, i int) string {
		var s strings.Builder
		s.WriteString(key)
		s.WriteByte('#')
		s.WriteString(strconv.Itoa(i)) // batch Id 1..n
		return s.String()
	}
	// &ds.NV{Name: "Age"},
	// &ds.NV{Name: "Name"},
	// &ds.NV{Name: "DOB"},
	// &ds.NV{Name: "Cars"},
	// &ds.NV{Name: "Siblings"},    <== important to define Nd type before refering to its attributes
	// &ds.NV{Name: "Siblings:Name"},
	// &ds.NV{Name: "Siblings:Age"},
	for _, a := range nv { // a.Name = "Age"
		//
		// field name repesents a scalar. It has a type that we use to generate a sortk <partition>#G#:<uid-pred>#:<scalarpred-type-abreviation>
		//
		//sortk2, attrDT2, ok2 = genSortK2(a.Name)
		// no match between NV name and type attribute name
		if sortk, attrDT, ok = genSortK(a.Name); !ok {
			// no match between NV name and type attribute name
			continue
		}
		//fmt.Println("UnmarshalNodeCache: a.Name, sortk, attrDT: ", a.Name, sortk, attrDT) //, sortk2, attrDT2, ok2)
		//
		// grab the *blk.DataItem from the cache for the nominated sortk.
		// we could query the child node to get this data or query the #G data which is its copy of the data
		//
		a.ItemTy = ty
		attrKey = sortk
		//
		if v, ok := nc.m[sortk]; ok {
			// based on data type and whether its a node or uid-pred
			switch attrDT {

			// Scalars
			case "I": // int
				a.Value = v.GetI()
			case "F": // float
				a.Value = v.GetF()
			case "S": // string
				a.Value = v.GetS()
			case "Bl": // bool
				a.Value = v.GetBl()
			case "DT": // DateTime - stored as string
				a.Value = v.GetDT()

			// Sets
			case "IS": // set int
				a.Value = v.GetIS()
			case "FS": // set float
				a.Value = v.GetFS()
			case "SS": // set string
				a.Value = v.GetSS()
			case "BS": // set binary
				a.Value = v.GetBS()

			// Lists
			case "LS": // list string
				a.Value = v.GetLS()
			case "LF": // list float
				a.Value = v.GetLF()
			case "LI": // list int
				a.Value = v.GetLI()
			case "LB": // List []byte
				a.Value = v.GetLB()
			case "LBl": // List bool
				a.Value = v.GetLBl()

			// Propagated Scalars
			case "ULS": // list string
				//a.Value = v.GetLBl()
				var allLS [][]string
				var allXbl [][]bool
				// data from root uid-pred block
				ls, xf := v.GetULS()

				allLS = append(allLS, ls[1:])
				allXbl = append(allXbl, xf[1:])
				// data from overflow blocks

				for _, v := range OfUIDs {
					nuid, err := nc.gc.FetchNode(util.UID(v))
					if err != nil {
						return err
					}
					// iterate over all overflow items in the overflow block for key attrKey
					for i := 1; true; i++ {
						if di, ok := nuid.m[sortK(attrKey, i)]; !ok {
							break //return fmt.Errorf("UnmarshalCache: SortK %q does not exist in cache", attrKey)
						} else {
							ls, xbl := di.GetULS()
							allLS = append(allLS, ls[1:])
							allXbl = append(allXbl, xbl[1:])
						}
					}
				}
				a.Value = allLS
				a.Null = allXbl
				a.State = State
				a.OfUIDs = OfUIDs

			case "ULF": // list float
				//a.Value = v.GetLBl()
				var allLF [][]float64
				var allXbl [][]bool
				// data from root uid-pred block
				lf, xf := v.GetULF()

				allLF = append(allLF, lf[1:])
				allXbl = append(allXbl, xf[1:])
				// data from overflow blocks
				for _, v := range OfUIDs {
					nuid, err := nc.gc.FetchNode(util.UID(v))
					if err != nil {
						return err
					}
					// iterate over all overflow items in the overflow block for key attrKey
					for i := 1; true; i++ {
						if di, ok := nuid.m[sortK(attrKey, i)]; !ok {
							break //return fmt.Errorf("UnmarshalCache: SortK %q does not exist in cache", attrKey)
						} else {
							lf, xbl := di.GetULF()
							allLF = append(allLF, lf[1:])
							allXbl = append(allXbl, xbl[1:])
						}
					}
				}
				a.Value = allLF
				a.Null = allXbl
				a.State = State
				a.OfUIDs = OfUIDs

			case "ULI": // list int

				var allLI [][]int64
				var allXbl [][]bool
				// data from root uid-pred block
				li, xf := v.GetULI()

				allLI = append(allLI, li[1:])
				allXbl = append(allXbl, xf[1:])
				// data from overflow blocks
				for _, v := range OfUIDs {
					nuid, err := nc.gc.FetchNode(util.UID(v))
					if err != nil {
						return err
					}
					// iterate over all overflow items in the overflow block for key attrKey
					for i := 1; true; i++ {
						if di, ok := nuid.m[sortK(attrKey, i)]; !ok {
							break //return fmt.Errorf("UnmarshalCache: SortK %q does not exist in cache", attrKey)
						} else {
							li, xbl := di.GetULI()
							allLI = append(allLI, li[1:])
							allXbl = append(allXbl, xbl[1:])
						}
					}
				}
				a.Value = allLI
				a.Null = allXbl
				a.State = State
				a.OfUIDs = OfUIDs

			case "ULB": // List []byte

				var allLB [][][]byte
				var allXbl [][]bool
				// data from root uid-pred block
				lb, xf := v.GetULB()

				allLB = append(allLB, lb[1:])
				allXbl = append(allXbl, xf[1:])
				// data from overflow blocks
				for _, v := range OfUIDs {
					nuid, err := nc.gc.FetchNode(util.UID(v))
					if err != nil {
						return err
					}
					for i := 1; true; i++ {
						if di, ok := nuid.m[sortK(attrKey, i)]; !ok {
							break //return fmt.Errorf("UnmarshalCache: SortK %q does not exist in cache", attrKey)
						} else {
							lb, xbl := di.GetULB()
							allLB = append(allLB, lb[1:])
							allXbl = append(allXbl, xbl[1:])
						}
					}
				}
				a.Value = allLB
				a.Null = allXbl
				a.State = State
				a.OfUIDs = OfUIDs

			case "ULBl": // List bool
				//a.Value = v.GetLBl()
				var allLBl [][]bool
				var allXbl [][]bool
				// data from root uid-pred block
				bl, xf := v.GetULBl()

				allLBl = append(allLBl, bl[1:])
				allXbl = append(allXbl, xf[1:])
				// data from overflow blocks
				for _, v := range OfUIDs {
					nuid, err := nc.gc.FetchNode(util.UID(v))
					if err != nil {
						return err
					}
					for i := 1; true; i++ {
						if di, ok := nuid.m[sortK(attrKey, i)]; !ok {
							break //return fmt.Errorf("UnmarshalCache: SortK %q does not exist in cache", attrKey)
						} else {
							bl, xbl := di.GetULBl()
							allLBl = append(allLBl, bl[1:])
							allXbl = append(allXbl, xbl[1:])
						}
					}
				}
				a.Value = allLBl
				a.Null = allXbl
				a.State = State
				a.OfUIDs = OfUIDs

			case "Nd": // uid-pred cUID+XF data
				var (
					allcuid [][][]byte
					xfall   [][]int
					//
					wg      sync.WaitGroup
					ncCh    chan *NodeCache
					limiter *grmgr.Limiter
				)

				cuid, xf, ofuids := v.GetNd()
				// share ofuids amoungst all propgatated data types
				if len(ofuids) > 0 {
					OfUIDs = ofuids[1:] // ignore dummy entry: TODO: check this is appropriate??
					// setup concurrent reads of UUID batches
					limiter = grmgr.New("Of", 6)
				} else {
					OfUIDs = ofuids
				}
				allcuid = append(allcuid, cuid[1:]) // ignore dummy entry
				xfall = append(xfall, xf[1:])       // ignore dummy entry

				// data from overflow blocks
				if len(OfUIDs) > 0 {

					// read overflow blocks concurrently
					go func() {

						for _, v := range OfUIDs {

							limiter.Ask()
							<-limiter.RespCh()

							wg.Add(1)
							go nc.gc.FetchUOB(util.UID(v), &wg, ncCh)

						}
						wg.Wait()
						close(ncCh) // End-of-UOBs
					}()

					// read child node UUIDs from channel
					for nuid := range ncCh {

						if err != nil {
							return err
						}
						for i := 1; true; i++ {
							if di, ok := nuid.m[sortK(attrKey, i)]; !ok {
								break // no more UUID batches
							} else {
								uof, xof := di.GetOfNd()
								// check if target item is populated. Note: #G#:S#1 will always contain atleast one cUID but #G#:S#2 may not contain any.
								// this occurs as UID item target is created as item id is incremented but associated scalar data target items are created on demand.
								// so a UID target item may exist without any associated scalar data targets. Each scalar data target items will always contain data associated with each cUID attached to parent.
								if len(uof) > 0 {
									allcuid = append(allcuid, uof[1:]) // ignore first entry
									xfall = append(xfall, xof[1:])     // ignore first entry
								}
							}
						}
					}
				}

				a.Value = allcuid
				a.State = xfall
				// share state amongst all propgated datat type
				State = xfall

			default:
				panic(fmt.Errorf("Unsupported data type %q", attrDT))
			}
		}
	}

	return nil

}

func (d *NodeCache) UnmarshalValue(attr string, i interface{}) error {
	if d == nil {
		return ErrCacheEmpty
	}
	var (
		aty blk.TyAttrD
		ty  string
		ok  bool
	)
	defer d.Unlock()

	if reflect.ValueOf(i).Kind() != reflect.Ptr {
		panic(fmt.Errorf("passed in value must be a pointer"))
	}

	if ty, ok = d.GetType(); !ok {
		return NoNodeTypeDefinedErr
	}
	if _, err := types.FetchType(ty); err != nil {
		return err
	}

	if aty, ok = types.TypeC.TyAttrC[ty+":"+attr]; !ok {
		panic(fmt.Errorf("Attribute %q not found in type %q", attr, ty))
	}
	// build a item clause
	var pd strings.Builder
	// pd.WriteString(aty.P) // item partition
	// pd.WriteByte('#')
	pd.WriteString("A#:") // scalar data
	pd.WriteString(aty.C) // attribute compressed identifier

	for _, v := range d.m {
		// match attribute descriptor
		if v.SortK == pd.String() {
			// we now know the attribute data type, populate interface value with attribute data
			switch aty.DT {
			case "I":
				if reflect.ValueOf(i).Elem().Kind() != reflect.Int {
					return fmt.Errorf("Input type does not match data type")
				}
				reflect.ValueOf(i).Elem().SetInt(v.GetI())
				//
				// non-reflect version below - does not work as fails to set i to value
				// must return i to work. So reflect is more elegant solution as it does an inplace set.
				// if _,ok := i.(*int); !ok {
				// 	return fmt.Errorf("Input type does not match data type")
				// } // or
				// switch i.(type) {
				// case *int, *int64:
				// default:
				// 	return fmt.Errorf("Input type does not match data type")
				// }
				// ii := v.GetI()
				// fmt.Println("Age: ", ii)
				// i = &ii
				return nil
			default:
				return fmt.Errorf("Input type does not match data type")
			}

		}
	}
	return fmt.Errorf("%s not found in data", attr)

}

// UnmarshalMap is an exmaple of reflect usage. Not used in main program.
func (d *NodeCache) UnmarshalMap(i interface{}) error {
	if d == nil {
		return ErrCacheEmpty
	}
	defer d.Unlock()

	if !(reflect.ValueOf(i).Kind() == reflect.Ptr && reflect.ValueOf(i).Elem().Kind() == reflect.Struct) {
		return fmt.Errorf("passed in value must be a pointer to struct")
	}
	var (
		ty string
		ok bool
	)
	if ty, ok = d.GetType(); !ok {
		return NoNodeTypeDefinedErr
	}
	if _, err := types.FetchType(ty); err != nil {
		return err
	}

	if ty, ok = d.GetType(); !ok {
		return NoNodeTypeDefinedErr
	}
	if _, err := types.FetchType(ty); err != nil {
		return err
	}

	var (
		aty blk.TyAttrD
	)

	genAttrKey := func(attr string) string {
		if aty, ok = types.TypeC.TyAttrC[ty+":"+attr]; !ok {
			return ""
		}
		// build a item clause
		var pd strings.Builder
		//pd.WriteString(aty.P) // item partition
		pd.WriteString("A#:")
		pd.WriteString(aty.C) // attribute compressed identifier
		return pd.String()
	}

	typeOf := reflect.TypeOf(i).Elem()
	valueOf := reflect.ValueOf(i).Elem()
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		valueField := valueOf.Field(i)
		// field name should match an attribute identifer
		attrKey := genAttrKey(field.Name)
		if attrKey == "" {
			continue
		}
		for _, v := range d.m {
			// match attribute descriptor
			if v.SortK == attrKey {
				//fmt.Printf("v = %#v\n", v.SortK)
				// we now know the attribute data type, populate interface value with attribute data
				switch x := aty.DT; x {
				case "I": // int
					valueField.SetInt(v.GetI())
				case "F": // float
					valueField.SetFloat(v.GetF())
				case "S": // string
					valueField.SetString(v.GetS())
				case "Bl": // bool
					valueField.SetBool(v.GetBl())
				// case "DT": // bool
				// 	valueField.SetString(v.GetDT())
				// case "B": // binary []byte
				// 	valueField.SetBool(v.GetB())
				case "LS": // list string
					valueOf_ := reflect.ValueOf(v.GetLS())
					newSlice := reflect.MakeSlice(field.Type, 0, 0)
					valueField.Set(reflect.AppendSlice(newSlice, valueOf_))
				case "LF": // list float
					valueOf_ := reflect.ValueOf(v.GetLF())
					newSlice := reflect.MakeSlice(field.Type, 0, 0)
					valueField.Set(reflect.AppendSlice(newSlice, valueOf_))
				case "LI": // list int
					valueOf_ := reflect.ValueOf(v.GetLI())
					newSlice := reflect.MakeSlice(field.Type, 0, 0)
					valueField.Set(reflect.AppendSlice(newSlice, valueOf_))
				case "LB": // List []byte
					valueOf_ := reflect.ValueOf(v.GetLB())
					newSlice := reflect.MakeSlice(field.Type, 0, 0)
					valueField.Set(reflect.AppendSlice(newSlice, valueOf_))
				case "LBl": // List bool
					valueOf_ := reflect.ValueOf(v.GetLB())
					newSlice := reflect.MakeSlice(field.Type, 0, 0)
					valueField.Set(reflect.AppendSlice(newSlice, valueOf_))
				// case "Nd": // List []byte
				// 	valueOf_ := reflect.ValueOf(v.GetNd())
				// 	fmt.Println("In Nd: Kind(): ", valueOf_.Kind(), valueOf_.Type().Elem(), valueOf_.Len()) //  slice string 4
				// 	newSlice := reflect.MakeSlice(field.Type, 0, 0)
				// 	valueField.Set(reflect.AppendSlice(newSlice, valueOf_))
				case "IS": // set int
				case "FS": // set float
				case "SS": // set string
				case "BS": // set binary
				default:
					panic(fmt.Errorf("Unsupported data type %q", x))
				}

			}
		}
	}
	return nil

}

func (d *NodeCache) GetType() (tyN string, ok bool) {
	var di *blk.DataItem

	syslog(fmt.Sprintf("GetType: d.m: %#v\n", d.m))
	if di, ok = d.m["A#A#T"]; !ok {
		//
		// check other predicates as most have a Ty attribute defined (currently propagated data does not)
		// this check enables us to use more specific sortK values when fetching a node rather than using top level "A#" that fetch all node data (performance hit)
		//
		for _, di := range d.m {
			//
			if len(di.GetTy()) != 0 {
				ty, b := types.GetTyLongNm(di.GetTy())
				if b == false {
					panic(fmt.Errorf("cache.GetType() errored. Could not find long type name for short name %s", di.GetTy()))
				}
				return ty, true
			}
		}
		panic(fmt.Errorf("GetType: no A#T entry in NodeCache"))
		return "", ok
	}
	ty, b := types.GetTyLongNm(di.GetTy())
	if b == false {
		panic(fmt.Errorf("cache.GetType() errored. Could not find long type name for short name %s", di.GetTy()))
	}
	return ty, true
}

// SetOvflBlkFull sets in cache and database the state of the overflow block UID
// in the node's particular uid-predicate (sortk) to FULL. It will nolonger be
// available accept data propagation
// the node is locked in the calling (client) pkg.
func (pn *NodeCache) SetOvflBlkFull(ovflBlkUID util.UID, sortk string) error {
	var cIdx int

	di := pn.m[sortk]

	for i, v := range di.Nd {
		if bytes.Equal(ovflBlkUID, v) {
			// set flag to full
			di.XF[i] = blk.OvflItemFull
			cIdx = i
			break
		}
	}
	//
	// preserve cache change to db
	//
	err := db.SaveOvflBlkFull(di, cIdx)
	if err != nil {
		return err
	}

	return nil
}

func (pn *NodeCache) CommitUPred(sortK string, pUID, cUID, tUID util.UID, id int, cnt int, ty string) error { // (util.UID, int, error) {

	var err error
	// di, ok := pn.m[sortK]
	// if !ok {
	// 	panic(fmt.Errorf("CommitUPred inconsistency. Sortk %q does not exist in cache", sortK))
	// }
	// err := db.SaveCompleteUpred(di)
	// if err != nil {
	// 	return (err)
	// }
	err = pn.SetUpredAvailable(sortK, pUID, cUID, tUID, id, cnt, ty)

	return err
}

// ConfigureUpred returns either the parent block or an overflow block as the target for the propagation of child scalar data
// in the case of overflow block it may require more overflow blocks to be created if less than a certain number are available.
// ConfigureUpred makes changes to the node cache (i.e. adds overflows in the cache first). The last operation ofConfigureUpred
// writes the uid-pred portion of the nocde cache to the database
// sortK: uid-predicate of interest in parent node
// pUID: parent UID
// cUID: child UID
// rsvCnt:
//
// Note: it is required to read the db data as decisions are based on this data. We then change the data and save the complete block back to the db
//
func (pn *NodeCache) ConfigureUpred(sortK string, pUID, cUID util.UID, rsvCnt ...int) (util.UID, int, error) {
	var (
		ok          bool
		embedded    int
		ovflBlocks  int
		id          int
		di          *blk.DataItem // existing item
		availOfUids []util.UID    // available overflow uids
		newOfUID    []util.UID
		tUID        util.UID // chosen overflow UID to use
		itemId      []int
	)
	//
	// check if its a recursive call and if greater than 1 recursive call abort
	//
	syslog(fmt.Sprintf("ConfigureUpred:  pUID,cUID,sortK : %s   %s   %s", pUID.String(), cUID.String(), sortK))
	switch {
	case len(rsvCnt) == 0:
		rsvCnt = append(rsvCnt, 1)
	default:
		rsvCnt[0] += 1
	}
	if rsvCnt[0] > param.MaxOvFlBlocks {
		return nil, 0, fmt.Errorf(fmt.Sprintf("Abort: Recursive calls to ConfigureUpred exceeeds %d", param.MaxOvFlBlocks))
	}
	//
	// exclusive parent node lock has been applied in calling routine
	//
	if di, ok = pn.m[sortK]; !ok {
		// no uid-pred exists - create an empty one
		syslog(fmt.Sprintf("ConfigureUpred: sortK not cached so create empty blk.DataItem for pUID %s", pUID))
		//TODO - when is this used?
		di = new(blk.DataItem)
		pn.m[sortK] = di
		//	return nil, 0, fmt.Errorf("GetTargetBlock: Target attribute %q does not exit", sortK)
	}
	// XF is a LN (list of Number) populated in uid-predicate. Flag value 1 : c-UID, 2 : inUse 3 : c-UID is soft deleted (detached), 4 : overflow UID, 5 : overflow Blk InUse  6 : Ovflw item FUll (not applicable anymore)
	// Get the current distribution of propagation data either embedded in parent node or in overflow blocks (UIDs)
	for i, v := range di.XF {
		switch {
		case v <= blk.UIDdetached:
			// includes attached cUIDs, inUse cUIDs &  attached cUIDs
			embedded++
		case v == blk.OvflBlockUID || v == blk.OvflItemFull || v == blk.OuidInuse:
			// count overflow UIDs in any state
			ovflBlocks++
			if v == blk.OvflBlockUID { // || v == blk.OvflItemFull {
				// available overflow uids only
				availOfUids = append(availOfUids, di.Nd[i])
				itemId = append(itemId, di.Id[i])
			}
		}
	}
	// for _, v := range availOfUids {
	// 	fmt.Printf("Available OfUID: %s\n", util.UID(v).String())
	// }
	//
	if embedded < param.EmbeddedChildNodes && ovflBlocks == 0 {
		//
		// append  cUID  to Nd, XF (not using overflow yet) to cached data di
		//
		di.Nd = append(di.Nd, cUID)
		di.XF = append(di.XF, blk.CuidInuse)
		di.Id = append(di.Id, 0)
		//
		err := db.SaveCompleteUpred(di)
		if err != nil {
			panic(err)
			return nil, 0, fmt.Errorf("SaveCompleteUpred: %s", err)
		}
		return pUID, 0, nil // attachment point is the parent UID
	}
	//
	if len(availOfUids) <= param.OvFlBlocksGrowBy && ovflBlocks < param.MaxOvFlBlocks {
		//
		// create an Overflow UID and subsequent physical block
		//
		ouid, err := util.MakeUID()
		if err != nil {
			return nil, 0, fmt.Errorf("Failed to make UID: %s", err.Error())
		}
		//
		// add new Overlfow block - append to exising Nd, Xf, Id. These have their equivalents in Dynamo as List types.
		//
		di.Nd = append(di.Nd, ouid)
		di.XF = append(di.XF, blk.OvflBlockUID)
		di.Id = append(di.Id, 1)
		newOfUID = append(newOfUID, ouid)
		availOfUids = append(availOfUids, ouid)
		itemId = append(itemId, 1)
		//
		// update database with new overflow UIDs and XF state
		//
		err = db.SaveCompleteUpred(di)
		if err != nil {
			return nil, 0, fmt.Errorf("SaveCompleteUpred: %s", err)
		}
		//
		// create associated overflow blocks
		//
		err = db.MakeOvflBlocks(di, newOfUID, 1)
		if err != nil {
			return nil, 0, fmt.Errorf("MakeOvflBlocks: %s", err)
		}
	}
	//
	// keep adding overflow blocks until max limit reached then go back and populate into existing overflow blocks
	// incrementing the overflow batch count. An overflow block has 1:n batches. Each batch contains upto param.OvfwBatchLimit.
	// When the batch limit is exceeded (deteched using Dynamo SIZE() in a conditional update) and MaxOvFlBlocks is exceeded we then simply create
	// new batches in the existing overflow blocks
	//

	if ovflBlocks == param.MaxOvFlBlocks && len(availOfUids) == 0 {
		// only option now is to create an overflow batch
		for i, v := range di.XF {
			if v == blk.OvflItemFull {
				di.XF[i] = blk.OvflBlockUID // overflow block can now accept new entries but Id must be increased
				di.Id[i] += 1
				// create new overflow batch (sortk#id)
				db.CreateOvflBatch(di.Nd[i], sortK, di.Id[i])
			}
		}
		// try again
		return pn.ConfigureUpred(sortK, pUID, cUID, rsvCnt[0])
	}
	//
	// choose an overflow block to use - must be available
	//
	var idx int
	tUID = availOfUids[len(availOfUids)-1]
	// get current Id for tUID
	for i, v := range di.Nd {
		if bytes.Equal(v, tUID) {
			id = di.Id[i]
			di.XF[i] = blk.OuidInuse
			idx = i
			break
		}
	}
	//
	// mark cache entry as InUse
	//
	// var idx int
	// for i, v := range di.Nd {
	// 	if bytes.Equal(v, tUID) {
	// 		di.XF[i] = blk.OuidInuse
	// 		idx = i
	// 		break
	// 	}
	// }
	//
	// generate overflow batch item sortk (a overflow block has 1:M batches)
	//
	sortk := sortK + "#" + strconv.Itoa(id)
	//
	// append child UID to Nd and XF in chosen Overflow Block. Child data will be
	// taken care of in PropagateChildData()
	//
	err := db.SaveChildUIDtoOvflBlock(cUID, tUID, sortk, id)
	if err != nil {
		if errors.Is(err, db.ErrConditionalCheckFailed) {
			di.XF[idx] = blk.OvflItemFull
			return pn.ConfigureUpred(sortK, pUID, cUID, rsvCnt[0])
		}
		return nil, 0, err

	}
	//
	// preserve all cache changes to db
	//
	db.SaveCompleteUpred(di)

	return tUID, id, nil

}

// type FacetAttr struct {
// 	Attr  string
// 	Facet string
// 	Ty  string
// 	Abrev string
// }
// type expression struct {
// 	arg []Arguments
// 	expr
// }

// type Attribute struct {
// 	alias  string
// 	name   string
// 	args   []Arguments
// 	facets []Facet
// 	filter []Filter
// 	attrs  []attribute
// }

// func (u *UIDs) Attr() {}

// type query struct {
// 	alias
// 	name    string
// 	var_    string
// 	f       string
// 	cascade bool
// 	filter  []Filter
// 	attr    []attribute
// 	args    []Arguments
// }

type FacetTy struct {
	Name string
	DT   string
	C    string
}

type FacetCache map[types.TyAttr][]FacetTy

var FacetC FacetCache

func AddReverseEdge(cuid, puid []byte, pTy string, sortk string) error {
	return nil
}
