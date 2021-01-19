package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/DynamoGraph/dbConn"
	param "github.com/DynamoGraph/dygparam"
	slog "github.com/DynamoGraph/syslog"
	"github.com/DynamoGraph/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
)

type Equality int

const (
	logid = "dpDB: "
)

// api for GQL query functions

type NodeResult struct {
	PKey  util.UID
	SortK string
	Ty    string
}
type pKey struct {
	PKey  []byte
	SortK string
}

type AttrName = string

type DPResult struct {
	Ix    string
	PKey  []byte
	SortK string
	Ty    string
}

type DataT struct {
	Uid util.UID
	Eod bool
}

var (
	dynSrv *dynamodb.DynamoDB
	err    error
	//tynames   []tyNames
	//tyShortNm map[string]string
)

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

func init() {
	dynSrv = dbConn.New()
}

func ScanForDPitems(attr AttrName, dpCh chan<- DataT) {

	var keyC expression.KeyConditionBuilder
	//
	// DD determines what index to search based on Key value. Here Key is Name and DD knows its a string hence index P_S
	// Ix=X - unprocessed
	// Ix=Y - processed
	keyC = expression.KeyAnd(expression.Key("Ty").Equal(expression.Value(attr)), expression.Key("Ix").Equal(expression.Value("X")))

	expr, err := expression.NewBuilder().WithKeyCondition(keyC).Build()
	if err != nil {
		panic(newDBExprErr("ScanForDPitems", attr, "", err))
	}
	//
	input := &dynamodb.QueryInput{
		KeyConditionExpression:    expr.KeyCondition(),
		FilterExpression:          expr.Filter(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	}
	input = input.SetTableName(param.GraphTable).SetIndexName("Ty_Ix").SetReturnConsumedCapacity("TOTAL")
	//
	eof := DataT{util.UID{}, true}
	for {
		t0 := time.Now()
		result, err := dynSrv.Query(input)
		t1 := time.Now()
		if err != nil {
			panic(newDBSysErr("ScanForDPitems", "Query", err))
		}
		syslog(fmt.Sprintf("ScanForDPitems:consumed capacity for Query index P_S, %s.  ItemCount %d  Duration: %s ", result.ConsumedCapacity, len(result.Items), t1.Sub(t0)))
		//
		if int(*result.Count) == 0 {
			break
		}
		//
		dpresult := make([]DPResult, len(result.Items))
		err = dynamodbattribute.UnmarshalListOfMaps(result.Items, &dpresult)
		if err != nil {
			panic(newDBUnmarshalErr("ScanForDPitems", attr, "", "UnmarshalListOfMaps", err))
		}
		//
		var x DataT
		for _, v := range dpresult {
			x = DataT{util.UID(v.PKey), false}
			dpCh <- x
		}
		// LastEvaluatedKey map[string]*AttributeValue
		if len(result.LastEvaluatedKey) == 0 {
			break
		}
		input.ExclusiveStartKey = result.LastEvaluatedKey
		fmt.Println("LastEvaluatedKey: ")
	}

	dpCh <- eof
	return

}

func Promote(parent, uid util.UID, psortk, sortk string, data interface{}) {
	var (
		expr   expression.Expression
		upd    expression.UpdateBuilder
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
			case "Nd":
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
	switch x := data.(type) {
	case [][]byte:

		xf := make([]int, 1)
		v := make([][]byte, 1)
		v[0] = util.UID(x[1])
		upd = expression.Set(expression.Name("Nd"), expression.ListAppend(expression.Name("Nd"), expression.Value(v)))
		upd = upd.Set(expression.Name("XF"), expression.ListAppend(expression.Name("XF"), expression.Value(xf)))
		upd = upd.Set(expression.Name("Id"), expression.ListAppend(expression.Name("Id"), expression.Value(1)))
		//
		expr, err = expression.NewBuilder().WithUpdate(upd).Build()
		if err != nil {
			panic(newDBExprErr("PropagateChildData", "", "", err))
		}

	case []string:
		null := make([]bool, 1, 1)
		if data == nil {
			null[0] = true
		}
		v := make([]string, 1, 1)
		v[0] = x[1]
		upd = expression.Set(expression.Name(sortk), expression.ListAppend(expression.Name(sortk), expression.Value(v)))
		upd = upd.Set(expression.Name("XBl"), expression.ListAppend(expression.Name("XBl"), expression.Value(null)))
		expr, err = expression.NewBuilder().WithUpdate(upd).Build()
		if err != nil {
			panic(newDBExprErr("PropagateChildData", "", "", err))
		}
	}
	values = expr.Values()
	// convert expression values result from binary Set to binary List
	convertSet2List()

	//	pUIDb64 := pUID.Encodeb64()
	pkey := pKey{PKey: parent, SortK: psortk + "#" + sortk[2:]}
	av, err := dynamodbattribute.MarshalMap(&pkey)
	//
	if err != nil {
		panic(newDBMarshalingErr("PropagateChildData", uid.String(), "", "MarshalMap", err))
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
			panic(err)
			panic(newDBSysErr("PropagateChildData", "createPropagationScalarItem", err))

		}
	}

}

func InitPromoteUID(pUID util.UID, sortk string) {

	var (
		av map[string]*dynamodb.AttributeValue
	)
	convertSet2list := func(av map[string]*dynamodb.AttributeValue) {
		// fix to possible sdk error/issue for Binary ListAppend operations. SDK builds
		//  a BS rather than a LIST for LISTAPPEND operation invovling binary data.
		// This is the default for binary for some reason - very odd.
		// We therefore need to convert from BS created by the SDK to LB (List Binary)
		for k, v := range av {
			switch k {
			case "Nd":
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

	type ItemLB struct {
		PKey  []byte
		SortK string
		N     int
		Id    []int
		Nd    [][]byte
		XF    []int
	}
	// null value for predicate ie. not defined in item. Set value to 0 and use XB to identify as null value
	xf := make([]int, 1, 1)
	xf[0] = 1
	f := make([][]byte, 1, 1)
	f[0] = []byte("__NULL__")
	var n int
	id := make([]int, 1, 1)
	// populate with dummy item to establish LIST
	a := ItemLB{PKey: pUID, SortK: sortk, Nd: f, XF: xf, Id: id, N: n}
	av, err = dynamodbattribute.MarshalMap(a)
	if err != nil {
		panic(fmt.Errorf("XX %s: %s", "Error: failed to marshal type definition ", err.Error()))
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
			panic(fmt.Errorf("XX Error: PutItem, %s", err.Error()))
		}
	}

}

func InitPromoteString(pUID util.UID, sortk string) {

	var (
		av map[string]*dynamodb.AttributeValue
	)

	b := make([]bool, 1, 1)
	b[0] = true
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
	a := ItemLS{PKey: pUID, SortK: sortk, LS: f, XBl: b}
	av, err = dynamodbattribute.MarshalMap(a)
	if err != nil {
		panic(fmt.Errorf("XX %s: %s", "Error: failed to marshal type definition ", err.Error()))
	}

	{
		t0 := time.Now()
		ret, err := dynSrv.PutItem(&dynamodb.PutItemInput{
			TableName:              aws.String(param.GraphTable),
			Item:                   av,
			ReturnConsumedCapacity: aws.String("TOTAL"),
		})
		t1 := time.Now()
		syslog(fmt.Sprintf("InitPromoteString: consumed capacity for PutItem  %s. Duration: %s", ret.ConsumedCapacity, t1.Sub(t0)))
		if err != nil {
			panic(fmt.Errorf("InitPromoteString Error: PutItem, %s", err.Error()))
		}
	}
}

func PromoteUID(pUID, cUID util.UID, sortk string, nd []byte, xf int) {

	var (
		av     map[string]*dynamodb.AttributeValue
		err    error
		expr   expression.Expression
		upd    expression.UpdateBuilder
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
	xf_ := make([]int, 1)
	xf_[0] = xf
	nl := make([]int, 1)
	nl[0] = 1
	v := make([][]byte, 1)
	v[0] = nd
	upd = expression.Set(expression.Name("Nd"), expression.ListAppend(expression.Name("Nd"), expression.Value(v)))
	upd = upd.Set(expression.Name("XF"), expression.ListAppend(expression.Name("XF"), expression.Value(xf_)))
	upd = upd.Set(expression.Name("Id"), expression.ListAppend(expression.Name("Id"), expression.Value(nl)))
	//
	expr, err = expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		panic(newDBExprErr("PromoteUID", "", "", err))
	}

	values = expr.Values()
	// convert expression values result from binary Set to binary List
	convertSet2List()
	pkey := pKey{PKey: pUID, SortK: sortk}
	av, err = dynamodbattribute.MarshalMap(&pkey)
	//
	input := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: values,
		UpdateExpression:          expr.Update(),
		//		ReturnValues:              aws.String("UPDATED_OLD"),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(input)
		t1 := time.Now()
		syslog(fmt.Sprintf("PromoteUID:consumed capacity for UpdateItem  %s.  Duration: %s", uio.ConsumedCapacity, t1.Sub(t0)))
		if err != nil {
			//TODO -  what if this is not the error?? Need to check err.
			panic(newDBSysErr("PromoteUID", "createPropagationScalarItem", err))
		}
	}
}

func PromoteString(pUID, cUID util.UID, sortk string, value string, nullv bool) {

	var (
		av   map[string]*dynamodb.AttributeValue
		err  error
		expr expression.Expression
		upd  expression.UpdateBuilder
	)

	v := make([]string, 1, 1)
	v[0] = value
	upd = expression.Set(expression.Name("LS"), expression.ListAppend(expression.Name("LS"), expression.Value(v)))
	nv := make([]bool, 1, 1)
	nv[0] = nullv
	// no predicate value in item - set associated null flag, XBl, to true
	upd = upd.Set(expression.Name("XBl"), expression.ListAppend(expression.Name("XBl"), expression.Value(nv)))
	expr, err = expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		panic(newDBExprErr("PropagateChildData", "", "", err))
	}
	// convert expression values result from binary Set to binary List
	pkey := pKey{PKey: pUID, SortK: sortk}
	av, err = dynamodbattribute.MarshalMap(&pkey)
	//
	input := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
		//		ReturnValues:              aws.String("UPDATED_OLD"),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(input)
		t1 := time.Now()
		syslog(fmt.Sprintf("PromoteUID:consumed capacity for UpdateItem  %s.  Duration: %s", uio.ConsumedCapacity, t1.Sub(t0)))
		if err != nil {
			//TODO -  what if this is not the error?? Need to check err.
			panic(newDBSysErr("PromoteUID", "createPropagationScalarItem", err))
		}
	}
}

