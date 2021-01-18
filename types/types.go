package types

import (
	"fmt"
	"strings"

	blk "github.com/DynamoGraph/block"
	param "github.com/DynamoGraph/dygparam"
	slog "github.com/DynamoGraph/syslog"
	"github.com/DynamoGraph/types/internal/db"
)

const (
	logid = "types: "
)

type Ty = string     // type
type TyAttr = string // type:attr
type AttrTy = string

//type FacetIdent string // type:attr:facet
//
// Derived Type Attributes cache
//
type TyCache map[Ty]blk.TyAttrBlock

//
// caches for type-attribute and type-attribute-facet
//
type TyAttrCache map[TyAttr]blk.TyAttrD // map[TyAttr]blk.TyItem

//TODO: create a cache for lookup via attribute long name to get type, type-short-name, attribute-short-name. This map will be used to support the Has function.

type AttrTyCache map[AttrTy]string

//var TyAttrC TyAttrCache

//
type TypeCache struct {
	//sync.RWMutex // as all types are loaded at startup - no concurrency control required
	TyAttrC TyAttrCache
	TyC     TyCache
	AttrTy  AttrTyCache
}

var (
	err error
	//
	graph     string
	TypeC     TypeCache
	tyShortNm map[string]string
)

func logerr(e error, panic_ ...bool) {

	if len(panic_) > 0 && panic_[0] {
		slog.Log(logid, e.Error(), true)
		panic(e)
	}
	slog.Log(logid, e.Error())
}

func GetTyShortNm(longNm string) (string, bool) {
	s, ok := tyShortNm[longNm]
	return s, ok
}

func GetTyLongNm(tyNm string) (string, bool) {
	for shortNm, longNm := range tyShortNm {
		if tyNm == longNm {
			return shortNm, true
		}
	}
	return "", false
}

func syslog(s string) {
	slog.Log(logid, s)
}

func SetGraph(graph_ string) {
	graph = graph_
	db.SetGraph(graph)
	//
	// cache holding the attributes belonging to a type
	///
	TypeC.TyC = make(TyCache)
	//
	// DataTy caches for type-attribute and type-attribute-facet
	//
	TypeC.TyAttrC = make(TyAttrCache)
	//
	TypeC.AttrTy = make(AttrTyCache)
	//
	tynames, err := db.GetTypeShortNames()
	if err != nil {
		panic(err)
	}
	if len(tynames) == 0 {
		panic(fmt.Errorf("No short name type data loaded"))
	}
	//
	// populate type short name cache. This cache is conccurent safe as it is readonly from now on.
	//
	tyShortNm = make(map[string]string)
	for _, v := range tynames {
		tyShortNm[v.LongNm] = v.ShortNm
	}
	//
	// Load data dictionary (i.e ALL type info) - makes for concurrent safe FetchType()
	//
	{
		dd, err := db.LoadDataDictionary() // type TyIBlock []TyItem
		if err != nil {
			panic(err)
		}
		populateTyCaches(dd)
	}
}

