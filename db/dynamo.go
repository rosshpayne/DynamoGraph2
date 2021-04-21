package db

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	blk "github.com/DynamoGraph/block"
	"github.com/DynamoGraph/dbConn"
	//gerr "github.com/DynamoGraph/dygerror"
	param "github.com/DynamoGraph/dygparam"
	mon "github.com/DynamoGraph/gql/monitor"
	slog "github.com/DynamoGraph/syslog"
	"github.com/DynamoGraph/util"

	//"github.com/aws/aws-sdk-go/aws/awserr"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
)

const (
	DELETE = 'D'
	ADD    = 'A'
	// graph table name
)

type gsiResult struct {
	PKey  []byte
	SortK string
}

//  ItemCache struct is the transition between Dynamodb types and the actual attribute type defined in the DD.
//  Number (dynamodb type) -> float64 (transition) -> int (internal app & as defined in DD)
//  process: dynamodb -> ItemCache -> DD conversion if necessary to application variables -> ItemCache -> Dynamodb
//	types that require conversion from ItemCache to internal are:
//   DD:   int         conversion: float64 -> int
//   DD:   datetime    conversion: string -> time.Time
//  all the other datatypes do not need to be converted.

var (
	dynSrv *dynamodb.DynamoDB
)

func logerr(e error, panic_ ...bool) {

	if len(panic_) > 0 && panic_[0] {
		slog.Log("DB: ", e.Error(), true)
		panic(e)
	}
	slog.Log("DB: ", e.Error())
}

func syslog(s string) {
	slog.Log("DB: ", s)
}

//  NOTE: tyShortNm is duplicated in cache pkg. It exists in in db package only to support come code in rdfload.go that references the db version rather than the cache which it cannot access
// because of input-cycle issues. Once this reference is rdfload is removed the cache version should be the only one used.

type tyNames struct {
	ShortNm string `json:"Atr"`
	LongNm  string
}

var (
	err       error
	tynames   []tyNames
	tyShortNm map[string]string
)

func init() {
	dynSrv = dbConn.New()
}

func GetTypeShortNames() ([]tyNames, error) {
	return tynames, nil
}

// NodeExists
func NodeExists(uid util.UID, subKey ...string) (bool, error) {

	var sortk string

	if len(subKey) > 0 {
		sortk = subKey[0]
	} else {
		sortk = "A#T"
	}
	pkey := pKey{PKey: uid, SortK: sortk}

	av, err := dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		return false, newDBMarshalingErr("NodeExists", uid.String(), sortk, "MarshalMap", err)
	}
	//
	input := &dynamodb.GetItemInput{
		Key: av,
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	// GetItem
	//
	t0 := time.Now()
	result, err := dynSrv.GetItem(input)
	t1 := time.Now()
	if err != nil {
		return false, newDBSysErr("NodeExists", "GetItem", err)
	}
	syslog(fmt.Sprintf("NodeExists: consumed capacity for GetItem: %s  Duration: %s", result.ConsumedCapacity, t1.Sub(t0)))

	if len(result.Item) == 0 {
		return false, nil
	}
	return true, nil
}

// FetchNode performs a Query with KeyBeginsWidth on the SortK value, so all item belonging to the SortK are fetched.
func FetchNode(uid util.UID, subKey ...string) (blk.NodeBlock, error) {

	var sortk string
	if len(subKey) > 0 {
		sortk = subKey[0]
		slog.Log("DB FetchNode: ", fmt.Sprintf(" node: %s subKey: %s", uid.String(), sortk))
	} else {
		sortk = "A#"
		slog.Log("DB FetchNode: ", fmt.Sprintf(" node: %s subKey: %s", uid.String(), sortk))
	}

	keyC := expression.KeyEqual(expression.Key("PKey"), expression.Value(uid)).And(expression.KeyBeginsWith(expression.Key("SortK"), sortk))
	expr, err := expression.NewBuilder().WithKeyCondition(keyC).Build()
	if err != nil {
		return nil, newDBExprErr("FetchTNode", uid.String(), sortk, err)
	}
	//
	input := &dynamodb.QueryInput{
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL").SetConsistentRead(true)
	//
	// Query
	//
	t0 := time.Now()
	result, err := dynSrv.Query(input)
	t1 := time.Now()
	if err != nil {
		return nil, newDBSysErr("DB FetchNode", "Query", err)
	}
	dur := t1.Sub(t0)
	syslog(fmt.Sprintf("FetchNode:consumed capacity for Query  %s. ItemCount %d  Duration: %s", result.ConsumedCapacity.String(), len(result.Items), dur.String()))
	//
	if int(*result.Count) == 0 {
		// is subKey a G type (uid-predicate) ie. child data block associated with current parent node, create empty dataItem.
		if len(subKey) > 0 && strings.Index(subKey[0], "#G#") != -1 {
			data := make(blk.NodeBlock, 1)
			data[0] = new(blk.DataItem)
			return data, nil
		}
		return nil, newDBNoItemFound("FetchNode", uid.String(), "", "Query")
	}
	data := make(blk.NodeBlock, *result.Count)
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &data)
	if err != nil {
		return nil, newDBUnmarshalErr("FetchNode", uid.String(), "", "UnmarshalListOfMaps", err)
	}
	//
	// send stats
	//
	v := mon.Fetch{CapacityUnits: *result.ConsumedCapacity.CapacityUnits, Items: len(result.Items), Duration: dur}
	stat := mon.Stat{Id: mon.DBFetch, Value: &v}
	mon.StatCh <- stat

	return data, nil
}

func FetchNodeItem(uid util.UID, sortk string) (blk.NodeBlock, error) {

	// stat := mon.Stat{Id: mon.DBFetch}
	// mon.StatCh <- stat

	// proj := expression.NamesList(expression.Name("SortK"), expression.Name("Nd"), expression.Name("XF"), expression.Name("Id"))
	// expr, err := expression.NewBuilder().WithProjection(proj).Build()
	// if err != nil {
	// 	return nil, newDBExprErr("FetchNodeItem", "", "", err)
	// }
	// TODO: remove encoding when load of data via cli is not used.

	pkey := pKey{PKey: []byte(uid), SortK: sortk}
	av, err := dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		return nil, newDBMarshalingErr("FetchItem", "", sortk, "MarshalMap", err)
	}
	//
	input := &dynamodb.GetItemInput{
		Key: av,
		// ProjectionExpression:     expr.Projection(),
		// ExpressionAttributeNames: expr.Names(),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	// GetItem
	//
	t0 := time.Now()
	result, err := dynSrv.GetItem(input)
	t1 := time.Now()
	dur := t1.Sub(t0)
	if err != nil {
		return nil, newDBSysErr("FetchNodeItem", "GetItem", err)
	}
	syslog(fmt.Sprintf("FetchNodeItem:consumed capacity for GetItem  %s. Duration: %s", result.ConsumedCapacity.String(), t1.Sub(t0)))
	//
	if len(result.Item) == 0 {
		return nil, newDBNoItemFound("FetchNodeItem", "", sortk, "GetItem")
	}
	//
	var di blk.DataItem
	err = dynamodbattribute.UnmarshalMap(result.Item, &di)
	if err != nil {
		return nil, newDBUnmarshalErr("FetchNodeItem", "", sortk, "UnmarshalMap", err)
	}
	nb := make(blk.NodeBlock, 1, 1)
	nb[0] = &di

	v := mon.Fetch{CapacityUnits: *result.ConsumedCapacity.CapacityUnits, Items: len(result.Item), Duration: dur}
	stat := mon.Stat{Id: mon.DBFetch, Value: &v}
	mon.StatCh <- stat

	return nb, nil
	//
}

// // FetchNodeEncode() first base64 encodes UID before querying db.
// // this is required as a workaround to CLI loaded binary data that is effectively double encoded. Firstly it is encoded in the text file then it is encode by the CLI during parsing (which is shouldn't do)
// func FetchNodeEncode(uid util.UID, subKey ...string) (blk.NodeBlock, error) {
// 	var keyC expression.KeyConditionBuilder

// 	uidb64 := util.UID(uid.Encodeb64()) //uid
// 	// Dynamodb sdk will not convert UID to base64 so we must. This is unlike PutItem that will encode all binaries to base64.
// 	fmt.Printf("*********** db FetchNodeEncode: [%08b]%d uid: %s subKey: %q\n", uid, len(uid), uid, subKey[0])
// 	if len(subKey) > 0 {
// 		keyC = expression.KeyEqual(expression.Key("PKey"), expression.Value(uidb64)).And(expression.KeyBeginsWith(expression.Key("SortK"), subKey[0]))
// 	} else {
// 		keyC = expression.KeyEqual(expression.Key("PKey"), expression.Value(uidb64)).And(expression.KeyBeginsWith(expression.Key("SortK"), "A#"))
// 	}
// 	expr, err := expression.NewBuilder().WithKeyCondition(keyC).Build()
// 	if err != nil {
// 		return nil, newDBExprErr("FetchNodeEncode", uidb64.String(), "", err)
// 	}
// 	//
// 	input := &dynamodb.QueryInput{
// 		KeyConditionExpression:    expr.KeyCondition(),
// 		FilterExpression:          expr.Filter(),
// 		ExpressionAttributeNames:  expr.Names(),
// 		ExpressionAttributeValues: expr.Values(),
// 	}
// 	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL").SetConsistentRead(false)
// 	//
// 	// Query - returns ALL the node data + any propagated child data
// 	//
// 	t0 := time.Now()
// 	result, err := dynSrv.Query(input)
// 	t1 := time.Now()
// 	//fmt.Println("DB Access: ", t1.Sub(t0))
// 	if err != nil {
// 		return nil, newDBSysErr("FetchNode", "Query", err)
// 	}
// 	syslog(fmt.Sprintf("FetchNodeEncode:consumed capacity for Query  %s. ItemCount %d  Duration: %s", result.ConsumedCapacity.String(), len(result.Items), t1.Sub(t0)))
// 	//
// 	if int(*result.Count) == 0 {
// 		return nil, newDBNoItemFound("FetchNodeEncode", uidb64.String(), "", "Query")
// 	}
// 	if logerr == nil {
// 		fmt.Println("logerr is nil")
// 	}
// 	data := make(blk.NodeBlock, *result.Count)
// 	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &data)
// 	if err != nil {
// 		return nil, newDBUnmarshalErr("FetchNodeEncode", uidb64.String(), "", "UnmarshalListOfMaps", err)
// 	}

// 	return data, nil
// }

type pKey struct {
	PKey  []byte
	SortK string
}

