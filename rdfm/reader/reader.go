package reader

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"text/scanner"
	"unicode"

	slog "github.com/DynamoGraph/syslog"
	"github.com/DynamoGraph/util"
)

const (
	Director uint8 = 1
	Actor          = 2
)

// internal identifier
type IID = string

type PersonT struct {
	Id   IID
	Uid  util.UID
	Ty   uint8 // 1 : director, 2 : actor,  3: both
	Name string
	//
	ActorPerformance []*PerformanceT
	DirectorFilm     []*MovieT
	//
	sync.Mutex
}
type PersonMap map[IID]*PersonT // lookup using IID to get Person data + new UUID

type GenreT struct {
	Id   IID
	Uid  util.UID
	Name string
	sync.Mutex
}
type GenreMap map[IID]*GenreT

//type GenreMvMap map[string][]*MovieT // genre->movie

type CharacterT struct {
	Id   IID
	Uid  util.UID
	Name string
}
type CharacterMap map[IID]*CharacterT

type PerformanceT struct {
	Id        IID
	Uid       util.UID
	Film      *MovieT
	Actor     *PersonT
	Character *CharacterT
}
type performanceMap map[IID]*PerformanceT

type MovieT struct {
	Id   IID
	Uid  util.UID
	Name []string // array of names associated with film
	Ird  string
	//	Ird         time.Time. - ignore time datatype for loading purposes. treat as Time only when required otherwise stick to string.
	Genre       []*GenreT
	Director    []*PersonT
	Performance []*PerformanceT // aka "starring" in rdf file
}
type MovieMap map[IID]*MovieT

// transition structs between RDF tuple and dynamodb.AttributeValue

type Decoder struct {
	r     io.Reader
	batch uint // number of tuples in batch
}

func NewRDFloader(f io.Reader) *Decoder {
	return &Decoder{r: f}
}

func syslog(s string) {
	slog.Log("RDFreader: ", s)
}

type ndShortNm = string

// func openRDFfile(fname string) *os.File {
// 	f, err := os.Open(fname)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	return f
// }
//
// input rdf data - rdf file data. Sent on verify channel to be consumed by verification process
//

//ype RdfNodes

type RDFReader struct {
	bs   *bufio.Scanner
	ts   scanner.Scanner
	line int
}

type Reader interface {
	Read() error
}

var (
	Movie  MovieMap
	Person PersonMap
	Genre  GenreMap
	//	GenreMovies GenreMvMap
	Performance performanceMap
	Character   CharacterMap
	//
	filmLimit int // number of films to migrate
)

func init() {
	Movie = make(MovieMap)
	Person = make(PersonMap)
	Genre = make(GenreMap)
	//	GenreMovies = make(GenreMvMap)
	Performance = make(performanceMap)
	Character = make(CharacterMap)
}

func New(f io.Reader) Reader {

	rdf := new(RDFReader)
	//
	// create bufio (split file into lines) and text scanners (split lines into tokens)
	//
	rdf.bs = bufio.NewScanner(f)
	var s scanner.Scanner
	// treat leading `_:` as part of an identifier
	s.IsIdentRune = func(ch rune, i int) bool {
		return ch == '<' && i == 0 || ch == '>' && i > 0 || unicode.IsLetter(ch) || unicode.IsDigit(ch) && i > 0 || ch == '-' || ch == '.' || ch == '_'
	}
	rdf.ts = s
	//
	return rdf
}

type cur struct {
	obj, subj, pred string
	set             bool
}

func SetLimit(n int) {
	filmLimit = n
}

