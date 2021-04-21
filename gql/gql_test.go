package gql

import (
	"fmt"
	"strings"
	"testing"
)

func compareStat(result interface{}, expected interface{}) bool {
	//
	// return true when args are different
	//
	if result == nil && expected != nil {
		switch x := expected.(type) {
		case int:
			if x != 0 {
				return true
			}
		case []int:
			if x[0] != 0 {
				return true
			}
		}
		return false
	}

	switch x := result.(type) {

	case int:
		return expected.(int) != x

	case []int:
		if result == nil && len(x) == 1 && x[0] == 0 {
			return false
		}
		fmt.Println("in comparStat ", x)
		if exp, ok := expected.([]int); !ok {
			panic(fmt.Errorf("Expected should be []int"))
		} else {

			for i, v := range x {
				if i == len(exp) {
					return false
				}
				if v != exp[i] {
					return true
				}
			}
			// 			if len(x) > len(exp) {
			// 				for i := len(exp); i < len(x); i++ {
			// 					if x[i] != 0 {
			// 						return true
			// 					}
			// 				}
			// 			}
			return false
		}
	}
	return true
}

func compareJSON(doc, expected string) bool {

	return trimWS(doc) != trimWS(expected)

}

// trimWS trims whitespaces from input string. Objective is to compare strings real content - not influenced by whitespaces
func trimWS(input string) string {

	var out strings.Builder
	for _, v := range input {
		if !(v == '\u0009' || v == '\u0020' || v == '\u000A' || v == '\u000D' || v == ',') {
			out.WriteRune(v)
		}
	}
	return out.String()

}

// checkErrors compares actual errors from test against slice of expected errors
func checkErrors(errs []error, expectedErr []string, t *testing.T) {

	for _, ex := range expectedErr {
		if len(ex) == 0 {
			break
		}
		found := false
		for _, err := range errs {
			if trimWS(err.Error()) == trimWS(ex) {
				found = true
			}
		}
		if !found {
			t.Errorf(`Expected Error = [%q]`, ex)
		}
	}
	for _, got := range errs {
		found := false
		for _, exp := range expectedErr {
			if trimWS(got.Error()) == trimWS(exp) {
				found = true
			}
		}
		if !found {
			t.Errorf(`Unexpected Error = [%q]`, got.Error())
		}
	}
}

func TestSimpleRootQuery1a(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2)) {
    Age
    Name
  }
 }`

	expectedTouchLvl = []int{3}
	expectedTouchNodes = 3

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestSimpleRootQuery1b(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 1)) {
    Age
    Name
    Siblings {
    	Name
    }
  }
}`

	expectedTouchLvl = []int{1, 1}
	expectedTouchNodes = 2

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestSimpleRootQuery1c(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 1)) {
    Age
    Name
    Friends {
    	Name
    	Age
    	Siblings {
    		Name
    	}
    }
  }
}`

	expectedTouchLvl = []int{1, 3, 6}
	expectedTouchNodes = 10

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestSimpleRootQuery1d(t *testing.T) {

	// Friends {
	// 	Age
	// }
	input := `{
  directors(func: eq(count(Siblings), 1)) {
    Age
    Name
    Friends {
    	Name
    	Age
    	Siblings {
    		Name
    		Friends {
    			Name
    			Age
    		}
    	}
    }
  }
}`

	expectedTouchLvl = []int{1, 3, 6, 14}
	expectedTouchNodes = 24

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestRootQuery1e1(t *testing.T) {

	// Friends {
	// 	Age
	// }
	input := `{
  directors(func: anyofterms(Comment,"sodium Germany Chris")) {
    Age
    Name
    Comment
    Friends {
    	Name
    	Age
    	Siblings {
    		Name
    		Friends {
    			Name
    			Age
    		}
    	}
    }
  }
}`
	expectedTouchLvl = []int{2, 4, 7, 15}
	expectedTouchNodes = 28

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestRootQueryAnyPlusFilter2(t *testing.T) {

	// Friends {
	// 	Age
	// }
	input := `{
  directors(func: anyofterms(Comment,"sodium Germany Chris"))  @filter(gt(Age,60)) {
    Age
    Name
    Comment
    Friends {
    	Name
    	Age
    	Siblings {
    		Name
    		Friends {
    			Name
    			Age
    			Comment
    		}
    	}
    }
  }
}`
	expectedTouchLvl = []int{1, 2, 3, 6}
	expectedTouchNodes = 12

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
	//Shutdown()
}

func TestRootQueryAnyPlusFilter3(t *testing.T) {

	// Friends {
	// 	Age
	// }
	input := `{
  directors(func:  eq(count(Siblings), 2))  {
    Age
    Name
    Comment
    Friends {
    	Name
    	Age
    	Siblings {
    		Name
    		Friends {
    			Name
    			Age
    			Comment
    		}
    	}
    }
  }
}`
	expectedTouchLvl = []int{3, 7, 12, 27}
	expectedTouchNodes = 49

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestRootQuery1f(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2)) {
    Age
    Name
    Friends {
      Age
    	Name
    	Friends {
    	  Name
		  Age
		  Siblings {
		      Name
		      Age
		      Friends {
		          Name
		          Age
		          DOB
		      }
		  }
	    }
	    Siblings {
    		Name
	   	}
    }
  }
}`
	expectedTouchLvl = []int{3, 7, 30, 32, 73}
	expectedTouchNodes = 145

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
	//Shutdown()
}