// SaveCompleteUpred saves all Nd & Xf & Id values. See SaveUpredAvailability which saves an individual UID state.
func SaveCompleteUpred(di *blk.DataItem) error {
	//
	var (
		err    error
		expr   expression.Expression
		upd    expression.UpdateBuilder
		values map[string]*dynamodb.AttributeValue
	)

	convertSet2List := func() {
		// fix to possible sdk error/issue for Binary ListAppend operations. SDK builds
		//  a BS rather than a LIST for LISTAPPEND operation invovling binary data.
		// This is the default for binary for some reason - very odd.
		// We therefore need to convert from BS created by the SDK to LB (List Binary)
		var s strings.Builder
		for k, v := range expr.Names() {
			switch *v {
			case "Nd":
				s.WriteByte(':')
				s.WriteByte(k[1])
				// check if BS is used and then convert if it is
				var nl []*dynamodb.AttributeValue
				for i, u := range values[s.String()].BS {
					if i == 0 {
						nl = make([]*dynamodb.AttributeValue, len(values[s.String()].BS), len(values[s.String()].BS))
					}
					nl[i] = &dynamodb.AttributeValue{B: u}
					if i == len(values[s.String()].BS)-1 {
						values[s.String()] = &dynamodb.AttributeValue{L: nl} // this nils AttributeValue{B }
					}
				}
				s.Reset()
			}
		}
	}
	// update all elements in XF, Id, Nd
	upd = expression.Set(expression.Name("XF"), expression.Value(di.XF))
	upd = upd.Set(expression.Name("Id"), expression.Value(di.Id))
	upd = upd.Set(expression.Name("Nd"), expression.Value(di.Nd))
	expr, err = expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		return newDBExprErr("SaveCompleteUpred", "", "", err)
	}
	values = expr.Values()
	// convert expression values result from binary Set to binary List
	convertSet2List()
	//
	// Marshal primary key of parent node
	//
	pKey := pKey{PKey: di.PKey, SortK: di.SortK}
	syslog(fmt.Sprintf("SaveCompleteUpred: pKey: %#v   sortk %s", pKey, di.SortK))
	av, err := dynamodbattribute.MarshalMap(&pKey)
	if err != nil {
		return newDBMarshalingErr("SaveCompleteUpred", util.UID(di.GetPkey()).String(), "", "MarshalMap", err)
	}
	//
	update := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: values,
		UpdateExpression:          expr.Update(),
	}
	update = update.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(update)
		t1 := time.Now()
		if err != nil {
			return newDBSysErr("SaveCompleteUpred", "UpdateItem", err)
		}
		syslog(fmt.Sprintf("SaveCompleteUpred:consumed capacity for Query  %s.  Duration: %s", uio.ConsumedCapacity, t1.Sub(t0)))

	}
	return nil
}

// SaveUpredAvailability writes availability state of the uid-pred to storage
func SaveUpredState(di *blk.DataItem, uid util.UID, status int, idx int, cnt int, attrNm string, ty string) error {
	//
	var (
		err    error
		expr   expression.Expression
		upd    expression.UpdateBuilder
		values map[string]*dynamodb.AttributeValue
	)

	convertSet2List := func() {
		// fix to possible sdk error/issue for Binary ListAppend operations. SDK builds
		//  a BS rather than a LIST for LISTAPPEND operation invovling binary data.
		// This is the default for binary for some reason - very odd.
		// We therefore need to convert from BS created by the SDK to LB (List Binary)
		var s strings.Builder
		for k, v := range expr.Names() {
			switch *v {
			case "Nd":
				s.WriteByte(':')
				s.WriteByte(k[1])
				// check if BS is used and then convert if it is
				var nl []*dynamodb.AttributeValue
				for i, u := range values[s.String()].BS {
					if i == 0 {
						nl = make([]*dynamodb.AttributeValue, len(values[s.String()].BS), len(values[s.String()].BS))
					}
					nl[i] = &dynamodb.AttributeValue{B: u}
					if i == len(values[s.String()].BS)-1 {
						values[s.String()] = &dynamodb.AttributeValue{L: nl} // this nils AttributeValue{B }
					}
				}
				s.Reset()
			}
		}
	}
	// modify the target element in the XF List type.
	//syslog(fmt.Sprintf("SaveUpredAvailable: %d %d %d ", status, idx, cnt))
	entry := "XF[" + strconv.Itoa(idx) + "]"
	upd = expression.Set(expression.Name(entry), expression.Value(status))
	upd = upd.Set(expression.Name("P"), expression.Value(attrNm))
	upd = upd.Set(expression.Name("Ty"), expression.Value(ty))
	upd = upd.Add(expression.Name("N"), expression.Value(cnt))
	expr, err = expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		return newDBExprErr("SaveUpredAvailable", "", "", err)
	}
	values = expr.Values()
	// convert expression values result from binary Set to binary List
	convertSet2List()
	//
	// Marshal primary key of parent node
	//
	pKey := pKey{PKey: di.PKey, SortK: di.SortK}
	av, err := dynamodbattribute.MarshalMap(&pKey)
	if err != nil {
		return newDBMarshalingErr("SaveUpredAvailable", util.UID(di.GetPkey()).String(), "", "MarshalMap", err)
	}
	//
	update := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: values,
		UpdateExpression:          expr.Update(),
	}
	update = update.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(update)
		t1 := time.Now()
		if err != nil {
			return newDBSysErr("SaveUpredAvailable", "UpdateItem", err)
		}
		syslog(fmt.Sprintf("SaveUpredAvailable:consumed capacity for Query  %s.  Duration: %s", uio.ConsumedCapacity, t1.Sub(t0)))

	}
	return nil
}

// SaveOvflBlkFull - overflow block has become full due to child data propagation.
// Mark it as full so it will not be chosen in future to load child data.
// this was called from the cache service.
func SaveOvflBlkFull(di *blk.DataItem, idx int) error {

	pkey := pKey{PKey: di.PKey, SortK: di.SortK}

	av, err := dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		return newDBMarshalingErr("SaveOvflBlkFull", util.UID(di.PKey).String(), "", "MarshalMap", err)
	}
	//
	// use cIdx to update XF entry
	//
	idxs := "XF[" + strconv.Itoa(idx) + "]"
	upd := expression.Set(expression.Name(idxs), expression.Value(blk.OvflItemFull))
	expr, err := expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		return newDBExprErr("SaveOvflBlkFull", "", "", err)
	}
	//
	updii := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
	}
	updii = updii.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(updii)
		t1 := time.Now()
		if err != nil {
			return newDBSysErr("SaveOvflBlkFull", "UpdateItem", err)
		}
		syslog(fmt.Sprintf("SaveOvflBlkFull: consumed capacity for UpdateItem : %s  Duration: %s\n", uio.ConsumedCapacity, t1.Sub(t0)))
	}

	return nil
}

// SetCUIDpgFlag is used as part of the recovery when child data propagation when attaching a node exceeds the db item size. This will only happen in the overflow blocks
// which share the item with thousands of child UID.
//
func SetCUIDpgFlag(tUID, cUID util.UID, sortk string) error {

	proj := expression.NamesList(expression.Name("Nd"))
	expr, err := expression.NewBuilder().WithProjection(proj).Build()
	if err != nil {
		return newDBExprErr("SetCUIDpgFlag", "", "", err)
	}
	// TODO: remove encoding when load of data via cli is not used.

	pkey := pKey{PKey: tUID, SortK: sortk}

	av, err := dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		return newDBMarshalingErr("SetCUIDpgFlag", tUID.String(), sortk, "MarshalMap", err)
	}
	//
	input := &dynamodb.GetItemInput{
		Key:                      av,
		ProjectionExpression:     expr.Projection(),
		ExpressionAttributeNames: expr.Names(),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	type Attached struct {
		Nd [][]byte
	}
	//
	// GetItem
	//
	result, err := dynSrv.GetItem(input)
	if err != nil {
		return newDBSysErr("SetCUIDpgFlag", "GetItem", err)
	}
	syslog(fmt.Sprintf("SetCUIDpgFlag: consumed capacity for GetItem: %s ", result.ConsumedCapacity))

	if len(result.Item) == 0 {
		return newDBNoItemFound("SetCUIDpgFlag", util.UID(tUID).String(), sortk, "GetItem")
	}
	//
	rec := &Attached{}
	err = dynamodbattribute.UnmarshalMap(result.Item, rec)
	if err != nil {
		return newDBUnmarshalErr("SetCUIDpgFlag", util.UID(tUID).String(), sortk, "UnmarshalMap", err)
	}
	//
	// the last entry i Attached should be the cUID that caused the item size issue
	//
	if !bytes.Equal(rec.Nd[len(rec.Nd)-1], cUID) {
		return fmt.Errorf("Data inconsistency in SetCUIDpgFlag. Last UID in Nd of overflow block doesn't match expected child UID")
	}

	idx := "XF[" + strconv.Itoa(len(rec.Nd)) + "]"

	upd := expression.Set(expression.Name(idx), expression.Value(blk.UIDdetached))
	expr, err = expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		return newDBExprErr("SetCUIDpgFlag", "", "", err)
	}
	//
	updii := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
	}
	updii = updii.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(updii)
		t1 := time.Now()
		if err != nil {
			return newDBSysErr("SetCUIDpgFlag", "UpdateItem", err)
		}
		syslog(fmt.Sprintf("SetCUIDpgFlag: consumed capacity for UpdateItem : %s  Duration: %s\n", uio.ConsumedCapacity, t1.Sub(t0)))
	}

	return nil
}

// SaveChildUIDtoOvflBlock appends cUID and XF (of ChildUID only) to overflow block
// This data is not cached.
// this function is similar to the mechanics of PropagateChildData (which deals with Scalar data)
// but is concerned with adding Child UID to the Nd and XF attributes.
func SaveChildUIDtoOvflBlock(cUID, tUID util.UID, sortk string, id int) error { //

	var (
		err    error
		expr   expression.Expression
		upd    expression.UpdateBuilder
		values map[string]*dynamodb.AttributeValue
	)

	convertSet2List := func() {
		// fix to possible sdk error/issue for Binary ListAppend operations. SDK builds
		//  a BS rather than a LIST for LISTAPPEND operation invovling binary data.
		// This is the default for binary for some reason - very odd.
		// We therefore need to convert from BS created by the SDK to LB (List Binary)
		var s strings.Builder
		for k, v := range expr.Names() {
			switch *v {
			case "Nd", "XF":
				s.WriteByte(':')
				s.WriteByte(k[1])
				// check if BS is used and then convert if it is
				var nl []*dynamodb.AttributeValue
				for i, u := range values[s.String()].BS {
					if i == 0 {
						nl = make([]*dynamodb.AttributeValue, len(values[s.String()].BS), len(values[s.String()].BS))
					}
					nl[i] = &dynamodb.AttributeValue{B: u}
					if i == len(values[s.String()].BS)-1 {
						values[s.String()] = &dynamodb.AttributeValue{L: nl} // this nils AttributeValue{B }
					}
				}
				s.Reset()
			}
		}
	}
	//
	// Marshal primary key of parent node
	//
	if param.DebugOn {
		fmt.Println("***** SaveChildUIDtoOvflBlock:  ", cUID.String(), tUID.String(), sortk)
	}
	pkey := pKey{PKey: tUID, SortK: sortk}
	av, err := dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		return newDBMarshalingErr("SaveChildUIDtoOvflBlock", "", "", "MarshalMap", err)
	}
	// add child-uid to overflow block item. Consider using SIZE on attribute ND to limit number of RCUs that need to be consumed.
	// if size was restricted to 100K or 25 RCS that woould be quicker and less costly than 400K item size.
	//
	v := make([][]byte, 1, 1)
	v[0] = []byte(cUID)
	upd = expression.Set(expression.Name("Nd"), expression.ListAppend(expression.Name("Nd"), expression.Value(v)))
	cond := expression.Name("XF").Size().LessThanEqual(expression.Value(param.OvfwBatchLimit))
	//
	// add associated flag values
	//
	xf := make([]int, 1, 1)
	xf[0] = blk.ChildUID
	upd = upd.Set(expression.Name("XF"), expression.ListAppend(expression.Name("XF"), expression.Value(xf)))
	// increment count of nodes
	upd = upd.Add(expression.Name("Cnt"), expression.Value(1))
	//
	expr, err = expression.NewBuilder().WithCondition(cond).WithUpdate(upd).Build()
	if err != nil {
		return newDBExprErr("SaveChildUIDtoOvflBlock", "", "", err)
	}
	// convert selected attribute from  Set to  List
	values = expr.Values()
	// expression.Build() will marshal array atributes as SETs rather than Lists.
	// Need to convert from BS to LIST as UpdateItem will get errr due to conflict of Set type in dynamodb.AttributeValue and List in database.
	convertSet2List()
	//
	input := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(input)
		t1 := time.Now()
		if err != nil {
			return newDBSysErr("SaveChildUIDtoOvflBlock", "UpdateItem", err)
		}
		syslog(fmt.Sprintf("SaveChildUIDtoOvflBlock: consumed updateitem capacity: %s, Duration: %s\n", uio.ConsumedCapacity, t1.Sub(t0)))
	}
	return nil

}