func UpdateIX(pUID util.UID) {

	var (
		av map[string]*dynamodb.AttributeValue
	)

	upd := expression.Set(expression.Name("Ix"), expression.Value("Y"))
	expr, err := expression.NewBuilder().WithUpdate(upd).Build()
	if err != nil {
		panic(newDBExprErr("UpdateIX", "", "", err))
	}
	// convert expression values result from binary Set to binary List
	pkey := pKey{PKey: pUID, SortK: "A#A#T"}
	av, err = dynamodbattribute.MarshalMap(&pkey)
	//
	input := &dynamodb.UpdateItemInput{
		Key:                       av,
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
		//		ReturnValues:              aws.String("UPDATED_OLD"),
	}
	input = input.SetTableName(param.GraphTable).SetReturnConsumedCapacity("TOTAL")
	{
		t0 := time.Now()
		uio, err := dynSrv.UpdateItem(input)
		t1 := time.Now()
		syslog(fmt.Sprintf("UpdateIX:consumed capacity for UpdateItem  %s.  Duration: %s", uio.ConsumedCapacity, t1.Sub(t0)))
		if err != nil {
			//TODO -  what if this is not the error?? Need to check err.
			panic(newDBSysErr("UpdateIX", "createPropagationScalarItem", err))
		}
	}
}
