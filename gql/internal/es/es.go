package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	param "github.com/DynamoGraph/dygparam"
	"github.com/DynamoGraph/gql/internal/db"
	slog "github.com/DynamoGraph/syslog"
	"github.com/DynamoGraph/util"

	esv7 "github.com/elastic/go-elasticsearch/v7"
)

const (
	logid = "gqlES: "
	fatal = true
	idxNm = "myidx001"
)
const (
	esIndex    = "myidx001"
	allofterms = " AND "
	anyofterms = " OR "
)

func syslog(s string, fatal_ ...bool) {
	if len(fatal_) > 0 {
		slog.Log(logid, s, fatal_[0])
	} else {
		slog.Log(logid, s)
	}
}

var (
	cfg esv7.Config
	es  *esv7.Client
	err error
)

func init() {

	if !param.ElasticSearchOn {
		syslog("ElasticSearch Disabled....")
		return
	}
	cfg = esv7.Config{
		Addresses: []string{
			//	"http://ec2-54-234-180-49.compute-1.amazonaws.com:9200",
			"http://ip-172-31-18-75.ec2.internal:9200",
		},
		// ...
	}
	es, err = esv7.NewClient(cfg)
	if err != nil {
		syslog(fmt.Sprintf("Error creating the client: %s", err))
	}

	//
	// 1. Get cluster info
	//
	res, err := es.Info()
	if err != nil {
		syslog(fmt.Sprintf("Error getting response: %s", err))
	}
	defer res.Body.Close()
	// Check response status
	if res.IsError() {
		syslog(fmt.Sprintf("Error: %s", res.String()))
	}
	// Deserialize the response into a map.
	var r map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		syslog(fmt.Sprintf("Error parsing the response body: %s", err))
	}
	// Print client and server version numbers.
	syslog(fmt.Sprintf("Client: %s", esv7.Version))
	syslog(fmt.Sprintf("Server: %s", r["version"].(map[string]interface{})["number"]))
}

func Query(name string, qstring string) db.QResult {

	fmt.Printf("In Query: [%s]. [%s]\n", name, qstring)
	// a => predicate
	// value => space delimited list of terms

	type data struct {
		Field string
		Query string
	}

	var buf bytes.Buffer

	// ElasticSearch DSL Query
	esQuery := ` { "query": {
					 "bool": {
					   "must": [
	    				    {
	    				      "match": {
	    				        "attr": "{{.Field}}"            
					       }
					     },
					     {
					       "query_string": {
			    		         "query": "{{.Query}}" 
			    		      }
					     }
					    ]
				   }
				}
			}`
	//
	// process text template, esQuery
	//
	{
		input := data{Field: name, Query: qstring}
		tp := template.Must(template.New("query").Parse(esQuery))
		err := tp.Execute(&buf, input)
		if err != nil {
			syslog(fmt.Sprintf("Error in template execute: %s", err.Error()), fatal)
		}
	}

	// Perform the search request.
	t0 := time.Now()
	res, err := es.Search(
		es.Search.WithContext(context.Background()),
		es.Search.WithIndex(idxNm),
		es.Search.WithBody(&buf),
		es.Search.WithTrackTotalHits(true),
		es.Search.WithPretty(),
	)
	t1 := time.Now()
	if err != nil {
		syslog(fmt.Sprintf("Error getting response: %s", err), fatal)
	}
	defer res.Body.Close()

	syslog(fmt.Sprintf("ES Search duration: %s", t1.Sub(t0)))
	if res.IsError() {
		var e map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
			syslog(fmt.Sprintf("Error parsing the response body: %s", err), fatal)
		} else {
			// Print the response status and error information.
			syslog(fmt.Sprintf("[%s] %s: %s",
				res.Status(),
				e["error"].(map[string]interface{})["type"],
				e["error"].(map[string]interface{})["reason"],
			), fatal)
		}
	}
	var (
		r      map[string]interface{}
		result db.QResult
	)
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		syslog(fmt.Sprintf("Error parsing the response body: %s", err), fatal)
	}
	// package the ID and document source for each hit into db.QResult.
	for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {

		source := hit.(map[string]interface{})["_source"]

		pkey_ := hit.(map[string]interface{})["_id"].(string)
		pkey := pkey_[:strings.Index(pkey_, "|")]
		sortk := source.(map[string]interface{})["sortk"].(string)
		ty := source.(map[string]interface{})["type"].(string)

		dbres := db.NodeResult{PKey: util.FromString(pkey), SortK: sortk, Ty: ty}
		result = append(result, dbres)
	}

	return result

}