// TODO write the following func called from client issuing AttachNode

//func (pn *NodeCache) GetTargetBlock(sortK string, cUID util.UID) util.UID {
// AddOverflowUIDs(pn, newOfUID) - called from cache.GetTargetBlock
// sortk points to uid-pred e.g. A#G#:S,  which is the target of the data propagation
func AddOvflUIDs(di *blk.DataItem, OfUIDs []util.UID) error {

	var (
		err    error
		expr   expression.Expression
		upd    expression.UpdateBuilder
		values map[string]*dynamodb.AttributeValue
	)

	convertSet2List := func() {
		// fix to possible sdk error/issue for Binary ListAppend operations. SDK builds
		//  a BS rather than a LIST for LISTAPPEND operation invovling binary data.
		// This is the default for binary for some reason - very odd.
		// We therefore need to convert from BS created by the SDK to LB (List Binary)
		var s strings.Builder
		for k, v := range expr.Names() {
			switch *v {
			case "Nd":
				s.WriteByte(':')
				s.WriteByte(k[1])
				// check if BS is used and then convert if it is
				var nl []*dynamodb.AttributeValue
				for i, u := range values[s.String()].BS {
					if i == 0 {
						nl = make([]*dynamodb.AttributeValue, len(values[s.String()].BS), len(values[s.String()].BS))
					}
					nl[i] = &dynamodb.AttributeValue{B: u}
					if i == len(values[s.String()].BS)-1 {
						values[s.String()] = &dynamodb.AttributeValue{L: nl} // this nils AttributeValue{B }
					}
				}
				s.Reset()
			}
		}
	}
	//
	// Marshal primary key of parent node
	//
	//	pkey := pKey{PKey: di.GetPkey(), SortK: di.GetSortK()}
	pkey := pKey{PKey: di.PKey, SortK: di.SortK}
	av, err := dynamodbattribute.MarshalMap(&pkey)
	//
	if err != nil {
		return newDBMarshalingErr("AddOvflUIDs", util.UID(di.GetPkey()).String(), "", "MarshalMap", err)
	}
	// add overflow block uids
	//
	v := make([][]byte, len(OfUIDs), len(OfUIDs))
	for i := 0; i < len(OfUIDs); i++ {
		v[i] = []byte(OfUIDs[i])
	}
	upd = expression.Set(expression.Name("Nd"), expression.ListAppend(expression.Name("Nd"), expression.Value(v)))
	//
	// add associated flag values
	//
	xf := make([]int, len(OfUIDs), len(OfUIDs))
	for i := 0; i < len(OfUIDs); i++ {
		xf[i] = di.XF[i]
	}
	upd = upd.Set(expression.Name("XF"), expression.ListAppend(expression.Name("XF"), expression.Value(xf)))
	//
	// add associated item id
	//
	id := make([]int, len(OfUIDs), len(OfUIDs))
	for i := 0; i < len(OfUIDs); i++ {
		id[i] = di.Id[i]
	}
	upd = upd.Set(expression.Name("Id"), expression.ListAppend(expression.Name("Id"), expression.Value(id)))
	// increment count of nodes
	upd = upd.Add(expression.Name("Cnt"), expression.Value(len(xf)))
	//
	expr, err = expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		return newDBExprErr("PropagateChildData", "", "", err)
	}
	// convert selected attribute from  Set to  List
	values = expr.Values()
	convertSet2List()
	//
	input := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(input)
		t1 := time.Now()
		if err != nil {
			return newDBSysErr("AddOvflUIDs", "UpdateItem", err)
		}
		syslog(fmt.Sprintf("AddOvflUIDs: consumed updateitem capacity: %s, Duration: %s\n", uio.ConsumedCapacity, t1.Sub(t0)))
	}
	return nil

}

// newUIDTarget - creates a dymamo item to receive cUIDs/XF/Id data. Actual progpagated data resides in items with sortK of
// <target-sortK>#<Id>#:<scalarPred>
func newUIDTarget(tUID util.UID, sortk string, id int) (map[string]*dynamodb.AttributeValue, error) { // create dummy item with flag value of DELETED. Why? To establish Nd & XF attributes as Lists rather than Sets..

	type TargetItem struct {
		PKey  []byte
		SortK string
		Nd    [][]byte
		XF    []int
	}

	nilItem := []byte("0")
	nilUID := make([][]byte, 1, 1)
	nilUID[0] = nilItem
	//
	xf := make([]int, 1, 1)
	xf[0] = blk.UIDdetached // this is a nil (dummy) entry so mark it deleted. Used to append other cUIDs too.
	//
	// create a "batch" sortk
	//
	var s strings.Builder
	s.WriteString(sortk)
	s.WriteByte('#')
	s.WriteString(strconv.Itoa(id)) //  <target-sortK>#<Id>
	//
	a := TargetItem{PKey: tUID, SortK: s.String(), Nd: nilUID, XF: xf}
	av, err := dynamodbattribute.MarshalMap(a)
	if err != nil {
		return nil, fmt.Errorf("newUIDTarget: ", err.Error())
	}
	return av, nil
}

// db.MakeOverflowBlock(ofblk)
//func MakeOvflBlocks(ofblk []*blk.OverflowItem, di *blk.DataItem) error {
func CreateOvflBatch(tUID util.UID, sortk string, id int) error {

	convertSet2list := func(av map[string]*dynamodb.AttributeValue) {
		// fix to possible sdk error/issue for Binary ListAppend operations. SDK builds
		//  a BS rather than a LIST for LISTAPPEND operation invovling binary data.
		// This is the default for binary for some reason - very odd.
		// We therefore need to convert from BS created by the SDK to LB (List Binary)
		for k, v := range av {
			switch k {
			case "Nd":
				if len(v.BS) > 0 {
					v.L = make([]*dynamodb.AttributeValue, len(v.BS), len(v.BS))
					for i, u := range v.BS {
						v.L[i] = &dynamodb.AttributeValue{B: u}
					}
					v.BS = nil
				}
			}
		}
	}

	av, err := newUIDTarget(tUID, sortk, id)
	if err != nil {
		return err
	}
	convertSet2list(av)

	t0 := time.Now()
	ret, err := dynSrv.PutItem(&dynamodb.PutItemInput{
		TableName:              aws.String(param.GraphTable),
		Item:                   av,
		ReturnConsumedCapacity: aws.String("TOTAL"),
	})
	t1 := time.Now()
	syslog(fmt.Sprintf("CreateOvflBatch: consumed capacity for PutItem  %s. Duration: %s", ret.ConsumedCapacity, t1.Sub(t0)))
	if err != nil {
		return newDBSysErr("CreateOvflBatch", "PutItem", err)
	}

	return nil
}

// db.MakeOverflowBlock(ofblk)
//func MakeOvflBlocks(ofblk []*blk.OverflowItem, di *blk.DataItem) error {
func MakeOvflBlocks(di *blk.DataItem, ofblk []util.UID, id int) error {
	// 	ofblk := make([]*blk.OverflowItem, 2)
	// ofblk[0].Pkey = v.Encodeb64()
	// ofblk[0].SortK = pn.SortK + "P" // associated parent node
	// ofblk[0].B = pn.Pkey            // parent node to which overflow block belongs

	// ofblk[1].Pkey = v
	// ofblk[1].SortK = pn.SortK // A#G#:S

	const (
		Item1 = 0 // Item 1
		Item2 = 1 // Item 2
	)
	//
	// Initialise overflow block with two items
	//
	// Block item 1 - contains a pointer back to the parent ie.PKEY of uid-pred containing the overflow block uid (in Nd)

	// Block item 2 - an uid-pred Nd equivalent item ie. contains uids of child nodes e.g. sortk A#G#:S#<id>. id=1..n
	//        each A#G#:S#<id> contains some configured number of uids e.g. 500, 1000 etc.
	//        The id value is sourced from the parent uid-pred "Id" attribute.

	convertSet2list := func(av map[string]*dynamodb.AttributeValue) {
		// fix to possible sdk error/issue for Binary ListAppend operations. SDK builds
		//  a BS rather than a LIST for LISTAPPEND operation invovling binary data.
		// This is the default for binary for some reason - very odd.
		// We therefore need to convert from BS created by the SDK to LB (List Binary)
		for k, v := range av {
			switch k {
			case "Nd":
				if len(v.BS) > 0 {
					v.L = make([]*dynamodb.AttributeValue, len(v.BS), len(v.BS))
					for i, u := range v.BS {
						v.L[i] = &dynamodb.AttributeValue{B: u}
					}
					v.BS = nil
				}
			}
		}
	}
	//
	var (
		av  map[string]*dynamodb.AttributeValue
		err error
	)
	//
	for _, v := range ofblk {

		for i := Item1; i <= Item2; i++ {
			switch i {

			case Item1:
				type OverflowI1 struct {
					PKey  []byte
					SortK string
					B     []byte
				}
				// item to identify parent node to which overflow block belongs
				a := OverflowI1{PKey: v, SortK: "P", B: di.PKey}
				av, err = dynamodbattribute.MarshalMap(a)
				if err != nil {
					return fmt.Errorf("MakeOverflowBlock %s: %s", "Error: failed to marshal type definition ", err.Error())
				}

			case Item2:
				av, err = newUIDTarget(v, di.SortK, id) // e.g. sortk#1 A#G#:S#1
				if err != nil {
					return err
				}
				convertSet2list(av)
			}

			{
				t0 := time.Now()
				ret, err := dynSrv.PutItem(&dynamodb.PutItemInput{
					TableName:              aws.String(param.GraphTable),
					Item:                   av,
					ReturnConsumedCapacity: aws.String("TOTAL"),
				})
				t1 := time.Now()
				syslog(fmt.Sprintf("MakeOverflowBlock: consumed capacity for PutItem  %s. Duration: %s", ret.ConsumedCapacity, t1.Sub(t0)))
				if err != nil {
					return newDBSysErr("MakeOverflowBlock", "PutItem", err)
				}
			}
		}
	}
	return nil
}

