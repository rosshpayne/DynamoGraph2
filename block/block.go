package block

import (
	"time"

	"github.com/DynamoGraph/util"
)

type DynaGType uint8

//type UIDstate int8

const (
	// Propagation UID flags. XF flag
	ChildUID int = iota + 1
	CuidInuse
	UIDdetached
	OvflBlockUID
	OuidInuse
	OvflItemFull
	EdgeFiltered // set to true when edge fails GQL uid-pred  filter
)

const (
	//
	// scalar types
	//
	N  DynaGType = iota // number
	S                   // string
	Bl                  // bool
	B                   // []byte
	DT                  // DateTime
	//
	// List (ordered set of any type but constrainted to a single type in DynaGraph)
	//
	LS  // []string
	LN  // []string
	LB  // [][]byte
	LBl // []bool
	Nd  // [][]byte // list of node UIDs
	//
	// Set (unordered set of a single type)
	//
	NS // []string
	SS // []string
	BS // [][]byte
	//
)

type DataItem struct {
	PKey  []byte // util.UID
	SortK string
	//attrName string
	//
	// scalar types
	//
	N  float64 // numbers are represented by strings and converted on the fly to dynamodb number
	S  string
	Bl bool
	B  []byte
	DT string // DateTime
	//
	// node type - listed in GSI so value can be associated with type for "has" operator
	//
	Ty string
	//
	// List (ordered set of any type but constrainted to a single type in DynaGraph)
	//
	LS  []string
	LN  []float64
	LB  [][]byte
	LBl []bool
	Nd  [][]byte // list of node UIDs, overflow block UIDs, oveflow index UIDs
	//
	// Set (unordered set of a single type)
	//
	NS []float64
	SS []string
	BS [][]byte
	// base64.StdEncoding.Decode requires []byte argument hence XB [][]byte (len 1) rather thab []byte
	// also Dynamo ListAppend only works with slices so []byte to [][]byte
	// note: I use XB rather than X as X causes a Unmarshalling error. See description of field in doco.
	XBl []bool // used for propagated child scalars (List data). True means associated child value is NULL (ie. is not defined)
	XF  []int  // used in uid-predicate 1 : c-UID, 2 : c-UID is soft deleted, 3 : ovefflow UID, 4 : overflow block full
	Id  []int  // overflow item number in Overflow block e.g. A#G:S#:A#3 where Id is 3 meaning its the third item in the overflow block. Each item containing 500 or more UIDs in Lists.
	//
}
type NodeBlock []*DataItem

//
// keys
//
func (dgv *DataItem) GetPkey() util.UID {
	return util.UID(dgv.PKey)
}
func (dgv *DataItem) GetSortK() string {
	return dgv.SortK
}

//
// Scalars - scalar data has no associated XBl null inidcator. Absense of item/predicate means it is null.
//
func (dgv *DataItem) GetS() string {
	return dgv.S
}

func (dgv *DataItem) GetTy() string {
	return dgv.Ty
}

func (dgv *DataItem) GetI() int64 {
	i := int64(dgv.N)
	return i
}
func (dgv *DataItem) GetF() float64 {
	return dgv.N
}
func (dgv *DataItem) GetDT() time.Time {
	t, _ := time.Parse(time.RFC3339, dgv.DT)
	return t
}
func (dgv *DataItem) GetB() []byte {
	return dgv.B
}
func (dgv *DataItem) GetBl() bool {
	return dgv.Bl
}

//
// Sets (no associated Null attributes)
//
func (dgv *DataItem) GetSS() []string {
	return dgv.SS
}

func (dgv *DataItem) GetIS() []int64 {
	is := make([]int64, len(dgv.NS), len(dgv.NS))
	for i, _ := range dgv.NS {
		is[i] = int64(dgv.NS[i])
	}
	//dgv.NS = nil // free
	return is
}

func (dgv *DataItem) GetFS() []float64 {
	return dgv.NS
}
func (dgv *DataItem) GetBS() [][]byte {
	return dgv.BS
}

//
// Lists - embedded in item
//
func (dgv *DataItem) GetLS() []string {
	return dgv.LS
}

//TODO - should this be []int??
func (dgv *DataItem) GetLI() []int64 {
	is := make([]int64, len(dgv.LN), len(dgv.LN))
	for i, _ := range dgv.LN {
		is[i] = int64(dgv.LN[i])
	}
	//dgv.LN = nil // free
	return is
}

func (dgv *DataItem) GetLF() []float64 {
	return dgv.LN
}
func (dgv *DataItem) GetLB() [][]byte {
	return dgv.LB
}
func (dgv *DataItem) GetLBl() []bool {
	return dgv.LBl
}

//
// Lists - only used for containing propagated values
//
func (dgv *DataItem) GetULS() ([]string, []bool) {
	return dgv.LS, dgv.XBl
}

//TODO - should this be []int??
func (dgv *DataItem) GetULI() ([]int64, []bool) {
	is := make([]int64, len(dgv.LN), len(dgv.LN))
	for i, _ := range dgv.LN {
		is[i] = int64(dgv.LN[i])
	}
	//dgv.LN = nil // free
	return is, dgv.XBl
}

func (dgv *DataItem) GetULF() ([]float64, []bool) {
	return dgv.LN, dgv.XBl
}
func (dgv *DataItem) GetULB() ([][]byte, []bool) {
	return dgv.LB, dgv.XBl
}
func (dgv *DataItem) GetULBl() ([]bool, []bool) {
	return dgv.LBl, dgv.XBl
}

