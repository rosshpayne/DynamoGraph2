package es

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	slog "github.com/DynamoGraph/syslog"

	esv7 "github.com/elastic/go-elasticsearch/v7"
	esapi "github.com/elastic/go-elasticsearch/v7/esapi"
	//	esv8 "github.com/elastic/go-elasticsearch/v8"
)

const (
	logid = "ElasticSearch: "
	esdoc = "dyngraph"
)

type Doc struct {
	Attr  string
	Value string
	PKey  string
	SortK string
	Type  string
}

func logerr(e error, panic_ ...bool) {

	if len(panic_) > 0 && panic_[0] {
		slog.Log(logid, e.Error(), true)
		panic(e)
	}
	slog.Log(logid, e.Error())
}

func syslog(s string) {
	slog.Log(logid, s)
}

var (
	cfg esv7.Config
)

func init() {

	cfg = esv7.Config{
		Addresses: []string{
			"http://ip-172-31-18-75.ec2.internal:9200",
		},
		// ...
	}
	es, err := esv7.NewClient(cfg)
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
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		syslog(fmt.Sprintf("Error parsing the response body: %s", err))
	}
	// Print client and server version numbers.
	syslog(fmt.Sprintf("Client: %s", esv7.Version))
	syslog(fmt.Sprintf("Server: %s", r["version"].(map[string]interface{})["number"]))
}

func Load(d *Doc) {

	syslog(fmt.Sprintf("in saveFTS. UID: [%s] %#v\n", d.Id, d))
	// Initialize a client with the default settings.
	//
	// An `ELASTICSEARCH_URL` environment variable will be used when exported.
	//
	es, err := esv7.NewClient(cfg)
	if err != nil {
		syslog(fmt.Sprintf("Error creating the client: %s", err))
	}
	//
	// 2. Index documents concurrently
	//
	// Build the request body.
	var b strings.Builder
	b.WriteString(`{"attr" : "`)
	b.WriteString(d.Attr)
	b.WriteString(`"value" : "`)
	b.WriteString(d.Value)
	b.WriteString(`",`)
	b.WriteString(`"sortk" : "`)
	b.WriteString(d.SortK)
	b.WriteString(`",`)
	b.WriteString(`"type" : "`)
	b.WriteString(d.Type)
	b.WriteString(`"}`)

	syslog(fmt.Sprintf("Body: %s", b.String()))
	// Set up the request object.
	req := esapi.IndexRequest{
		Index:      "myidx001",
		DocumentID: d.PKey,
		Body:       strings.NewReader(b.String()),
		Refresh:    "true",
	}

	// Perform the request with the client.
	res, err := req.Do(context.Background(), es)
	if err != nil {
		syslog(fmt.Sprintf("Error getting response: %s", err))
	}
	defer res.Body.Close()

	if res.IsError() {
		syslog(fmt.Sprintf("Error indexing document ID=%s. Status: %v ", d.Id, res.Status()))
	} else {
		// Deserialize the response into a map.
		var r map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
			syslog(fmt.Sprintf("Error parsing the response body: %s", err))
		} else {
			// Print the response status and indexed document version.
			syslog(fmt.Sprintf("[%s] %s; version=%d", res.Status(), r["result"], int(r["_version"].(float64))))
		}
	}
	//
	// 3. Search for the indexed documents
	//
	// Build the request body.
	// var buf bytes.Buffer
	// query := map[string]interface{}{
	// 	"query": map[string]interface{}{
	// 		"match": map[string]interface{}{
	// 			"text": "test",
	// 		},
	// 	},
	// }
	// if err := json.NewEncoder(&buf).Encode(query); err != nil {
	// 	syslog(fmt.Sprintf("Error encoding query: %s", err))
	// }

	// // Perform the search request.
	// res, err = es.Search(
	// 	es.Search.WithContext(context.Background()),
	// 	es.Search.WithIndex("graphstrings"),
	// 	es.Search.WithBody(&buf),
	// 	es.Search.WithTrackTotalHits(true),
	// 	es.Search.WithPretty(),
	// )
	// if err != nil {
	// 	syslog(fmt.Sprintf("Error getting response: %s", err))
	// }
	// defer res.Body.Close()

	// if res.IsError() {
	// 	var e map[string]interface{}
	// 	if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
	// 		syslog(fmt.Sprintf("Error parsing the response body: %s", err))
	// 	} else {
	// 		// Print the response status and error information.
	// 		lsyslog(fmt.Sprintf("[%s] %s: %s",
	// 			res.Status(),
	// 			e["error"].(map[string]interface{})["type"],
	// 			e["error"].(map[string]interface{})["reason"],
	// 		))
	// 	}
	// }

	// if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
	// 	syslog(fmt.Sprintf("Error parsing the response body: %s", err))
	// }
	// // Print the response status, number of results, and request duration.
	// syslog(fmt.Sprintf(
	// 	"[%s] %d hits; took: %dms",
	// 	res.Status(),
	// 	int(r["hits"].(map[string]interface{})["total"].(map[string]interface{})["value"].(float64)),
	// 	int(r["took"].(float64)),
	// ))
	// // Print the ID and document source for each hit.
	// for _, hit := range r["hits"].(map[string]interface{})["hits"].([]interface{}) {
	// 	log.Printf(" * ID=%s, %s", hit.(map[string]interface{})["_id"], hit.(map[string]interface{})["_source"])
	// }

	// log.Println(strings.Repeat("=", 37))
}