// ty.   - type of child? node
// puid - parent node uid
// sortK - uidpred of parent to append value G#:S (sibling) or G#:F (friend)
// value - child value
//func firstPropagationScalarItem(ty blk.TyAttrD, pUID util.UID, sortk, sortK string, tUID util.UID, id int, value interface{}) (int, error) { //, wg ...*sync.WaitGroup) error {
func InitialisePropagationItem(ty blk.TyAttrD, pUID util.UID, sortK string, tUID util.UID, id int) (int, error) {
	// **** where does Nd, XF get updated when in Overflow mode.????

	// defer func() {
	// 	if len(wg) > 0 {
	// 		wg[0].Done()
	// 	}
	// }()

	var (
		sortk string
		lty   string
		err   error
		av    map[string]*dynamodb.AttributeValue
	)

	convertSet2list := func(av map[string]*dynamodb.AttributeValue) {
		// fix to possible sdk error/issue for Binary ListAppend operations. SDK builds
		//  a BS rather than a LIST for LISTAPPEND operation invovling binary data.
		// This is the default for binary for some reason - very odd.
		// We therefore need to convert from BS created by the SDK to LB (List Binary)
		for k, v := range av {
			switch k {
			case "LB":
				switch {
				case len(v.BS) > 0:
					v.L = make([]*dynamodb.AttributeValue, len(v.BS), len(v.BS))
					for i, u := range v.BS {
						v.L[i] = &dynamodb.AttributeValue{B: u}
					}
				}
				v.BS = nil
			}
		}
	}
	//
	// increment item id as this as this routine creates a new one
	//
	// id++ // incremented in ConfigureUpred only after OvflBlockItemSize exceeded
	//
	if bytes.Equal(pUID, tUID) {
		if ty.DT != "Nd" {
			// simple scalar e.g. Age
			lty = "L" + ty.DT
			sortk = sortK + "#:" + ty.C // TODO: currently ignoring concept of partitioning data within node block. Is that right?
		} else {
			// uid-predicate e.g. Sibling
			lty = "Nd"
			//sortk = "A#G#:" + sortK[len(sortK)-1:] // TODO: currently ignoring concept of partitioning data within node block. Is that right?
			sortk = "A#G#:" + sortK[strings.LastIndex(sortK, ":")+1:]

		}
	} else {
		if ty.DT != "Nd" {
			// simple scalar e.g. Age
			lty = "L" + ty.DT
			sortk = sortK + "#:" + ty.C + "#" + strconv.Itoa(id) // TODO: currently ignoring concept of partitioning data within node block. Is that right?
		} else {
			// uid-predicate e.g. Sibling
			lty = "Nd"
			//sortk = "A#G#:" + sortK[len(sortK)-1:] // TODO: currently ignoring concept of partitioning data within node block. Is that right?
			sortk = "A#G#:" + sortK[strings.LastIndex(sortK, ":")+1:]

		}
	}

	var pkey_ []byte
	if bytes.Equal(pUID, tUID) {
		//		pUIDb64 := pUID.Encodeb64() used when loading data via API
		//	pkey_ = pUIDb64
		pkey_ = pUID
	} else {
		pkey_ = tUID
	}
	// NULL value representation using false = not null, and true = <value is null>
	b := make([]bool, 1, 1)
	b[0] = true
	// append child attr value to parent uid-pred list
	switch lty {

	case "LI", "LF":

		type ItemLN struct {
			PKey  []byte
			SortK string
			LN    []float64
			XBl   []bool
		}
		// null value for predicate ie. not defined in item. Set value to 0 and use XB to identify as null value
		f := make([]float64, 1, 1)
		f[0] = 0
		// populate with dummy item to establish LIST
		a := ItemLN{PKey: pkey_, SortK: sortk, LN: f, XBl: b}
		av, err = dynamodbattribute.MarshalMap(a)
		if err != nil {
			return id, fmt.Errorf("XX %s: %s", "Error: failed to marshal type definition ", err.Error())
		}

	case "LBl":
		type ItemLBl struct {
			PKey  []byte
			SortK string
			LBl   []bool
			XBl   []bool
		}
		// null value for predicate ie. not defined in item. Set value to 0 and use XB to identify as null value
		f := make([]bool, 1, 1)
		f[0] = false
		// populate with dummy item to establish LIST
		a := ItemLBl{PKey: pkey_, SortK: sortk, LBl: f, XBl: b}
		av, err = dynamodbattribute.MarshalMap(a)
		if err != nil {
			return id, fmt.Errorf("XX %s: %s", "Error: failed to marshal type definition ", err.Error())
		}

	case "LS":
		type ItemLS struct {
			PKey  []byte
			SortK string
			LS    []string
			XBl   []bool
		}
		// null value for predicate ie. not defined in item. Set value to 0 and use XB to identify as null value
		f := make([]string, 1, 1)
		f[0] = "__NULL__"
		// populate with dummy item to establish LIST
		a := ItemLS{PKey: pkey_, SortK: sortk, LS: f, XBl: b}
		av, err = dynamodbattribute.MarshalMap(a)
		if err != nil {
			return id, fmt.Errorf("XX %s: %s", "Error: failed to marshal type definition ", err.Error())
		}

	case "LDT":
		type ItemLDT struct {
			PKey  []byte
			SortK string
			LDT   []string
			XBl   []bool
		}
		// null value for predicate ie. not defined in item. Set value to 0 and use XB to identify as null value
		f := make([]string, 1, 1)
		f[0] = "__NULL__"
		// populate with dummy item to establish LIST
		a := ItemLDT{PKey: pkey_, SortK: sortk, LDT: f, XBl: b}
		av, err = dynamodbattribute.MarshalMap(a)
		if err != nil {
			return id, fmt.Errorf("XX %s: %s", "Error: failed to marshal type definition ", err.Error())
		}

	case "LB":
		type ItemLI struct {
			PKey  []byte
			SortK string
			LB    [][]byte
			XBl   []bool
		}
		// null value for predicate ie. not defined in item. Set value to 0 and use XB to identify as null value
		f := make([][]byte, 1, 1)
		f[0] = []byte("__NULL__")
		// populate with dummy item to establish LIST
		a := ItemLI{PKey: pkey_, SortK: sortk, LB: f, XBl: b}
		av, err = dynamodbattribute.MarshalMap(a)
		if err != nil {
			return id, fmt.Errorf("XX %s: %s", "Error: failed to marshal type definition ", err.Error())
		}

	}
	convertSet2list(av)
	{
		t0 := time.Now()
		ret, err := dynSrv.PutItem(&dynamodb.PutItemInput{
			TableName:              aws.String(param.GraphTable),
			Item:                   av,
			ReturnConsumedCapacity: aws.String("TOTAL"),
		})
		t1 := time.Now()
		syslog(fmt.Sprintf("createPropagationScalarItem: consumed capacity for PutItem  %s. Duration: %s", ret.ConsumedCapacity, t1.Sub(t0)))
		if err != nil {
			return id, fmt.Errorf("XX Error: PutItem, %s", err.Error())
		}
	}
	// {
	// 	// add type item so type is hundled with propagated data and does not have to be queried separately
	// 	type Item struct {
	// 		PKey  []byte
	// 		SortK string
	// 		Ty    string
	// 	}
	// 	// populate with dummy item to establish LIST
	// 	sortk := sortK + "#T"
	// 	a := Item{PKey: pkey_, sortk:, Ty: Ty.Name}
	// 	av, err = dynamodbattribute.MarshalMap(a)
	// 	if err != nil {
	// 		return id, fmt.Errorf("XX %s: %s", "Error: failed to marshal type definition ", err.Error())
	// 	}
	// 	t0 := time.Now()
	// 	ret, err := dynSrv.PutItem(&dynamodb.PutItemInput{
	// 		TableName:              aws.String(param.GraphTable),
	// 		Item:                   av,
	// 		ReturnConsumedCapacity: aws.String("TOTAL"),
	// 	})
	// 	t1 := time.Now()
	// 	syslog(fmt.Sprintf("createPropagationScalarItem: consumed capacity for PutItem for type info %s. Duration: %s", ret.ConsumedCapacity, t1.Sub(t0)))
	// 	if err != nil {
	// 		return id, fmt.Errorf("XX Error: PutItem, %s", err.Error())
	// 	}
	// }

	//return PropagateChildData(ty, pUID, sortK, tUID, id, value)
	return id, err

}

