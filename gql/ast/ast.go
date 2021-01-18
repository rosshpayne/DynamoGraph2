package ast

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	blk "github.com/DynamoGraph/block"
	"github.com/DynamoGraph/ds"
	expr "github.com/DynamoGraph/gql/expression"
	"github.com/DynamoGraph/gql/internal/db"
	"github.com/DynamoGraph/gql/token"
	"github.com/DynamoGraph/types"
	"github.com/DynamoGraph/util"
	//"github.com/DynamoGraph/rdf/grmgr"
)

type FargI interface {
	String() string
	Name() string
	farg()
}

type InnerFuncI interface {
	innerFunc()
}

type FilterI interface {
	AssignFilter(*expr.Expression)
	AssignFilterStmt(string)
}

// type InnerArgI interface {
// 	innerArg()
// }
type SelectI interface {
	Name() string
	AssignSelectList(SelectList)
	Initialise()
	//	hasNoData() bool
	assignData(string, ds.ClientNV, index) ds.NVmap
	getData(string) (ds.NVmap, ds.ClientNV, bool)
	getIdx(string) (index, bool)
	genNV(ty string) ds.ClientNV
	getnodes(string) (ds.NVmap, bool)
	getnodesc(string) (ds.ClientNV, bool)
}

type SelectList []*EdgeT

func (sl SelectList) String() string {
	var s strings.Builder
	for _, e := range sl {
		s.WriteString(e.String())
		s.WriteByte('\n')
	}
	return s.String()
}

// func (sl SelectList) GetPredicates() []string {
// 	var ps []string
// 	for _, e := range sl {
// 		ps = append(ps, e.GetPredicates()...)
// 	}
// 	return ps
// }

type EdgeT struct {
	Alias   name_
	VarName name_ // TODO: type should be Variable maybe
	Edge    EdgeI
	//f         aggrFunc - now in predicate (edgeT)
}

func (e EdgeT) String() string {
	var s strings.Builder
	if len(e.Alias.Name) > 0 {
		s.WriteString(e.Alias.Name)
		s.WriteString(" : ")
	}
	if len(e.VarName.Name) > 0 {
		s.WriteString(e.VarName.Name)
		s.WriteString(" as ")
	}
	s.WriteString(e.Edge.String())
	return s.String()
}

func (e *EdgeT) AssignName(input string, loc token.Pos) {
	//ValidateName(input, err, Loc)
	e.Alias = name_{Name: input, Loc: loc}
}

func (e *EdgeT) AssignVarName(input string, loc token.Pos) {
	//ValidateName(input, err, Loc)
	e.VarName = name_{Name: input, Loc: loc}
}

// func (e *EdgeT) GetPredicates() []string {
// 	return e.Edge.GetPredicates()
// }

func (e *EdgeT) JSON() string {
	var s strings.Builder
	if len(e.Alias.Name) != 0 {
		s.WriteString(e.Alias.String())
	}
	if len(e.VarName.Name) != 0 {
		//s.WriteString(GetVarValue(e.VarName))
	} else {
		s.WriteString(e.Edge.String())
	}
	return s.String()
}

type EdgeI interface {
	edge()
	String() string
	//	GetPredicates() []string
	Name() string
}

// type Arg1 interface {
// 	farg()
// }

type ScalarPred struct {
	Name_  name_
	Parent SelectI
}

func (s ScalarPred) edge() {}
func (s ScalarPred) farg() {}

func (s *ScalarPred) AssignName(input string, loc token.Pos) {
	//ValidateName(input, err, Loc)
	s.Name_ = name_{Name: input, Loc: loc}
}

func (s ScalarPred) String() string {
	return s.Name_.Name
}

func (s ScalarPred) Name() string {
	return s.Name_.Name
}

type NdNv map[util.UIDb64s]ds.ClientNV
type NdNvMap map[util.UIDb64s]ds.NVmap
type NdIdx map[util.UIDb64s]index

