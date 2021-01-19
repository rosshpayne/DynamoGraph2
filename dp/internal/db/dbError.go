package db

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go/aws/awserr"
)

// var (
// 	// error categories - returned from Unwrap()
// 	NoItemFoundErr  = errors.New("no item found")
// 	SystemErr       = errors.New("DB system error")
// 	MarshalingErr   = errors.New("DB marshaling error")
// 	UnmarshalingErr = errors.New("DB unmarshaling error")
// )

type DBExprErr struct {
	routine string
	pkey    string
	sortk   string
	err     error // aws dynamo expression error,InvalidParameterError, UnsetParameterError use errors.As
}

func newDBExprErr(rt string, pk string, sk string, err error) error {
	er := &DBExprErr{routine: rt, pkey: pk, sortk: sk, err: err}
	logerr(er)
	return er
}

func (e *DBExprErr) Error() string {
	if len(e.sortk) > 0 {
		return fmt.Sprintf("Expression error in %s [%s, $s]. %s", e.routine, e.pkey, e.sortk, e.err.Error())
	}
	if len(e.pkey) > 0 {
		return fmt.Sprintf("Expression error in %s [%s]. %s", e.routine, e.pkey, e.err.Error())
	}
	return fmt.Sprintf("Expression error in %s. %s", e.routine, e.err.Error())
}

func (e *DBExprErr) Unwrap() error {
	return e.err
}

var ErrItemSizeExceeded = errors.New("Item has reached its maximum allowed size")
var ErrAttributeDoesNotExist = errors.New("An Attribute specified in the update does not exist")
var ErrConditionalCheckFailed = errors.New("Conditional Check Failed Exception")

var UidPredSizeLimitReached = errors.New("uid-predicate item limit reached")

var NodeAttached = errors.New("Node is attached")

type DBSysErr struct {
	routine string
	api     string // DB statement
	err     error  // aws database error
}

func (e *DBSysErr) Unwrap() error {
	return e.err
}

func (e *DBSysErr) Error() string {
	return fmt.Sprintf("Sytem error in %s of %s. %s", e.api, e.routine, e.err.Error())
}
func newDBSysErr(rt string, api string, err error) error {

	var aerr awserr.Error

	if errors.As(err, &aerr) {
		switch aerr.Code() {
		case "ConditionalCheckFailedException":
			err = ErrConditionalCheckFailed
		}
		switch aerr.Message() {
		case "Item size has exceeded the maximum allowed size":
			// item size has exceeded the Dynamodb 400K limit. This limit is nolonger used as a trigger point to create a new UID target item for propagation.
			err = ErrItemSizeExceeded
		case "The provided expression refers to an attribute that does not exist in the item":
			err = ErrAttributeDoesNotExist
		}
	}
	syserr := &DBSysErr{routine: rt, api: api, err: err}
	logerr(syserr)
	return syserr
}

type DBNoItemFound struct {
	routine string
	pkey    string
	sortk   string
	api     string // DB statement
}

func newDBNoItemFound(rt string, pk string, sk string, api string) error {
	e := &DBNoItemFound{routine: rt, pkey: pk, sortk: sk, api: api}
	logerr(e)
	return e
}

func (e *DBNoItemFound) Error() string {

	if e.api == "Scan" {
		return fmt.Sprintf("No item found during %s operation in %s [%q]", e.api, e.routine, e.pkey)
	}
	if len(e.sortk) > 0 {
		return fmt.Sprintf("No item found during %s in %s for Pkey %q, Sortk %q", e.api, e.routine, e.pkey, e.sortk)
	}
	return fmt.Sprintf("No item found during %s in %s for Pkey %q", e.api, e.routine, e.pkey)

}

type DBMarshalingErr struct {
	routine string
	pkey    string
	sortk   string
	api     string // DB statement
	err     error  // aws database error
}

func newDBMarshalingErr(rt string, pk string, sk string, api string, err error) error {
	e := &DBMarshalingErr{routine: rt, pkey: pk, sortk: sk, api: api, err: err}
	logerr(e)
	return e
}

func (e *DBMarshalingErr) Error() string {
	if len(e.sortk) > 0 {
		return fmt.Sprintf("Marshalling error during %s in %s. [%q, %q]. Error: ", e.api, e.routine, e.pkey, e.sortk, e.pkey, e.err.Error())
	}
	return fmt.Sprintf("Marshalling error during %s in %s. [%q]. Error: ", e.api, e.routine, e.pkey, e.err.Error())
}

func (e *DBMarshalingErr) Unwrap() error {
	return e.err
}

type DBUnmarshalErr struct {
	routine string
	pkey    string
	sortk   string
	api     string // DB statement
	err     error  // aws database error
}

func newDBUnmarshalErr(rt string, pk string, sk string, api string, err error) error {
	e := &DBUnmarshalErr{routine: rt, pkey: pk, sortk: sk, api: api, err: err}
	logerr(e)
	return e
}

func (e *DBUnmarshalErr) Error() string {
	if len(e.sortk) > 0 {
		return fmt.Sprintf("Unmarshalling error during %s in %s. [%q, %q]. Error: %s ", e.api, e.routine, e.pkey, e.sortk, e.err.Error())
	}
	return fmt.Sprintf("Unmarshalling error during %s in %s. [%q]. Error: %s ", e.api, e.routine, e.pkey, e.err.Error())
}

func (e *DBUnmarshalErr) Unwrap() error {
	return e.err
}