// ty.   - type of child? node
// pUID - parent node uid
// sortK - uidpred of parent to append value G#:S (sibling) or G#:F (friend)
// value - child value
func PropagateChildData(ty blk.TyAttrD, pUID util.UID, sortK string, tUID util.UID, id int, value interface{}) (int, error) { //, wg ...*sync.WaitGroup) error {
	// **** where does Nd, XF get updated when in Overflow mode.????

	// defer func() {
	// 	if len(wg) > 0 {
	// 		wg[0].Done()
	// 	}
	// }()

	var (
		lty   string
		sortk string
		err   error
		expr  expression.Expression
		upd   expression.UpdateBuilder
		//xx      map[string]*dynamodb.AttributeValue
		values map[string]*dynamodb.AttributeValue
	)

	convertSet2List := func() {
		// fix to possible sdk error/issue for Binary ListAppend operations. SDK builds
		//  a BS rather than a LIST for LISTAPPEND operation invovling Binary data.
		// This is the default for binary for some reason - very odd.
		// We therefore need to convert from BS created by the SDK to LB (List Binary)
		var s strings.Builder
		for k, v := range expr.Names() {
			switch *v {
			case "Nd", "LB":
				s.WriteByte(':')
				s.WriteByte(k[1])
				// check if BS is used and then convert if it is
				if len(values[s.String()].BS) > 0 {
					nl := make([]*dynamodb.AttributeValue, 1, 1)
					nl[0] = &dynamodb.AttributeValue{B: values[s.String()].BS[0]}
					values[s.String()] = &dynamodb.AttributeValue{L: nl}
				}
				s.Reset()
			}
		}
	}
	if bytes.Equal(pUID, tUID) {
		if ty.DT != "Nd" {
			// simple scalar e.g. Age
			lty = "L" + ty.DT
			sortk = sortK + "#:" + ty.C // TODO: currently ignoring concept of partitioning data within node block. Is that right?
		} else {
			// TODO: can remove this section
			// uid-predicate e.g. Sibling
			lty = "Nd"
			//	sortk = "A#G#:" + sortK[len(sortK)-1:] // TODO: currently ignoring concept of partitioning data within node block. Is that right? Fix: this presumes single character short name
			sortk = "A#G#:" + sortK[strings.LastIndex(sortK, ":")+1:]
		}
	} else {
		if ty.DT != "Nd" {
			// simple scalar e.g. Age
			lty = "L" + ty.DT
			sortk = sortK + "#:" + ty.C + "#" + strconv.Itoa(id) // TODO: currently ignoring concept of partitioning data within node block. Is that right?
		} else {
			// TODO: can remove this section
			// uid-predicate e.g. Sibling
			lty = "Nd"
			//sortk = "A#G#:" + sortK[len(sortK)-1:] // TODO: currently ignoring concept of partitioning data within node block. Is that right? Fix: this presumes single character short name
			sortk = "A#G#:" + sortK[strings.LastIndex(sortK, ":")+1:]
		}
	}
	//
	// shadow XBl null identiier. Here null means there is no predicate specified in item, so its value is necessarily null (ie. not defined)
	//
	null := make([]bool, 1, 1)
	// no predicate value in item - set associated null flag, XBl, to true
	if value == nil {
		null[0] = true
	}
	// append child attr value to parent uid-pred list
	switch lty {

	case "LI", "LF":
		// null value for predicate ie. not defined in item. Set value to 0 and use XB to identify as null value
		if value == nil {
			null[0] = true
			switch ty.DT {
			case "I":
				value = int64(0)
			case "F":
				value = float64(0)
			}
		}
		fmt.Println("LN : ")
		switch x := value.(type) {
		case int:
			fmt.Println("LN int: ", x)
			v := make([]int, 1, 1)
			v[0] = x
			upd = expression.Set(expression.Name("LN"), expression.ListAppend(expression.Name("LN"), expression.Value(v)))
		case int32:
			fmt.Println("LN int32: ", x)
			v := make([]int32, 1, 1)
			v[0] = x
			upd = expression.Set(expression.Name("LN"), expression.ListAppend(expression.Name("LN"), expression.Value(v)))
		case int64:
			fmt.Println("LN int64: ", x)
			v := make([]int64, 1, 1)
			v[0] = x
			upd = expression.Set(expression.Name("LN"), expression.ListAppend(expression.Name("LN"), expression.Value(v)))
		case float64:
			fmt.Println("LN  float64: ", x)
			v := make([]float64, 1, 1)
			v[0] = x
			upd = upd.Set(expression.Name("LN"), expression.ListAppend(expression.Name("LN"), expression.Value(v)))
		// case string:
		// 	v := make([]string, 1)
		// 	v[0] = x
		// 	upd = expression.Set(expression.Name(lty), expression.ListAppend(expression.Name(lty), expression.Value(v)))
		default:
			// TODO: check if string - ok
			panic(fmt.Errorf("data type must be a number, int64, float64"))
		}
		// handle NULL values
		upd = upd.Set(expression.Name("XBl"), expression.ListAppend(expression.Name("XBl"), expression.Value(null)))
		expr, err = expression.NewBuilder().WithUpdate(upd).Build()
		if err != nil {
			return id, newDBExprErr("PropagateChildData", "", "", err)
		}

	case "LBl":
		if value == nil {
			value = false
		}
		if x, ok := value.(bool); !ok {
			logerr(fmt.Errorf("data type must be a bool"), true)
		} else {
			v := make([]bool, 1, 1)
			v[0] = x
			upd = expression.Set(expression.Name(lty), expression.ListAppend(expression.Name(lty), expression.Value(v)))
		}
		upd = upd.Set(expression.Name("XBl"), expression.ListAppend(expression.Name("XBl"), expression.Value(null)))
		expr, err = expression.NewBuilder().WithUpdate(upd).Build()
		if err != nil {
			return id, newDBExprErr("PropagateChildData", "", "", err)
		}

	case "LS":
		if value == nil {
			value = "__NULL__"
		}
		if x, ok := value.(string); !ok {
			logerr(fmt.Errorf("data type must be a string"), true)
		} else {
			v := make([]string, 1, 1)
			v[0] = x
			upd = expression.Set(expression.Name(lty), expression.ListAppend(expression.Name(lty), expression.Value(v)))
		}
		upd = upd.Set(expression.Name("XBl"), expression.ListAppend(expression.Name("XBl"), expression.Value(null)))
		expr, err = expression.NewBuilder().WithUpdate(upd).Build()
		if err != nil {
			return id, newDBExprErr("PropagateChildData", "", "", err)
		}

	case "LDT":

		if value == nil {
			value = "__NULL__"
		}
		if x, ok := value.(time.Time); !ok {
			logerr(fmt.Errorf("data type must be a time"), true)
		} else {
			v := make([]string, 1, 1)
			v[0] = x.String()
			upd = expression.Set(expression.Name(lty), expression.ListAppend(expression.Name(lty), expression.Value(v)))
		}
		upd = upd.Set(expression.Name("XBl"), expression.ListAppend(expression.Name("XBl"), expression.Value(null)))
		expr, err = expression.NewBuilder().WithUpdate(upd).Build()
		if err != nil {
			return id, newDBExprErr("PropagateChildData", "", "", err)
		}

	case "LB":
		if value == nil {
			value = []byte("__NULL__")
		}
		if x, ok := value.([]byte); !ok {
			logerr(fmt.Errorf("data type must be a byte slice"), true)
		} else {
			v := make([][]byte, 1, 1)
			v[0] = x
			upd = expression.Set(expression.Name(lty), expression.ListAppend(expression.Name(lty), expression.Value(value)))
		}
		upd = upd.Set(expression.Name("XBl"), expression.ListAppend(expression.Name("XBl"), expression.Value(null)))
		expr, err = expression.NewBuilder().WithUpdate(upd).Build()
		if err != nil {
			return id, newDBExprErr("PropagateChildData", "", "", err)
		}

		// update to Nd is performed in ConfigureUpred()
		//
		// case "Nd":
		// 	xf := make([]int, 1)
		// 	id_ := make([]int, 1)
		// 	if bytes.Equal(pUID, tUID) {
		// 		xf[0] = blk.ChildUID
		// 	} else {
		// 		xf[0] = blk.OvflBlockUID
		// 		id_[0] = 1
		// 	}
		// 	if x, ok := value.([]byte); !ok {
		// 		logerr(fmt.Errorf("data type must be a byte slice"), true)
		// 	} else {
		// 		v := make([][]byte, 1)
		// 		v[0] = x
		// 		upd = expression.Set(expression.Name("Nd"), expression.ListAppend(expression.Name("Nd"), expression.Value(v)))
		// 	}
		// 	upd = upd.Set(expression.Name("XF"), expression.ListAppend(expression.Name("XF"), expression.Value(xf)))
		// 	upd = upd.Set(expression.Name("Id"), expression.ListAppend(expression.Name("Id"), expression.Value(id_)))
		// 	//
		// 	expr, err = expression.NewBuilder().WithUpdate(upd).Build()
		// 	if err != nil {
		// 		return id, newDBExprErr("PropagateChildData", "", "", err)
		// 	}

	}
	values = expr.Values()
	// convert expression values result from binary Set to binary List
	convertSet2List()
	//
	// Marshal primary key of parent node
	//
	var pkey_ util.UID

	if bytes.Equal(pUID, tUID) {
		// pUIDb64 := pUID.Encodeb64() - when loading via CLI
		// pkey_ = []byte(pUIDb64)
		pkey_ = pUID
	} else {
		pkey_ = tUID
	}
	if param.DebugOn {
		fmt.Println("PropagateChildData: ADD CHILD DATA TO : ", pkey_, pkey_.Encodeb64())
	}
	//	pUIDb64 := pUID.Encodeb64()
	pkey := pKey{PKey: pkey_, SortK: sortk}
	av, err := dynamodbattribute.MarshalMap(&pkey)
	//
	if err != nil {
		return id, newDBMarshalingErr("PropagateChildData", pkey_.String(), "", "MarshalMap", err)
	}
	//
	input := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: values,
		UpdateExpression:          expr.Update(),
		//		ReturnValues:              aws.String("UPDATED_OLD"),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	// type UndoT struct {
	// 	Name   string
	// 	Set    string
	// 	Values map[string]*dynamodbattribute.AttributeValues
	// }
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(input)
		t1 := time.Now()
		syslog(fmt.Sprintf("PropagateChildData:consumed capacity for UpdateItem  %s.  Duration: %s", uio.ConsumedCapacity, t1.Sub(t0)))
		if err != nil {
			//TODO -  what if this is not the error?? Need to check err.
			return id, newDBSysErr("PropagateChildData", "createPropagationScalarItem", err)

		}
		//	undo.RegisterCh <- UndoT{Name: "Propagate", Set: "set #v=:v #XBl=:xbl", Values: uio.ReturnValues}
		// if seq == 1 {
		// 	return ErrItemSizeExceeded
		// }
	}

	return id, nil
}

// AddReverseEdge maintains reverse edge from child to parent
// e.g. Ross (parentnode) -> sibling -> Ian (childnode), Ian -> R%Sibling -> Ross
// sortk: e.g. A#G#:S, A#G#:F. Attachment point of paraent to which child data is copied.
//
// query: detach node connected to parent UID as a friend?
// Solution: specify field "BS" and  query condition 'contains(PBS,puid+"f")'          where f is the abreviation for friend predicate in type Person
//           if query errors then node is not attached to that predicate, so nothing to delete
//           if query returns, search returned BS and get tUID ie. BS[0][16:32]  which gives you all you need (puid,tUID) to mark {Nd, XF}as deleted.
//
// db.AddReverseEdge(eventID, seq, cUID, pUID, ptyName, sortK, tUID, &cwg)
func UpdateReverseEdge(cuid, puid, tUID util.UID, sortk string, batchId int) error {
	//
	// BS : set of binary values representing puid + tUID + sortk(last entry). Used to determine the tUID the child data saved to.
	// PBS : set of binary values representing puid + sortk (last entry). Can be used to quickly access if child is attached to parent
	pred := func(sk string) string {
		s_ := strings.SplitAfterN(sk, "#", -1) // A#G#:S#:D#3
		if len(s_) == 0 {
			panic(fmt.Errorf("buildExprValues: SortK of %q, must have at least one # separator", sk))
		}
		return s_[len(s_)-2][1:] + s_[len(s_)-1]
	}
	sortk += "#" + strconv.Itoa(batchId)
	bs := make([][]byte, 1, 1) // representing a binary set.
	bs[0] = append(puid, []byte(tUID)...)
	bs[0] = append(bs[0], pred(sortk)...) // D#3
	//
	//	pbs := make([][]byte, 1, 1) // representing a binary set.
	//	pbs[0] = append(puid, pred(sortk)...)
	//v2[0] = util.UID(v2[0]).Encodeb64_()
	//
	upd := expression.Add(expression.Name("BS"), expression.Value(bs))
	//	upd = upd.Add(expression.Name("PBS"), expression.Value(pbs))
	expr, err := expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		return newDBExprErr("AddReverseEdge", "", "", err)
	}
	//
	// Marshal primary key
	//
	pkey := pKey{PKey: cuid, SortK: "R#"}
	av, err := dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		return newDBMarshalingErr("AddReverseEdge", cuid.String(), "R#", "MarshalMap", err)
	}
	//
	input := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(input)
		t1 := time.Now()
		syslog(fmt.Sprintf("AddReverseEdge: consumed updateitem capacity: %s, Duration: %s\n", uio.ConsumedCapacity, t1.Sub(t0)))
		if err != nil {
			return newDBSysErr("AddReverseEdge", "UpdateItem", err)
		}
	}

	return nil
}