// func (s ScalarPred) GetPredicates() []string {
// 	return []string{s.Name_.Name}
// }
// type Data struct {
// 	ScKey ScalarKey
// 	Nd    map[util.UIDb64s]ds.ClientNV
// }
type UidPred struct {
	//
	// meta data description
	//
	Name_      name_   // gql predicate name
	Printed    bool    // false - not printed, true - has been printed
	Parent     SelectI // *RootStmt, *UidPred
	Filter     *expr.Expression
	filterStmt string
	Select     SelectList
	//
	// node edge data assoicated with this uidpred in GQL stmt
	//
	lvl    int // depth of graph (TODO is this used???)
	l      sync.Mutex
	nodes  NdNvMap // scalar nodes including PKey associated with each nodes belonging to this edge.
	nodesc NdNv
	nodesi NdIdx // nodes index into parent uid-pred's UL data. e.g. to get Age of this node - nv:=nodes.parent.nodes[uid]; age:= nv["Age"].([][]int); age[nodesi.i][nodesi.j]
	d      sync.Mutex
	// scalar nodes for nodes containing this uid-pred is contained in the parent.

}

func (p UidPred) edge() {}
func (u *UidPred) AssignName(input string, loc token.Pos) {
	//ValidateName(input, err, Loc)
	u.Name_ = name_{Name: input, Loc: loc}
}

func (u *UidPred) AssignFilter(e *expr.Expression) {
	u.Filter = e
}

func (u *UidPred) AssignFilterStmt(e string) {
	u.filterStmt = e
}

func (u *UidPred) GetLvl() int {
	return u.lvl
}

// func (u *UidPred) MakeNVM() {
// 	u.nvm = make(map[string][]ds.ClientNV)
// }

func (p *UidPred) Name() string {
	return p.Name_.Name
}

func (u UidPred) getnodes(uid string) (ds.NVmap, bool) {
	n, k := u.nodes[uid]
	return n, k
}

func (u *UidPred) getnodesc(uid string) (ds.ClientNV, bool) {
	n, ok := u.nodesc[uid]
	return n, ok
}

func (u *UidPred) AssignSelectList(s SelectList) {
	u.Select = s
}
func (u *UidPred) Initialise() {
	u.nodes = make(NdNvMap)
	u.nodesc = make(NdNv)
	u.nodesi = make(NdIdx)
}

// func (u *UidPred) hasNoData() bool {
// 	return u.nodes == nil
// }
func (u *UidPred) getIdx(key string) (index, bool) {
	i, ok := u.nodesi[key]
	return i, ok
}

func (u *UidPred) assignData(uid string, nvc ds.ClientNV, idx index) ds.NVmap {
	// make a ds.NVmap from nvc
	nvm := make(ds.NVmap)
	for _, v := range nvc {
		nvm[v.Name] = v
	}
	// save this edge (represented by key UID by assigning key to nodes).
	u.nodes[uid] = nvm
	u.nodesc[uid] = nvc
	u.nodesi[uid] = idx // index into UL cache data. TODO: is this used?
	return nvm
}

func (u *UidPred) getData(key string) (ds.NVmap, ds.ClientNV, bool) {
	nvm, _ := u.nodes[key]
	nvc, ok := u.nodesc[key]
	return nvm, nvc, ok
}

