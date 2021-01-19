package params

var GraphTable = "DyGraphOD" // can be modified by rdf.loader "i" argument

const (
	DebugOn = true
	//SysDebugOn = false

	TypesTable = "DyGTypes2"
	//
	// Parameters for:  Overflow Blocks - overflow blocks belong to a parent node. It is where the child UIDs and propagated scalar data is stored.
	//                  The overflow block is know as the target of propagation. Each overflow block is identifier by its own UUID.
	//					There are two targets for child data propagation. Either directly inot the the parent uid-pred (edge source). When this area becomes full n
	//                  (as determined by parameter EmbeddedChildNodes) child data is targeted to a selectected overflow block, kown as the target UID..
	//
	// EmbeddedChildNodes - number of cUIDs (and the assoicated propagated scalar data) stored in the paraent uid-pred attribute e.g. A#G#:S.
	// All uid-preds can be identified by the following sortk: <partitionIdentifier>#G#:<uid-pred-short-name>
	// for a parent with limited amount of scalar data the number of embedded child uids can be relatively large. For a parent
	// node with substantial scalar data this parameter should be corresponding small (< 5) to minimise the space consumed
	// within the parent block. The more space consumed by the embedded child node data the more RCUs required to read the parent Node data,
	// which will be an overhead in circumstances where child data is not required.
	EmbeddedChildNodes = 120 // prod value: 20
	// Overflow block
	//	AvailableOvflBlocks = 1 // prod value: 5
	//
	// MaxOvFlBlocks - max number of overflow blocks. Set to the desired number of concurrent reads on overflow blocks ie. the degree of parallelism required. Prod may have upto 100.
	// As each block resides in its own UUID (PKey) there shoud be little contention when reading them all in parallel. When max is reached the overflow
	// blocks are then reused with new overflow items (Identified by an ID at the end of the sortK e.g. A#G#:S#:N#3, here the id is 3)  being added to each existing block
	// There is no limit on the number of overflow items, hence no limit on the number of child nodes attached to a parent node.
	MaxOvFlBlocks = 20 // prod value : 100
	//
	// OvFlBlocksGrowBy - determines how may overflow blacks to create when there are no available blocks because they are all inUse.  Again the bigger the value the less contention
	// there will be in cases of high concurrency - ie. lots of child nodes being attached at once.
	OvFlBlocksGrowBy = 5 // prod value : 100
	//
	// OvfwBatchLimit - max number of child nodes assigned to a Overflow batch.  Value should maximise the space consumed in 4KB blocks to improve efficiency of a RCU but should limit
	// the number of RCU's required to access an individual child item during insert (an append operation), and update/delete.`
	// The limit is checked using the dynamodb SIZE function during insert of the child item into the overflow item.
	OvfwBatchLimit = 250 // Prod 100 to 500.

	ElasticSearchOn = true
)