// removeReverseEdge deletes parent UID from child's R# predicate, attributes BS (PBS was removed in sential func EdgeExists()
func removeReverseEdge(cuid, puid, tUID util.UID, bs []byte) error {

	if param.DebugOn {
		fmt.Println("RemoveReverseEdge: on ", cuid, tUID)
	}
	//
	// BS : set of binary values representing puid + tUID + sortk(last entry). Used to determine the tUID the child is data is saved to.
	// PBS : set of binary values representing puid + sortk (last entry). Can be used to quickly access of child is attached to parent

	bs_ := make([][]byte, 1, 1)
	bs_[0] = bs
	//
	upd := expression.Delete(expression.Name("BS"), expression.Value(bs_))
	cond := expression.AttributeNotExists(expression.Name("PBS"))
	expr, err := expression.NewBuilder().WithCondition(cond).WithUpdate(upd).Build()
	if err != nil {
		return newDBExprErr("RemoveReverseEdge", "", "", err)
	}
	//
	// Marshal primary key, sortK
	//
	//cuid := cuid.Encodeb64() // TODO should not encode as performed by SDK but as the CLI has encoded my encoded values we must encode here to provided the double encoding required..
	pkey := pKey{PKey: cuid, SortK: "R#"}
	av, err := dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		return newDBMarshalingErr("RemoveReverseEdge", cuid.String(), "R#", "MarshalMap", err)
	}
	//
	//
	if param.DebugOn {
		for k, v := range expr.Names() {
			fmt.Println(k, *v)
		}
		fmt.Println("ExpressionAttributeValues: ", expr.Values())

		xx := expr.Update()
		fmt.Println("expr.Update(): ", *xx)
	}

	input := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(input)
		t1 := time.Now()
		syslog(fmt.Sprintf("RemoveReverseEdge: consumed updateitem capacity: %s, Duration: %s\n", uio.ConsumedCapacity, t1.Sub(t0)))
		if err != nil {
			return newDBSysErr("RemoveReverseEdge", "UpdateItem124", err)
		}
	}

	return nil
}

// EdgeExists acts as a sentinel or CEG - Concurrent event gatekeeper, to the AttachNode and DetachNode operations.
// It guarantees the event (operation + data) can only run once.
// Rather than check parent is attached to child, ie. for cUID in pUID uid-pred which may contain millions of UIDs spread over multiple overflow blocks more efficient
// to check child is attached to parent in cUID's #R attribute.
// Solution: specify field "BS" and  query condition 'contains(PBS,pUID+"f")'          where f is the short name for the uid-pred predicate - combination of two will be unique
//           if update errors then node is not attached to that parent-node-predicate, so nothing to delete
//
func EdgeExists(cuid, puid util.UID, sortk string, action byte) (bool, error) {

	if param.DebugOn {
		fmt.Println("In EdgeExists: on ", cuid, puid, sortk)
	}
	//
	fmt.Println("In EdgeExists: on ", cuid, puid, sortk)
	var (
		upd  expression.UpdateBuilder
		cond expression.ConditionBuilder
		eav  map[string]*dynamodb.AttributeValue
	)
	// pred := func(sk string) string {
	// 	s_ := strings.Split(sk, "#")
	// 	if len(s_) == 0 {
	// 		panic(fmt.Errorf("buildExprValues: SortK of %q, must have at least one # separator", sk))
	// 	}
	// 	return s_[len(s_)-1][1:]
	// }
	// more efficient pred...
	pred := func(sk string) string {
		i := strings.LastIndex(sk, "#")
		return sk[i+2:]
	}
	// note EdgeExists is only called on known nodes - so not necessary to check nodes exist.
	//
	// if ok, err := NodeExists(cuid); !ok {
	// 	if err != nil {
	// 		return false, fmt.Errorf("Child node %s does not exist:", cuid)
	// 	} else {
	// 		return false, fmt.Errorf("Error in NodeExists %w", err)
	// 	}
	// }
	// if ok, err := NodeExists(puid, sortk); !ok {
	// 	if err != nil {
	// 		return false, fmt.Errorf("Parent node and/or attachment predicate %s does not exist")
	// 	} else {
	// 		return false, fmt.Errorf("Error in NodeExists %w", err)
	// 	}
	// }
	//
	// if the operation is AttachNode we want to ADD the parent node onlyif parent node does not exist otherwise error
	// if the operation is DetachNode we want to DELETE parent node only if parent node exists otherwise error
	//
	switch action {

	case DELETE:
		pbs := make([][]byte, 1, 1)
		pbs[0] = append(puid, pred(sortk)...)
		var pbsC []byte
		pbsC = append(puid, pred(sortk)...)
		// bs is removed in: removeReverseEdge which requires target UID which is not availabe when EdgeExists is called
		upd = expression.Delete(expression.Name("PBS"), expression.Value(pbs))
		// Contains requires a string for second argument however we want to put a B value. Use X as dummyy to be replaced in explicit AttributeValue stmt
		cond = expression.Contains(expression.Name("PBS"), "X")
		// replace gernerated AttributeValue values with corrected ones.
		eav = map[string]*dynamodb.AttributeValue{":0": &dynamodb.AttributeValue{B: pbsC}, ":1": &dynamodb.AttributeValue{BS: pbs}}

	case ADD:
		pbs := make([][]byte, 1, 1)
		pbs[0] = append(puid, pred(sortk)...)
		var pbsC []byte
		pbsC = append(puid, pred(sortk)...)
		//
		upd = expression.Add(expression.Name("PBS"), expression.Value(pbs))
		// Contains - sdk requires a string for second argument however we want to put a B value. Use X as dummyy to be replaced by explicit AttributeValue stmt
		cond = expression.Contains(expression.Name("PBS"), "X").Not()
		// workaround: as expression will want Contains(predicate,<string>), but we are comparing Binary will change in ExpressionAttributeValue to use binary attribute value.
		// also: compare with binary value "v" which is not base64 encoded as the sdk will encode during transport and dynamodb will decode and compare UID (16byte) with UID in set.
		eav = map[string]*dynamodb.AttributeValue{":0": &dynamodb.AttributeValue{B: pbsC}, ":1": &dynamodb.AttributeValue{BS: pbs}}
	}
	//
	expr, err := expression.NewBuilder().WithCondition(cond).WithUpdate(upd).Build()
	if err != nil {
		err := newDBExprErr("EdgeExists", "", "", err)
		if action == DELETE {
			return true, err
		}
		return false, err
	}
	//
	// Marshal primary key, sortK
	//
	//	cuidb64 := cuid.Encodeb64() // TODO should not encode as performed by SDK but as the CLI has encoded my encoded values we must encode here to provided the double encoding required..
	//pkey := pKey{PKey: cuid.Encodeb64(), SortK: "R#"}
	pkey := pKey{PKey: cuid, SortK: "R#"}
	av, err := dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		err := newDBMarshalingErr("EdgeExists", cuid.String(), "R#", "MarshalMap", err)
		if action == DELETE {
			return true, err
		}
		return false, err
	}
	//

	input := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: eav,
		UpdateExpression:          expr.Update(),
		ConditionExpression:       expr.Condition(),
		ReturnValues:              aws.String("ALL_OLD"),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(input)
		t1 := time.Now()
		syslog(fmt.Sprintf("EdgeExists: consumed updateitem capacity: %s, Duration: %s\n", uio.ConsumedCapacity, t1.Sub(t0)))
		if err != nil {
			fmt.Println("UpdateItem errored", err.Error())
			if errors.Is(newDBSysErr("EdgeExists", "UpdateItem", err), ErrConditionalCheckFailed) {
				//ignore error ErrConditionalCheckFailed
				if action == DELETE {
					return false, ErrConditionalCheckFailed
				}
				return true, ErrConditionalCheckFailed
			}

			if action == DELETE {
				return true, err
			}
			return false, err
		}
	}
	if action == DELETE {
		fmt.Println("Action delete...return true")
		return true, nil
	}
	return false, nil
}

// sortK A%G%:S