func (u *UidPred) genNV(ty string) ds.ClientNV {
	var nvc ds.ClientNV

	for _, v := range u.Select {

		switch x := v.Edge.(type) {

		case *UidPred:
			// uid-pred entry in NV
			un := x.Name() + ":"
			//
			fmt.Println()
			nv := &ds.NV{Name: un}
			nvc = append(nvc, nv)
			// add elements in uid-pred select list
			for _, vv := range x.Select {
				switch x := vv.Edge.(type) {
				case *ScalarPred:
					nv := &ds.NV{Name: un + x.Name()}
					nvc = append(nvc, nv)
				}
			}
			//
			// finally, add predicates from filter if present.
			// only include in list if not already already specified via the stmt specification
			// note: set the ignore attribute
			//
			if x.Filter != nil {
				var found bool
				for _, v := range x.Filter.GetPredicates() {
					found = false
					for _, x := range nvc {
						if x.Name == un+v {
							found = true
							break
						}
					}
					if !found {
						// filter predicate not in select list - add NV entry but mark as invisible (ignore) so it is not output
						nv := &ds.NV{Name: un + v, Ignore: true}
						nvc = append(nvc, nv)
					}
				}
			}
			//
			// check for nested types with cardinality 1:1 and add them to []NV
			//
			var (
				aty blk.TyAttrD
				ok  bool
			)
			if aty, ok = types.TypeC.TyAttrC[ty+":"+x.Name()]; !ok {
				panic(fmt.Errorf("%s not in %s", ty, x.Name()))
				continue // ignore this attribute as it is in current type
			}
			paty := aty
			for _, v := range x.Select {

				switch y := v.Edge.(type) {

				case *UidPred:

					if aty, ok = types.TypeC.TyAttrC[paty.Ty+":"+y.Name()]; !ok {
						panic(fmt.Errorf("%s %s not in %s", aty.Name, aty.Ty, y.Name()))
						continue // ignore this attribute as it is in current type
					}
					if aty.Card == "1:1" {
						un := x.Name() + ":" + y.Name() + ":"
						nv := &ds.NV{Name: un}
						nvc = append(nvc, nv)

						for _, v := range y.Select {

							switch z := v.Edge.(type) {

							case *ScalarPred:
								un += z.Name()
								nv := &ds.NV{Name: un}
								nvc = append(nvc, nv)
							}
						}
					}
				}
			}
		}

	}
	// remove duplicate entries in nvc
	return dedup(nvc)
}

// func (u *UidPred) GetPredicates() []string {
// 	var ps []string
// 	ps = append(ps, u.Name())
// 	// 	ps = append(ps, u.Filter.GetPredicates()...)
// 	ps = append(ps, u.Select.GetPredicates()...)
// 	return ps
// }

//func (p *UIDPred) innerArg() {}
func (p *UidPred) aggrArg() {}
func (p *UidPred) cntArg()  {}
func (p *UidPred) farg()    {} // in has() only
func (p UidPred) String() string {
	var s strings.Builder
	s.WriteString(p.Name_.Name)
	// Filter
	if p.Filter != nil {
		s.WriteString(" @filter( ")
		s.WriteString(p.filterStmt)
		s.WriteByte(')')
	}
	if p.Select != nil {
		s.WriteString("{\n")
		s.WriteString(p.Select.String())
		s.WriteByte('}')
	}
	return s.String()
}

type Variable struct {
	Name_ name_
}

func (u *Variable) AssignName(input string, loc token.Pos) {
	//ValidateName(input, err, Loc)
	u.Name_ = name_{Name: input, Loc: loc}
}
func (u *Variable) String() string {
	return "var(" + u.Name() + ")"
}

//func (r *Variable) innerArg()  {}
func (r *Variable) edge()   {}
func (r *Variable) cntArg() {}
func (r *Variable) Name() string {
	return r.Name_.Name
}

// not for root func: func (r *Variable) farg() {}

//func (r *Variable) innerFunc() {}
func (r *Variable) aggrArg() {}

// func (r *Variable) GetPredicates() []string {
// 	return nil
// }

//type ValFuncT func(v Variable) ValOut

type AggrArg interface {
	aggrArg()
}
type AggrFunc struct {
	Name name_ // count, avg takes either a variable argument or a uid-pred argument
	Arg  AggrArg
}

func (u *AggrFunc) AssignName(input string, loc token.Pos) {
	//ValidateName(input, err, Loc)
	u.Name = name_{Name: input, Loc: loc}
}