func TestRootFilter1(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2)) @filter(gt(Age,60)) {
    Age
    Name
    Friends {
      Age
    	Name
    	Friends {
    	  Name
		    Age
	    }
	    Siblings {
    		Name
	   	}
    }
  }
}`

	expectedTouchLvl = []int{2, 5, 21}
	expectedTouchNodes = 28

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestUPredFilter1(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(gt(Age,61)) {
      Age
    	Name
    	Friends {
    	  Name
		    Age
	    }
	    Siblings {
    		Name
	   	}
    }
  }
}`
	expectedTouchLvl = []int{3, 4, 18}
	expectedTouchNodes = 25

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestUPredFilter2(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(gt(Age,60)) {
      Age
    	Name
    	Friends @filter(gt(Age,60)) {
    	  Name
		    Age
	    }
	    Siblings {
    		Name
	   	}
    }
  }
}`

	expectedTouchLvl = []int{3, 4, 12}
	expectedTouchNodes = 19

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestUPredFilter3a(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends {
      Age
    	Name
    	Siblings {
    		Name
    		Age
	   	}
    	Friends  {
    	  Name
	    }
  }
}
}`

	expectedTouchLvl = []int{3, 7, 30}
	expectedTouchNodes = 40

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestUPredFilter3b(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(gt(Age,50)) {
      Age
    	Name
    	Siblings @filter(gt(Age,5)) {
    		Name
    		Age
	   	}
    	Friends @filter(gt(Age,50)) {
    	  Age
    	  Name
	    }
  }
}
}`
	expectedTouchLvl = []int{3, 5, 18}
	expectedTouchNodes = 26

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestUPredFilter3c(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(gt(Age,60)) {
      Age
    	Name
    	Siblings @filter(gt(Age,60)) {
    		Name
    		Age
	   	}
    	Friends @filter(gt(Age,50)) {
    	  Age
    	  Name
	    }
  }
}
}`

	expectedTouchLvl = []int{3, 4, 10}
	expectedTouchNodes = 17

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestUPredFilter4aa(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ){
    Age
    Name
    Friends @filter(gt(Age,62) or le(Age,40) or eq(Name,"Ross Payne")) {
      Age
      Name
      Comment
      Friends   {
    	  Name
    	  Age
	   }
     Siblings {
    		Age
    		Name
    		Comment
	  }
  }
}
}`

	expectedTouchLvl = []int{3, 6, 26}
	expectedTouchNodes = 35

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestUPredFilter4ab(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter( (le(Age,40) or eq(Name,"Ian Payne")) and ge(Age,62)    ) {
      Age
    	Name
    	Comment
    	Friends   {
    	  Name
    	  Age
	    }
    	Siblings {
    		Age
    		Name
    		Comment
	   	}
  }
}
}`
	expectedJSON = `{
        data: [
                {
                Age : 62,
                Name : "Ross Payne",
                Friends : [ 
                        { 
                        Age: 67,
                        Name: "Ian Payne",
                        Comment: "One of the best cab rides I have Payne seen to date! Anyone know how fast the train was going around 20 mins in?",
                        Friends : [ 
                                { 
                                Name: "Phil Smith",
                                Age: 36,
                                },
                                { 
                                Name: "Ross Payne",
                                Age: 62,
                                },
                                { 
                                Name: "Paul Payne",
                                Age: 58,
                                },
                        ],
                        Siblings : [ 
                                { 
                                Age: 58,
                                Name: "Paul Payne",
                                Comment: "A foggy snowy morning lit with Smith sodium lamps is an absolute dream",
                                },
                                { 
                                Age: 62,
                                Name: "Ross Payne",
                                Comment: "Another fun  video. Loved it my Payne Grandmother was from Passau. Dad was over in Germany but there was something going on over there at the time we won't discuss right now. Thanks for posting it. Have a great weekend everyone.",
                                },
                        ]
                        }
                ]
                }, 
                {
                Age : 67,
                Name : "Ian Payne",
                Friends : [ 
                ]
                }, 
                {
                Age : 58,
                Name : "Paul Payne",
                Friends : [ 
                        { 
                        Age: 67,
                        Name: "Ian Payne",
                        Comment: "One of the best cab rides I have Payne seen to date! Anyone know how fast the train was going around 20 mins in?",
                        Friends : [ 
                                { 
                                Name: "Phil Smith",
                                Age: 36,
                                },
                                { 
                                Name: "Ross Payne",
                                Age: 62,
                                },
                                { 
                                Name: "Paul Payne",
                                Age: 58,
                                },
                        ],
                        Siblings : [ 
                                { 
                                Age: 58,
                                Name: "Paul Payne",
                                Comment: "A foggy snowy morning lit with Smith sodium lamps is an absolute dream",
                                },
                                { 
                                Age: 62,
                                Name: "Ross Payne",
                                Comment: "Another fun  video. Loved it my Payne Grandmother was from Passau. Dad was over in Germany but there was something going on over there at the time we won't discuss right now. Thanks for posting it. Have a great weekend everyone.",
                                },
                        ]
                        }
                ]
                }
        ]
        }`

	expectedTouchLvl = []int{3, 2, 10}
	expectedTouchNodes = 15

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
	//Shutdown()
}