// DetachNode: sentinel func is EdgeExist() which is called before DetachNode() in Client routine.
// Assumption: child is the source of relatively few (hundreds not millions) edges. A parent can have millions of children edges. Is that reasonable?
// Design:
//	1. fetch BS  (Binary Set) from child's R predicate
//       BS is made up of:  [parentUID]#[tUID]#[targetPredicateAbrev]#[ItemNumberInTargetUID]
//. 2  search BS using bs input parameter
//  3  retrieve tUID , ItemNumber from BS
//  4. update parent using tUID & optionally ItemNumber and set XF to Detached.
//  5. remove BS from child's UID "R" predicate
//. 6. node is now detached
func DetachNode(cUID, pUID util.UID, sortk string) error {
	//
	// logically delete child data XB in parent node
	//
	// 1. Query PBS, BS in child R# predicate using pUID and sortk
	// 2. BS provides the tUID
	// 3. With pUID & tUID change XF flag to detached
	//
	// Locking strategy: as the child R data is not cached and  dynamo operations are repeatable it is not necessary to synchronise access.
	//                   The associated update to the parent's childs-uid-pred data also has no locking as the update is a single-data-update
	//
	sortkSplit := func(sk string) string {
		s_ := strings.Split(sk, "#")
		if len(s_) == 0 {
			panic(fmt.Errorf("buildExprValues: SortK of %q, must have at least one # separator", sk))
		}
		return s_[len(s_)-1][1:]
	}
	if param.DebugOn {
		fmt.Println("DetachNode:  cUID, pUID ", cUID.String(), pUID.String())
	}
	// TODO: check if nodes are attached - this should be performed in client pkg

	//
	// BS : set of binary values representing puid + tUID + sortk(last entry). Used to determine the tUID the child is data is saved to.
	// PBS : set of binary values representing puid + sortk (last entry). Can be used to quickly access of child is attached to parent

	pUIDb64 := pUID.Encodeb64()
	//cUIDb64 := cUID.Encodeb64()
	// TODO: remove cUIDb64 after CLI load has been replaced with RDF loag
	//pkey := pKey{PKey: cUIDb64, SortK: "R#"}

	pkey := pKey{PKey: cUID, SortK: "R#"}
	av, err := dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		return newDBMarshalingErr("DetachChild", pUIDb64.String(), sortk, "MarshalMap", err)
	}
	//
	input := &dynamodb.GetItemInput{
		Key: av,
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	type parents struct {
		BS [][]byte // binary set
		//		PBS [][]byte // binary set
	}
	var result *dynamodb.GetItemOutput

	result, err = dynSrv.GetItem(input)
	if err != nil {
		return newDBSysErr("DetachChild", "GetItem", err)
	}
	syslog(fmt.Sprintf("DetachChild: consumed capacity for GetItem: %s ", result.ConsumedCapacity))

	if len(result.Item) == 0 {
		return newDBNoItemFound("xDetachChild", cUID.String(), sortk, "GetItem")
	}
	//
	rec := &parents{}
	err = dynamodbattribute.UnmarshalMap(result.Item, rec)
	if err != nil {
		return newDBUnmarshalErr("DetachChild", cUID.String(), sortk, "UnmarshalMap", err)
	}
	//
	// find child UID and set soft delete flag in corresponding XF entry
	//
	predAbrev := sortkSplit(sortk)

	// var found bool
	found := false
	// find BS member to remove
	var bsMember []byte
	for _, v := range rec.BS {
		//	search based on pUID and target Predicate attach point (abreviated)
		if bytes.Equal(v[:16], pUID) {
			if bytes.Equal(v[32:32+len([]byte(predAbrev))], []byte(predAbrev)) {
				found = true
				bsMember = v
			}
		}
	}
	if !found {
		return fmt.Errorf("ErrNoParentAttachmentPointFound")
	}
	tUID := bsMember[16:32]
	//
	// get item-id within target UID (last component of BS)
	//
	fmt.Println("sn: ", string(bsMember[32:]))
	batchId := strings.Split(string(bsMember[32:]), "#")
	_, err = strconv.Atoi(batchId[1])
	if err != nil {
		return fmt.Errorf("DetachNode: expected item id to be a number in BS attribute %q", batchId[1])
	}
	//
	// update sortk based on target
	//
	if string(batchId[1]) != "0" {
		sortk += "#" + string(batchId[1])
	}
	fmt.Println("DetachNode based on sortk: ", sortk)
	// mark cuid as detached in parent uid-pred XF flag
	//
	proj := expression.NamesList(expression.Name("Nd"))
	expr, err := expression.NewBuilder().WithProjection(proj).Build()
	if err != nil {
		return newDBExprErr("EdgeExists", "", "", err)
	}
	// TODO: remove encoding when load of data via cli is not used.
	if bytes.Equal(tUID, pUID) {
		// pUID is  Base64 encoded twice due to CLI load
		pkey = pKey{PKey: util.UID(tUID).Encodeb64(), SortK: sortk}
	} else {
		pkey = pKey{PKey: tUID, SortK: sortk}
	}
	av, err = dynamodbattribute.MarshalMap(&pkey)
	if err != nil {
		return newDBMarshalingErr("DetachNode", pUIDb64.String(), sortk, "MarshalMap", err)
	}
	//
	input = &dynamodb.GetItemInput{
		Key:                      av,
		ProjectionExpression:     expr.Projection(),
		ExpressionAttributeNames: expr.Names(),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	type Attached struct {
		Nd [][]byte
	}
	//
	// GetItem
	//
	result, err = dynSrv.GetItem(input)
	if err != nil {
		return newDBSysErr("DetachNode", "GetItem", err)
	}
	syslog(fmt.Sprintf("DetachNode: consumed capacity for GetItem: %s ", result.ConsumedCapacity))

	if len(result.Item) == 0 {
		return newDBNoItemFound("sDetachNode", util.UID(tUID).String(), sortk, "GetItem")
	}
	//
	rec2 := &Attached{}
	err = dynamodbattribute.UnmarshalMap(result.Item, rec2)
	if err != nil {
		return newDBUnmarshalErr("DetachNode", util.UID(tUID).String(), sortk, "UnmarshalMap", err)
	}
	// find child index in Nd
	//
	var cIdx int

	//for i, v := range rec2.Nd {
	// search from end to front
	var v []byte
	for i := len(rec2.Nd); i > 0; i-- {
		v = rec2.Nd[i-1]
		fmt.Println("i,v=", i, v)
		if bytes.Equal(v, cUID) {
			cIdx = i - 1
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("Data Inconsistency: child node not found in target propagation block %s", util.UID(tUID).String())
	}
	//
	// use cIdx to update XF entry
	//
	idx := "XF[" + strconv.Itoa(cIdx) + "]"
	upd := expression.Set(expression.Name(idx), expression.Value(blk.UIDdetached))
	upd = upd.Add(expression.Name("N"), expression.Value(-1))
	expr, err = expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		return newDBExprErr("DetachNode", "", "", err)
	}
	//
	for k, v := range expr.Names() {
		fmt.Println(k, *v)
	}
	fmt.Println("ExpressionAttributeValues: ", expr.Values())
	xx := expr.Update()
	fmt.Println("expr.Update(): ", *xx)
	updii := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
	}
	updii = updii.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	//
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(updii)
		t1 := time.Now()
		if err != nil {
			return newDBSysErr("DetachNode", "UpdateItem", err)
		}
		syslog(fmt.Sprintf("DetachNode: consumed capacity for UpdateItem : %s  Duration: %s\n", uio.ConsumedCapacity, t1.Sub(t0)))
	}
	//
	// remove reverse edge uid from child node (R%<upred>)
	//
	return removeReverseEdge(cUID, pUID, tUID, bsMember)

}

// var subjTy map[string]string
// var subjUID map[string][]byte

// func MarshalRDF(input []blk.RDF) (item map[string]*dynamodb.AttributeValue, err error) {

// 	subjTy = make(map[string]string)
// 	subjUID = make(map[string][]byte)
// 	//
// 	// Marshal RDF data into dynamodb AttributeValues
// 	//
// 	itemAV := make(map[string]*dynamodb.AttributeValue)

// 	attrValue := func(ty string, iv interface{}, av_ *dynamodb.AttributeValue) *dynamodb.AttributeValue {

// 		av := &dynamodb.AttributeValue{}
// 		switch ty {

// 		case "BOOL": //  *bool `type:"boolean"`
// 			if v, ok := iv.(bool); !ok {
// 				err = fmt.Errorf("asdf")
// 			} else {
// 				//	av = &dynamodb.AttributeValue{BOOL: aws.Bool(v)}
// 				av.SetBOOL(v)
// 			}

// 		case "S": // *string `type:"string"`
// 			if v, ok := iv.(string); !ok {
// 				err = fmt.Errorf("asdf")
// 			} else {
// 				av.SetS(v)
// 			}

// 		case "SS": // []*string `type:"list"`
// 			if v, ok := iv.(string); !ok {
// 				err = fmt.Errorf("asdf")
// 			} else {
// 				av.SS = append(av_.SS, aws.String(v))
// 			}

// 		case "N": // *string `type:"string"`
// 			// when performing putitem always treat numbers as strings from source to PutItem. Save multiple conversions.
// 			if v, ok := iv.(string); !ok {
// 				err = fmt.Errorf("asdf")
// 			} else {
// 				av.SetN(v)
// 			}

// 		case "NS": // []*string `type:"list"`
// 			if v, ok := iv.(string); !ok {
// 				err = fmt.Errorf("asdf")
// 			} else {
// 				av.NS = append(av_.NS, aws.String(v))
// 			}

// 		case "B": // []byte `type:"blob"`
// 			if v, ok := iv.([]byte); !ok {
// 				err = fmt.Errorf("asdf")
// 			} else {
// 				av.SetB(v)
// 			}

// 		case "BS": // [][]byte `type:"list"`
// 			if v, ok := iv.([]byte); !ok {
// 				err = fmt.Errorf("asdf")
// 			} else {
// 				av.BS = append(av_.BS, v)
// 			}

// 		case "L": // []*AttributeValue `type:"list"`
// 			if v, ok := iv.(*dynamodb.AttributeValue); !ok {
// 				err = fmt.Errorf("asdf")
// 			} else {
// 				av.L = append(av_.L, v)
// 			}
// 		}
// 		return av
// 	}
// 	// use item in PutItemInput.Item=item
// 	var uid map[string][]byte // set of rdf anonymous node identifiers
// 	uid = make(map[string][]byte)

// 	for i, r := range input {
// 		if u, ok := uid[r.Subj]; !ok {
// 			// rdf.Subject has not been seen before
// 			// generate UID
// 			uid[r.Subj] = util.MakeUID()
// 		}

// 		if r.Pred == "__type" {

// 			at, err := FetchTy(r.Obj.Ty)
// 			if err != nil {
// 				logerr.Panic(err)
// 			}
// 			subjTy[r.Subj] = r.Obj.Ty // S, SS, N, NS, B, BS,
// 			fmt.Println(at)
// 			fmt.Printf("%#v\n", cache.TyAttrC)
// 			for k, v := range SubjTy {
// 				fmt.Printf("SubjTy: %s %s\n", k, v)
// 			}

// 		}

// 		if r.Pred == "DOB" {
// 			ty := cache.TyAttrC[buildKey()] //Person+":"+DOB
// 			t1 := time.Now()
// 			fmt.Printf("DOB type is: %s, %s \n", ty.DT, ty.Ty)
// 		}
// 		if r.pred == "Age" {
// 			ty := cache.TyAttrC[buildKey()] //ty_+":"+pred]
// 			t1 := time.Now()
// 			fmt.Printf("Age type is: %s, %s \n", ty.DT, ty.Ty)
// 		}

// 		item[r.Pred] = attrValue(r.Obj.ty, r.Obj.value, item[r.Pred])
// 	}
// 	//
// 	return
// }

// allofterms

// Syntax Example: allofterms(predicate, "space-separated term list")

// Schema Types: string

// Index Required: term

// Matches strings that have all specified terms in any order; case insensitive.

// anyofterms

// Syntax Example: anyofterms(predicate, "space-separated term list")

// Schema Types: string

// Index Required: term

// Matches strings that have any of the specified terms in any order; case insensitive.

// Regular Expressions

// Syntax Examples: regexp(predicate, /regular-expression/) or case insensitive regexp(predicate, /regular-expression/i)

// Schema Types: string

// Index Required: trigram

// Full-Text Search

// Syntax Examples: alloftext(predicate, "space-separated text") and anyoftext(predicate, "space-separated text")

// Schema Types: string

// Index Required: fulltext

// equal to

// Syntax Examples:

// eq(predicate, value)
// eq(val(varName), value)
// eq(predicate, val(varName))
// eq(count(predicate), value)
// eq(predicate, [val1, val2, ..., valN])
// eq(predicate, [$var1, "value", ..., $varN])

// Syntax Examples: for inequality IE

// IE(predicate, value)
// IE(val(varName), value)
// IE(predicate, val(varName))
// IE(count(predicate), value)
// With IE replaced by