// Read read rdf file and bundles common tuples into nodes upto len(n) nodes and passes back via n.
// number of nodes in n is returnd plus error if any.
func (rn *RDFReader) Read() error {

	var (
		l string
	)
	defer DumpErrs()

	for rn.bs.Scan() {

		rn.line++

		l = strings.Trim(rn.bs.Text(), " ")
		l = strings.Trim(rn.bs.Text(), string(9))
		if len(l) == 0 {
			continue
		}
		if l[0] == '#' {
			fmt.Printf("xxfound  %q\n", l)
			l = strings.Trim(l[1:], " -")
			l = strings.Trim(l, "-")
			l = strings.Trim(l, " ")
		}
		fmt.Println("Process: ", l[:5])

		switch l[:5] {

		case "direc":
			rn.loadDirectors()

		case "actor":
			rn.loadActors()

		case "genre":
			rn.loadGenre()

		case "movie":
			rn.loadMovies()
			// not interested in contents of file after movie rdfs as they are all type rdfs only
			// for k, v := range GenreMovies {
			// 	fmt.Printf("GenreMovies: %s  len %d", k, len(v))
			// 	for _, vv := range v {
			// 		fmt.Println("vv.Name: ", vv.Uid, vv.Name)
			// 	}
			// }
			return nil

		default:
			return fmt.Errorf("Inconsistency: expected movie genre, actor, director got %s", l[:5])
		}

	}
	return nil
}

func (rn *RDFReader) loadDirectors() {
	var (
		i               = 0
		l               string
		subj, pred, obj string
		dot             string
	)
	for rn.bs.Scan() {
		i++
		l = strings.Trim(l, " ")
		l = strings.Trim(rn.bs.Text(), string(9))

		if len(l) == 0 {
			continue
		}
		if l[0] == '#' {
			break
		}
		//fmt.Println("x: ", l)
		rn.ts.Init(strings.NewReader(l))
		//fmt.Println("In LoadDirectors...")
		//s.Mode ^= scanner.SkipComments // disable skip comments is enabled by default. Xor will toggle bit 10 to 0 to enable comment display

		subj, pred, obj, dot = "", "", "", ""

		for i, tok := 0, rn.ts.Scan(); tok != scanner.EOF; tok = rn.ts.Scan() {

			//fmt.Println("In text/scanner: ", rn.ts.TokenText())
			switch i {
			case 0:
				subj = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 1:
				pred = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 2:
				obj = rn.ts.TokenText()
				switch obj[0] {
				case '"':
					obj = strings.Trim(obj, string('"'))
				case '<':
					obj = strings.TrimLeft(strings.TrimRight(obj, ">"), "<")
				}
			default:
				dot = rn.ts.TokenText()
			}
			i++
		}
		//fmt.Println(">> ", subj, pred, obj)
		if dot != "." {
			saveErr(fmt.Errorf("Load Directors errored: expected . got ", dot))
		}
		uid, err := util.MakeUID()
		if err != nil {
			saveErr(err)
		}
		if pred == "name" {
			Person[IID(subj)] = &PersonT{Id: IID(subj), Name: obj, Uid: uid, Ty: 1}
		} else {
			saveErr(fmt.Errorf("LoadDirectors: expected a predicate of name got %q", pred))
		}

	}

	syslog(fmt.Sprintf("Persons: %d", len(Person)))

}

func (rn *RDFReader) loadActors() {
	var (
		i               = 0
		l               string
		subj, pred, obj string
		dot             string
	)

	fmt.Println("In Loadactors...")
	for rn.bs.Scan() {
		i++
		l = strings.Trim(l, " ")
		l = strings.Trim(rn.bs.Text(), string(9))
		//fmt.Printf("rn.line %d, %q\n", i, l)
		if len(l) == 0 {
			continue
		}
		//	fmt.Printf("rn.line %d, %q\n", i, l)
		if l[0] == '#' {
			break
		}
		//fmt.Printf("init  %q\n", l)
		rn.ts.Init(strings.NewReader(l))

		//s.Mode ^= scanner.SkipComments // disable skip comments is enabled by default. Xor will toggle bit 10 to 0 to enable comment display

		subj, pred, obj, dot = "", "", "", ""

		for i, tok := 0, rn.ts.Scan(); tok != scanner.EOF; tok = rn.ts.Scan() {

			//fmt.Println("In text/scanner: ", rn.ts.TokenText())
			switch i {
			case 0:
				subj = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 1:
				pred = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 2:
				obj = rn.ts.TokenText()
				switch obj[0] {
				case '"':
					obj = strings.Trim(obj, string('"'))
				case '<':
					obj = strings.TrimLeft(strings.TrimRight(obj, ">"), "<")
				}
			default:
				dot = rn.ts.TokenText()
			}
			i++
		}
		if dot != "." {
			saveErr(fmt.Errorf("LoadActors errored: expected . but got ", dot))
		}
		uid, err := util.MakeUID()
		if err != nil {
			saveErr(err)
		}
		if pred == "name" {
			if v, ok := Person[IID(subj)]; ok {
				// person is a director
				v.Ty |= Actor
			} else {
				Person[IID(subj)] = &PersonT{Id: IID(subj), Name: obj, Uid: uid, Ty: 1 << (Actor - 1)}
			}
		} else {
			saveErr(fmt.Errorf("LoadDirectors: expected a predicate of name got %s", pred))
		}

	}
	// count actors who are also diectors
	var m, j, k int
	for _, v := range Person {
		switch v.Ty {
		case 1: // directors only
			k++
		case 2: // actors
			j++
		case 3: // actors & directors
			m++
		}
		if v.Ty != 2 {
			i++
		}
	}
	syslog(fmt.Sprintf("Verification: Persons: %d", len(Person)))
	syslog(fmt.Sprintf("Verification: Directors only: %d", k))
	syslog(fmt.Sprintf("Verification: Actors only: %d", j))
	syslog(fmt.Sprintf("Verification: actors & directors: %d", m))

}

