package ast

import (
	"fmt"
	"sort"
	"strings"

	blk "github.com/DynamoGraph/block"
	"github.com/DynamoGraph/ds"
	"github.com/DynamoGraph/gql/monitor"
	mon "github.com/DynamoGraph/gql/monitor"
	"github.com/DynamoGraph/util"
)

func (r *RootStmt) MarshalJSON() string {
	//

	var out strings.Builder

	// marshal UIDs by sorted order
	var uids sort.StringSlice
	for k, _ := range r.nodesc {
		uids = append(uids, k)
	}
	sort.Sort(uids)

	if len(uids) > 0 {
		out.WriteString(fmt.Sprintf("\n{\ndata: [\n"))
	}
	for i, uid := range uids {

		nvc := r.nodesc[uid]
		nvm := r.nodes[uid]
		// monitor: increment node touched counter

		stat2 := mon.Stat{Id: mon.TouchNodeFiltered, Lvl: 0}
		mon.StatCh <- stat2

		out.WriteString(fmt.Sprintf("\t{\n"))

		for k, s := range r.Select {

			switch x := s.Edge.(type) { // AAA

			case *ScalarPred:

				nv := nvm[x.Name()]
				switch x := nv.Value.(type) {
				case int64:
					out.WriteString(fmt.Sprintf("%s%s : %v,\n", strings.Repeat("\t", 1), nv.Name, x))
				case string:
					out.WriteString(fmt.Sprintf("%s%s : %q,\n", strings.Repeat("\t", 1), nv.Name, x))
				case float64:
					out.WriteString(fmt.Sprintf("%s%s : %f,\n", strings.Repeat("\t", 1), nv.Name, x))
				default:
					out.WriteString(fmt.Sprintf("%s %s : %v,\n", strings.Repeat("\t", 1), nv.Name, x))
				}

			case *UidPred: // child of child, R.N.N
				// save the scalar predicates belonging to uid-pred x: e.g. Friends:Name, Friednds:Age
				var spred []*ds.NV

				for _, v := range nvc {
					if !(strings.Index(v.Name, ":") > -1 && v.Name[:strings.Index(v.Name, ":")] == x.Name() && len(v.Name) > len(x.Name())+1) {
						continue
					}
					// save to spred only if edge is not Ignored. Ignore set during genNV().
					if !v.Ignore {
						spred = append(spred, v)
					}
				}
				// get child uids that belong to edge x and print out the scalar attributes for x (see AAA)
				//
				var s strings.Builder
				out.WriteString(fmt.Sprintf("%s%s : [ \n", strings.Repeat("\t", 1), x.Name()))
				//
				//  see method cache.UnmarshalNodeCache for description of the design of the node cache which the following code interragates.
				//
				upred := nvm[x.Name()+":"]
				for i, uids := range upred.Value.([][][]byte) {
					for j, v := range uids {
						s.Reset()
						if upred.State[i][j] == blk.UIDdetached || upred.State[i][j] == blk.EdgeFiltered {
							stat := mon.Stat{Id: mon.TouchNode, Lvl: x.lvl}
							mon.StatCh <- stat
							continue // edge soft delete set or edge failed filter condition in GQL stmt
						}
						// monitor: increment touch counter
						stat := mon.Stat{Id: mon.TouchNode, Lvl: x.lvl}
						mon.StatCh <- stat
						stat = mon.Stat{Id: mon.TouchNodeFiltered, Lvl: x.lvl}
						mon.StatCh <- stat

						s.WriteString(fmt.Sprintf("%s{ \n", strings.Repeat("\t", 2)))
						// s.WriteString(fmt.Sprintf("%sidx: { i: %d, j: %d }\n", strings.Repeat("\t", 2), i, j))
						// s.WriteString(fmt.Sprintf("%suid: %s\n", strings.Repeat("\t", 2), util.UID(v).String()))
						for _, scalar := range spred {
							// check if uid-pred
							if strings.Count(scalar.Name, ":") > 1 { // actor.performance:performance.character:
								// ignore it's not scalar
								continue
							}
							pred := scalar.Name[strings.Index(scalar.Name, ":")+1:] // Friends:Age -> Age

							switch z := scalar.Value.(type) {
							case [][]string:
								s.WriteString(fmt.Sprintf("%s%s: %q,\n", strings.Repeat("\t", 2), pred, z[i][j]))
							case [][]int64:
								s.WriteString(fmt.Sprintf("%s%s: %d,\n", strings.Repeat("\t", 2), pred, z[i][j]))
							case [][]float64:
								s.WriteString(fmt.Sprintf("%s%s: %g,\n", strings.Repeat("\t", 2), pred, z[i][j]))
							case [][]bool:
								s.WriteString(fmt.Sprintf("%s%s: %v,\n", strings.Repeat("\t", 2), pred, z[i][j]))
								// TODO: what about other data types, sets in particular SS,SN..
							}
						}
						out.WriteString(s.String())
						//
						// walk the graph using uid-pred attributes belonging to edge x.
						// marshalJSON will print the scalar values associated with each child node of x.
						//
						for _, p := range x.Select {
							if y, ok := p.Edge.(*UidPred); ok {
								// only need to run marshalJSON once for all uid-pred's in x
								y.marshalJSON(v, &out)
								break
							}
						}
						if j == len(uids)-1 {
							out.WriteString(fmt.Sprintf("%s}\n", strings.Repeat("\t", 2)))
						} else {
							out.WriteString(fmt.Sprintf("%s},\n", strings.Repeat("\t", 2)))
						}
					}
				}
				if k == len(r.Select)-1 {
					out.WriteString(fmt.Sprintf("%s]\n", strings.Repeat("\t", 1)))
				}
			}
		}
		if i < len(uids)-1 {
			out.WriteString(fmt.Sprintf("%s}, \n", strings.Repeat("\t", 1)))
		} else {
			out.WriteString(fmt.Sprintf("%s}\n", strings.Repeat("\t", 1)))
		}
	}
	if len(uids) >= 1 {
		out.WriteString(fmt.Sprintf("]\n"))
	}
	out.WriteString(fmt.Sprintf("}\n"))

	monitor.PrintCh <- struct{}{}

	return out.String()
}