func (e *AggrFunc) edge() {}

//func (e *AggrFunc) innerFunc() {}

type CounterI interface {
	cntArg()
	String() string
	//	GetPredicates() []string
	Name() string
}

type CountFunc struct {
	Arg CounterI // uidPred, UID, Variable
}

func (e *CountFunc) farg() {}
func (e *CountFunc) edge() {}
func (e *CountFunc) String() string {
	var s strings.Builder
	s.WriteString("count(")
	s.WriteString(e.Arg.String())
	s.WriteByte(')')
	return s.String()
}
func (e *CountFunc) Name() string {
	return e.Arg.Name()
}

// func (e *CountFunc) GetPredicates() []string {
// 	return e.Arg.GetPredicates()
// }

type UID struct{}

func (e UID) edge()   {}
func (e UID) cntArg() {}
func (e UID) String() string {
	return "uid"
}
func (e UID) GetPredicates() []string {
	return nil
}

func (e UID) Name() string {
	return ""
}

type Values []interface{} // int,float,string,$var

// =========================  GQLFunc  =============================================

type FuncT func(FargI, interface{}) db.QResult

//type FuncT func(predfunc FargI, value interface{}, nv []ds.NV, ty string) []db.QResult

type GQLFunc struct {
	//	name  name_ // for String() purposes - TODO: check its not used if so remove it
	FName name_ // function name
	F     FuncT
	Farg  FargI // either predicate, count, var
	//	IFarg InnerArgI   // either uidPred, variable
	Value interface{} //  literal value: string,int,float,$var, List of string,int,float,$var
}

func (g *GQLFunc) AssignName(input string, loc token.Pos) {
	//ValidateName(input, err, Loc)
	g.FName = name_{Name: input, Loc: loc}
}

// func (g *GQLFunc) Execute() []db.QResult {
// 	//
// 	return g.F(g.Farg, g.Value)
// }

func (g *GQLFunc) Name() string {
	return g.FName.Name
}

func (f *GQLFunc) String() string {
	var s strings.Builder
	s.WriteString(f.FName.Name)
	s.WriteByte('(')
	s.WriteString(f.Farg.String())
	s.WriteByte(',')
	switch x := f.Value.(type) {
	case string:
		s.WriteByte('"')
		s.WriteString(x)
		s.WriteByte('"')
	case int:
		s.WriteString(strconv.Itoa(x))
	case float64:
		s.WriteString(strconv.FormatFloat(x, 'G', -1, 64))
		// list of literals, list of $varN...
	}
	s.WriteByte(')')
	return s.String()
}

// func (f *GQLFunc) String() string {
// 	//
// 	var s strings.Builder
// 	s.WriteString(f.Name.String())
// 	s.WriteByte('(')
// 	if f.Farg != nil {
// 		switch x := f.Farg.(type) {
// 		case *CountFunc:
// 			s.WriteString("count(")
// 			switch y := x.Arg.(type) {
// 			case *UidPred:
// 				s.WriteString(y.Name())
// 			case UID:
// 				s.WriteString("uid")
// 			}
// 			s.WriteString(")")
// 		case ScalarPred:
// 			s.WriteString(x.Name())
// 			//
// 		}
// 	}
// 	s.WriteByte(',')
// 	switch x := f.Value.(type) {
// 	case string:
// 		s.WriteByte('"')
// 		s.WriteString(x)
// 		s.WriteByte('"')
// 	case int:
// 		s.WriteString(strconv.Itoa(x))
// 	case float64:
// 		s.WriteString(strconv.FormatFloat(x, 'G', -1, 64))
// 		// list of literals, list of $varN...
// 	}
// 	s.WriteString(")")

// 	return s.String()
// }

// ============== Select/edge List ==============

