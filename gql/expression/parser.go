package expression

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DynamoGraph/gql/expression/ast"
	"github.com/DynamoGraph/gql/expression/lexer"
	"github.com/DynamoGraph/gql/expression/token"
	"github.com/DynamoGraph/types"
)

type (
	Parser struct {
		l *lexer.Lexer

		extend bool

		abort     bool
		stmtType  string
		curToken  *token.Token
		peekToken *token.Token

		perror []error
	}
)

var (
	opt bool = true
)

const (
	FILTER = "filter"
	ROOT   = "root"
)

//

func NewParser(input string) *Parser {
	p := &Parser{}
	p.l = lexer.New(input)
	// Read two tokens, to initialise curToken and peekToken
	p.nextToken()
	p.nextToken()
	//ast.CacheClear()
	return p
}

func (p *Parser) printToken(s ...string) {
	if len(s) > 0 {
		fmt.Printf("** Current Token: [%s] %s %s %v %s [%s]\n", s[0], p.curToken.Type, p.curToken.Literal, "Next Token:  ", p.peekToken.Type, p.peekToken.Literal)
	} else {
		fmt.Println("** Current Token: ", p.curToken.Type, p.curToken.Literal, "Next Token:  ", p.peekToken.Type, p.peekToken.Literal)
	}
}

func (p *Parser) hasError() bool {
	if len(p.perror) > 17 || p.abort {
		return true
	}
	return false
}

// addErr appends to error slice held in parser.
func (p *Parser) addErr(s string) error {
	if strings.Index(s, " at line: ") == -1 {
		s += fmt.Sprintf(" at line: %d, column: %d", p.curToken.Loc.Line, p.curToken.Loc.Col)
	}
	e := errors.New(s)
	p.perror = append(p.perror, e)
	return e
}

func (p *Parser) nextToken(s ...string) {
	p.curToken = p.peekToken

	p.peekToken = p.l.NextToken() // get another token from lexer:    [,+,(,99,Identifier,keyword etc.
	if len(s) > 0 {
		fmt.Printf("** Current Token: [%s] %s %s %s %s %s\n", s[0], p.curToken.Type, p.curToken.Literal, "Next Token:  ", p.peekToken.Type, p.peekToken.Literal)
	}
	if p.curToken != nil {
		if p.curToken.Illegal {
			p.addErr(fmt.Sprintf("Illegal %s token, [%s]", p.curToken.Type, p.curToken.Literal))
		}
	}
}

// @filter(has(director.film)) {
// @filter(allofterms(name, "jones indiana"))
// @filter(alloftext(predicate, "space-separated text")
// @filter(eq(val(films), [1,2,3]))
// @filter(uid(<uid1>, ..., <uidn>))
// @filter(uid(a)) for variable a
// @filter(uid_in(predicate2, <uid>))
// @filter(eq(predicate, value)
// @filter(eq(val(varName), value)
// @filter(eq(predicate, val(varName))
// @filter(eq(count(predicate), value)
// @filter(eq(predicate, [val1, val2, ..., valN])
// @filter(eq(predicate, [$var1, "value", ..., $varN])

// @filter(eq(count(uid-pred),5)

// ParseFunction parses the function components of a complete boolean expression e.g. f1 and ( f2 or f3)
// f1,f2,f3 return true/false based on the graph data (in the nv struct) and type passed into them

