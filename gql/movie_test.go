package gql

import (
	"testing"
	"time"
)

func TestMoviex(t *testing.T) {

	input := `{
  me(func: allofterms(title, "jones indiana")) {
    title
    film.genre {
      name
    }
  }
}`

	expectedTouchLvl = []int{5, 19}
	expectedTouchNodes = 24

	stmt := Execute("Movies", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMovieCrusade(t *testing.T) {

	input := `{
  me(func: allofterms(title, "jones indiana crusade")) {
    title
    film.genre {
      name
    }
  }
}`

	expectedTouchLvl = []int{1, 4}
	expectedTouchNodes = 5

	stmt := Execute("Movies", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMovieEq(t *testing.T) {

	input := `{
  me(func:eq(title, "Poison")) {
    title
    film.genre {
      name
    }
  }
}`

	expectedTouchLvl = []int{1, 7}
	expectedTouchNodes = 8

	stmt := Execute("Movies", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMovie1a(t *testing.T) {

	input := `{
  me(func: eq(name, "Steven Spielberg")) @filter(has(director.film)) {
    name
    director.film  {
      title
    }
  }
}`

	expectedTouchLvl = []int{1, 30}
	expectedTouchNodes = 31

	stmt := Execute("Movies", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}
func TestMovie1b(t *testing.T) {

	input := `{
  me(func: eq(name, "Steven Spielberg")) @filter(has(director.film)) {
    name
    director.film @filter(anyofterms(title,"War Minority") {
      title
    }
  }
}`

	expectedTouchLvl = []int{1, 3}
	expectedTouchNodes = 4

	stmt := Execute("Movies", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMovie1c(t *testing.T) {

	input := `{
  me(func: eq(count(film.genre), 13)) {
    title
    film.genre {
      name
    }
  }
}`

	expectedTouchLvl = []int{6, 78}
	expectedTouchNodes = 84

	stmt := Execute("Movies", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMovie1d(t *testing.T) {

	input := `{
  me(func: eq(count(film.genre), 13)) {
    title
    film.director {
    	name
    }
  }
}`

	expectedTouchLvl = []int{6, 6}
	expectedTouchNodes = 12

	stmt := Execute("Movies", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMovie1e(t *testing.T) {

	input := `{
  me(func: eq(count(film.genre), 13)) {
    title
    film.director {
    	name
    }
    film.genre {
    	name
    }
  }
}`

	expectedTouchLvl = []int{6, 84}
	expectedTouchNodes = 90

	stmt := Execute("Movies", input)
	t.Log(stmt.String())
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMovie1g(t *testing.T) {

	input := `{
  me(func: eq(count(film.genre), 13))  {
    title
    film.genre  @filter( anyofterms(name, "Musical comedy" )) {
    	name
    }
    film.director {
    	name
    }
  }
}`

	expectedTouchLvl = []int{6, 20}
	expectedTouchNodes = 26

	stmt := Execute("Movies", input)
	t.Log(stmt.String())
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestMoviePS0(t *testing.T) {

	input := `{
  me(func: eq(name,"Peter Sellers") ) {
    name
    actor.performance {
    	performance.film {
    		title
    	}
    	performance.character {
    		name
    	}
    	performance.actor {
    		name
    	}
  }
}
}`

	expectedTouchLvl = []int{1, 15, 45}
	expectedTouchNodes = 61

	stmt := Execute("Movies", input)
	t.Log(stmt.String())
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMoviePS2(t *testing.T) {

	input := `{
  me(func: eq(name,"Peter Sellers") ) {
    name
    actor.performance {
    	performance.film {
    		title
    		film.director {
    			name
    		}
    	}
  }
}
}`

	expectedTouchLvl = []int{1, 15, 15, 19}
	expectedTouchNodes = 50

	stmt := Execute("Movies", input)
	t.Log(stmt.String())
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMoviePS3a(t *testing.T) {

	input := `{
  me(func: eq(name,"Peter Sellers") ) {
    name
    actor.performance {
    	performance.film  {
    		title
    		film.director {
    			name
    		}
    		film.performance {
    				performance.actor {
    					name
    				}
    				performance.character {
    					name
    				}
    			}
    	}
  }
}
}`

	expectedTouchLvl = []int{1, 15, 15, 391, 744}
	expectedTouchNodes = 1166
	stmt := Execute("Movies", input)
	t0 := time.Now()
	result := stmt.MarshalJSON()
	t1 := time.Now()
	t.Log("Marshal elapsedTime; ", t1.Sub(t0))
	t.Log(stmt.String())

	validate(t, result)

}

func TestMoviePS3b(t *testing.T) {

	input := `{
  me(func: eq(name,"Peter Sellers") ) {
    name
    actor.performance {
    	performance.film  {
    		title
    		film.director @filter(eq(name,"Stanley Kubrick") ) {
    			name
    		}
    	}
  }
}
}`

	expectedTouchLvl = []int{1, 15, 15, 4}
	expectedTouchNodes = 35

	stmt := Execute("Movies", input)
	t.Log(stmt.String())
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMoviePS3c(t *testing.T) {

	input := `{
  me(func: eq(name,"Peter Sellers") ) {
    name
    actor.performance {
    	performance.film  {
    		title
    		film.director {
    			name
    		}
    	}
    	performance.character {
    		name
    	}
    	performance.actor {
    		name
    	}
  }
}
}`

	expectedTouchLvl = []int{1, 15, 45, 19}
	expectedTouchNodes = 80

	stmt := Execute("Movies", input)
	t.Log(stmt.String())
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMoviePS3d(t *testing.T) {

	input := `{
  me(func: eq(name,"Peter Sellers") ) {
    name
    actor.performance {
    	performance.character {
    		name
    	}
  }
}
}`

	expectedTouchLvl = []int{1, 15, 15}
	expectedTouchNodes = 31

	stmt := Execute("Movies", input)
	t.Log(stmt.String())
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestMoviePS3e(t *testing.T) {

	input := `{
  me(func: eq(name,"Peter Sellers") ) {
    name
    actor.performance {
    	performance.film  {
    		title
    	}
    	performance.character {
    		name
    	}
    	performance.actor {
    		name
    	}
  }
}
}`

	expectedTouchLvl = []int{1, 15, 45}
	expectedTouchNodes = 61

	stmt := Execute("Movies", input)
	t.Log(stmt.String())
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

// func TestMoviePS4(t *testing.T) {

// 	input := `{
//   me(func: eq(name,"Peter Sellers") ) {
//     name
//     actor.performance {
//     	performance.film  @filter( eq(film.director,variable(<stanley-kubrick-uid>) ) {
//     		title
//     		film.director  ) {
//     			name
//     		}
//     	}
//   }
// }
// }`

// 	expectedTouchLvl = []int{1, 15, 15, 4}
// 	expectedTouchNodes = 35

// 	stmt := Execute("Movies", input)
// 	t.Log(stmt.String())
// 	result := stmt.MarshalJSON()
// 	t.Log(stmt.String())

// 	validate(t, result)

// }

func TestMovieFilms(t *testing.T) {

	input := `{
  Mackenzie(func:allofterms(name, "crook mackenzie")) {
    name
    actor.performance {
      performance.film {
        title
      }
      performance.character {
        name
      }
    }
  }
}`

	expectedTouchLvl = []int{1, 8, 16}
	expectedTouchNodes = 25
	stmt := Execute("Movies", input)
	t0 := time.Now()
	result := stmt.MarshalJSON()
	t1 := time.Now()
	t.Log("Marshal elapsedTime; ", t1.Sub(t0))
	t.Log(stmt.String())

	validate(t, result)

}