// {
//   ID as var(func: allofterms(name@en, "Steven")) @filter(has(director.film)) {
//     director.film {
//       num_actors as count(starring)
//     }
//     average as avg(val(num_actors))
//   }

//   films(func: uid(ID)) {
//     director_id : uid
//     english_name : name@en
//     average_actors : val(average)
//     num_films : count(director.film)

//     films : director.film {
//       name : name@en
//       english_name : name@en
//       french_name : name@fr
//     }
//   }
// }
// {
//   me(func: eq(name@en, "Steven Spielberg")) @filter(has(director.film)) {
//     name@en
//     director.film @filter(allofterms(name@en, "jones indiana"))  {
//       name@en
//     }
//   }
// }

//func (s *ScalarPred) farg() {}

// ============== RootStmt ==============

// type NdNv map[util.UIDb64s]ds.ClientNV
// type NdNvMap map[util.UIDb64s]ds.NVmap
// type NdIdx map[util.UIDb64s]index

type RootStmt struct {
	Name_      name_
	Var        *Variable
	Lang       string
	RootFunc   GQLFunc          // generates []uid from GSI data io.Writer Write([]byte) (int, error)
	First      int              // , first : 3
	filterStmt string           // for printing filter expression
	Filter     *expr.Expression //
	Select     SelectList
	//
	//  Node data associated with stmt. Data stored as map with UUID of node, as key, and ds.NV containing  node attribute data.
	//
	nodes  NdNvMap
	nodesc NdNv
	nodesi NdIdx
	d      sync.Mutex
}

func (r *RootStmt) AssignName(input string, loc token.Pos) {
	//ValidateName(input, err, Loc)
	r.Name_ = name_{Name: input, Loc: loc}
}

func (r *RootStmt) AssignSelectList(s SelectList) {
	r.Select = s
}

func (r *RootStmt) Name() string {
	return "root"
}
func (r *RootStmt) AssignFilter(e *expr.Expression) {
	r.Filter = e
}

func (r *RootStmt) AssignFilterStmt(e string) {
	r.filterStmt = e
}

func (r *RootStmt) AssignVarName(input string, loc token.Pos) {
	//ValidateName(input, err, Loc)
	r.Var.AssignName(input, loc)
}

// func (r *RootStmt) hasNoData() bool {
// 	return r.nodes == nil
// }
func (r *RootStmt) Initialise() {
	r.nodes = make(NdNvMap)
	r.nodesc = make(NdNv)
	r.nodesi = make(NdIdx)
}

func (r *RootStmt) getnodes(uid string) (ds.NVmap, bool) {
	n, k := r.nodes[uid]
	return n, k
}

func (r *RootStmt) getnodesc(uid string) (ds.ClientNV, bool) {
	n, ok := r.nodesc[uid]
	return n, ok
}

func (r *RootStmt) assignData(key string, nvc ds.ClientNV, idx index) ds.NVmap {
	// create a NVmap
	nvm := make(ds.NVmap)
	for _, v := range nvc {
		nvm[v.Name] = v
	}
	// add to existing nodes on this edge
	r.d.Lock()
	r.nodes[key] = nvm
	r.nodesc[key] = nvc
	r.nodesi[key] = idx
	r.d.Unlock()

	return nvm
}

func (r *RootStmt) getData(key string) (ds.NVmap, ds.ClientNV, bool) {
	nvm, ok := r.nodes[key]
	nvc, ok := r.nodesc[key]
	return nvm, nvc, ok
}

func (r *RootStmt) getIdx(key string) (index, bool) {
	i, ok := r.nodesi[key]
	return i, ok
}