func (rn *RDFReader) loadGenre() {
	var (
		i               = 0
		l               string
		subj, pred, obj string
		dot             string
	)
	fmt.Println("loadGenre......")
	for rn.bs.Scan() {
		i++
		l = strings.Trim(l, " ")
		l = strings.Trim(rn.bs.Text(), string(9))
		//fmt.Printf("rn.line %d, %q\n", i, l)
		if len(l) == 0 {
			continue
		}
		//	fmt.Printf("rn.line %d, %q\n", i, l)
		if l[0] == '#' {
			break
		}
		//	fmt.Printf("init  %q\n", l)
		rn.ts.Init(strings.NewReader(l))

		//s.Mode ^= scanner.SkipComments // disable skip comments is enabled by default. Xor will toggle bit 10 to 0 to enable comment display

		subj, pred, obj, dot = "", "", "", ""

		for i, tok := 0, rn.ts.Scan(); tok != scanner.EOF; tok = rn.ts.Scan() {

			//	fmt.Println("In text/scanner: ", rn.ts.TokenText())
			switch i {
			case 0:
				subj = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 1:
				pred = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 2:
				obj = rn.ts.TokenText()
				switch obj[0] {
				case '"':
					obj = strings.Trim(obj, string('"'))
				case '<':
					obj = strings.TrimLeft(strings.TrimRight(obj, ">"), "<")
				}
			default:
				dot = rn.ts.TokenText()
			}
			i++
		}
		if dot != "." {
			saveErr(fmt.Errorf("LoadGenre errored: expected . but got ", dot))
		}
		uid, err := util.MakeUID()
		if err != nil {
			saveErr(err)
		}
		if pred == "name" {
			Genre[IID(subj)] = &GenreT{Id: IID(subj), Name: obj, Uid: uid}
		} else {
			saveErr(fmt.Errorf("LoadDirectors: expected a predicate of name got %s", pred))
		}

	}
	syslog(fmt.Sprintf("Verification: Genres: %d", len(Genre)))
}