func populateTyCaches(allTypes blk.TyIBlock) {
	var (
		tyNm  string
		a     blk.TyAttrD
		tc    blk.TyAttrBlock
		tyMap map[string]bool
	)
	tyMap = make(map[string]bool)

	genTyAttr := func(ty string, attr string) TyAttr {
		var s strings.Builder
		// generte key for TyAttrC:  <typeName>:<attrName> e.g. Person:Age
		s.WriteString(ty)
		s.WriteByte(':')
		s.WriteString(attr)
		return s.String()
	}
	for k, v := range allTypes {
		tyNm = v.Nm[strings.Index(v.Nm, ".")+1:]
		v.Nm = tyNm
		if _, ok := tyMap[tyNm]; !ok {
			tyMap[tyNm] = true
		}
		allTypes[k] = v
	}

	for k, v := range tyMap {
		fmt.Println("tyMap: ", k, v)
	}

	for k, v := range allTypes {
		fmt.Println("allTypes: ", k, v)
	}

	for ty, _ := range tyMap {

		fmt.Println("load type data for ", ty)
		for _, v := range allTypes {
			// if not current ty then
			if v.Nm != ty {
				continue
			}
			//
			TypeC.AttrTy[v.Atr+"#"+v.Nm] = v.C // support attribute lookup for Has(<attribute>) function
			//
			// checl of DT is a UID attribute and gets its base type
			//	fmt.Printf("DT:%#v \n", v)
			if len(v.Ty) == 0 {
				panic(fmt.Errorf("DT not defined for %#v", v))
			}
			//
			// scalar type or abstract type e.g [person]
			//
			if v.Ty[0] == '[' {
				// abstract type
				card := v.Cardinality
				if len(v.Cardinality) == 0 {
					card = "1:N"
				} else {
					if v.Cardinality != "1:1" && v.Cardinality != "1:N" {
						panic(fmt.Errorf("Type data error: wrong cardinality value [%s]", v.Cardinality))
					}
				}
				a = blk.TyAttrD{Name: v.Atr, DT: "Nd", C: v.C, Ty: v.Ty[1 : len(v.Ty)-1], P: v.P, Pg: v.Pg, N: v.N, IncP: v.IncP, Ix: v.Ix, Card: card}
			} else {
				// scalar
				a = blk.TyAttrD{Name: v.Atr, DT: v.Ty, C: v.C, P: v.P, N: v.N, Pg: v.Pg, IncP: v.IncP, Ix: v.Ix}
			}
			tc = append(tc, a)
			//
			TypeC.TyAttrC[genTyAttr(ty, v.Atr)] = a
			tyShortNm, ok := GetTyShortNm(ty)
			if !ok {
				panic(fmt.Errorf("Error in populateTyCaches: Type short name not found"))
			}
			TypeC.TyAttrC[genTyAttr(tyShortNm, v.Atr)] = a

			// fc, _ := FacetCache[tyAttr]
			// for _, vf := range v.F {
			// 	vfs := strings.Split(vf, "#")
			// 	if len(vfs) == 3 {
			// 		f := FacetTy{Name: vfs[0], DT: vfs[1], C: vfs[2]}
			// 		fc = append(fc, f)
			// 	} else {
			// 		panic(fmt.Errorf("%s", "Facet type information must contain 3 elements: <facetName>#<datatype>#<compressedIdentifer>"))
			// 	}
			// }
			// FacetCache[tyAttr] = fc
		}
		//
		TypeC.TyC[ty] = tc
		tc = nil
	}
	if param.DebugOn {
		fmt.Println("==== TypeC.AttrTy")
		for k, v := range TypeC.AttrTy {
			for _, v2 := range v {
				fmt.Printf("%s       %c\n", k, v2)
			}
		}
		fmt.Println("\n==== TypeC.TyC")
		for k, v := range TypeC.TyC {
			for _, v2 := range v {
				fmt.Printf("%s       %#v\n", k, v2)
			}
		}
		fmt.Println("\n===== TypeC.TyAttrC")
		for k, v := range TypeC.TyAttrC {
			fmt.Printf("%s       %#v\n", k, v)
		}
	}
	// confirm caches are populated
	if len(TypeC.TyC) == 0 {
		panic(fmt.Errorf("typeC.TyC is empty"))
	}
	if len(TypeC.AttrTy) == 0 {
		panic(fmt.Errorf("typeC.AttrTy is empty"))
	}
	if len(TypeC.TyAttrC) == 0 {
		panic(fmt.Errorf("typeC.TyAttrC is empty"))
	}
	//panic(fmt.Errorf("Testing load of DD"))
	fmt.Println("End populateTyCaches...")
}

func FetchType(ty Ty) (blk.TyAttrBlock, error) {

	// check if ty is long name using GetTyShortNm which presumes the input is a long name
	if _, ok := GetTyShortNm(ty); !ok {
		// must be a short name - check it exists using GetTyLongNm which only accepts a short name
		if longTy, ok := GetTyLongNm(ty); !ok {
			return nil, fmt.Errorf("FetchType: error %q type not found or short name not defined", ty)
		} else {
			ty = longTy
		}
	}
	if ty, ok := TypeC.TyC[ty]; ok { // ty= Person
		return ty, nil
	}
	return nil, fmt.Errorf("No type %q found", ty)

}

func IsScalarPred(pred string) bool { //TODO: pass in Type so uid-pred is checked against type not whole data dictionary
	for _, v := range TypeC.TyC {
		for _, vv := range v {
			if vv.Name == pred && len(vv.Ty) == 0 {
				// is a scalar in one type so presume its ok
				return true
			}
		}
	}
	return false
}

func IsUidPred(pred string) bool { //TODO: pass in Type so uid-pred is checked against type not whole data dictionary

	for _, v := range TypeC.TyC {
		for _, vv := range v {
			if vv.Name == pred && len(vv.Ty) > 0 {
				// is a uid-pred in one type so presume its ok
				return true
			}
		}
	}
	return false
}

func IsScalarInTy(ty string, pred string) bool { //TODO: pass in Type so uid-pred is checked against type not whole data dictionary
	if t, ok := TypeC.TyAttrC[ty+":"+pred]; !ok {
		return false
	} else if len(t.Ty) != 0 {
		return false
	}
	return true
}

func IsUidPredInTy(ty string, pred string) bool { //TODO: pass in Type so uid-pred is checked against type not whole data dictionary

	if t, ok := TypeC.TyAttrC[ty+":"+pred]; !ok {
		return false
	} else if len(t.Ty) == 0 {
		return false
	}
	return true
}