// genNV generates NV nodes based on type (parameter ty) passed in
func (r *RootStmt) genNV(ty string) ds.ClientNV {
	var nvc ds.ClientNV
	//
	// source: root filter expression
	//
	if r.Filter != nil {
		for _, x := range r.Filter.GetPredicates() {
			switch {
			case types.IsUidPredInTy(ty, x):
				nv := &ds.NV{Name: x + ":"}
				nvc = append(nvc, nv)
			case types.IsScalarInTy(ty, x):
				nv := &ds.NV{Name: x}
				nvc = append(nvc, nv)
			}
		}
	}
	//
	// source: select list
	//
	for _, v := range r.Select {

		switch x := v.Edge.(type) {

		case *ScalarPred:
			nv := &ds.NV{Name: x.Name()}
			nvc = append(nvc, nv)

		case *UidPred:
			var un string
			un = x.Name() + ":"
			nv := &ds.NV{Name: un}
			nvc = append(nvc, nv)

			for _, vv := range x.Select {
				switch x := vv.Edge.(type) {
				case *ScalarPred:
					upred := un + x.Name()
					nv := &ds.NV{Name: upred}
					nvc = append(nvc, nv)
				}
			}
			if x.Filter != nil {
				var found bool
				for _, v := range x.Filter.GetPredicates() {
					found = false
					for _, x := range nvc {
						if x.Name == un+v {
							found = true
							break
						}
					}
					if !found {
						// filter predicate not in select list - add NV entry but mark as invisible (ignore) so it is not output
						nv := &ds.NV{Name: un + v, Ignore: true}
						nvc = append(nvc, nv)
					}
				}
			}

			//
			// check for nested types with cardinality 1:1 and add them to []NV
			//
			var (
				aty blk.TyAttrD
				ok  bool
			)
			if aty, ok = types.TypeC.TyAttrC[ty+":"+x.Name()]; !ok {
				panic(fmt.Errorf("%s not in %s", ty, x.Name()))
				continue // ignore this attribute as it is in current type
			}
			paty := aty
			for _, v := range x.Select {

				switch y := v.Edge.(type) {

				case *UidPred:

					if aty, ok = types.TypeC.TyAttrC[paty.Ty+":"+y.Name()]; !ok {
						panic(fmt.Errorf("%s %s not in %s", aty.Name, aty.Ty, y.Name()))
						continue // ignore this attribute as it is in current type
					}
					fmt.Printf("AST yy: %s %s\n", aty.Ty, aty.Card)
					if aty.Card == "1:1" {
						un := x.Name() + ":" + y.Name() + ":"
						nv := &ds.NV{Name: un}
						nvc = append(nvc, nv)

						for _, v := range y.Select {

							switch z := v.Edge.(type) {

							case *ScalarPred:
								un += z.Name()
								nv := &ds.NV{Name: un}
								nvc = append(nvc, nv)
							}
						}
					}
				}
			}
			// var (
			// 	aty  blk.TyAttrD
			// 	aty2 blk.TyAttrD
			// 	ok   bool
			// )
			// fmt.Println("AST 1 : ", ty+":"+x.Name())
			// if aty, ok = types.TypeC.TyAttrC[ty+":"+x.Name()]; !ok {
			// 	panic(fmt.Errorf("%s not in %s", ty, x.Name()))
			// 	continue // ignore this attribute as it is in current type
			// }
			// fmt.Printf("AST: %#v\n", aty)
			// for _, v := range x.Select {

			// 	switch y := v.Edge.(type) {

			// 	case *UidPred:
			// 		fmt.Println("AST 2 : ", aty.Ty+":"+y.Name())

			// 		if aty2, ok = types.TypeC.TyAttrC[aty.Ty+":"+y.Name()]; !ok {
			// 			panic(fmt.Errorf("%s not in %s", aty.Ty, y.Name()))
			// 			continue // ignore this attribute as it is in current type
			// 		}
			// 		fmt.Printf("AST: %#v\n", aty2)
			// 		if aty2.Card == "1:1" {
			// 			fmt.Println("********************  1:1")
			// 			un := x.Name() + ":" + y.Name() + ":"
			// 			nv := &ds.NV{Name: un}
			// 			nvc = append(nvc, nv)

			// 			fmt.Println("AST: len(y.Select) ", len(y.Select))

			// 			for _, v := range y.Select {

			// 				switch z := v.Edge.(type) {

			// 				case *ScalarPred:
			// 					fmt.Println("AST: scalar ", z.Name())
			// 					un += z.Name()
			// 					nv := &ds.NV{Name: un}
			// 					nvc = append(nvc, nv)
			// 				}
			// 			}
			// 		}
			// 	}
			// }
		}
	}

	return dedup(nvc)
}