// le less than or equal to
// lt less than
// ge greater than or equal to
// gt greather than
type AttrName = string

func GSIS(attr AttrName, lv string) ([]gsiResult, error) {
	//
	// DD determines what index to search based on Key value. Here Key is Name and DD knows its a string hence index P_S
	//
	keyC := expression.KeyAnd(expression.Key("P").Equal(expression.Value(attr)), expression.Key("S").Equal(expression.Value(lv)))
	expr, err := expression.NewBuilder().WithKeyCondition(keyC).Build()
	if err != nil {
		return nil, newDBExprErr("GSIS", attr, "", err)
	}
	//
	input := &dynamodb.QueryInput{
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}
	input = input.SetTableName(param.GraphTable).SetIndexName("P_S").SetReturnConsumedCapacity("TOTAL")
	//
	result, err := dynSrv.Query(input)
	if err != nil {
		return nil, newDBSysErr("GSIS", "Query", err)
	}
	syslog(fmt.Sprintf("GSIS:consumed capacity for Query index P_S, %s.  ItemCount %d  %d ", result.ConsumedCapacity, len(result.Items), *result.Count))
	//
	if int(*result.Count) == 0 {
		return nil, newDBNoItemFound("GSIS", attr, lv, "Query")
	}
	//
	ptR := make([]gsiResult, len(result.Items))
	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &ptR)
	if err != nil {
		return nil, newDBUnmarshalErr("GSIS", attr, lv, "UnmarshalListOfMaps", err)
	}
	//
	return ptR, nil
}

// func GSIN(attr AttrName, lv float64) []gsiResult {

// 	// keyCondition := expression.KeyAnd(expression.Key("TeamName").Equal(expression.Value("Wildcats")), expression.Key("Number").Equal(expression.Value(1)))

// 	//
// 	// DD determines what index to search based on Key value. Here Key is Name and DD knows its a string hence index P_S
// 	//
// 	keyC := expression.KeyAnd(expression.Key("P").Equal(expression.Value(attr)), expression.Key("S").Equal(expression.Value(lv)))
// 	expr, err := expression.NewBuilder().WithKeyCondition(keyC).Build()
// 	if err != nil {
// 		panic(err)
// 	}
// 	//
// 	input := &dynamodb.QueryInput{
// 		KeyConditionExpression:    expr.KeyCondition(),
// 		FilterExpression:          expr.Filter(),
// 		ExpressionAttributeNames:  expr.Names(),
// 		ExpressionAttributeValues: expr.Values(),
// 	}
// 	input = input.SetTableName(param.GraphTable).SetIndexName("P_S").SetReturnConsumedCapacity("TOTAL")
// 	//
// 	result, err := dynSrv.Query(input)
// 	if err != nil {
// 		//return taskRecT{}, fmt.Errorf("Error in Query of Tasks: " + err.Error())
// 		logerr(err)
// 		panic(err)
// 	}
// 	logerr("Query P_S GSI: \n", result.ConsumedCapacity)
// 	if int(*result.Count) == 0 {
// 		// this is caused by a goto operation exceeding EOL
// 		logerr("No data found")
// 	}
// 	ptR := make([]gsiResult, len(result.Items))
// 	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &ptR)
// 	if err != nil {
// 		logerr("Error: %s - %s", "in UnmarshalMap in loadInstructions ", err.Error())
// 	}
// 	return ptR
// }

// func GSIB(attr AttrName, lv []byte) []gsiResult {

// 	// keyCondition := expression.KeyAnd(expression.Key("TeamName").Equal(expression.Value("Wildcats")), expression.Key("Number").Equal(expression.Value(1)))

// 	//
// 	// DD determines what index to search based on Key value. Here Key is Name and DD knows its a string hence index P_S
// 	//
// 	keyC := expression.KeyAnd(expression.Key("P").Equal(expression.Value(attr)), expression.Key("S").Equal(expression.Value(lv)))
// 	expr, err := expression.NewBuilder().WithKeyCondition(keyC).Build()
// 	if err != nil {
// 		panic(err)
// 	}
// 	//
// 	input := &dynamodb.QueryInput{
// 		KeyConditionExpression:    expr.KeyCondition(),
// 		FilterExpression:          expr.Filter(),
// 		ExpressionAttributeNames:  expr.Names(),
// 		ExpressionAttributeValues: expr.Values(),
// 	}
// 	input = input.SetTableName(param.GraphTable).SetIndexName("P_B").SetReturnConsumedCapacity("TOTAL")
// 	//
// 	result, err := dynSrv.Query(input)
// 	if err != nil {
// 		//return taskRecT{}, fmt.Errorf("Error in Query of Tasks: " + err.Error())
// 		logerr(err)
// 		panic(err)
// 	}
// 	logerr("Query P_S GSI: \n", result.ConsumedCapacity)
// 	if int(*result.Count) == 0 {
// 		// this is caused by a goto operation exceeding EOL
// 		logerr("No data found")
// 	}
// 	ptR := make([]gsiResult, len(result.Items))
// 	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &ptR)
// 	if err != nil {
// 		logerr("Error: %s - %s", "in UnmarshalMap in loadInstructions ", err.Error())
// 	}
// 	return ptR
// }

//
// 	// PKey is returned as base64 encode string of the binary
// 	//
// 	fmt.Printf("[%s] %d\n", ptR[0].PKey, len(ptR))
// 	uid := ptR[0].PKey
// 	//
// 	//
// 	//
// 	keyC = expression.KeyAnd(expression.Key("P").Equal(expression.Value("Age")), expression.Key("N").Equal(expression.Value(67)))
// 	expr, err = expression.NewBuilder().WithKeyCondition(keyC).Build()
// 	if err != nil {
// 		panic(err)
// 	}
// 	//
// 	input = &dynamodb.QueryInput{
// 		KeyConditionExpression:    expr.KeyCondition(),
// 		FilterExpression:          expr.Filter(),
// 		ExpressionAttributeNames:  expr.Names(),
// 		ExpressionAttributeValues: expr.Values(),
// 	}
// 	input = input.SetTableName(param.GraphTable).SetIndexName("P_N").SetReturnConsumedCapacity("TOTAL")
// 	//
// 	result, err = dynSrv.Query(input)
// 	if err != nil {
// 		//return taskRecT{}, fmt.Errorf("Error in Query of Tasks: " + err.Error())
// 		logerr(err)
// 	}
// 	logerr("Query P_N GSI: \n", result.ConsumedCapacity)
// 	if int(*result.Count) == 0 {
// 		// this is caused by a goto operation exceeding EOL
// 		logerr("No data found")
// 	}
// 	ptR = make([]gsiResult, len(result.Items))
// 	err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &ptR)
// 	if err != nil {
// 		logerr("Error: %s - %s", "in UnmarshalMap in loadInstructions ", err.Error())
// 	}
// 	//
// 	// PKey is returned as base64 encode string of the binary
// 	//
// 	fmt.Printf("[%s] %s %d\n", ptR[0].PKey, ptR[0].SortK, len(ptR))

// 	//
// 	// use ptR.PKey to Query table
// 	//
// 	//
// 	//
// 	//
// 	{
// 		keyBeginsWith := expression.KeyBeginsWith(expression.Key("SortK"), "A")
// 		keyC := expression.KeyEqual(expression.Key("PKey"), expression.Value(uid)).And(keyBeginsWith)
// 		expr, err := expression.NewBuilder().WithKeyCondition(keyC).Build()
// 		if err != nil {
// 			panic(err)
// 		}
// 		//
// 		input = &dynamodb.QueryInput{
// 			KeyConditionExpression:    expr.KeyCondition(),
// 			FilterExpression:          expr.Filter(),
// 			ExpressionAttributeNames:  expr.Names(),
// 			ExpressionAttributeValues: expr.Values(),
// 		}
// 		input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
// 		//
// 		result, err := dynSrv.Query(input)
// 		if err != nil {
// 			//return taskRecT{}, fmt.Errorf("Error in Query of Tasks: " + err.Error())
// 			logerr(err)
// 		}
// 		logerr("Query DyGraph: \n", result.ConsumedCapacity)
// 		if int(*result.Count) == 0 {
// 			// this is caused by a goto operation exceeding EOL
// 			logerr("No data found")
// 		}
// 		item := make([]ItemCache, len(result.Items))
// 		err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &item)
// 		if err != nil {
// 			logerr("Error: %s - %s", "in UnmarshalMap in loadInstructions ", err.Error())
// 		}
// 		for _, v := range item {
// 			fmt.Println(v.SortK[2:], v.S, v.N, v.B, v.Nd, v.LN, v.LS, v.Bl, v.SS)
// 		}
// 	}

// }

// {
//   data(func: eq(name, "Alice")) { // Alice returns UIDs of a particular type, in this case type [Person]
//									consequently the following attribute names must match those in Person

//     name
//     friend @facets {
//       name
//       car @facets
//     }
//   }
// }

// type Qattr interface {
// 	Attr()
// }

// //type Scalar struct {
// // 	name
// // 	facets []Facet
// // }

// // func (s *Scalar) Attr() {}

// // type UIDs struct {
// // 	name
// // 	facets []Facet
// // }

// type facetI interface {
// 	facet()
// }

// type fstring string

// func (f fstring) facet() {}

// type fint int32

// func (f *fint) facet() {}

// type ffloat float64

// func (f *ffloat) facet() {}

// type fdatetime time.Time

// func (f fdatetime) facet() {}

// type fbool bool

// func (f fbool) facet() {}

// @facets(orderdesc: rating)
// @facets(eq(close, true) AND eq(relative, true))
// @facets(eq(close, true))

// type Facet struct {
// 	alias string
// 	name  string
// 	value facetI //  string, bool, int, float and dateTime. For int and float, only 32-bit signed integers and 64-bit floats are accepted.
// }

// // @filter(le(initial_release_date, "2000"))
// // @filter(allofterms(name@en, "jones indiana") OR allofterms(name@en, "jurassic park"))
// // Connectives AND, OR and NOT join filters and can be built into arbitrarily complex filters, such as (NOT A OR B) AND (C AND NOT (D OR E)).
// // Note that, NOT binds more tightly than AND which binds more tightly than OR.

// // filter is applied to each Nd returning true or false.
// type Filter struct {
// 	alias string
// 	expr  expression // allofterms(name@en, "jones indiana") OR allofterms(name@en, "jurassic park")   expr(arg ...argument)
// }

// func DDAttrFetch(t TypeIdent, a TyAttr) ([]AttrType, error) {
// 	var t []DDtype
// 	if t, ok := DDcache[t]; !ok {
// 		t, err = DBFetch(t)
// 		return nil, err
// 	}
// 	return t, nil
// }

// func DDAttrFacetFetch(t TypeIdent, a TyAttr) ([]AttrType, error) {
// 	var t []DDtype
// 	if t, ok := DDcache[t]; !ok {
// 		t, err = DBFetch(t)
// 		return nil, err
// 	}
// 	return t, nil
// }
