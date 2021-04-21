package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DynamoGraph/rdfm/reader"
	slog "github.com/DynamoGraph/syslog"
)

var inputFile = flag.String("f", "rdf_test.rdf", "RDF Filename ")
var numFilms = flag.Int("n", 0, "Number of Films to migrate")

func syslog(s string) {
	slog.Log("rdfLoader: ", s)
}

// uid PKey of the sname-UID pairs - consumed and populated by the SaveRDFNode()

//func Load(f io.Reader) error {
func main() {

	//
	flag.Parse()
	fmt.Println("inputfile: ", *inputFile)
	fmt.Println("films to migrate: ", *numFilms)
	if *numFilms == 0 || len(*inputFile) == 0 {
		flag.PrintDefaults()
		return
	}
	f, err := os.Open(*inputFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	reader.SetLimit(*numFilms)

	rdr := reader.New(f)
	//
	slog.Off()
	t0 := time.Now()
	//
	// read RDF file
	//
	err = rdr.Read()
	if err != nil {
		fmt.Println(err)
		return
	}
	//
	t1 := time.Now()
	//
	frdf, err := os.Create("generated.rdf")
	if err != nil {
		panic(err)
	}
	//
	// generate RDF file in my format
	//
	// Person
	//
	var s strings.Builder
	s.WriteString(" __type ")
	s.WriteString(fmt.Sprintf("%q", "Person"))
	s.WriteString("  .\n")
	ty := s.String()

	for _, p := range reader.Person { //person {
		s.Reset()
		s.WriteString("_:")
		s.WriteString(p.Id)
		id := s.String()
		s.WriteString(ty)
		s.WriteString(id)
		s.WriteString(" name ")
		s.WriteString(fmt.Sprintf(`%q  .`, p.Name))
		s.WriteString("\n")
		for _, pf := range p.ActorPerformance {
			s.WriteString(id)
			s.WriteString(" actor.performance ")
			s.WriteString("_:")
			s.WriteString(pf.Id)
			s.WriteString("  .\n")
		}
		for _, ap := range p.DirectorFilm {
			s.WriteString(id)
			s.WriteString(" director.film ")
			s.WriteString("_:")
			s.WriteString(ap.Id)
			s.WriteString("  .\n")
		}
		_, err = frdf.Write([]byte(s.String()))
		//		fmt.Println(s.String())
		if err != nil {
			panic(err)
		}
	}
	//
	// Genre
	//
	s.Reset()
	s.WriteString(" __type ")
	s.WriteString(fmt.Sprintf("%q", "Genre"))
	s.WriteString("  .\n")
	ty = s.String()
	fmt.Println("genre: count ", len(reader.Genre))
	for _, g := range reader.Genre { // genre {
		s.Reset()
		s.WriteString("_:")
		s.WriteString(g.Id)
		s.WriteString(ty)
		s.WriteString("_:")
		s.WriteString(g.Id)
		s.WriteString(" name ")
		s.WriteString(fmt.Sprintf(`%q`, g.Name))
		s.WriteString("  .")
		s.WriteByte('\n')

		_, err = frdf.Write([]byte(s.String()))
		if err != nil {
			panic(err)
		}
	}
	//
	// Performance
	//
	fmt.Println("output Performance ..................................")
	s.Reset()
	s.WriteString(" __type ")
	s.WriteString(fmt.Sprintf("%q", "Performance"))
	ty = s.String()
	fmt.Println(ty, len(reader.Performance))
	for _, p := range reader.Performance { //performance {
		s.Reset()
		s.WriteString("_:")
		s.WriteString(p.Id)
		id := s.String()
		s.WriteString(ty)
		s.WriteString("  .\n")
		s.WriteString(id)
		s.WriteString(" performance.film ")
		s.WriteString(fmt.Sprintf(`%s%s`, "_:", p.Film.Id))
		s.WriteString("  .\n")
		s.WriteString(id)
		s.WriteString(" performance.actor ")
		s.WriteString(fmt.Sprintf(`%s%s`, "_:", p.Actor.Id))
		s.WriteString("  .\n")
		s.WriteString(id)
		s.WriteString(" performance.character ")
		s.WriteString(fmt.Sprintf(`%s%s`, "_:", p.Character.Id))
		s.WriteString("  .\n")

		_, err = frdf.Write([]byte(s.String()))
		if err != nil {
			panic(err)
		}
	}
	//
	// Character
	//
	fmt.Println("output Character ..................................")
	s.Reset()
	s.WriteString(" __type ")
	s.WriteString(fmt.Sprintf("%q", "Character"))
	ty = s.String()
	fmt.Println(ty, len(reader.Character))
	for _, p := range reader.Character {
		s.Reset()
		s.WriteString("_:")
		s.WriteString(p.Id)
		id := s.String()
		s.WriteString(ty)
		s.WriteString("  .\n")
		s.WriteString(id)
		s.WriteString(" name ")
		s.WriteString(fmt.Sprintf(`%q  .`, p.Name))
		s.WriteString("\n")
		_, err = frdf.Write([]byte(s.String()))
		if err != nil {
			panic(err)
		}
	}
	//
	// Film
	//
	fmt.Println("output Performance ..................................")
	s.Reset()
	s.WriteString(" __type ")
	s.WriteString(fmt.Sprintf("%q", "Film"))
	ty = s.String()
	for _, p := range reader.Movie {
		s.Reset()
		s.WriteString("_:")
		s.WriteString(p.Id)
		id := s.String()
		s.WriteString(ty)
		s.WriteString("  .\n")
		s.WriteString(id)
		s.WriteString(" title ")
		s.WriteString(fmt.Sprintf(`%q  .`, p.Name[0]))
		s.WriteString("\n")
		s.WriteString(id)
		s.WriteString(" initial_release_date ")
		s.WriteString(fmt.Sprintf(`%q  .`, p.Ird))
		s.WriteString("\n")
		for _, p := range p.Genre {
			s.WriteString(id)
			s.WriteString(" film.genre ")
			s.WriteString(fmt.Sprintf(`%s%s  .`, "_:", p.Id))
			s.WriteString("\n")
		}
		for _, p := range p.Performance {
			s.WriteString(id)
			s.WriteString(" film.performance ")
			s.WriteString(fmt.Sprintf(`%s%s  .`, "_:", p.Id))
			s.WriteString("\n")
		}
		for _, p := range p.Director {
			s.WriteString(id)
			s.WriteString(" film.director ")
			s.WriteString(fmt.Sprintf(`%s%s  .`, "_:", p.Id))
			s.WriteString("\n")
		}
		_, err = frdf.Write([]byte(s.String()))
		if err != nil {
			panic(err)
		}
	}
	t2 := time.Now()
	err = frdf.Close()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("\n===== errors =====")
	reader.DumpErrs()
	fmt.Printf("\n===== timings for %d films =====\n", len(reader.Movie))
	fmt.Printf("Time to load maps: %s\n", t1.Sub(t0))
	fmt.Printf("Time to generate output file: %s\n", t2.Sub(t1))
}