// 	fmt.Println("MarshalJSON root:   ")

// }

func (u *UidPred) marshalJSON(uid_ []uint8, out *strings.Builder) {

	uid := util.UID(uid_).String()
	//
	nvc, ok := u.Parent.getnodesc(uid)
	if !ok {
		panic(fmt.Errorf("Error in u.marshalJSON. uid %q not in nodesc for %s", uid, u.Name()))
	}
	nvm, ok := u.Parent.getnodes(uid)
	if !ok {
		panic(fmt.Errorf("Error in u.marshalJSON. uid %q not in nodes for %s", uid, u.Name()))
	}

	upred := u.Parent.(*UidPred)

	for k, s := range upred.Select {

		switch x := s.Edge.(type) {

		case *UidPred: // child of child, R.N.N

			var spred []*ds.NV

			for _, v := range nvc {
				if !(strings.Index(v.Name, ":") > -1 && v.Name[:strings.Index(v.Name, ":")] == x.Name() && len(v.Name) > len(x.Name())+1) {
					continue
				}
				if !v.Ignore {
					spred = append(spred, v)
				}
			}
			//
			// get child uids that belong to edge x and print out the scalar attributes for x
			//
			var s strings.Builder
			out.WriteString(fmt.Sprintf("%s%s : [ \n", strings.Repeat("\t", u.lvl), x.Name()))
			upred_ := nvm[x.Name()+":"]
			for i, uids := range upred_.Value.([][][]byte) {
				for j, v := range uids {
					//fmt.Printf("i, j, UID: %d %d, %s", i, j, util.UID(v).String())
					if upred_.State[i][j] == blk.UIDdetached || upred_.State[i][j] == blk.EdgeFiltered {
						stat := mon.Stat{Id: mon.TouchNode, Lvl: x.lvl}
						mon.StatCh <- stat
						continue // soft delete set or failed filter condition
					}
					s.Reset()
					stat := mon.Stat{Id: mon.TouchNode, Lvl: x.lvl}
					mon.StatCh <- stat
					stat = mon.Stat{Id: mon.TouchNodeFiltered, Lvl: x.lvl}
					mon.StatCh <- stat

					s.WriteString(fmt.Sprintf("%s{ \n", strings.Repeat("\t", u.lvl+1)))
					// s.WriteString(fmt.Sprintf("%sidx: { i: %d, j: %d }\n", strings.Repeat("\t", u.lvl+1), i, j))
					// s.WriteString(fmt.Sprintf("%suid: %s\n", strings.Repeat("\t", u.lvl+1), util.UID(v).String()))

					for _, scalar := range spred {

						pred := scalar.Name[strings.Index(scalar.Name, ":")+1:]
						// trailing : ignore for output. //TODO : check why
						if strings.Index(pred, ":") > -1 {
							continue
						}
						switch z := scalar.Value.(type) {
						case [][]string:
							s.WriteString(fmt.Sprintf("%s%s: %q,\n", strings.Repeat("\t", u.lvl+1), pred, z[i][j]))
						case [][]int64:
							s.WriteString(fmt.Sprintf("%s%s: %d,\n", strings.Repeat("\t", u.lvl+1), pred, z[i][j]))
						case [][]float64:
							s.WriteString(fmt.Sprintf("%s%s: %g,\n", strings.Repeat("\t", u.lvl+1), pred, z[i][j]))
						case [][]bool:
							s.WriteString(fmt.Sprintf("%s%s: %v,\n", strings.Repeat("\t", u.lvl+1), pred, z[i][j]))
							// TODO: what about other data types, sets in particular SS,SN..
						}
					}
					out.WriteString(s.String())
					//
					// walk the graph using uid-pred attributes belonging to edge x.
					// MarshalJSON will print the scalar values associated with each child node of x.
					//
					for _, p := range x.Select {

						if y, ok := p.Edge.(*UidPred); ok {
							// only need to run marshalJSON once for all uid-pred's in x. Once filter is incorporated this will change.
							y.marshalJSON(v, out)
							break
						}
					}
					out.WriteString(fmt.Sprintf("%s},\n", strings.Repeat("\t", u.lvl+1)))
				}
			}
			if k == len(upred.Select)-1 {
				out.WriteString(fmt.Sprintf("%s]\n", strings.Repeat("\t", u.lvl)))
			} else {
				out.WriteString(fmt.Sprintf("%s],\n", strings.Repeat("\t", u.lvl)))
			}

		}
	}
}
