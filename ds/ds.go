package ds

import (
	"fmt"
	"strconv"
	"strings"

	blk "github.com/DynamoGraph/block"
	"github.com/DynamoGraph/util"
)

// NV is an abstraction layer immediate above the cached representation of the graph which itelf is sourced 1:1 from the database
// It is populated in the nodecache.UnmarshalCache method. NV presents the data for query consumption.
// See MarshalJSON to see it in use.
//
type NV struct {
	Name     string      // predicate (no type required as its based on contents in cache) Name (S), Age (N), Siblings: (Nds), Siblings:Name (list), Friends: (nds), Friends:Name (list)
	Value    interface{} // its value from cache
	Filtered bool        // true indicates item satisifed uid-pred filter condition and should be ignored when marshaling
	ItemTy   string      // TODO: is this used???
	Ignore   bool
	// short name of the type Name belongs to e.g. Pn (for Person)
	//
	// used by UnmarshalCache
	//
	Sortk  string
	OfUIDs [][]byte // overflow blocks ids
	// ... for Overflow blocks only
	State [][]int  // Nd only (propagated child UIDs only) - states: cuid, cuid-detached, cuid-filtered
	Null  [][]bool // For propagated scalar values only first slice reps Overflow block, second reps each child node in the overflow block
}

type ClientNV []*NV

type NVmap map[string]*NV // string is a Node UUID?

func (c ClientNV) MarshalJSON() {

	fmt.Println("MarshalJSON...")

	var s strings.Builder

	for _, v := range c {

		fmt.Println("v.Name ", v.Name)

		switch x := v.Value.(type) {
		case int64: //

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : "`)
			i64 := strconv.Itoa(int(x))
			s.WriteString(i64)
			s.WriteString(`"} `)

		case float64: //

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : "`)
			s64 := strconv.FormatFloat(x, 'E', -1, 64)
			s.WriteString(s64)
			s.WriteString(`"}`)

		case string: // S, DT

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : "`)
			s.WriteString(x)
			s.WriteString(`" }`)

		case bool: // Bl

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : "`)
			if x {
				s.WriteString("true")
			} else {
				s.WriteString("false")
			}
			s.WriteString(` } }`)

		case []byte: // B

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : "`)
			s.WriteString(string(x))
			s.WriteString(`" }`)

		case []string: // LS, NS

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : [`)
			for i, k := range x {
				s.WriteByte('"')
				s.WriteString(k)
				s.WriteByte('"')
				if i < len(x)-1 {
					s.WriteByte(',')
				}
				if i > 10 {
					s.WriteString("...")
					break
				}
			}
			s.WriteString(` ] }`)

		case []float64: // LN, NS

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : [ `)
			for i, k := range x {
				s.WriteByte('"')
				s64 := strconv.FormatFloat(k, 'E', -1, 64)
				s.WriteString(s64)
				s.WriteByte('"')
				if i < len(x)-1 {
					s.WriteByte(',')
				}
				if i > 10 {
					s.WriteString("...")
					break
				}
			}
			s.WriteString(` ] }`)

		case []int64: //

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : [ `)
			for i, k := range x {
				s.WriteByte('"')
				i64 := strconv.Itoa(int(k))
				s.WriteString(i64)
				s.WriteByte('"')
				if i < len(x)-1 {
					s.WriteByte(',')
				}
				if i > 10 {
					s.WriteString("...")
					break
				}
			}
			s.WriteByte('}')

		case []bool: // LBl

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : [ `)
			for i, k := range x {
				if k {
					s.WriteString("true")
				} else {
					s.WriteString("false")
				}
				if i < len(x)-1 {
					s.WriteByte(',')
				}
				if i > 10 {
					s.WriteString("...")
					break
				}
			}
			s.WriteString(` ] }`)

		case [][]byte: // BS

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : [ `)
			for i, k := range x {
				s.WriteByte('"')
				s.WriteString(string(k))
				s.WriteByte('"')
				if i < len(x)-1 {
					s.WriteByte(',')
				}
				if i > 10 {
					s.WriteString("...")
					break
				}
			}
			s.WriteString(` ] }`)

		case [][]int64: // Nd

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : [ `)
			for i, k := range x {
				//
				for ii, kk := range k {
					// is node not attached - hence its values should not be printed.
					if v.State[i][ii] == blk.UIDdetached {
						continue
					}
					// is value null
					if v.Null[i][ii] {
						s.WriteString("_NULL_")
					} else {
						// each int matches to one child UID
						s.WriteByte('"')
						i64 := strconv.Itoa(int(kk))
						s.WriteString(i64)
						s.WriteByte('"')
					}
					if ii < len(k)-1 {
						s.WriteByte(',')
					}
					if ii > 10 {
						s.WriteString("... ]")
						break
					}
				}
			}
			s.WriteString(` ] }`)

		case [][]float64: // Nd

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : [ `)
			for i, k := range x {
				// check if null
				for ii, kk := range k {
					// is node not attached - hence its values should not be printed.
					if v.State[i][ii] == blk.UIDdetached {
						continue
					}
					// is value null
					if v.Null[i][ii] {
						s.WriteString("_NULL_")
					} else {
						// each int matches to one child UID
						s.WriteByte('"')
						s64 := strconv.FormatFloat(kk, 'E', -1, 64)
						s.WriteString(s64)
						s.WriteByte('"')
					}
					if ii < len(k)-1 {
						s.WriteByte(',')
					}
					if ii > 10 {
						s.WriteString("... ]")
						break
					}
				}
			}
			s.WriteString(`] }`)

		case [][]string:

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : [ `)

			// for i, v := range x {
			// 	fmt.Println("X: ", i, v)
			// }
			// for i, v := range v.State {
			// 	fmt.Println("State: ", i, v)
			// }
			for i, k := range x {
				//
				for ii, kk := range k {
					// is node attached .
					if v.State[i][ii] == blk.UIDdetached {
						continue
					}
					// is value null
					if v.Null[i][ii] {
						s.WriteString("_NULL_")
					} else {
						// each int matches to one child UID
						s.WriteByte('"')
						s.WriteString(kk)
						s.WriteByte('"')
					}
					if ii < len(k)-1 {
						s.WriteByte(',')
					}
					if ii > 10 {
						s.WriteString("... ]")
						break
					}
				}
			}
			s.WriteString(`] }`)

		case []util.UID: // Nd

			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(`" : [ `)
			for i, k := range x {
				s.WriteByte('"')
				//	s.WriteString("__XX__")

				kk := k.Encodeb64().String()
				s.WriteString(kk)
				s.WriteByte('"')
				if i < len(x)-1 {
					s.WriteByte(',')
				}
				if i > 10 {
					s.WriteString("...")
					break
				}
			}
			s.WriteString(` ] }`)

		case nil:
			// no interface value assigned
			s.WriteString(`{ "`)
			s.WriteString(v.Name)
			s.WriteString(" : Null")
			s.WriteString(`}"`)

		}
		s.WriteByte('\n')
	}
	fmt.Println(s.String())

}