func (rn *RDFReader) loadMovies() error {
	var (
		l               string
		subj, pred, obj string
		dot             string
		newMovie        *MovieT
		newPerformance  *PerformanceT
		nameWillbeNew   bool = true
		i               int
	)
	fmt.Printf("in Loadmovies:......")
	for rn.bs.Scan() {

		l = strings.Trim(l, " ")
		l = strings.Trim(rn.bs.Text(), string(9))
		//fmt.Printf("rn.line %d, %q\n", i, l)
		//fmt.Println("loadMovies: l=", l)
		if len(l) == 0 {
			continue
		}
		//fmt.Printf("rn.line %d, %q\n", i, l)
		if l[0] == '#' {
			fmt.Println("loadMovies: break ")
			break
		}

		////fmt.Printf("Loadmovies: %q\n", l)
		rn.ts.Init(strings.NewReader(l))

		//s.Mode ^= scanner.SkipComments // disable skip comments is enabled by default. Xor will toggle bit 10 to 0 to enable comment display

		subj, pred, obj, dot = "", "", "", ""

		for i, tok := 0, rn.ts.Scan(); tok != scanner.EOF; tok = rn.ts.Scan() {

			//fmt.Println("In text/scanner: ", rn.ts.TokenText())
			switch i {
			case 0:
				subj = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 1:
				pred = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 2:
				obj = rn.ts.TokenText()
				switch obj[0] {
				case '"':
					obj = strings.Trim(obj, string('"'))
				case '<':
					obj = strings.TrimLeft(strings.TrimRight(obj, ">"), "<")
				}
			default:
				dot = rn.ts.TokenText()
			}
			i++
		}
		if dot != "." {
			saveErr(fmt.Errorf("LoadMovie errored: expected . but got ", dot))
		}
		//	fmt.Println("Movies: i,subj,pred,obj: ", i, subj, pred, obj)
		//

		switch pred {

		case "name": // film name - TODO: shoud be slice to hold multi values
			if nameWillbeNew {
				//
				// Load ALL movies
				//
				i++
				if i > filmLimit {
					return nil
				}
				//fmt.Println("LoadMovie: ",i)
				uid, err := util.MakeUID()
				if err != nil {
					saveErr(err)
				}
				nameWillbeNew = false
				newMovie = &MovieT{Uid: uid, Id: IID(subj)}
				newMovie.Name = append(newMovie.Name, obj)
				//
				Movie[IID(subj)] = newMovie
				//	fmt.Printf("%s\n", strings.Repeat("_", 65))
			} else {
				newMovie.Name = append(newMovie.Name, obj)
			}

		case "initial_release_date":
			nameWillbeNew = true
			newMovie.Ird = obj

		case "director.film":
			nameWillbeNew = true
			if d, ok := Person[IID(subj)]; !ok {
				saveErr(fmt.Errorf("LoadMovies: Person (director) not found %q", subj))
			} else {
				newMovie.Director = append(newMovie.Director, d)
				d.DirectorFilm = append(d.DirectorFilm, newMovie)
			}
			//newMovie.Director = append(newMovie.Director, IID(subj))

		case "genre":
			nameWillbeNew = true
			if d, ok := Genre[IID(obj)]; !ok {
				saveErr(fmt.Errorf("LoadMovies: Person (director) not found %q", subj))
			} else {
				newMovie.Genre = append(newMovie.Genre, d)
			}
			//newMovie.Genre = append(newMovie.Genre, IID(obj))
			//GenreMovies[obj] = append(GenreMovies[obj], newMovie)

		case "starring":
			nameWillbeNew = true
			uid, _ := util.MakeUID()
			newPerformance = &PerformanceT{Id: IID(obj), Uid: uid, Film: newMovie}
			Performance[IID(obj)] = newPerformance
			newMovie.Performance = append(newMovie.Performance, newPerformance)
			//:q!fmt.Printf("Movie so far: %#v\n", *newMovie)
			newPerformance.loadPerformance(rn, newMovie.Id)

		default:
			saveErr(fmt.Errorf("LoadMovie predicate %s not expected ", pred))
		}

	}
	// fmt.Printf("newMovie.performance: %#v\n", newMovie.performance)
	//	rn.bs.Scan()
	// type MovieT struct {
	// Id          IID
	// Uid         util.UID
	// Name        []string
	// Ird         string
	// Genre       []IID
	// Director    []IID
	// Performance []*PerformanceT

	syslog(fmt.Sprintf("Verification: Movies(films): %d", len(Movie)))
	return nil
}