func (p *Parser) ParseFunction(s *FilterFunc, tc *token.Token) *Parser {

	//
	fmt.Printf("in ParseFunction: d,s=%#v    curToken:  %#v    tc=%#v \n", s, p.curToken, tc)

	gqlf := &ast.GQLFunc{}
	gqlf.AssignName(tc.Literal, tc.Loc)
	s.gqlFunc = gqlf

	switch token.TokenType(tc.Literal) {

	case token.IDENT:
		//
		gqlf.F = regFunc[p.curToken.Literal]
		p.nextToken() // read over rfunc
		//
		// argument: scalar-pred, val(<varName>), count(<uid-pred>),
		switch token.TokenType(tc.Literal) {
		case token.IDENT:

			if !types.IsScalarPred(p.curToken.Literal) {
				p.addErr(fmt.Sprintf("%s is not a scalar predicate", p.curToken.Literal))
			}
			pred := ast.ScalarPred{}
			pred.AssignName(p.curToken.Literal, p.curToken.Loc)
			gqlf.Farg = pred

		case token.VAL:
			p.nextToken() // read over val
			if p.curToken.Type != token.LPAREN {
				p.addErr(fmt.Sprintf(`expected ( but got %q`, p.curToken.Literal))
			}
			p.nextToken() // read over (
			v := ast.Variable{}
			v.AssignName(p.curToken.Literal, p.curToken.Loc)
			gqlf.Farg = v

		case token.COUNT:
			// count(<uid-pred>) // TODO: is that all for count
			p.nextToken() // read over count
			if !types.IsUidPred(p.curToken.Literal) {
				p.addErr(fmt.Sprintf("%s must be a uid predicate to appear in count function", p.curToken.Literal))
			}
			p.nextToken() // read over (
			c := ast.CountFunc{}
			v := &ast.UidPred{}
			v.AssignName(p.curToken.Literal, p.curToken.Loc)
			c.Arg = v
			gqlf.Farg = c
		}

	case token.GT, token.GE, token.LE, token.LT, token.EQ:
		// gt(<scalar pred>, <int|float|string>)
		var err error
		p.nextToken() // read over (

		h := ast.ScalarPred{}
		h.AssignName(p.curToken.Literal, p.curToken.Loc)

		switch tc.Literal {
		case token.GT:
			gqlf.F = ast.GT
		case token.GE:
			gqlf.F = ast.GE
		case token.LE:
			gqlf.F = ast.LE
		case token.LT:
			gqlf.F = ast.LT
		case token.EQ:
			gqlf.F = ast.EQ
		}
		gqlf.Farg = h
		// TODO: assigning to value is performed at end of switch. Is the code below redundant? Hence the return.
		p.nextToken() // read over scalar pred
		fmt.Printf("curToken: %#v\n", p.curToken)
		switch p.curToken.Type {
		case token.INT:
			if gqlf.Value, err = strconv.Atoi(p.curToken.Literal); err != nil {
				panic(fmt.Errorf("cannot convert %s to int", p.curToken.Literal))
			}
		case token.FLOAT:
			if gqlf.Value, err = strconv.ParseFloat(p.curToken.Literal, 64); err != nil {
				panic(fmt.Errorf("cannot convert %s to int", p.curToken.Literal))
			}
		case token.STRING:
			gqlf.Value = p.curToken.Literal
		default:
			panic(fmt.Errorf("Expected a float, string or int as value to function not %s, %s", p.curToken.Type, p.curToken.Literal))
		}
		p.nextToken() // read over has value
		p.nextToken() // read over )
		fmt.Printf(" %s ..................... \ngqlf: %#v  \np.curToken: %#v \n", tc.Literal, gqlf, p.curToken)
		return p

	case token.HAS:
		// has(<any pred>)

		p.nextToken() // read over (

		gqlf.F = ast.HAS

		switch p.curToken.Type {

		case token.IDENT:
			switch {
			case types.IsScalarPred(p.curToken.Literal):

				s := ast.ScalarPred{}
				s.AssignName(p.curToken.Literal, p.curToken.Loc)
				gqlf.Farg = s

			case types.IsUidPred(p.curToken.Literal):

				// if gqlf.Name() != token.HAS {
				// 	p.addErr(fmt.Sprintf(`UID Predicates only allowed as argument to Has()`))
				// 	return p
				// }
				s := &ast.UidPred{}
				s.AssignName(p.curToken.Literal, p.curToken.Loc)
				gqlf.Farg = s

			}
		}

		p.nextToken() // read over ident
		p.nextToken() // read over has )
		return p

	case token.UID:
		// uid(<uid1>, ..., <uidn>))
		// uid(a)) for variable a
		u := ast.Uid{}
		us := []string{}
		for p.nextToken(); p.curToken.Type == token.RPAREN; p.nextToken() {
			us = append(us, p.curToken.Literal)
		}
		u.Uids = us

		gqlf.F = ast.UID
		gqlf.Farg = u

	case token.UID_IN:
		// uid_in(<uid-pred>),
		p.nextToken() // read over uid_in
		if !types.IsUidPred(p.curToken.Literal) {
			p.addErr(fmt.Sprintf("predicate, %q must be a uid predicate when used in the uid_in function"))
		}
		uin := ast.Uid_IN{}
		upred := &ast.UidPred{}
		upred.AssignName(p.curToken.Literal, p.curToken.Loc)
		uin.Pred = upred

		gqlf.F = ast.UID_IN
		gqlf.Farg = uin

	case token.ALLOFTERMS, token.ANYOFTERMS:

		p.nextToken() // read over (
		h := ast.ScalarPred{}
		h.AssignName(p.curToken.Literal, p.curToken.Loc)

		p.nextToken() // read over argument name
		switch token.TokenType(tc.Literal) {
		case token.ALLOFTERMS:
			gqlf.F = ast.AllOfTerms
		case token.ANYOFTERMS:
			gqlf.F = ast.AnyOfTerms
		}
		gqlf.Farg = h
	}
	// parse value

	switch p.curToken.Type {
	case token.STRING:
		gqlf.Value = p.curToken.Literal
	case token.INT:
		i, _ := strconv.Atoi(p.curToken.Literal)
		gqlf.Value = i
	case token.FLOAT:
		f, err := strconv.ParseFloat(p.curToken.Literal, 64)
		if err != nil {
			p.addErr(fmt.Sprintf(`Errored in converting literal, %q, to float64. %s`, p.curToken.Literal, err.Error()))
			return p
		}
		gqlf.Value = f
	case token.LBRACKET:
		// TODO implement var, var, var,....
	default:
		p.addErr(fmt.Sprintf(`Expected a string or number got %s instead`, p.curToken.Literal))
		return p
	}
	p.nextToken() // read over value

	for i := 0; i < 2; i++ {
		if p.curToken.Type != token.RPAREN {
			p.addErr(fmt.Sprintf(`Expected )  got %s instead`, p.curToken.Literal))
			return p
		}
		p.nextToken() // read over )
	}
	return p

}
