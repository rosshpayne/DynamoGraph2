// package expression only used for filter operations
// rootfunc needs only lexer/parser to determine what func to execute, which returns []{[]util.UID, Type}, where each item is passed to expression to return true/false
package expression

import (
	"fmt"

	"github.com/DynamoGraph/gql/expression/ast"
	"github.com/DynamoGraph/gql/expression/token"
)

// @filter(allofterms(name@en, "jones indiana") OR allofterms(name@en, "jurassic park"))

const (
	LPAREN uint8 = 1
)

const (
	NIL operator = "-"
)

var regFunc map[string]ast.FuncT

func init() {

	register := func(t string, f ast.FuncT) {
		regFunc[t] = f
	}

	regFunc = make(map[string]ast.FuncT)

	register(token.EQ, ast.EQ)
	register(token.LT, ast.LT)
	register(token.GT, ast.GT)
	register(token.GE, ast.GE)
	register(token.LE, ast.LE)
	register(token.HAS, ast.HAS)
	register(token.UID, ast.UID)
	register(token.UID_IN, ast.UID_IN)
	register(token.ANYOFTERMS, ast.AnyOfTerms)
	register(token.ALLOFTERMS, ast.AllOfTerms)
}

func New(input string) *Expression {

	type state struct {
		opr token.TokenType
	}

	var (
		tok         *token.Token
		loperand    *FilterFunc
		roperand    *FilterFunc
		operandL    bool            // put next INT in numL
		extendRight bool            // Used when a higher precedence operation detected. Assigns the latest Expression to the right operand of the current Expression.
		opr         token.TokenType // string
		opr_        token.TokenType // state held copy of opr
		e, en       *Expression     // "e" points to current Expression in graph while "en" is the latest Expression to be created and added to the graph using addParent() or extendRight() functions.
		lp          []state
	)

	pushState := func() {

		s := state{opr: opr}
		lp = append(lp, s)
	}

	popState := func() {
		var s state
		if len(lp) == 0 {
			//TODO - handle errors more gracefully
			panic(fmt.Errorf("redundant ) at end of expression"))
		}
		s, lp = lp[len(lp)-1], lp[:len(lp)-1]
		opr_ = s.opr
	}

	// as the parser processes the input left to right it builds a tree (graph) by creating an Expression as each operator is parsed and then immediately links
	// it to the previous Expression. If the Expression is at the same precedence level it links the new Expression as the parent of the current Expression. In the case
	// of higher precedence operations it links to the right of the current Expression (func: extendRight). Walking the tree and evaluating each Expression returns the final result.

	fmt.Printf("\n In expression. create new parser for: [%s] \n", input)

	//l := lexer.New(input)
	p := NewParser(input)
	operandL = true

	// TODO - initial full parse to validate left and right parenthesis match

	for p.curToken.Type != token.EOF {

		// expression parsing has a requirement to have double peek ("abc", a is current, b is peek, c is double peek) capability.
		// to achieve this with code designed for single peek capability is inelegant but works
		// save current token to tok
		// generate the next Token (via nexToken()), which normally would be the current Token but in this scenario is the peek token
		// this leads to the peek token (peekToken) enabling a double-peek ahead.
		//
		tok = p.curToken
		p.nextToken()
		//fmt.Printf("\n\n >>> Loop token: \ntok  %#v \n peekToken: %#v\n", tok, p.curToken)

		switch tok.Type {
		case token.EOF:
			break
		case token.LPAREN:
			//
			// LPAREN is represented in the graph by a "NULL" Expression (node) consisting of operator "+" and left operand of 1.
			//
			// or ( true
			//

			pushState()

			if opr != "" {

				if loperand != nil {

					en, opr = makeExpr(loperand, opr, nil)
					if e == nil {
						e, en = en, nil
					} else {
						e = e.extendRight(en)
					}

				} else {

					en, opr = makeExpr(nil, opr, nil)
					if e == nil {
						e, en = en, nil
					} else {
						e = e.addParent(en)
					}
				}
			}
			//
			// add NULL Expression representing "(". Following operation will be extend Right.
			//
			en = &Expression{left: nil, opr: token.TokenType("-"), right: nil}
			if e == nil {
				e, en = en, nil
			} else {
				e = e.extendRight(en)
			}
			fmt.Printf("e: %#v", *e)
			extendRight = true
			operandL = true

		case token.RPAREN:

			popState()

			// navigate current Expression e, up to next LPARAM Expression
			for e = e.parent; e.parent != nil && e.opr != "-"; e = e.parent {
				fmt.Printf("xRPAREN: %s\n", e.opr)
			}

			if e.parent != nil && e.parent.opr != "-" {
				// opr_ represents the operator that existed at the associated "(". Sourced from state.
				if opr_ == "AND" {
					fmt.Println("opr_ is adjusting to e.parent ", opr_)
					e = e.parent
				}
			}

		//		case token.TRUE, token.FALSE: // this will be functions that return true/false
		case token.FUNC:

			d := &FilterFunc{}
			//	fmt.Printf(" >>>> token.FUNC d: %#v   tok: %#v\n", d, tok)
			p.ParseFunction(d, tok)

			//	for ;p.curToken.Type != token.LBRACE; p.curToken.nextToken(){}
			//fmt.Printf("************.  back to input parser: \ntok: %#v,  \npeekToken: %#v   \nopr: %s\n", tok, p.curToken, opr)

			//
			// look ahead to next operator and check for higher precedence operation
			//
			tok := p.curToken // tok represents the Peek token
			if opr == token.OR && tok.Type == token.AND {
				//
				if extendRight {
					en, opr = makeExpr(loperand, opr, nil)
					if e == nil {
						e, en = en, nil
					} else {
						e = e.extendRight(en)
						extendRight = false
					}

				} else if loperand == nil {
					// add operator only node to graph - no left, right operands. addParent will attach left, and future ExtendRIght will attach right.
					en, opr = makeExpr(nil, opr, nil)
					e = e.addParent(en)

				} else {
					// make expr for existing numL and opr
					en, opr = makeExpr(loperand, opr, nil)
					if e == nil {
						e, en = en, nil
					} else {
						e = e.extendRight(en)
					}
				}
				// all higher precedence operations or explicit (), perform an "extendRight" to create a new branch in the graph.
				extendRight = true
				// new branches begin with a left operand
				operandL = true
			}

			if operandL {

				//fmt.Printf("*** loperand = %#v  \nfunc: %#v\n", d, *d.gqlFunc)
				loperand = d
				operandL = false

			} else {

				roperand = d
				if loperand != nil {

					en, opr = makeExpr(loperand, opr, roperand)
					if e == nil {
						e, en = en, nil
					} else {
						e = e.extendRight(en)
					}

				} else {

					en, opr = makeExpr(nil, opr, roperand)

					if extendRight {
						e = e.extendRight(en)
					} else {
						e = e.addParent(en)
					}

				}
				extendRight = false
				operandL = false
				loperand = nil
			}

		case token.OR, token.AND:

			opr = tok.Type

		case token.NOT:

			// handle any operands not formed into Expression
			if loperand != nil {
				en, opr = makeExpr(loperand, opr, nil)
				if e == nil {
					e, en = en, nil
				} else {
					e = e.extendRight(en)
				}
				extendRight = true
			}
			// create NOT node (Expression)
			en, opr = makeExpr(nil, token.NOT, nil)
			if e == nil {
				e, en = en, nil
			} else {
				if extendRight {
					e = e.extendRight(en)
				} else {
					e = e.addParent(en)
				}
			}
			// make following Expression extend right after NOT Expression
			extendRight = true
			operandL = false

		}
		if tok.Type == token.EOF {
			break
		}

	}
	if e == nil {
		// not boolean expression just a bool - create a dummy expression
		e, _ = makeExpr(loperand, token.NOOP, &FilterFunc{value: true})
		//fmt.Printf("Dummy expression  %#v\n", e)
		return e

	}
	findRoot(e)
	//fmt.Printf("Root %#v\n", x)
	return findRoot(e)
}