func TestUPredFilter4ac(t *testing.T) {

	//  (le(Age,40) or eq(Name,"Ian Payne")) and ge(Age,62) )        x
	//  le(Age,40) or eq(Name,"Ian Payne") and ge(Age,62) )          -
	//   le(Age,40) or eq(Name,"Ian Payne") and le (Age,62)          -
	//   le(Age,40) or eq(Name,"Ian Payne")                          -
	//   (le(Age,40) or eq(Name,"Ian Payne") )                       -
	//.   (le(Age,40) and eq(Name,"Ian Payne") )                     -
	//   ge(Age,62) and ( le(Age,40) or eq(Name,"Ian Payne") )       -
	//
	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter( (le(Age,40) or eq(Name,"Ian Payne")) and ge(Age,62)    ) {
      Age
    	Name
    	Comment
    	Friends   {
    	  Name
    	  Age
    	  Friends {
    	  	Name
    	  	Age
    	  }
	    }
    	Siblings {
    		Age
    		Name
    		Comment
	   	}
  }
}
}`
	expectedJSON = `{
        data: [
                {
                Age : 62,
                Name : "Ross Payne",
                Friends : [
                        {
                        Age: 67,
                        Name: "Ian Payne",
                        Comment: "One of the best cab rides I have Payne seen to date! Anyone know how fast the train was going around 20 mins in?",
                        Friends : [
                                {
                                Name: "Phil Smith",
                                Age: 36,
                                Friends : [
                                        {
                                        Name: "Paul Payne",
                                        Age: 58,
                                        },
                                        {
                                        Name: "Ross Payne",
                                        Age: 62,
                                        },
                                        {
                                        Name: "Ian Payne",
                                        Age: 67,
                                        },
                                ]
                                },
                                {
                                Name: "Ross Payne",
                                Age: 62,
                                Friends : [
                                        {
                                        Name: "Phil Smith",
                                        Age: 36,
                                        },
                                        {
                                        Name: "Ian Payne",
                                        Age: 67,
                                        },
                                ]
                                },
                                {
                                Name: "Paul Payne",
                                Age: 58,
                                Friends : [
                                        {
                                        Name: "Ross Payne",
                                        Age: 62,
                                        },
                                        {
                                        Name: "Ian Payne",
                                        Age: 67,
                                        },
                                ]
                                },
                        ],
                        Siblings : [
                                {
                                Age: 58,
                                Name: "Paul Payne",
                                Comment: "A foggy snowy morning lit with Smith sodium lamps is an absolute dream",
                                },
                                {
                                Age: 62,
                                Name: "Ross Payne",
                                Comment: "Another fun  video. Loved it my Payne Grandmother was from Passau. Dad was over in Germany but there was something going on over there at the time we won't discuss right now. Thanks for posting it. Have a great weekend everyone.",
                                },
                        ]
                        }
                ]
                },
                {
                Age : 67,
                Name : "Ian Payne",
                Friends : [
                ]
                },
                {
                Age : 58,
                Name : "Paul Payne",
                Friends : [
                        {
                        Age: 67,
                        Name: "Ian Payne",
                        Comment: "One of the best cab rides I have Payne seen to date! Anyone know how fast the train was going around 20 mins in?",
                        Friends : [
                                {
                                Name: "Phil Smith",
                                Age: 36,
                                Friends : [
                                        {
                                        Name: "Paul Payne",
                                        Age: 58,
                                        },
                                        {
                                        Name: "Ross Payne",
                                        Age: 62,
                                        },
                                        {
                                        Name: "Ian Payne",
                                        Age: 67,
                                        },
                                ]
                                },
                                {
                                Name: "Ross Payne",
                                Age: 62,
                                Friends : [
                                        {
                                        Name: "Phil Smith",
                                        Age: 36,
                                        },
                                        {
                                        Name: "Ian Payne",
                                        Age: 67,
                                        },
                                ]
                                },
                                {
                                Name: "Paul Payne",
                                Age: 58,
                                Friends : [
                                        {
                                        Name: "Ross Payne",
                                        Age: 62,
                                        },
                                        {
                                        Name: "Ian Payne",
                                        Age: 67,
                                        },
                                ]
                                },
                        ],
                        Siblings : [
                                {
                                Age: 58,
                                Name: "Paul Payne",
                                Comment: "A foggy snowy morning lit with Smith sodium lamps is an absolute dream",
                                },
                                {
                                Age: 62,
                                Name: "Ross Payne",
                                Comment: "Another fun  video. Loved it my Payne Grandmother was from Passau. Dad was over in Germany but there was something going on over there at the time we won't discuss right now. Thanks for posting it. Have a great weekend everyone.",
                                },
                        ]
                        }
                ]
                }
        ]
        }`

	expectedTouchLvl = []int{3, 2, 10, 14}
	expectedTouchNodes = 29

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
	//Shutdown()
}

func TestUPredFilter4b(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(gt(Age,60)) {
      Age
    	Name
    	Siblings @filter(gt(Age,60)) {
    		Name
	   	}
    	Friends @filter(gt(Age,60)) {
    	  Name
	    }

    }
  }
}`

	expectedTouchLvl = []int{3, 4, 8}
	expectedTouchNodes = 15

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestUPredFilter5(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(gt(Age,60)) {
      Age
    	Name
    	Friends @filter(gt(Age,62)) {
    	  Name
    	  Comment
	    }
	    Siblings @filter(gt(Age,60)) {
    		Name
    		DOB
	   	}
    }
  }
}`

	expectedTouchLvl = []int{3, 4, 6}
	expectedTouchNodes = 13

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestRootHas1(t *testing.T) {

	input := `{
	  me(func: has(Address)) {
	    Name
		Address
		Age
		Siblings {
			Name
			Age
		}
	    }
	}`
	expectedTouchLvl = []int{1, 2}
	expectedTouchNodes = 3

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestRootHas2(t *testing.T) {

	input := `{
	  me(func: has(Siblings)) {
	    Name
		Address
		Age
		Siblings {
			Name
			Age
		}
	    }
	}`

	expectedTouchLvl = []int{4, 7}
	expectedTouchNodes = 11

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestRootHasWithFilter(t *testing.T) {

	input := `{
	  me(func: has(Siblings)) @filter(has(Address)) {
	    Name
		  Address
		  Age
		  Siblings {
		  	Name
			  Age
		  }
	  }
	}`

	expectedTouchLvl = []int{1, 2}
	expectedTouchNodes = 3

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestRootFilterHas1(t *testing.T) {

	input := `{
	  me(func: eq(count(Siblings),2)) @filter(has(Address)) {
	    Name
		Address
		Age
		Siblings {
			Name
			Age
		}
	    }
	}`

	expectedTouchLvl = []int{1, 2}
	expectedTouchNodes = 3

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestRootFilterHas2(t *testing.T) {

	input := `{
	  me(func: eq(count(Siblings),2)) @filter(has(Friends)) {
	    Name
		Address
		Age
		Siblings {
			Name
			Age
		}
	    }
	}`

	expectedTouchLvl = []int{3, 6}
	expectedTouchNodes = 9

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestUidPredFilterHasScalar(t *testing.T) {

	input := `{
	  me(func: eq(count(Siblings),2)) @filter(has(Friends)) {
	    Name
		Address
		Age
		Siblings @filter(has(Address)) {
			Name
			Age
		}
	    }
	}`

	expectedJSON = `{
        data: [
                {
                Name : "Ross Payne",
                Address : "67/55 Burkitt St Page, ACT, Australia",
                Age : 62,
                Siblings : [ 
                ]
                }, 
                {
                Name : "Ian Payne",
                 Address : <nil>,
                Age : 67,
                Siblings : [ 
                        { 
                        Name: "Ross Payne",
                        Age: 62,
                        },
                ]
                }, 
                {
                Name : "Paul Payne",
                 Address : <nil>,
                Age : 58,
                Siblings : [ 
                        { 
                        Name: "Ross Payne",
                        Age: 62,
                        },
                ]
                }
        ]
        }`

	expectedTouchLvl = []int{3, 2}
	expectedTouchNodes = 5

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

// func TestUidPredFilterHasUidPred(t *testing.T) {

// 	input := `{
// 	  me(func: eq(count(Siblings),2)) @filter(has(Friends)) {
// 	    Name
// 		Address
// 		Age
// 		Siblings @filter(has(Friends)) {
// 			Name
// 			Age
// 		}
// 	    }
// 	}`

// 	expectedJSON = `{
//         data: [
//                 {
//                 Name : "Ross Payne",
//                 Address : "67/55 Burkitt St Page, ACT, Australia",
//                 Age : 62,
//                 Siblings : [
//                 ]
//                 },
//                 {
//                 Name : "Ian Payne",
//                  Address : <nil>,
//                 Age : 67,
//                 Siblings : [
//                         {
//                         Name: "Ross Payne",
//                         Age: 62,
//                         },
//                 ]
//                 },
//                 {
//                 Name : "Paul Payne",
//                  Address : <nil>,
//                 Age : 58,
//                 Siblings : [
//                         {
//                         Name: "Ross Payne",
//                         Age: 62,
//                         },
//                 ]
//                 }
//         ]
//         }`

// 	expectedTouchLvl = []int{3, 2}
// 	expectedTouchNodes = 5

// 	stmt := Execute("Relationship", input)
// 	result := stmt.MarshalJSON()
// 	t.Log(stmt.String())

// 	validate(t, result)
// }

func TestRootFilteranyofterms1(t *testing.T) {

	input := `{
	  me(func: eq(count(Siblings),2)) @filter(anyofterms(Comment,"sodium Germany Chris")) {
	    Name
		Comment
	    }
	  }`

	expectedJSON = `{
        data: [
                {
                Name : "Ross Payne",
                Comment : "Another fun  video. Loved it my Payne Grandmother was from Passau. Dad was over in Germany but there was something going on over there at the time we won't discuss right now. Thanks for posting it. Have a great weekend everyone.",
                }, 
                {
                Name : "Paul Payne",
                Comment : "A foggy snowy morning lit with Smith sodium lamps is an absolute dream",
                }
        ]
        }`

	expectedTouchLvl = []int{2}
	expectedTouchNodes = 2

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestRootFilterallofterms1a(t *testing.T) {

	input := `{
	  me(func: eq(count(Siblings),2)) @filter(allofterms(Comment,"sodium Germany Chris")) {
	    Name
	    }
	  }`

	// Expected values should be populated even when no result is expected - mostly for documentation purposes
	expectedTouchLvl = []int{0}
	expectedTouchNodes = 0

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestRootFilteranyofterms1b(t *testing.T) {

	input := `{
	  me(func: eq(count(Siblings),2)) @filter(anyofterms(Comment,"sodium Germany Chris") and eq(Name,"Ian Payne")) {
	    Name
	    }
	  }`

	expectedTouchLvl = []int{0}
	expectedTouchNodes = 0

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestRootFilterallofterms1c(t *testing.T) {

	input := `{
	  me(func: eq(count(Siblings),2)) @filter(allofterms(Comment,"sodium Germany Chris") or eq(Name,"Ian Payne")) {
	    Name
	    }
	  }`

	expectedJSON = `{
        data: [
                {
                Name : "Ian Payne",
                }
        ]
        }`

	expectedTouchLvl = []int{1}
	expectedTouchNodes = 1

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestRootFilteranyofterms1d(t *testing.T) {

	input := `{
	  me(func: eq(count(Siblings),2)) @filter(anyofterms(Comment,"sodium Germany Chris") or eq(Name,"Ian Payne")) {
	    Name
	    }
	  }`

	expectedTouchLvl = []int{3}
	expectedTouchNodes = 3

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestRootFilteranyofterms1e(t *testing.T) {

	input := `{
	  me(func: eq(count(Siblings),2)) @filter(anyofterms(Comment,"sodium Germany Chris") and eq(Name,"Ross Payne")) {
	    Name
	    }
	  }`

	expectedJSON = `       {
        data: [
                {
                Name : "Ross Payne",
                }
        ]
        }`

	expectedTouchLvl = []int{1}
	expectedTouchNodes = 1

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestUPredFilterterms1a(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends  {
      Age
    	Name
    	Comment
    	Friends{
    	  Name
	    }
	    Siblings {
    		Name
	   	}
    }
  }
}`
	expectedTouchLvl = []int{3, 7, 30}
	expectedTouchNodes = 40

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestUPredFilterterms1b1(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(anyofterms(Comment,"sodium Germany Chris")) {
        Age
    	Name
    	Comment
    	Friends @filter(gt(Age,62)) {
    	  Age
    	  Name
	    }
	    Siblings @filter(gt(Age,55)) {
    	  Name
	   	}
    }
  }
}`

	expectedTouchLvl = []int{3, 3, 9}
	expectedTouchNodes = 15

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}

func TestUPredFilterterms1b2(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(anyofterms(Comment,"sodium Germany Chris")) {
        Age
    	Name
    	Friends @filter(gt(Age,62)) {
    	  Age
    	  Name
	    }
	    Siblings @filter(gt(Age,58)) {
    		Name
    		Age
	   	}
    }
  }
}`

	expectedTouchLvl = []int{3, 3, 7}
	expectedTouchNodes = 13

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}

func TestUPredFilterterms1b3(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(anyofterms(Comment,"sodium Germany Chris")) {
        Age
    	Name
    	Comment
    	Friends @filter(eq(Age,62)) {
    	  Age
    	  Name
	    }
	    Siblings @filter(gt(Age,55)) {
    	  Name
	   	}
    }
  }
}`

	expectedTouchLvl = []int{3, 3, 7}
	expectedTouchNodes = 13

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)
}
func TestUPredFiltertermsStat(t *testing.T) {

	input := `{
  directors(func: eq(count(Siblings), 2) ) {
    Age
    Name
    Friends @filter(anyofterms(Comment,"sodium Germany Chris")) {
        Age
    	Name
    	Comment
    	Friends @filter(gt(Age,62)) {
    	  Age
    	  Name
	    }
	    Siblings @filter(gt(Age,55)) {
    	  Name
	   	}
    }
  }
}`

	expectedTouchLvl = []int{3, 3, 9}
	expectedTouchNodes = 15

	stmt := Execute("Relationship", input)
	result := stmt.MarshalJSON()
	t.Log(stmt.String())

	validate(t, result)

}