//
// Propagated Scalars - all List based (UID-pred stuff)
//
// func (dgv *DataItem) GetULS() ([]string, []bool) {
// 	return dgv.LS, dgv.XBl
// }
// func (dgv *DataItem) GetULI() ([]int64, []bool) {
// 	is := make([]int64, len(dgv.LN), len(dgv.LN))
// 	for i, _ := range dgv.LN {
// 		is[i] = int64(dgv.LN[i])
// 	}
// 	//dgv.LN = nil // free
// 	return is
// }
// func (dgv *DataItem) GetULF() ([]float64, []bool) {
// 	return dgv.LN
// }
// func (dgv *DataItem) GetULB() ([][]byte, []bool) {
// 	return dgv.LB
// }
// func (dgv *DataItem) GetULBl() ([]bool, []bool) {
// 	return dgv.LBl
// }
func (dgv *DataItem) GetNd() (nd [][]byte, xf []int, ovfl [][]byte) {

	// for i, v := range dgv.Nd {
	// 	fmt.Println("GetNd: i  UID ", i, util.UID(v).String())
	// }
	for i, v := range dgv.Nd {
		if x := dgv.XF[i]; x <= UIDdetached {
			nd = append(nd, util.UID(v))
			xf = append(xf, x)
		} else {
			ovfl = append(ovfl, util.UID(v))
		}
	}
	//
	return nd, xf, ovfl

}

func (dgv *DataItem) GetOfNd() ([][]byte, []int) {

	return dgv.Nd[1:], dgv.XF[1:]
}

type OverflowItem struct {
	PKey  []byte
	SortK string
	//
	// List (ordered set of any type but constrainted to a single type in DynaGraph)
	//
	Nd [][]byte // list of child node UIDs
	B  []byte   // parent UID
	// scalar data
	LS  []string
	LN  []float64
	LB  [][]byte
	LBl []bool
	LDT []string // DateTime
	// flags
	XBl []bool
}

type OverflowBlock [][]*OverflowItem

type Index struct {
	PKey  []byte
	SortK string
	//
	Ouid [][]byte // overflow block UIDs
	//
	XB [][]byte
	XF [][]int
}

type IndexBlock []*Index

//
// ClientNV from AttachNode is persisted using this struct
//
// type StreamCnv struct {
// 	PKey  []byte // ClientNV UID (not node UID)
// 	SortK string // predicate
// 	//attrName string
// 	//
// 	// scalar value to propagate
// 	//
// 	N  float64 // numbers are represented by strings and converted on the fly to dynamodb number
// 	S  string
// 	Bl bool
// 	B  []byte
// }

// func (nv *StreamCnv) GetB() []byte {
// 	return nv.B
// }
// func (nv *StreamCnv) GetBl() bool {
// 	return nv.Bl
// }
// func (nv *StreamCnv) GetDT() time.Time {
// 	t, _ := time.Parse(time.RFC3339, nv.DT)
// 	return t
// }
// func (nv *StreamCnv) GetS() string {
// 	return nv.S
// }
// func (nv *StreamCnv) GetI() int64 {
// 	i := int64(nv.N)
// 	return i
// }
// func (nv *StreamCnv) GetF() float64 {
// 	return nv.N
// }

//
// type dictionary
//
type TyItem struct {
	Nm          string   // type name
	Atr         string   // attribute name
	Ty          string   // DataType
	F           []string // facets name#DataType#CompressedIdentifer
	C           string   // compressed identifer for attribute
	P           string   // data partition containig attribute data - TODO: is this obselete???
	Pg          bool     // true: propagate scalar data to parent
	N           bool     // NULLABLE. False : not null (attribute will always exist ie. be populated), True: nullable (attribute may not exist)
	Cd          int      // cardinality - NOT USED
	Sz          int      // average size of attribute data - NOT USED
	Ix          string   // supported indexes: FT=Full Text (S type only), "x" combined with Ty will index in GSI Ty_Ix
	IncP        []string // (optional). List of attributes to be propagated. If empty all scalars will be propagated.
	Cardinality string   // 1:N , 1:1
}

type TyIBlock []*TyItem

// type attribute block, derived from TyItem
type TyAttrD struct {
	Name string // Attribute Identfier
	DT   string // Derived value. Attribute Data Type - Nd,DT,N,S etc
	C    string // Attribute short identifier
	Ty   string // For uid-pred only, the type it respresents e.g "Person"
	P    string // data partition (aka shard) containing attribute
	N    bool   // true: nullable (attribute may not exist) false: not nullable
	Pg   bool   // true: propagate scalar data to parent
	IncP []string
	Ix   string // index type
	Card string
}

type TyAttrBlock []TyAttrD

func (t TyAttrBlock) GetUIDpredC() []string {
	var predC []string
	for _, v := range t {
		if v.DT == "Nd" {
			predC = append(predC, v.C)
		}
	}
	return predC
}

//
// type TyCache map[Ty]blk.TyAttrBlock
// var TyC TyCache
// type TyAttrCache map[Ty_Attr]blk.TyAttrD // map[Ty_Attr]blk.TyItem
// var TyAttrC TyAttrCache

// ***************** rdf ***********************

type ObjT struct {
	Ty    string      // type def from Type
	Value interface{} // rdf object value
}

// RDF SPO
type RDF struct {
	Subj string //[]byte // subject
	Pred string // predicate
	Obj  ObjT   // object
}