func (r *RootStmt) String() string {
	var s strings.Builder

	s.WriteByte('{')
	s.WriteByte('\n')
	s.WriteString(r.Name_.String())
	s.WriteString("(func: ")
	s.WriteString(r.RootFunc.String())
	if r.First > 0 {
		s.WriteString(",first : ")
		s.WriteString(strconv.Itoa(r.First))
	}
	s.WriteByte(')')
	if r.Filter != nil {
		s.WriteString("@filter( ")
		s.WriteString(r.filterStmt)
		s.WriteByte(')')
	}
	s.WriteString("{\n")
	s.WriteString(r.Select.String())
	s.WriteByte('}')

	s.WriteByte('}')

	return s.String()
}

// Predicates lists all predicates involved in the root stmt i.e. in RootFunc filter, and edges
// func (r *RootStmt) RetrievePredicates() []string {
// 	var s []string
// 	if r.RootFunc.Farg != nil {
// 		switch x := r.RootFunc.Farg.(type) {
// 		case *CountFunc:
// 			switch y := x.Arg.(type) {
// 			case *UidPred:
// 				s = append(s, y.Name.Name)
// 			}
// 		case ScalarPred:
// 			s = append(s, x.Name.Name)
// 		}
// 	}
// 	// s = append(s, r.Filter.GetPredicates()...)
// 	s = append(s, r.Select.GetPredicates()...)

// 	return s
// }

// func dedup(s []string) []string {
// 	var ss []string
// 	var found bool
// 	ss = append(ss, s[0])
// 	for _, e := range s[1:] {
// 		found = false
// 		for _, d := range ss {
// 			if d == e {
// 				found = true
// 				break
// 			}
// 		}
// 		if !found {
// 			ss = append(ss, e)
// 		}
// 	}
// 	return ss
// }

func dedup(s ds.ClientNV) ds.ClientNV {
	var ss ds.ClientNV
	var found bool
	ss = append(ss, s[0])
	for _, e := range s[1:] {
		found = false
		for _, d := range ss {
			if d.Name == e.Name {
				found = true
				break
			}
		}
		if !found {
			ss = append(ss, e)
		}
	}
	return ss
}

// ============== QResult ==============

// type QResult struct {
// 	UID   util.UID
// 	SortK string
// 	Ty    string
// }

// ============== NameI  ========================

type NameAssigner interface {
	AssignName(string, token.Pos)
}

// ===============  NameValue_  =========================

// type NameValue_ string

// func (n NameValue_) String() string {
// 	return string(n)
// }

// func (a NameValue_) Equals(b NameValue_) bool {
// 	return string(a) == string(b)
// }

// func (a NameValue_) EqualString(b string) bool {
// 	return string(a) == b
// }

// ===============  name_  =========================

type name_ struct {
	Name string
	Loc  token.Pos
}

func (n name_) String() string {
	return string(n.Name)
}

func (n name_) AtPosition() string {
	if n.Loc.Col == 0 && n.Loc.Line == 0 {
		//panic(fmt.Errorf("Error in AtPosition(), Loc not set"))
		return "Loc not set"
	}
	return "" + strconv.Itoa(n.Loc.Line) + " " + strconv.Itoa(n.Loc.Col) + "] "
}

func (n name_) Exists() bool {
	if len(n.Name) > 0 {
		return true
	}
	return false
}