func (p *PerformanceT) loadPerformance(rn *RDFReader, movieId IID) error {

	var (
		i               = 0
		l               string
		subj, pred, obj string
		dot             string
	)
	var ii int8
	for rn.bs.Scan() {
		i++
		l = strings.Trim(l, " ")
		l = strings.Trim(rn.bs.Text(), string(9))
		//fmt.Printf("rn.line %d, %q\n", i, l)
		if len(l) == 0 {
			continue
		}
		//fmt.Printf("In Performance:  rn.line %d, %q\n", i, l)
		if l[0] == '#' {
			break
		}
		//		fmt.Printf("loadPerformance:  %q\n", l)
		rn.ts.Init(strings.NewReader(l))

		//s.Mode ^= scanner.SkipComments // disable skip comments is enabled by default. Xor will toggle bit 10 to 0 to enable comment display

		subj, pred, obj, dot = "", "", "", ""

		for i, tok := 0, rn.ts.Scan(); tok != scanner.EOF; tok = rn.ts.Scan() {

			//fmt.Println("=========================== In text/scanner: ", rn.ts.TokenText())
			switch i {
			case 0:
				subj = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 1:
				pred = strings.TrimLeft(strings.TrimRight(rn.ts.TokenText(), ">"), "<")
			case 2:
				obj = rn.ts.TokenText()
				switch obj[0] {
				case '"':
					obj = strings.Trim(obj, string('"'))
				case '<':
					obj = strings.TrimLeft(strings.TrimRight(obj, ">"), "<")
				}
			default:
				dot = rn.ts.TokenText()
			}
			i++
		}
		if dot != "." {
			fmt.Println("Errored: expected . but got ", dot)
		}
		//fmt.Println("Performance: ", i, subj, pred, obj)
		//
		ii++
		switch pred {

		case "name":
			p.Character.Name = obj // character name - presuming it is the last performance entry - need to find a better way to terminate loadPerformance.

		case "performance.film":
			//p.Film = IID(obj)

			if m, ok := Movie[IID(obj)]; !ok {
				saveErr(fmt.Errorf("Expected film id of %q got %q", p.Id, subj))
			} else {
				p.Film = m
			}

		case "performance.actor":
			if person, ok := Person[IID(obj)]; !ok {
				saveErr(fmt.Errorf("Actor does not exist in person map %q", subj))
			} else {
				// check person is an actor
				if person.Ty&2 != 2 {
					saveErr(fmt.Errorf("Expected actor but got director ", p.Id, subj))
				}
				/// check obj is current performance id
				if IID(obj) != p.Id {
					saveErr(fmt.Errorf("Expected performance id of %q got %q", p.Id, subj))
				}
				// add actor to performance
				//fmt.Println("============ performance.actor set =========")
				p.Actor = person
			}

		case "actor.film": // should be actor.performance - belongs to person node

			// validate subj is Person(actor) and obj is a Performance
			// if p.Actor != IID(subj) {
			// 	fmt.Printf("Expected actor id of %q got %q", p.Actor, subj)
			// }
			// if p.IID != IID(obj) {
			// 	fmt.Printf("Expected film id of %q got %q", p.Actor, subj)
			// }
			if person, ok := Person[IID(subj)]; !ok {
				saveErr(fmt.Errorf("Actor does not exist"))
			} else {
				// check person is an actor
				if person.Ty&2 != 2 {
					saveErr(fmt.Errorf("Expected actor but got director ", p.Id, subj))
				}
				/// check obj is current performance id
				if IID(obj) != p.Id {
					saveErr(fmt.Errorf("Expected performance id of %q got %q", p.Id, subj))
				}
				// add performance to actor
				person.ActorPerformance = append(person.ActorPerformance, p)
			}
			//

		case "performance.character":
			uid, _ := util.MakeUID()
			p.Character = &CharacterT{Id: IID(obj), Uid: uid}
			Character[IID(obj)] = p.Character

		default:
			break
		}
		if ii == 5 {
			//fmt.Println("FInished loadPerformance  " + strings.Repeat("-", 55))
			break
		}

	}

	return nil
}

var dataErrs []error

func saveErr(err error) {
	dataErrs = append(dataErrs)
	if len(dataErrs) > 10 {
		DumpErrs()
		os.Exit(1)
	}
}

func DumpErrs() {
	if len(dataErrs) == 0 {
		fmt.Println("*** No errors ***")
		syslog("*** No errors ***")
		return
	}
	for _, v := range dataErrs {
		syslog(fmt.Sprintf("Errors: %s", v.Error()))
		fmt.Println(v.Error())
	}
}
